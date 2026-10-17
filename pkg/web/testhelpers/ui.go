//go:build !no_browser

// Package web (in directory pkg/web/testhelpers) holds a copy of the headless-browser
// popout assertion helper also defined in pkg/web/testhelpers_ui.go. It declares package
// name web rather than testhelpers, and nothing in the module imports it; it differs from
// the pkg/web copy only in using golang.org/x/text/cases for the chart title.
package web

import (
	"context"
	"fmt"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// AssertPopoutDatasetOrdering builds a popout for the given chart type and
// asserts the synthesized popout has the main data at datasets[0] (same length
// as dashboard main dataset) and the average dashed two-point horizontal line
// at datasets[1]. This is exported so other UI tests can reuse it.
//
// browserCtx must be a chromedp context whose page is the dashboard served by ts, with
// charts already rendered: the popout config is built in-page from charts.<chartType>,
// then the browser navigates to ts.URL + "/chart/<chartType>?config=...". expectedDashLen
// is the dashboard's main-dataset length that the popout's datasets[0] must match.
// Failures are reported with t.Fatalf. It polls up to about 4 seconds for the popout
// chart to appear.
func AssertPopoutDatasetOrdering(t *testing.T, browserCtx context.Context, ts *httptest.Server, chartType string, expectedDashLen int) {
	// Build cfg JSON in-page from charts.<chartType> metadata
	title := cases.Title(language.Und).String(chartType)
	buildCfg := fmt.Sprintf(`(function(){ try { var chartObj = charts && charts.%s; var datasetsMeta = []; if (chartObj && chartObj.data && Array.isArray(chartObj.data.datasets)) { chartObj.data.datasets.forEach(function(ds){ var meta = {}; if (ds.label) meta.label = ds.label; if (ds.borderColor) meta.borderColor = ds.borderColor; if (ds.backgroundColor) meta.backgroundColor = ds.backgroundColor; if (ds.borderDash) meta.borderDash = ds.borderDash; if (ds.borderWidth !== undefined) meta.borderWidth = ds.borderWidth; if (ds.fill !== undefined) meta.fill = ds.fill; if (ds.pointRadius !== undefined) meta.pointRadius = ds.pointRadius; if (ds.tension !== undefined) meta.tension = ds.tension; if (String(ds.label).toLowerCase().indexOf('average')>=0) meta.role='average'; if (String(ds.label).toLowerCase().indexOf('trend')>=0) meta.role='trend'; if (String(ds.label).toLowerCase().indexOf('today')>=0 || String(ds.label).toLowerCase().indexOf('total')>=0) meta.role='total'; datasetsMeta.push(meta); }); } var cfg = { type: '%[1]s', field: '%[1]s', title: '%[2]s', color: (chartObj && chartObj.data && chartObj.data.datasets && chartObj.data.datasets[0] && chartObj.data.datasets[0].borderColor) || '#007bff', units: window.units || {}, datasets: datasetsMeta }; return JSON.stringify(cfg); } catch(e) { return ''; } })()`, chartType, title)

	var cfgJSON string
	if err := chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(buildCfg, &cfgJSON)); err != nil {
		t.Fatalf("failed to build cfg JSON for %s: %v", chartType, err)
	}
	if cfgJSON == "" {
		t.Fatalf("built cfg JSON was empty for %s", chartType)
	}

	// Navigate to popout URL
	popURL := ts.URL + "/chart/" + chartType + "?config=" + url.QueryEscape(cfgJSON)
	if err := chromedp.Run(browserCtx, chromedp.Navigate(popURL)); err != nil {
		t.Fatalf("failed to navigate to popout url for %s: %v", chartType, err)
	}

	// call initCharts on popout page to create popout chart
	if err := chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`typeof initCharts === 'function' && initCharts()`, nil)); err != nil {
		t.Fatalf("failed to initCharts on popout for %s: %v", chartType, err)
	}

	// wait for charts.popout to exist and have datasets
	var popExists bool
	for i := 0; i < 40; i++ {
		_ = chromedp.Run(browserCtx, chromedp.Evaluate(`Boolean(window.charts && window.charts.popout && window.charts.popout.data && Array.isArray(window.charts.popout.data.datasets) && window.charts.popout.data.datasets.length>0)`, &popExists))
		if popExists {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !popExists {
		t.Fatalf("popout chart not initialized or missing datasets for %s", chartType)
	}

	// read popout datasets info
	var pop0Len int
	var pop1Len int
	var pop1Dash string
	var pop1Y0, pop1Y1 float64
	if err := chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`(function(){ try { var p = window.charts.popout; return p.data.datasets[0].data.length; } catch(e){ return 0; } })()`, &pop0Len)); err != nil {
		t.Fatalf("failed to read popout dataset0 length for %s: %v", chartType, err)
	}
	if err := chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`(function(){ try { var p = window.charts.popout; return p.data.datasets[1] ? p.data.datasets[1].data.length : 0; } catch(e){ return 0; } })()`, &pop1Len)); err != nil {
		t.Fatalf("failed to read popout dataset1 length for %s: %v", chartType, err)
	}
	if err := chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`(function(){ try { var p = window.charts.popout; return p.data.datasets[1] && p.data.datasets[1].borderDash ? JSON.stringify(p.data.datasets[1].borderDash) : '[]'; } catch(e){ return '[]'; } })()`, &pop1Dash)); err != nil {
		t.Fatalf("failed to read popout dataset1 borderDash for %s: %v", chartType, err)
	}
	// read first and second y of average line if present
	if pop1Len >= 2 {
		_ = chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`(function(){ try { var d = window.charts.popout.data.datasets[1].data; return d[0].y; } catch(e){ return null } })()`, &pop1Y0))
		_ = chromedp.Run(browserCtx, chromedp.EvaluateAsDevTools(`(function(){ try { var d = window.charts.popout.data.datasets[1].data; return d[1].y; } catch(e){ return null } })()`, &pop1Y1))
	}

	// Assertions
	if pop0Len != expectedDashLen {
		t.Fatalf("popout main dataset length mismatch for %s: popout=%d dashboard=%d", chartType, pop0Len, expectedDashLen)
	}
	// dataset[1] should be dashed (borderDash non-empty array)
	if pop1Len == 0 {
		t.Fatalf("popout average dataset missing for %s (expected dataset[1] to exist)", chartType)
	}
	if pop1Dash == "[]" {
		t.Fatalf("popout average dataset borderDash is empty for %s; expected dashed average line", chartType)
	}
	// average should be two-point horizontal line with equal y values
	if pop1Len != 2 {
		t.Fatalf("popout average dataset expected 2 points but has %d for %s", pop1Len, chartType)
	}
	if pop1Y0 != pop1Y1 {
		t.Fatalf("popout average line not horizontal for %s: y0=%v y1=%v", chartType, pop1Y0, pop1Y1)
	}
}
