package weather

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// routeTransportToServer is a helper to route the default transport to the given test server; it returns a function that restores the original transport.
func routeTransportToServer(srv *httptest.Server) func() {
	old := http.DefaultTransport
	target, _ := url.Parse(srv.URL)
	http.DefaultTransport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		return old.RoundTrip(req)
	})
	return func() { http.DefaultTransport = old }
}

// TestAPIDataSource_FetchObservation_CustomURL checks that fetchObservation from a custom URL stores the observation and counts it.
func TestAPIDataSource_FetchObservation_CustomURL(t *testing.T) {
	obsJSON := `{"obs":[{"timestamp": 1696761600, "wind_avg": 2.5, "brightness": 100, "uv": 3, "precip": 0.0, "precipitation_type": 0, "battery": 3.8, "report_interval": 60}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(obsJSON))
	}))
	defer srv.Close()

	a := NewAPIDataSource(0, "token", "s", APIDataSourceOptions{CustomURL: srv.URL, GeneratedPath: "/api/generate-weather"})

	// call unexported fetchObservation directly
	a.fetchObservation()

	if a.GetLatestObservation() == nil {
		t.Fatalf("expected latest observation to be set after fetchObservation")
	}
	if a.GetStatus().ObservationCount == 0 {
		t.Fatalf("expected observation count to increase after fetchObservation")
	}
}

// TestAPIDataSource_FetchForecast_WithTransportOverride checks that fetchForecast stores the forecast returned by the API.
func TestAPIDataSource_FetchForecast_WithTransportOverride(t *testing.T) {
	// Create a forecast response
	fr := ForecastResponse{
		Forecast: struct {
			Daily []ForecastPeriod `json:"daily"`
		}{Daily: []ForecastPeriod{{Time: 1, AirTemperature: 21.0}}},
		CurrentConditions: ForecastPeriod{Time: 1, AirTemperature: 21.0},
	}
	b, _ := json.Marshal(fr)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	}))
	defer srv.Close()

	restore := routeTransportToServer(srv)
	defer restore()

	a := NewAPIDataSource(1, "token", "s", APIDataSourceOptions{CustomURL: "", GeneratedPath: "/api/generate-weather"})

	// call fetchForecast which uses GetForecast (which will use overridden transport)
	a.fetchForecast()

	if a.GetForecast() == nil {
		t.Fatalf("expected forecast to be set after fetchForecast")
	}
}

// TestFilterToOneMinuteIncrements_EdgeCases checks empty and single-element inputs.
func TestFilterToOneMinuteIncrements_EdgeCases(t *testing.T) {
	// Empty input
	empty := []*Observation{}
	out := filterToOneMinuteIncrements(empty, 10)
	if len(out) != 0 {
		t.Fatalf("expected empty output for empty input")
	}

	// Single item should return that item
	single := []*Observation{{Timestamp: 1000}}
	out2 := filterToOneMinuteIncrements(single, 10)
	if len(out2) != 1 || out2[0].Timestamp != 1000 {
		t.Fatalf("expected single-item preserved")
	}
}

// TestParseStationStatusHTML_AltBatteryPattern checks that the fallback battery pattern extracts status and voltage.
func TestParseStationStatusHTML_AltBatteryPattern(t *testing.T) {
	html := `<div id="diagnostic-info">Battery Voltage<span class="lv-value-display">Good (2.69v)</span></div>`
	status, err := parseStationStatusHTML(html, "debug")
	if err != nil {
		t.Fatalf("parseStationStatusHTML returned error: %v", err)
	}
	if status.BatteryStatus != "Good" {
		t.Fatalf("expected BatteryStatus Good, got %s", status.BatteryStatus)
	}
	if status.BatteryVoltage != "2.69V" {
		t.Fatalf("expected BatteryVoltage 2.69V, got %s", status.BatteryVoltage)
	}
}
