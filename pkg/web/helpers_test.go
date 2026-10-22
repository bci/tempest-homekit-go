package web

import (
	"math"
	"testing"
	"time"

	"tempest-homekit-go/pkg/weather"
)

// TestCalculateSeaLevelPressure checks that sea-level pressure equals station
// pressure at 0 m and is higher at 1000 m.
func TestCalculateSeaLevelPressure(t *testing.T) {
	// Known inputs - station pressure around sea level should be near station value
	stationPressure := 1013.25
	temp := 15.0
	elevation := 0.0

	p := calculateSeaLevelPressure(stationPressure, temp, elevation)
	if math.Abs(p-stationPressure) > 0.0001 {
		t.Fatalf("expected sea level pressure to equal station pressure at elevation 0, got %.4f", p)
	}

	// Higher elevation should increase sea level pressure (since station pressure is lower)
	elevation = 1000.0 // meters
	p2 := calculateSeaLevelPressure(stationPressure, temp, elevation)
	if p2 <= p {
		t.Fatalf("expected sea level pressure at elevation 1000m to be greater than at 0m: %.4f <= %.4f", p2, p)
	}
}

// TestGetPressureDescription checks the Low, High and Normal pressure bands.
func TestGetPressureDescription(t *testing.T) {
	if getPressureDescription(970) != "Low" {
		t.Fatalf("expected Low for 970")
	}
	if getPressureDescription(1030) != "High" {
		t.Fatalf("expected High for 1030")
	}
	if getPressureDescription(1013) != "Normal" {
		t.Fatalf("expected Normal for 1013")
	}
}

// TestGetPressureWeatherForecast checks the Rising/Fair Weather and the two
// Falling forecast branches.
func TestGetPressureWeatherForecast(t *testing.T) {
	if getPressureWeatherForecast(1025, "Rising") != "Fair Weather" {
		t.Fatalf("unexpected forecast for 1025 Rising")
	}
	if getPressureWeatherForecast(1005, "Falling") != "Unsettled" {
		t.Fatalf("unexpected forecast for 1005 Falling")
	}
	if getPressureWeatherForecast(995, "Falling") != "Stormy" {
		t.Fatalf("unexpected forecast for 995 Falling")
	}
}

// TestGetPressureTrend checks Rising, Falling and Stable trends from short
// synthetic histories.
func TestGetPressureTrend(t *testing.T) {
	// Build simple history with rising pressure over time
	now := time.Now()
	history := []weather.Observation{
		{Timestamp: now.Add(-10 * time.Minute).Unix(), StationPressure: 1000.0, AirTemperature: 15.0},
		{Timestamp: now.Add(-5 * time.Minute).Unix(), StationPressure: 1002.0, AirTemperature: 15.0},
		{Timestamp: now.Unix(), StationPressure: 1004.5, AirTemperature: 15.0},
	}

	trend := getPressureTrend(history, 0.0)
	if trend != "Rising" {
		t.Fatalf("expected Rising trend, got %s", trend)
	}

	// Falling trend
	history = []weather.Observation{
		{Timestamp: now.Add(-10 * time.Minute).Unix(), StationPressure: 1015.0, AirTemperature: 15.0},
		{Timestamp: now.Add(-5 * time.Minute).Unix(), StationPressure: 1013.0, AirTemperature: 15.0},
		{Timestamp: now.Unix(), StationPressure: 1010.5, AirTemperature: 15.0},
	}
	trend = getPressureTrend(history, 0.0)
	if trend != "Falling" {
		t.Fatalf("expected Falling trend, got %s", trend)
	}

	// Stable
	history = []weather.Observation{
		{Timestamp: now.Add(-10 * time.Minute).Unix(), StationPressure: 1013.0, AirTemperature: 15.0},
		{Timestamp: now.Unix(), StationPressure: 1013.4, AirTemperature: 15.0},
	}
	trend = getPressureTrend(history, 0.0)
	if trend != "Stable" {
		t.Fatalf("expected Stable trend, got %s", trend)
	}
}
