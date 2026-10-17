package weather

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestParseDeviceObservations_FieldMapping ensures parseDeviceObservations correctly maps multiple fields from the array.
func TestParseDeviceObservations_FieldMapping(t *testing.T) {
	data := [][]interface{}{
		{float64(1620001000), 0.1, 1.2, 3.4, 90.0, 0.0, 1015.5, 19.5, 55.0, 100.0, 2.0, 0.0, 0.5, 1.0, 7.5, 1.0, 3.9, 60.0},
	}

	obs := parseDeviceObservations(data)
	if len(obs) != 1 {
		t.Fatalf("expected 1 parsed observation, got %d", len(obs))
	}

	o := obs[0]
	if o.Timestamp != 1620001000 {
		t.Fatalf("timestamp mismatch: %d", o.Timestamp)
	}
	if o.StationPressure == 0 || o.AirTemperature == 0 {
		t.Fatalf("expected non-zero pressure and temperature, got %v and %v", o.StationPressure, o.AirTemperature)
	}
	if o.RainAccumulated != 0.5 {
		t.Fatalf("unexpected rain accumulated: %v", o.RainAccumulated)
	}
	if o.PrecipitationType != 1 {
		t.Fatalf("unexpected precip type: %d", o.PrecipitationType)
	}
}

// TestFilterToOneMinuteIncrements_MaxCountAndOrder verifies filterToOneMinuteIncrements respects maxCount and returns chronological order.
func TestFilterToOneMinuteIncrements_MaxCountAndOrder(t *testing.T) {
	var all []*Observation
	base := int64(1620000000)
	// create observations every 60 seconds for 10 points
	for i := 0; i < 10; i++ {
		all = append(all, &Observation{Timestamp: base + int64(i*60)})
	}

	filtered := filterToOneMinuteIncrements(all, 5)
	if len(filtered) != 5 {
		t.Fatalf("expected 5 filtered observations, got %d", len(filtered))
	}

	// chronological order (oldest first)
	for i := 1; i < len(filtered); i++ {
		if filtered[i].Timestamp <= filtered[i-1].Timestamp {
			t.Fatalf("expected increasing timestamps, got %d then %d", filtered[i-1].Timestamp, filtered[i].Timestamp)
		}
	}
}

// TestGetObservationFromURL_Errors tests error conditions for GetObservationFromURL: non-200, invalid JSON, and empty obs.
func TestGetObservationFromURL_Errors(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("/err", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte("internal error"))
	})

	mux.HandleFunc("/badjson", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("not json"))
	})

	mux.HandleFunc("/empty", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"obs": []}`))
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	if _, err := GetObservationFromURL(ts.URL + "/err"); err == nil {
		t.Fatalf("expected error for 500 response")
	}
	if _, err := GetObservationFromURL(ts.URL + "/badjson"); err == nil {
		t.Fatalf("expected JSON parse error")
	}
	if _, err := GetObservationFromURL(ts.URL + "/empty"); err == nil {
		t.Fatalf("expected error for empty obs array")
	}
}
