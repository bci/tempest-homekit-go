package weather

import (
	"testing"
	"time"
)

// TestParseDeviceObservations checks that a device row maps air temperature and station pressure.
func TestParseDeviceObservations(t *testing.T) {
	ts := float64(time.Now().Unix())
	arr := [][]interface{}{
		{
			ts,    // timestamp
			0.1,   // wind_lull
			1.2,   // wind_avg
			2.3,   // wind_gust
			180.0, // wind_dir
			0.0,   // placeholder
			1012.3,
			22.5,
			55.0,
			200.0,
			3.0,
			400.0,
			0.0,
			0.0,
			0.0,
			0.0,
			3.8,
			60.0,
		},
	}

	obs := parseDeviceObservations(arr)
	if len(obs) != 1 {
		t.Fatalf("expected 1 observation, got %d", len(obs))
	}
	if obs[0].AirTemperature != 22.5 {
		t.Fatalf("unexpected AirTemperature: %v", obs[0].AirTemperature)
	}
	if obs[0].StationPressure != 1012.3 {
		t.Fatalf("unexpected StationPressure: %v", obs[0].StationPressure)
	}
}
