package alarm

import (
	"testing"

	"tempest-homekit-go/pkg/weather"
)

// TestChangeDetectionAnyChange checks the "*" operator on lightning_count: the first
// observation sets a baseline, an unchanged value does not fire, and any increase or
// decrease (including back to zero) fires. Cases share one Alarm and preset its previous
// value with SetPreviousValue.
func TestChangeDetectionAnyChange(t *testing.T) {
	evaluator := NewEvaluator()
	alarm := &Alarm{
		Name:      "Lightning Monitor",
		Condition: "*lightning_count",
	}

	tests := []struct {
		name           string
		previousValue  *float64
		currentValue   float64
		expectedResult bool
		description    string
	}{
		{
			name:           "First observation - no trigger",
			previousValue:  nil,
			currentValue:   0,
			expectedResult: false,
			description:    "First observation establishes baseline",
		},
		{
			name:           "No change - no trigger",
			previousValue:  float64Ptr(0),
			currentValue:   0,
			expectedResult: false,
			description:    "Value unchanged from previous",
		},
		{
			name:           "Increase detected",
			previousValue:  float64Ptr(0),
			currentValue:   1,
			expectedResult: true,
			description:    "Value increased from 0 to 1",
		},
		{
			name:           "Another increase",
			previousValue:  float64Ptr(1),
			currentValue:   3,
			expectedResult: true,
			description:    "Value increased from 1 to 3",
		},
		{
			name:           "Decrease also triggers",
			previousValue:  float64Ptr(3),
			currentValue:   2,
			expectedResult: true,
			description:    "Value decreased from 3 to 2",
		},
		{
			name:           "Back to zero",
			previousValue:  float64Ptr(2),
			currentValue:   0,
			expectedResult: true,
			description:    "Value changed to 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up previous value if provided
			if tt.previousValue != nil {
				alarm.SetPreviousValue("lightning_count", *tt.previousValue)
			}

			obs := &weather.Observation{
				LightningStrikeCount: int(tt.currentValue),
			}

			result, err := evaluator.EvaluateWithAlarm(alarm.Condition, obs, alarm)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if result != tt.expectedResult {
				t.Errorf("%s: expected %v, got %v (previous: %v, current: %.0f)",
					tt.description, tt.expectedResult, result, tt.previousValue, tt.currentValue)
			}
		})
	}
}

// TestChangeDetectionIncrease checks the ">" operator on rain_rate: it fires only when
// the value rises above the previous one, not when it is steady or falls.
func TestChangeDetectionIncrease(t *testing.T) {
	evaluator := NewEvaluator()
	alarm := &Alarm{
		Name:      "Rain Increasing",
		Condition: ">rain_rate",
	}

	tests := []struct {
		name           string
		previousValue  *float64
		currentValue   float64
		expectedResult bool
		description    string
	}{
		{
			name:           "First observation - no trigger",
			previousValue:  nil,
			currentValue:   0,
			expectedResult: false,
			description:    "First observation establishes baseline",
		},
		{
			name:           "No change - no trigger",
			previousValue:  float64Ptr(0),
			currentValue:   0,
			expectedResult: false,
			description:    "No rain yet",
		},
		{
			name:           "Rain starts",
			previousValue:  float64Ptr(0),
			currentValue:   0.5,
			expectedResult: true,
			description:    "Rain started (0 to 0.5)",
		},
		{
			name:           "Rain increases",
			previousValue:  float64Ptr(0.5),
			currentValue:   2.0,
			expectedResult: true,
			description:    "Rain increased (0.5 to 2.0)",
		},
		{
			name:           "Rain steady - no trigger",
			previousValue:  float64Ptr(2.0),
			currentValue:   2.0,
			expectedResult: false,
			description:    "Rain rate unchanged",
		},
		{
			name:           "Rain decreases - no trigger",
			previousValue:  float64Ptr(2.0),
			currentValue:   1.0,
			expectedResult: false,
			description:    "Rain decreased (should not trigger increase operator)",
		},
		{
			name:           "Rain increases again",
			previousValue:  float64Ptr(1.0),
			currentValue:   3.0,
			expectedResult: true,
			description:    "Rain increased again (1.0 to 3.0)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up previous value if provided
			if tt.previousValue != nil {
				alarm.SetPreviousValue("rain_rate", *tt.previousValue)
			}

			obs := &weather.Observation{
				RainAccumulated: tt.currentValue,
			}

			result, err := evaluator.EvaluateWithAlarm(alarm.Condition, obs, alarm)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if result != tt.expectedResult {
				t.Errorf("%s: expected %v, got %v (previous: %v, current: %.1f)",
					tt.description, tt.expectedResult, result, tt.previousValue, tt.currentValue)
			}
		})
	}
}

