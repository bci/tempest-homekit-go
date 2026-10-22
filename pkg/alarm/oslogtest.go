package alarm

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"time"

	"tempest-homekit-go/pkg/weather"
)

// TestOSLogConfiguration tests oslog notification by sending a test message (macOS only).
//
// alarmsJSON is the alarm config input accepted by [LoadAlarmConfig];
// stationName fills {{station}}. It returns an error immediately on non-macOS
// systems, and otherwise if the config fails to load or the send fails. On
// success it prints Console.app and `log show` commands for finding the
// "Tempest-Test" message.
func TestOSLogConfiguration(alarmsJSON, stationName string) error {
	fmt.Println("Testing oslog notification (macOS unified logging)...")
	fmt.Println()

	// Check if running on macOS
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("oslog is only available on macOS (current OS: %s)", runtime.GOOS)
	}

	// Load alarm configuration (uses factory for real delivery path)
	config, err := LoadAlarmConfig(alarmsJSON)
	if err != nil {
		return fmt.Errorf("failed to load alarm configuration: %w", err)
	}

	// Create oslog notifier using factory
	factory := NewNotifierFactory(config)
	notifier, err := factory.GetNotifier("oslog")
	if err != nil {
		return fmt.Errorf("failed to create oslog notifier: %w", err)
	}

	// Create test alarm
	testAlarm := &Alarm{
		Name:        "OSLog Test",
		Description: "Test oslog notification",
		Enabled:     true,
	}

	// Create test channel with template
	testChannel := &Channel{
		Type:     "oslog",
		Template: fmt.Sprintf("Tempest-Test: Test oslog notification from station {{station}} - Temp: {{temperature_c}}°C, Humidity: {{humidity}}%%, Pressure: {{pressure}}mb at {{timestamp}}"),
	}

	// Create test observation
	testObs := &weather.Observation{
		Timestamp:        time.Now().Unix(),
		AirTemperature:   20.0,
		RelativeHumidity: 50.0,
		WindAvg:          5.0,
		StationPressure:  1013.25,
	}

	// Send test notification
	fmt.Println("Sending test message to oslog...")
	if err = notifier.Send(testAlarm, testChannel, testObs, stationName); err != nil {
		return fmt.Errorf("failed to send test notification: %w", err)
	}

	fmt.Println()
	fmt.Println("✅ OSLog notification sent successfully!")
	fmt.Println()
	fmt.Println("To verify delivery:")
	fmt.Println("  - Open Console.app and search for 'Tempest-Test'")
	fmt.Println("  - Or use: log show --predicate 'eventMessage CONTAINS \"Tempest-Test\"' --last 1m")
	fmt.Println("  - Or stream: log stream --predicate 'eventMessage CONTAINS \"Tempest\"'")

	return nil
}

// RunOSLogTest is a convenience function that wraps TestOSLogConfiguration and exits.
// It never returns: it calls os.Exit(0) on success and log.Fatalf on failure.
func RunOSLogTest(alarmsJSON, stationName string) {
	if err := TestOSLogConfiguration(alarmsJSON, stationName); err != nil {
		log.Fatalf("OSLog test failed: %v", err)
	}
	os.Exit(0)
}
