package alarm

import (
	"strings"
	"testing"
	"time"

	"tempest-homekit-go/pkg/weather"
)

// TestNewTemplateVariables checks the plain-text expansion of the composite variables
// {{app_info}}, {{alarm_info}}, {{sensor_info}} and {{alarm_condition}}, alone and
// combined. The expected "v1.7.0" must match appVersion.
func TestNewTemplateVariables(t *testing.T) {
	alarm := &Alarm{
		Name:        "Test Alarm",
		Description: "Test alarm description",
		Condition:   "temperature > 30",
		Cooldown:    1800,
		Enabled:     true,
		Tags:        []string{"test", "temperature"},
	}

	obs := &weather.Observation{
		AirTemperature:       25.5,
		RelativeHumidity:     65.0,
		StationPressure:      1013.2,
		WindAvg:              5.6,
		WindGust:             8.1,
		WindDirection:        245.0,
		Illuminance:          45230.0,
		UV:                   6,
		RainAccumulated:      0.0,
		RainDailyTotal:       12.7,
		LightningStrikeCount: 0,
		Timestamp:            time.Now().Unix(),
	}

	tests := []struct {
		name     string
		template string
		contains []string
	}{
		{
			name:     "app_info variable",
			template: "App Info: {{app_info}}",
			contains: []string{"Tempest HomeKit Bridge", "v1.7.0", "Uptime:", "Go"},
		},
		{
			name:     "alarm_info variable",
			template: "Alarm Info: {{alarm_info}}",
			contains: []string{"Test Alarm", "Test alarm description", "temperature > 30", "30 minutes", "test, temperature"},
		},
		{
			name:     "sensor_info variable",
			template: "Sensor Info: {{sensor_info}}",
			contains: []string{"Temperature:", "Humidity:", "Pressure:", "Wind Speed:", "UV Index:"},
		},
		{
			name:     "alarm_condition variable",
			template: "Condition: {{alarm_condition}}",
			contains: []string{"temperature > 30"},
		},
		{
			name:     "combined variables",
			template: "{{alarm_name}}: {{alarm_condition}}\n{{sensor_info}}\n{{app_info}}",
			contains: []string{"Test Alarm", "temperature > 30", "Temperature:", "Tempest HomeKit Bridge"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := expandTemplate(tt.template, alarm, obs, "Test Station")

			for _, substr := range tt.contains {
				if !strings.Contains(result, substr) {
					t.Errorf("Expected result to contain %q, but it didn't.\nResult: %s", substr, result)
				}
			}
		})
	}
}

// TestNewTemplateVariablesHTML checks that the composite variables expand to HTML
// (tables and styled divs) when the template is an HTML document.
func TestNewTemplateVariablesHTML(t *testing.T) {
	alarm := &Alarm{
		Name:        "HTML Test",
		Description: "Testing HTML formatting",
		Condition:   "temperature > 25",
		Cooldown:    3600,
		Enabled:     true,
		Tags:        []string{"html"},
	}

	obs := &weather.Observation{
		AirTemperature:   30.0,
		RelativeHumidity: 70.0,
		StationPressure:  1015.0,
		WindAvg:          10.0,
		WindGust:         15.0,
		WindDirection:    180.0,
		Illuminance:      50000.0,
		UV:               8,
		RainAccumulated:  0.5,
		RainDailyTotal:   25.4,
		Timestamp:        time.Now().Unix(),
	}

	tests := []struct {
		name     string
		template string
		contains []string
	}{
		{
			name:     "HTML app_info",
			template: "<html><body>{{app_info}}</body></html>",
			contains: []string{"<div", "style=", "Tempest HomeKit Bridge", "v1.7.0"},
		},
		{
			name:     "HTML alarm_info",
			template: "<html><body>{{alarm_info}}</body></html>",
			contains: []string{"<table", "<tr>", "<td", "HTML Test", "Testing HTML formatting"},
		},
		{
			name:     "HTML sensor_info",
			template: "<html><body>{{sensor_info}}</body></html>",
			contains: []string{"<table", "<tr>", "<td", "Temperature:", "86.0°F", "30.0°C"},
		},
		{
			name:     "Complete HTML email",
			template: `<html><body><h1>{{alarm_name}}</h1><p>{{alarm_description}}</p>{{alarm_info}}{{sensor_info}}{{app_info}}</body></html>`,
			contains: []string{"<html>", "<h1>HTML Test</h1>", "<table", "Temperature:", "Tempest HomeKit Bridge"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := expandTemplate(tt.template, alarm, obs, "HTML Station")

			for _, substr := range tt.contains {
				if !strings.Contains(result, substr) {
					t.Errorf("Expected result to contain %q, but it didn't.\nResult: %s", substr, result)
				}
			}
		})
	}
}

