package alarm

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"time"

	"tempest-homekit-go/pkg/weather"
)

// TestEventLogConfiguration tests eventlog notification by sending a test message (Windows only).
//
// alarmsJSON is the alarm config input accepted by [LoadAlarmConfig];
// stationName fills {{station}}. It returns an error immediately on non-Windows
// systems, and otherwise if the config fails to load or the send fails. Note
// the Windows EventLogNotifier currently only writes to the application log,
// so the Event Viewer instructions it prints do not yet match the delivery.
func TestEventLogConfiguration(alarmsJSON, stationName string) error {
	fmt.Println("Testing Windows Event Log notification...")
	fmt.Println()

	// Check if running on Windows
	if runtime.GOOS != "windows" {
		return fmt.Errorf("eventlog is only available on Windows (current OS: %s)", runtime.GOOS)
	}

	// Load alarm configuration (uses factory for real delivery path)
	config, err := LoadAlarmConfig(alarmsJSON)
	if err != nil {
		return fmt.Errorf("failed to load alarm configuration: %w", err)
	}

	// Create eventlog notifier using factory
	factory := NewNotifierFactory(config)
	notifier, err := factory.GetNotifier("eventlog")
	if err != nil {
		return fmt.Errorf("failed to create eventlog notifier: %w", err)
	}

	// Create test alarm
	testAlarm := &Alarm{
		Name:        "EventLog Test",
		Description: "Test Windows Event Log notification",
		Enabled:     true,
	}

	// Create test channel with template
	testChannel := &Channel{
		Type:     "eventlog",
		Template: fmt.Sprintf("Tempest-Test: Test event log notification from station {{station}} - Temp: {{temperature_c}}°C, Humidity: {{humidity}}%%, Pressure: {{pressure}}mb at {{timestamp}}"),
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
	fmt.Println("Sending test message to Windows Event Log...")
	if err = notifier.Send(testAlarm, testChannel, testObs, stationName); err != nil {
		return fmt.Errorf("failed to send test notification: %w", err)
	}

	fmt.Println()
	fmt.Println("✅ EventLog notification sent successfully!")
	fmt.Println()
	fmt.Println("To verify delivery:")
	fmt.Println("  - Open Event Viewer (eventvwr.msc)")
	fmt.Println("  - Navigate to: Windows Logs → Application")
	fmt.Println("  - Look for events from source 'TempestHomeKit'")
	fmt.Println("  - Filter by: Source = TempestHomeKit")

	return nil
}

// RunEventLogTest is a convenience function that wraps TestEventLogConfiguration and exits.
// It never returns: it calls os.Exit(0) on success and log.Fatalf on failure.
func RunEventLogTest(alarmsJSON, stationName string) {
	if err := TestEventLogConfiguration(alarmsJSON, stationName); err != nil {
		log.Fatalf("EventLog test failed: %v", err)
	}
	os.Exit(0)
}
