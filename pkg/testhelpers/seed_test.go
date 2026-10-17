//go:build !no_browser

package testhelpers

import (
	"tempest-homekit-go/pkg/weather"
	"testing"
	"time"
)

// collectObservations runs SeedObservationsWithOptions and returns copies of
// every observation passed to the updater, in order.
func collectObservations(t *testing.T, chartTypes []string, base time.Time, opts SeedOptions) []weather.Observation {
	t.Helper()
	var out []weather.Observation
	updater := func(o *weather.Observation) {
		// copy the value since SeedObservations reuses the pointer
		kop := *o
		out = append(out, kop)
	}
	SeedObservationsWithOptions(t, updater, chartTypes, base, opts)
	return out
}

// TestSeedObservations_DefaultsAndSpacing checks the default of 5 points and
// the default 5-minute timestamp spacing.
func TestSeedObservations_DefaultsAndSpacing(t *testing.T) {
	base := time.Now()
	opts := SeedOptions{Points: 0, TimeSpacing: 0, RandSeed: 42, TempNoise: 0.0}
	obs := collectObservations(t, []string{"temperature"}, base, opts)
	// default points is 5
	if len(obs) != 5 {
		t.Fatalf("expected 5 points by default, got %d", len(obs))
	}
	// check spacing: default TimeSpacing is 5m so second point should be base - 5m
	if obs[1].Timestamp != base.Add(-5*time.Minute).Unix() {
		t.Fatalf("unexpected timestamp spacing: got %v expected %v", time.Unix(obs[1].Timestamp, 0), base.Add(-5*time.Minute))
	}
}

// Determinism is covered in seed_unit_test.go; avoid duplicate test names.

// TestSeedObservations_TemperatureSeasonLocation checks that winter plus
// desert with zero noise gives exactly 14 C.
func TestSeedObservations_TemperatureSeasonLocation(t *testing.T) {
	base := time.Unix(1600000000, 0)
	// winter + desert => base temp should be 8 + 6 = 14 when TempNoise=0
	opts := SeedOptions{Points: 3, TimeSpacing: 1 * time.Minute, RandSeed: 1, TempNoise: 0.0, Season: "winter", Location: "desert"}
	obs := collectObservations(t, []string{"temperature"}, base, opts)
	if len(obs) == 0 {
		t.Fatal("no observations generated")
	}
	if obs[0].AirTemperature != 14.0 {
		t.Fatalf("expected temp 14.0 for winter+desert with zero noise, got %v", obs[0].AirTemperature)
	}
}

// TestSeedObservations_RainMonotonicAndCoastalBias checks that accumulated
// rain never decreases across generated points and that coastal accumulates at
// least as much as inland.
func TestSeedObservations_RainMonotonicAndCoastalBias(t *testing.T) {
	base := time.Unix(1600000000, 0)
	opts := SeedOptions{Points: 8, TimeSpacing: 1 * time.Minute, RandSeed: 99}
	obs := collectObservations(t, []string{"rain"}, base, opts)
	if len(obs) < 2 {
		t.Fatalf("expected multiple rain points, got %d", len(obs))
	}
	// ensure non-decreasing accumulated rain
	for i := 1; i < len(obs); i++ {
		if obs[i].RainAccumulated < obs[i-1].RainAccumulated {
			t.Fatalf("rain decreased at index %d (%v -> %v)", i, obs[i-1].RainAccumulated, obs[i].RainAccumulated)
		}
	}
	// coastal should on average increase per-point; run coastal and non-coastal and compare last values
	c1 := collectObservations(t, []string{"rain"}, base, SeedOptions{Points: 6, RandSeed: 5, Location: "coastal"})
	c2 := collectObservations(t, []string{"rain"}, base, SeedOptions{Points: 6, RandSeed: 5, Location: "inland"})
	if len(c1) == 0 || len(c2) == 0 {
		t.Fatalf("unexpected empty results for coastal/inland")
	}
	if c1[len(c1)-1].RainAccumulated < c2[len(c2)-1].RainAccumulated {
		t.Fatalf("expected coastal total >= inland total (coastal=%v inland=%v)", c1[len(c1)-1].RainAccumulated, c2[len(c2)-1].RainAccumulated)
	}
}

// TestSeedObservations_UVBaseAndNoise checks that desert UV is exactly 6 when
// UVNoise is 0.
func TestSeedObservations_UVBaseAndNoise(t *testing.T) {
	base := time.Unix(1600000000, 0)
	// desert with UVNoise=0 should always equal uvBase 6
	opts := SeedOptions{Points: 4, TimeSpacing: 1 * time.Minute, RandSeed: 11, UVNoise: 0, Location: "desert"}
	obs := collectObservations(t, []string{"uv"}, base, opts)
	if len(obs) == 0 {
		t.Fatal("no uv observations")
	}
	for _, o := range obs {
		if o.UV != 6 {
			t.Fatalf("expected UV=6 for desert with UVNoise=0, got %d", o.UV)
		}
	}
}