// TestFormatSensorInfoPlainText checks the plain-text sensor block: dual temperature
// units, pressure in mb, compass wind direction, thousands separators, rain rate and
// daily rain in inches and mm, and the lightning count.
func TestFormatSensorInfoPlainText(t *testing.T) {
	obs := &weather.Observation{
		AirTemperature:       25.5,
		RelativeHumidity:     65.0,
		StationPressure:      1013.2,
		WindAvg:              5.6,
		WindGust:             8.1,
		WindDirection:        245.0,
		Illuminance:          45230.0,
		UV:                   6,
		RainAccumulated:      2.5,
		RainDailyTotal:       25.4,
		LightningStrikeCount: 3,
		Timestamp:            time.Now().Unix(),
	}

	result := formatSensorInfo(obs, false)

	expectedParts := []string{
		"Temperature: 77.9°F (25.5°C)",
		"Humidity: 65%",
		"Pressure: 1013.20 mb",
		"Wind Speed:",
		"Wind Gust:",
		"Wind Direction: 245° (SW)",
		"UV Index: 6",
		"Illuminance: 45,230 lux",
		"Rain Rate: 2.50 mm/hr",
		"Daily Rain: 1.00 in (25.4 mm)",
		"Lightning: 3 strikes",
	}

	for _, part := range expectedParts {
		if !strings.Contains(result, part) {
			t.Errorf("Expected sensor info to contain %q\nGot: %s", part, result)
		}
	}
}

// TestFormatSensorInfoHTML checks that formatSensorInfo in HTML mode produces a table
// with the formatted temperature, humidity and pressure values.
func TestFormatSensorInfoHTML(t *testing.T) {
	obs := &weather.Observation{
		AirTemperature:   30.0,
		RelativeHumidity: 70.0,
		StationPressure:  1015.0,
		Timestamp:        time.Now().Unix(),
	}

	result := formatSensorInfo(obs, true)

	expectedParts := []string{
		"<table",
		"<tr>",
		"<td",
		"Temperature:",
		"86.0°F (30.0°C)",
		"Humidity:",
		"70%",
		"Pressure:",
		"1015.00 mb",
	}

	for _, part := range expectedParts {
		if !strings.Contains(result, part) {
			t.Errorf("Expected HTML sensor info to contain %q\nGot: %s", part, result)
		}
	}
}

// TestFormatAlarmInfo checks formatAlarmInfo in plain text (name, condition, cooldown in
// hours, comma-joined tags) and that the HTML form is a table with the name.
func TestFormatAlarmInfo(t *testing.T) {
	alarm := &Alarm{
		Name:        "High Temperature",
		Description: "Temperature exceeded threshold",
		Condition:   "temperature > 85F",
		Cooldown:    3600,
		Enabled:     true,
		Tags:        []string{"critical", "temperature"},
	}

	// Test plain text
	result := formatAlarmInfo(alarm, false)
	if !strings.Contains(result, "Alarm: High Temperature") {
		t.Errorf("Expected plain text to contain alarm name")
	}
	if !strings.Contains(result, "Condition: temperature > 85F") {
		t.Errorf("Expected plain text to contain condition")
	}
	if !strings.Contains(result, "Cooldown: 1 hours") {
		t.Errorf("Expected plain text to contain formatted cooldown")
	}
	if !strings.Contains(result, "Tags: critical, temperature") {
		t.Errorf("Expected plain text to contain tags")
	}

	// Test HTML
	resultHTML := formatAlarmInfo(alarm, true)
	if !strings.Contains(resultHTML, "<table") {
		t.Errorf("Expected HTML to contain table tag")
	}
	if !strings.Contains(resultHTML, "High Temperature") {
		t.Errorf("Expected HTML to contain alarm name")
	}
}

// TestFormatAppInfo checks that formatAppInfo includes the app name, version and uptime
// in plain text and a styled div in HTML. The expected "v1.7.0" must match appVersion.
func TestFormatAppInfo(t *testing.T) {
	// Test plain text
	result := formatAppInfo(false)
	if !strings.Contains(result, "Tempest HomeKit Bridge") {
		t.Errorf("Expected plain text to contain app name")
	}
	if !strings.Contains(result, "v1.7.0") {
		t.Errorf("Expected plain text to contain version")
	}
	if !strings.Contains(result, "Uptime:") {
		t.Errorf("Expected plain text to contain uptime")
	}

	// Test HTML
	resultHTML := formatAppInfo(true)
	if !strings.Contains(resultHTML, "<div") {
		t.Errorf("Expected HTML to contain div tag")
	}
	if !strings.Contains(resultHTML, "style=") {
		t.Errorf("Expected HTML to contain style attribute")
	}
	if !strings.Contains(resultHTML, "v1.7.0") {
		t.Errorf("Expected HTML to contain version")
	}
}

// TestFormatNumber checks that formatNumber rounds to an integer and inserts thousands
// separators.
func TestFormatNumber(t *testing.T) {
	tests := []struct {
		input    float64
		expected string
	}{
		{123.0, "123"},
		{1234.0, "1,234"},
		{12345.0, "12,345"},
		{123456.0, "123,456"},
		{1234567.0, "1,234,567"},
		{45.5, "46"}, // Rounds to nearest
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatNumber(tt.input)
			if result != tt.expected {
				t.Errorf("formatNumber(%.1f) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