// TestChangeDetectionDecrease checks the "<" operator on lightning_distance: it fires
// only when the distance drops below the previous one, not when it is steady or grows.
func TestChangeDetectionDecrease(t *testing.T) {
	evaluator := NewEvaluator()
	alarm := &Alarm{
		Name:      "Lightning Getting Closer",
		Condition: "<lightning_distance",
	}

	tests := []struct {
		name           string
		previousValue  *float64
		currentValue   float64
		expectedResult bool
		description    string
	}{
		{
			name:           "First observation - no trigger",
			previousValue:  nil,
			currentValue:   50,
			expectedResult: false,
			description:    "First observation establishes baseline",
		},
		{
			name:           "Distance same - no trigger",
			previousValue:  float64Ptr(50),
			currentValue:   50,
			expectedResult: false,
			description:    "Distance unchanged",
		},
		{
			name:           "Lightning closer",
			previousValue:  float64Ptr(50),
			currentValue:   30,
			expectedResult: true,
			description:    "Lightning got closer (50 to 30km)",
		},
		{
			name:           "Much closer",
			previousValue:  float64Ptr(30),
			currentValue:   10,
			expectedResult: true,
			description:    "Lightning much closer (30 to 10km)",
		},
		{
			name:           "Very close",
			previousValue:  float64Ptr(10),
			currentValue:   2,
			expectedResult: true,
			description:    "Lightning very close (10 to 2km)",
		},
		{
			name:           "Lightning farther - no trigger",
			previousValue:  float64Ptr(2),
			currentValue:   5,
			expectedResult: false,
			description:    "Lightning moved away (should not trigger decrease)",
		},
		{
			name:           "Closer again",
			previousValue:  float64Ptr(5),
			currentValue:   3,
			expectedResult: true,
			description:    "Lightning closer again (5 to 3km)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up previous value if provided
			if tt.previousValue != nil {
				alarm.SetPreviousValue("lightning_distance", *tt.previousValue)
			}

			obs := &weather.Observation{
				LightningStrikeAvg: tt.currentValue,
			}

			result, err := evaluator.EvaluateWithAlarm(alarm.Condition, obs, alarm)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if result != tt.expectedResult {
				t.Errorf("%s: expected %v, got %v (previous: %v, current: %.0fkm)",
					tt.description, tt.expectedResult, result, tt.previousValue, tt.currentValue)
			}
		})
	}
}

// TestChangeDetectionWithCompoundConditions checks change-detection clauses combined
// with ordinary comparisons through && and ||.
func TestChangeDetectionWithCompoundConditions(t *testing.T) {
	evaluator := NewEvaluator()

	tests := []struct {
		name           string
		condition      string
		setupAlarm     func(*Alarm)
		obs            *weather.Observation
		expectedResult bool
		description    string
	}{
		{
			name:      "Any lightning AND close",
			condition: "*lightning_count && lightning_distance < 10",
			setupAlarm: func(a *Alarm) {
				a.SetPreviousValue("lightning_count", 0)
			},
			obs: &weather.Observation{
				LightningStrikeCount: 1,
				LightningStrikeAvg:   5,
			},
			expectedResult: true,
			description:    "Lightning detected and close",
		},
		{
			name:      "Any lightning BUT far",
			condition: "*lightning_count && lightning_distance < 10",
			setupAlarm: func(a *Alarm) {
				a.SetPreviousValue("lightning_count", 0)
			},
			obs: &weather.Observation{
				LightningStrikeCount: 1,
				LightningStrikeAvg:   20,
			},
			expectedResult: false,
			description:    "Lightning detected but too far",
		},
		{
			name:      "Rain increasing OR wind increasing",
			condition: ">rain_rate || >wind_gust",
			setupAlarm: func(a *Alarm) {
				a.SetPreviousValue("rain_rate", 0)
				a.SetPreviousValue("wind_gust", 10)
			},
			obs: &weather.Observation{
				RainAccumulated: 1.0,
				WindGust:        10,
			},
			expectedResult: true,
			description:    "Rain increased (wind unchanged)",
		},
		{
			name:      "Temperature high AND humidity increasing",
			condition: "temperature > 30 && >humidity",
			setupAlarm: func(a *Alarm) {
				a.SetPreviousValue("humidity", 60)
			},
			obs: &weather.Observation{
				AirTemperature:   35,
				RelativeHumidity: 75,
			},
			expectedResult: true,
			description:    "High temp and humidity rising",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			alarm := &Alarm{
				Name:      tt.name,
				Condition: tt.condition,
			}

			if tt.setupAlarm != nil {
				tt.setupAlarm(alarm)
			}

			result, err := evaluator.EvaluateWithAlarm(tt.condition, tt.obs, alarm)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if result != tt.expectedResult {
				t.Errorf("%s: expected %v, got %v", tt.description, tt.expectedResult, result)
			}
		})
	}
}

