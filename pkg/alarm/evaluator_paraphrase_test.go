package alarm

import (
	"testing"
)

// TestParaphrase checks the exact English produced by Evaluator.Paraphrase for simple
// comparisons with and without units, the change-detection prefixes (*, >, <), AND/OR
// chains, equality operators and the empty condition.
func TestParaphrase(t *testing.T) {
	evaluator := NewEvaluator()

	tests := []struct {
		name      string
		condition string
		expected  string
	}{
		{
			name:      "Simple temperature comparison (F)",
			condition: "temperature > 85F",
			expected:  "When temperature exceeds 85°F",
		},
		{
			name:      "Simple temperature comparison (C)",
			condition: "temperature > 30C",
			expected:  "When temperature exceeds 30°C",
		},
		{
			name:      "Humidity comparison",
			condition: "humidity >= 80",
			expected:  "When humidity is at least 80",
		},
		{
			name:      "Wind speed comparison",
			condition: "wind_speed > 25mph",
			expected:  "When wind speed exceeds 25 mph",
		},
		{
			name:      "Lightning distance comparison",
			condition: "lightning_distance < 5",
			expected:  "When lightning distance is below 5",
		},
		{
			name:      "Rain rate comparison",
			condition: "rain_rate > 0",
			expected:  "When rain rate exceeds 0",
		},
		{
			name:      "Change detection - any change",
			condition: "*lightning_count",
			expected:  "When lightning strike count changes (any value)",
		},
		{
			name:      "Change detection - increase",
			condition: ">rain_rate",
			expected:  "When rain rate increases",
		},
		{
			name:      "Change detection - decrease",
			condition: "<lightning_distance",
			expected:  "When lightning distance decreases",
		},
		{
			name:      "AND condition",
			condition: "temperature > 85F && humidity > 80",
			expected:  "When temperature exceeds 85°F AND humidity exceeds 80",
		},
		{
			name:      "OR condition",
			condition: "lux > 50000 || uv > 8",
			expected:  "When light level exceeds 50000 OR UV index exceeds 8",
		},
		{
			name:      "Complex AND condition",
			condition: "temperature > 35C && humidity > 80 && wind_speed < 5m/s",
			expected:  "When temperature exceeds 35°C AND humidity exceeds 80 AND wind speed is below 5 m/s",
		},
		{
			name:      "Complex OR condition",
			condition: "*lightning_count || rain_rate > 10",
			expected:  "When lightning strike count changes (any value) OR rain rate exceeds 10",
		},
		{
			name:      "Equality check",
			condition: "precipitation_type == 1",
			expected:  "When precipitation type is 1",
		},
		{
			name:      "Not equal check",
			condition: "wind_direction != 0",
			expected:  "When wind direction is not 0",
		},
		{
			name:      "Empty condition",
			condition: "",
			expected:  "No condition specified",
		},
		{
			name:      "Pressure range with AND",
			condition: "pressure >= 980 && pressure <= 1050",
			expected:  "When pressure is at least 980 AND pressure is at most 1050",
		},
		{
			name:      "Light level range with OR",
			condition: "lux < 1000 || lux > 100000",
			expected:  "When light level is below 1000 OR light level exceeds 100000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := evaluator.Paraphrase(tt.condition)
			if result != tt.expected {
				t.Errorf("Paraphrase(%q) = %q, want %q", tt.condition, result, tt.expected)
			}
		})
	}
}

// TestFormatFieldName checks that formatFieldName maps field names and their aliases to
// human-readable labels and passes unknown names through unchanged.
func TestFormatFieldName(t *testing.T) {
	evaluator := NewEvaluator()

	tests := []struct {
		field    string
		expected string
	}{
		{"temperature", "temperature"},
		{"temp", "temperature"},
		{"humidity", "humidity"},
		{"wind_speed", "wind speed"},
		{"wind", "wind speed"},
		{"wind_gust", "wind gust"},
		{"lux", "light level"},
		{"light", "light level"},
		{"uv", "UV index"},
		{"uv_index", "UV index"},
		{"rain_rate", "rain rate"},
		{"rain_daily", "daily rainfall"},
		{"lightning_count", "lightning strike count"},
		{"lightning_distance", "lightning distance"},
		{"unknown_field", "unknown_field"},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			result := evaluator.formatFieldName(tt.field)
			if result != tt.expected {
				t.Errorf("formatFieldName(%q) = %q, want %q", tt.field, result, tt.expected)
			}
		})
	}
}

// TestFormatValue checks that formatValue renders F/C suffixes (either case) as degree
// symbols, spaces out mph and m/s units, and leaves bare numbers unchanged.
func TestFormatValue(t *testing.T) {
	evaluator := NewEvaluator()

	tests := []struct {
		value    string
		expected string
	}{
		{"85F", "85°F"},
		{"85f", "85°F"},
		{"30C", "30°C"},
		{"30c", "30°C"},
		{"25mph", "25 mph"},
		{"10m/s", "10 m/s"},
		{"50", "50"},
		{"1013.25", "1013.25"},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			result := evaluator.formatValue(tt.value)
			if result != tt.expected {
				t.Errorf("formatValue(%q) = %q, want %q", tt.value, result, tt.expected)
			}
		})
	}
}
