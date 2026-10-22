package weather

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestGetObservationFromURL_Success checks the mapping of a complete observation JSON object.
func TestGetObservationFromURL_Success(t *testing.T) {
	// Build a minimal ObservationResponse JSON matching expected structure
	jsonBody := `{"obs":[{"timestamp":1620000000,"wind_lull":0,"wind_avg":1.2,"wind_gust":2.3,"wind_direction":180,"station_pressure":1012.5,"air_temperature":20.5,"relative_humidity":50,"brightness":1000,"uv":3,"solar_radiation":0,"precip":0,"precip_accum_local_day":0,"precipitation_type":0,"lightning_strike_avg_distance":0,"lightning_strike_count":0,"battery":3.7,"report_interval":60}]}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(jsonBody))
	}))
	defer srv.Close()

	obs, err := GetObservationFromURL(srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if obs == nil {
		t.Fatalf("expected observation")
	}
	if obs.Timestamp != 1620000000 {
		t.Fatalf("unexpected timestamp: %d", obs.Timestamp)
	}
	if obs.AirTemperature != 20.5 {
		t.Fatalf("unexpected air temp: %v", obs.AirTemperature)
	}
	if obs.UV != 3 {
		t.Fatalf("unexpected uv: %d", obs.UV)
	}
}

// TestGetObservationFromURL_Non200_Local checks that GetObservationFromURL fails on an HTTP 500.
func TestGetObservationFromURL_Non200_Local(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("oops"))
	}))
	defer srv.Close()

	_, err := GetObservationFromURL(srv.URL)
	if err == nil {
		t.Fatalf("expected error for non-200 response")
	}
}
