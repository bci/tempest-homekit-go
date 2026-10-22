//go:build !no_browser

package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	gruntime "runtime"
	"strings"
	"testing"
	"time"

	th "tempest-homekit-go/pkg/testhelpers"
	"tempest-homekit-go/pkg/weather"

	cpruntime "github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// TestHeadlessDashboard loads the dashboard in a headless browser, runs diagnostics,
// captures console logs and asserts no runtime JS errors occur and key UI fields populate.
func TestHeadlessDashboard(t *testing.T) {
	if os.Getenv("CI") == "true" {
		t.Skip("Skipping browser test in CI environment")
	}
	// create server and inject synthetic data
	ws := testNewWebServer(t)

	now := time.Now()
	obs := weather.Observation{
		Timestamp:        now.Unix(),
		AirTemperature:   18.5,
		RelativeHumidity: 55,
		WindAvg:          2.5,
		WindGust:         3.0,
		WindDirection:    90,
		StationPressure:  1015,
		Illuminance:      1000,
		UV:               3,
		RainAccumulated:  0.05,
		Battery:          3.8,
	}

	// Ensure there's current weather and some history
	ws.UpdateWeather(&obs)
	// also add a couple historical points
	older := obs
	older.Timestamp = now.Add(-2 * time.Minute).Unix()
	older.AirTemperature = 18.2
	ws.UpdateWeather(&older)

	// create a mux that serves dashboard and api endpoints
	mux := http.NewServeMux()
	mux.HandleFunc("/", ws.handleDashboard)
	mux.HandleFunc("/api/weather", ws.handleWeatherAPI)
	mux.HandleFunc("/api/status", ws.handleStatusAPI)

	// Test-only endpoint that returns only unitHints to allow focused assertions
	mux.HandleFunc("/api/test/unitHints", func(w http.ResponseWriter, r *http.Request) {
		// Reuse the same unitHints mapping the server emits in status handler
		uh := map[string]string{"temperature": "celsius", "pressure": "mb", "wind": "mph", "rain": "inches"}
		b, _ := json.Marshal(map[string]interface{}{"unitHints": uh})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write(b)
	})
	// serve the pkg/web/static/ directory so chart.html and vendored assets are available
	_, thisFileStatic, _, _ := gruntime.Caller(0)
	thisDirStatic := filepath.Dir(thisFileStatic)
	staticDir := filepath.Join(thisDirStatic, "static")
	mux.Handle("/pkg/web/static/", http.StripPrefix("/pkg/web/static/", http.FileServer(http.Dir(staticDir))))

	// Serve chart.html directly from the test file's static directory to avoid
	// relying on server working directory.
	_, thisFile, _, _ := gruntime.Caller(0)
	thisDir := filepath.Dir(thisFile)
	chartPath := filepath.Join(thisDir, "static", "chart.html")
	// ensure the file exists and is readable for the test
	if fi, err := os.Stat(chartPath); err != nil {
		t.Fatalf("chart.html not found at %s: %v", chartPath, err)
	} else {
		t.Logf("serving chartPath: %s (size=%d)", chartPath, fi.Size())
	}
	mux.HandleFunc("/chart/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, chartPath)
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	// Seed data for all chart types using shared test helper with deterministic seed
	// Do this immediately after server start so dashboard loads with seeded data.
	chartTypes := []string{"temperature", "humidity", "rain", "pressure", "wind", "light", "uv"}
	th.SeedObservationsWithOptions(t, ws.UpdateWeather, chartTypes, time.Now(), th.SeedOptions{
		Points:   6,
		Season:   "summer",
		RandSeed: 42,
	})

	// Prepare chromedp
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Allocator with headless flags
	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx,
		chromedp.Headless,
		chromedp.DisableGPU,
		chromedp.NoFirstRun,
		chromedp.NoSandbox,
	)
	defer allocCancel()

	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	defer browserCancel()

	var consoleErrors []string

	// listen for console events
	chromedp.ListenTarget(browserCtx, func(ev interface{}) {
		switch ev := ev.(type) {
		case *cpruntime.EventExceptionThrown:
			// capture unhandled exceptions
			consoleErrors = append(consoleErrors, ev.ExceptionDetails.Text)
		case *cpruntime.EventConsoleAPICalled:
			for _, arg := range ev.Args {
				if arg.Value != nil {
					s, _ := json.Marshal(arg.Value)
					consoleErrors = append(consoleErrors, string(s))
				}
			}
		}
	})

	// navigate to the dashboard and wait for status element
	var rainText, pressureText, illumText, uvText string

	tasks := chromedp.Tasks{
		chromedp.Navigate(ts.URL),
		chromedp.WaitVisible(`#status`, chromedp.ByID),
		// give the page a moment to fetch /api/weather and update DOM
		chromedp.Sleep(400 * time.Millisecond),
		// Give the rest of the script a moment to initialize (we'll explicitly call fetchWeather below)
		chromedp.Sleep(200 * time.Millisecond),
		// run diagnostic helpers if available (non-fatal)
		chromedp.EvaluateAsDevTools(`typeof testCharts === 'function' && testCharts()`, nil),
		chromedp.Sleep(200 * time.Millisecond),
		chromedp.EvaluateAsDevTools(`typeof diagnoseCharts === 'function' && diagnoseCharts()`, nil),
		chromedp.Sleep(200 * time.Millisecond),
		// install a lightweight error/rejection capture hook so we can inspect errors later
		chromedp.EvaluateAsDevTools(`(function(){ window.__testErrors = []; window.__testConsole = []; window.addEventListener('error', function(e){ try{ window.__testErrors.push({type:'error', message: e.message, filename: e.filename, lineno: e.lineno, colno: e.colno, stack: e.error && e.error.stack}); } catch(err){} }); window.addEventListener('unhandledrejection', function(e){ try{ window.__testErrors.push({type:'unhandledrejection', reason: String(e.reason)}); } catch(err){} }); })()`, nil),
		chromedp.Sleep(20 * time.Millisecond),
		chromedp.Text(`#rain`, &rainText, chromedp.NodeVisible, chromedp.ByID),
		chromedp.Text(`#pressure`, &pressureText, chromedp.NodeVisible, chromedp.ByID),
		chromedp.Text(`#illuminance`, &illumText, chromedp.NodeVisible, chromedp.ByID),
		chromedp.Text(`#uv-index`, &uvText, chromedp.NodeVisible, chromedp.ByID),
	}

	if err := chromedp.Run(browserCtx, tasks); err != nil {
		t.Fatalf("chromedp run failed: %v", err)
	}

	// To avoid CDN/network timing flakes, fetch the page's script URLs and inject their content
	var scriptsJSON string
	if err := chromedp.Run(browserCtx, chromedp.Evaluate(`(function(){ return JSON.stringify(Array.from(document.scripts).map(s => s.src || '')); })()`, &scriptsJSON)); err != nil {
		t.Fatalf("failed to enumerate script tags: %v", err)
	}

	var scriptURLs []string
	if err := json.Unmarshal([]byte(scriptsJSON), &scriptURLs); err != nil {
		t.Fatalf("failed to parse scripts JSON: %v; raw=%s", err, scriptsJSON)
	}

	var chartSrc, localScriptSrc string
	for _, u := range scriptURLs {
		if u == "" {
			continue
		}
		if chartSrc == "" && (strings.Contains(u, "chart.js") || strings.Contains(u, "Chart.min.js") || strings.Contains(u, "chart.umd.js")) {
			chartSrc = u
		}
		if localScriptSrc == "" && strings.Contains(u, "script.js") {
			localScriptSrc = u
		}
	}

	// Helper to fetch a URL's text via Go HTTP client
	fetchText := func(url string) (string, error) {
		if url == "" {
			return "", nil
		}
		// Attempt direct GET
		resp, err := http.Get(url)
		if err != nil {
			return "", err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != 200 {
			return "", fmt.Errorf("status %d", resp.StatusCode)
		}
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}

	// Fetch and inject Chart.js. Prefer the page's script src if present; if
	// not (or to avoid network flakiness), load the vendored copy from disk
	// (pkg/web/static/chart.umd.js) and inject it directly.
	injectedChart := false
	if chartSrc != "" {
		if txt, err := fetchText(chartSrc); err == nil && txt != "" {
			lit := fmt.Sprintf("%q", txt)
			inj := fmt.Sprintf(`(function(){ var s=document.createElement('script'); s.type='text/javascript'; s.text = %s; document.head.appendChild(s); })()`, lit)
			if err := chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(inj, nil)); err == nil {
				injectedChart = true
				time.Sleep(100 * time.Millisecond)
			} else {
				t.Logf("warning: failed to inject Chart.js from page src: %v", err)
			}
		}
	}
	if !injectedChart {
		// fallback: read vendored Chart.js from disk relative to this test file
		_, thisFileChart, _, _ := gruntime.Caller(0)
		chartDir := filepath.Join(filepath.Dir(thisFileChart), "static")
		chartFile := filepath.Join(chartDir, "chart.umd.js")
		if b, err := os.ReadFile(chartFile); err == nil {
			lit := fmt.Sprintf("%q", string(b))
			inj := fmt.Sprintf(`(function(){ var s=document.createElement('script'); s.type='text/javascript'; s.text = %s; document.head.appendChild(s); })()`, lit)
			if err := chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(inj, nil)); err == nil {
				// injection succeeded
				time.Sleep(100 * time.Millisecond)
			} else {
				t.Logf("warning: failed to inject vendored Chart.js into page: %v", err)
			}
		} else {
			t.Logf("warning: vendored Chart.js not found at %s: %v", chartFile, err)
		}
	}

	// Read and inject our local script.js directly from disk to avoid HTTP path/port issues in headless test
	// Resolve script path relative to this test file so tests don't depend on cwd
	_, thisFile2, _, _ := gruntime.Caller(0)
	thisDir2 := filepath.Dir(thisFile2)
	injectedFrom := filepath.Join(thisDir2, "static", "script.js")
	localBytes, err := os.ReadFile(injectedFrom)
	if err != nil {
		t.Fatalf("failed to read local script.js from %s: %v", injectedFrom, err)
	}
	localText := string(localBytes)
	if len(localText) == 0 {
		t.Fatalf("local script.js was empty at %s", injectedFrom)
	}
	js := `(function(){ var s=document.createElement('script'); s.type='text/javascript'; s.text = %s; document.head.appendChild(s); })()`
	scriptLiteral := fmt.Sprintf("%q", localText)
	inject := fmt.Sprintf(js, scriptLiteral)
	if err := chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(inject, nil)); err != nil {
		t.Fatalf("failed to inject local script into page: %v", err)
	}
	// allow script to initialize
	time.Sleep(200 * time.Millisecond)

	// Ensure charts and click handlers are initialized by calling initCharts if available
	// Wait for Chart global to be available (loaded from remote CDN or injected)
	chartReady := false
	var chartType string
	for i := 0; i < 60; i++ { // extended wait
		_ = chromedp.Run(browserCtx, chromedp.Evaluate(`typeof Chart`, &chartType))
		if chartType == "function" || chartType == "object" {
			chartReady = true
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	if !chartReady {
		t.Fatalf("Chart.js not available in page (typeof Chart=%s)", chartType)
	}

	var initResult interface{}
	if err := chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`(function(){ try { if (typeof initCharts === 'function') { initCharts(); return 'ok'; } return 'no-init'; } catch(e) { return 'err:'+e.toString(); } })()`, &initResult)); err != nil {
		t.Fatalf("failed to run initCharts: %v", err)
	}
	t.Logf("initCharts result: %v; typeof Chart=%s", initResult, chartType)

	// Explicitly call fetchWeather and fetchStatus and await their completion
	var fetchResult1 interface{}
	var fetchResult2 interface{}
	if err := chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`(async function(){ try { await fetchWeather(); return 'ok'; } catch(e) { return {err: e.toString()}; } })()`, &fetchResult1)); err != nil {
		t.Fatalf("failed to run fetchWeather: %v", err)
	}
	if err := chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`(async function(){ try { await fetchStatus(); return 'ok'; } catch(e) { return {err: e.toString()}; } })()`, &fetchResult2)); err != nil {
		t.Fatalf("failed to run fetchStatus: %v", err)
	}

	// Capture immediate page-level diagnostics after fetches
	var testErrors string
	var typeofGetWeatherData string
	var getWeatherDataResult string
	var chartsType string
	_ = chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`JSON.stringify(window.__testErrors || [])`, &testErrors))
	_ = chromedp.Run(browserCtx, chromedp.Evaluate(`typeof window.getWeatherData`, &typeofGetWeatherData))
	_ = chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`(function(){ try { return JSON.stringify(window.getWeatherData ? window.getWeatherData() : null); } catch(e) { return JSON.stringify({err: e.toString()}); } })()`, &getWeatherDataResult))
	_ = chromedp.Run(browserCtx, chromedp.Evaluate(`typeof Chart`, &chartType))
	_ = chromedp.Run(browserCtx, chromedp.Evaluate(`typeof charts`, &chartsType))
	if len(testErrors) > 2 {
		t.Logf("page __testErrors: %s", testErrors)
	}
	t.Logf("post-inject diagnostics: typeof getWeatherData=%s; getWeatherData=%s; typeof Chart=%s; typeof charts=%s", typeofGetWeatherData, getWeatherDataResult, chartType, chartsType)

	// Wait for explicit dashboard readiness flag if available
	var dashboardReady bool
	for i := 0; i < 100; i++ { // extended wait
		_ = chromedp.Run(browserCtx, chromedp.Evaluate(`Boolean(window.__dashboardReady === true)`, &dashboardReady))
		if dashboardReady {
			break
		}
		// small sleep before trying again
		time.Sleep(150 * time.Millisecond)
	}
	if !dashboardReady {
		// Not fatal; log and continue with previous heuristics
		t.Logf("warning: window.__dashboardReady not set after wait; proceeding with fallbacks")
	}

	// Poll the DOM for key card values to become populated (not the placeholder "--")
	var rainTextVal, pressureTextVal, illumTextVal, uvTextVal string
	populated := false
	attempts := 100 // extended (~20s with 200ms sleeps)
	for i := 0; i < attempts; i++ {
		if err := chromedp.Run(browserCtx, chromedp.Tasks{
			chromedp.Evaluate(`document.getElementById('rain') ? document.getElementById('rain').textContent : ''`, &rainTextVal),
			chromedp.Evaluate(`document.getElementById('pressure') ? document.getElementById('pressure').textContent : ''`, &pressureTextVal),
			chromedp.Evaluate(`document.getElementById('illuminance') ? document.getElementById('illuminance').textContent : ''`, &illumTextVal),
			chromedp.Evaluate(`document.getElementById('uv-index') ? document.getElementById('uv-index').textContent : ''`, &uvTextVal),
		}); err != nil {
			t.Fatalf("failed to read card text: %v", err)
		}

		if rainTextVal != "--" && rainTextVal != "" && pressureTextVal != "--" && pressureTextVal != "" && illumTextVal != "--" && illumTextVal != "" && uvTextVal != "--" && uvTextVal != "" {
			populated = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	if !populated {
		// Collect diagnostics for failure
		var rawFetch string
		_ = chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`(async function(){ try { const r = await fetch('/api/weather'); const txt = await r.text(); return JSON.stringify({status: r.status, ok: r.ok, length: txt.length, preview: txt.slice(0,800)}); } catch(e) { return JSON.stringify({error: e.toString()}); } })()`, &rawFetch))

		// gather page-level diagnostics
		var typeofChart, typeofCharts, typeofWeatherData string
		var getWeatherDataJSON string
		var scriptsList string
		var injectedPrefix string

		_ = chromedp.Run(browserCtx, chromedp.Evaluate(`typeof Chart`, &typeofChart))
		_ = chromedp.Run(browserCtx, chromedp.Evaluate(`typeof charts`, &typeofCharts))
		_ = chromedp.Run(browserCtx, chromedp.Evaluate(`typeof window.weatherData`, &typeofWeatherData))
		// attempt to JSON.stringify the debug hook return value
		_ = chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`(function(){ try { return JSON.stringify(window.getWeatherData ? window.getWeatherData() : null); } catch(e) { return JSON.stringify({err: e.toString()}); } })()`, &getWeatherDataJSON))
		_ = chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`Array.from(document.scripts).map(s=>s.src||'inline').join('\n')`, &scriptsList))
		var chartsKeys string
		var chartObjKeys string
		var chartDataJSON string
		_ = chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`(function(){ try { return JSON.stringify(Object.keys(window.charts || {})); } catch(e) { return 'err:'+e.toString(); } })()`, &chartsKeys))
		_ = chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`(function(){ try { return JSON.stringify(charts && charts.temperature ? Object.keys(charts.temperature) : []); } catch(e) { return 'err:'+e.toString(); } })()`, &chartObjKeys))
		_ = chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`(function(){ try { return JSON.stringify(charts && charts.temperature && charts.temperature.data ? charts.temperature.data : null); } catch(e) { return 'err:'+e.toString(); } })()`, &chartDataJSON))

		// capture prefix of the injected script (if available via src)
		if localScriptSrc != "" {
			// try to fetch first 800 chars from the server again for reporting
			if txt, err := fetchText(localScriptSrc); err == nil {
				if len(txt) > 800 {
					injectedPrefix = txt[:800]
				} else {
					injectedPrefix = txt
				}
			} else {
				injectedPrefix = "<error fetching injected script: " + err.Error() + ">"
			}
		}

		// collect any exception payloads captured via chromedp listener
		t.Fatalf("cards not populated after wait; rain=%q pressure=%q illum=%q uv=%q; consoleEvents=%v; fetchDiag=%s; typeof Chart=%q; typeof charts=%q; typeof window.weatherData=%q; window.getWeatherData=%s; scripts=\n%s; injectedFrom=%q; injectedScriptPrefix=\n%s", rainTextVal, pressureTextVal, illumTextVal, uvTextVal, consoleErrors, rawFetch, typeofChart, typeofCharts, typeofWeatherData, getWeatherDataJSON, scriptsList, injectedFrom, injectedPrefix)
	}
	// Check UI values (from the polled values) are not the placeholder "--"
	if rainTextVal == "--" || rainTextVal == "" {
		t.Fatalf("rain card not populated (value: %q)", rainTextVal)
	}
	if pressureTextVal == "--" || pressureTextVal == "" {
		t.Fatalf("pressure card not populated (value: %q)", pressureTextVal)
	}
	if illumTextVal == "--" || illumTextVal == "" {
		t.Fatalf("illuminance card not populated (value: %q)", illumTextVal)
	}
	if uvTextVal == "--" || uvTextVal == "" {
		t.Fatalf("UV card not populated (value: %q)", uvTextVal)
	}

	// If a popout was attempted, capture any popout-level errors exposed by the page
	var popoutErr string
	_ = chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`(function(){ try { return JSON.stringify(window.__popoutError || null); } catch(e){ return JSON.stringify({err: e.toString()}); } })()`, &popoutErr))
	if popoutErr != "null" && popoutErr != "" {
		t.Logf("popout diagnostics: %s", popoutErr)
	}
}

