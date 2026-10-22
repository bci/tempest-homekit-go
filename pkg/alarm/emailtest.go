package alarm

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"tempest-homekit-go/pkg/weather"
)

// appVersion is the application version shown in test messages and in
// {{app_info}}. It is maintained by hand and must start with "v".
const appVersion = "v1.7.0"

// TestEmailConfiguration tests the email configuration by sending a test email.
//
// It reads the provider settings directly from the environment, preferring
// Microsoft 365 when MS365_CLIENT_ID, MS365_CLIENT_SECRET and MS365_TENANT_ID
// are all set and SMTP when SMTP_HOST is set, prints them with secrets masked,
// and sends a test email to TEST_EMAIL_RECIPIENT through the real
// [NotifierFactory] path. alarmsJSON is the alarm config input accepted by
// [LoadAlarmConfig]; stationName fills {{station}}. It returns an error if no
// provider or required variable is set, the recipient is missing or has no
// "@" and ".", the config fails to load, or sending fails.
func TestEmailConfiguration(alarmsJSON, stationName string) error {
	fmt.Println("Reading email configuration from environment variables...")
	fmt.Println()

	// Read email configuration from environment variables
	// Check for Microsoft 365 configuration FIRST (prioritize over SMTP)
	clientID := os.Getenv("MS365_CLIENT_ID")
	clientSecret := os.Getenv("MS365_CLIENT_SECRET")
	tenantID := os.Getenv("MS365_TENANT_ID")

	var provider string
	switch {
	case clientID != "" && clientSecret != "" && tenantID != "":
		provider = "microsoft365"
	case os.Getenv("SMTP_HOST") != "":
		provider = "smtp"
	default:
		return fmt.Errorf("no email configuration found. Set either:\n  - MS365_CLIENT_ID, MS365_CLIENT_SECRET, MS365_TENANT_ID for Microsoft 365\n  - SMTP_HOST, SMTP_PORT, SMTP_USERNAME, SMTP_PASSWORD for SMTP")
	}

	// Get common email settings - prioritize based on provider
	var fromAddress string
	if provider == "microsoft365" {
		fromAddress = os.Getenv("MS365_FROM_ADDRESS")
		if fromAddress == "" {
			return fmt.Errorf("MS365_FROM_ADDRESS must be set for Microsoft 365 email")
		}
	} else {
		fromAddress = os.Getenv("SMTP_FROM_ADDRESS")
		if fromAddress == "" {
			fromAddress = os.Getenv("SMTP_USERNAME")
		}
		if fromAddress == "" {
			return fmt.Errorf("SMTP_FROM_ADDRESS or SMTP_USERNAME must be set for SMTP email")
		}
	}

	fromName := os.Getenv("SMTP_FROM_NAME")
	if fromName == "" {
		fromName = "Tempest Weather Alerts"
	}

	fmt.Printf("Email provider: %s\n", provider)
	fmt.Printf("From address: %s\n", fromAddress)
	fmt.Printf("From name: %s\n", fromName)
	fmt.Println()

	// Validate provider-specific configuration
	switch provider {
	case "microsoft365":
		fmt.Println("Validating Microsoft 365 OAuth2 configuration...")

		if clientID == "" {
			return fmt.Errorf("MS365_CLIENT_ID is not set (required for Microsoft 365)")
		}
		if len(clientID) >= 12 {
			fmt.Printf("Client ID: %s...%s\n", clientID[:8], clientID[len(clientID)-4:])
		} else {
			fmt.Printf("Client ID: %s\n", clientID)
		}

		if clientSecret == "" {
			return fmt.Errorf("MS365_CLIENT_SECRET is not set (required for Microsoft 365)")
		}
		fmt.Printf("Client Secret: [configured, %d characters]\n", len(clientSecret))

		if tenantID == "" {
			return fmt.Errorf("MS365_TENANT_ID is not set (required for Microsoft 365)")
		}
		fmt.Printf("Tenant ID: %s\n", tenantID)
		fmt.Println()
	case "smtp":
		fmt.Println("Validating SMTP configuration...")

		smtpHost := os.Getenv("SMTP_HOST")
		smtpPort := os.Getenv("SMTP_PORT")
		smtpUser := os.Getenv("SMTP_USERNAME")
		smtpPass := os.Getenv("SMTP_PASSWORD")

		if smtpHost == "" {
			return fmt.Errorf("SMTP_HOST is not set")
		}
		fmt.Printf("SMTP Host: %s\n", smtpHost)

		if smtpPort == "" {
			smtpPort = "587"
		}
		fmt.Printf("SMTP Port: %s\n", smtpPort)

		if smtpUser == "" {
			return fmt.Errorf("SMTP_USERNAME is not set")
		}
		fmt.Printf("SMTP Username: %s\n", smtpUser)

		if smtpPass == "" {
			return fmt.Errorf("SMTP_PASSWORD is not set")
		}
		fmt.Printf("SMTP Password: [configured, %d characters]\n", len(smtpPass))
		fmt.Println()
	}

	// Recipient email comes from command line parameter
	recipientEmail := os.Getenv("TEST_EMAIL_RECIPIENT")
	if recipientEmail == "" {
		return fmt.Errorf("no recipient email provided")
	}

	// Validate email format
	if !strings.Contains(recipientEmail, "@") || !strings.Contains(recipientEmail, ".") {
		return fmt.Errorf("invalid email address format: %s", recipientEmail)
	}

	fmt.Println()
	fmt.Printf("Sending test email to %s...\n", recipientEmail)
	fmt.Println()

	// Load alarm configuration to use the factory (tests real delivery path)
	config, err := LoadAlarmConfig(alarmsJSON)
	if err != nil {
		return fmt.Errorf("failed to load alarm configuration: %w", err)
	}

	// Create email notifier using factory (same path as real alarms)
	factory := NewNotifierFactory(config)
	notifier, err := factory.GetNotifier("email")
	if err != nil {
		return fmt.Errorf("failed to create email notifier: %w", err)
	}

	// Get command line args
	cmdLineArgs := "none"
	if len(os.Args) > 1 {
		cmdLineArgs = strings.Join(os.Args[1:], " ")
	}

	// Create test email channel
	channel := &Channel{
		Type: "email",
		Email: &EmailConfig{
			To:      []string{recipientEmail},
			Subject: "Tempest HomeKit Go - Test Email",
			Body:    "", // Will use template
		},
		Template: `🌤️ Tempest HomeKit Go - Test Email

This is a test email from Tempest HomeKit Go.

Application Information:
- Name: tempest-homekit-go
- Version: ` + appVersion + `
- Timestamp: ` + time.Now().Format("2006-01-02 15:04:05 MST") + `
- Command Line: ` + cmdLineArgs + `

Email Configuration:
- Provider: ` + provider + `
- From: ` + fromAddress + `
- Station: {{station}}

✅ If you received this email, your email configuration is working correctly!

Current Weather Data:
- Temperature: {{temperature_c}}°C / {{temperature_f}}°F
- Humidity: {{humidity}}%
- Pressure: {{pressure}} mb
- Wind Speed: {{wind_speed}} m/s
- Observation Time: {{timestamp}}`,
	}

	// Create test alarm and observation for template expansion
	testAlarm := &Alarm{
		Name:        "Email Configuration Test",
		Description: "Test email to verify email settings",
		Enabled:     true,
	}

	testObs := &weather.Observation{
		Timestamp:        time.Now().Unix(),
		AirTemperature:   20.0,
		RelativeHumidity: 50.0,
		WindAvg:          5.0,
		StationPressure:  1013.25,
	}

	// Send test email
	if err = notifier.Send(testAlarm, channel, testObs, stationName); err != nil {
		return fmt.Errorf("failed to send test email: %w", err)
	}

	fmt.Println()
	fmt.Println("✅ Test email sent successfully!")
	fmt.Println()
	fmt.Printf("Check the inbox for %s\n", recipientEmail)
	fmt.Println()

	if provider == "microsoft365" {
		fmt.Println("If you don't see the email:")
		fmt.Println("  1. Check your spam/junk folder")
		fmt.Println("  2. Verify the from address is allowed to send")
		fmt.Println("  3. Check Microsoft 365 admin console for any errors")
		fmt.Println("  4. Ensure Mail.Send permission is granted and admin consent given")
		fmt.Println("  5. Check Azure AD app registration is configured correctly")
	} else {
		fmt.Println("If you don't see the email:")
		fmt.Println("  1. Check your spam/junk folder")
		fmt.Println("  2. Verify SMTP settings are correct")
		fmt.Println("  3. Check SMTP server logs for errors")
	}

	return nil
}

// RunEmailTest is a convenience function that wraps TestEmailConfiguration and exits.
// It never returns: it calls os.Exit(0) on success and log.Fatalf on failure.
func RunEmailTest(alarmsJSON, stationName string) {
	if err := TestEmailConfiguration(alarmsJSON, stationName); err != nil {
		log.Fatalf("Email test failed: %v", err)
	}
	os.Exit(0)
}
