//go:build !no_browser
// +build !no_browser

package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	gruntime "runtime"
	"tempest-homekit-go/pkg/weather"
	"testing"
	"time"

	cpruntime "github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// chromedpAvailable checks if Chrome/Chromium is available for headless testing.
//
// It launches a headless browser and navigates to about:blank within a 2-second
// timeout, returning false on any failure so callers can t.Skip instead of failing.
func chromedpAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, chromedp.Headless, chromedp.DisableGPU, chromedp.NoFirstRun, chromedp.NoSandbox)
	defer allocCancel()

	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	defer browserCancel()

	// Try to run a simple task
	err := chromedp.Run(browserCtx, chromedp.Tasks{
		chromedp.Navigate("about:blank"),
	})

	return err == nil
}

// TestPopoutDiagnostics navigates to the dashboard, clicks temperature, wind and pressure
// small-card charts, and captures any window.__popoutError and console logs emitted
// during popout initialization. This test is diagnostic-only and never fails; it
// reports findings via t.Log so maintainers can inspect issues.
func TestPopoutDiagnostics(t *testing.T) {
	if os.Getenv("CI") == "true" {
		t.Skip("Skipping browser test in CI environment")
	}
	// Skip if Chrome/Chromium not available (common in CI environments)
	if !chromedpAvailable() {
		t.Skip("Skipping popout diagnostics test: Chrome/Chromium not available (required for headless browser testing)")
	}

	t.Helper()
	ws := testNewWebServer(t)

	// Ensure deterministic weather data exists
	now := time.Now()
	// Create a simple observation to populate server state
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

	// serve static files from pkg/web/static
	_, thisFileStatic, _, _ := gruntime.Caller(0)
	staticDir := filepath.Join(filepath.Dir(thisFileStatic), "static")
	mux.Handle("/pkg/web/static/", http.StripPrefix("/pkg/web/static/", http.FileServer(http.Dir(staticDir))))

	// serve chart.html directly
	_, thisFile, _, _ := gruntime.Caller(0)
	chartPath := filepath.Join(filepath.Dir(thisFile), "static", "chart.html")
	if _, err := os.Stat(chartPath); err != nil {
		t.Fatalf("chart.html not found: %v", err)
	}
	mux.HandleFunc("/chart/", func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, chartPath) })

	ts := httptest.NewServer(mux)
	defer ts.Close()

	// chromedp setup
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, chromedp.Headless, chromedp.DisableGPU, chromedp.NoFirstRun, chromedp.NoSandbox)
	defer allocCancel()
	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	defer browserCancel()

	// capture console messages
	var consoleMsgs []string
	chromedp.ListenTarget(browserCtx, func(ev interface{}) {
		switch ev := ev.(type) {
		case *cpruntime.EventExceptionThrown:
			consoleMsgs = append(consoleMsgs, ev.ExceptionDetails.Text)
		case *cpruntime.EventConsoleAPICalled:
			for _, arg := range ev.Args {
				if arg.Value != nil {
					j, _ := json.Marshal(arg.Value)
					consoleMsgs = append(consoleMsgs, string(j))
				}
			}
		}
	})

	// navigate and inject local script (avoid CDN)
	if err := chromedp.Run(browserCtx, chromedp.Tasks{
		chromedp.Navigate(ts.URL),
		chromedp.WaitVisible(`#status`, chromedp.ByID),
		chromedp.Sleep(200 * time.Millisecond),
	}); err != nil {
		t.Fatalf("navigate failed: %v", err)
	}

	// read local script.js and inject
	_, thisFile2, _, _ := gruntime.Caller(0)
	injectedFrom := filepath.Join(filepath.Dir(thisFile2), "static", "script.js")
	data, err := os.ReadFile(injectedFrom)
	if err != nil {
		t.Fatalf("failed reading script.js: %v", err)
	}
	inj := `(function(){ var s=document.createElement('script'); s.type='text/javascript'; s.text = %q; document.head.appendChild(s); })()`
	if err := chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(fmt.Sprintf(inj, string(data)), nil)); err != nil {
		// fallback: just continue
		t.Logf("warning: failed to inject local script: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// helper to click and capture diagnostics for a given canvas id
	clickAndCapture := func(canvasID string) (string, []string, error) {
		// reset window.__popoutError and collected console messages
		_ = chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`window.__popoutError = null; window.__testConsole = [];`, nil))
		consoleMsgs = nil
		if err := chromedp.Run(browserCtx, chromedp.Click(`#`+canvasID, chromedp.NodeVisible)); err != nil {
			return "", nil, err
		}
		// give popout code time to run and set window.__popoutError if any
		time.Sleep(500 * time.Millisecond)
		var popErr string
		_ = chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`(function(){ try { return JSON.stringify(window.__popoutError || null); } catch(e) { return JSON.stringify({err: e.toString()}); } })()`, &popErr))
		// copy current console messages
		copied := append([]string(nil), consoleMsgs...)
		return popErr, copied, nil
	}

	// List of canvases to test
	canvases := []string{"temperature-chart", "wind-chart", "pressure-chart", "humidity-chart", "light-chart", "uv-chart"}
	for _, id := range canvases {
		popErr, logs, err := clickAndCapture(id)
		if err != nil {
			t.Logf("click %s error: %v", id, err)
			continue
		}
		t.Logf("Popout diagnostics for %s:\n  popoutError=%s\n  consoleLogs=%v", id, popErr, logs)
	}

	// The test intentionally passes — diagnostics are recorded in t.Log
}
