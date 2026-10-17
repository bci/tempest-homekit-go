package config

import (
	"os"
	"testing"
)

// TestGetEnvOrDefault tests environment variable handling
func TestGetEnvOrDefault(t *testing.T) {
	testEnvVar := "TEST_ENV_VAR_12345"

	// Test with set environment variable
	if err := os.Setenv(testEnvVar, "test-value"); err != nil {
		t.Fatalf("failed to set env: %v", err)
	}
	defer func() { _ = os.Unsetenv(testEnvVar) }()

	result := getEnvOrDefault(testEnvVar, "default-value")
	if result != "test-value" {
		t.Errorf("Expected 'test-value', got '%s'", result)
	}

	// Test with unset environment variable (should return default)
	_ = os.Unsetenv(testEnvVar)
	result = getEnvOrDefault(testEnvVar, "default-value")
	if result != "default-value" {
		t.Errorf("Expected 'default-value', got '%s'", result)
	}

	// Test with empty environment variable (should return default, not empty)
	if err := os.Setenv(testEnvVar, ""); err != nil {
		t.Fatalf("failed to set env: %v", err)
	}
	result = getEnvOrDefault(testEnvVar, "default-value")
	if result != "default-value" {
		t.Errorf("Expected 'default-value' for empty env var, got '%s'", result)
	}
}

// TestParseSensorConfigEdgeCases tests edge cases in sensor parsing
func TestParseSensorConfigEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected SensorConfig
	}{
		{
			name:     "empty string",
			input:    "",
			expected: SensorConfig{}, // All false
		},
		{
			name:     "only commas",
			input:    ",,,",
			expected: SensorConfig{}, // All false (empty strings ignored)
		},
		{
			name:  "sensors with extra spaces",
			input: " temp , humidity , lux ",
			expected: SensorConfig{
				Temperature: true,
				Humidity:    true,
				Light:       true,
			},
		},
		{
			name:  "mixed case sensors",
			input: "TEMP,Humidity,LUX",
			expected: SensorConfig{
				Temperature: true,
				Humidity:    true,
				Light:       true,
			},
		},
		{
			name:  "temperature alias",
			input: "temperature,humidity",
			expected: SensorConfig{
				Temperature: true,
				Humidity:    true,
			},
		},
		{
			name:  "light alias",
			input: "temp,light",
			expected: SensorConfig{
				Temperature: true,
				Light:       true,
			},
		},
		{
			name:  "single sensor",
			input: "uv",
			expected: SensorConfig{
				UV: true,
			},
		},
		{
			name:  "uvi alias",
			input: "temp,uvi",
			expected: SensorConfig{
				Temperature: true,
				UV:          true,
			},
		},
		{
			name:  "mixed aliases",
			input: "temperature,light,uvi",
			expected: SensorConfig{
				Temperature: true,
				Light:       true,
				UV:          true,
			},
		},
		{
			name:  "all sensors explicitly",
			input: "temp,humidity,lux,wind,rain,pressure,uv,lightning",
			expected: SensorConfig{
				Temperature: true,
				Humidity:    true,
				Light:       true,
				Wind:        true,
				Rain:        true,
				Pressure:    true,
				UV:          true,
				Lightning:   true,
			},
		},
		{
			name:  "duplicate sensors",
			input: "temp,temp,humidity,temp",
			expected: SensorConfig{
				Temperature: true,
				Humidity:    true,
			},
		},
		{
			name:  "unknown sensor ignored in fallback",
			input: "temp,unknown,humidity",
			expected: SensorConfig{
				Temperature: true,
				Humidity:    true,
				// unknown sensor is silently ignored in ParseSensorConfig
				// (validation happens elsewhere)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseSensorConfig(tt.input)
			if result != tt.expected {
				t.Errorf("ParseSensorConfig(%q) = %+v, expected %+v", tt.input, result, tt.expected)
			}
		})
	}
}

// TestParseSensorConfigPresets tests all preset combinations
func TestParseSensorConfigPresets(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected SensorConfig
	}{
		{
			name:  "all uppercase",
			input: "ALL",
			expected: SensorConfig{
				Temperature: true,
				Humidity:    true,
				Light:       true,
				Wind:        true,
				Rain:        true,
				Pressure:    true,
				UV:          true,
				Lightning:   true,
			},
		},
		{
			name:  "min uppercase",
			input: "MIN",
			expected: SensorConfig{
				Temperature: true,
				Humidity:    true,
				Light:       true,
			},
		},
		{
			name:  "mixed case all",
			input: "All",
			expected: SensorConfig{
				Temperature: true,
				Humidity:    true,
				Light:       true,
				Wind:        true,
				Rain:        true,
				Pressure:    true,
				UV:          true,
				Lightning:   true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseSensorConfig(tt.input)
			if result != tt.expected {
				t.Errorf("ParseSensorConfig(%q) = %+v, expected %+v", tt.input, result, tt.expected)
			}
		})
	}
}

