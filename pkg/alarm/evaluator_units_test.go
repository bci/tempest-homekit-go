package alarm

import (
	"testing"

	"tempest-homekit-go/pkg/weather"
)

// TestUnitConversionTemperature tests temperature unit conversions (F to C)
// in conditions: F/f thresholds are converted to Celsius, C/c and bare numbers are taken
// as Celsius, and mixed units work inside compound conditions.
func TestUnitConversionTemperature(t *testing.T) {
	evaluator := NewEvaluator()

	tests := []struct {
		name      string
		condition string
		tempC     float64
		expected  bool
	}{
		// Fahrenheit conversions
		{
			name:      "80F equals 26.67C",
			condition: "temperature > 80F",
			tempC:     27.0, // Above 26.67C
			expected:  true,
		},
		{
			name:      "80F equals 26.67C - false case",
			condition: "temperature > 80F",
			tempC:     26.0, // Below 26.67C
			expected:  false,
		},
		{
			name:      "32F equals 0C (freezing)",
			condition: "temperature < 32F",
			tempC:     -1.0, // Below 0C
			expected:  true,
		},
		{
			name:      "212F equals 100C (boiling)",
			condition: "temperature >= 212F",
			tempC:     100.0, // Exactly 100C
			expected:  true,
		},
		{
			name:      "Lowercase f suffix",
			condition: "temperature > 75f",
			tempC:     24.0, // 24C = 75.2F
			expected:  true,
		},

		// Explicit Celsius (no conversion needed)
		{
			name:      "30C explicit",
			condition: "temperature > 30C",
			tempC:     31.0,
			expected:  true,
		},
		{
			name:      "Lowercase c suffix",
			condition: "temperature > 30c",
			tempC:     31.0,
			expected:  true,
		},

		// No unit suffix (assumed Celsius)
		{
			name:      "No unit suffix defaults to Celsius",
			condition: "temperature > 25",
			tempC:     26.0,
			expected:  true,
		},

		// Complex conditions
		{
			name:      "Compound condition with F and C",
			condition: "temperature > 32F && temperature < 100C",
			tempC:     50.0, // 50C is above 32F (0C) and below 100C
			expected:  true,
		},
		{
			name:      "OR condition with mixed units",
			condition: "temperature < 32F || temperature > 95F",
			tempC:     40.0, // 40C = 104F, above 95F
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obs := &weather.Observation{
				AirTemperature: tt.tempC,
			}

			result, err := evaluator.Evaluate(tt.condition, obs)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("Condition '%s' with temp %.2fC: expected %v, got %v",
					tt.condition, tt.tempC, tt.expected, result)
			}
		})
	}
}

// TestUnitConversionWindSpeed tests wind speed unit conversions (mph to m/s)
// in conditions: mph in any case is converted, m/s, M/S, "ms" and bare numbers are taken
// as m/s, and mixed units work inside compound conditions.
func TestUnitConversionWindSpeed(t *testing.T) {
	evaluator := NewEvaluator()

	tests := []struct {
		name      string
		condition string
		windMS    float64
		expected  bool
	}{
		// MPH conversions
		{
			name:      "25mph equals ~11.18m/s",
			condition: "wind_speed > 25mph",
			windMS:    11.5, // Above 11.18 m/s
			expected:  true,
		},
		{
			name:      "25mph equals ~11.18m/s - false case",
			condition: "wind_speed > 25mph",
			windMS:    10.0, // Below 11.18 m/s
			expected:  false,
		},
		{
			name:      "50mph wind gust",
			condition: "wind_gust > 50mph",
			windMS:    25.0, // 25 m/s = ~55.9 mph
			expected:  true,
		},
		{
			name:      "Lowercase mph",
			condition: "wind_speed > 10mph",
			windMS:    5.0, // 5 m/s = ~11.2 mph
			expected:  true,
		},
		{
			name:      "Uppercase MPH",
			condition: "wind_speed > 10MPH",
			windMS:    5.0, // 5 m/s = ~11.2 mph
			expected:  true,
		},

		// Explicit m/s (no conversion needed)
		{
			name:      "10m/s explicit",
			condition: "wind_speed > 10m/s",
			windMS:    11.0,
			expected:  true,
		},
		{
			name:      "Explicit M/S uppercase",
			condition: "wind_speed > 10M/S",
			windMS:    11.0,
			expected:  true,
		},
		{
			name:      "ms without slash",
			condition: "wind_speed > 10ms",
			windMS:    11.0,
			expected:  true,
		},

		// No unit suffix (assumed m/s)
		{
			name:      "No unit suffix defaults to m/s",
			condition: "wind_speed > 10",
			windMS:    11.0,
			expected:  true,
		},

		// Complex conditions
		{
			name:      "Compound condition with mph and m/s",
			condition: "wind_speed > 10mph && wind_gust < 50mph",
			windMS:    15.0, // 15 m/s = ~33.6 mph (above 10mph, below 50mph)
			expected:  true,
		},
		{
			name:      "OR condition with mixed units",
			condition: "wind_speed < 5mph || wind_speed > 30mph",
			windMS:    15.0, // 15 m/s = ~33.6 mph, above 30mph
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obs := &weather.Observation{
				WindAvg:  tt.windMS,
				WindGust: tt.windMS,
			}

			result, err := evaluator.Evaluate(tt.condition, obs)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("Condition '%s' with wind %.2fm/s: expected %v, got %v",
					tt.condition, tt.windMS, tt.expected, result)
			}
		})
	}
}

