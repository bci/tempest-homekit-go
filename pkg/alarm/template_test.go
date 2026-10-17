package alarm

import (
	"strings"
	"testing"

	"tempest-homekit-go/pkg/weather"
)

// TestExpandTemplate_Coverage checks that each template variable, and templates using
// many of them, contains the expected formatted value.
func TestExpandTemplate_Coverage(t *testing.T) {
	alarm := &Alarm{
		Name:        "Test Alarm",
		Description: "Test alarm for template expansion",
	}

	obs := &weather.Observation{
		Timestamp:            1697234567,
		AirTemperature:       25.5,
		RelativeHumidity:     65.0,
		WindAvg:              5.5,
		WindGust:             8.2,
		StationPressure:      1013.25,
		Illuminance:          15000,
		UV:                   6,
		RainAccumulated:      5.2,
		LightningStrikeCount: 3,
		LightningStrikeAvg:   8,
	}

	stationName := "Test Station"

	tests := []struct {
		name     string
		template string
		contains []string
	}{
		{
			name:     "Temperature Celsius",
			template: "Temperature: {{temperature_c}}°C",
			contains: []string{"25.5"},
		},
		{
			name:     "Temperature Fahrenheit",
			template: "Temperature: {{temperature_f}}°F",
			contains: []string{"77.9"},
		},
		{
			name:     "Wind Gust",
			template: "Wind gust: {{wind_gust}} m/s",
			contains: []string{"8.2"},
		},
		{
			name:     "UV Index",
			template: "UV: {{uv}}",
			contains: []string{"6"},
		},
		{
			name:     "Lightning Count",
			template: "Lightning: {{lightning_count}} strikes",
			contains: []string{"3"},
		},
		{
			name:     "Lightning Distance",
			template: "Distance: {{lightning_distance}} miles",
			contains: []string{"8"},
		},
		{
			name:     "Rain Daily",
			template: "Rain: {{rain_daily}} mm",
			contains: []string{"5.20"},
		},
		{
			name:     "Rain Rate",
			template: "Rain rate: {{rain_rate}} mm/hr",
			contains: []string{"5.20"},
		},
		{
			name:     "Alarm Name",
			template: "Alarm: {{alarm_name}}",
			contains: []string{"Test Alarm"},
		},
		{
			name:     "Alarm Description",
			template: "Description: {{alarm_description}}",
			contains: []string{"Test alarm for template expansion"},
		},
		{
			name:     "Station Name",
			template: "Station: {{station}}",
			contains: []string{"Test Station"},
		},
		{
			name:     "Complex Template",
			template: "🌡️ {{station}}: {{temperature_c}}°C ({{temperature_f}}°F), Humidity: {{humidity}}%, Wind: {{wind_speed}} m/s (gust {{wind_gust}}), UV: {{uv}}, Lightning: {{lightning_count}} at {{lightning_distance}} miles",
			contains: []string{"Test Station", "25.5", "77.9", "65", "5.5", "8.2", "6", "3", "8"},
		},
		{
			name:     "All Variables",
			template: "{{temperature}}|{{temperature_c}}|{{temperature_f}}|{{humidity}}|{{pressure}}|{{wind_speed}}|{{wind_gust}}|{{lux}}|{{uv}}|{{rain_rate}}|{{rain_daily}}|{{lightning_count}}|{{lightning_distance}}|{{alarm_name}}|{{alarm_description}}|{{station}}|{{timestamp}}",
			contains: []string{"25.5", "77.9", "65", "1013.25", "5.5", "8.2", "15000", "6", "5.20", "3", "8", "Test Alarm", "Test alarm for template expansion", "Test Station"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := expandTemplate(tt.template, alarm, obs, stationName)
			for _, expected := range tt.contains {
				if !strings.Contains(result, expected) {
					t.Errorf("Expected result to contain '%s', got: %s", expected, result)
				}
			}
		})
	}
}

