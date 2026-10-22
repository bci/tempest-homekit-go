package weather

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestHelpers_GetFloat64AndGetInt_Added tests basic helper conversions for nil and numeric values.
func TestHelpers_GetFloat64AndGetInt_Added(t *testing.T) {
	if got := getFloat64(nil); got != 0.0 {
		t.Fatalf("expected 0.0 for nil, got %v", got)
	}
	if got := getInt(nil); got != 0 {
		t.Fatalf("expected 0 for nil, got %v", got)
	}
	var f interface{} = 12.34
	if got := getFloat64(f); got != 12.34 {
		t.Fatalf("expected 12.34, got %v", got)
	}
	if got := getInt(f); got != 12 {
		t.Fatalf("expected 12, got %v", got)
	}
}

// TestParseDeviceObservations_Simple_Added parses a device observations array and ensures fields map correctly.
func TestParseDeviceObservations_Simple_Added(t *testing.T) {
	ts := float64(time.Now().Unix())
	arr := make([]interface{}, 18)
	arr[0] = ts
	arr[1] = 0.1
	arr[2] = 1.2
	arr[3] = 2.3
	arr[4] = 90.0
	arr[6] = 1013.2
	arr[7] = 19.5
	arr[8] = 55.0
	arr[10] = 3.0
	arr[11] = 200.0
	arr[16] = 3.7
	arr[17] = 60

	obs := parseDeviceObservations([][]interface{}{arr})
	if len(obs) != 1 {
		t.Fatalf("expected 1 observation, got %d", len(obs))
	}
	if obs[0].AirTemperature != 19.5 {
		t.Fatalf("expected air temp 19.5, got %v", obs[0].AirTemperature)
	}
}

// TestFindStationAndDeviceID_Added gives FindStationByName and GetTempestDeviceID helper coverage.
func TestFindStationAndDeviceID_Added(t *testing.T) {
	stations := []Station{
		{StationID: 1, Name: "Alpha", StationName: "AlphaStation", Devices: []Device{{DeviceID: 10, DeviceType: "HB"}}},
		{StationID: 2, Name: "Beta", StationName: "BetaStation", Devices: []Device{{DeviceID: 20, DeviceType: "ST"}}},
	}

	s := FindStationByName(stations, "Beta")
	if s == nil {
		t.Fatalf("expected to find station Beta")
	}
	id, err := GetTempestDeviceID(s)
	if err != nil {
		t.Fatalf("unexpected error getting device id: %v", err)
	}
	if id != 20 {
		t.Fatalf("expected device id 20, got %d", id)
	}
}

// TestGetObservationFromURL_Success_Added checks the GetObservationFromURL success case.
func TestGetObservationFromURL_Success_Added(t *testing.T) {
	now := time.Now().Unix()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		response := map[string]interface{}{
			"obs": []map[string]interface{}{{"timestamp": float64(now), "air_temperature": 22.5}},
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	obs, err := GetObservationFromURL(server.URL)
	if err != nil {
		t.Fatalf("GetObservationFromURL failed: %v", err)
	}
	if obs.AirTemperature != 22.5 {
		t.Fatalf("unexpected temp: %v", obs.AirTemperature)
	}
}

// TestGetObservationFromURL_NoObs_Added checks GetObservationFromURL error handling when there are no observations.
func TestGetObservationFromURL_NoObs_Added(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		response := map[string]interface{}{"obs": []interface{}{}}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	_, err := GetObservationFromURL(server.URL)
	if err == nil {
		t.Fatalf("expected error when no observations are returned")
	}
}