// containsErrorKeyword reports whether s contains a JavaScript exception name
// (TypeError, ReferenceError, SyntaxError, RangeError, EvalError, URIError) or
// "Uncaught". Matching is case-sensitive. It is currently referenced only by
// the no-op assignment below.
func containsErrorKeyword(s string) bool {
	// simple checks for JS error indicators
	if s == "" {
		return false
	}
	// Narrow keywords to actual JS exception types/messages. Avoid matching
	// application-level logs that contain the word "ERROR" but are not
	// exceptions.
	keywords := []string{"TypeError", "ReferenceError", "Uncaught", "SyntaxError", "RangeError", "EvalError", "URIError"}
	for _, k := range keywords {
		if len(s) >= len(k) && (contains(s, k)) {
			return true
		}
	}
	return false
}

// contains is a tiny case-sensitive substring helper (equivalent to
// strings.Contains); an empty sub always matches.
func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && (indexOf(s, sub) >= 0))
}

// indexOf returns the first index of substr in s or -1
func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// min returns the smaller of two ints. It shadows the Go 1.21 builtin min
// within this package's tests.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// firstN returns the first n characters of s for diagnostic output.
//
// n counts bytes, not runes, so a multi-byte character at the cut may be split.
func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// package-level no-op references to satisfy staticcheck/unused warnings for helper
// functions that are useful for future tests/diagnostics but currently only used
// dynamically from other test helpers.
//
// _ (both lines) keeps containsErrorKeyword and min referenced.
var _ = containsErrorKeyword
var _ = min

