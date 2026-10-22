package weather

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestAPIDataSource_GetType checks the generated, custom-URL and API type detection.
func TestAPIDataSource_GetType(t *testing.T) {
	a1 := NewAPIDataSource(0, "", "", APIDataSourceOptions{CustomURL: "http://localhost:8080/api/generate-weather", GeneratedPath: "/api/generate-weather"})
	if a1.GetType() != DataSourceGenerated {
		t.Fatalf("expected Generated, got %s", a1.GetType())
	}

	a2 := NewAPIDataSource(0, "", "", APIDataSourceOptions{CustomURL: "http://localhost:8080/custom-endpoint", GeneratedPath: "/api/generate-weather"})
	if a2.GetType() != DataSourceCustomURL {
		t.Fatalf("expected CustomURL, got %s", a2.GetType())
	}

	a3 := NewAPIDataSource(0, "", "", APIDataSourceOptions{CustomURL: "", GeneratedPath: "/api/generate-weather"})
	if a3.GetType() != DataSourceAPI {
		t.Fatalf("expected API, got %s", a3.GetType())
	}
}

// TestAPIDataSource_CustomURLAndStartStop checks that a generated-endpoint source reports its URL and starts and stops cleanly.
func TestAPIDataSource_CustomURLAndStartStop(t *testing.T) {
	jsonBody := `{"obs":[{"timestamp": 1696761600, "wind_avg": 2.5, "brightness": 200, "uv": 3, "precip": 0.0, "precipitation_type": 0, "battery": 3.8, "report_interval": 60}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(jsonBody))
	}))
	defer srv.Close()

	opts := APIDataSourceOptions{CustomURL: srv.URL + "/api/generate-weather", GeneratedPath: "/api/generate-weather"}
	ds := NewAPIDataSource(123, "token", "Test Station", opts)

	if ds.GetType() != DataSourceGenerated {
		t.Fatalf("expected Generated type, got %v", ds.GetType())
	}

	status := ds.GetStatus()
	if status.CustomURL == "" {
		t.Fatalf("expected CustomURL in status when custom URL provided")
	}

	ch, err := ds.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if ch == nil {
		t.Fatalf("expected non-nil channel from Start")
	}

	// Wait briefly for the initial fetch to occur
	time.Sleep(50 * time.Millisecond)

	if err := ds.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

// TestAPIDataSource_GeneratedDetectionAndFetch checks that a started generated-endpoint source fetches an initial observation.
func TestAPIDataSource_GeneratedDetectionAndFetch(t *testing.T) {
	jsonBody := `{"obs":[{"timestamp": 1696761600, "wind_avg": 2.5, "brightness": 200, "uv": 3, "precip": 0.0, "precipitation_type": 0, "battery": 3.8, "report_interval": 60}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(jsonBody))
	}))
	defer srv.Close()

	ds := NewAPIDataSource(2, "token", "s", APIDataSourceOptions{CustomURL: srv.URL + "/api/generate-weather", GeneratedPath: "/api/generate-weather"})
	if ds.GetType() != DataSourceGenerated {
		t.Fatalf("expected Generated, got %s", ds.GetType())
	}

	ch, err := ds.Start()
	if err != nil {
		t.Fatalf("Start error: %v", err)
	}
	if ch == nil {
		t.Fatalf("expected non-nil channel")
	}

	// Wait for initial fetch
	time.Sleep(50 * time.Millisecond)

	if obs := ds.GetLatestObservation(); obs == nil {
		t.Fatalf("expected latest observation to be set")
	}

	if err := ds.Stop(); err != nil {
		t.Fatalf("Stop error: %v", err)
	}
}
