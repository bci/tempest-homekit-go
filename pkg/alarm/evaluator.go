package alarm

import (
	"fmt"
	"strconv"
	"strings"

	"tempest-homekit-go/pkg/logger"
	"tempest-homekit-go/pkg/weather"
)

// Evaluator evaluates alarm conditions against weather observations. It is
// stateless; per-alarm state for change detection lives on [Alarm], so one
// Evaluator can be shared by all alarms.
type Evaluator struct{}

// NewEvaluator creates a new alarm evaluator.
func NewEvaluator() *Evaluator {
	return &Evaluator{}
}

// Evaluate checks if an alarm condition is met given weather data.
// It passes no alarm, so change-detection operators (*field, >field, <field)
// return an error; use [Evaluator.EvaluateWithAlarm] for those.
func (e *Evaluator) Evaluate(condition string, obs *weather.Observation) (bool, error) {
	return e.EvaluateWithAlarm(condition, obs, nil)
}

// EvaluateWithAlarm checks if an alarm condition is met given weather data and alarm state.
//
// Supported syntax:
//   - Comparisons "field op value" with op one of >, <, >=, <=, ==, !=. The
//     value may carry a unit suffix (80F, 25mph, 80%); see parseValueWithUnits.
//   - Change detection "*field" (any change), ">field" (increase) and
//     "<field" (decrease) relative to the alarm's previous value; these need
//     a non-nil alarm.
//   - Compound conditions joined with && or ||. If the condition contains
//     "&&" it is split on "&&" only, otherwise on "||"; there is no
//     precedence, grouping or mixing of the two.
//
// Examples:
//
//	"temperature > 85"
//	"humidity > 80 && temperature > 35"
//	"lux > 10000 && lux < 50000"
//	"*lightning_count"     (triggers on any change)
//	">rain_rate"           (triggers when rain increases)
//	"<lightning_distance"  (triggers when lightning gets closer)
//
// It returns false and an error for an empty operand around && or ||, an
// unknown field, an unparsable value, or a change-detection operator without
// an alarm. Change-detection terms update alarm's previous values as a side
// effect. obs must be non-nil.
func (e *Evaluator) EvaluateWithAlarm(condition string, obs *weather.Observation, alarm *Alarm) (bool, error) {
	// Parse and evaluate the condition
	// Supports: >, <, >=, <=, ==, !=, &&, ||, *field, >field, <field
	// Examples:
	//   "temperature > 85"
	//   "humidity > 80 && temperature > 35"
	//   "lux > 10000 && lux < 50000"
	//   "rain_rate > 0"
	//   "*lightning_count" (triggers on any change)
	//   ">rain_rate" (triggers when rain increases)
	//   "<lightning_distance" (triggers when lightning gets closer)

	condition = strings.TrimSpace(condition)

	// Debug log the evaluation attempt
	logger.Debug("Evaluating condition: %s (temp=%.1f, humidity=%.0f, pressure=%.2f)",
		condition, obs.AirTemperature, obs.RelativeHumidity, obs.StationPressure)

	// Handle compound conditions with && and ||
	if strings.Contains(condition, "&&") {
		parts := strings.Split(condition, "&&")
		for i, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				return false, fmt.Errorf("AND operator (&&) requires expressions on both sides (missing expression at position %d)", i+1)
			}
			result, err := e.evaluateSimpleWithAlarm(part, obs, alarm)
			if err != nil {
				logger.Debug("Evaluation error for part '%s': %v", part, err)
				return false, err
			}
			if !result {
				logger.Debug("AND condition failed on part: %s", part)
				return false, nil
			}
		}
		logger.Debug("AND condition passed: %s", condition)
		return true, nil
	}

	if strings.Contains(condition, "||") {
		parts := strings.Split(condition, "||")
		for i, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				return false, fmt.Errorf("OR operator (||) requires expressions on both sides (missing expression at position %d)", i+1)
			}
			result, err := e.evaluateSimpleWithAlarm(part, obs, alarm)
			if err != nil {
				logger.Debug("Evaluation error for part '%s': %v", part, err)
				return false, err
			}
			if result {
				logger.Debug("OR condition passed on part: %s", part)
				return true, nil
			}
		}
		logger.Debug("OR condition failed: %s", condition)
		return false, nil
	}

	// Simple condition
	return e.evaluateSimpleWithAlarm(condition, obs, alarm)
}

