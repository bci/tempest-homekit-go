package alarm

import (
	"fmt"
	"log"
	"os"
	"strings"

	"tempest-homekit-go/pkg/weather"
)

// TestSMSConfiguration tests the SMS configuration by sending a test SMS.
//
// It loads alarmsJSON with [LoadAlarmConfig] to obtain the SMS provider
// settings from the environment, prints them with secrets masked, and sends a
// test message to TEST_SMS_RECIPIENT (which must be in E.164 form, starting
// with "+") through the real [NotifierFactory] path; stationName appears in
// the message. Progress is also logged with an [SMS-TEST] prefix. It returns
// an error if the config fails to load, no or an unsupported provider is
// configured, required credentials are missing, the recipient is missing or
// malformed, or sending fails.
func TestSMSConfiguration(alarmsJSON, stationName string) error {
	log.Printf("[SMS-TEST] Starting SMS configuration test for station: %s", stationName)

	// Load alarm config to get provider configuration
	config, err := LoadAlarmConfig(alarmsJSON)
	if err != nil {
		log.Printf("[SMS-TEST] ERROR: Failed to load alarm configuration: %v", err)
		return fmt.Errorf("failed to load alarm configuration: %w", err)
	}

	log.Printf("[SMS-TEST] Successfully loaded alarm configuration with %d alarms", len(config.Alarms))

	// Check if SMS is configured
	if config.SMS == nil {
		log.Printf("[SMS-TEST] ERROR: No SMS configuration found")
		return fmt.Errorf("no SMS configuration found - set TWILIO_* or AWS_* environment variables in .env")
	}

	provider := config.SMS.Provider
	log.Printf("[SMS-TEST] SMS provider configured: %s", provider)

	fmt.Println()
	fmt.Println("================================================================")
	fmt.Println("SMS CONFIGURATION TEST")
	fmt.Println("================================================================")
	fmt.Println()
	fmt.Printf("Provider: %s\n", provider)
	fmt.Println()

	// Display provider-specific configuration
	switch provider {
	case "twilio":
		fmt.Println("Twilio Configuration:")
		fmt.Printf("  Account SID: %s\n", maskString(config.SMS.AccountSID))
		fmt.Printf("  Auth Token: %s\n", maskString(config.SMS.AuthToken))
		fmt.Printf("  From Number: %s\n", config.SMS.FromNumber)
		fmt.Println()
		if config.SMS.AccountSID == "" || config.SMS.AuthToken == "" || config.SMS.FromNumber == "" {
			log.Printf("[SMS-TEST] ERROR: Missing Twilio credentials")
			fmt.Println("⚠️  WARNING: Missing Twilio credentials")
			fmt.Println("   Required environment variables:")
			fmt.Println("   - TWILIO_ACCOUNT_SID")
			fmt.Println("   - TWILIO_AUTH_TOKEN")
			fmt.Println("   - TWILIO_FROM_NUMBER")
			return fmt.Errorf("incomplete Twilio configuration")
		}
	case "aws_sns", "sns", "aws":
		fmt.Println("AWS SNS Configuration:")
		fmt.Printf("  Access Key ID: %s\n", maskString(config.SMS.AWSAccessKey))
		fmt.Printf("  Secret Key: %s\n", maskString(config.SMS.AWSSecretKey))
		fmt.Printf("  Region: %s\n", config.SMS.AWSRegion)
		if config.SMS.AWSSNSTopicARN != "" {
			fmt.Printf("  Topic ARN: %s\n", config.SMS.AWSSNSTopicARN)
		}
		fmt.Println()
		if config.SMS.AWSAccessKey == "" || config.SMS.AWSSecretKey == "" || config.SMS.AWSRegion == "" {
			log.Printf("[SMS-TEST] ERROR: Missing AWS SNS credentials")
			fmt.Println("⚠️  WARNING: Missing AWS SNS credentials")
			fmt.Println("   Required environment variables:")
			fmt.Println("   - AWS_ACCESS_KEY_ID")
			fmt.Println("   - AWS_SECRET_ACCESS_KEY")
			fmt.Println("   - AWS_REGION")
			return fmt.Errorf("incomplete AWS SNS configuration")
		}
	default:
		log.Printf("[SMS-TEST] ERROR: Unsupported SMS provider: %s", provider)
		return fmt.Errorf("unsupported SMS provider: %s", provider)
	}

	// Recipient phone comes from command line parameter
	recipientNumber := os.Getenv("TEST_SMS_RECIPIENT")
	if recipientNumber == "" {
		log.Printf("[SMS-TEST] ERROR: No recipient phone number provided")
		return fmt.Errorf("no recipient phone number provided")
	}

	log.Printf("[SMS-TEST] Test SMS recipient: %s", recipientNumber)

	// Validate E.164 format
	if !strings.HasPrefix(recipientNumber, "+") {
		log.Printf("[SMS-TEST] ERROR: Phone number must start with + (E.164 format required)")
		return fmt.Errorf("phone number must start with + (E.164 format required)")
	}
	fmt.Println("================================================================")

	fmt.Println()
	fmt.Println("Sending test SMS...")
	log.Printf("[SMS-TEST] Attempting to send test SMS to %s", recipientNumber)

	// Create SMS notifier using factory (same path as real alarms)
	factory := NewNotifierFactory(config)
	smsNotifier, err := factory.GetNotifier("sms")
	if err != nil {
		log.Printf("[SMS-TEST] ERROR: Failed to create SMS notifier: %v", err)
		return fmt.Errorf("failed to create SMS notifier: %w", err)
	}

	log.Printf("[SMS-TEST] Successfully created SMS notifier")

	testAlarm := &Alarm{
		Name:        "Test SMS",
		Description: "Test SMS to verify SMS settings",
		Enabled:     true,
	}

	testChannel := &Channel{
		Type: "sms",
		SMS: &SMSConfig{
			To:      []string{recipientNumber},
			Message: fmt.Sprintf("Test SMS from %s\n\nProvider: %s\nTime: {{timestamp}}\nStation: %s\n\nIf you received this, your SMS configuration is working correctly!", appVersion, provider, stationName),
		},
	}

	// Create test observation
	testObs := &weather.Observation{
		Timestamp:        int64(0), // Will use current time in template
		AirTemperature:   20.0,
		RelativeHumidity: 50.0,
		WindAvg:          5.0,
		StationPressure:  1013.25,
	}

	log.Printf("[SMS-TEST] Created test alarm and observation, sending SMS...")

	// Send test SMS
	err = smsNotifier.Send(testAlarm, testChannel, testObs, stationName)
	if err != nil {
		log.Printf("[SMS-TEST] ERROR: Failed to send test SMS: %v", err)
		return fmt.Errorf("failed to send test SMS: %w", err)
	}

	log.Printf("[SMS-TEST] SUCCESS: Test SMS sent successfully to %s", recipientNumber)

	fmt.Println()
	fmt.Println("✅ Test SMS sent successfully!")
	fmt.Println()
	fmt.Printf("Check the phone %s for the test message\n", recipientNumber)
	fmt.Println()

	switch provider {
	case "twilio":
		fmt.Println("If you don't see the SMS:")
		fmt.Println("  1. Check if your Twilio account is in trial mode")
		fmt.Println("  2. Verify the recipient number is verified (trial accounts only)")
		fmt.Println("  3. Check Twilio console for delivery status")
		fmt.Println("  4. Ensure your Twilio number has SMS capabilities")
		fmt.Println("  5. Check for sufficient Twilio account balance")
	case "aws_sns", "sns", "aws":
		fmt.Println("If you don't see the SMS:")
		fmt.Println("  1. Check if your AWS account is in SNS sandbox mode")
		fmt.Println("  2. Verify the recipient number is verified (sandbox accounts only)")
		fmt.Println("  3. Check AWS SNS console for delivery status")
		fmt.Println("  4. Ensure your region supports SMS (not all regions do)")
		fmt.Println("  5. Check AWS spending limits and account status")
		fmt.Println("  6. Run ./scripts/setup-aws-sns.sh to configure production settings")
	}

	fmt.Println()
	fmt.Println("================================================================")

	log.Printf("[SMS-TEST] SMS configuration test completed successfully")

	return nil
}

