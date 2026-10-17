package alarm

import (
	"runtime"
	"testing"
	"time"

	"tempest-homekit-go/pkg/weather"
)

// TestSyslogNotifier_Send checks that SyslogNotifier.Send with no syslog config does not
// panic; an error is expected and success is only logged.
func TestSyslogNotifier_Send(t *testing.T) {
	// Test syslog notifier (won't actually send, but covers code paths)
	notifier := &SyslogNotifier{}

	alarm := &Alarm{
		Name:        "Test Alarm",
		Description: "Test",
		Condition:   "temperature > 25",
	}

	channel := &Channel{
		Type:     "syslog",
		Template: "Test message: {{temperature_c}}°C",
	}

	obs := &weather.Observation{
		AirTemperature: 30.0,
		Timestamp:      time.Now().Unix(),
	}

	// This should not panic even without actual syslog connection
	err := notifier.Send(alarm, channel, obs, "TestStation")
	// Error is expected since we don't have syslog configured
	if err == nil {
		t.Log("Syslog send succeeded (unexpected but OK for test)")
	}
}

// TestEventLogNotifier_Send checks that EventLogNotifier.Send does not panic; an error
// (expected off Windows) is only logged.
func TestEventLogNotifier_Send(t *testing.T) {
	// Test eventlog notifier
	notifier := &EventLogNotifier{}

	alarm := &Alarm{
		Name:        "Test Alarm",
		Description: "Test",
		Condition:   "temperature > 25",
	}

	channel := &Channel{
		Type:     "eventlog",
		Template: "Test message: {{temperature_c}}°C",
	}

	obs := &weather.Observation{
		AirTemperature: 30.0,
		Timestamp:      time.Now().Unix(),
	}

	// This should not panic
	err := notifier.Send(alarm, channel, obs, "TestStation")
	// May error on non-Windows, but should not panic
	if err != nil {
		t.Logf("EventLog send error (expected on non-Windows): %v", err)
	}
}

// TestNotifierFactory_SMSNotifier checks that the factory returns an *SMSNotifier for
// "sms" when an SMS provider is configured.
func TestNotifierFactory_SMSNotifier(t *testing.T) {
	config := &AlarmConfig{
		SMS: &SMSGlobalConfig{
			Provider: "twilio",
		},
	}

	factory := NewNotifierFactory(config)
	notifier, err := factory.GetNotifier("sms")

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if notifier == nil {
		t.Error("Expected SMS notifier, got nil")
	}

	_, ok := notifier.(*SMSNotifier)
	if !ok {
		t.Errorf("Expected SMSNotifier type, got %T", notifier)
	}
}

// TestSyslogNotifier_PriorityLevels checks that each syslog priority string, including
// unknown and empty ones, can be sent without panicking; errors are ignored.
func TestSyslogNotifier_PriorityLevels(t *testing.T) {
	tests := []struct {
		name     string
		priority string
	}{
		{"error priority", "error"},
		{"warning priority", "warning"},
		{"info priority", "info"},
		{"default priority", "unknown"},
		{"empty priority", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &AlarmConfig{
				Syslog: &SyslogConfig{
					Priority: tt.priority,
					Tag:      "test",
				},
			}

			notifier := &SyslogNotifier{config: config.Syslog}
			alarm := &Alarm{Name: "Test Alarm"}
			channel := &Channel{Template: "Test message"}
			obs := &weather.Observation{}

			// Send will try to connect to local syslog, which may or may not exist
			// We're just testing that different priority values don't cause panics
			err := notifier.Send(alarm, channel, obs, "TestStation")
			// Error is expected if syslog is not available
			// We just want to ensure no panic occurs
			_ = err
		})
	}
}

// TestSyslogNotifier_NetworkConfiguration checks that a UDP syslog target that is not
// listening does not cause a panic; success is only logged.
func TestSyslogNotifier_NetworkConfiguration(t *testing.T) {
	config := &AlarmConfig{
		Syslog: &SyslogConfig{
			Network:  "udp",
			Address:  "localhost:514",
			Priority: "warning",
			Tag:      "test",
		},
	}

	notifier := &SyslogNotifier{config: config.Syslog}
	alarm := &Alarm{Name: "Test Alarm"}
	channel := &Channel{Template: "Test message"}
	obs := &weather.Observation{}

	// This will fail to connect but should not panic
	err := notifier.Send(alarm, channel, obs, "TestStation")
	// Error is expected since we're not running a syslog server
	if err == nil {
		t.Log("Syslog connection succeeded (unexpected but not an error)")
	}
}