// evaluateSimpleWithAlarm evaluates a simple comparison with optional alarm state.
//
// A condition whose first character is *, > or < is routed to
// evaluateChangeDetection (and fails if alarm is nil). Otherwise the first
// operator found, trying >=, <=, !=, ==, >, < in that order, splits the
// condition into field and value. It returns an error when no operator is
// found after a non-empty field, the field is unknown, or the value does not
// parse.
func (e *Evaluator) evaluateSimpleWithAlarm(condition string, obs *weather.Observation, alarm *Alarm) (bool, error) {
	// Check for unary change-detection operators first
	if len(condition) > 0 {
		firstChar := condition[0]
		if firstChar == '*' || firstChar == '>' || firstChar == '<' {
			// This is a unary operator for change detection
			if alarm == nil {
				return false, fmt.Errorf("change-detection operator %c requires alarm context", firstChar)
			}
			return e.evaluateChangeDetection(condition, obs, alarm)
		}
	}

	// Parse the condition: "field operator value"
	operators := []string{">=", "<=", "!=", "==", ">", "<"}

	var field, operator, valueStr string
	for _, op := range operators {
		if idx := strings.Index(condition, op); idx > 0 {
			field = strings.TrimSpace(condition[:idx])
			operator = op
			valueStr = strings.TrimSpace(condition[idx+len(op):])
			break
		}
	}

	if operator == "" {
		return false, fmt.Errorf("invalid condition format: %s (expected 'field operator value')", condition)
	}

	// Get the field value from observation
	fieldValue, err := e.getFieldValue(field, obs)
	if err != nil {
		return false, err
	}

	// Parse the comparison value with unit conversion support
	compareValue, err := e.parseValueWithUnits(valueStr, field)
	if err != nil {
		return false, fmt.Errorf("invalid comparison value %s: %w", valueStr, err)
	}

	// Perform comparison
	return e.compare(fieldValue, operator, compareValue), nil
}

// getFieldValue extracts a field value from the weather observation.
//
// The field name is lower-cased and spaces become underscores, and several
// aliases are accepted (temp, wind, light, uv_index, rain_accumulated,
// rain_accumulation). Note rain_daily currently reads RainAccumulated, not
// RainDailyTotal. It returns an error for an unknown field.
func (e *Evaluator) getFieldValue(field string, obs *weather.Observation) (float64, error) {
	field = strings.ToLower(strings.ReplaceAll(field, " ", "_"))

	switch field {
	case "temperature", "temp":
		return obs.AirTemperature, nil
	case "humidity":
		return float64(obs.RelativeHumidity), nil
	case "pressure":
		return obs.StationPressure, nil
	case "wind_speed", "wind":
		return obs.WindAvg, nil
	case "wind_gust":
		return obs.WindGust, nil
	case "wind_direction":
		return float64(obs.WindDirection), nil
	case "lux", "light":
		return obs.Illuminance, nil
	case "uv", "uv_index":
		return float64(obs.UV), nil
	case "rain_rate", "rain_accumulated":
		return obs.RainAccumulated, nil
	case "rain_daily", "rain_accumulation":
		return obs.RainAccumulated, nil
	case "lightning_count":
		return float64(obs.LightningStrikeCount), nil
	case "lightning_distance":
		return obs.LightningStrikeAvg, nil
	case "precipitation_type":
		return float64(obs.PrecipitationType), nil
	default:
		return 0, fmt.Errorf("unknown field: %s", field)
	}
}

// parseValueWithUnits parses a value string with optional unit suffix and converts to base units
// Supports:
//   - Temperature: 80F or 80f -> Celsius, 32C or 32c -> Celsius (no conversion)
//   - Wind: 25mph -> m/s, 10m/s or 10 -> m/s (no conversion)
//   - Humidity: 80% -> 80 (no conversion, just strip %)
//
// Unit suffixes are only recognised for the fields named above (wind units
// apply to wind_speed, wind and wind_gust); for any other field the value must
// be a plain number. The error is the strconv.ParseFloat error.
func (e *Evaluator) parseValueWithUnits(valueStr string, field string) (float64, error) {
	valueStr = strings.TrimSpace(valueStr)
	field = strings.ToLower(field)

	// Check for temperature fields (stored in Celsius)
	if field == "temperature" || field == "temp" {
		// Check for Fahrenheit suffix
		if strings.HasSuffix(strings.ToLower(valueStr), "f") {
			valueStr = strings.TrimSuffix(strings.TrimSuffix(valueStr, "f"), "F")
			fahrenheit, err := strconv.ParseFloat(strings.TrimSpace(valueStr), 64)
			if err != nil {
				return 0, err
			}
			// Convert Fahrenheit to Celsius: (F - 32) * 5/9
			celsius := (fahrenheit - 32.0) * 5.0 / 9.0
			return celsius, nil
		}
		// Check for explicit Celsius suffix (optional, already in Celsius)
		if strings.HasSuffix(strings.ToLower(valueStr), "c") {
			valueStr = strings.TrimSuffix(strings.TrimSuffix(valueStr, "c"), "C")
		}
	}

	// Check for wind speed fields (stored in m/s)
	if field == "wind_speed" || field == "wind" || field == "wind_gust" {
		// Check for mph suffix
		if strings.HasSuffix(strings.ToLower(valueStr), "mph") {
			valueStr = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(valueStr, "mph"), "MPH"), "Mph")
			mph, err := strconv.ParseFloat(strings.TrimSpace(valueStr), 64)
			if err != nil {
				return 0, err
			}
			// Convert mph to m/s: mph * 0.44704
			ms := mph * 0.44704
			return ms, nil
		}
		// Check for explicit m/s suffix (optional, already in m/s)
		if strings.HasSuffix(strings.ToLower(valueStr), "m/s") {
			valueStr = strings.TrimSuffix(valueStr, "m/s")
			valueStr = strings.TrimSuffix(valueStr, "M/S")
		} else if strings.HasSuffix(strings.ToLower(valueStr), "ms") {
			valueStr = strings.TrimSuffix(strings.TrimSuffix(valueStr, "ms"), "MS")
		}
	}

	// Check for humidity fields (stored as percentage, strip % if present)
	if field == "humidity" {
		valueStr = strings.TrimSuffix(valueStr, "%")
	}

	// Parse as plain number
	return strconv.ParseFloat(strings.TrimSpace(valueStr), 64)
}

