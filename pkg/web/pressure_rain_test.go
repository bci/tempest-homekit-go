package web

import (
	"math"
	"testing"
	"time"

	"tempest-homekit-go/pkg/weather"
)

// TestCalculateSeaLevelPressure_Extra checks that sea-level pressure equals
// station pressure at zero elevation.
func TestCalculateSeaLevelPressure_Extra(t *testing.T) {
	stationPressure := 1013.25
	temperature := 15.0
	elevation := 0.0

	slp := calculateSeaLevelPressure(stationPressure, temperature, elevation)
	if math.Abs(slp-stationPressure) > 1e-9 {
		t.Fatalf("expected sea level pressure equal to station pressure at zero elevation; got %v vs %v", slp, stationPressure)
	}
}

// TestGetPressureDescription_Extra checks the Low, High and Normal pressure
// bands.
func TestGetPressureDescription_Extra(t *testing.T) {
	if getPressureDescription(970) != "Low" {
		t.Fatalf("expected Low for 970")
	}
	if getPressureDescription(1025) != "High" {
		t.Fatalf("expected High for 1025")
	}
	if getPressureDescription(1013) != "Normal" {
		t.Fatalf("expected Normal for 1013")
	}
}

// TestGetPressureTrend_Extra checks Rising, Falling and Stable trends from
// two-point histories.
func TestGetPressureTrend_Extra(t *testing.T) {
	// Rising
	d1 := []weather.Observation{{Timestamp: time.Now().Add(-10 * time.Minute).Unix(), StationPressure: 1000}, {Timestamp: time.Now().Unix(), StationPressure: 1002.5}}
	if getPressureTrend(d1, 0) != "Rising" {
		t.Fatalf("expected Rising, got %s", getPressureTrend(d1, 0))
	}

	// Falling
	d2 := []weather.Observation{{Timestamp: time.Now().Add(-10 * time.Minute).Unix(), StationPressure: 1005}, {Timestamp: time.Now().Unix(), StationPressure: 1002}}
	if getPressureTrend(d2, 0) != "Falling" {
		t.Fatalf("expected Falling, got %s", getPressureTrend(d2, 0))
	}

	// Stable (small change)
	d3 := []weather.Observation{{Timestamp: time.Now().Add(-10 * time.Minute).Unix(), StationPressure: 1010}, {Timestamp: time.Now().Unix(), StationPressure: 1010.5}}
	if getPressureTrend(d3, 0) != "Stable" {
		t.Fatalf("expected Stable, got %s", getPressureTrend(d3, 0))
	}
}

// TestGetPressureWeatherForecast_Extra checks every branch of
// getPressureWeatherForecast.
func TestGetPressureWeatherForecast_Extra(t *testing.T) {
	if getPressureWeatherForecast(1015, "Rising") != "Fair Weather" {
		t.Fatalf("unexpected forecast for Rising/1015")
	}
	if getPressureWeatherForecast(1010, "Rising") != "Storm Clearing" {
		t.Fatalf("unexpected forecast for Rising/1010")
	}
	if getPressureWeatherForecast(995, "Falling") != "Stormy" {
		t.Fatalf("unexpected forecast for Falling/995")
	}
	if getPressureWeatherForecast(1005, "Falling") != "Unsettled" {
		t.Fatalf("unexpected forecast for Falling/1005")
	}
	if getPressureWeatherForecast(1015, "Falling") != "Change Coming" {
		t.Fatalf("unexpected forecast for Falling/1015")
	}
	if getPressureWeatherForecast(1025, "Stable") != "Fair Weather" {
		t.Fatalf("unexpected forecast for Stable/1025")
	}
	if getPressureWeatherForecast(995, "Stable") != "Stormy" {
		t.Fatalf("unexpected forecast for Stable/995")
	}
	if getPressureWeatherForecast(1013, "Stable") != "Settled" {
		t.Fatalf("unexpected forecast for Stable/1013")
	}
}

// TestCalculateDailyRainForTimeAndAccumulation_Extra checks the latest-minus-
// earliest daily rain total from both calculateDailyRainForTime and
// calculateDailyRainAccumulation, and the single-observation case.
func TestCalculateDailyRainForTimeAndAccumulation_Extra(t *testing.T) {
	// Build a simple history for today: startOfDay reading 1.0, later reading 1.5
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	t1 := start.Add(2 * time.Hour)
	t2 := start.Add(6 * time.Hour)

	ws := &WebServer{}
	ws.dataHistory = []weather.Observation{
		{Timestamp: start.Unix(), RainAccumulated: 1.0},
		{Timestamp: t1.Unix(), RainAccumulated: 1.2},
		{Timestamp: t2.Unix(), RainAccumulated: 1.5},
	}

	// Calculate for t2
	got := ws.calculateDailyRainForTime(t2, start)
	if math.Abs(got-0.5) > 1e-6 {
		t.Fatalf("expected 0.5 daily rain for target time, got %v", got)
	}

	// calculateDailyRainAccumulation should yield same result (latest-earliest)
	total := ws.calculateDailyRainAccumulation()
	if math.Abs(total-0.5) > 1e-6 {
		t.Fatalf("expected daily accumulation 0.5, got %v", total)
	}

	// Single observation case: should return the single value if reasonable
	ws2 := &WebServer{}
	ws2.dataHistory = []weather.Observation{{Timestamp: start.Unix(), RainAccumulated: 0.8}}
	single := ws2.calculateDailyRainForTime(start.Add(1*time.Hour), start)
	if math.Abs(single-0.8) > 1e-6 {
		t.Fatalf("expected single-reading result 0.8, got %v", single)
	}
}
