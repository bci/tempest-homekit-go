package config

import (
	"strings"
	"testing"
)

// TestValidateTestEmailParameter tests validation logic for test email addresses
func TestValidateTestEmailParameter(t *testing.T) {
	tests := []struct {
		name      string
		email     string
		shouldErr bool
		errMsg    string
	}{
		{
			name:      "valid email",
			email:     "user@example.com",
			shouldErr: false,
		},
		{
			name:      "valid email with subdomain",
			email:     "admin@mail.example.com",
			shouldErr: false,
		},
		{
			name:      "valid email with plus",
			email:     "user+test@example.com",
			shouldErr: false,
		},
		{
			name:      "looks like a flag - single dash",
			email:     "-alarms",
			shouldErr: true,
			errMsg:    "flag",
		},
		{
			name:      "looks like a flag - double dash",
			email:     "--alarms",
			shouldErr: true,
			errMsg:    "flag",
		},
		{
			name:      "looks like a flag - test-sms",
			email:     "--test-sms",
			shouldErr: true,
			errMsg:    "flag",
		},
		{
			name:      "empty email",
			email:     "",
			shouldErr: false, // Empty is okay, means not set
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate validation logic from main.go
			err := validateTestEmail(tt.email)

			if tt.shouldErr {
				if err == nil {
					t.Errorf("Expected error for email '%s', got none", tt.email)
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Expected error containing '%s', got: %v", tt.errMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error for email '%s', got: %v", tt.email, err)
				}
			}
		})
	}
}

// validateTestEmail mimics the validation logic in main.go
func validateTestEmail(email string) error {
	if email == "" {
		return nil
	}
	if strings.HasPrefix(email, "-") {
		return &ValidationError{Field: "email", Message: "Invalid email address: looks like a flag"}
	}
	return nil
}

// TestValidateTestSMSParameter tests validation logic for test SMS phone numbers
func TestValidateTestSMSParameter(t *testing.T) {
	tests := []struct {
		name      string
		phone     string
		shouldErr bool
		errMsg    string
	}{
		{
			name:      "valid phone with plus",
			phone:     "+15555551234",
			shouldErr: false,
		},
		{
			name:      "valid phone without plus",
			phone:     "15555551234",
			shouldErr: false,
		},
		{
			name:      "valid international phone",
			phone:     "+447911123456",
			shouldErr: false,
		},
		{
			name:      "looks like a flag - single dash",
			phone:     "-alarms",
			shouldErr: true,
			errMsg:    "flag",
		},
		{
			name:      "looks like a flag - double dash",
			phone:     "--alarms",
			shouldErr: true,
			errMsg:    "flag",
		},
		{
			name:      "looks like a flag - test-email",
			phone:     "--test-email",
			shouldErr: true,
			errMsg:    "flag",
		},
		{
			name:      "empty phone",
			phone:     "",
			shouldErr: false, // Empty is okay, means not set
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate validation logic from main.go
			err := validateTestSMS(tt.phone)

			if tt.shouldErr {
				if err == nil {
					t.Errorf("Expected error for phone '%s', got none", tt.phone)
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Expected error containing '%s', got: %v", tt.errMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error for phone '%s', got: %v", tt.phone, err)
				}
			}
		})
	}
}

// validateTestSMS mimics the validation logic in main.go
func validateTestSMS(phone string) error {
	if phone == "" {
		return nil
	}
	// Allow + prefix (international format)
	if strings.HasPrefix(phone, "+") {
		return nil
	}
	// Reject if looks like a flag
	if strings.HasPrefix(phone, "-") {
		return &ValidationError{Field: "phone", Message: "Invalid phone number: looks like a flag"}
	}
	return nil
}

// ValidationError represents a validation error returned by the test-local
// validators in this file.
type ValidationError struct {
	Field   string // Name of the field that failed validation
	Message string // Human-readable message returned by Error
}

// Error implements the error interface by returning the message alone.
func (e *ValidationError) Error() string {
	return e.Message
}

// TestValidationErrorFormat tests error message formatting
func TestValidationErrorFormat(t *testing.T) {
	err := &ValidationError{
		Field:   "email",
		Message: "Invalid email address",
	}

	if err.Error() != "Invalid email address" {
		t.Errorf("Expected error message 'Invalid email address', got '%s'", err.Error())
	}
}

// TestTestEmailRejectsFlags ensures email validation rejects flag-like values
func TestTestEmailRejectsFlags(t *testing.T) {
	flagLikeValues := []string{
		"-alarms",
		"--alarms",
		"-test-sms",
		"--test-sms",
		"-station",
		"--station",
		"-token",
		"--token",
	}

	for _, val := range flagLikeValues {
		t.Run(val, func(t *testing.T) {
			err := validateTestEmail(val)
			if err == nil {
				t.Errorf("Expected validation to reject flag-like value '%s'", val)
			}
		})
	}
}

// TestTestSMSRejectsFlags ensures SMS validation rejects flag-like values (except +)
func TestTestSMSRejectsFlags(t *testing.T) {
	flagLikeValues := []string{
		"-alarms",
		"--alarms",
		"-test-email",
		"--test-email",
		"-station",
		"--station",
	}

	for _, val := range flagLikeValues {
		t.Run(val, func(t *testing.T) {
			err := validateTestSMS(val)
			if err == nil {
				t.Errorf("Expected validation to reject flag-like value '%s'", val)
			}
		})
	}
}

// TestTestSMSAcceptsInternationalFormat ensures + prefix is allowed for phone numbers
func TestTestSMSAcceptsInternationalFormat(t *testing.T) {
	validPhones := []string{
		"+15555551234",
		"+447911123456",
		"+861234567890",
		"+33123456789",
	}

	for _, phone := range validPhones {
		t.Run(phone, func(t *testing.T) {
			err := validateTestSMS(phone)
			if err != nil {
				t.Errorf("Expected validation to accept international phone '%s', got error: %v", phone, err)
			}
		})
	}
}