// evaluateChangeDetection evaluates unary change-detection operators
// Supports:
//
//	*field - triggers on ANY change in field value
//	>field - triggers when field value increases from previous
//	<field - triggers when field value decreases from previous
//
// The first evaluation for a field only records a baseline and returns false.
// On a trigger, the previous value is saved with [Alarm.SetTriggerContext] so
// notifications can show what was compared. The current value is then stored
// as the new previous value. Errors are returned for a condition shorter than
// two characters or an unknown field.
func (e *Evaluator) evaluateChangeDetection(condition string, obs *weather.Observation, alarm *Alarm) (bool, error) {
	if len(condition) < 2 {
		return false, fmt.Errorf("invalid change-detection condition: %s", condition)
	}

	operator := condition[0]
	fieldName := strings.TrimSpace(condition[1:])

	// Get current field value
	currentValue, err := e.getFieldValue(fieldName, obs)
	if err != nil {
		return false, err
	}

	// Get previous value
	previousValue, hasPrevious := alarm.GetPreviousValue(fieldName)

	// If no previous value exists, don't trigger (need baseline)
	if !hasPrevious {
		logger.Debug("No previous value for %s, establishing baseline: %.2f", fieldName, currentValue)
		// Store current value for next evaluation
		alarm.SetPreviousValue(fieldName, currentValue)
		return false, nil
	}

	// Evaluate based on operator
	var triggered bool
	switch operator {
	case '*':
		// Any change
		changed := currentValue != previousValue
		if changed {
			logger.Debug("Change detected in %s: %.2f -> %.2f", fieldName, previousValue, currentValue)
		}
		triggered = changed

	case '>':
		// Increase from previous
		increased := currentValue > previousValue
		if increased {
			logger.Debug("Increase detected in %s: %.2f -> %.2f", fieldName, previousValue, currentValue)
		}
		triggered = increased

	case '<':
		// Decrease from previous
		decreased := currentValue < previousValue
		if decreased {
			logger.Debug("Decrease detected in %s: %.2f -> %.2f", fieldName, previousValue, currentValue)
		}
		triggered = decreased

	default:
		return false, fmt.Errorf("unknown change-detection operator: %c", operator)
	}

	// If triggered, store the previous value in trigger context for notification display
	if triggered {
		alarm.SetTriggerContext(map[string]float64{fieldName: previousValue})
	}

	// Store current value for next evaluation (only after comparison is done)
	alarm.SetPreviousValue(fieldName, currentValue)

	return triggered, nil
}

// compare performs the actual comparison a operator b and returns its
// result; an unrecognised operator yields false.
func (e *Evaluator) compare(a float64, operator string, b float64) bool {
	var result bool
	switch operator {
	case ">":
		result = a > b
	case "<":
		result = a < b
	case ">=":
		result = a >= b
	case "<=":
		result = a <= b
	case "==":
		result = a == b
	case "!=":
		result = a != b
	default:
		result = false
	}

	logger.Debug("Comparison: %.2f %s %.2f = %v", a, operator, b, result)
	return result
}

// GetAvailableFields returns a list of supported field names for conditions,
// including aliases. It omits the rain_accumulated and rain_accumulation
// aliases that getFieldValue also accepts.
func (e *Evaluator) GetAvailableFields() []string {
	return []string{
		"temperature", "temp",
		"humidity",
		"pressure",
		"wind_speed", "wind",
		"wind_gust",
		"wind_direction",
		"lux", "light",
		"uv", "uv_index",
		"rain_rate",
		"rain_daily",
		"lightning_count",
		"lightning_distance",
		"precipitation_type",
	}
}