// TestEventLogNotifier_RuntimeCheck checks that EventLogNotifier.Send succeeds on
// Windows; on other platforms it only checks that the syslog fallback does not panic.
func TestEventLogNotifier_RuntimeCheck(t *testing.T) {
	notifier := &EventLogNotifier{}
	alarm := &Alarm{Name: "Test Alarm"}
	channel := &Channel{Template: "Test message: {{alarm_name}}"}
	obs := &weather.Observation{AirTemperature: 25.0}

	err := notifier.Send(alarm, channel, obs, "TestStation")
	if runtime.GOOS == "windows" {
		// On Windows, should use event log (simplified implementation that just logs)
		if err != nil {
			t.Errorf("Expected no error on Windows, got: %v", err)
		}
	} else {
		// On Unix, falls back to syslog which may or may not be available
		// We just want to ensure no panic
		_ = err
	}
}

// TestSMSNotifier_Send_MissingConfig checks that SMSNotifier.Send errors with "SMS
// configuration missing for channel" when the channel has no SMS block.
func TestSMSNotifier_Send_MissingConfig(t *testing.T) {
	// Test with nil SMS config
	notifier := &SMSNotifier{
		config: &SMSGlobalConfig{
			Provider: "twilio",
		},
	}

	alarm := &Alarm{
		Name:      "Test Alarm",
		Condition: "temperature > 25",
	}

	channel := &Channel{
		Type: "sms",
		SMS:  nil, // Missing SMS config
	}

	obs := &weather.Observation{
		AirTemperature: 30.0,
		Timestamp:      time.Now().Unix(),
	}

	err := notifier.Send(alarm, channel, obs, "TestStation")
	if err == nil {
		t.Error("Expected error for missing SMS config")
	}
	if err.Error() != "SMS configuration missing for channel" {
		t.Errorf("Expected 'SMS configuration missing' error, got: %v", err)
	}
}

// TestSMSNotifier_Send_MissingGlobalConfig checks that SMSNotifier.Send errors with
// "global SMS configuration not set" when the notifier has no global SMS config.
func TestSMSNotifier_Send_MissingGlobalConfig(t *testing.T) {
	// Test with nil global config
	notifier := &SMSNotifier{}

	alarm := &Alarm{
		Name:      "Test Alarm",
		Condition: "temperature > 25",
	}

	channel := &Channel{
		Type: "sms",
		SMS: &SMSConfig{
			To:      []string{"+15555551234"},
			Message: "Test",
		},
	}

	obs := &weather.Observation{
		AirTemperature: 30.0,
		Timestamp:      time.Now().Unix(),
	}

	err := notifier.Send(alarm, channel, obs, "TestStation")
	if err == nil {
		t.Error("Expected error for missing global SMS config")
	}
	if err.Error() != "global SMS configuration not set" {
		t.Errorf("Expected 'global SMS configuration not set' error, got: %v", err)
	}
}

// TestEmailNotifier_Send_MissingConfig checks that EmailNotifier.Send errors with "email
// configuration missing for channel" when the channel has no email block.
func TestEmailNotifier_Send_MissingConfig(t *testing.T) {
	// Test with nil email config
	notifier := &EmailNotifier{
		config: &EmailGlobalConfig{
			Provider:    "smtp",
			FromAddress: "test@example.com",
		},
	}

	alarm := &Alarm{
		Name:      "Test Alarm",
		Condition: "temperature > 25",
	}

	channel := &Channel{
		Type:  "email",
		Email: nil, // Missing email config
	}

	obs := &weather.Observation{
		AirTemperature: 30.0,
		Timestamp:      time.Now().Unix(),
	}

	err := notifier.Send(alarm, channel, obs, "TestStation")
	if err == nil {
		t.Error("Expected error for missing email config")
	}
	if err.Error() != "email configuration missing for channel" {
		t.Errorf("Expected 'email configuration missing' error, got: %v", err)
	}
}

