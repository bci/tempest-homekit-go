package weather

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// testRoundTripper forwards any outgoing request to the provided test server URL tsURL, keeping path and query, using Transport (or http.DefaultTransport when nil).
type testRoundTripper struct {
	tsURL     string
	Transport http.RoundTripper
}

// RoundTrip re-issues req against the test server with the same method, body and headers.
func (t *testRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Build new URL pointing to test server
	newURL := t.tsURL + req.URL.Path
	if req.URL.RawQuery != "" {
		newURL = newURL + "?" + req.URL.RawQuery
	}
	newReq, err := http.NewRequest(req.Method, newURL, req.Body)
	if err != nil {
		return nil, err
	}
	newReq.Header = req.Header
	// Use the provided underlying transport to avoid calling this RoundTrip again
	tr := t.Transport
	if tr == nil {
		tr = http.DefaultTransport
	}
	return tr.RoundTrip(newReq)
}

// TestGetHistoricalObservationsAndForecast_WithTestServer checks history (non-empty, newest first) and forecast retrieval against a simulated WeatherFlow API.
func TestGetHistoricalObservationsAndForecast_WithTestServer(t *testing.T) {
	// Prepare handler to simulate WeatherFlow API
	mux := http.NewServeMux()

	// Station details endpoint: /swd/rest/stations/{id}
	mux.HandleFunc("/swd/rest/stations/", func(w http.ResponseWriter, r *http.Request) {
		// Extract station ID from path
		// Return a StationDetailsResponse with one ST device
		resp := StationDetailsResponse{
			Stations: []Station{{StationID: 123, Name: "X", StationName: "X", Devices: []Device{{DeviceID: 555, DeviceType: "ST", SerialNumber: "ST-555"}}}},
		}
		b, _ := json.Marshal(resp)
		w.WriteHeader(200)
		_, _ = w.Write(b)
	})

	// Observations endpoint: /swd/rest/observations/device/{deviceID}
	mux.HandleFunc("/swd/rest/observations/device/", func(w http.ResponseWriter, r *http.Request) {
		// Return a HistoricalResponse with two sample obs arrays
		hr := HistoricalResponse{
			StationID:   123,
			StationName: "X",
			Obs: [][]interface{}{
				{float64(1620000000), 0.0, 1.0, 2.0, 180.0, 0.0, 1012.0, 20.0, 50.0, 100.0, 5.0, 0.0, 0.0, 0.0, 10.0, 0.0, 3.7, 60.0},
				{float64(1620000060), 0.0, 1.1, 2.1, 180.0, 0.0, 1012.1, 20.1, 50.1, 100.0, 5.0, 0.0, 0.0, 0.0, 10.0, 0.0, 3.7, 60.0},
			},
		}
		b, _ := json.Marshal(hr)
		w.WriteHeader(200)
		_, _ = w.Write(b)
	})

	// Forecast endpoint: /swd/rest/better_forecast
	mux.HandleFunc("/swd/rest/better_forecast", func(w http.ResponseWriter, r *http.Request) {
		fr := ForecastResponse{StationID: 123, StationName: "X"}
		fr.Forecast.Daily = []ForecastPeriod{{Time: 1620000000, Icon: "sunny", AirTemperature: 20.0}}
		b, _ := json.Marshal(fr)
		w.WriteHeader(200)
		_, _ = w.Write(b)
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	// Override default transport to forward all requests to our test server
	oldTransport := http.DefaultTransport
	http.DefaultTransport = &testRoundTripper{tsURL: ts.URL, Transport: oldTransport}
	defer func() { http.DefaultTransport = oldTransport }()

	// Run GetHistoricalObservationsWithProgress which internally calls GetStationDetails and observations endpoints
	obs, err := GetHistoricalObservationsWithProgress(123, "token", "debug", nil, 1000)
	if err != nil {
		t.Fatalf("unexpected error from GetHistoricalObservationsWithProgress: %v", err)
	}
	if len(obs) == 0 {
		t.Fatalf("expected historical observations, got 0")
	}

	// Ensure dedup/ordering: timestamps should be in descending order
	for i := 1; i < len(obs); i++ {
		if obs[i-1].Timestamp < obs[i].Timestamp {
			t.Fatalf("expected descending timestamps but got %d before %d", obs[i-1].Timestamp, obs[i].Timestamp)
		}
	}

	// Test GetForecast
	f, err := GetForecast(123, "token")
	if err != nil {
		t.Fatalf("unexpected error from GetForecast: %v", err)
	}
	if f == nil || len(f.Forecast.Daily) == 0 {
		t.Fatalf("expected forecast data, got %+v", f)
	}
}

// TestParseStationStatusHTML_Simple checks that battery and uptime are extracted from a single-line HTML fragment.
func TestParseStationStatusHTML_Simple(t *testing.T) {
	html := `<div><span class="lv-param-label">Battery Voltage</span><span class="lv-value-display"> Good (2.69v)</span>` +
		`<span class="lv-param-label">Uptime</span><span class="lv-value-display">63d 13h 6m 1s</span>` +
		`<span class="lv-param-label">Serial Number</span><span class="lv-value-display">ST-12345</span></div>`

	status, err := parseStationStatusHTML(html, "debug")
	if err != nil {
		t.Fatalf("unexpected error parsing HTML: %v", err)
	}
	if status.BatteryVoltage == "" && status.BatteryStatus == "" {
		t.Fatalf("expected battery info parsed, got %+v", status)
	}
	if status.DeviceUptime == "" {
		t.Fatalf("expected device uptime parsed, got %+v", status)
	}
}
