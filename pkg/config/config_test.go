package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestValidateConfigDefaults ensures that validateConfig applies default unit values
// when cfg.Units or cfg.UnitsPressure are empty. This prevents regressions when
// Config structs are constructed programmatically (e.g., in tests).
func TestValidateConfigDefaults(t *testing.T) {
	cfg := &Config{
		Token:         "dummy-token",
		StationName:   "TestStation",
		Pin:           "12345678",
		LogLevel:      "error",
		WebPort:       "8080",
		Sensors:       "temp",
		Units:         "",
		UnitsPressure: "",
	}

	if err := validateConfig(cfg); err != nil {
		t.Fatalf("expected validateConfig to succeed, got error: %v", err)
	}

	if cfg.Units != "imperial" {
		t.Fatalf("expected Units to default to 'imperial', got '%s'", cfg.Units)
	}

	if cfg.UnitsPressure != "inHg" {
		t.Fatalf("expected UnitsPressure to default to 'inHg', got '%s'", cfg.UnitsPressure)
	}
}

// TestLoadConfig checks that LoadConfig picks up token, station, PIN and log level from environment variables.
func TestLoadConfig(t *testing.T) {
	// Set env vars
	if err := os.Setenv("TEMPEST_TOKEN", "testtoken"); err != nil {
		t.Fatalf("failed to set env: %v", err)
	}
	if err := os.Setenv("TEMPEST_STATION_NAME", "teststation"); err != nil {
		t.Fatalf("failed to set env: %v", err)
	}
	if err := os.Setenv("HOMEKIT_PIN", "12345678"); err != nil {
		t.Fatalf("failed to set env: %v", err)
	}
	if err := os.Setenv("LOG_LEVEL", "debug"); err != nil {
		t.Fatalf("failed to set env: %v", err)
	}
	defer func() { _ = os.Unsetenv("TEMPEST_TOKEN") }()
	defer func() { _ = os.Unsetenv("TEMPEST_STATION_NAME") }()
	defer func() { _ = os.Unsetenv("HOMEKIT_PIN") }()
	defer func() { _ = os.Unsetenv("LOG_LEVEL") }()

	cfg := LoadConfig()
	if cfg.Token != "testtoken" {
		t.Errorf("Expected token testtoken, got %s", cfg.Token)
	}
	if cfg.StationName != "teststation" {
		t.Errorf("Expected station teststation, got %s", cfg.StationName)
	}
	if cfg.Pin != "12345678" {
		t.Errorf("Expected pin 12345678, got %s", cfg.Pin)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("Expected loglevel debug, got %s", cfg.LogLevel)
	}
}

// TestParseSensorConfigAll checks that "all" enables every sensor.
func TestParseSensorConfigAll(t *testing.T) {
	config := ParseSensorConfig("all")
	expected := SensorConfig{
		Temperature: true,
		Humidity:    true,
		Light:       true,
		Wind:        true,
		Rain:        true,
		Pressure:    true,
		UV:          true,
		Lightning:   true,
	}
	if config != expected {
		t.Errorf("Expected all sensors enabled, got %+v", config)
	}
}

// TestParseSensorConfigMin checks that "min" enables only temperature, humidity and light.
func TestParseSensorConfigMin(t *testing.T) {
	config := ParseSensorConfig("min")
	expected := SensorConfig{
		Temperature: true,
		Humidity:    true,
		Light:       true,
		Wind:        false,
		Rain:        false,
		Pressure:    false,
		UV:          false,
		Lightning:   false,
	}
	if config != expected {
		t.Errorf("Expected min sensor config, got %+v", config)
	}
}

// TestParseSensorConfigTempOnly checks that "temp" enables only temperature.
func TestParseSensorConfigTempOnly(t *testing.T) {
	config := ParseSensorConfig("temp")
	expected := SensorConfig{
		Temperature: true,
		Humidity:    false,
		Light:       false,
		Wind:        false,
		Rain:        false,
		Pressure:    false,
		UV:          false,
		Lightning:   false,
	}
	if config != expected {
		t.Errorf("Expected temp-only config, got %+v", config)
	}
}

