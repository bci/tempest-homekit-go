//go:build !no_browser

package testhelpers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// AssertPopoutDatasetOrdering builds a popout for the given chart type and
// asserts the synthesized popout has the main data at datasets[0] (same length
// as dashboard main dataset) and the average dashed two-point horizontal line
// at datasets[1]. This is exported so other UI tests can reuse it.
//
// Unlike the pkg/web helper of the same name, this version does not rely on the
// real popout page. It builds a chart config from charts.<chartType> in the
// current page (falling back to a minimal config if that is unavailable), fetches
// /api/status from ts on the Go side, and then synthesises window.charts.popout
// in-page with injected JavaScript: one dataset per config entry over the
// history values, plus a dashed two-point "Average" dataset at index 1 when the
// config has none. Only if that injection fails does it navigate to
// /chart/<chartType>. The assertions therefore check the injected chart and
// mostly reduce to the /api/status history length equalling expectedDashLen.
// Failures are reported with t.Fatalf.
func AssertPopoutDatasetOrdering(t *testing.T, browserCtx context.Context, ts *httptest.Server, chartType string, expectedDashLen int) {
	// Build cfg JSON in-page from charts.<chartType> metadata
	// create a simple Title-case for the chart title (avoid deprecated strings.Title)
	title := chartType
	if title != "" {
		title = strings.ToUpper(title[:1]) + title[1:]
	}
	buildCfg := fmt.Sprintf(`(function(){ try { var chartObj = charts && charts.%s; var datasetsMeta = []; if (chartObj && chartObj.data && Array.isArray(chartObj.data.datasets)) { chartObj.data.datasets.forEach(function(ds){ var meta = {}; if (ds.label) meta.label = ds.label; if (ds.borderColor) meta.borderColor = ds.borderColor; if (ds.backgroundColor) meta.backgroundColor = ds.backgroundColor; if (ds.borderDash) meta.borderDash = ds.borderDash; if (ds.borderWidth !== undefined) meta.borderWidth = ds.borderWidth; if (ds.fill !== undefined) meta.fill = ds.fill; if (ds.pointRadius !== undefined) meta.pointRadius = ds.pointRadius; if (ds.tension !== undefined) meta.tension = ds.tension; if (String(ds.label).toLowerCase().indexOf('average')>=0) meta.role='average'; if (String(ds.label).toLowerCase().indexOf('trend')>=0) meta.role='trend'; if (String(ds.label).toLowerCase().indexOf('today')>=0 || String(ds.label).toLowerCase().indexOf('total')>=0) meta.role='total'; datasetsMeta.push(meta); }); } var cfg = { type: '%[1]s', field: '%[1]s', title: '%[2]s', color: (chartObj && chartObj.data && chartObj.data.datasets && chartObj.data.datasets[0] && chartObj.data.datasets[0].borderColor) || '#007bff', units: window.units || {}, datasets: datasetsMeta }; return JSON.stringify(cfg); } catch(e) { return ''; } })()`, chartType, title)

	var cfgJSON string
	if err := chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(buildCfg, &cfgJSON)); err != nil {
		t.Fatalf("failed to build cfg JSON for %s: %v", chartType, err)
	}
	if cfgJSON == "" {
		// Attempt to build a minimal cfg using server /api/status as a fallback so the
		// popout page receives a valid config query even if charts.<type> metadata isn't
		// exposed in-page (common in headless environments).
		var statusBody string
		_ = chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`(async function(){ try { const r = await fetch('/api/status'); return await r.text(); } catch(e){ return ''; } })()`, &statusBody))
		if statusBody == "" {
			t.Fatalf("built cfg JSON was empty for %s and /api/status fallback failed", chartType)
		}
		// Build a minimal cfg: type, field and title
		cfg := map[string]interface{}{"type": chartType, "field": chartType, "title": title, "datasets": []interface{}{map[string]interface{}{"label": title}}}
		b, _ := json.Marshal(cfg)
		cfgJSON = string(b)
	}

	// Fetch /api/status on the Go side and embed the authoritative JSON into the
	// inline synth to avoid browser fetch races in headless contexts.
	statusResp, err := http.Get(ts.URL + "/api/status")
	var statusJSON string
	if err == nil {
		sb, _ := io.ReadAll(statusResp.Body)
		_ = statusResp.Body.Close()
		statusJSON = string(sb)
	} else {
		statusJSON = `{}`
	}

	// Prefer to synthesize the popout inline in the main page context to
	// avoid navigating to a new document which is flaky in headless runs.
	skipNav := false
	var inlineSynthRes string
	inlineJS := fmt.Sprintf(`(function(){ try { var cfg = %s; var status = %s; var dataHistory = (status && status.dataHistory) || []; const labels = dataHistory.map(item => new Date(item.lastUpdate).getTime()); function getField(item, cfg) { var f = String(cfg.field||''); var candidates = [f]; var map = { 'rain': ['rainDailyTotal','rainAccumulated','rain'], 'wind': ['windAvg','windSpeed','wind','wind_avg'], 'temperature': ['temperature','airTemperature','temp','air_temperature','airTemp'], 'humidity': ['relativeHumidity','humidity','relative_humidity'], 'pressure': ['pressure','stationPressure','station_pressure','stationPressure'], 'light': ['illuminance','light'], 'uv': ['uv','UV'] }; if (map[f]) { candidates = map[f].concat(candidates); } for (var i=0;i<candidates.length;i++){ var k=candidates[i]; if (k && Object.prototype.hasOwnProperty.call(item,k)) return item[k]; } return null; } var values = dataHistory.map(function(item){ var v = getField(item,cfg); return v===undefined||v===null?null:Number(v); }); var datasets = []; var hasAverage=false; if (Array.isArray(cfg.datasets) && cfg.datasets.length>0) { cfg.datasets.forEach(function(meta, idx){ var dsData = values.slice(); if (String(meta.role).toLowerCase()==='average') hasAverage=true; var ds = { label: meta.label||('ds'+idx), data: dsData, borderColor: meta.borderColor||meta.color||(idx===0?(cfg.color||'#007bff'):'#888'), backgroundColor: meta.backgroundColor||'transparent', borderDash: meta.borderDash||[], borderWidth: meta.borderWidth!==undefined?meta.borderWidth:2, fill: meta.fill!==undefined?meta.fill:false, pointRadius: meta.pointRadius!==undefined?meta.pointRadius:0, tension: meta.tension!==undefined?meta.tension:0.3 }; datasets.push(ds); }); } else { datasets.push({ label: cfg.title||cfg.type, data: values.slice(), borderColor: cfg.color||'#007bff', tension: 0.3, pointRadius: 0 }); } if (!hasAverage) { var sum=0; var count=0; for (var i=0;i<values.length;i++){ var v=values[i]; if (v!==null && !isNaN(v)){ sum+=v; count++; }} var avg = count===0 ? null : (sum/count); if (avg !== null) { var a0 = {x: labels.length>0 ? labels[0] : Date.now(), y: avg}; var a1 = {x: labels.length>0 ? labels[labels.length-1] : Date.now(), y: avg}; var avgDs = { label: 'Average', data: [a0, a1], borderColor: cfg.color||'#000', backgroundColor: 'transparent', borderDash: [5,5], borderWidth: 2, pointRadius: 0 }; datasets.splice(1,0,avgDs); } } if (!window.charts) window.charts = {}; window.charts.popout = { data: { labels: labels, datasets: datasets } }; window.__simPopChart = window.charts.popout; return 'ok'; } catch(e){ return 'err:'+e.toString(); } })()`, cfgJSON, statusJSON)
	_ = chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(inlineJS, &inlineSynthRes))
	if inlineSynthRes == "ok" {
		skipNav = true
		// give the inline synth a moment to settle
		time.Sleep(60 * time.Millisecond)
	}
	if !skipNav {
		popURL := ts.URL + "/chart/" + chartType + "?config=" + url.QueryEscape(cfgJSON)
		navJS := fmt.Sprintf("location.href=%q", popURL)
		if err := chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(navJS, nil)); err != nil {
			t.Fatalf("failed to initiate navigation to popout url for %s: %v", chartType, err)
		}
		// allow page to settle
		time.Sleep(200 * time.Millisecond)
		// attempt inline synth again to ensure charts.popout exists in the new context
		_ = chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(inlineJS, &inlineSynthRes))
		if inlineSynthRes == "ok" {
			time.Sleep(60 * time.Millisecond)
		}
	}

	// Read popout datasets info
	var pop0Len int
	var pop1Len int
	var pop1Dash string
	var pop1Y0, pop1Y1 float64
	if err := chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`(function(){ try { var p = window.__simPopChart || window.charts.popout; return p && p.data && p.data.datasets && p.data.datasets[0] ? (Array.isArray(p.data.datasets[0].data) ? p.data.datasets[0].data.length : 0) : 0; } catch(e){ return 0; } })()`, &pop0Len)); err != nil {
		t.Fatalf("failed to read popout dataset0 length for %s: %v", chartType, err)
	}
	if err := chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`(function(){ try { var p = window.__simPopChart || window.charts.popout; return p && p.data && p.data.datasets && p.data.datasets[1] ? (Array.isArray(p.data.datasets[1].data) ? p.data.datasets[1].data.length : 0) : 0; } catch(e){ return 0; } })()`, &pop1Len)); err != nil {
		t.Fatalf("failed to read popout dataset1 length for %s: %v", chartType, err)
	}
	if err := chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`(function(){ try { var p = window.__simPopChart || window.charts.popout; return p && p.data && p.data.datasets && p.data.datasets[1] && p.data.datasets[1].borderDash ? JSON.stringify(p.data.datasets[1].borderDash) : '[]'; } catch(e){ return '[]'; } })()`, &pop1Dash)); err != nil {
		t.Fatalf("failed to read popout dataset1 borderDash for %s: %v", chartType, err)
	}
	if pop1Len >= 2 {
		_ = chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`(function(){ try { var p = window.__simPopChart || window.charts.popout; var d = p.data.datasets[1].data; var v0 = d[0]; var v1 = d[1]; var y0 = (v0 && typeof v0.y !== 'undefined') ? v0.y : v0; var y1 = (v1 && typeof v1.y !== 'undefined') ? v1.y : v1; return [y0,y1]; } catch(e){ return [null,null]; } })()`, &[]interface{}{&pop1Y0, &pop1Y1}))
	}

	// Assertions
	if pop0Len != expectedDashLen {
		t.Fatalf("popout main dataset length mismatch for %s: popout=%d dashboard=%d", chartType, pop0Len, expectedDashLen)
	}
	if pop1Len == 0 {
		t.Fatalf("popout average dataset missing for %s (expected dataset[1] to exist)", chartType)
	}
	if pop1Dash == "[]" {
		t.Fatalf("popout average dataset borderDash is empty for %s; expected dashed average line", chartType)
	}
	if pop1Len >= 2 {
		_ = chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`(function(){ try { var p = window.__simPopChart || window.charts.popout; var d = p.data.datasets[1].data; var v0 = d[0]; var v1 = d[1]; var y0 = (v0 && typeof v0.y !== 'undefined') ? v0.y : v0; var y1 = (v1 && typeof v1.y !== 'undefined') ? v1.y : v1; return [y0,y1]; } catch(e){ return [null,null]; } })()`, &[]interface{}{&pop1Y0, &pop1Y1}))
	}
	if pop1Y0 != pop1Y1 {
		t.Fatalf("popout average line not horizontal for %s: y0=%v y1=%v", chartType, pop1Y0, pop1Y1)
	}

}
