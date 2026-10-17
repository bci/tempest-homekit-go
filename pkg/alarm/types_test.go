package alarm

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

// TestAlarmConfigValidateIndividualAlarms checks that AlarmConfig.Validate rejects an
// alarm without a name, a condition or at least one channel.
func TestAlarmConfigValidateIndividualAlarms(t *testing.T) {
	tests := []struct {
		name      string
		config    AlarmConfig
		wantError bool
	}{
		{
			name: "valid alarm",
			config: AlarmConfig{
				Alarms: []Alarm{{
					Name:      "test-alarm",
					Condition: "temperature > 85",
					Enabled:   true,
					Channels:  []Channel{{Type: "console", Template: "Alert: {{condition}}"}},
				}},
			},
			wantError: false,
		},
		{
			name: "missing name",
			config: AlarmConfig{
				Alarms: []Alarm{{
					Condition: "temperature > 85",
					Channels:  []Channel{{Type: "console", Template: "Alert"}},
				}},
			},
			wantError: true,
		},
		{
			name: "missing condition",
			config: AlarmConfig{
				Alarms: []Alarm{{
					Name:     "test",
					Channels: []Channel{{Type: "console", Template: "Alert"}},
				}},
			},
			wantError: true,
		},
		{
			name: "missing channels",
			config: AlarmConfig{
				Alarms: []Alarm{{
					Name:      "test",
					Condition: "temperature > 85",
				}},
			},
			wantError: true,
		},
		{
			name: "empty channels",
			config: AlarmConfig{
				Alarms: []Alarm{{
					Name:      "test",
					Condition: "temperature > 85",
					Channels:  []Channel{},
				}},
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantError {
				t.Errorf("Validate() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

// TestAlarmCanFire checks CanFire for a never-fired alarm, cooldowns that have and have
// not expired, a zero cooldown and a disabled alarm.
func TestAlarmCanFire(t *testing.T) {
	tests := []struct {
		name  string
		setup func() *Alarm
		want  bool
	}{
		{
			name: "never fired before",
			setup: func() *Alarm {
				return &Alarm{
					Name:     "test",
					Enabled:  true,
					Cooldown: 1800,
				}
			},
			want: true,
		},
		{
			name: "cooldown not expired",
			setup: func() *Alarm {
				a := &Alarm{
					Name:     "test",
					Enabled:  true,
					Cooldown: 1800,
				}
				a.MarkFired()
				return a
			},
			want: false,
		},
		{
			name: "cooldown expired",
			setup: func() *Alarm {
				a := &Alarm{
					Name:     "test",
					Enabled:  true,
					Cooldown: 1, // 1 second cooldown
				}
				a.MarkFired()
				time.Sleep(2 * time.Second) // Wait for cooldown to expire
				return a
			},
			want: true,
		},
		{
			name: "zero cooldown",
			setup: func() *Alarm {
				a := &Alarm{
					Name:     "test",
					Enabled:  true,
					Cooldown: 0,
				}
				a.MarkFired()
				return a
			},
			want: true,
		},
		{
			name: "alarm disabled",
			setup: func() *Alarm {
				return &Alarm{
					Name:     "test",
					Enabled:  false,
					Cooldown: 0,
				}
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			alarm := tt.setup()
			got := alarm.CanFire()
			if got != tt.want {
				t.Errorf("CanFire() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestChannelValidate checks Channel.Validate for every channel type: the required
// template or per-type block, CSV/JSON paths (with a default message filled in when
// empty), webhook URL and body, and empty or unknown types.
func TestChannelValidate(t *testing.T) {
	tests := []struct {
		name      string
		channel   Channel
		wantError bool
	}{
		{
			name:      "valid console channel",
			channel:   Channel{Type: "console", Template: "Alert: {{condition}}"},
			wantError: false,
		},
		{
			name:      "valid syslog channel",
			channel:   Channel{Type: "syslog", Template: "Tempest-Alarm: {{condition}}"},
			wantError: false,
		},
		{
			name: "valid email channel",
			channel: Channel{
				Type: "email",
				Email: &EmailConfig{
					To:      []string{"test@example.com"},
					Subject: "Weather Alert",
					Body:    "Alert: {{condition}}",
				},
			},
			wantError: false,
		},
		{
			name: "valid sms channel",
			channel: Channel{
				Type: "sms",
				SMS: &SMSConfig{
					To:      []string{"+1234567890"},
					Message: "Alert: {{condition}}",
				},
			},
			wantError: false,
		},
		{
			name: "valid webhook channel",
			channel: Channel{
				Type: "webhook",
				Webhook: &WebhookConfig{
					URL:  "https://example.com/webhook",
					Body: `{"alert": "{{alarm_name}}", "message": "{{condition}}"}`,
				},
			},
			wantError: false,
		},
		{
			name: "valid csv channel",
			channel: Channel{
				Type: "csv",
				CSV: &CSVConfig{
					Path:    "/tmp/test.csv",
					MaxDays: 30,
					Message: "{{timestamp}},{{alarm_name}},{{temperature}}",
				},
			},
			wantError: false,
		},
		{
			name: "valid json channel",
			channel: Channel{
				Type: "json",
				JSON: &JSONConfig{
					Path:    "/tmp/test.json",
					MaxDays: 30,
					Message: `{"timestamp": "{{timestamp}}", "alarm": "{{alarm_name}}"}`,
				},
			},
			wantError: false,
		},
		{
			name: "csv channel with empty message gets default",
			channel: Channel{
				Type: "csv",
				CSV: &CSVConfig{
					Path:    "/tmp/test.csv",
					MaxDays: 30,
					Message: "",
				},
			},
			wantError: false,
		},
		{
			name: "json channel with empty message gets default",
			channel: Channel{
				Type: "json",
				JSON: &JSONConfig{
					Path:    "/tmp/test.json",
					MaxDays: 30,
					Message: "",
				},
			},
			wantError: false,
		},
		{
			name:      "console without template",
			channel:   Channel{Type: "console"},
			wantError: true,
		},
		{
			name:      "email without config",
			channel:   Channel{Type: "email"},
			wantError: true,
		},
		{
			name:      "sms without config",
			channel:   Channel{Type: "sms"},
			wantError: true,
		},
		{
			name:      "csv without config",
			channel:   Channel{Type: "csv"},
			wantError: true,
		},
		{
			name:      "json without config",
			channel:   Channel{Type: "json"},
			wantError: true,
		},
		{
			name: "csv without path",
			channel: Channel{
				Type: "csv",
				CSV: &CSVConfig{
					MaxDays: 30,
					Message: "test",
				},
			},
			wantError: true,
		},
		{
			name: "json without path",
			channel: Channel{
				Type: "json",
				JSON: &JSONConfig{
					MaxDays: 30,
					Message: "test",
				},
			},
			wantError: true,
		},
		{
			name: "webhook without url",
			channel: Channel{
				Type: "webhook",
				Webhook: &WebhookConfig{
					Body: "test body",
				},
			},
			wantError: true,
		},
		{
			name: "webhook without body",
			channel: Channel{
				Type: "webhook",
				Webhook: &WebhookConfig{
					URL: "https://example.com",
				},
			},
			wantError: true,
		},
		{
			name:      "empty type",
			channel:   Channel{Type: ""},
			wantError: true,
		},
		{
			name:      "invalid type",
			channel:   Channel{Type: "invalid"},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.channel.Validate()
			if (err != nil) != tt.wantError {
				t.Errorf("Validate() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

// TestAlarmConfigValidate checks AlarmConfig.Validate for a valid config, duplicate
// alarm names, an invalid alarm, and an empty config, which is allowed so the manager can
// wait for the file to be filled in.
func TestAlarmConfigValidate(t *testing.T) {
	tests := []struct {
		name      string
		config    AlarmConfig
		wantError bool
	}{
		{
			name: "valid config",
			config: AlarmConfig{
				Alarms: []Alarm{
					{
						Name:      "alarm1",
						Condition: "temperature > 85",
						Channels:  []Channel{{Type: "console", Template: "Alert 1"}},
					},
					{
						Name:      "alarm2",
						Condition: "humidity > 80",
						Channels:  []Channel{{Type: "console", Template: "Alert 2"}},
					},
				},
			},
			wantError: false,
		},
		{
			name: "duplicate alarm names",
			config: AlarmConfig{
				Alarms: []Alarm{
					{
						Name:      "alarm1",
						Condition: "temperature > 85",
						Channels:  []Channel{{Type: "console", Template: "Alert"}},
					},
					{
						Name:      "alarm1",
						Condition: "humidity > 80",
						Channels:  []Channel{{Type: "console", Template: "Alert"}},
					},
				},
			},
			wantError: true,
		},
		{
			name: "invalid alarm",
			config: AlarmConfig{
				Alarms: []Alarm{
					{
						Name:      "", // missing name
						Condition: "temperature > 85",
						Channels:  []Channel{{Type: "console", Template: "Alert"}},
					},
				},
			},
			wantError: true,
		},
		{
			name: "empty config",
			config: AlarmConfig{
				Alarms: []Alarm{},
			},
			wantError: false, // Empty config is allowed - manager can watch for file changes
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantError {
				t.Errorf("Validate() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

// TestLoadAlarmConfig checks LoadAlarmConfig with inline JSON, an "@" file reference,
// malformed JSON and a missing file.
func TestLoadAlarmConfig(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		setup     func() (string, func())
		wantError bool
		wantCount int
	}{
		{
			name:  "inline JSON",
			input: `{"alarms":[{"name":"test","condition":"temperature > 85","channels":[{"type":"console","template":"Alert"}]}]}`,
			setup: func() (string, func()) {
				return "", func() {}
			},
			wantError: false,
			wantCount: 1,
		},
		{
			name:  "file reference",
			input: "@test.json",
			setup: func() (string, func()) {
				tmpfile, err := os.CreateTemp("", "test*.json")
				if err != nil {
					t.Fatalf("failed to create temp file: %v", err)
				}
				config := AlarmConfig{
					Alarms: []Alarm{
						{
							Name:      "test",
							Condition: "temperature > 85",
							Channels:  []Channel{{Type: "console", Template: "Alert"}},
						},
					},
				}
				data, err := json.Marshal(config)
				if err != nil {
					_ = tmpfile.Close()
					t.Fatalf("failed to marshal config: %v", err)
				}
				if _, err := tmpfile.Write(data); err != nil {
					_ = tmpfile.Close()
					t.Fatalf("failed to write temp file: %v", err)
				}
				if err := tmpfile.Close(); err != nil {
					t.Fatalf("failed to close temp file: %v", err)
				}
				return "@" + tmpfile.Name(), func() { _ = os.Remove(tmpfile.Name()) }
			},
			wantError: false,
			wantCount: 1,
		},
		{
			name:  "invalid JSON",
			input: `{invalid json}`,
			setup: func() (string, func()) {
				return "", func() {}
			},
			wantError: true,
		},
		{
			name:  "missing file",
			input: "@/nonexistent/file.json",
			setup: func() (string, func()) {
				return "", func() {}
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, cleanup := tt.setup()
			if input != "" {
				tt.input = input
			}
			defer cleanup()

			config, err := LoadAlarmConfig(tt.input)
			if (err != nil) != tt.wantError {
				t.Errorf("LoadAlarmConfig() error = %v, wantError %v", err, tt.wantError)
				return
			}

			if !tt.wantError && len(config.Alarms) != tt.wantCount {
				t.Errorf("LoadAlarmConfig() got %d alarms, want %d", len(config.Alarms), tt.wantCount)
			}
		})
	}
}

// TestAlarmJSON checks that an Alarm with tags and console and email channels round-trips
// through JSON.
func TestAlarmJSON(t *testing.T) {
	alarm := Alarm{
		Name:        "test-alarm",
		Description: "Test description",
		Condition:   "temperature > 85",
		Tags:        []string{"temperature", "heat"},
		Cooldown:    1800,
		Enabled:     true,
		Channels: []Channel{
			{Type: "console", Template: "Alert"},
			{Type: "email", Email: &EmailConfig{To: []string{"test@example.com"}, Subject: "Test", Body: "Body"}},
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(alarm)
	if err != nil {
		t.Fatalf("Failed to marshal alarm: %v", err)
	}

	// Unmarshal back
	var decoded Alarm
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal alarm: %v", err)
	}

	// Verify fields
	if decoded.Name != alarm.Name {
		t.Errorf("Name = %v, want %v", decoded.Name, alarm.Name)
	}
	if decoded.Condition != alarm.Condition {
		t.Errorf("Condition = %v, want %v", decoded.Condition, alarm.Condition)
	}
	if len(decoded.Tags) != len(alarm.Tags) {
		t.Errorf("Tags length = %v, want %v", len(decoded.Tags), len(alarm.Tags))
	}
	if decoded.Cooldown != alarm.Cooldown {
		t.Errorf("Cooldown = %v, want %v", decoded.Cooldown, alarm.Cooldown)
	}
	if decoded.Enabled != alarm.Enabled {
		t.Errorf("Enabled = %v, want %v", decoded.Enabled, alarm.Enabled)
	}
	if len(decoded.Channels) != len(alarm.Channels) {
		t.Errorf("Channels length = %v, want %v", len(decoded.Channels), len(alarm.Channels))
	}
}