// RunSMSTest is a convenience function that wraps TestSMSConfiguration and exits.
// It never returns: it calls os.Exit(0) on success and log.Fatalf on failure.
func RunSMSTest(alarmsJSON, stationName string) {
	log.Printf("[SMS-TEST] Starting isolated SMS test mode")
	log.Printf("[SMS-TEST] This test will send a test SMS and exit - no alarm processing will occur")

	err := TestSMSConfiguration(alarmsJSON, stationName)
	if err != nil {
		log.Printf("[SMS-TEST] SMS test failed: %v", err)
		log.Fatalf("SMS test failed: %v", err)
	}

	log.Printf("[SMS-TEST] SMS test completed successfully - exiting")
	os.Exit(0)
}

// maskString masks a string for display: it shows the first 4 and last 4
// characters of strings longer than 8, "***" plus the last 2 characters of
// strings of 3 to 8 characters, just "***" for 1 or 2 characters, and
// "(not set)" for an empty string.
func maskString(s string) string {
	if s == "" {
		return "(not set)"
	}
	if len(s) <= 2 {
		// Too short to show any of it without revealing all of it.
		return "***"
	}
	if len(s) <= 8 {
		return "***" + s[len(s)-2:]
	}
	return s[:4] + "****" + s[len(s)-4:]
}
