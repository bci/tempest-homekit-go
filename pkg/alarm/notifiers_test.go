package alarm

import (
	"strings"
	"testing"

	"tempest-homekit-go/pkg/weather"
)

// TestExpandTemplate checks that expandTemplate substitutes each observation, alarm and
// station variable, combines several in one template and leaves unknown variables
// untouched.
func TestExpandTemplate(t *testing.T) {
	alarm := &Alarm{
		Name:        "test-alarm",
		Description: "Test alarm description",
		Condition:   "temperature > 85",
		Tags:        []string{"temperature", "heat"},
	}

	obs := &weather.Observation{
		AirTemperature:       30.5,
		RelativeHumidity:     75.0,
		StationPressure:      1013.25,
		WindAvg:              5.5,
		WindGust:             8.5,
		WindDirection:        180,
		Illuminance:          45000,
		UV:                   8,
		RainAccumulated:      2.5,
		LightningStrikeCount: 5,
		LightningStrikeAvg:   10.5,
		Timestamp:            1728526800, // 2024-10-09
	}

	stationName := "Test Station"

	tests := []struct {
		name     string
		template string
		want     []string
	}{
		{
			name:     "alarm name",
			template: "Alarm: {{alarm_name}}",
			want:     []string{"Alarm: test-alarm"},
		},
		{
			name:     "station name",
			template: "Station: {{station}}",
			want:     []string{"Station: Test Station"},
		},
		{
			name:     "temperature",
			template: "Temperature: {{temperature}}°C",
			want:     []string{"Temperature: 30.5°C"},
		},
		{
			name:     "temperature Fahrenheit",
			template: "Temperature: {{temperature_f}}°F",
			want:     []string{"Temperature: 86."},
		},
		{
			name:     "humidity",
			template: "Humidity: {{humidity}}%",
			want:     []string{"Humidity: 75%"},
		},
		{
			name:     "pressure",
			template: "Pressure: {{pressure}} hPa",
			want:     []string{"Pressure: 1013.25 hPa"},
		},
		{
			name:     "wind speed",
			template: "Wind: {{wind_speed}} m/s",
			want:     []string{"Wind: 5.5 m/s"},
		},
		{
			name:     "wind gust",
			template: "Gust: {{wind_gust}} m/s",
			want:     []string{"Gust: 8.5 m/s"},
		},
		{
			name:     "wind direction",
			template: "Direction: {{wind_direction}}°",
			want:     []string{"Direction: 180°"},
		},
		{
			name:     "lux",
			template: "Light: {{lux}} lux",
			want:     []string{"Light: 45000 lux"},
		},
		{
			name:     "uv",
			template: "UV: {{uv}}",
			want:     []string{"UV: 8"},
		},
		{
			name:     "rain rate",
			template: "Rain: {{rain_rate}} mm",
			want:     []string{"Rain: 2.50 mm"},
		},
		{
			name:     "lightning count",
			template: "Lightning: {{lightning_count}} strikes",
			want:     []string{"Lightning: 5 strikes"},
		},
		{
			name:     "lightning distance",
			template: "Distance: {{lightning_distance}} km",
			want:     []string{"Distance: 10.5 km"},
		},
		{
			name:     "timestamp",
			template: "Time: {{timestamp}}",
			want:     []string{"Time: ", "2024-"},
		},
		{
			name:     "multiple variables",
			template: "{{station}}: {{alarm_name}} (Temp: {{temperature}}°C, Humidity: {{humidity}}%)",
			want:     []string{"Test Station", "test-alarm", "30.5°C", "75%"},
		},
		{
			name:     "unknown variable",
			template: "{{unknown}}",
			want:     []string{"{{unknown}}"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := expandTemplate(tt.template, alarm, obs, stationName)

			for _, want := range tt.want {
				if !strings.Contains(result, want) {
					t.Errorf("expandTemplate() = %q, should contain %q", result, want)
				}
			}
		})
	}
}

