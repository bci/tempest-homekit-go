package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"tempest-homekit-go/pkg/weather"
)

// createTestServer creates a WebServer with minimal configuration for testing
// Prefer the centralized test factory which accepts *testing.T so failures are reported correctly.
func createTestServer(t *testing.T) *WebServer {
	return testNewWebServer(t)
}

// TestWeatherAndStatusEndpoints feeds observations out of order and checks
// that /api/weather returns data and /api/status returns history in
// chronological order with non-negative incremental rain.
func TestWeatherAndStatusEndpoints(t *testing.T) {
	ws := createTestServer(t)

	// Inject synthetic weather observations in non-sorted order to ensure server sorts them
	now := time.Now()
	obs1 := weather.Observation{Timestamp: now.Add(-2 * time.Minute).Unix(), AirTemperature: 20.1, RelativeHumidity: 50, WindAvg: 3.2, WindGust: 4.5, StationPressure: 1012.3, Illuminance: 1200, UV: 2, RainAccumulated: 0.1, LightningStrikeAvg: 0, LightningStrikeCount: 0, Battery: 3.7}
	obs2 := weather.Observation{Timestamp: now.Add(-1 * time.Minute).Unix(), AirTemperature: 20.3, RelativeHumidity: 51, WindAvg: 3.4, WindGust: 4.7, StationPressure: 1012.5, Illuminance: 1300, UV: 3, RainAccumulated: 0.12, LightningStrikeAvg: 0, LightningStrikeCount: 0, Battery: 3.7}
	obs3 := weather.Observation{Timestamp: now.Unix(), AirTemperature: 20.5, RelativeHumidity: 52, WindAvg: 3.6, WindGust: 5.0, StationPressure: 1012.7, Illuminance: 1400, UV: 4, RainAccumulated: 0.15, LightningStrikeAvg: 0, LightningStrikeCount: 0, Battery: 3.7}

	// Update server with observations (intentionally out of order)
	ws.UpdateWeather(&obs2)
	ws.UpdateWeather(&obs1)
	ws.UpdateWeather(&obs3)

	// Start an httptest server using the WebServer handlers
	mux := http.NewServeMux()
	mux.HandleFunc("/api/weather", ws.handleWeatherAPI)
	mux.HandleFunc("/api/status", ws.handleStatusAPI)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	// Call /api/weather
	resp, err := http.Get(ts.URL + "/api/weather")
	if err != nil {
		t.Fatalf("failed to GET /api/weather: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status for /api/weather: %d", resp.StatusCode)
	}

	var weatherResp WeatherResponse
	if err := json.NewDecoder(resp.Body).Decode(&weatherResp); err != nil {
		t.Fatalf("failed to decode /api/weather response: %v", err)
	}

	// Basic numeric field checks
	if weatherResp.Temperature == 0 {
		t.Fatalf("expected non-zero temperature in /api/weather response")
	}
	if weatherResp.Illuminance == 0 {
		t.Fatalf("expected non-zero illuminance in /api/weather response")
	}

	// Call /api/status
	resp2, err := http.Get(ts.URL + "/api/status")
	if err != nil {
		t.Fatalf("failed to GET /api/status: %v", err)
	}
	defer func() { _ = resp2.Body.Close() }()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status for /api/status: %d", resp2.StatusCode)
	}

	var statusResp StatusResponse
	if err := json.NewDecoder(resp2.Body).Decode(&statusResp); err != nil {
		t.Fatalf("failed to decode /api/status response: %v", err)
	}

	// Ensure dataHistory is chronological (oldest -> newest)
	if len(statusResp.DataHistory) < 3 {
		t.Fatalf("expected at least 3 history points, got %d", len(statusResp.DataHistory))
	}

	// Verify the timestamps in DataHistory are non-decreasing
	var prev time.Time
	for i, h := range statusResp.DataHistory {
		tsParsed, err := time.Parse(time.RFC3339, h.LastUpdate)
		if err != nil {
			t.Fatalf("failed to parse LastUpdate for history index %d: %v", i, err)
		}
		if i > 0 && tsParsed.Before(prev) {
			t.Fatalf("history not sorted chronologically: index %d is before previous", i)
		}
		prev = tsParsed
	}

	// Verify incremental rain values are not negative
	for i, h := range statusResp.DataHistory {
		if h.RainAccum < 0 {
			t.Fatalf("negative incremental rain at history index %d: %f", i, h.RainAccum)
		}
	}
}
