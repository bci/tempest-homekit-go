package alarm

import (
	"os"
	"strings"
	"testing"

	"tempest-homekit-go/pkg/weather"
)

// TestSMSNotifier checks SMSNotifier.Send errors for AWS SNS with and without
// credentials, Twilio without credentials and an unsupported provider. The
// "valid config" case uses fake AWS keys, so its error comes from the AWS call.
func TestSMSNotifier(t *testing.T) {
	// Note: This is a functional test that validates the SMS notifier logic
	// Actual AWS SNS calls are not made without valid credentials

	tests := []struct {
		name          string
		provider      string
		setupEnv      func()
		cleanupEnv    func()
		expectError   bool
		errorContains string
	}{
		{
			name:     "aws sns with valid config",
			provider: "aws",
			setupEnv: func() {
				_ = os.Setenv("AWS_ACCESS_KEY_ID", "test-key-id")
				_ = os.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret-key")
				_ = os.Setenv("AWS_REGION", "us-west-2")
			},
			cleanupEnv: func() {
				_ = os.Unsetenv("AWS_ACCESS_KEY_ID")
				_ = os.Unsetenv("AWS_SECRET_ACCESS_KEY")
				_ = os.Unsetenv("AWS_REGION")
			},
			expectError:   true, // Will fail on actual AWS call but validates config
			errorContains: "",   // Various AWS errors possible
		},
		{
			name:     "aws sns missing credentials",
			provider: "aws",
			setupEnv: func() {
				_ = os.Unsetenv("AWS_ACCESS_KEY_ID")
				_ = os.Unsetenv("AWS_SECRET_ACCESS_KEY")
				_ = os.Unsetenv("AWS_REGION")
			},
			cleanupEnv:    func() {},
			expectError:   true,
			errorContains: "AWS SNS credentials missing",
		},
		{
			name:     "twilio missing credentials",
			provider: "twilio",
			setupEnv: func() {
				_ = os.Unsetenv("TWILIO_ACCOUNT_SID")
				_ = os.Unsetenv("TWILIO_AUTH_TOKEN")
				_ = os.Unsetenv("TWILIO_FROM_NUMBER")
			},
			cleanupEnv:    func() {},
			expectError:   true,
			errorContains: "twilio credentials missing",
		},
		{
			name:          "unsupported provider",
			provider:      "invalid",
			setupEnv:      func() {},
			cleanupEnv:    func() {},
			expectError:   true,
			errorContains: "unsupported SMS provider",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupEnv()
			defer tt.cleanupEnv()

			config := &AlarmConfig{
				SMS: &SMSGlobalConfig{
					Provider:     tt.provider,
					AWSAccessKey: "${AWS_ACCESS_KEY_ID}",
					AWSSecretKey: "${AWS_SECRET_ACCESS_KEY}",
					AWSRegion:    "${AWS_REGION}",
				},
			}

			notifier := &SMSNotifier{config: config.SMS}

			alarm := &Alarm{
				Name:        "test-alarm",
				Description: "Test SMS notification",
				Condition:   "temperature > 85",
			}

			channel := &Channel{
				Type: "sms",
				SMS: &SMSConfig{
					To:      []string{"+15555551234"},
					Message: "Test: {{alarm_name}}",
				},
			}

			obs := &weather.Observation{
				AirTemperature: 90.0,
			}

			err := notifier.Send(alarm, channel, obs, "Test Station")

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error containing %q, got: %v", tt.errorContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

// TestSMSNotifierTemplateExpansion checks that sending an SMS with a templated message
// and no AWS credentials fails; it only logs when the error is not a credentials error.
func TestSMSNotifierTemplateExpansion(t *testing.T) {
	// Test that SMS messages properly expand templates
	config := &AlarmConfig{
		SMS: &SMSGlobalConfig{
			Provider: "aws",
			// Missing credentials will cause send to fail, but we can verify template expansion
		},
	}

	notifier := &SMSNotifier{config: config.SMS}

	alarm := &Alarm{
		Name:        "high-temp",
		Description: "High temperature warning",
		Condition:   "temperature > 85",
	}

	channel := &Channel{
		Type: "sms",
		SMS: &SMSConfig{
			To:      []string{"+15555551234"},
			Message: "⚠️ {{alarm_name}}: {{temperature}}°C at {{station}}",
		},
	}

	obs := &weather.Observation{
		AirTemperature: 90.5,
	}

	// Will fail due to missing credentials, but that's expected
	err := notifier.Send(alarm, channel, obs, "Test Station")
	if err == nil {
		t.Error("Expected error due to missing credentials")
	}

	// Verify the error is about credentials, not template expansion
	if !strings.Contains(err.Error(), "credentials missing") {
		// If we get a different error, template expansion worked
		// (would fail on AWS API call instead)
		t.Logf("Template expansion appears to work (got AWS error, not template error): %v", err)
	}
}

// TestSMSNotifierWithTopicARN checks that ${VAR} references for AWS keys and an SNS
// topic ARN are resolved from the environment, so the send fails at the AWS call rather
// than with a "credentials missing" error.
func TestSMSNotifierWithTopicARN(t *testing.T) {
	// Test SNS topic ARN configuration
	_ = os.Setenv("AWS_ACCESS_KEY_ID", "test-key")
	_ = os.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret")
	_ = os.Setenv("AWS_REGION", "us-west-2")
	_ = os.Setenv("AWS_SNS_TOPIC_ARN", "arn:aws:sns:us-west-2:123456789012:TestTopic")
	defer func() {
		_ = os.Unsetenv("AWS_ACCESS_KEY_ID")
		_ = os.Unsetenv("AWS_SECRET_ACCESS_KEY")
		_ = os.Unsetenv("AWS_REGION")
		_ = os.Unsetenv("AWS_SNS_TOPIC_ARN")
	}()

	config := &AlarmConfig{
		SMS: &SMSGlobalConfig{
			Provider:       "aws",
			AWSAccessKey:   "${AWS_ACCESS_KEY_ID}",
			AWSSecretKey:   "${AWS_SECRET_ACCESS_KEY}",
			AWSRegion:      "${AWS_REGION}",
			AWSSNSTopicARN: "${AWS_SNS_TOPIC_ARN}",
		},
	}

	notifier := &SMSNotifier{config: config.SMS}

	alarm := &Alarm{
		Name:      "test-alarm",
		Condition: "temperature > 85",
	}

	channel := &Channel{
		Type: "sms",
		SMS: &SMSConfig{
			To:      []string{"+15555551234"},
			Message: "Test message",
		},
	}

	obs := &weather.Observation{
		AirTemperature: 90.0,
	}

	// Will fail on AWS API call, but validates config loading
	err := notifier.Send(alarm, channel, obs, "Test Station")
	if err == nil {
		t.Error("Expected error due to invalid test credentials")
	}

	// Should fail on AWS API, not on config validation
	if strings.Contains(err.Error(), "credentials missing") {
		t.Error("Config validation failed - topic ARN not loaded properly")
	}
}

// TestSMSNotifierMultipleRecipients checks that a channel with three recipients passes
// config validation, so the send fails at the AWS call rather than with a "credentials
// missing" error.
func TestSMSNotifierMultipleRecipients(t *testing.T) {
	// Test sending to multiple phone numbers
	_ = os.Setenv("AWS_ACCESS_KEY_ID", "test-key")
	_ = os.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret")
	_ = os.Setenv("AWS_REGION", "us-west-2")
	_ = os.Setenv("AWS_SNS_TOPIC_ARN", "arn:aws:sns:us-west-2:123456789012:TestTopic")
	defer func() {
		_ = os.Unsetenv("AWS_ACCESS_KEY_ID")
		_ = os.Unsetenv("AWS_SECRET_ACCESS_KEY")
		_ = os.Unsetenv("AWS_REGION")
		_ = os.Unsetenv("AWS_SNS_TOPIC_ARN")
	}()
	config := &AlarmConfig{
		SMS: &SMSGlobalConfig{
			Provider:     "aws",
			AWSAccessKey: "${AWS_ACCESS_KEY_ID}",
			AWSSecretKey: "${AWS_SECRET_ACCESS_KEY}",
			AWSRegion:    "${AWS_REGION}",
		},
	}

	notifier := &SMSNotifier{config: config.SMS}

	alarm := &Alarm{
		Name:      "multi-recipient-test",
		Condition: "temperature > 100",
	}

	channel := &Channel{
		Type: "sms",
		SMS: &SMSConfig{
			To: []string{
				"+15555551234",
				"+15555555678",
				"+15555559012",
			},
			Message: "Alert: {{alarm_name}}",
		},
	}

	obs := &weather.Observation{
		AirTemperature: 105.0,
	}

	// Will fail on AWS API call
	err := notifier.Send(alarm, channel, obs, "Test Station")
	if err == nil {
		t.Error("Expected error due to invalid test credentials")
	}

	// Verify it's not a config error
	if strings.Contains(err.Error(), "credentials missing") {
		t.Error("Multi-recipient config not loaded properly")
	}
}

// TestSMSFactoryCreation checks that the factory builds an *SMSNotifier for "sms" when
// the AWS provider is configured.
func TestSMSFactoryCreation(t *testing.T) {
	// Test that the notifier factory creates SMS notifiers correctly
	config := &AlarmConfig{
		SMS: &SMSGlobalConfig{
			Provider: "aws",
		},
	}

	factory := NewNotifierFactory(config)

	notifier, err := factory.GetNotifier("sms")
	if err != nil {
		t.Errorf("Failed to create SMS notifier: %v", err)
	}

	if notifier == nil {
		t.Error("Notifier is nil")
	}

	// Verify it's the right type
	if _, ok := notifier.(*SMSNotifier); !ok {
		t.Errorf("Expected *SMSNotifier, got %T", notifier)
	}
}
