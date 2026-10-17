//go:build !no_browser

package testhelpers

import (
	"testing"
	"time"

	"tempest-homekit-go/pkg/weather"
)

// TestSeedObservations_CountsAndTimestamps checks the total observation count
// and that each chart type's chunk steps back from base in 5-minute intervals.
func TestSeedObservations_CountsAndTimestamps(t *testing.T) {
	chartTypes := []string{"temperature", "humidity", "rain"}
	base := time.Now().Truncate(time.Second)
	points := 4

	var got []weather.Observation
	updater := func(o *weather.Observation) {
		got = append(got, *o)
	}

	// exercise the coastal profile for this test
	opts := SeedOptions{
		Points:      points,
		Season:      "winter",
		Location:    "coastal",
		TimeSpacing: 5 * time.Minute,
		TempNoise:   0.0,
		UVNoise:     0,
		RandSeed:    base.Unix(),
	}
	SeedObservationsWithOptions(t, updater, chartTypes, base, opts)

	expected := len(chartTypes) * points
	if len(got) != expected {
		t.Fatalf("expected %d observations, got %d", expected, len(got))
	}

	// verify timestamps per-chart chunk
	for ci := range chartTypes {
		for j := 0; j < points; j++ {
			idx := ci*points + j
			want := base.Add(-time.Duration(j*5) * time.Minute).Unix()
			if got[idx].Timestamp != want {
				t.Fatalf("timestamp mismatch at chart %d point %d: want %d got %d", ci, j, want, got[idx].Timestamp)
			}
		}
	}
}

// TestSeedObservations_ValueRanges checks that each chart type's values fall
// in plausible ranges for the desert profile and that timestamps are 5 minutes
// apart.
func TestSeedObservations_ValueRanges(t *testing.T) {
	chartTypes := []string{"temperature", "humidity", "rain", "pressure", "wind", "light", "uv"}
	base := time.Now()
	points := 3

	var got []weather.Observation
	updater := func(o *weather.Observation) {
		got = append(got, *o)
	}

	// exercise the desert profile with a small amount of noise
	opts := SeedOptions{
		Points:      points,
		Season:      "summer",
		Location:    "desert",
		TimeSpacing: 5 * time.Minute,
		TempNoise:   0.3,
		UVNoise:     1,
		RandSeed:    base.Unix(),
	}
	SeedObservationsWithOptions(t, updater, chartTypes, base, opts)

	if len(got) != len(chartTypes)*points {
		t.Fatalf("unexpected number of observations: %d", len(got))
	}

	for ci, ct := range chartTypes {
		// first point index for this chart
		baseIdx := ci * points
		switch ct {
		case "temperature":
			// expect reasonable summer temps > 14
			if got[baseIdx].AirTemperature < 14.0 {
				t.Fatalf("temperature too low: %v", got[baseIdx].AirTemperature)
			}
		case "humidity":
			if got[baseIdx].RelativeHumidity < 10 || got[baseIdx].RelativeHumidity > 100 {
				t.Fatalf("humidity out of range: %v", got[baseIdx].RelativeHumidity)
			}
		case "rain":
			// rain should be increasing per point; check first vs last
			first := got[baseIdx].RainAccumulated
			last := got[baseIdx+points-1].RainAccumulated
			if last <= first {
				t.Fatalf("rainAccumulated not increasing: first=%v last=%v", first, last)
			}
		case "pressure":
			p := got[baseIdx].StationPressure
			if p < 900 || p > 1100 {
				t.Fatalf("pressure out of sensible range: %v", p)
			}
		case "wind":
			wa := got[baseIdx].WindAvg
			wg := got[baseIdx].WindGust
			if wa < 0 || wg < wa {
				t.Fatalf("wind values invalid: avg=%v gust=%v", wa, wg)
			}
		case "light":
			if got[baseIdx].Illuminance < 0 {
				t.Fatalf("illuminance negative: %v", got[baseIdx].Illuminance)
			}
		case "uv":
			if got[baseIdx].UV < 0 || got[baseIdx].UV > 11 {
				t.Fatalf("uv out of expected range: %v", got[baseIdx].UV)
			}
		}
		// timestamps within chunk should be spaced by 5 minutes
		for j := 1; j < points; j++ {
			prev := got[baseIdx+j-1].Timestamp
			cur := got[baseIdx+j].Timestamp
			// SeedObservations produces descending timestamps (base, base-5m, base-10m..)
			if prev-cur != int64(5*60) {
				t.Fatalf("unexpected timestamp spacing for %s at idx %d: prev=%d cur=%d", ct, j, prev, cur)
			}
		}
	}
}
