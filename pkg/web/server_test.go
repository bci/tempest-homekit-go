package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"tempest-homekit-go/pkg/weather"
)

// TestNewWebServer checks that NewWebServer stores the port, elevation and log
// level it was given.
func TestNewWebServer(t *testing.T) {
	server := testNewWebServer(t)

	if server.port != "8080" {
		t.Errorf("Expected port 8080, got %s", server.port)
	}
	if server.elevation != 100.0 {
		t.Errorf("Expected elevation 100.0, got %f", server.elevation)
	}
	if server.logLevel != "info" {
		t.Errorf("Expected log level info, got %s", server.logLevel)
	}
}

// Pressure helper tests are consolidated in helpers_test.go to avoid duplication.

// TestWebServerWeatherEndpoint checks that /api/weather returns 200 with a
// JSON content type once an observation exists.
func TestWebServerWeatherEndpoint(t *testing.T) {
	server := testNewWebServer(t)

	// Add some test data
	testObs := &weather.Observation{
		Timestamp:        time.Now().Unix(),
		AirTemperature:   25.0,
		RelativeHumidity: 60.0,
		WindAvg:          5.5,
		WindDirection:    180.0,
		StationPressure:  1013.25,
		UV:               5.0,
		Illuminance:      50000.0,
		RainAccumulated:  0.0,
	}
	server.UpdateWeather(testObs)

	req, err := http.NewRequest("GET", "/api/weather", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleWeatherAPI)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Weather handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	// Check that the response contains JSON (basic check)
	contentType := rr.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", contentType)
	}
}

// TestWebServerStatusEndpoint checks that /api/status returns 200 with a JSON
// content type even before any observation.
func TestWebServerStatusEndpoint(t *testing.T) {
	server := testNewWebServer(t)

	req, err := http.NewRequest("GET", "/api/status", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(server.handleStatusAPI)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Status handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	// Check that the response contains JSON
	contentType := rr.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", contentType)
	}
}

// TestUpdateForecast checks that UpdateWeather stores the current observation;
// despite its name it does not exercise UpdateForecast.
func TestUpdateForecast(t *testing.T) {
	server := testNewWebServer(t)

	testObs := &weather.Observation{
		Timestamp:        time.Now().Unix(),
		AirTemperature:   25.0,
		RelativeHumidity: 60.0,
	}

	server.UpdateWeather(testObs)

	if server.weatherData == nil {
		t.Error("Weather data should be updated")
	}
	if server.weatherData.AirTemperature != 25.0 {
		t.Errorf("Expected temperature 25.0, got %f", server.weatherData.AirTemperature)
	}
}

// TestSetHistoryLoadingProgress checks that SetHistoryLoadingProgress marks
// loading in progress and records the step counts.
func TestSetHistoryLoadingProgress(t *testing.T) {
	server := testNewWebServer(t)

	server.SetHistoryLoadingProgress(1, 3, "Loading data...")

	if !server.historyLoadingProgress.isLoading {
		t.Error("History loading should be marked as in progress")
	}
	if server.historyLoadingProgress.currentStep != 1 {
		t.Errorf("Expected current step 1, got %d", server.historyLoadingProgress.currentStep)
	}
	if server.historyLoadingProgress.totalSteps != 3 {
		t.Errorf("Expected total steps 3, got %d", server.historyLoadingProgress.totalSteps)
	}
}

// TestSetHistoryLoadingComplete checks that SetHistoryLoadingComplete clears
// the in-progress flag.
func TestSetHistoryLoadingComplete(t *testing.T) {
	server := testNewWebServer(t)

	// First set it to loading
	server.SetHistoryLoadingProgress(1, 3, "Loading...")

	// Then mark as complete
	server.SetHistoryLoadingComplete()

	if server.historyLoadingProgress.isLoading {
		t.Error("History loading should be marked as complete")
	}
}