// TestEmailNotifier_Send_MissingGlobalConfig checks that EmailNotifier.Send errors with
// "global email configuration not set" when the notifier has no global email config.
func TestEmailNotifier_Send_MissingGlobalConfig(t *testing.T) {
	// Test with nil global config
	notifier := &EmailNotifier{}

	alarm := &Alarm{
		Name:      "Test Alarm",
		Condition: "temperature > 25",
	}

	channel := &Channel{
		Type: "email",
		Email: &EmailConfig{
			To:      []string{"test@example.com"},
			Subject: "Test",
			Body:    "Test body",
		},
	}

	obs := &weather.Observation{
		AirTemperature: 30.0,
		Timestamp:      time.Now().Unix(),
	}

	err := notifier.Send(alarm, channel, obs, "TestStation")
	if err == nil {
		t.Error("Expected error for missing global email config")
	}
	if err.Error() != "global email configuration not set" {
		t.Errorf("Expected 'global email configuration not set' error, got: %v", err)
	}
}

// TestNotifierFactory_GetNotifier_UnknownType checks that GetNotifier returns a nil
// notifier and an error for an unknown type.
func TestNotifierFactory_GetNotifier_UnknownType(t *testing.T) {
	config := &AlarmConfig{}
	factory := NewNotifierFactory(config)

	notifier, err := factory.GetNotifier("unknown-type")
	if err == nil {
		t.Error("Expected error for unknown notifier type")
	}
	if notifier != nil {
		t.Error("Expected nil notifier for unknown type")
	}
}

// TestNotifierFactory_GetNotifier_AllTypes checks that GetNotifier returns a notifier
// for each built-in type; an oslog failure is tolerated because it is macOS-only.
func TestNotifierFactory_GetNotifier_AllTypes(t *testing.T) {
	config := &AlarmConfig{}
	factory := NewNotifierFactory(config)

	types := []string{"console", "syslog", "oslog", "eventlog", "email", "sms"}
	for _, notifierType := range types {
		t.Run(notifierType, func(t *testing.T) {
			notifier, err := factory.GetNotifier(notifierType)
			if notifierType == "oslog" {
				// oslog may not be available on all platforms
				if err != nil {
					t.Logf("oslog not available (expected on non-macOS): %v", err)
				}
				return
			}
			if err != nil {
				t.Errorf("Expected no error for type %s, got: %v", notifierType, err)
			}
			if notifier == nil {
				t.Errorf("Expected notifier for type %s, got nil", notifierType)
			}
		})
	}
}

// TestEmailNotifier_TemplateFromChannel checks that an email channel with an empty Body
// falls back to the channel Template rather than failing with the missing-config error;
// the send itself is expected to fail for lack of an SMTP server.
func TestEmailNotifier_TemplateFromChannel(t *testing.T) {
	// Test that channel.Template is used when email.Body is empty
	notifier := &EmailNotifier{
		config: &EmailGlobalConfig{
			Provider:    "smtp",
			FromAddress: "test@example.com",
			FromName:    "Test Sender",
		},
	}

	alarm := &Alarm{
		Name:      "Test Alarm",
		Condition: "temperature > 25",
	}

	channel := &Channel{
		Type: "email",
		Email: &EmailConfig{
			To:      []string{"recipient@example.com"},
			Subject: "Test Subject",
			Body:    "", // Empty body - should use channel.Template
		},
		Template: "Temperature is {{temperature_c}}°C", // This should be used
	}

	obs := &weather.Observation{
		AirTemperature: 30.0,
		Timestamp:      time.Now().Unix(),
	}

	// This will fail to actually send (no SMTP configured), but we're testing
	// that the template logic works correctly
	err := notifier.Send(alarm, channel, obs, "TestStation")

	// We expect an SMTP error, not a template error
	if err != nil && err.Error() == "email configuration missing for channel" {
		t.Error("Template fallback logic may not be working correctly")
	}
}