// TestParseSensorConfigCustom checks that a comma-separated list enables exactly the named sensors.
func TestParseSensorConfigCustom(t *testing.T) {
	config := ParseSensorConfig("temp,humidity,wind")
	expected := SensorConfig{
		Temperature: true,
		Humidity:    true,
		Light:       false,
		Wind:        true,
		Rain:        false,
		Pressure:    false,
		UV:          false,
		Lightning:   false,
	}
	if config != expected {
		t.Errorf("Expected custom sensor config, got %+v", config)
	}
}

// TestParseElevationFeet checks that values with an "ft" suffix are converted to metres.
func TestParseElevationFeet(t *testing.T) {
	tests := []struct {
		input    string
		expected float64
		hasError bool
	}{
		{"903ft", 275.2344, false}, // 903 feet = 275.2344 meters (more precise)
		{"1000ft", 304.8, false},   // 1000 feet = 304.8 meters
		{"0ft", 0.0, false},
		{"100.5ft", 30.6324, false}, // 100.5 feet = 30.6324 meters
	}

	for _, test := range tests {
		result, err := parseElevation(test.input)
		if test.hasError {
			if err == nil {
				t.Errorf("Expected error for input %s, got none", test.input)
			}
		} else {
			if err != nil {
				t.Errorf("Unexpected error for input %s: %v", test.input, err)
			}
			// Allow small floating point differences
			if abs(result-test.expected) > 0.01 {
				t.Errorf("For input %s, expected %.4f meters, got %.4f", test.input, test.expected, result)
			}
		}
	}
}

// TestParseElevationMeters checks that values with an "m" suffix are returned as metres.
func TestParseElevationMeters(t *testing.T) {
	tests := []struct {
		input    string
		expected float64
		hasError bool
	}{
		{"275m", 275.0, false},
		{"100.5m", 100.5, false},
		{"0m", 0.0, false},
		{"1000m", 1000.0, false},
	}

	for _, test := range tests {
		result, err := parseElevation(test.input)
		if test.hasError {
			if err == nil {
				t.Errorf("Expected error for input %s, got none", test.input)
			}
		} else {
			if err != nil {
				t.Errorf("Unexpected error for input %s: %v", test.input, err)
			}
			if abs(result-test.expected) > 0.01 {
				t.Errorf("For input %s, expected %.4f meters, got %.4f", test.input, test.expected, result)
			}
		}
	}
}

// TestParseElevationNoUnit checks that a bare number is treated as metres.
func TestParseElevationNoUnit(t *testing.T) {
	// Should assume meters when no unit provided
	result, err := parseElevation("275")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result != 275.0 {
		t.Errorf("Expected 275.0 meters, got %.4f", result)
	}
}

// TestParseElevationInvalid checks that malformed elevation strings return an error.
func TestParseElevationInvalid(t *testing.T) {
	invalidInputs := []string{
		"invalid",
		"ft",
		"m",
		"",
		"abc123ft",
		"123xyz",
	}

	for _, input := range invalidInputs {
		_, err := parseElevation(input)
		if err == nil {
			t.Errorf("Expected error for invalid input %s, got none", input)
		}
	}
}

