package alarm

import (
	"fmt"
	"log"
	"os"
	"time"

	"tempest-homekit-go/pkg/weather"
)

// TestSyslogConfiguration tests syslog notification by sending a test message.
//
// alarmsJSON is the alarm config input accepted by [LoadAlarmConfig];
// stationName fills {{station}}. It prints the SYSLOG_* settings in effect
// (or the defaults), sends one message through the real syslog notifier, and
// prints where to look for it. It returns an error if the config fails to
// load or the send fails.
func TestSyslogConfiguration(alarmsJSON, stationName string) error {
	fmt.Println("Testing syslog notification...")
	fmt.Println()

	// Load alarm configuration (uses factory for real delivery path)
	config, err := LoadAlarmConfig(alarmsJSON)
	if err != nil {
		return fmt.Errorf("failed to load alarm configuration: %w", err)
	}

	// Display syslog configuration
	if config.Syslog != nil {
		fmt.Println("Syslog Configuration:")
		if config.Syslog.Network != "" {
			fmt.Printf("  Network: %s\n", config.Syslog.Network)
			fmt.Printf("  Address: %s\n", config.Syslog.Address)
		} else {
			fmt.Println("  Type: Local syslog")
		}
		fmt.Printf("  Priority: %s\n", config.Syslog.Priority)
		if config.Syslog.Tag != "" {
			fmt.Printf("  Tag: %s\n", config.Syslog.Tag)
		}
		fmt.Println()
	} else {
		fmt.Println("No syslog configuration found in .env")
		fmt.Println("   Optional environment variables:")
		fmt.Println("   - SYSLOG_NETWORK (tcp/udp, empty for local)")
		fmt.Println("   - SYSLOG_ADDRESS (e.g., localhost:514)")
		fmt.Println("   - SYSLOG_PRIORITY (error/warning/info)")
		fmt.Println("   - SYSLOG_TAG (custom tag)")
		fmt.Println()
		fmt.Println("Using defaults: local syslog, priority=warning")
		fmt.Println()
	}

	// Create syslog notifier using factory
	factory := NewNotifierFactory(config)
	notifier, err := factory.GetNotifier("syslog")
	if err != nil {
		return fmt.Errorf("failed to create syslog notifier: %w", err)
	}

	// Create test alarm
	testAlarm := &Alarm{
		Name:        "Syslog Test",
		Description: "Test syslog notification",
		Enabled:     true,
	}

	// Create test channel with template
	testChannel := &Channel{
		Type:     "syslog",
		Template: fmt.Sprintf("Tempest-Test: Test syslog notification from station {{station}} - Temp: {{temperature_c}}°C, Humidity: {{humidity}}%%, Pressure: {{pressure}}mb at {{timestamp}}"),
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
	fmt.Println("Sending test message to syslog...")
	if err = notifier.Send(testAlarm, testChannel, testObs, stationName); err != nil {
		return fmt.Errorf("failed to send test notification: %w", err)
	}

	fmt.Println()
	fmt.Println("Syslog notification sent successfully!")
	fmt.Println()
	fmt.Println("To verify delivery:")
	if config.Syslog != nil && config.Syslog.Network != "" {
		fmt.Printf("  - Check remote syslog server: %s\n", config.Syslog.Address)
	} else {
		fmt.Println("  - macOS: Check /var/log/system.log or Console.app")
		fmt.Println("  - Linux: Check /var/log/syslog or /var/log/messages")
		fmt.Println("  - Or use: sudo tail -f /var/log/system.log | grep Tempest")
	}

	return nil
}

// RunSyslogTest is a convenience function that wraps TestSyslogConfiguration and exits.
// It never returns: it calls os.Exit(0) on success and log.Fatalf on failure.
func RunSyslogTest(alarmsJSON, stationName string) {
	if err := TestSyslogConfiguration(alarmsJSON, stationName); err != nil {
		log.Fatalf("Syslog test failed: %v", err)
	}
	os.Exit(0)
}
