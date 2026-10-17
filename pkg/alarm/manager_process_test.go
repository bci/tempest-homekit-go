package alarm

import (
	"os"
	"path/filepath"
	"testing"

	"tempest-homekit-go/pkg/weather"
)

// TestManager_ProcessObservation_Nil checks that ProcessObservation(nil) on a
// file-backed Manager returns without panicking.
func TestManager_ProcessObservation_Nil(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "alarms.json")

	config := `{
		"alarms": [
			{
				"name": "Test",
				"condition": "temperature > 25",
				"enabled": true,
				"channels": [{"type": "console", "template": "Test"}]
			}
		]
	}`

	err := os.WriteFile(configFile, []byte(config), 0644)
	if err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	manager, err := NewManager("@"+configFile, "TestStation")
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Stop()

	// Process nil observation - should not crash
	manager.ProcessObservation(nil)
}

// TestManager_ProcessObservation_DisabledAlarm checks that a disabled alarm whose
// condition matches never records a fire time.
func TestManager_ProcessObservation_DisabledAlarm(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "alarms.json")

	config := `{
		"alarms": [
			{
				"name": "Disabled Alarm",
				"condition": "temperature > 25",
				"enabled": false,
				"cooldown": 3600,
				"channels": [{"type": "console", "template": "Test"}]
			}
		]
	}`

	err := os.WriteFile(configFile, []byte(config), 0644)
	if err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	manager, err := NewManager("@"+configFile, "TestStation")
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Stop()

	obs := &weather.Observation{
		AirTemperature: 30.0, // Would trigger if enabled
	}

	// Process observation - should skip disabled alarm
	manager.ProcessObservation(obs)

	// Verify alarm never fired
	alarm := &manager.config.Alarms[0]
	if !alarm.GetLastFired().IsZero() {
		t.Error("Disabled alarm should not have fired")
	}
}

// TestManager_ProcessObservation_InvalidCondition checks that an alarm whose condition
// fails to evaluate is skipped rather than fired.
func TestManager_ProcessObservation_InvalidCondition(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "alarms.json")

	config := `{
		"alarms": [
			{
				"name": "Invalid Condition",
				"condition": "invalid_field > 100",
				"enabled": true,
				"cooldown": 3600,
				"channels": [{"type": "console", "template": "Test"}]
			}
		]
	}`

	err := os.WriteFile(configFile, []byte(config), 0644)
	if err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	manager, err := NewManager("@"+configFile, "TestStation")
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Stop()

	obs := &weather.Observation{
		AirTemperature: 30.0,
	}

	// Process observation - should handle error gracefully
	manager.ProcessObservation(obs)

	// Alarm should not have fired due to evaluation error
	alarm := &manager.config.Alarms[0]
	if !alarm.GetLastFired().IsZero() {
		t.Error("Alarm with invalid condition should not have fired")
	}
}

// TestManager_ProcessObservation_TriggeredAlarm checks that an enabled alarm whose
// condition matches records a fire time.
func TestManager_ProcessObservation_TriggeredAlarm(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "alarms.json")

	config := `{
		"alarms": [
			{
				"name": "High Temperature",
				"condition": "temperature > 25",
				"enabled": true,
				"cooldown": 1,
				"channels": [{"type": "console", "template": "Temp: {{temperature_c}}°C"}]
			}
		]
	}`

	err := os.WriteFile(configFile, []byte(config), 0644)
	if err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	manager, err := NewManager("@"+configFile, "TestStation")
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Stop()

	obs := &weather.Observation{
		AirTemperature: 30.0,
	}

	// Process observation - should trigger alarm
	manager.ProcessObservation(obs)

	// Verify alarm fired
	alarm := &manager.config.Alarms[0]
	if alarm.GetLastFired().IsZero() {
		t.Error("Alarm should have fired")
	}
}

// TestManager_ProcessObservation_MultipleAlarms checks that one observation fires each
// matching alarm and leaves non-matching ones untouched.
func TestManager_ProcessObservation_MultipleAlarms(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "alarms.json")

	config := `{
		"alarms": [
			{
				"name": "High Temperature",
				"condition": "temperature > 25",
				"enabled": true,
				"cooldown": 1,
				"channels": [{"type": "console", "template": "High temp"}]
			},
			{
				"name": "Low Temperature",
				"condition": "temperature < 10",
				"enabled": true,
				"cooldown": 1,
				"channels": [{"type": "console", "template": "Low temp"}]
			},
			{
				"name": "High Humidity",
				"condition": "humidity > 80",
				"enabled": true,
				"cooldown": 1,
				"channels": [{"type": "console", "template": "High humidity"}]
			}
		]
	}`

	err := os.WriteFile(configFile, []byte(config), 0644)
	if err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	manager, err := NewManager("@"+configFile, "TestStation")
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Stop()

	obs := &weather.Observation{
		AirTemperature:   30.0, // Triggers first alarm
		RelativeHumidity: 85.0, // Triggers third alarm
	}

	// Process observation
	manager.ProcessObservation(obs)

	// Verify first alarm fired
	alarm1 := &manager.config.Alarms[0]
	if alarm1.GetLastFired().IsZero() {
		t.Error("First alarm should have fired (high temp)")
	}

	// Verify second alarm did NOT fire
	alarm2 := &manager.config.Alarms[1]
	if !alarm2.GetLastFired().IsZero() {
		t.Error("Second alarm should not have fired (low temp)")
	}

	// Verify third alarm fired
	alarm3 := &manager.config.Alarms[2]
	if alarm3.GetLastFired().IsZero() {
		t.Error("Third alarm should have fired (high humidity)")
	}
}

// TestManager_GetConfigPath_WithAtSign checks that a Manager created from "@path"
// reports the path without the "@" prefix.
func TestManager_GetConfigPath_WithAtSign(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "alarms.json")

	config := `{
		"alarms": [
			{
				"name": "Test",
				"condition": "temperature > 25",
				"enabled": true,
				"channels": [{"type": "console", "template": "Test"}]
			}
		]
	}`

	err := os.WriteFile(configFile, []byte(config), 0644)
	if err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	manager, err := NewManager("@"+configFile, "TestStation")
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Stop()

	// GetConfigPath should return the path without @ prefix
	path := manager.GetConfigPath()
	if path != configFile {
		t.Errorf("Expected path '%s', got '%s'", configFile, path)
	}
}

// TestManager_GetConfigPath_Inline checks that a Manager created from inline JSON
// reports "Inline configuration" as its config path.
func TestManager_GetConfigPath_Inline(t *testing.T) {
	config := `{
		"alarms": [
			{
				"name": "Test",
				"condition": "temperature > 25",
				"enabled": true,
				"channels": [{"type": "console", "template": "Test"}]
			}
		]
	}`

	manager, err := NewManager(config, "TestStation")
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Stop()

	// GetConfigPath should return "Inline configuration" for inline JSON
	path := manager.GetConfigPath()
	if path != "Inline configuration" {
		t.Errorf("Expected 'Inline configuration', got '%s'", path)
	}
}