// TestLoadConfigEdgeCases tests edge cases in config loading
func TestLoadConfigEdgeCases(t *testing.T) {
	// Save original env vars
	originalToken := os.Getenv("TEMPEST_TOKEN")
	originalStation := os.Getenv("TEMPEST_STATION_NAME")
	originalPin := os.Getenv("HOMEKIT_PIN")
	originalLogLevel := os.Getenv("LOG_LEVEL")
	originalWebPort := os.Getenv("WEB_PORT")
	originalSensors := os.Getenv("SENSORS")

	defer func() {
		// Restore original env vars
		if originalToken != "" {
			_ = os.Setenv("TEMPEST_TOKEN", originalToken)
		} else {
			_ = os.Unsetenv("TEMPEST_TOKEN")
		}
		if originalStation != "" {
			_ = os.Setenv("TEMPEST_STATION_NAME", originalStation)
		} else {
			_ = os.Unsetenv("TEMPEST_STATION_NAME")
		}
		if originalPin != "" {
			_ = os.Setenv("HOMEKIT_PIN", originalPin)
		} else {
			_ = os.Unsetenv("HOMEKIT_PIN")
		}
		if originalLogLevel != "" {
			_ = os.Setenv("LOG_LEVEL", originalLogLevel)
		} else {
			_ = os.Unsetenv("LOG_LEVEL")
		}
		if originalWebPort != "" {
			_ = os.Setenv("WEB_PORT", originalWebPort)
		} else {
			_ = os.Unsetenv("WEB_PORT")
		}
		if originalSensors != "" {
			_ = os.Setenv("SENSORS", originalSensors)
		} else {
			_ = os.Unsetenv("SENSORS")
		}
	}()

	// Clear all env vars for clean test
	_ = os.Unsetenv("TEMPEST_TOKEN")
	_ = os.Unsetenv("TEMPEST_STATION_NAME")
	_ = os.Unsetenv("HOMEKIT_PIN")
	_ = os.Unsetenv("LOG_LEVEL")
	_ = os.Unsetenv("WEB_PORT")
	_ = os.Unsetenv("SENSORS")

	// Test with all defaults (no env vars or flags)
	// Note: This test focuses on the config struct creation,
	// not the full LoadConfig which includes flag parsing and validation
	cfg := &Config{
		Token:       getEnvOrDefault("TEMPEST_TOKEN", ""),
		StationName: getEnvOrDefault("TEMPEST_STATION_NAME", ""),
		Pin:         getEnvOrDefault("HOMEKIT_PIN", "00102003"),
		LogLevel:    getEnvOrDefault("LOG_LEVEL", "error"),
		WebPort:     getEnvOrDefault("WEB_PORT", "8080"),
		Sensors:     getEnvOrDefault("SENSORS", "temp,lux,humidity,uv"),
	}

	// Test expected defaults (Token and StationName now empty by default)
	if cfg.Token != "" {
		t.Errorf("Expected empty default token, got %s", cfg.Token)
	}
	if cfg.StationName != "" {
		t.Errorf("Expected empty default station name, got %s", cfg.StationName)
	}
	if cfg.Pin != "00102003" {
		t.Errorf("Expected default PIN, got %s", cfg.Pin)
	}
	if cfg.LogLevel != "error" {
		t.Errorf("Expected default log level, got %s", cfg.LogLevel)
	}
	if cfg.WebPort != "8080" {
		t.Errorf("Expected default web port, got %s", cfg.WebPort)
	}
	if cfg.Sensors != "temp,lux,humidity,uv" {
		t.Errorf("Expected default sensors, got %s", cfg.Sensors)
	}
}

// TestParseElevationEdgeCases tests additional elevation parsing cases
// including Earth's actual elevation range validation
func TestParseElevationEdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expected  float64
		shouldErr bool
	}{
		{"zero feet", "0ft", 0.0, false},
		{"zero meters", "0m", 0.0, false},
		{"decimal feet", "100.5ft", 30.632, false},
		{"decimal meters", "100.5m", 100.5, false},
		{"death valley elevation", "-282ft", -85.95, false},
		{"dead sea elevation", "-430m", -430.0, false},
		{"mount everest elevation", "29029ft", 8848.0, false},
		{"high elevation", "8848m", 8848.0, false},
		{"leading zeros", "0100ft", 30.48, false},
		{"trailing spaces", "100ft ", 30.48, false},
		{"leading spaces", " 100ft", 30.48, false},
		{"both spaces", " 100ft ", 30.48, false},
		{"uppercase units", "100FT", 30.48, false},
		{"mixed case units", "100Ft", 30.48, false},
		{"just number", "100", 100.0, false},
		{"below dead sea", "-500m", 0, true},
		{"above everest", "9000m", 0, true},
		{"way too low feet", "-2000ft", 0, true},
		{"way too high feet", "35000ft", 0, true},
		{"no number", "ft", 0, true},
		{"invalid unit", "100km", 0, true},
		{"multiple units", "100ftm", 0, true},
		{"number in middle", "f100t", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseElevation(tt.input)

			if tt.shouldErr {
				if err == nil {
					t.Errorf("Expected error for input '%s', but got none", tt.input)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error for input '%s', but got: %v", tt.input, err)
				}
				// Allow small floating point differences
				if absFloat(result-tt.expected) > 0.1 {
					t.Errorf("Expected %f for input '%s', got %f", tt.expected, tt.input, result)
				}
			}
		})
	}
}

// absFloat is a helper function for floating point comparison; it returns
// the absolute value of x.
func absFloat(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
