//go:build !no_browser

package testhelpers

import (
	"tempest-homekit-go/pkg/weather"
	"testing"
	"time"
)

// TestSeedOptionsDeterminism checks that the same options and seed give
// identical temperatures across two runs.
func TestSeedOptionsDeterminism(t *testing.T) {
	opts := SeedOptions{Points: 3, TimeSpacing: 1 * time.Minute, TempNoise: 0.1, UVNoise: 1, RandSeed: 42, Location: "coastal"}
	var a, b []weather.Observation
	updaterA := func(o *weather.Observation) { a = append(a, *o) }
	updaterB := func(o *weather.Observation) { b = append(b, *o) }

	base := time.Now()
	chartTypes := []string{"temperature", "humidity"}

	SeedObservationsWithOptions(t, updaterA, chartTypes, base, opts)
	SeedObservationsWithOptions(t, updaterB, chartTypes, base, opts)

	if len(a) != len(b) {
		t.Fatalf("expected same length, got %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].AirTemperature != b[i].AirTemperature {
			t.Fatalf("expected deterministic temps, idx %d: %v vs %v", i, a[i].AirTemperature, b[i].AirTemperature)
		}
	}
}

// TestSeedOptions_RainAndLocationEffects checks that the coastal rain bias
// gives at least as much accumulated rain as desert with the same seed.
func TestSeedOptions_RainAndLocationEffects(t *testing.T) {
	base := time.Now()
	optsCoastal := SeedOptions{Points: 4, TimeSpacing: 1 * time.Minute, RandSeed: 99, Location: "coastal"}
	optsDesert := SeedOptions{Points: 4, TimeSpacing: 1 * time.Minute, RandSeed: 99, Location: "desert"}

	var coastal []weather.Observation
	var desert []weather.Observation
	SeedObservationsWithOptions(t, func(o *weather.Observation) { coastal = append(coastal, *o) }, []string{"rain"}, base, optsCoastal)
	SeedObservationsWithOptions(t, func(o *weather.Observation) { desert = append(desert, *o) }, []string{"rain"}, base, optsDesert)

	if len(coastal) != len(desert) {
		t.Fatalf("expected same number of points, got %d vs %d", len(coastal), len(desert))
	}

	// coastal increments should be >= desert increments because coastal adds +1.0
	if coastal[len(coastal)-1].RainAccumulated < desert[len(desert)-1].RainAccumulated {
		t.Fatalf("expected coastal accumulated rain >= desert, got %f vs %f", coastal[len(coastal)-1].RainAccumulated, desert[len(desert)-1].RainAccumulated)
	}
}
