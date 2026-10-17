package alarm

import (
	"os"
	"testing"
)

// TestTestEmailConfiguration_NoMS365Credentials checks that TestEmailConfiguration
// fails with a "no email configuration found" error when neither the MS365_* nor the
// SMTP_* environment variables are set. It unsets those variables and does not restore them.
func TestTestEmailConfiguration_NoMS365Credentials(t *testing.T) {
	// Clear all email-related env vars
	_ = os.Unsetenv("MS365_CLIENT_ID")
	_ = os.Unsetenv("MS365_CLIENT_SECRET")
	_ = os.Unsetenv("MS365_TENANT_ID")
	_ = os.Unsetenv("SMTP_HOST")
	_ = os.Unsetenv("MS365_FROM_ADDRESS")
	_ = os.Unsetenv("SMTP_FROM_ADDRESS")

	err := TestEmailConfiguration("", "TestStation")
	if err == nil {
		t.Fatal("Expected error when no email provider configured, got nil")
	}

	// Check that error mentions no configuration found
	if !contains(err.Error(), "no email configuration found") {
		t.Errorf("Expected error to mention 'no email configuration found', got '%s'", err.Error())
	}
}

// contains reports whether substr occurs within s. It is a hand-rolled equivalent of
// strings.Contains used by the email-configuration tests in this file.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsSubstring(s, substr))
}

// containsSubstring reports whether substr occurs at any offset in s by brute-force
// comparison of every window; it is the scanning half of contains.
func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestTestEmailConfiguration_MS365MissingFromAddress checks that complete MS365 client
// credentials without MS365_FROM_ADDRESS produce the exact "MS365_FROM_ADDRESS must be
// set" error.
func TestTestEmailConfiguration_MS365MissingFromAddress(t *testing.T) {
	// Set MS365 credentials but no from address
	_ = os.Setenv("MS365_CLIENT_ID", "test-client-id")
	_ = os.Setenv("MS365_CLIENT_SECRET", "test-secret")
	_ = os.Setenv("MS365_TENANT_ID", "test-tenant-id")
	_ = os.Unsetenv("MS365_FROM_ADDRESS")
	_ = os.Unsetenv("SMTP_FROM_ADDRESS")
	defer func() {
		_ = os.Unsetenv("MS365_CLIENT_ID")
		_ = os.Unsetenv("MS365_CLIENT_SECRET")
		_ = os.Unsetenv("MS365_TENANT_ID")
	}()

	err := TestEmailConfiguration("", "TestStation")
	if err == nil {
		t.Fatal("Expected error when MS365_FROM_ADDRESS not set, got nil")
	}

	expectedMsg := "MS365_FROM_ADDRESS must be set for Microsoft 365 email"
	if err.Error() != expectedMsg {
		t.Errorf("Expected error '%s', got '%s'", expectedMsg, err.Error())
	}
}

// TestTestEmailConfiguration_SMTPMissingHost checks that, with no MS365 credentials and
// no SMTP_HOST, TestEmailConfiguration reports that no email configuration was found.
func TestTestEmailConfiguration_SMTPMissingHost(t *testing.T) {
	// No MS365 credentials, no SMTP host
	_ = os.Unsetenv("MS365_CLIENT_ID")
	_ = os.Unsetenv("MS365_CLIENT_SECRET")
	_ = os.Unsetenv("MS365_TENANT_ID")
	_ = os.Unsetenv("SMTP_HOST")

	err := TestEmailConfiguration("", "TestStation")
	if err == nil {
		t.Fatal("Expected error when no email provider configured, got nil")
	}

	// Check that error mentions no configuration found
	if !contains(err.Error(), "no email configuration found") {
		t.Errorf("Expected error to mention 'no email configuration found', got '%s'", err.Error())
	}
}

// TestTestEmailConfiguration_SMTPMissingFromAddress checks that SMTP_HOST without
// SMTP_FROM_ADDRESS or SMTP_USERNAME produces the exact "SMTP_FROM_ADDRESS or
// SMTP_USERNAME must be set" error.
func TestTestEmailConfiguration_SMTPMissingFromAddress(t *testing.T) {
	// Set SMTP host but no from address
	_ = os.Setenv("SMTP_HOST", "smtp.example.com")
	_ = os.Unsetenv("SMTP_FROM_ADDRESS")
	_ = os.Unsetenv("SMTP_USERNAME")
	_ = os.Unsetenv("MS365_CLIENT_ID")
	defer func() { _ = os.Unsetenv("SMTP_HOST") }()

	err := TestEmailConfiguration("", "TestStation")
	if err == nil {
		t.Fatal("Expected error when SMTP_FROM_ADDRESS not set, got nil")
	}

	expectedMsg := "SMTP_FROM_ADDRESS or SMTP_USERNAME must be set for SMTP email"
	if err.Error() != expectedMsg {
		t.Errorf("Expected error '%s', got '%s'", expectedMsg, err.Error())
	}
}

// TestTestEmailConfiguration_ProviderDetectionPriority checks that Microsoft 365 wins
// provider detection when both MS365 credentials and SMTP_HOST are set: the error is the
// MS365 missing-from-address one, not an SMTP error.
func TestTestEmailConfiguration_ProviderDetectionPriority(t *testing.T) {
	// Set both MS365 and SMTP - MS365 should take priority
	_ = os.Setenv("MS365_CLIENT_ID", "test-client-id")
	_ = os.Setenv("MS365_CLIENT_SECRET", "test-secret")
	_ = os.Setenv("MS365_TENANT_ID", "test-tenant-id")
	_ = os.Setenv("SMTP_HOST", "smtp.example.com")
	_ = os.Unsetenv("MS365_FROM_ADDRESS")
	defer func() {
		_ = os.Unsetenv("MS365_CLIENT_ID")
		_ = os.Unsetenv("MS365_CLIENT_SECRET")
		_ = os.Unsetenv("MS365_TENANT_ID")
		_ = os.Unsetenv("SMTP_HOST")
	}()

	err := TestEmailConfiguration("", "TestStation")
	if err == nil {
		t.Fatal("Expected error when MS365_FROM_ADDRESS not set, got nil")
	}

	// Should fail with MS365 error, not SMTP error
	expectedMsg := "MS365_FROM_ADDRESS must be set for Microsoft 365 email"
	if err.Error() != expectedMsg {
		t.Errorf("Expected MS365 error (provider should be MS365), got '%s'", err.Error())
	}
}

// TestAppVersion checks that the appVersion constant is non-empty and starts with "v".
func TestAppVersion(t *testing.T) {
	if appVersion == "" {
		t.Error("appVersion constant should not be empty")
	}

	// Version should start with 'v'
	if appVersion[0] != 'v' {
		t.Errorf("appVersion should start with 'v', got '%s'", appVersion)
	}
}