// TestExpandTemplate_MissingValues checks that an all-zero observation still renders,
// with zero values and the station name.
func TestExpandTemplate_MissingValues(t *testing.T) {
	alarm := &Alarm{
		Name: "Test",
	}

	// Observation with zero/missing values
	obs := &weather.Observation{}
	stationName := "Empty Station"

	template := "Temp: {{temperature_c}}, Humidity: {{humidity}}%, Station: {{station}}"
	result := expandTemplate(template, alarm, obs, stationName)

	// Should still render without errors
	if !strings.Contains(result, "Empty Station") {
		t.Errorf("Expected station name in result, got: %s", result)
	}
	if !strings.Contains(result, "0.0") {
		t.Errorf("Expected zero values in result, got: %s", result)
	}
}

// TestExpandTemplate_UnknownVariables checks that known variables are substituted while
// unknown {{...}} placeholders are left as written.
func TestExpandTemplate_UnknownVariables(t *testing.T) {
	alarm := &Alarm{Name: "Test"}
	obs := &weather.Observation{AirTemperature: 20.0}
	stationName := "Station"

	template := "Temp: {{temperature_c}}°C, Unknown: {{unknown_var}}, Another: {{another_unknown}}"
	result := expandTemplate(template, alarm, obs, stationName)

	// Known variables should be replaced
	if !strings.Contains(result, "20.0") {
		t.Errorf("Expected temperature value, got: %s", result)
	}

	// Unknown variables should remain unchanged
	if !strings.Contains(result, "{{unknown_var}}") {
		t.Errorf("Expected unknown variable to remain, got: %s", result)
	}
	if !strings.Contains(result, "{{another_unknown}}") {
		t.Errorf("Expected another unknown variable to remain, got: %s", result)
	}
}

// TestExpandTemplate_EmptyTemplate checks that an empty template expands to an empty
// string.
func TestExpandTemplate_EmptyTemplate(t *testing.T) {
	alarm := &Alarm{Name: "Test"}
	obs := &weather.Observation{}
	stationName := "Station"

	result := expandTemplate("", alarm, obs, stationName)
	if result != "" {
		t.Errorf("Expected empty result, got: %s", result)
	}
}

// TestExpandTemplate_NoVariables checks that a template without placeholders is returned
// unchanged.
func TestExpandTemplate_NoVariables(t *testing.T) {
	alarm := &Alarm{Name: "Test"}
	obs := &weather.Observation{}
	stationName := "Station"

	template := "This is a plain text message with no variables"
	result := expandTemplate(template, alarm, obs, stationName)

	if result != template {
		t.Errorf("Expected template unchanged, got: %s", result)
	}
}