// TestParseValueWithUnits tests the unit parsing function directly:
// parseValueWithUnits converts by field, leaves fields without units unchanged and
// errors on unparseable input.
func TestParseValueWithUnits(t *testing.T) {
	evaluator := NewEvaluator()

	tests := []struct {
		name     string
		value    string
		field    string
		expected float64
		hasError bool
	}{
		// Temperature conversions
		{"80F to C", "80F", "temperature", 26.666666666666668, false},
		{"32F to C", "32F", "temp", 0.0, false},
		{"212F to C", "212F", "temperature", 100.0, false},
		{"75f lowercase", "75f", "temperature", 23.88888888888889, false},
		{"30C explicit", "30C", "temperature", 30.0, false},
		{"30c lowercase", "30c", "temp", 30.0, false},
		{"25 no unit", "25", "temperature", 25.0, false},

		// Wind speed conversions
		{"25mph to m/s", "25mph", "wind_speed", 11.176, false},
		{"50mph to m/s", "50mph", "wind_gust", 22.352, false},
		{"10MPH uppercase", "10MPH", "wind_speed", 4.4704, false},
		{"10m/s explicit", "10m/s", "wind_speed", 10.0, false},
		{"10M/S uppercase", "10M/S", "wind_gust", 10.0, false},
		{"10ms no slash", "10ms", "wind_speed", 10.0, false},
		{"15 no unit", "15", "wind_speed", 15.0, false},

		// Other fields (no conversion)
		{"humidity no unit", "80", "humidity", 80.0, false},
		{"pressure no unit", "1013.25", "pressure", 1013.25, false},

		// Error cases
		{"invalid number", "abc", "temperature", 0, true},
		{"invalid F", "abcF", "temperature", 0, true},
		{"invalid mph", "abcmph", "wind_speed", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluator.parseValueWithUnits(tt.value, tt.field)

			if tt.hasError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			// Use delta comparison for floating point
			delta := 0.001
			if result < tt.expected-delta || result > tt.expected+delta {
				t.Errorf("Value '%s' for field '%s': expected %.6f, got %.6f",
					tt.value, tt.field, tt.expected, result)
			}
		})
	}
}

// TestRealWorldScenarios tests practical alarm conditions such as heat, freeze,
// high-wind and comfort-range alerts written with Fahrenheit and mph thresholds.
func TestRealWorldScenarios(t *testing.T) {
	evaluator := NewEvaluator()

	tests := []struct {
		name      string
		condition string
		obs       *weather.Observation
		expected  bool
	}{
		{
			name:      "Heat warning (80F threshold)",
			condition: "temperature > 80F",
			obs: &weather.Observation{
				AirTemperature: 27.0, // 80.6F
			},
			expected: true,
		},
		{
			name:      "Freeze warning (32F threshold)",
			condition: "temperature < 32F",
			obs: &weather.Observation{
				AirTemperature: -1.0, // 30.2F
			},
			expected: true,
		},
		{
			name:      "High wind alert (25mph threshold)",
			condition: "wind_gust > 25mph",
			obs: &weather.Observation{
				WindGust: 12.0, // 26.8mph
			},
			expected: true,
		},
		{
			name:      "Severe weather (hot and windy)",
			condition: "temperature > 95F && wind_gust > 30mph",
			obs: &weather.Observation{
				AirTemperature: 36.0, // 96.8F
				WindGust:       14.0, // 31.3mph
			},
			expected: true,
		},
		{
			name:      "Comfortable conditions (between 65F and 75F)",
			condition: "temperature > 65F && temperature < 75F",
			obs: &weather.Observation{
				AirTemperature: 20.0, // 68F
			},
			expected: true,
		},
		{
			name:      "Mixed units in complex condition",
			condition: "temperature > 30C || wind_speed > 20mph",
			obs: &weather.Observation{
				AirTemperature: 25.0, // Below 30C
				WindAvg:        10.0, // 22.4mph (above 20mph)
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluator.Evaluate(tt.condition, tt.obs)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("Condition '%s': expected %v, got %v",
					tt.condition, tt.expected, result)
			}
		})
	}
}
