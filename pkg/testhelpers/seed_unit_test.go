//go:build !no_browser

package testhelpers

import (
	"testing"
	"time"

	"tempest-homekit-go/pkg/weather"
)

// TestSeedObservations_Deterministic checks that two runs with the same seed
// produce identical timestamps, temperatures and humidities.
//
// Test that SeedObservationsWithOptions produces deterministic output for the same seed
func TestSeedObservations_Deterministic(t *testing.T) {
	base := time.Now().Truncate(time.Second)
	chartTypes := []string{"temperature", "humidity"}
	opts := SeedOptions{
		Points:      4,
		Season:      "summer",
		Location:    "coastal",
		TimeSpacing: 5 * time.Minute,
		TempNoise:   0.1,
		UVNoise:     1,
		RandSeed:    12345,
	}

	var first []weather.Observation
	SeedObservationsWithOptions(t, func(o *weather.Observation) { first = append(first, *o) }, chartTypes, base, opts)

	var second []weather.Observation
	SeedObservationsWithOptions(t, func(o *weather.Observation) { second = append(second, *o) }, chartTypes, base, opts)

	if len(first) != len(second) {
		t.Fatalf("expected same number of observations, got %d and %d", len(first), len(second))
	}

	for i := range first {
		if first[i].Timestamp != second[i].Timestamp {
			t.Fatalf("timestamps differ at index %d: %d vs %d", i, first[i].Timestamp, second[i].Timestamp)
		}
		// compare a couple of fields for determinism
		if first[i].AirTemperature != second[i].AirTemperature {
			t.Fatalf("air temperature differs at index %d: %v vs %v", i, first[i].AirTemperature, second[i].AirTemperature)
		}
		if first[i].RelativeHumidity != second[i].RelativeHumidity {
			t.Fatalf("relative humidity differs at index %d: %v vs %v", i, first[i].RelativeHumidity, second[i].RelativeHumidity)
		}
	}
}

// TestClamp_EdgeCases checks clamp below, above and within the range.
func TestClamp_EdgeCases(t *testing.T) {
	if clamp(5, 10, 20) != 10 {
		t.Fatalf("clamp below low failed")
	}
	if clamp(25, 10, 20) != 20 {
		t.Fatalf("clamp above high failed")
	}
	if clamp(15, 10, 20) != 15 {
		t.Fatalf("clamp within range failed")
	}
}
