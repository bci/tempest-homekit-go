//go:build !no_browser

package testhelpers

import (
	"fmt"
	"testing"
	"time"

	"tempest-homekit-go/pkg/weather"
)

// TestExample_SeedObservationsWithOptions checks that one observation is
// produced per chart type per point, and prints them in verbose runs.
//
// Example showing how to call SeedObservationsWithOptions in a test. This is
// not a unit test per-se, but having it as an example helps other test authors
// discover the options-driven seeder.
func TestExample_SeedObservationsWithOptions(t *testing.T) {
	base := time.Now().Truncate(time.Second)
	chartTypes := []string{"temperature", "humidity", "uv"}

	var got []weather.Observation
	updater := func(o *weather.Observation) {
		got = append(got, *o)
	}

	opts := SeedOptions{
		Points:      5,
		Season:      "summer",
		Location:    "coastal",
		TimeSpacing: 5 * time.Minute,
		TempNoise:   0.5,
		UVNoise:     2,
		RandSeed:    base.Unix(),
	}

	SeedObservationsWithOptions(t, updater, chartTypes, base, opts)

	// basic sanity check
	if len(got) != len(chartTypes)*opts.Points {
		t.Fatalf("unexpected observation count: %d", len(got))
	}

	// print a sample for debug visibility in verbose runs
	if testing.Verbose() {
		for i, o := range got {
			fmt.Printf("%02d: %+v\n", i, o)
		}
	}
}
