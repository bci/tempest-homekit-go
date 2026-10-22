package weather

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestGetFloat64AndGetInt checks nil and numeric conversions of the JSON value helpers.
func TestGetFloat64AndGetInt(t *testing.T) {
	if getFloat64(nil) != 0.0 {
		t.Fatalf("expected 0.0 for nil")
	}
	if getInt(nil) != 0 {
		t.Fatalf("expected 0 for nil")
	}

	// float64 value
	var v interface{} = 3.14
	if getFloat64(v) != 3.14 {
		t.Fatalf("expected 3.14")
	}
	// getInt should convert float64 to int
	if getInt(5.0) != 5 {
		t.Fatalf("expected 5")
	}
}

// TestFindStationByNameAndGetTempestDeviceID checks lookup by Name and finding the station Tempest device.
func TestFindStationByNameAndGetTempestDeviceID(t *testing.T) {
	stations := []Station{
		{StationID: 1, Name: "Alpha", StationName: "Alpha"},
		{StationID: 2, Name: "Beta", StationName: "Beta Station", Devices: []Device{{DeviceID: 10, DeviceType: "ST"}}},
	}

	s := FindStationByName(stations, "Beta")
	if s == nil || s.StationID != 2 {
		t.Fatalf("expected to find Beta station")
	}

	id, err := GetTempestDeviceID(s)
	if err != nil || id != 10 {
		t.Fatalf("expected device id 10, got %d (err: %v)", id, err)
	}
}

// TestGetObservationFromURL_HTTPServer checks that GetObservationFromURL decodes an observation from a local server.
func TestGetObservationFromURL_HTTPServer(t *testing.T) {
	// Minimal valid ObservationResponse JSON
	jsonBody := `{"obs":[{"timestamp": 1696761600, "wind_avg": 1.23, "brightness": 100, "uv": 5, "precip": 0.0, "precipitation_type": 0, "battery": 3.7, "report_interval": 60}]}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(jsonBody))
	}))
	defer srv.Close()

	obs, err := GetObservationFromURL(srv.URL)
	if err != nil {
		t.Fatalf("GetObservationFromURL failed: %v", err)
	}
	if obs == nil {
		t.Fatalf("expected non-nil observation")
	}
	if obs.WindAvg != 1.23 {
		t.Fatalf("unexpected WindAvg: %v", obs.WindAvg)
	}
}
