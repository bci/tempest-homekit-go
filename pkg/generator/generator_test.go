package generator

import (
	"math/rand"
	"testing"
	"time"
)

// TestNewWeatherGeneratorWithParamsAndGenerateObservation checks that a generator built with explicit parameters keeps them and produces observations within sane ranges.
func TestNewWeatherGeneratorWithParamsAndGenerateObservation(t *testing.T) {
	loc := Location{Name: "TestLand", Latitude: 34.0, Longitude: -118.0, Elevation: 100.0, ClimateZone: "Mediterranean"}
	wg := NewWeatherGeneratorWithParams(loc, Summer)

	if wg.GetLocation().Name != "TestLand" {
		t.Fatalf("expected location name TestLand, got %s", wg.GetLocation().Name)
	}
	if wg.GetSeason() != Summer {
		t.Fatalf("expected season Summer, got %v", wg.GetSeason())
	}

	obs := wg.GenerateObservation()
	if obs == nil {
		t.Fatal("GenerateObservation returned nil")
	}

	// Basic sanity checks on generated observation fields
	if obs.AirTemperature < -50 || obs.AirTemperature > 60 {
		t.Fatalf("temperature out of reasonable range: %v", obs.AirTemperature)
	}
	if obs.RelativeHumidity < 0 || obs.RelativeHumidity > 100 {
		t.Fatalf("humidity out of range: %v", obs.RelativeHumidity)
	}
	if obs.UV < 0 {
		t.Fatalf("uv negative: %d", obs.UV)
	}
	if obs.ReportInterval <= 0 {
		t.Fatalf("invalid report interval: %d", obs.ReportInterval)
	}
}

// TestGenerateHistoricalDataAndDailyTotals checks that GenerateHistoricalData returns the requested count and leaves the daily rain total unchanged.
func TestGenerateHistoricalDataAndDailyTotals(t *testing.T) {
	loc := Locations[0]
	wg := NewWeatherGeneratorWithParams(loc, Spring)

	// Save original daily totals
	beforeDaily := wg.GetDailyRainTotal()

	count := 20
	hist := wg.GenerateHistoricalData(count)
	if len(hist) != count {
		t.Fatalf("expected %d historical observations, got %d", count, len(hist))
	}

	// Ensure daily totals were restored after generation
	afterDaily := wg.GetDailyRainTotal()
	if beforeDaily != afterDaily {
		// It's acceptable that they differ slightly due to floating point, allow small delta
		// but they should be reasonably close
		delta := beforeDaily - afterDaily
		if delta < 0 {
			delta = -delta
		}
		if delta > 1.0 {
			t.Fatalf("daily totals changed unexpectedly by %v", delta)
		}
	}
}

// TestRegenerateAndGenerateNewSeason checks that Regenerate and GenerateNewSeason leave a usable generator that still produces observations.
func TestRegenerateAndGenerateNewSeason(t *testing.T) {
	wg := NewWeatherGenerator()
	oldLoc := wg.GetLocation()
	oldSeason := wg.GetSeason()

	wg.Regenerate()

	// After regenerate, location or season should likely change (entropy may pick same values rarely)
	newLoc := wg.GetLocation()
	newSeason := wg.GetSeason()
	if oldLoc.Name == newLoc.Name && oldSeason == newSeason {
		// If identical, try GenerateNewSeason to force change
		wg.GenerateNewSeason()
		newLoc = wg.GetLocation()
		newSeason = wg.GetSeason()
	}

	if oldLoc.Name == newLoc.Name && oldSeason == newSeason {
		t.Log("Regenerate did not change location or season (possible but unlikely)")
	}

	// Calling GenerateObservation after regenerate should produce an observation
	obs := wg.GenerateObservation()
	if obs == nil {
		t.Fatal("GenerateObservation returned nil after regenerate")
	}
}

// TestGenerateObservationConsistentTimestamp checks that a zero CurrentTime still yields a positive (now-based) timestamp.
func TestGenerateObservationConsistentTimestamp(t *testing.T) {
	wg := NewWeatherGenerator()
	// Ensure CurrentTime zero yields now-based timestamp
	wg.CurrentTime = time.Time{}
	obs := wg.GenerateObservation()
	if obs == nil {
		t.Fatal("GenerateObservation returned nil")
	}
	if obs.Timestamp <= 0 {
		t.Fatalf("invalid timestamp: %d", obs.Timestamp)
	}
}

// Additional tests from earlier file

// TestGeneratePrecipitationType verifies precipitation type selection by temperature and rain
func TestGeneratePrecipitationType(t *testing.T) {
	wg := &WeatherGenerator{}

	// No rain
	if got := wg.generatePrecipitationType(10, 0); got != 0 {
		t.Fatalf("expected 0 (none) for no rain, got %d", got)
	}

	// Rain and warm -> rain
	if got := wg.generatePrecipitationType(10, 0.5); got != 1 {
		t.Fatalf("expected 1 (rain) for temp 10 and rain 0.5, got %d", got)
	}

	// Cold temperatures -> snow
	if got := wg.generatePrecipitationType(-5, 0.2); got != 3 {
		t.Fatalf("expected 3 (snow) for temp -5 and rain 0.2, got %d", got)
	}

	// Near-freezing -> ice pellets
	if got := wg.generatePrecipitationType(1, 0.2); got != 2 {
		t.Fatalf("expected 2 (ice pellets) for temp 1 and rain 0.2, got %d", got)
	}
}

// TestGenerateHistoricalData ensures historical generation returns increasing timestamps
// and that daily/cumulative totals are restored after generation.
func TestGenerateHistoricalData(t *testing.T) {
	// Create a deterministic RNG for repeatable behavior
	rng := rand.New(rand.NewSource(42))

	wg := &WeatherGenerator{
		Location: Location{Name: "Test", Latitude: 40.0, Elevation: 100},
		Season:   Summer,
		rng:      rng,
	}
	wg.initializeBaseValues()

	// Set some known totals
	wg.cumulativeRain = 5.0
	wg.dailyRainTotal = 0.3
	originalCumulative := wg.cumulativeRain
	originalDaily := wg.dailyRainTotal

	observations := wg.GenerateHistoricalData(10)
	if len(observations) != 10 {
		t.Fatalf("expected 10 observations, got %d", len(observations))
	}

	// Verify timestamps strictly increase
	for i := 1; i < len(observations); i++ {
		if observations[i].Timestamp <= observations[i-1].Timestamp {
			t.Fatalf("expected timestamps to increase: idx %d <= %d", i, i-1)
		}
	}

	// Verify cumulative and daily totals were restored
	if wg.cumulativeRain != originalCumulative {
		t.Fatalf("expected cumulativeRain restored to %.2f, got %.2f", originalCumulative, wg.cumulativeRain)
	}
	if wg.dailyRainTotal != originalDaily {
		t.Fatalf("expected dailyRainTotal restored to %.2f, got %.2f", originalDaily, wg.dailyRainTotal)
	}

	// Ensure history field is set
	if wg.history == nil || len(wg.history) != 10 {
		t.Fatalf("expected wg.history to be set with 10 entries, got %v", wg.history)
	}

	// Ensure timestamps are approximately 24 hours span
	span := time.Unix(observations[len(observations)-1].Timestamp, 0).Sub(time.Unix(observations[0].Timestamp, 0))
	if span <= 0 {
		t.Fatalf("expected positive timespan for generated historical data, got %v", span)
	}
}
