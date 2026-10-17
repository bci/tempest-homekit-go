package alarm

import (
	"testing"
	"time"
)

// TestManager_Getters checks the Manager accessors (GetConfig, GetAlarmCount,
// GetEnabledAlarmCount, GetConfigPath, GetLastLoadTime) on a hand-built Manager, and the
// counts for an empty alarm list.
func TestManager_Getters(t *testing.T) {
	// Create a test alarm configuration
	config := &AlarmConfig{
		Alarms: []Alarm{
			{
				Name:        "Test Alarm 1",
				Description: "First test alarm",
				Condition:   "temperature > 30",
				Enabled:     true,
				Cooldown:    3600,
			},
			{
				Name:        "Test Alarm 2",
				Description: "Second test alarm",
				Condition:   "humidity > 80",
				Enabled:     false,
				Cooldown:    1800,
			},
			{
				Name:        "Test Alarm 3",
				Description: "Third test alarm",
				Condition:   "wind_speed > 10",
				Enabled:     true,
				Cooldown:    900,
			},
		},
	}

	manager := &Manager{
		config:       config,
		configPath:   "@test-alarms.json",
		lastLoadTime: time.Now(),
	}

	// Test GetConfig
	t.Run("GetConfig", func(t *testing.T) {
		cfg := manager.GetConfig()
		if cfg == nil {
			t.Fatal("GetConfig returned nil")
		}
		if len(cfg.Alarms) != 3 {
			t.Errorf("Expected 3 alarms, got %d", len(cfg.Alarms))
		}
	})

	// Test GetAlarmCount
	t.Run("GetAlarmCount", func(t *testing.T) {
		count := manager.GetAlarmCount()
		if count != 3 {
			t.Errorf("Expected alarm count 3, got %d", count)
		}
	})

	// Test GetEnabledAlarmCount
	t.Run("GetEnabledAlarmCount", func(t *testing.T) {
		count := manager.GetEnabledAlarmCount()
		if count != 2 {
			t.Errorf("Expected enabled alarm count 2, got %d", count)
		}
	})

	// Test GetConfigPath
	t.Run("GetConfigPath", func(t *testing.T) {
		path := manager.GetConfigPath()
		if path != "@test-alarms.json" {
			t.Errorf("Expected config path '@test-alarms.json', got '%s'", path)
		}
	})

	// Test GetLastLoadTime
	t.Run("GetLastLoadTime", func(t *testing.T) {
		loadTime := manager.GetLastLoadTime()
		if loadTime.IsZero() {
			t.Error("Expected non-zero last load time")
		}
		if time.Since(loadTime) > time.Minute {
			t.Error("Last load time seems too old")
		}
	})

	// Test with empty alarms list
	t.Run("GetAlarmCount_EmptyConfig", func(t *testing.T) {
		emptyManager := &Manager{
			config: &AlarmConfig{
				Alarms: []Alarm{},
			},
		}
		count := emptyManager.GetAlarmCount()
		if count != 0 {
			t.Errorf("Expected alarm count 0 for empty config, got %d", count)
		}
	})

	// Test GetEnabledAlarmCount with empty config
	t.Run("GetEnabledAlarmCount_EmptyConfig", func(t *testing.T) {
		emptyManager := &Manager{
			config: &AlarmConfig{
				Alarms: []Alarm{},
			},
		}
		count := emptyManager.GetEnabledAlarmCount()
		if count != 0 {
			t.Errorf("Expected enabled alarm count 0 for empty config, got %d", count)
		}
	})
}

// TestAlarm_Getters checks the Alarm cooldown accessors (GetLastFired,
// GetCooldownRemaining, IsInCooldown) for in-cooldown, expired and never-fired alarms,
// and that GetTriggerValue returns a value set with SetTriggerContext.
func TestAlarm_Getters(t *testing.T) {
	now := time.Now()
	alarm := &Alarm{
		Name:      "Test Alarm",
		Condition: "temperature > 25",
		Enabled:   true,
		Cooldown:  3600,
		lastFired: now.Add(-1800 * time.Second), // Fired 30 minutes ago
	}

	// Test GetLastFired
	t.Run("GetLastFired", func(t *testing.T) {
		lastFired := alarm.GetLastFired()
		if lastFired.IsZero() {
			t.Error("Expected non-zero last fired time")
		}
		diff := now.Sub(lastFired)
		if diff < 1799*time.Second || diff > 1801*time.Second {
			t.Errorf("Expected last fired ~1800 seconds ago, got %v", diff)
		}
	})

	// Test GetCooldownRemaining
	t.Run("GetCooldownRemaining", func(t *testing.T) {
		remaining := alarm.GetCooldownRemaining()
		// Should have ~1800 seconds remaining (3600 - 1800)
		if remaining < 1799 || remaining > 1801 {
			t.Errorf("Expected ~1800 seconds remaining, got %d", remaining)
		}
	})

	// Test IsInCooldown
	t.Run("IsInCooldown", func(t *testing.T) {
		if !alarm.IsInCooldown() {
			t.Error("Alarm should be in cooldown")
		}
	})

	// Test IsInCooldown when not in cooldown
	t.Run("IsInCooldown_Expired", func(t *testing.T) {
		oldAlarm := &Alarm{
			Name:      "Old Alarm",
			Condition: "temperature > 25",
			Enabled:   true,
			Cooldown:  3600,
			lastFired: now.Add(-7200 * time.Second), // Fired 2 hours ago
		}
		if oldAlarm.IsInCooldown() {
			t.Error("Alarm should not be in cooldown")
		}
	})

	// Test GetCooldownRemaining when not in cooldown
	t.Run("GetCooldownRemaining_Expired", func(t *testing.T) {
		oldAlarm := &Alarm{
			Name:      "Old Alarm",
			Condition: "temperature > 25",
			Enabled:   true,
			Cooldown:  3600,
			lastFired: now.Add(-7200 * time.Second),
		}
		remaining := oldAlarm.GetCooldownRemaining()
		if remaining != 0 {
			t.Errorf("Expected 0 remaining cooldown, got %v", remaining)
		}
	})

	// Test GetLastFired when never fired
	t.Run("GetLastFired_NeverFired", func(t *testing.T) {
		newAlarm := &Alarm{
			Name:      "New Alarm",
			Condition: "temperature > 25",
			Enabled:   true,
			Cooldown:  3600,
		}
		lastFired := newAlarm.GetLastFired()
		if !lastFired.IsZero() {
			t.Error("Expected zero time for alarm that never fired")
		}
	})

	// Test GetTriggerValue
	t.Run("GetTriggerValue", func(t *testing.T) {
		alarm.SetTriggerContext(map[string]float64{"temperature": 30.5})
		val, ok := alarm.GetTriggerValue("temperature")
		if !ok {
			t.Error("Expected trigger value to be present")
		}
		if val != 30.5 {
			t.Errorf("Expected trigger value 30.5, got %f", val)
		}
	})
}
