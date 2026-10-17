package alarm

import (
	"testing"

	"tempest-homekit-go/pkg/weather"
)

// approxEqual reports whether a and b differ by no more than tol, for comparing
// unit-converted floats.
func approxEqual(a, b, tol float64) bool {
	if a > b {
		return a-b <= tol
	}
	return b-a <= tol
}

// TestParseValueWithUnits_TemperatureF checks that "80F" on the temperature field is
// converted to about 26.667 degrees Celsius.
func TestParseValueWithUnits_TemperatureF(t *testing.T) {
	e := NewEvaluator()
	v, err := e.parseValueWithUnits("80F", "temperature")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 80F -> 26.666... C
	if !approxEqual(v, 26.6666667, 0.0001) {
		t.Fatalf("unexpected conversion value: %v", v)
	}
}

// TestParseValueWithUnits_WindMPH checks that "10mph" on the wind field is converted to
// 4.4704 m/s.
func TestParseValueWithUnits_WindMPH(t *testing.T) {
	e := NewEvaluator()
	v, err := e.parseValueWithUnits("10mph", "wind")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !approxEqual(v, 4.4704, 1e-4) {
		t.Fatalf("unexpected mph->m/s conversion: %v", v)
	}
}

// TestEvaluate_SimpleAndCompound checks that Evaluate returns true for a simple
// comparison, an AND of two true clauses and an OR with one true clause.
func TestEvaluate_SimpleAndCompound(t *testing.T) {
	e := NewEvaluator()
	obs := &weather.Observation{AirTemperature: 20.0, RelativeHumidity: 40}

	ok, err := e.Evaluate("temperature > 15", obs)
	if err != nil || !ok {
		t.Fatalf("expected true for temperature > 15: err=%v ok=%v", err, ok)
	}

	ok, err = e.Evaluate("temperature > 15 && humidity < 80", obs)
	if err != nil || !ok {
		t.Fatalf("expected true for compound AND: err=%v ok=%v", err, ok)
	}

	ok, err = e.Evaluate("temperature < 15 || humidity == 40", obs)
	if err != nil || !ok {
		t.Fatalf("expected true for compound OR: err=%v ok=%v", err, ok)
	}
}

// TestEvaluate_InvalidFormatAndUnknownField checks that Evaluate errors on a condition
// with no operator and on an unknown field name.
func TestEvaluate_InvalidFormatAndUnknownField(t *testing.T) {
	e := NewEvaluator()
	obs := &weather.Observation{AirTemperature: 10}

	_, err := e.Evaluate("temperature 10", obs)
	if err == nil {
		t.Fatalf("expected error for invalid format")
	}

	_, err = e.Evaluate("foobar > 1", obs)
	if err == nil {
		t.Fatalf("expected error for unknown field")
	}
}

// TestChangeDetection_IncreaseAndAnyChange checks that the first change-detection
// evaluation only records a baseline, that ">" fires on an increase, that "*" does not
// fire on an unchanged value but does on a change, and that the alarm's trigger context
// then holds the previous value.
func TestChangeDetection_IncreaseAndAnyChange(t *testing.T) {
	e := NewEvaluator()
	a := &Alarm{Enabled: true}

	// First evaluation establishes baseline and should not trigger
	obs1 := &weather.Observation{AirTemperature: 10}
	triggered, err := e.EvaluateWithAlarm(">temperature", obs1, a)
	if err != nil || triggered {
		t.Fatalf("expected no trigger on first evaluation: err=%v triggered=%v", err, triggered)
	}

	// Increase temperature -> should trigger
	obs2 := &weather.Observation{AirTemperature: 12}
	triggered, err = e.EvaluateWithAlarm(">temperature", obs2, a)
	if err != nil || !triggered {
		t.Fatalf("expected trigger on increase: err=%v triggered=%v", err, triggered)
	}

	// Test any-change operator
	// baseline set already for temperature; changing to same value should not trigger
	obs3 := &weather.Observation{AirTemperature: 12}
	triggered, err = e.EvaluateWithAlarm("*temperature", obs3, a)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if triggered {
		t.Fatalf("expected no trigger when value unchanged for * operator")
	}
	// change value
	obs4 := &weather.Observation{AirTemperature: 13}
	triggered, err = e.EvaluateWithAlarm("*temperature", obs4, a)
	if err != nil || !triggered {
		t.Fatalf("expected trigger on any change: err=%v triggered=%v", err, triggered)
	}
	// ensure trigger context contains previous value
	if v, ok := a.GetTriggerValue("temperature"); !ok || v != 12 {
		t.Fatalf("expected trigger context previous value 12, got %v ok=%v", v, ok)
	}
}