// TestClearDatabase checks that ClearDatabase removes every file in the database directory.
func TestClearDatabase(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := filepath.Join(os.TempDir(), "homekit_test_db")
	defer func() { _ = os.RemoveAll(tempDir) }()

	// Create the directory and some test files
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	testFiles := []string{"keypair", "uuid", "version", "schema"}
	for _, file := range testFiles {
		testFile := filepath.Join(tempDir, file)
		f, err := os.Create(testFile)
		if err != nil {
			t.Fatalf("Failed to create test file %s: %v", file, err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("Failed to close test file %s: %v", file, err)
		}
	}

	// Verify files exist before clearing
	for _, file := range testFiles {
		if _, err := os.Stat(filepath.Join(tempDir, file)); os.IsNotExist(err) {
			t.Fatalf("Test file %s should exist before clearing", file)
		}
	}

	// Clear the database
	err := ClearDatabase(tempDir)
	if err != nil {
		t.Fatalf("ClearDatabase failed: %v", err)
	}

	// Verify files are gone
	for _, file := range testFiles {
		if _, err := os.Stat(filepath.Join(tempDir, file)); !os.IsNotExist(err) {
			t.Errorf("Test file %s should not exist after clearing", file)
		}
	}
}

// TestClearDatabaseNonExistentDir checks that clearing a missing directory is not an error.
func TestClearDatabaseNonExistentDir(t *testing.T) {
	// Should not error when directory doesn't exist
	err := ClearDatabase("/non/existent/directory")
	if err != nil {
		t.Errorf("ClearDatabase should not error on non-existent directory, got: %v", err)
	}
}

// TestValidateTokenAndStationRequired tests that both token and station are required for API mode
func TestValidateTokenAndStationRequired(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *Config
		expectError bool
		errorMsg    string
	}{
		{
			name: "API mode with token but no station",
			cfg: &Config{
				Token:       "test-token",
				StationName: "",
				LogLevel:    "error",
				Units:       "imperial",
				WebPort:     "8080",
				Pin:         "12345678",
			},
			expectError: true,
			errorMsg:    "both --token and --station are required",
		},
		{
			name: "API mode with station but no token",
			cfg: &Config{
				Token:       "",
				StationName: "Test Station",
				LogLevel:    "error",
				Units:       "imperial",
				WebPort:     "8080",
				Pin:         "12345678",
			},
			expectError: true,
			errorMsg:    "WeatherFlow API token is required",
		},
		{
			name: "API mode with both token and station",
			cfg: &Config{
				Token:       "test-token",
				StationName: "Test Station",
				LogLevel:    "error",
				Units:       "imperial",
				WebPort:     "8080",
				Pin:         "12345678",
			},
			expectError: false,
		},
		{
			name: "Generated weather mode without token",
			cfg: &Config{
				Token:               "",
				StationName:         "Generated",
				UseGeneratedWeather: true,
				LogLevel:            "error",
				Units:               "imperial",
				WebPort:             "8080",
				Pin:                 "12345678",
			},
			expectError: false,
		},
		{
			name: "UDP stream mode without token",
			cfg: &Config{
				Token:       "",
				StationName: "Test Station",
				UDPStream:   true,
				LogLevel:    "error",
				Units:       "imperial",
				WebPort:     "8080",
				Pin:         "12345678",
			},
			expectError: false,
		},
		{
			name: "Custom station URL without token",
			cfg: &Config{
				Token:       "",
				StationName: "Test Station",
				StationURL:  "http://localhost:8080/api/weather",
				LogLevel:    "error",
				Units:       "imperial",
				WebPort:     "8080",
				Pin:         "12345678",
			},
			expectError: false,
		},
		{
			name: "Alarm editor mode without token or station",
			cfg: &Config{
				Token:       "",
				StationName: "",
				AlarmsEdit:  "@alarms.json",
				LogLevel:    "error",
				Units:       "imperial",
				WebPort:     "8080",
				Pin:         "12345678",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.cfg)
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error containing '%s', got nil", tt.errorMsg)
				} else if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
				}
			}
		})
	}
}

// abs is a helper function for floating point comparison; it returns the
// absolute value of x.
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// contains is a helper function for substring checking; it reports whether
// substr occurs in s (an empty substr always matches).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && indexOfSubstring(s, substr) >= 0))
}

// indexOfSubstring returns the index of the first occurrence of substr in s,
// or -1 if it is absent.
func indexOfSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
