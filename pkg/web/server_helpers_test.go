package web

import (
	"math"
	"testing"
	"time"

	"tempest-homekit-go/pkg/weather"
)

// TestCalculateSeaLevelPressure_Basic checks that calculateSeaLevelPressure
// returns a positive, non-NaN value for typical inputs.
func TestCalculateSeaLevelPressure_Basic(t *testing.T) {
	// Use known values to ensure output is reasonable
	stationPressure := 1000.0 // mb
	tempC := 15.0
	elevation := 100.0

	p := calculateSeaLevelPressure(stationPressure, tempC, elevation)
	if math.IsNaN(p) || p <= 0 {
		t.Fatalf("invalid sea level pressure: %v", p)
	}
}

// TestGetPressureDescription_Basic checks the Low, High and Normal pressure
// bands.
func TestGetPressureDescription_Basic(t *testing.T) {
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

// TestGetPressureTrendAndForecast checks that a steadily rising history is
// "Rising" and yields a non-empty forecast.
func TestGetPressureTrendAndForecast(t *testing.T) {
	// Build a history that trends up
	now := time.Now()
	h := []weather.Observation{}
	for i := 0; i < 5; i++ {
		obs := weather.Observation{Timestamp: now.Add(time.Duration(i) * time.Minute).Unix(), StationPressure: 1000.0 + float64(i), AirTemperature: 15.0}
		h = append(h, obs)
	}
	trend := getPressureTrend(h, 0)
	if trend != "Rising" {
		t.Fatalf("expected Rising, got %s", trend)
	}

	fc := getPressureWeatherForecast(1015, trend)
	if fc == "" {
		t.Fatalf("expected non-empty forecast for rising pressure")
	}
}

// TestCalculateDailyRainForTime_Basic checks that calculateDailyRainForTime
// reports positive rain for an increasing history.
func TestCalculateDailyRainForTime_Basic(t *testing.T) {
	ws := &WebServer{}
	// create history for today with increasing rain accumulation
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	ws.dataHistory = []weather.Observation{
		{Timestamp: start.Unix(), RainAccumulated: 1.0},
		{Timestamp: start.Add(1 * time.Hour).Unix(), RainAccumulated: 1.5},
		{Timestamp: start.Add(2 * time.Hour).Unix(), RainAccumulated: 2.0},
	}

	target := start.Add(90 * time.Minute)
	got := ws.calculateDailyRainForTime(target, start)
	if got <= 0 {
		t.Fatalf("expected positive daily rain total, got %v", got)
	}
}