// TestParaphraseAndAvailableFields checks that Paraphrase keeps a unit marker for a
// Fahrenheit threshold and that GetAvailableFields lists "temperature".
func TestParaphraseAndAvailableFields(t *testing.T) {
	e := NewEvaluator()
	p := e.Paraphrase("temperature > 80F")
	if p == "" {
		t.Fatalf("expected paraphrase text")
	}
	// should include Fahrenheit symbol through formatValue
	if len(p) == 0 || (!containsRune(p, 'F') && !containsRune(p, '°')) {
		t.Fatalf("expected paraphrase to include unit, got: %s", p)
	}

	fields := e.GetAvailableFields()
	found := false
	for _, f := range fields {
		if f == "temperature" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected temperature in available fields")
	}
}

// containsRune reports whether rune r occurs in s (a helper for checking that a
// paraphrase kept its unit symbol).
func containsRune(s string, r rune) bool {
	for _, c := range s {
		if c == r {
			return true
		}
	}
	return false
}

// TestEvaluatorSimpleConditions checks every comparison operator and field alias against
// one fixed observation, plus errors for an unknown field, an invalid operator (">>")
// and a condition with no operator.
func TestEvaluatorSimpleConditions(t *testing.T) {
	obs := &weather.Observation{
		AirTemperature:       30.0,
		RelativeHumidity:     75.0,
		StationPressure:      1013.25,
		WindAvg:              5.5,
		WindGust:             8.0,
		WindDirection:        180,
		Illuminance:          15000,
		UV:                   7,
		RainAccumulated:      2.5,
		LightningStrikeCount: 3,
		LightningStrikeAvg:   5.2,
		PrecipitationType:    1,
	}

	evaluator := NewEvaluator()

	tests := []struct {
		name      string
		condition string
		want      bool
		wantError bool
	}{
		// Temperature tests
		{"temp greater than", "temperature > 25", true, false},
		{"temp less than", "temperature < 25", false, false},
		{"temp equals", "temperature == 30", true, false},
		{"temp not equals", "temperature != 25", true, false},
		{"temp greater or equal", "temperature >= 30", true, false},
		{"temp less or equal", "temperature <= 30", true, false},
		{"temp alias", "temp > 25", true, false},

		// Humidity tests
		{"humidity greater", "humidity > 70", true, false},
		{"humidity less", "humidity < 70", false, false},

		// Pressure tests
		{"pressure greater", "pressure > 1000", true, false},
		{"pressure less", "pressure < 1000", false, false},

		// Wind tests
		{"wind speed greater", "wind_speed > 5", true, false},
		{"wind alias", "wind > 5", true, false},
		{"wind gust greater", "wind_gust > 7", true, false},
		{"wind direction", "wind_direction == 180", true, false},

		// Light tests
		{"lux greater", "lux > 10000", true, false},
		{"light alias", "light > 10000", true, false},

		// UV tests
		{"uv greater", "uv > 5", true, false},
		{"uv index alias", "uv_index > 5", true, false},

		// Rain tests
		{"rain rate", "rain_rate > 2", true, false},
		{"rain daily", "rain_daily > 2", true, false},

		// Lightning tests
		{"lightning count", "lightning_count > 0", true, false},
		{"lightning distance", "lightning_distance < 10", true, false},

		// Precipitation type
		{"precip type", "precipitation_type == 1", true, false},

		// Invalid field
		{"invalid field", "fake_field > 100", false, true},
		{"invalid operator", "temperature >> 25", false, true},
		{"invalid syntax", "temperature", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluator.Evaluate(tt.condition, obs)
			if (err != nil) != tt.wantError {
				t.Errorf("Evaluate() error = %v, wantError %v", err, tt.wantError)
				return
			}
			if !tt.wantError && result != tt.want {
				t.Errorf("Evaluate() = %v, want %v for condition: %s", result, tt.want, tt.condition)
			}
		})
	}
}

// TestEvaluatorCompoundConditions checks the truth tables of && and || for two and three
// clauses, including that a strict ">" is false when the value equals the threshold.
func TestEvaluatorCompoundConditions(t *testing.T) {
	obs := &weather.Observation{
		AirTemperature:   30.0,
		RelativeHumidity: 85.0,
		Illuminance:      20000,
		UV:               8,
	}

	evaluator := NewEvaluator()

	tests := []struct {
		name      string
		condition string
		want      bool
	}{
		{"and both true", "temperature > 25 && humidity > 80", true},
		{"and first false", "temperature < 25 && humidity > 80", false},
		{"and second false", "temperature > 25 && humidity < 80", false},
		{"and both false", "temperature < 25 && humidity < 80", false},

		{"or both true", "temperature > 25 || humidity > 80", true},
		{"or first true", "temperature > 25 || humidity < 80", true},
		{"or second true", "temperature < 25 || humidity > 80", true},
		{"or both false", "temperature < 25 || humidity < 80", false},

		{"multiple and", "temperature > 25 && humidity > 80 && lux > 15000", true},
		{"multiple or", "temperature < 25 || humidity < 80 || lux > 15000", true},

		{"complex condition", "temperature > 25 && humidity > 80", true},
		{"heat index condition", "temperature > 30 && humidity > 60", false}, // temp is exactly 30
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluator.Evaluate(tt.condition, obs)
			if err != nil {
				t.Fatalf("Evaluate() unexpected error = %v", err)
			}
			if result != tt.want {
				t.Errorf("Evaluate() = %v, want %v for condition: %s", result, tt.want, tt.condition)
			}
		})
	}
}

