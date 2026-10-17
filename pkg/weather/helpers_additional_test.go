package weather

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestParseDeviceObservations_SkipsShortArrays checks that observation arrays shorter than 18 values are skipped.
func TestParseDeviceObservations_SkipsShortArrays(t *testing.T) {
	// Observation arrays shorter than 18 should be skipped
	data := [][]interface{}{
		{1, 2, 3},
	}
	obs := parseDeviceObservations(data)
	if len(obs) != 0 {
		t.Fatalf("expected 0 observations for short arrays, got %d", len(obs))
	}
}

// TestParseDeviceObservations_ParsesValidArray checks that a full 18-value row yields one observation.
func TestParseDeviceObservations_ParsesValidArray(t *testing.T) {
	data := [][]interface{}{
		{float64(time.Now().Unix()), 0.0, 1.0, 2.0, 180.0, 0.0, 1012.0, 20.0, 50.0, 100.0, 5.0, 0.0, 0.0, 0.0, 10.0, 0.0, 3.7, 60.0},
	}
	obs := parseDeviceObservations(data)
	if len(obs) != 1 {
		t.Fatalf("expected 1 parsed observation, got %d", len(obs))
	}
}

// TestFindStationByNameAndDeviceID checks lookup by StationName and finding the Tempest device ID.
func TestFindStationByNameAndDeviceID(t *testing.T) {
	stations := []Station{
		{StationID: 1, Name: "A", StationName: "Alpha", Devices: []Device{{DeviceID: 100, DeviceType: "ST"}}},
		{StationID: 2, Name: "B", StationName: "Beta", Devices: []Device{{DeviceID: 200, DeviceType: "SKY"}}},
	}

	s := FindStationByName(stations, "Alpha")
	if s == nil || s.StationID != 1 {
		t.Fatalf("expected to find station Alpha with ID 1")
	}

	id, err := GetTempestDeviceID(&stations[0])
	if err != nil || id != 100 {
		t.Fatalf("expected Tempest device id 100, got %d, err=%v", id, err)
	}
}

// TestFilterToOneMinuteIncrements_Basic checks that 30-second data filtered to 2 points returns them oldest first.
func TestFilterToOneMinuteIncrements_Basic(t *testing.T) {
	// Create a series of observations 30 seconds apart; filtering to 2 points should pick every ~60s
	base := int64(1000000)
	var inputs []*Observation
	for i := 0; i < 6; i++ {
		inputs = append(inputs, &Observation{Timestamp: base + int64(i*30)})
	}

	filtered := filterToOneMinuteIncrements(inputs, 2)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 filtered observations, got %d", len(filtered))
	}
	if filtered[0].Timestamp >= filtered[1].Timestamp {
		t.Fatalf("expected filtered timestamps in ascending order")
	}
}

// TestFilterToOneMinuteIncrements_Smoke checks that irregularly spaced data yields a non-empty result.
func TestFilterToOneMinuteIncrements_Smoke(t *testing.T) {
	now := time.Now().Unix()
	observations := []*Observation{
		{Timestamp: now},
		{Timestamp: now - 30},
		{Timestamp: now - 90},
		{Timestamp: now - 160},
	}
	filtered := filterToOneMinuteIncrements(observations, 10)
	if len(filtered) == 0 {
		t.Fatalf("expected some filtered observations")
	}
}

// TestGetObservationFromURL_HttpServer checks that a minimal observation JSON object is decoded.
func TestGetObservationFromURL_HttpServer(t *testing.T) {
	// Setup a test server that returns minimal valid observation JSON
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"obs":[{"timestamp":1620000000,"air_temperature":20.5}]}`))
	}))
	defer ts.Close()

	obs, err := GetObservationFromURL(ts.URL)
	if err != nil {
		t.Fatalf("unexpected error from GetObservationFromURL: %v", err)
	}
	if obs.Timestamp != 1620000000 {
		t.Fatalf("unexpected timestamp parsed: %d", obs.Timestamp)
	}
}