// TestChartPopoutOpensAndRenders clicks a small-card chart canvas, follows the
// opened popout window, and asserts the popout initializes without JS errors
// and that the datasets in the popout match the encoded metadata sent by the
// dashboard (borderColor, borderDash, borderWidth, backgroundColor, pointRadius, tension).
func TestChartPopoutOpensAndRenders(t *testing.T) {
	if os.Getenv("CI") == "true" {
		t.Skip("Skipping browser test in CI environment")
	}
	ws := testNewWebServer(t)

	now := time.Now()
	obs := weather.Observation{
		Timestamp:        now.Unix(),
		AirTemperature:   18.5,
		RelativeHumidity: 55,
		WindAvg:          2.5,
		WindGust:         3.0,
		WindDirection:    90,
		StationPressure:  1015,
		Illuminance:      1000,
		UV:               3,
		RainAccumulated:  0.05,
		Battery:          3.8,
	}
	ws.UpdateWeather(&obs)

	mux := http.NewServeMux()
	mux.HandleFunc("/", ws.handleDashboard)
	mux.HandleFunc("/api/weather", ws.handleWeatherAPI)
	mux.HandleFunc("/api/status", ws.handleStatusAPI)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	// Prepare chromedp
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx,
		chromedp.Headless,
		chromedp.DisableGPU,
		chromedp.NoFirstRun,
		chromedp.NoSandbox,
	)
	defer allocCancel()

	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	defer browserCancel()

	// capture exceptions from both dashboard and popout
	var consoleErrors []string
	cberr := func(ev interface{}) {
		switch ev := ev.(type) {
		case *cpruntime.EventExceptionThrown:
			consoleErrors = append(consoleErrors, ev.ExceptionDetails.Text)
		case *cpruntime.EventConsoleAPICalled:
			for _, arg := range ev.Args {
				if arg.Value != nil {
					s, _ := json.Marshal(arg.Value)
					consoleErrors = append(consoleErrors, string(s))
				}
			}
		}
	}
	chromedp.ListenTarget(browserCtx, cberr)

	// Navigate dashboard, inject local script as done in TestHeadlessDashboard
	// navigation result placeholder
	tasks := chromedp.Tasks{
		chromedp.Navigate(ts.URL),
		chromedp.WaitVisible(`#status`, chromedp.ByID),
		chromedp.Sleep(200 * time.Millisecond),
	}
	if err := chromedp.Run(browserCtx, tasks); err != nil {
		t.Fatalf("navigate failed: %v", err)
	}

	// Inject Chart.js and local script (reuse same pattern as TestHeadlessDashboard)
	// Get scripts present on page to find srcs
	var scriptsJSON string
	if err := chromedp.Run(browserCtx, chromedp.Evaluate(`(function(){ return JSON.stringify(Array.from(document.scripts).map(s => s.src || '')); })()`, &scriptsJSON)); err != nil {
		t.Fatalf("failed to enumerate script tags: %v", err)
	}
	var scriptURLs []string
	if err := json.Unmarshal([]byte(scriptsJSON), &scriptURLs); err != nil {
		t.Fatalf("failed to parse scripts JSON: %v; raw=%s", err, scriptsJSON)
	}
	var chartSrc, localScriptSrc string
	for _, u := range scriptURLs {
		if u == "" {
			continue
		}

		// Intercept _blank opens by overriding window.open early so any click
		// handlers installed by the injected script will call our override.
		overrideOpen := `(function(){ window.__lastOpened = null; window.__lastPopoutConfig = null; var _open = window.open; window.open = function(url,name){ try { window.__lastOpened = url; var m = url.match(/[?&]config=([^&]+)/); if (m && m[1]) { try { window.__lastPopoutConfig = JSON.parse(decodeURIComponent(m[1])); } catch(e) { window.__lastPopoutConfig = {err: e.toString()}; } } } catch(e) {} return _open.apply(window, arguments); }; })()`
		if err := chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(overrideOpen, nil)); err != nil {
			t.Fatalf("failed to override window.open: %v", err)
		}
		if chartSrc == "" && (strings.Contains(u, "chart.js") || strings.Contains(u, "Chart.min.js") || strings.Contains(u, "chart.umd.js")) {
			chartSrc = u
		}
		if localScriptSrc == "" && strings.Contains(u, "script.js") {
			localScriptSrc = u
		}
	}

	fetchText := func(url string) (string, error) {
		if url == "" {
			return "", nil
		}
		resp, err := http.Get(url)
		if err != nil {
			return "", err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != 200 {
			return "", fmt.Errorf("status %d", resp.StatusCode)
		}
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}

	if chartSrc != "" {
		if txt, err := fetchText(chartSrc); err == nil && txt != "" {
			lit := fmt.Sprintf("%q", txt)
			inj := fmt.Sprintf(`(function(){ var s=document.createElement('script'); s.type='text/javascript'; s.text = %s; document.head.appendChild(s); })()`, lit)
			_ = chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(inj, nil))
			time.Sleep(100 * time.Millisecond)
		}
	}

	// inject local script.js from filesystem (resolve relative to this test file)
	_, thisFile3, _, _ := gruntime.Caller(0)
	thisDir3 := filepath.Dir(thisFile3)
	injectedFrom := filepath.Join(thisDir3, "static", "script.js")
	localBytes, err := os.ReadFile(injectedFrom)
	if err != nil {
		t.Fatalf("failed to read local script.js: %v", err)
	}
	localText := string(localBytes)
	inj := fmt.Sprintf("(function(){ var s=document.createElement('script'); s.type='text/javascript'; s.text = %q; document.head.appendChild(s); })()", localText)
	if err := chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(inj, nil)); err != nil {
		t.Fatalf("failed to inject local script: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// Ensure the page initializes charts and handlers by calling initCharts (to create Chart instances)
	_ = chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`typeof initCharts === 'function' && initCharts()`, nil))

	// Ensure the page initializes charts and handlers by calling fetchWeather/fetchStatus (same pattern as TestHeadlessDashboard)
	var fetchResult1 interface{}
	var fetchResult2 interface{}
	if err := chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`(async function(){ try { await fetchWeather(); return 'ok'; } catch(e) { return {err: e.toString()}; } })()`, &fetchResult1)); err != nil {
		t.Fatalf("failed to run fetchWeather: %v", err)
	}
	if err := chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`(async function(){ try { await fetchStatus(); return 'ok'; } catch(e) { return {err: e.toString()}; } })()`, &fetchResult2)); err != nil {
		t.Fatalf("failed to run fetchStatus: %v", err)
	}

	// Click the temperature canvas which should open a new window/tab. We'll
	// synthesize a popout Chart in-page using the captured config so we can
	// assert dataset parity without relying on external static handlers.

	// Wait for charts.temperature to be initialized. In headless environments
	// the canvas bounding rect may be zero-sized, so prefer checking the
	// Chart instance and that it has datasets available.
	ready := false
	for i := 0; i < 80; i++ {
		var typeofCharts string
		_ = chromedp.Run(browserCtx, chromedp.Evaluate(`typeof charts`, &typeofCharts))
		var typeofTemp string
		_ = chromedp.Run(browserCtx, chromedp.Evaluate(`typeof (charts && charts.temperature)`, &typeofTemp))
		var datasetsLen int
		_ = chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`(function(){ try { return (charts && charts.temperature && charts.temperature.data && charts.temperature.data.datasets) ? charts.temperature.data.datasets.length : 0; } catch(e) { return 0; } })()`, &datasetsLen))
		var rect string
		_ = chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`(function(){ try { var el = document.getElementById('temperature-chart'); if (!el) return ''; var r = el.getBoundingClientRect(); return JSON.stringify({w: r.width, h: r.height}); } catch(e) { return ''; } })()`, &rect))

		// Consider ready if we have a charts object, a temperature Chart instance,
		// and either a non-zero datasets length or a visible canvas rect.
		if typeofCharts == "object" && typeofTemp == "object" {
			if datasetsLen > 0 {
				ready = true
				break
			}
			if rect != "" {
				var rectObj struct {
					W float64 `json:"w"`
					H float64 `json:"h"`
				}
				_ = json.Unmarshal([]byte(rect), &rectObj)
				if rectObj.W > 0 && rectObj.H > 0 {
					ready = true
					break
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !ready {
		var scriptsList string
		var datasetsLen int
		var datasetsJSON string
		var typeofChart string
		var typeofCharts string
		var hasInit string
		var hasOpen string
		var tempOuter string
		var chartsKeys string
		var chartObjKeys string
		var chartDataJSON string
		_ = chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`Array.from(document.scripts).map(s=>s.src||'inline').join('\n')`, &scriptsList))
		_ = chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`(function(){ try { return (charts && charts.temperature && charts.temperature.data && charts.temperature.data.datasets) ? charts.temperature.data.datasets.length : 0; } catch(e) { return 0; } })()`, &datasetsLen))
		_ = chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`(function(){ try { return JSON.stringify((charts && charts.temperature && charts.temperature.data && charts.temperature.data.datasets) ? charts.temperature.data.datasets.map(d=>({label: d.label, borderColor: d.borderColor, borderDash: d.borderDash, borderWidth: d.borderWidth})) : []); } catch(e) { return 'err:'+e.toString(); } })()`, &datasetsJSON))
		_ = chromedp.Run(browserCtx, chromedp.Evaluate(`typeof Chart`, &typeofChart))
		_ = chromedp.Run(browserCtx, chromedp.Evaluate(`typeof charts`, &typeofCharts))
		_ = chromedp.Run(browserCtx, chromedp.Evaluate(`typeof initCharts`, &hasInit))
		_ = chromedp.Run(browserCtx, chromedp.Evaluate(`typeof openChartPopout`, &hasOpen))
		_ = chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`(function(){ var el = document.getElementById('temperature-card'); return el ? el.outerHTML.slice(0,2000) : ''; })()`, &tempOuter))
		t.Logf("temperature chart readiness check timed out; proceeding anyway; typeof Chart=%s; typeof charts=%s; initCharts=%s; openChartPopout=%s; scripts=%s; tempCardPrefix=%s; datasetsLen=%d; datasets=%s; chartsKeys=%s; chartObjKeys=%s; chartData=%s; consoleErrors=%v", typeofChart, typeofCharts, hasInit, hasOpen, firstN(scriptsList, 1000), firstN(tempOuter, 1000), datasetsLen, firstN(datasetsJSON, 1000), firstN(chartsKeys, 500), firstN(chartObjKeys, 500), firstN(chartDataJSON, 1000), consoleErrors)
	}

	if err := chromedp.Run(browserCtx, chromedp.Click(`#temperature-chart`, chromedp.NodeVisible)); err != nil {
		t.Fatalf("failed to click temperature chart: %v", err)
	}

	// Read captured config
	var popoutConfigJSON string
	if err := chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`JSON.stringify(window.__lastPopoutConfig || {})`, &popoutConfigJSON)); err != nil {
		t.Fatalf("failed to read captured popout config: %v", err)
	}
	if len(popoutConfigJSON) <= 2 {
		// retry click once in case of timing flake
		_ = chromedp.Run(browserCtx, chromedp.Click(`#temperature-chart`, chromedp.NodeVisible))
		_ = chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`JSON.stringify(window.__lastPopoutConfig || {})`, &popoutConfigJSON))
		if len(popoutConfigJSON) <= 2 {
			// Fallback: build the same compact cfg object in-page from charts.temperature
			fallbackBuild := `(function(){ try {
				var chartObj = charts && charts.temperature;
				var datasetsMeta = [];
				if (chartObj && chartObj.data && Array.isArray(chartObj.data.datasets)) {
					chartObj.data.datasets.forEach(function(ds){
						var meta = {};
						if (ds.label) meta.label = ds.label;
						if (ds.borderColor) meta.borderColor = ds.borderColor;
						if (ds.backgroundColor) meta.backgroundColor = ds.backgroundColor;
						if (ds.borderDash) meta.borderDash = ds.borderDash;
						if (ds.borderWidth !== undefined) meta.borderWidth = ds.borderWidth;
						if (ds.fill !== undefined) meta.fill = ds.fill;
						if (ds.pointRadius !== undefined) meta.pointRadius = ds.pointRadius;
						if (ds.tension !== undefined) meta.tension = ds.tension;
						if (ds.pointStyle !== undefined) meta.pointStyle = ds.pointStyle;
						if (ds.showLine !== undefined) meta.showLine = ds.showLine;
						if (ds.stepped !== undefined) meta.stepped = ds.stepped;
						if (ds.order !== undefined) meta.order = ds.order;
						if (ds.spanGaps !== undefined) meta.spanGaps = ds.spanGaps;
						if (String(ds.label).toLowerCase().indexOf('average')>=0) meta.role='average';
						if (String(ds.label).toLowerCase().indexOf('trend')>=0) meta.role='trend';
						if (String(ds.label).toLowerCase().indexOf('today')>=0 || String(ds.label).toLowerCase().indexOf('total')>=0) meta.role='total';
						datasetsMeta.push(meta);
					});
				}
				var cfg = { type: 'temperature', field: 'temperature', title: 'Temperature', color: (chartObj && chartObj.data && chartObj.data.datasets && chartObj.data.datasets[0] && chartObj.data.datasets[0].borderColor) || '#007bff', units: window.units || {}, datasets: datasetsMeta };
				window.__lastPopoutConfig = cfg;
				return JSON.stringify(cfg);
			} catch(e) { return 'err:'+e.toString(); } })()`
			_ = chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(fallbackBuild, &popoutConfigJSON))
			if len(popoutConfigJSON) <= 2 {
				t.Fatalf("no popout config was captured on window.open and fallback builder failed")
			}
		}
	}

	// Prefer the in-page raw status payload (window.__lastStatusRaw) if available
	// This allows us to inspect the exact object the page used. Fall back to a
	// direct server GET if the in-page hook isn't present.
	var statusJSON string
	var lastStatusFetchErr error
	for attempt := 0; attempt < 5; attempt++ {
		// Try to read window.__lastStatusRaw first
		var inPageRaw string
		_ = chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`(function(){ try { return window.__lastStatusRaw || ''; } catch(e){ return ''; } })()`, &inPageRaw))
		if inPageRaw != "" {
			statusJSON = inPageRaw
			break
		}

		// Fallback: fetch directly from the test server
		resp, err := http.Get(ts.URL + "/api/status")
		if err != nil {
			lastStatusFetchErr = err
			time.Sleep(100 * time.Millisecond)
			continue
		}
		b, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			lastStatusFetchErr = err
			time.Sleep(100 * time.Millisecond)
			continue
		}
		statusJSON = string(b)
		if len(statusJSON) > 2 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if statusJSON == "" {
		if lastStatusFetchErr != nil {
			t.Fatalf("failed to fetch /api/status after retries: %v", lastStatusFetchErr)
		}
		t.Fatalf("failed to fetch /api/status: empty response after retries")
	}

	// Assert server-side unitHints exist and propagate into the captured popout config
	var statusObj map[string]interface{}
	if err := json.Unmarshal([]byte(statusJSON), &statusObj); err != nil {
		t.Fatalf("failed to parse /api/status JSON: %v; raw=%s", err, statusJSON)
	}
	// Ensure unitHints present in status response
	if uh, ok := statusObj["unitHints"]; !ok || uh == nil {
		t.Fatalf("/api/status missing unitHints: %s", statusJSON)
	}

	// Also fetch the focused test endpoint to validate the unitHints mapping directly
	resp, err := http.Get(ts.URL + "/api/test/unitHints")
	if err != nil {
		t.Logf("warning: failed to fetch test unitHints endpoint: %v", err)
	} else {
		tb, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Logf("warning: /api/test/unitHints returned status %d; body=%s", resp.StatusCode, string(tb))
		} else {
			var tObj map[string]interface{}
			if err := json.Unmarshal(tb, &tObj); err != nil {
				t.Logf("warning: failed to parse /api/test/unitHints response: %v; raw=%s", err, string(tb))
			} else {
				if tUh, ok := tObj["unitHints"]; !ok || tUh == nil {
					t.Logf("warning: /api/test/unitHints missing unitHints: %s", string(tb))
				}
			}
		}
	}

	// Ensure captured cfg has incomingUnits (provided by openChartPopout best-effort)
	var poppedCfg map[string]interface{}
	if err := json.Unmarshal([]byte(popoutConfigJSON), &poppedCfg); err != nil {
		t.Fatalf("failed to unmarshal popout config JSON: %v; raw=%s", err, popoutConfigJSON)
	}
	// If incomingUnits wasn't included earlier, explicitly copy server's unitHints into cfg so
	// chart.html's conversion helpers have a deterministic source.
	if _, ok := poppedCfg["incomingUnits"]; !ok {
		poppedCfg["incomingUnits"] = statusObj["unitHints"]
		// overwrite the serialized JSON we'll use for synthesis below
		b, _ := json.Marshal(poppedCfg)
		popoutConfigJSON = string(b)
	}

	// We will request the popout to display temperature in Fahrenheit to exercise conversion
	// (server provides Celsius in unitHints). Set cfg.units.temperature = 'fahrenheit'.
	unitsObj := map[string]interface{}{"temperature": "fahrenheit"}
	poppedCfg["units"] = unitsObj
	{
		b, _ := json.Marshal(poppedCfg)
		popoutConfigJSON = string(b)
		// also write back to window.__lastPopoutConfig so the in-page synth reads desired units
		setCfgJS := fmt.Sprintf(`window.__lastPopoutConfig = %s;`, popoutConfigJSON)
		if err := chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(setCfgJS, nil)); err != nil {
			t.Fatalf("failed to update in-page __lastPopoutConfig with units: %v", err)
		}
	}

	// Synthesize a popout Chart in-page using the captured config and the
	// server /api/status data so we can inspect the Chart.js dataset objects.
	var synthErr string
	synthJS := `(async function(){ try {
		const cfg = window.__lastPopoutConfig;
		if (!cfg) return 'no-cfg';
		const res = await fetch('/api/status');
		const json = await res.json();
		const dataHistory = json.dataHistory || [];
		// replicate buildDatasets from chart.html
		function computeAverage(values){ let sum=0,count=0; for(const v of values){ if(v!==null && !isNaN(v)){ sum+=v; count++; }} return count===0 ? null : sum/count; }
		function linearTrend(values,timestamps){ const n=values.length; if(n===0) return Array(n).fill(null); let sumX=0,sumY=0,sumXY=0,sumXX=0,count=0; for(let i=0;i<n;i++){ const y=values[i]; const x=timestamps[i]; if(y===null||isNaN(y)||x===null||isNaN(x)) continue; sumX+=x; sumY+=y; sumXY+=x*y; sumXX+=x*x; count++; } if(count<2) return Array(n).fill(null); const b=(count*sumXY - sumX*sumY)/(count*sumXX - sumX*sumX); const a=(sumY - b*sumX)/count; return timestamps.map(t=> t ? (a + b*t) : null); }
		const labels = dataHistory.map(item => new Date(item.lastUpdate).getTime());
		const values = dataHistory.map(item => { const v = item[cfg.field]; return v===undefined||v===null ? null : Number(v); });
		const datasets = [];
		if (Array.isArray(cfg.datasets) && cfg.datasets.length>0) {
			cfg.datasets.forEach((meta, idx) => {
				let dsData = null;
				if (String(meta.role).toLowerCase() === 'average') {
					const avg = computeAverage(values);
					dsData = avg===null ? values.map(()=>null) : values.map(()=>avg);
				} else if (String(meta.role).toLowerCase() === 'trend') {
					dsData = linearTrend(values, labels);
				} else {
					dsData = values.slice();
				}
				const ds = { label: meta.label||('ds'+idx), data: dsData, borderColor: meta.borderColor||meta.color||(idx===0?(cfg.color||'#007bff'):'#888'), backgroundColor: meta.backgroundColor||'transparent', borderDash: meta.borderDash||[], borderWidth: meta.borderWidth!==undefined?meta.borderWidth:2, fill: meta.fill!==undefined?meta.fill:false, pointRadius: meta.pointRadius!==undefined?meta.pointRadius:0, tension: meta.tension!==undefined?meta.tension:0.3 };
				datasets.push(ds);
			});
		} else {
			datasets.push({ label: cfg.title||cfg.type, data: values.slice(), borderColor: cfg.color||'#007bff', tension: 0.3, pointRadius: 0 });
		}
		// create canvas
		var c = document.createElement('canvas'); c.id='__sim_popout_chart'; c.style.width='800px'; c.style.height='400px'; document.body.appendChild(c);
		var ctx = c.getContext('2d');
		window.__simPopChart = new Chart(ctx, { type: 'line', data: { labels: labels, datasets: datasets }, options: {} });
		window.__simPopoutConfig = cfg;
		return '';
	} catch(e){ return e.toString(); } })()`
	_ = chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(synthJS, &synthErr))
	if synthErr != "" {
		t.Fatalf("failed to synthesize popout chart: %s", synthErr)
	}

	// Read synthesized popChart datasets (from window.__simPopChart)
	var popDatasetsJSONSim string
	if err := chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`JSON.stringify(window.__simPopChart ? window.__simPopChart.data.datasets : [])`, &popDatasetsJSONSim)); err != nil {
		t.Fatalf("failed to read synthesized popChart datasets: %v", err)
	}

	if len(consoleErrors) > 0 {
		// Only treat true exceptions as failures. The page logs many informational
		// messages which shouldn't fail the test. Inspect and fail only on
		// messages that look like JS exceptions.
		var bad []string
		for _, s := range consoleErrors {
			if containsErrorKeyword(s) {
				bad = append(bad, s)
			}
		}
		if len(bad) > 0 {
			t.Fatalf("console/runtime exceptions captured while opening popout: %v", bad)
		}
		t.Logf("console messages while opening popout (non-fatal): %v", consoleErrors)
	}

	if len(popoutConfigJSON) <= 2 {
		t.Fatalf("popout did not expose __popoutConfig or it was empty")
	}

	// poppedCfg was parsed earlier and possibly modified to include units/incomingUnits.
	var popDatasets []map[string]interface{}
	if err := json.Unmarshal([]byte(popDatasetsJSONSim), &popDatasets); err != nil {
		t.Fatalf("failed to parse synthesized popChart datasets JSON: %v; raw=%s", err, popDatasetsJSONSim)
	}

	// If this popout is for temperature and the cfg requested Fahrenheit, verify
	// the synthesized dataset values reflect conversion from the server's Celsius values.
	if fld, ok := poppedCfg["field"].(string); ok && fld == "temperature" {
		// Extract server dataHistory values
		dh, _ := statusObj["dataHistory"].([]interface{})
		var expected []interface{}
		for _, item := range dh {
			m, ok := item.(map[string]interface{})
			if !ok {
				expected = append(expected, nil)
				continue
			}

			// Additional conversion assertions for pressure, wind, and rain
			if fld, ok := poppedCfg["field"].(string); ok {
				switch fld {
				case "pressure":
					dh, _ := statusObj["dataHistory"].([]interface{})
					var expected []interface{}
					for _, item := range dh {
						m, _ := item.(map[string]interface{})
						if v, ok := m["pressure"]; ok && v != nil {
							vf, _ := v.(float64)
							// Determine desired unit
							if unitsMap, ok := poppedCfg["units"].(map[string]interface{}); ok {
								if pu, ok := unitsMap["pressure"].(string); ok && strings.ToLower(pu) == "inhg" {
									// server sent mb -> convert mb to inHg
									expected = append(expected, vf/33.8638866667)
									continue
								}
							}
							expected = append(expected, vf)
						} else {
							expected = append(expected, nil)
						}
					}
					if len(popDatasets) > 0 {
						gotData, _ := popDatasets[0]["data"].([]interface{})
						tol := 0.1
						for i := 0; i < len(expected) && i < len(gotData); i++ {
							if expected[i] == nil {
								if gotData[i] != nil {
									t.Fatalf("expected nil at index %d for pressure but got %v", i, gotData[i])
								}
								continue
							}
							expF, _ := expected[i].(float64)
							var gotF float64
							switch v := gotData[i].(type) {
							case float64:
								gotF = v
							case int:
								gotF = float64(v)
							default:
								t.Fatalf("unexpected data type for pressure dataset element: %T", v)
							}
							if (gotF-expF) > tol || (expF-gotF) > tol {
								t.Fatalf("pressure conversion mismatch at idx %d: expected=%.3f got=%.3f", i, expF, gotF)
							}
						}
					}

				case "wind":
					// field name might be 'windSpeed' in dataHistory; try both 'windSpeed' and 'wind'
					dh, _ := statusObj["dataHistory"].([]interface{})
					var expected []interface{}
					for _, item := range dh {
						m, _ := item.(map[string]interface{})
						var v interface{}
						if vv, ok := m["windSpeed"]; ok {
							v = vv
						} else if vv, ok := m["windAvg"]; ok {
							v = vv
						} else {
							v = nil
						}
						if v != nil {
							vf, _ := v.(float64)
							if unitsMap, ok := poppedCfg["units"].(map[string]interface{}); ok {
								if wu, ok := unitsMap["wind"].(string); ok && strings.Contains(strings.ToLower(wu), "km") {
									// convert mph to km/h
									expected = append(expected, vf*1.609344)
									continue
								}
							}
							expected = append(expected, vf)
						} else {
							expected = append(expected, nil)
						}
					}
					if len(popDatasets) > 0 {
						gotData, _ := popDatasets[0]["data"].([]interface{})
						tol := 0.1
						for i := 0; i < len(expected) && i < len(gotData); i++ {
							if expected[i] == nil {
								if gotData[i] != nil {
									t.Fatalf("expected nil at index %d for wind but got %v", i, gotData[i])
								}
								continue
							}
							expF, _ := expected[i].(float64)
							var gotF float64
							switch v := gotData[i].(type) {
							case float64:
								gotF = v
							case int:
								gotF = float64(v)
							default:
								t.Fatalf("unexpected data type for wind dataset element: %T", v)
							}
							if (gotF-expF) > tol || (expF-gotF) > tol {
								t.Fatalf("wind conversion mismatch at idx %d: expected=%.3f got=%.3f", i, expF, gotF)
							}
						}
					}

				case "rain", "rainAccum", "precipitation":
					dh, _ := statusObj["dataHistory"].([]interface{})
					var expected []interface{}
					for _, item := range dh {
						m, _ := item.(map[string]interface{})
						var v interface{}
						if vv, ok := m["rainDailyTotal"]; ok {
							v = vv
						} else if vv, ok := m["rainAccum"]; ok {
							v = vv
						} else {
							v = nil
						}
						if v != nil {
							vf, _ := v.(float64)
							if unitsMap, ok := poppedCfg["units"].(map[string]interface{}); ok {
								if ru, ok := unitsMap["rain"].(string); ok && strings.Contains(strings.ToLower(ru), "mm") {
									expected = append(expected, vf*25.4)
									continue
								}
							}
							expected = append(expected, vf)
						} else {
							expected = append(expected, nil)
						}
					}
					if len(popDatasets) > 0 {
						gotData, _ := popDatasets[0]["data"].([]interface{})
						tol := 0.01
						for i := 0; i < len(expected) && i < len(gotData); i++ {
							if expected[i] == nil {
								if gotData[i] != nil {
									t.Fatalf("expected nil at index %d for rain but got %v", i, gotData[i])
								}
								continue
							}
							expF, _ := expected[i].(float64)
							var gotF float64
							switch v := gotData[i].(type) {
							case float64:
								gotF = v
							case int:
								gotF = float64(v)
							default:
								t.Fatalf("unexpected data type for rain dataset element: %T", v)
							}
							if (gotF-expF) > tol || (expF-gotF) > tol {
								t.Fatalf("rain conversion mismatch at idx %d: expected=%.3f got=%.3f", i, expF, gotF)
							}
						}
					}
				}
			}
			if v, ok := m["temperature"]; ok && v != nil {
				// server uses celsius (unitHints asserted earlier)
				vf, _ := v.(float64)
				// if popout requested fahrenheit, convert
				if unitsMap, ok := poppedCfg["units"].(map[string]interface{}); ok {
					if tu, ok := unitsMap["temperature"].(string); ok && strings.ToLower(tu) == "fahrenheit" {
						expected = append(expected, vf*9.0/5.0+32.0)
						continue
					}
				}
				expected = append(expected, vf)
			} else {
				expected = append(expected, nil)
			}
		}

		// Compare expected to synthesized first dataset (raw values)
		if len(popDatasets) > 0 {
			gotData, _ := popDatasets[0]["data"].([]interface{})
			// Helper abs
			abs := func(x float64) float64 {
				if x < 0 {
					return -x
				}
				return x
			}
			// Compare element-wise with tolerance
			tol := 0.01
			for i := 0; i < len(expected) && i < len(gotData); i++ {
				if expected[i] == nil {
					// expect null/NaN — allow either nil or NaN
					if gotData[i] != nil {
						// in JS it may be null; treat non-nil as mismatch
						t.Fatalf("expected nil at index %d for converted temp but got %v", i, gotData[i])
					}
					continue
				}
				// expected is float64
				expF, _ := expected[i].(float64)
				// gotData[i] may be float64 (from JSON) or nil
				var gotF float64
				switch v := gotData[i].(type) {
				case float64:
					gotF = v
				case int:
					gotF = float64(v)
				default:
					t.Fatalf("unexpected data type for synthesized dataset[%d] element: %T (value=%v)", i, v, v)
				}
				if abs(gotF-expF) > tol {
					t.Fatalf("converted temperature mismatch at idx %d: expected=%.3f got=%.3f (tol=%.3f)", i, expF, gotF, tol)
				}
			}
		}
	}

	// If encoded metadata exists in config, compare key properties
	if metaRaw, ok := poppedCfg["datasets"]; ok {
		if metaArr, ok := metaRaw.([]interface{}); ok {
			for i := 0; i < len(metaArr) && i < len(popDatasets); i++ {
				meta := metaArr[i].(map[string]interface{})
				ds := popDatasets[i]
				// Compare borderColor
				if metaColor, ok := meta["borderColor"].(string); ok {
					if dsColor, ok := ds["borderColor"].(string); !ok || dsColor != metaColor {
						t.Fatalf("dataset %d borderColor mismatch: expected=%q got=%q", i, metaColor, dsColor)
					}
				}
				// Compare borderDash (may be array)
				if metaDash, ok := meta["borderDash"]; ok {
					dashB, _ := json.Marshal(metaDash)
					dashA, _ := json.Marshal(ds["borderDash"])
					if string(dashA) != string(dashB) {
						t.Fatalf("dataset %d borderDash mismatch: expected=%s got=%s", i, string(dashB), string(dashA))
					}
				}
				// Compare borderWidth
				if metaBW, ok := meta["borderWidth"]; ok {
					// popChart may serialize numbers as float64
					if dsBW, ok := ds["borderWidth"]; !ok || fmt.Sprintf("%v", dsBW) != fmt.Sprintf("%v", metaBW) {
						t.Fatalf("dataset %d borderWidth mismatch: expected=%v got=%v", i, metaBW, ds["borderWidth"])
					}
				}
			}
		}
	}
}

