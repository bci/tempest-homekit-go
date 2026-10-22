package web

import (
	"math"
	"testing"
)

// TestCalculateSeaLevelPressure_Sanity checks that sea-level pressure equals
// station pressure at 0 m and is higher at 100 m.
func TestCalculateSeaLevelPressure_Sanity(t *testing.T) {
	stationPressure := 1013.25
	temp := 15.0
	elevation0 := 0.0
	elevation1 := 100.0

	p0 := calculateSeaLevelPressure(stationPressure, temp, elevation0)
	p1 := calculateSeaLevelPressure(stationPressure, temp, elevation1)

	if math.Abs(p0-stationPressure) > 0.0001 {
		t.Fatalf("expected sea level pressure at elevation 0 to equal station pressure; got %f vs %f", p0, stationPressure)
	}
	if p1 <= p0 {
		t.Fatalf("expected sea level pressure at higher elevation to be greater: %f <= %f", p1, p0)
	}
}
