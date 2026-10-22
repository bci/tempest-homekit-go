package alarm

import (
	"os"
	"path/filepath"
	"testing"

	"tempest-homekit-go/pkg/weather"
)

// TestManager_ReloadConfig checks that reloadConfig picks up a rewritten config file,
// updating both the alarm count and the enabled-alarm count.
func TestManager_ReloadConfig(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "test-alarms.json")

	initialConfig := `{
		"alarms": [
			{
				"name": "Test Alarm",
				"condition": "temperature > 25",
				"enabled": true,
				"cooldown": 3600,
				"channels": [
					{
						"type": "console",
						"template": "Test: {{temperature_c}}°C"
					}
				]
			}
		]
	}`

	err := os.WriteFile(configFile, []byte(initialConfig), 0644)
	if err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	// Create manager
	manager, err := NewManager("@"+configFile, "TestStation")
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Stop()

	// Verify initial config
	if manager.GetAlarmCount() != 1 {
		t.Errorf("Expected 1 alarm, got %d", manager.GetAlarmCount())
	}

	// Modify the config file with 2 alarms
	updatedConfig := `{
		"alarms": [
			{
				"name": "Test Alarm 1",
				"condition": "temperature > 25",
				"enabled": true,
				"cooldown": 3600,
				"channels": [
					{
						"type": "console",
						"template": "Test 1"
					}
				]
			},
			{
				"name": "Test Alarm 2",
				"condition": "humidity > 80",
				"enabled": false,
				"cooldown": 1800,
				"channels": [
					{
						"type": "console",
						"template": "Test 2"
					}
				]
			}
		]
	}`

	err = os.WriteFile(configFile, []byte(updatedConfig), 0644)
	if err != nil {
		t.Fatalf("Failed to update config: %v", err)
	}

	// Call reloadConfig directly to test the reload mechanism
	err = manager.reloadConfig()
	if err != nil {
		t.Fatalf("Failed to reload config: %v", err)
	}

	// Verify config was reloaded
	if manager.GetAlarmCount() != 2 {
		t.Errorf("Expected 2 alarms after reload, got %d", manager.GetAlarmCount())
	}

	if manager.GetEnabledAlarmCount() != 1 {
		t.Errorf("Expected 1 enabled alarm after reload, got %d", manager.GetEnabledAlarmCount())
	}
}

// TestManager_ReloadConfig_InvalidJSON checks that reloadConfig returns an error for a
// file that is no longer valid JSON and keeps the previously loaded alarms.
func TestManager_ReloadConfig_InvalidJSON(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "test-alarms.json")

	initialConfig := `{
		"alarms": [
			{
				"name": "Test Alarm",
				"condition": "temperature > 25",
				"enabled": true,
				"cooldown": 3600,
				"channels": [
					{
						"type": "console",
						"template": "Test"
					}
				]
			}
		]
	}`

	err := os.WriteFile(configFile, []byte(initialConfig), 0644)
	if err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	manager, err := NewManager("@"+configFile, "TestStation")
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Stop()

	// Write invalid JSON
	err = os.WriteFile(configFile, []byte("{invalid json"), 0644)
	if err != nil {
		t.Fatalf("Failed to write invalid config: %v", err)
	}

	// Reload should fail
	err = manager.reloadConfig()
	if err == nil {
		t.Error("Expected error when reloading invalid JSON, got nil")
	}

	// Manager should still have original config
	if manager.GetAlarmCount() != 1 {
		t.Errorf("Expected 1 alarm after failed reload, got %d", manager.GetAlarmCount())
	}
}

// TestManager_ReloadConfig_InvalidConfig checks that reloadConfig returns an error when
// the new file fails validation (an alarm without a name) and keeps the previously
// loaded alarms.
func TestManager_ReloadConfig_InvalidConfig(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "test-alarms.json")

	initialConfig := `{
		"alarms": [
			{
				"name": "Test Alarm",
				"condition": "temperature > 25",
				"enabled": true,
				"cooldown": 3600,
				"channels": [
					{
						"type": "console",
						"template": "Test"
					}
				]
			}
		]
	}`

	err := os.WriteFile(configFile, []byte(initialConfig), 0644)
	if err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	manager, err := NewManager("@"+configFile, "TestStation")
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Stop()

	// Write invalid config (no name)
	invalidConfig := `{
		"alarms": [
			{
				"condition": "temperature > 25",
				"enabled": true,
				"cooldown": 3600,
				"channels": [
					{
						"type": "console",
						"template": "Test"
					}
				]
			}
		]
	}`

	err = os.WriteFile(configFile, []byte(invalidConfig), 0644)
	if err != nil {
		t.Fatalf("Failed to write invalid config: %v", err)
	}

	// Reload should fail validation
	err = manager.reloadConfig()
	if err == nil {
		t.Error("Expected error when reloading invalid config, got nil")
	}

	// Manager should still have original config
	if manager.GetAlarmCount() != 1 {
		t.Errorf("Expected 1 alarm after failed reload, got %d", manager.GetAlarmCount())
	}
}

// TestEvaluateSimple exercises evaluateSimple through Evaluate with simple, AND and OR
// conditions that evaluate both true and false.
func TestEvaluateSimple(t *testing.T) {
	// Test the evaluateSimple function through Evaluate (which calls it)
	e := NewEvaluator()

	obs := &weather.Observation{
		AirTemperature:   30.0,
		RelativeHumidity: 75.0,
		WindAvg:          5.0,
		StationPressure:  1013.25,
	}

	// Test simple evaluation without alarm context
	result, err := e.Evaluate("temperature > 25", obs)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if !result {
		t.Error("Expected condition to be true")
	}

	result, err = e.Evaluate("humidity > 80", obs)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if result {
		t.Error("Expected condition to be false")
	}

	// Test AND condition
	result, err = e.Evaluate("temperature > 25 && humidity > 50", obs)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if !result {
		t.Error("Expected AND condition to be true")
	}

	// Test OR condition
	result, err = e.Evaluate("temperature > 50 || humidity > 50", obs)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if !result {
		t.Error("Expected OR condition to be true (humidity > 50)")
	}

	// Test combined AND/OR condition (without parentheses)
	result, err = e.Evaluate("temperature > 25 && humidity > 70", obs)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if !result {
		t.Error("Expected combined condition to be true (30 > 25 && 75 > 70)")
	}

	// Test false AND
	result, err = e.Evaluate("temperature > 50 && humidity > 70", obs)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if result {
		t.Error("Expected false AND condition (temperature not > 50)")
	}
}