// TestConsoleNotifier checks that ConsoleNotifier.Send returns no error.
func TestConsoleNotifier(t *testing.T) {
	notifier := &ConsoleNotifier{}

	alarm := &Alarm{
		Name:      "test-alarm",
		Condition: "temperature > 85",
	}

	channel := &Channel{
		Type:     "console",
		Template: "Alert: {{alarm_name}}",
	}

	obs := &weather.Observation{
		AirTemperature: 90.0,
	}

	// Should not panic
	err := notifier.Send(alarm, channel, obs, "Test Station")
	if err != nil {
		t.Errorf("ConsoleNotifier.Send() error = %v", err)
	}
}

// TestNotifierFactory checks that GetNotifier returns a notifier for each supported
// channel type, webhook included, and an error for an invalid one.
func TestNotifierFactory(t *testing.T) {
	config := &AlarmConfig{}
	factory := NewNotifierFactory(config)

	tests := []struct {
		name    string
		channel string
		want    bool
	}{
		{
			name:    "console notifier",
			channel: "console",
			want:    true,
		},
		{
			name:    "syslog notifier",
			channel: "syslog",
			want:    true,
		},
		{
			name:    "eventlog notifier",
			channel: "eventlog",
			want:    true,
		},
		{
			name:    "email notifier",
			channel: "email",
			want:    true,
		},
		{
			name:    "sms notifier",
			channel: "sms",
			want:    true,
		},
		{
			name:    "webhook notifier",
			channel: "webhook",
			want:    true,
		},
		{
			name:    "invalid notifier",
			channel: "invalid",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notifier, err := factory.GetNotifier(tt.channel)
			gotNotifier := notifier != nil && err == nil
			if gotNotifier != tt.want {
				t.Errorf("GetNotifier(%q) returned notifier=%v error=%v, want notifier=%v", tt.channel, notifier != nil, err, tt.want)
			}
		})
	}
}

// TestExpandTemplateWithEmptyValues checks that zero readings are rendered ("0.0", "0")
// rather than blanked.
func TestExpandTemplateWithEmptyValues(t *testing.T) {
	alarm := &Alarm{Name: "test"}
	stationName := "Test"

	tests := []struct {
		name     string
		obs      *weather.Observation
		template string
		want     string
	}{
		{
			name:     "zero temperature",
			obs:      &weather.Observation{AirTemperature: 0.0},
			template: "Temp: {{temperature}}",
			want:     "Temp: 0.0",
		},
		{
			name:     "zero UV",
			obs:      &weather.Observation{UV: 0},
			template: "UV: {{uv}}",
			want:     "UV: 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := expandTemplate(tt.template, alarm, tt.obs, stationName)
			if result != tt.want {
				t.Errorf("expandTemplate() = %q, want %q", result, tt.want)
			}
		})
	}
}

// TestExpandTemplateEdgeCases checks empty and variable-free templates, that an
// unterminated "{{" is left alone, and that extra outer braces survive substitution.
func TestExpandTemplateEdgeCases(t *testing.T) {
	alarm := &Alarm{Name: "test"}
	obs := &weather.Observation{AirTemperature: 25.0}
	stationName := "Test"

	tests := []struct {
		name     string
		template string
		want     string
	}{
		{
			name:     "empty template",
			template: "",
			want:     "",
		},
		{
			name:     "template without variables",
			template: "Plain text message",
			want:     "Plain text message",
		},
		{
			name:     "incomplete variable",
			template: "Temp: {{temperature",
			want:     "Temp: {{temperature",
		},
		{
			name:     "nested braces",
			template: "Value: {{{temperature}}}",
			want:     "Value: {25.0}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := expandTemplate(tt.template, alarm, obs, stationName)
			if result != tt.want {
				t.Errorf("expandTemplate() = %q, want %q", result, tt.want)
			}
		})
	}
}