// TestExpandTemplate_LastValues_TriggerContext checks that each {{last_*}} variable
// renders the value stored in the alarm's trigger context next to the current value.
func TestExpandTemplate_LastValues_TriggerContext(t *testing.T) {
	alarm := &Alarm{
		Name: "Test",
	}

	// Set trigger context with previous values
	alarm.triggerContext = map[string]float64{
		"temperature":     20.0,
		"humidity":        60.0,
		"pressure":        1010.0,
		"wind_speed":      3.0,
		"wind_gust":       5.0,
		"wind_direction":  180.0,
		"lux":             10000.0,
		"uv":              3.0,
		"rain_rate":       2.0,
		"rain_daily":      10.0,
		"lightning_count": 1.0,
	}

	obs := &weather.Observation{
		AirTemperature:       25.0,
		RelativeHumidity:     70.0,
		StationPressure:      1015.0,
		WindAvg:              5.0,
		WindGust:             8.0,
		WindDirection:        270.0,
		Illuminance:          15000.0,
		UV:                   5,
		RainAccumulated:      5.0,
		LightningStrikeCount: 3,
	}

	stationName := "Test Station"

	tests := []struct {
		name     string
		template string
		contains []string
	}{
		{
			name:     "Last Temperature",
			template: "Temp changed from {{last_temperature}}°C to {{temperature_c}}°C",
			contains: []string{"20.0", "25.0"},
		},
		{
			name:     "Last Humidity",
			template: "Humidity changed from {{last_humidity}}% to {{humidity}}%",
			contains: []string{"60", "70"},
		},
		{
			name:     "Last Pressure",
			template: "Pressure changed from {{last_pressure}} to {{pressure}}",
			contains: []string{"1010.00", "1015.00"},
		},
		{
			name:     "Last Wind Speed",
			template: "Wind speed changed from {{last_wind_speed}} to {{wind_speed}}",
			contains: []string{"3.0", "5.0"},
		},
		{
			name:     "Last Wind Gust",
			template: "Wind gust changed from {{last_wind_gust}} to {{wind_gust}}",
			contains: []string{"5.0", "8.0"},
		},
		{
			name:     "Last Wind Direction",
			template: "Wind direction changed from {{last_wind_direction}}° to {{wind_direction}}°",
			contains: []string{"180", "270"},
		},
		{
			name:     "Last Lux",
			template: "Illuminance changed from {{last_lux}} to {{lux}}",
			contains: []string{"10000", "15000"},
		},
		{
			name:     "Last UV",
			template: "UV changed from {{last_uv}} to {{uv}}",
			contains: []string{"3", "5"},
		},
		{
			name:     "Last Rain Rate",
			template: "Rain rate changed from {{last_rain_rate}} to {{rain_rate}}",
			contains: []string{"2.00", "5.00"},
		},
		{
			name:     "Last Rain Daily",
			template: "Rain daily changed from {{last_rain_daily}} to {{rain_daily}}",
			contains: []string{"10.00", "5.00"},
		},
		{
			name:     "Last Lightning Count",
			template: "Lightning count changed from {{last_lightning_count}} to {{lightning_count}}",
			contains: []string{"1", "3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := expandTemplate(tt.template, alarm, obs, stationName)
			for _, expected := range tt.contains {
				if !strings.Contains(result, expected) {
					t.Errorf("Expected result to contain '%s', got: %s", expected, result)
				}
			}
		})
	}
}

// TestExpandTemplate_LastValues_NoTriggerContext checks that {{last_temperature}} renders
// "N/A" when the alarm has neither a trigger context nor a previous value.
func TestExpandTemplate_LastValues_NoTriggerContext(t *testing.T) {
	alarm := &Alarm{
		Name: "Test",
	}

	// No trigger context set - should show "N/A"
	obs := &weather.Observation{
		AirTemperature: 25.0,
	}

	stationName := "Test Station"

	template := "Temp: {{temperature_c}}°C (was {{last_temperature}}°C)"
	result := expandTemplate(template, alarm, obs, stationName)

	if !strings.Contains(result, "25.0") {
		t.Errorf("Expected current temperature, got: %s", result)
	}
	if !strings.Contains(result, "N/A") {
		t.Errorf("Expected N/A for missing last_temperature, got: %s", result)
	}
}

// TestExpandTemplate_LastValues_PreviousValue checks that {{last_*}} variables fall back
// to the alarm's previous values when no trigger context is set.
func TestExpandTemplate_LastValues_PreviousValue(t *testing.T) {
	alarm := &Alarm{
		Name: "Test",
	}

	// Set previous values (but no trigger context)
	alarm.previousValue = map[string]float64{
		"temperature": 22.0,
		"humidity":    65.0,
	}

	obs := &weather.Observation{
		AirTemperature:   25.0,
		RelativeHumidity: 70.0,
	}

	stationName := "Test Station"

	template := "Temp: {{temperature_c}}°C (was {{last_temperature}}°C), Humidity: {{humidity}}% (was {{last_humidity}}%)"
	result := expandTemplate(template, alarm, obs, stationName)

	// Should use previous values as fallback
	if !strings.Contains(result, "25.0") {
		t.Errorf("Expected current temperature, got: %s", result)
	}
	if !strings.Contains(result, "22.0") {
		t.Errorf("Expected previous temperature, got: %s", result)
	}
	if !strings.Contains(result, "70") {
		t.Errorf("Expected current humidity, got: %s", result)
	}
	if !strings.Contains(result, "65") {
		t.Errorf("Expected previous humidity, got: %s", result)
	}
}