// TestEvaluatorGetAvailableFields checks that GetAvailableFields returns at least ten
// names, including the main fields and their short aliases.
func TestEvaluatorGetAvailableFields(t *testing.T) {
	evaluator := NewEvaluator()
	fields := evaluator.GetAvailableFields()

	// Check that we have the expected number of fields
	if len(fields) < 10 {
		t.Errorf("GetAvailableFields() returned %d fields, expected at least 10", len(fields))
	}

	// Check for some key fields
	expectedFields := []string{
		"temperature", "temp",
		"humidity",
		"pressure",
		"wind_speed", "wind",
		"lux", "light",
		"uv", "uv_index",
		"rain_rate",
		"lightning_count",
	}

	fieldMap := make(map[string]bool)
	for _, field := range fields {
		fieldMap[field] = true
	}

	for _, expected := range expectedFields {
		if !fieldMap[expected] {
			t.Errorf("GetAvailableFields() missing expected field: %s", expected)
		}
	}
}

// TestEvaluatorEdgeCases checks zero-valued readings, a condition padded with
// whitespace, and the errors for an empty condition, a missing field, a missing value
// and a missing operator.
func TestEvaluatorEdgeCases(t *testing.T) {
	obs := &weather.Observation{
		AirTemperature: 0.0,
		UV:             0,
	}

	evaluator := NewEvaluator()

	tests := []struct {
		name      string
		condition string
		want      bool
		wantError bool
	}{
		{"zero temperature equals", "temperature == 0", true, false},
		{"zero temperature not equals", "temperature != 0", false, false},
		{"zero uv", "uv == 0", true, false},
		{"whitespace condition", "  temperature > 0  ", false, false},
		{"empty condition", "", false, true},
		{"only operator", "> 5", false, true},
		{"missing value", "temperature >", false, true},
		{"missing operator", "temperature 5", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluator.Evaluate(tt.condition, obs)
			if (err != nil) != tt.wantError {
				t.Errorf("Evaluate() error = %v, wantError %v", err, tt.wantError)
				return
			}
			if !tt.wantError && result != tt.want {
				t.Errorf("Evaluate() = %v, want %v for condition: %s", result, tt.want, tt.condition)
			}
		})
	}
}

// TestEvaluatorNegativeValues checks that negative thresholds and negative readings
// compare correctly.
func TestEvaluatorNegativeValues(t *testing.T) {
	obs := &weather.Observation{
		AirTemperature:  -10.0,
		StationPressure: 980.5,
	}

	evaluator := NewEvaluator()

	tests := []struct {
		name      string
		condition string
		want      bool
	}{
		{"negative temp greater", "temperature > -15", true},
		{"negative temp less", "temperature < -5", true},
		{"negative temp equals", "temperature == -10", true},
		{"pressure comparison", "pressure < 1000", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluator.Evaluate(tt.condition, obs)
			if err != nil {
				t.Fatalf("Evaluate() unexpected error = %v", err)
			}
			if result != tt.want {
				t.Errorf("Evaluate() = %v, want %v for condition: %s", result, tt.want, tt.condition)
			}
		})
	}
}

// TestEvaluatorFloatPrecision checks comparisons against decimal thresholds close to the
// observed value, including exact equality on 25.5.
func TestEvaluatorFloatPrecision(t *testing.T) {
	obs := &weather.Observation{
		AirTemperature: 25.5,
		WindAvg:        3.14159,
	}

	evaluator := NewEvaluator()

	tests := []struct {
		name      string
		condition string
		want      bool
	}{
		{"float comparison greater", "temperature > 25.4", true},
		{"float comparison less", "temperature < 25.6", true},
		{"float comparison equals", "temperature == 25.5", true},
		{"wind decimal", "wind_speed > 3.14", true},
		{"wind decimal precise", "wind_speed < 3.15", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluator.Evaluate(tt.condition, obs)
			if err != nil {
				t.Fatalf("Evaluate() unexpected error = %v", err)
			}
			if result != tt.want {
				t.Errorf("Evaluate() = %v, want %v for condition: %s", result, tt.want, tt.condition)
			}
		})
	}
}
