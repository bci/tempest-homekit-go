package alarm

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"

	"tempest-homekit-go/pkg/logger"
)

// captureLogOutput captures log output during test execution: it sets the logger
// level to "info" so validation warnings are emitted, redirects the standard log package
// to a buffer while f runs, and returns what was written. It restores log output to
// os.Stderr afterwards but leaves the logger level at "info".
func captureLogOutput(f func()) string {
	var buf bytes.Buffer
	// Set log level to info so validation warnings are captured
	logger.SetLogLevel("info")
	log.SetOutput(&buf)
	f()
	log.SetOutput(os.Stderr)
	return buf.String()
}

// TestValidateConfigProviders checks the warnings validateConfigProviders logs: none
// for console-only or disabled alarms or for a fully configured SMTP setup, and specific
// warnings naming the missing variables when email or SMS channels are used without a
// provider or with a partially configured SMTP or Twilio provider. Environment variables
// are restored afterwards.
func TestValidateConfigProviders(t *testing.T) {
	// Save original env vars
	origVars := map[string]string{
		"SMTP_HOST":           os.Getenv("SMTP_HOST"),
		"SMTP_USERNAME":       os.Getenv("SMTP_USERNAME"),
		"SMTP_PASSWORD":       os.Getenv("SMTP_PASSWORD"),
		"SMTP_FROM_ADDRESS":   os.Getenv("SMTP_FROM_ADDRESS"),
		"TWILIO_ACCOUNT_SID":  os.Getenv("TWILIO_ACCOUNT_SID"),
		"TWILIO_AUTH_TOKEN":   os.Getenv("TWILIO_AUTH_TOKEN"),
		"TWILIO_FROM_NUMBER":  os.Getenv("TWILIO_FROM_NUMBER"),
		"MS365_CLIENT_ID":     os.Getenv("MS365_CLIENT_ID"),
		"MS365_CLIENT_SECRET": os.Getenv("MS365_CLIENT_SECRET"),
		"MS365_TENANT_ID":     os.Getenv("MS365_TENANT_ID"),
		"MS365_FROM_ADDRESS":  os.Getenv("MS365_FROM_ADDRESS"),
	}
	defer func() {
		for k, v := range origVars {
			if v == "" {
				_ = os.Unsetenv(k)
			} else {
				_ = os.Setenv(k, v)
			}
		}
	}()

	t.Run("no delivery methods used", func(t *testing.T) {
		config := &AlarmConfig{
			Alarms: []Alarm{
				{
					Name:      "test",
					Enabled:   true,
					Condition: "temperature > 100",
					Channels: []Channel{
						{Type: "console", Template: "test"},
					},
				},
			},
		}

		output := captureLogOutput(func() {
			validateConfigProviders(config)
		})

		if strings.Contains(output, "⚠️") {
			t.Errorf("Expected no warnings for console-only alarms, got: %s", output)
		}
	})

	t.Run("email used but no provider configured", func(t *testing.T) {
		// Clear all env vars
		for k := range origVars {
			_ = os.Unsetenv(k)
		}

		config := &AlarmConfig{
			Alarms: []Alarm{
				{
					Name:      "test",
					Enabled:   true,
					Condition: "temperature > 100",
					Channels: []Channel{
						{
							Type: "email",
							Email: &EmailConfig{
								To:      []string{"test@example.com"},
								Subject: "Test",
								Body:    "Test",
							},
						},
					},
				},
			},
		}

		output := captureLogOutput(func() {
			validateConfigProviders(config)
		})

		if !strings.Contains(output, "⚠️") {
			t.Error("Expected warning for missing email provider")
		}
		if !strings.Contains(output, "no email provider is configured") {
			t.Errorf("Expected specific warning message, got: %s", output)
		}
		if !strings.Contains(output, "SMTP_HOST") || !strings.Contains(output, "MS365_CLIENT_ID") {
			t.Error("Expected suggestions for both SMTP and MS365")
		}
	})

	t.Run("SMTP email configured but missing variables", func(t *testing.T) {
		// Clear all env vars
		for k := range origVars {
			_ = os.Unsetenv(k)
		}

		// Set only SMTP_HOST
		_ = os.Setenv("SMTP_HOST", "smtp.example.com")

		config, _ := LoadAlarmConfig(`{
			"alarms": [{
				"name": "test",
				"enabled": true,
				"condition": "temperature > 100",
				"channels": [{
					"type": "email",
					"email": {
						"to": ["test@example.com"],
						"subject": "Test",
						"body": "Test"
					}
				}]
			}]
		}`)

		output := captureLogOutput(func() {
			validateConfigProviders(config)
		})

		if !strings.Contains(output, "⚠️") {
			t.Error("Expected warning for missing SMTP variables")
		}
		if !strings.Contains(output, "SMTP_USERNAME") {
			t.Error("Expected warning about missing SMTP_USERNAME")
		}
		if !strings.Contains(output, "SMTP_PASSWORD") {
			t.Error("Expected warning about missing SMTP_PASSWORD")
		}
		if !strings.Contains(output, "SMTP_FROM_ADDRESS") {
			t.Error("Expected warning about missing SMTP_FROM_ADDRESS")
		}
	})

	t.Run("SMS used but no provider configured", func(t *testing.T) {
		// Clear all env vars
		for k := range origVars {
			_ = os.Unsetenv(k)
		}

		config := &AlarmConfig{
			Alarms: []Alarm{
				{
					Name:      "test",
					Enabled:   true,
					Condition: "temperature > 100",
					Channels: []Channel{
						{
							Type: "sms",
							SMS: &SMSConfig{
								To:      []string{"+15555551234"},
								Message: "Test",
							},
						},
					},
				},
			},
		}

		output := captureLogOutput(func() {
			validateConfigProviders(config)
		})

		if !strings.Contains(output, "⚠️") {
			t.Error("Expected warning for missing SMS provider")
		}
		if !strings.Contains(output, "no SMS provider is configured") {
			t.Errorf("Expected specific warning message, got: %s", output)
		}
		if !strings.Contains(output, "TWILIO_ACCOUNT_SID") || !strings.Contains(output, "AWS_ACCESS_KEY_ID") {
			t.Error("Expected suggestions for both Twilio and AWS SNS")
		}
	})

	t.Run("Twilio SMS configured but missing variables", func(t *testing.T) {
		// Clear all env vars
		for k := range origVars {
			_ = os.Unsetenv(k)
		}

		// Set only TWILIO_ACCOUNT_SID
		_ = os.Setenv("TWILIO_ACCOUNT_SID", "AC123456")

		config, _ := LoadAlarmConfig(`{
			"alarms": [{
				"name": "test",
				"enabled": true,
				"condition": "temperature > 100",
				"channels": [{
					"type": "sms",
					"sms": {
						"to": ["+15555551234"],
						"message": "Test"
					}
				}]
			}]
		}`)

		output := captureLogOutput(func() {
			validateConfigProviders(config)
		})

		if !strings.Contains(output, "⚠️") {
			t.Error("Expected warning for missing Twilio variables")
		}
		if !strings.Contains(output, "TWILIO_AUTH_TOKEN") {
			t.Error("Expected warning about missing TWILIO_AUTH_TOKEN")
		}
		if !strings.Contains(output, "TWILIO_FROM_NUMBER") {
			t.Error("Expected warning about missing TWILIO_FROM_NUMBER")
		}
	})

	t.Run("disabled alarms are ignored", func(t *testing.T) {
		// Clear all env vars
		for k := range origVars {
			_ = os.Unsetenv(k)
		}

		config := &AlarmConfig{
			Alarms: []Alarm{
				{
					Name:      "test",
					Enabled:   false, // Disabled!
					Condition: "temperature > 100",
					Channels: []Channel{
						{
							Type: "email",
							Email: &EmailConfig{
								To:      []string{"test@example.com"},
								Subject: "Test",
								Body:    "Test",
							},
						},
					},
				},
			},
		}

		output := captureLogOutput(func() {
			validateConfigProviders(config)
		})

		if strings.Contains(output, "⚠️") {
			t.Errorf("Expected no warnings for disabled alarms, got: %s", output)
		}
	})

	t.Run("fully configured SMTP email - no warnings", func(t *testing.T) {
		// Clear all env vars
		for k := range origVars {
			_ = os.Unsetenv(k)
		}

		// Set all SMTP vars
		_ = os.Setenv("SMTP_HOST", "smtp.example.com")
		_ = os.Setenv("SMTP_USERNAME", "user@example.com")
		_ = os.Setenv("SMTP_PASSWORD", "password")
		_ = os.Setenv("SMTP_FROM_ADDRESS", "alerts@example.com")

		config, _ := LoadAlarmConfig(`{
			"alarms": [{
				"name": "test",
				"enabled": true,
				"condition": "temperature > 100",
				"channels": [{
					"type": "email",
					"email": {
						"to": ["test@example.com"],
						"subject": "Test",
						"body": "Test"
					}
				}]
			}]
		}`)

		output := captureLogOutput(func() {
			validateConfigProviders(config)
		})

		if strings.Contains(output, "⚠️") {
			t.Errorf("Expected no warnings for fully configured SMTP, got: %s", output)
		}
	})
}