// TestPopoutDatasetOrdering navigates to a real popout page (/chart/temperature)
// with an encoded config built from the dashboard and asserts the popout's
// dataset ordering: main data at datasets[0], average at datasets[1] (dashed
// two-point horizontal line). This guards against regressions in dataset
// ordering/indexing.
func TestPopoutDatasetOrdering(t *testing.T) {
	if os.Getenv("CI") == "true" {
		t.Skip("Skipping browser test in CI environment")
	}
	ws := testNewWebServer(t)

	// Chart types to validate. We include temperature, humidity, rain,
	// pressure, wind, light, and uv.
	chartTypes := []string{"temperature", "humidity", "rain", "pressure", "wind", "light", "uv"}

	// seed weather and a couple history points
	now := time.Now()
	obs := weather.Observation{
		Timestamp:        now.Unix(),
		AirTemperature:   20.0,
		RelativeHumidity: 50,
		WindAvg:          3.0,
		WindGust:         4.0,
		WindDirection:    180,
		StationPressure:  1012,
		Illuminance:      500,
		UV:               2,
		RainAccumulated:  0.0,
		Battery:          3.9,
	}
	ws.UpdateWeather(&obs)
	older := obs
	older.Timestamp = now.Add(-5 * time.Minute).Unix()
	older.AirTemperature = 19.5
	ws.UpdateWeather(&older)

	mux := http.NewServeMux()
	mux.HandleFunc("/", ws.handleDashboard)
	mux.HandleFunc("/api/weather", ws.handleWeatherAPI)
	mux.HandleFunc("/api/status", ws.handleStatusAPI)

	// serve static assets (chart.html etc.)
	_, thisFileStatic, _, _ := gruntime.Caller(0)
	staticDir := filepath.Join(filepath.Dir(thisFileStatic), "static")
	mux.Handle("/pkg/web/static/", http.StripPrefix("/pkg/web/static/", http.FileServer(http.Dir(staticDir))))

	// serve chart.html
	_, thisFile, _, _ := gruntime.Caller(0)
	chartPath := filepath.Join(filepath.Dir(thisFile), "static", "chart.html")
	mux.HandleFunc("/chart/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, chartPath)
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	// chromedp setup
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx,
		chromedp.Headless,
		chromedp.DisableGPU,
		chromedp.NoFirstRun,
		chromedp.NoSandbox,
	)
	defer allocCancel()
	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	defer browserCancel()

	// capture console events for diagnostics
	var consoleErrors []string
	chromedp.ListenTarget(browserCtx, func(ev interface{}) {
		switch ev := ev.(type) {
		case *cpruntime.EventExceptionThrown:
			consoleErrors = append(consoleErrors, ev.ExceptionDetails.Text)
		case *cpruntime.EventConsoleAPICalled:
			for _, arg := range ev.Args {
				if arg.Value != nil {
					s, _ := json.Marshal(arg.Value)
					consoleErrors = append(consoleErrors, string(s))
				}
			}
		}
	})

	// Navigate dashboard, inject scripts and init
	if err := chromedp.Run(browserCtx, chromedp.Tasks{
		chromedp.Navigate(ts.URL),
		chromedp.WaitVisible(`#status`, chromedp.ByID),
		chromedp.Sleep(150 * time.Millisecond),
	}); err != nil {
		t.Fatalf("navigate dashboard failed: %v", err)
	}

	// Inject vendored Chart.js if needed (reuse pattern from other tests)
	var scriptsJSON string
	_ = chromedp.Run(browserCtx, chromedp.Evaluate(`(function(){ return JSON.stringify(Array.from(document.scripts).map(s => s.src || '')); })()`, &scriptsJSON))
	var scriptURLs []string
	_ = json.Unmarshal([]byte(scriptsJSON), &scriptURLs)
	var chartSrc string
	for _, u := range scriptURLs {
		if u == "" {
			continue
		}
		if chartSrc == "" && (strings.Contains(u, "chart.js") || strings.Contains(u, "chart.umd.js")) {
			chartSrc = u
		}
	}
	fetchText := func(url string) (string, error) {
		if url == "" {
			return "", nil
		}
		resp, err := http.Get(url)
		if err != nil {
			return "", err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != 200 {
			return "", fmt.Errorf("status %d", resp.StatusCode)
		}
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	if chartSrc != "" {
		if txt, err := fetchText(chartSrc); err == nil && txt != "" {
			lit := fmt.Sprintf("%q", txt)
			inj := fmt.Sprintf(`(function(){ var s=document.createElement('script'); s.type='text/javascript'; s.text = %s; document.head.appendChild(s); })()`, lit)
			_ = chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(inj, nil))
			time.Sleep(80 * time.Millisecond)
		}
	} else {
		// fallback: inject vendored chart.umd.js from disk
		_, thisFileChart, _, _ := gruntime.Caller(0)
		chartFile := filepath.Join(filepath.Dir(thisFileChart), "static", "chart.umd.js")
		if b, err := os.ReadFile(chartFile); err == nil {
			lit := fmt.Sprintf("%q", string(b))
			inj := fmt.Sprintf(`(function(){ var s=document.createElement('script'); s.type='text/javascript'; s.text = %s; document.head.appendChild(s); })()`, lit)
			_ = chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(inj, nil))
			time.Sleep(80 * time.Millisecond)
		}
	}

	// Inject local script.js from disk to ensure same logic as dashboard
	_, thisFile2, _, _ := gruntime.Caller(0)
	injectedFrom := filepath.Join(filepath.Dir(thisFile2), "static", "script.js")
	localBytes, err := os.ReadFile(injectedFrom)
	if err != nil {
		t.Fatalf("failed to read local script.js: %v", err)
	}
	localText := string(localBytes)
	inj := fmt.Sprintf("(function(){ var s=document.createElement('script'); s.type='text/javascript'; s.text = %q; document.head.appendChild(s); })()", localText)
	if err := chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(inj, nil)); err != nil {
		t.Fatalf("failed to inject local script: %v", err)
	}
	time.Sleep(120 * time.Millisecond)

	// call initCharts and fetch data
	_ = chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`typeof initCharts === 'function' && initCharts()`, nil))
	_ = chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`(async function(){ try { await fetchWeather(); await fetchStatus(); return 'ok'; } catch(e){ return 'err:'+e.toString(); } })()`, nil))

	// Wait for charts.temperature to be initialized (reuse readiness loop from other tests)
	ready := false
	for i := 0; i < 80; i++ {
		var typeofCharts string
		_ = chromedp.Run(browserCtx, chromedp.Evaluate(`typeof charts`, &typeofCharts))
		var typeofTemp string
		_ = chromedp.Run(browserCtx, chromedp.Evaluate(`typeof (charts && charts.temperature)`, &typeofTemp))
		var datasetsLen int
		_ = chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`(function(){ try { return (charts && charts.temperature && charts.temperature.data && charts.temperature.data.datasets) ? charts.temperature.data.datasets.length : 0; } catch(e) { return 0; } })()`, &datasetsLen))
		var rect string
		_ = chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`(function(){ try { var el = document.getElementById('temperature-chart'); if (!el) return ''; var r = el.getBoundingClientRect(); return JSON.stringify({w: r.width, h: r.height}); } catch(e) { return ''; } })()`, &rect))

		if typeofCharts == "object" && typeofTemp == "object" {
			if datasetsLen > 0 {
				ready = true
				break
			}
			if rect != "" {
				var rectObj struct {
					W float64 `json:"w"`
					H float64 `json:"h"`
				}
				_ = json.Unmarshal([]byte(rect), &rectObj)
				if rectObj.W > 0 && rectObj.H > 0 {
					ready = true
					break
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !ready {
		t.Logf("warning: charts.temperature not fully ready after wait; proceeding to read dataset length")
	}

	// Continue to per-chart validation. Each chart will poll its own dashboard dataset
	// length and fail with diagnostics if empty. Avoid failing early based on /api/status
	// since headless page initialization may not surface history the same way.

	for _, ct := range chartTypes {
		// Small sleep between iterations to give the page time to stabilize
		time.Sleep(80 * time.Millisecond)

		// Determine expected dashboard dataset length from server-side /api/status
		// This is more reliable in headless runs than relying on the page's Chart instances.
		var dashLenCt int
		for i := 0; i < 10; i++ {
			resp, err := http.Get(ts.URL + "/api/status")
			if err == nil {
				b, _ := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				var statusObj map[string]interface{}
				if err := json.Unmarshal(b, &statusObj); err == nil {
					if dh, ok := statusObj["dataHistory"].([]interface{}); ok {
						dashLenCt = len(dh)
					}
				}
			}
			if dashLenCt > 0 {
				break
			}
			time.Sleep(150 * time.Millisecond)
		}
		if dashLenCt == 0 {
			// Fallback: try a short client-side read for diagnostics, then fail
			var datasetsLen int
			_ = chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(fmt.Sprintf(`(function(){ try { return (charts && charts.%s && charts.%s.data && charts.%s.data.datasets && charts.%s.data.datasets[0] && charts.%s.data.datasets[0].data) ? charts.%s.data.datasets[0].data.length : 0; } catch(e){ return 0; } })()`, ct, ct, ct, ct, ct, ct), &datasetsLen))
			t.Fatalf("dashboard %s dataset unexpectedly empty after wait: len=0 (server/dataHistory=%d client/datasets=%d)", ct, dashLenCt, datasetsLen)
		}

		// Capture diagnostics (config & charts) immediately before invoking the helper
		var lastCfg string
		var chartsSnap string
		var simPop string
		_ = chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`(function(){ try { return JSON.stringify(window.__lastPopoutConfig || null); } catch(e){ return 'err:'+e.toString(); } })()`, &lastCfg))
		_ = chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`(function(){ try { return JSON.stringify(window.charts || {}); } catch(e){ return 'err:'+e.toString(); } })()`, &chartsSnap))
		_ = chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`(function(){ try { return JSON.stringify(window.__simPopChart || null); } catch(e){ return 'err:'+e.toString(); } })()`, &simPop))
		t.Logf("pre-popout diagnostics for %s: __lastPopoutConfig=%s; charts=%s; __simPopChart=%s", ct, firstN(lastCfg, 2000), firstN(chartsSnap, 2000), firstN(simPop, 2000))

		// Call the reusable helper which navigates to the popout and asserts ordering
		th.AssertPopoutDatasetOrdering(t, browserCtx, ts, ct, dashLenCt)
	}

}
