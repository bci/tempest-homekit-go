//go:build !no_browser

// Package testhelpers provides helpers for the headless-browser UI tests: a
// seeded generator of synthetic weather observations (SeedObservationsWithOptions)
// and a popout-chart assertion for chromedp sessions (AssertPopoutDatasetOrdering).
//
// Every file is behind the !no_browser build tag, so building with
// -tags=no_browser leaves the package empty.
package testhelpers

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"tempest-homekit-go/pkg/weather"
)

// SeedOptions configures the behavior of SeedObservationsWithOptions.
//
// Fields:
//   - Points is the number of observations per chart type; <= 0 means 5.
//   - Season "winter" lowers temperature (to 8 C base) and humidity; any other
//     value is treated as the default (20 C base).
//   - Location "desert" raises temperature and UV and lowers humidity;
//     "coastal" adds 1.0 to every rain increment; other values are neutral.
//   - TimeSpacing is the gap between successive timestamps; <= 0 means 5 minutes.
//   - TempNoise is the standard deviation of Gaussian noise added to temperature.
//   - UVNoise is the maximum random amount added to the UV base (inclusive);
//     it must not be negative.
//   - RandSeed seeds the generator; equal options and seed give equal output.
type SeedOptions struct {
	Points      int
	Season      string
	Location    string
	TimeSpacing time.Duration
	TempNoise   float64
	UVNoise     int
	RandSeed    int64
}

// SeedObservationsWithOptions generates deterministic, simple synthetic
// weather.Observation values for UI tests. It calls the provided updater for
// each generated Observation in chartTypes order (all points for chartTypes[0],
// then chartTypes[1], ...). Timestamps are produced as base, base-TimeSpacing,
// base-2*TimeSpacing etc so tests can assert spacing.
//
// Each chart type fills only its own fields ("temperature", "humidity",
// "rain", "pressure", "wind", "light", "uv"; anything else gets a temperature
// around 15 C) and leaves the rest zero. A fresh Observation is allocated for
// every call to updater. Rain accumulates in generation order, so
// RainAccumulated grows as timestamps go back in time. Randomness comes only
// from a source seeded with opts.RandSeed, so output is reproducible.
func SeedObservationsWithOptions(t *testing.T, updater func(*weather.Observation), chartTypes []string, base time.Time, opts SeedOptions) {
	t.Helper()
	if opts.Points <= 0 {
		opts.Points = 5
	}
	if opts.TimeSpacing <= 0 {
		opts.TimeSpacing = 5 * time.Minute
	}
	r := rand.New(rand.NewSource(opts.RandSeed))

	for _, ct := range chartTypes {
		var accumulatedRain float64
		for j := 0; j < opts.Points; j++ {
			ts := base.Add(-time.Duration(j) * opts.TimeSpacing).Unix()
			o := &weather.Observation{Timestamp: ts}

			switch ct {
			case "temperature":
				baseTemp := 20.0
				if opts.Season == "winter" {
					baseTemp = 8.0
				}
				if opts.Location == "desert" {
					baseTemp += 6.0
				}
				// small gaussian noise
				o.AirTemperature = baseTemp + r.NormFloat64()*opts.TempNoise
			case "humidity":
				h := 60.0
				if opts.Location == "desert" {
					h = 20.0
				}
				if opts.Season == "winter" {
					h -= 5.0
				}
				o.RelativeHumidity = clamp(h+(r.Float64()*10.0-5.0), 0, 100)
			case "rain":
				inc := r.Float64() * 2.0
				if opts.Location == "coastal" {
					inc += 1.0
				}
				accumulatedRain += inc
				o.RainAccumulated = accumulatedRain
				o.RainDailyTotal = accumulatedRain
			case "pressure":
				o.StationPressure = 1000.0 + r.Float64()*50.0
			case "wind":
				wa := r.Float64() * 10.0
				wg := wa + r.Float64()*5.0
				o.WindAvg = wa
				o.WindGust = wg
			case "light":
				o.Illuminance = math.Abs(r.NormFloat64()) * 200.0
			case "uv":
				uvBase := 3
				if opts.Location == "desert" {
					uvBase = 6
				}
				o.UV = uvBase + r.Intn(opts.UVNoise+1)
			default:
				// generic sensible defaults
				o.AirTemperature = 15.0 + r.NormFloat64()*1.5
			}

			updater(o)
		}
	}
}

// clamp returns v limited to the closed range [lo, hi].
func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
