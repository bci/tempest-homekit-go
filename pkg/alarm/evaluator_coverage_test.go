package alarm

import (
	"testing"

	"tempest-homekit-go/pkg/weather"
)

// TestEvaluator_Compare_AllCases checks every comparison operator for the greater, less
// and equal cases on the temperature field.
func TestEvaluator_Compare_AllCases(t *testing.T) {
	e := NewEvaluator()

	tests := []struct {
		name      string
		condition string
		temp      float64
		expected  bool
	}{
		{"Greater than true", "temperature > 5", 10.0, true},
		{"Greater than false", "temperature > 10", 5.0, false},
		{"Greater than equal", "temperature > 10", 10.0, false},
		{"Less than true", "temperature < 10", 5.0, true},
		{"Less than false", "temperature < 5", 10.0, false},
		{"Less than equal", "temperature < 10", 10.0, false},
		{"Greater or equal true (greater)", "temperature >= 5", 10.0, true},
		{"Greater or equal true (equal)", "temperature >= 10", 10.0, true},
		{"Greater or equal false", "temperature >= 10", 5.0, false},
		{"Less or equal true (less)", "temperature <= 10", 5.0, true},
		{"Less or equal true (equal)", "temperature <= 10", 10.0, true},
		{"Less or equal false", "temperature <= 5", 10.0, false},
		{"Equal true", "temperature == 10", 10.0, true},
		{"Equal false", "temperature == 5", 10.0, false},
		{"Not equal true", "temperature != 5", 10.0, true},
		{"Not equal false", "temperature != 10", 10.0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obs := &weather.Observation{
				AirTemperature: tt.temp,
			}

			result, err := e.Evaluate(tt.condition, obs)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("Expected %v for condition '%s' with temp %.1f, got %v", tt.expected, tt.condition, tt.temp, result)
			}
		})
	}
}

// TestEvaluator_ChangeDetection_EdgeCases checks that "*temperature" does not fire on the
// first observation, fires when the value changes and does not fire when it repeats.
func TestEvaluator_ChangeDetection_EdgeCases(t *testing.T) {
	e := NewEvaluator()
	alarm := &Alarm{
		Name:      "Change Test",
		Condition: "*temperature",
	}

	// First observation - no previous value
	obs1 := &weather.Observation{
		AirTemperature: 20.0,
	}

	result, err := e.EvaluateWithAlarm("*temperature", obs1, alarm)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	// First observation should not trigger (no previous value)
	if result {
		t.Error("Expected false for first observation (no previous value)")
	}

	// Second observation - value changed
	obs2 := &weather.Observation{
		AirTemperature: 25.0,
	}

	result, err = e.EvaluateWithAlarm("*temperature", obs2, alarm)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !result {
		t.Error("Expected true for changed value")
	}

	// Third observation - same value
	obs3 := &weather.Observation{
		AirTemperature: 25.0,
	}

	result, err = e.EvaluateWithAlarm("*temperature", obs3, alarm)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result {
		t.Error("Expected false for unchanged value")
	}
}

// TestEvaluator_IncreaseDetection checks that ">temperature" fires on a rise and not on a
// fall after a baseline observation.
func TestEvaluator_IncreaseDetection(t *testing.T) {
	e := NewEvaluator()
	alarm := &Alarm{
		Name:      "Increase Test",
		Condition: ">temperature",
	}

	// First observation
	obs1 := &weather.Observation{
		AirTemperature: 20.0,
	}
	if _, err := e.EvaluateWithAlarm(">temperature", obs1, alarm); err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Temperature increases
	obs2 := &weather.Observation{
		AirTemperature: 25.0,
	}
	result, err := e.EvaluateWithAlarm(">temperature", obs2, alarm)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !result {
		t.Error("Expected true for temperature increase")
	}

	// Temperature decreases
	obs3 := &weather.Observation{
		AirTemperature: 20.0,
	}
	result, err = e.EvaluateWithAlarm(">temperature", obs3, alarm)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result {
		t.Error("Expected false for temperature decrease")
	}
}

// TestEvaluator_DecreaseDetection checks that "<temperature" fires on a fall and not on a
// rise after a baseline observation.
func TestEvaluator_DecreaseDetection(t *testing.T) {
	e := NewEvaluator()
	alarm := &Alarm{
		Name:      "Decrease Test",
		Condition: "<temperature",
	}

	// First observation
	obs1 := &weather.Observation{
		AirTemperature: 25.0,
	}
	if _, err := e.EvaluateWithAlarm("<temperature", obs1, alarm); err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Temperature decreases
	obs2 := &weather.Observation{
		AirTemperature: 20.0,
	}
	result, err := e.EvaluateWithAlarm("<temperature", obs2, alarm)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !result {
		t.Error("Expected true for temperature decrease")
	}

	// Temperature increases
	obs3 := &weather.Observation{
		AirTemperature: 25.0,
	}
	result, err = e.EvaluateWithAlarm("<temperature", obs3, alarm)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result {
		t.Error("Expected false for temperature increase")
	}
}

// TestGetAvailableFields checks that GetAvailableFields is non-empty and includes every
// documented field name and alias.
func TestGetAvailableFields(t *testing.T) {
	e := NewEvaluator()
	fields := e.GetAvailableFields()

	expectedFields := []string{
		"temperature", "temp", "humidity", "pressure",
		"wind_speed", "wind", "wind_gust", "wind_direction",
		"lux", "light", "uv", "uv_index",
		"rain_rate", "rain_daily",
		"lightning_count", "lightning_distance",
	}

	if len(fields) == 0 {
		t.Error("Expected non-empty list of available fields")
	}

	// Check that expected fields are present
	for _, expected := range expectedFields {
		found := false
		for _, field := range fields {
			if field == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected field '%s' not found in available fields", expected)
		}
	}
}