// Paraphrase converts a condition into human-readable text, for example
// "temperature > 85F" becomes "When temperature exceeds 85°F". It splits on &&
// or || the same way [Evaluator.EvaluateWithAlarm] does, never fails, and
// returns "No condition specified" for an empty condition.
func (e *Evaluator) Paraphrase(condition string) string {
	condition = strings.TrimSpace(condition)
	if condition == "" {
		return "No condition specified"
	}

	// Handle compound conditions
	if strings.Contains(condition, "&&") {
		parts := strings.Split(condition, "&&")
		paraphrases := []string{}
		for _, part := range parts {
			paraphrases = append(paraphrases, e.paraphraseSimple(strings.TrimSpace(part)))
		}
		return "When " + strings.Join(paraphrases, " AND ")
	}

	if strings.Contains(condition, "||") {
		parts := strings.Split(condition, "||")
		paraphrases := []string{}
		for _, part := range parts {
			paraphrases = append(paraphrases, e.paraphraseSimple(strings.TrimSpace(part)))
		}
		return "When " + strings.Join(paraphrases, " OR ")
	}

	return "When " + e.paraphraseSimple(condition)
}

// paraphraseSimple converts a simple condition into human-readable text.
// Change-detection forms become "<field> changes (any value)", "increases" or
// "decreases"; comparisons use phrases such as "exceeds" and "is at least".
// Anything it cannot recognise is returned unchanged.
func (e *Evaluator) paraphraseSimple(condition string) string {
	// Handle change detection operators
	if len(condition) > 0 {
		firstChar := condition[0]
		if firstChar == '*' {
			fieldName := strings.TrimSpace(condition[1:])
			return e.formatFieldName(fieldName) + " changes (any value)"
		} else if firstChar == '>' {
			fieldName := strings.TrimSpace(condition[1:])
			return e.formatFieldName(fieldName) + " increases"
		} else if firstChar == '<' && !strings.Contains(condition, "<=") {
			// Check if this is unary < (not binary comparison)
			fieldName := strings.TrimSpace(condition[1:])
			if !strings.ContainsAny(fieldName, "0123456789") {
				return e.formatFieldName(fieldName) + " decreases"
			}
		}
	}

	// Handle binary comparisons
	for _, op := range []string{">=", "<=", "!=", "==", ">", "<"} {
		if strings.Contains(condition, op) {
			parts := strings.SplitN(condition, op, 2)
			if len(parts) == 2 {
				fieldName := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				opText := map[string]string{
					">=": "is at least",
					"<=": "is at most",
					"!=": "is not",
					"==": "is",
					">":  "exceeds",
					"<":  "is below",
				}[op]
				return e.formatFieldName(fieldName) + " " + opText + " " + e.formatValue(value)
			}
		}
	}

	return condition
}

// formatFieldName converts a field name into human-readable text (for example
// wind_speed becomes "wind speed"); unknown names are returned lower-cased.
func (e *Evaluator) formatFieldName(field string) string {
	field = strings.ToLower(strings.TrimSpace(field))
	fieldNames := map[string]string{
		"temperature":        "temperature",
		"temp":               "temperature",
		"humidity":           "humidity",
		"pressure":           "pressure",
		"wind_speed":         "wind speed",
		"wind":               "wind speed",
		"wind_gust":          "wind gust",
		"wind_direction":     "wind direction",
		"lux":                "light level",
		"light":              "light level",
		"uv":                 "UV index",
		"uv_index":           "UV index",
		"rain_rate":          "rain rate",
		"rain_daily":         "daily rainfall",
		"lightning_count":    "lightning strike count",
		"lightning_distance": "lightning distance",
		"precipitation_type": "precipitation type",
	}
	if name, ok := fieldNames[field]; ok {
		return name
	}
	return field
}

// formatValue formats a value with its unit if applicable: an F or C suffix
// (either case) becomes °F or °C, and mph or m/s gain a separating space.
// Other values, including percentages, are returned trimmed but unchanged.
func (e *Evaluator) formatValue(value string) string {
	value = strings.TrimSpace(value)
	// Check for temperature units
	if strings.HasSuffix(strings.ToUpper(value), "F") {
		return value[:len(value)-1] + "°F"
	}
	if strings.HasSuffix(strings.ToUpper(value), "C") {
		return value[:len(value)-1] + "°C"
	}
	// Check for speed units
	if strings.HasSuffix(value, "mph") {
		return value[:len(value)-3] + " mph"
	}
	if strings.HasSuffix(value, "m/s") {
		return value[:len(value)-3] + " m/s"
	}
	// Check for humidity percentage
	if strings.HasSuffix(value, "%") {
		return value
	}
	return value
}