// TestChangeDetectionMultipleFields checks that one Alarm tracks previous values per
// field independently, so evaluating different fields updates only that field's
// baseline.
func TestChangeDetectionMultipleFields(t *testing.T) {
	evaluator := NewEvaluator()
	alarm := &Alarm{
		Name:      "Multi-field Monitor",
		Condition: "*lightning_count",
	}

	// Track multiple fields independently
	alarm.SetPreviousValue("lightning_count", 0)
	alarm.SetPreviousValue("rain_rate", 0)
	alarm.SetPreviousValue("wind_gust", 10)

	// Change lightning
	obs1 := &weather.Observation{
		LightningStrikeCount: 1,
		RainAccumulated:      0,
		WindGust:             10,
	}
	result, err := evaluator.EvaluateWithAlarm("*lightning_count", obs1, alarm)
	if err != nil {
		t.Fatalf("Error evaluating lightning: %v", err)
	}
	if !result {
		t.Error("Expected lightning change to trigger")
	}

	// Change rain (using same alarm object)
	obs2 := &weather.Observation{
		LightningStrikeCount: 1,
		RainAccumulated:      0.5,
		WindGust:             10,
	}
	result, err = evaluator.EvaluateWithAlarm(">rain_rate", obs2, alarm)
	if err != nil {
		t.Fatalf("Error evaluating rain: %v", err)
	}
	if !result {
		t.Error("Expected rain increase to trigger")
	}

	// Verify previous values are tracked independently
	prev, ok := alarm.GetPreviousValue("lightning_count")
	if !ok || prev != 1 {
		t.Errorf("Expected lightning_count previous value 1, got %v (exists: %v)", prev, ok)
	}

	prev, ok = alarm.GetPreviousValue("rain_rate")
	if !ok || prev != 0.5 {
		t.Errorf("Expected rain_rate previous value 0.5, got %v (exists: %v)", prev, ok)
	}
}

// TestChangeDetectionErrors checks that change detection errors without an alarm
// context, on an unknown field, and when the operator has no field name.
func TestChangeDetectionErrors(t *testing.T) {
	evaluator := NewEvaluator()

	tests := []struct {
		name        string
		condition   string
		alarm       *Alarm
		expectError bool
		description string
	}{
		{
			name:        "No alarm context",
			condition:   "*lightning_count",
			alarm:       nil,
			expectError: true,
			description: "Change detection requires alarm context",
		},
		{
			name:        "Invalid field",
			condition:   "*invalid_field",
			alarm:       &Alarm{Name: "Test"},
			expectError: true,
			description: "Unknown field should error",
		},
		{
			name:        "Empty condition",
			condition:   "*",
			alarm:       &Alarm{Name: "Test"},
			expectError: true,
			description: "Operator without field",
		},
		{
			name:        "Just operator",
			condition:   ">",
			alarm:       &Alarm{Name: "Test"},
			expectError: true,
			description: "Operator without field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obs := &weather.Observation{
				LightningStrikeCount: 1,
			}

			_, err := evaluator.EvaluateWithAlarm(tt.condition, obs, tt.alarm)
			if tt.expectError && err == nil {
				t.Errorf("%s: expected error but got none", tt.description)
			}
			if !tt.expectError && err != nil {
				t.Errorf("%s: unexpected error: %v", tt.description, err)
			}
		})
	}
}

// TestBackwardCompatibility checks that plain comparisons give the same result through
// Evaluate and through EvaluateWithAlarm with a nil alarm.
func TestBackwardCompatibility(t *testing.T) {
	evaluator := NewEvaluator()

	// Test that regular conditions still work without alarm context
	tests := []struct {
		name      string
		condition string
		obs       *weather.Observation
		expected  bool
	}{
		{
			name:      "Simple comparison",
			condition: "temperature > 30",
			obs: &weather.Observation{
				AirTemperature: 35,
			},
			expected: true,
		},
		{
			name:      "Compound condition",
			condition: "humidity > 80 && temperature > 30",
			obs: &weather.Observation{
				AirTemperature:   35,
				RelativeHumidity: 85,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test old Evaluate method (no alarm context)
			result, err := evaluator.Evaluate(tt.condition, tt.obs)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}

			// Test new EvaluateWithAlarm method with nil alarm
			result2, err := evaluator.EvaluateWithAlarm(tt.condition, tt.obs, nil)
			if err != nil {
				t.Fatalf("Unexpected error with nil alarm: %v", err)
			}
			if result2 != tt.expected {
				t.Errorf("Expected %v with nil alarm, got %v", tt.expected, result2)
			}
		})
	}
}

// float64Ptr returns a pointer to a copy of v, so table cases can express an optional
// previous value (nil means no baseline).
func float64Ptr(v float64) *float64 {
	return &v
}
