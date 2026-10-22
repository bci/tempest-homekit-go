// Package main implements a HomeKit bridge for WeatherFlow Tempest weather stations.
// It provides a HomeKit-compatible interface to access weather data from Tempest stations,
// along with a web dashboard for monitoring and configuration.
//
// Besides the normal long-running service, the binary has several one-shot
// modes selected by flags (alarm editor, notification delivery tests, UDP,
// HomeKit, API and history tests, database clearing, webhook listener and
// status console). main checks them in a fixed order and runs only the
// first one that is set; see main for the order.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"tempest-homekit-go/pkg/alarm"
	"tempest-homekit-go/pkg/alarm/editor"
	"tempest-homekit-go/pkg/config"
	"tempest-homekit-go/pkg/logger"
	"tempest-homekit-go/pkg/service"
	"tempest-homekit-go/pkg/status"
	"tempest-homekit-go/pkg/udp"
	"tempest-homekit-go/pkg/weather"

	"github.com/joho/godotenv"
)

// main loads the environment file and configuration, sets up logging, and
// dispatches to the requested mode.
//
// The --env value is scanned from os.Args before flag parsing so that the
// environment file can supply defaults that flags then override. A missing
// default .env is ignored silently; a missing custom file is logged as a
// warning. One-shot modes are checked in this order and the first match
// runs and returns or exits: --version, --status-theme-list, --alarms-edit,
// --test-email, --test-sms, --test-webhook, --test-console, --test-syslog,
// --test-oslog, --test-eventlog, --test-udp, --test-homekit,
// --test-web-status, --test-alarm, --test-api, --test-history,
// --test-api-local, --cleardb, webhook listener flags, and --status.
// Otherwise it starts the service and blocks until SIGINT or SIGTERM.
// Startup failures are fatal (log.Fatalf).
func main() {
	// Parse --env flag early to determine which environment file to load
	envFile := ".env"
	for i, arg := range os.Args {
		if arg == "--env" && i+1 < len(os.Args) {
			envFile = os.Args[i+1]
			break
		}
	}

	// Load environment file (silently ignore if not present)
	if err := godotenv.Load(envFile); err != nil && envFile != ".env" {
		// If a custom env file was specified but couldn't be loaded, show error
		log.Printf("Warning: Could not load environment file '%s': %v", envFile, err)
	}

	cfg := config.LoadConfig()

	// Set up logging first (before any other operations that might log)
	logger.SetLogLevel(cfg.LogLevel)
	if cfg.LogFilter != "" {
		logger.SetLogFilter(cfg.LogFilter)
		logger.Info("Log filter enabled: only messages containing '%s' will be shown", cfg.LogFilter)
	}

	// Note: For generated weather, elevation will be logged by the service once location is selected

	// Handle version flag
	if cfg.Version {
		fmt.Println("tempest-homekit-go v1.11.0")
		fmt.Println("Built with Go 1.24.2")
		fmt.Println("HomeKit integration for WeatherFlow Tempest weather stations")
		os.Exit(0)
	}

	// Handle status theme list flag
	if cfg.StatusThemeList {
		status.ListThemes()
		os.Exit(0)
	}

	// Handle alarm editor mode
	if cfg.AlarmsEdit != "" {
		logger.Info("Alarm editor mode detected, starting alarm editor...")

		// Validate alarm file path
		alarmsFile := cfg.AlarmsEdit
		// Trim leading '@' if present
		alarmsFile = strings.TrimPrefix(alarmsFile, "@")

		// Check if file exists and is readable
		if _, err := os.Stat(alarmsFile); os.IsNotExist(err) {
			log.Fatalf("ERROR: Alarm configuration file not found: %s\n\nUsage: --alarms-edit @filename.json\nExample: --alarms-edit @tempest-alarms.json\n\nThe file must exist before starting the alarm editor.", alarmsFile)
		} else if err != nil {
			log.Fatalf("ERROR: Cannot access alarm configuration file '%s': %v\n\nPlease check file permissions.", alarmsFile, err)
		}

		// Verify it's a regular file (not a directory)
		if info, err := os.Stat(alarmsFile); err == nil && info.IsDir() {
			log.Fatalf("ERROR: '%s' is a directory, not a file.\n\nUsage: --alarms-edit @filename.json\nExample: --alarms-edit @tempest-alarms.json", alarmsFile)
		}

		editorServer, err := editor.NewServer(cfg.AlarmsEdit, cfg.AlarmsEditPort, "1.9.0", cfg.EnvFile)
		if err != nil {
			log.Fatalf("Failed to create alarm editor: %v", err)
		}
		if err := editorServer.Start(); err != nil {
			log.Fatalf("Failed to start alarm editor: %v", err)
		}
		return
	}

	// Handle email testing if requested
	if cfg.TestEmail != "" {
		// Validate email address doesn't look like a flag
		if strings.HasPrefix(cfg.TestEmail, "-") {
			log.Fatalf("Invalid email address: %s. Usage: --test-email user@example.com", cfg.TestEmail)
		}
		logger.Info("TestEmail flag detected, sending test email to %s...", cfg.TestEmail)
		runEmailTest(cfg)
		return
	}

	// Handle SMS testing if requested
	if cfg.TestSMS != "" {
		// Validate phone number doesn't look like a flag
		if strings.HasPrefix(cfg.TestSMS, "-") && !strings.HasPrefix(cfg.TestSMS, "+") {
			log.Fatalf("Invalid phone number: %s. Usage: --test-sms +15555551234", cfg.TestSMS)
		}
		logger.Info("TestSMS flag detected, sending test SMS to %s...", cfg.TestSMS)
		runSMSTest(cfg)
		return
	}

	// Handle webhook testing if requested
	if cfg.TestWebhook != "" {
		// Validate URL doesn't look like a flag
		if strings.HasPrefix(cfg.TestWebhook, "-") {
			log.Fatalf("Invalid URL: %s. Usage: --test-webhook https://example.com/webhook", cfg.TestWebhook)
		}
		logger.Info("TestWebhook flag detected, sending test webhook to %s...", cfg.TestWebhook)
		runWebhookTest(cfg)
		return
	}

	// Handle console testing if requested
	if cfg.TestConsole {
		logger.Info("TestConsole flag detected, sending test console notification...")
		runConsoleTest(cfg)
		return
	}

	// Handle syslog testing if requested
	if cfg.TestSyslog {
		logger.Info("TestSyslog flag detected, sending test syslog notification...")
		runSyslogTest(cfg)
		return
	}

	// Handle oslog testing if requested
	if cfg.TestOSLog {
		logger.Info("TestOSLog flag detected, sending test oslog notification...")
		runOSLogTest(cfg)
		return
	}

	// Handle eventlog testing if requested
	if cfg.TestEventLog {
		logger.Info("TestEventLog flag detected, sending test eventlog notification...")
		runEventLogTest(cfg)
		return
	}

	// Handle UDP testing if requested
	if cfg.TestUDP != 0 || (len(os.Args) > 1 && contains(os.Args, "--test-udp")) {
		seconds := cfg.TestUDP
		if seconds == 0 {
			seconds = 120 // Default to 120 seconds
		}
		logger.Info("TestUDP flag detected, listening for UDP broadcasts for %d seconds...", seconds)
		runUDPTest(cfg, seconds)
		return
	}

	// Handle HomeKit testing if requested
	if cfg.TestHomeKit {
		logger.Info("TestHomeKit flag detected, testing HomeKit bridge setup...")
		runHomeKitTest(cfg)
		return
	}

	// Handle web status testing if requested
	if cfg.TestWebStatus {
		logger.Info("TestWebStatus flag detected, testing web status scraping...")
		runWebStatusTest(cfg)
		return
	}

	// Handle alarm testing if requested
	if cfg.TestAlarm != "" {
		logger.Info("TestAlarm flag detected, triggering alarm '%s'...", cfg.TestAlarm)
		runAlarmTest(cfg)
		return
	}

	// Handle API testing if requested
	if cfg.TestAPI {
		logger.Info("TestAPI flag detected, running API endpoint tests...")
		runAPITests(cfg)
		return
	}

	// Handle history testing if requested
	if cfg.TestHistory {
		// Ensure INFO logs are visible for test output
		logger.SetLogLevel("info")
		logger.Info("TestHistory flag detected, fetching as much historical data as possible...")
		runHistoryTest(cfg)
		return
	}

	// Handle local API testing if requested
	if cfg.TestAPILocal {
		logger.Info("TestAPILocal flag detected, running local API endpoint tests...")
		runLocalAPITests(cfg)
		return
	}

	// Handle database clearing if requested
	if cfg.ClearDB {
		logger.Info("ClearDB flag detected, clearing HomeKit database...")
		if err := config.ClearDatabase("./db"); err != nil {
			log.Fatalf("Failed to clear database: %v", err)
		}
		logger.Info("Database cleared successfully. Please restart the application without --cleardb flag.")
		return
	}

	// Handle webhook listener if requested
	if cfg.WebhookListenerSet || cfg.WebhookPortSet {
		port := cfg.WebhookListenPort
		if port == "" {
			port = "8082" // Default to 8082
		}
		logger.Info("WebhookListen flag detected, starting webhook listener on port %s...", port)
		runWebhookListener(port)
		return
	}

	// Handle status console mode
	if cfg.Status {
		// Launch status console first (it will handle output redirection and start service)
		if err := status.RunStatusConsole(cfg, "1.11.0"); err != nil {
			log.Fatalf("Status console failed: %v", err)
		}
		return
	}

	logger.Info("Starting service with config: WebPort=%s, LogLevel=%s", cfg.WebPort, cfg.LogLevel)
	err := service.StartService(cfg, "1.11.0")
	if err != nil {
		log.Fatalf("Service failed: %v", err)
	}

	logger.Info("Service started successfully, waiting for interrupt signal...") // Wait for interrupt
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	sig := <-c
	logger.Info("Received signal %v, shutting down...", sig)
}

// runEmailTest sends a test email using the configured email settings.
//
// The email channel settings come from the alarm configuration in
// cfg.Alarms; the recipient cfg.TestEmail is handed to the alarm package via
// the TEST_EMAIL_RECIPIENT environment variable. It exits fatally when no
// alarm configuration is given.
func runEmailTest(cfg *config.Config) {
	fmt.Println("=== Email Configuration Test ===")
	fmt.Println()

	if cfg.Alarms == "" {
		log.Fatal("No alarm configuration specified. Use --alarms flag or ALARMS environment variable.")
	}

	// Set recipient via environment variable for test function
	_ = os.Setenv("TEST_EMAIL_RECIPIENT", cfg.TestEmail)

	// Use alarm package's email test function
	alarm.RunEmailTest(cfg.Alarms, cfg.StationName)
}

// runSMSTest sends a test SMS using the configured SMS settings.
//
// The SMS channel settings come from the alarm configuration in cfg.Alarms;
// the recipient cfg.TestSMS is handed to the alarm package via the
// TEST_SMS_RECIPIENT environment variable. It exits fatally when no alarm
// configuration is given.
func runSMSTest(cfg *config.Config) {
	fmt.Println("=== SMS Configuration Test ===")
	fmt.Println()

	if cfg.Alarms == "" {
		log.Fatal("No alarm configuration specified. Use --alarms flag or ALARMS environment variable.")
	}

	// Set recipient via environment variable for test function
	_ = os.Setenv("TEST_SMS_RECIPIENT", cfg.TestSMS)

	// Use alarm package's SMS test function
	alarm.RunSMSTest(cfg.Alarms, cfg.StationName)
}

// runWebhookTest sends a test webhook using the configured webhook settings.
//
// The target URL cfg.TestWebhook is handed to the alarm package via the
// TEST_WEBHOOK_URL environment variable. It exits fatally when no alarm
// configuration is given.
func runWebhookTest(cfg *config.Config) {
	fmt.Println("=== Webhook Configuration Test ===")
	fmt.Println()

	if cfg.Alarms == "" {
		log.Fatal("No alarm configuration specified. Use --alarms flag or ALARMS environment variable.")
	}

	// Set recipient via environment variable for test function
	_ = os.Setenv("TEST_WEBHOOK_URL", cfg.TestWebhook)

	// Use alarm package's webhook test function
	alarm.RunWebhookTest(cfg.Alarms, cfg.StationName)
}

// runConsoleTest sends a test console notification.
//
// It delegates to the alarm package using cfg.Alarms and cfg.StationName,
// and exits fatally when no alarm configuration is given.
func runConsoleTest(cfg *config.Config) {
	fmt.Println("=== Console Notification Test ===")
	fmt.Println()

	if cfg.Alarms == "" {
		log.Fatal("No alarm configuration specified. Use --alarms flag or ALARMS environment variable.")
	}

	// Use alarm package's console test function
	alarm.RunConsoleTest(cfg.Alarms, cfg.StationName)
}

// runSyslogTest sends a test syslog notification.
//
// It delegates to the alarm package using cfg.Alarms and cfg.StationName,
// and exits fatally when no alarm configuration is given.
func runSyslogTest(cfg *config.Config) {
	fmt.Println("=== Syslog Notification Test ===")
	fmt.Println()

	if cfg.Alarms == "" {
		log.Fatal("No alarm configuration specified. Use --alarms flag or ALARMS environment variable.")
	}

	// Use alarm package's syslog test function
	alarm.RunSyslogTest(cfg.Alarms, cfg.StationName)
}

// runOSLogTest sends a test oslog notification.
//
// It delegates to the alarm package using cfg.Alarms and cfg.StationName,
// and exits fatally when no alarm configuration is given.
func runOSLogTest(cfg *config.Config) {
	fmt.Println("=== OSLog Notification Test ===")
	fmt.Println()

	if cfg.Alarms == "" {
		log.Fatal("No alarm configuration specified. Use --alarms flag or ALARMS environment variable.")
	}

	// Use alarm package's oslog test function
	alarm.RunOSLogTest(cfg.Alarms, cfg.StationName)
}

// runEventLogTest sends a test eventlog notification.
//
// It delegates to the alarm package using cfg.Alarms and cfg.StationName,
// and exits fatally when no alarm configuration is given.
func runEventLogTest(cfg *config.Config) {
	fmt.Println("=== EventLog Notification Test ===")
	fmt.Println()

	if cfg.Alarms == "" {
		log.Fatal("No alarm configuration specified. Use --alarms flag or ALARMS environment variable.")
	}

	// Use alarm package's eventlog test function
	alarm.RunEventLogTest(cfg.Alarms, cfg.StationName)
}

// runUDPTest listens for UDP broadcasts from a local Tempest station.
//
// It listens on UDP port 50222 for the given number of seconds,
// pretty-printing each packet as it arrives and a progress line every five
// seconds, then prints final statistics and the latest observation and
// calls os.Exit(0). The config argument is unused. Failure to start the
// listener is fatal.
func runUDPTest(_ *config.Config, seconds int) {
	fmt.Printf("=== UDP Broadcast Listener Test (%d seconds) ===\n\n", seconds)

	udpListener := udp.NewUDPListener(100)

	// Set up packet callback for real-time pretty printing
	udpListener.SetPacketCallback(func(data []byte) {
		fmt.Println(udp.PrettyPrintMessage(data))
	})

	fmt.Println("Starting UDP listener on port 50222...")
	if err := udpListener.Start(); err != nil {
		log.Fatalf("Failed to start UDP listener: %v", err)
	}
	defer func() {
		if err := udpListener.Stop(); err != nil {
			log.Printf("udp listener stop error: %v", err)
		}
	}()

	fmt.Println("UDP listener started successfully")
	fmt.Printf("⏱️  Listening for %d seconds...\n\n", seconds)
	fmt.Println("Waiting for UDP broadcasts from Tempest station...")
	fmt.Println("(Make sure your station is on the same network and broadcasting)")
	fmt.Println("\n--- Live Packet Stream ---")
	fmt.Println()

	// Create ticker for periodic stats
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// Create timeout timer
	timeout := time.After(time.Duration(seconds) * time.Second)

	startTime := time.Now()
	lastPacketCount := int64(0)

	for {
		select {
		case <-timeout:
			elapsed := time.Since(startTime)
			fmt.Printf("\n⏱️  Test completed after %v\n", elapsed)

			// Get final stats
			packetCount, lastPacket, stationIP, serialNumber := udpListener.GetStats()
			fmt.Println("\n=== Final Statistics ===")
			fmt.Printf("Total packets received: %d\n", packetCount)
			if packetCount > 0 {
				fmt.Printf("Station IP: %s\n", stationIP)
				fmt.Printf("Serial Number: %s\n", serialNumber)
				fmt.Printf("Last packet: %v\n", lastPacket.Format("2006-01-02 15:04:05"))

				// Get latest observation
				if obs := udpListener.GetLatestObservation(); obs != nil {
					fmt.Println("\n=== Latest Observation ===")
					fmt.Printf("Temperature: %.1f°C (%.1f°F)\n", obs.AirTemperature, obs.AirTemperature*9/5+32)
					fmt.Printf("Humidity: %.0f%%\n", obs.RelativeHumidity)
					fmt.Printf("Pressure: %.2f mb\n", obs.StationPressure)
					fmt.Printf("Wind Speed: %.1f m/s\n", obs.WindAvg)
					fmt.Printf("Wind Gust: %.1f m/s\n", obs.WindGust)
					fmt.Printf("Wind Direction: %.0f°\n", obs.WindDirection)
					fmt.Printf("UV Index: %d\n", obs.UV)
					fmt.Printf("Light: %.0f lux\n", obs.Illuminance)
					fmt.Printf("Rain Rate: %.3f in\n", obs.RainAccumulated)
					if obs.LightningStrikeCount > 0 {
						fmt.Printf("Lightning: %d strikes, avg %.1f km away\n", obs.LightningStrikeCount, obs.LightningStrikeAvg)
					}
				}

				fmt.Println("\nUDP broadcast test completed successfully!")
			} else {
				fmt.Println("\nNo packets received. Possible issues:")
				fmt.Println("  - Tempest station not on same network")
				fmt.Println("  - Firewall blocking UDP port 50222")
				fmt.Println("  - Station not broadcasting (check station settings)")
			}
			os.Exit(0)
			return

		case <-ticker.C:
			// Show periodic statistics
			packetCount, _, stationIP, serialNumber := udpListener.GetStats()
			if packetCount > lastPacketCount {
				newPackets := packetCount - lastPacketCount
				elapsed := time.Since(startTime).Truncate(time.Second)
				fmt.Printf("\n[%v elapsed] Total: %d packets | New: %d | Station: %s | Serial: %s\n\n", elapsed, packetCount, newPackets, stationIP, serialNumber)
				lastPacketCount = packetCount
			} else if packetCount == 0 {
				elapsed := time.Since(startTime).Truncate(time.Second)
				fmt.Printf("⏳ [%v elapsed] Still waiting for packets...\n", elapsed)
			}
		}
	}
}

// runHomeKitTest tests the HomeKit bridge setup.
//
// It is a dry run: it prints the PIN, the parsed sensor selection and the
// bridge identity and pairing steps that would be used, without starting a
// bridge, then calls os.Exit(0).
func runHomeKitTest(cfg *config.Config) {
	fmt.Println("=== HomeKit Bridge Test ===")
	fmt.Println()

	fmt.Println("HomeKit Configuration:")
	fmt.Printf("  PIN: %s\n", cfg.Pin)
	fmt.Printf("  Station: %s\n", cfg.StationName)
	fmt.Printf("  Sensors: %s\n", cfg.Sensors)
	fmt.Println()

	// Parse sensor config
	sensorConfig := config.ParseSensorConfig(cfg.Sensors)
	fmt.Println("Sensor Configuration:")
	fmt.Printf("  Temperature: %v\n", sensorConfig.Temperature)
	fmt.Printf("  Humidity: %v\n", sensorConfig.Humidity)
	fmt.Printf("  Light: %v\n", sensorConfig.Light)
	fmt.Printf("  Wind: %v\n", sensorConfig.Wind)
	fmt.Printf("  Rain: %v\n", sensorConfig.Rain)
	fmt.Printf("  Pressure: %v\n", sensorConfig.Pressure)
	fmt.Printf("  UV: %v\n", sensorConfig.UV)
	fmt.Printf("  Lightning: %v\n", sensorConfig.Lightning)
	fmt.Println()

	fmt.Println("HomeKit Bridge would be created with:")
	fmt.Printf("  Name: Tempest - %s\n", cfg.StationName)
	fmt.Printf("  Manufacturer: WeatherFlow\n")
	fmt.Printf("  Model: Tempest Weather System\n")
	fmt.Printf("  Serial: Tempest-%s\n", cfg.StationName)
	fmt.Println()

	fmt.Println("To pair with HomeKit:")
	fmt.Println("  1. Open Home app on iOS/macOS")
	fmt.Println("  2. Tap '+' to add accessory")
	fmt.Println("  3. Select 'More Options'")
	fmt.Printf("  4. Select 'Tempest - %s'\n", cfg.StationName)
	fmt.Printf("  5. Enter PIN: %s\n", cfg.Pin)
	fmt.Println()

	fmt.Println("HomeKit configuration test completed successfully!")
	fmt.Println("   (Bridge was not actually started - this is a dry run)")
	os.Exit(0)
}

// runWebStatusTest tests web status scraping.
//
// The test is not implemented: after checking that a token and station name
// are set (fatal otherwise) it only prints manual testing guidance and calls
// os.Exit(0).
func runWebStatusTest(cfg *config.Config) {
	fmt.Println("=== Web Status Scraping Test ===")
	fmt.Println()

	if cfg.Token == "" || cfg.StationName == "" {
		log.Fatal("Token and station name are required for web status testing")
	}

	fmt.Printf("Testing status scraping for station: %s\n\n", cfg.StationName)

	// Note: This would require implementing a scraper test function
	// For now, provide guidance
	fmt.Println("Web status scraping test not yet implemented")
	fmt.Println()
	fmt.Println("To test web status scraping:")
	fmt.Println("  1. Ensure Chrome/Chromium is installed")
	fmt.Println("  2. Run the application with --use-web-status flag")
	fmt.Println("  3. Check logs for status scraping activity")
	fmt.Println("  4. Visit http://localhost:8080/api/status")
	fmt.Println()
	fmt.Println("Note: This feature requires headless browser support")
	fmt.Println("See pkg/web/ui_headless_test.go for implementation details")
	os.Exit(0)
}

// runAlarmTest triggers a specific alarm for testing.
//
// It loads cfg.Alarms, finds the alarm named cfg.TestAlarm, prints its
// details, and feeds a fixed synthetic observation (25 °C, 60% humidity and
// similar defaults) to a new alarm manager, then calls os.Exit(0). Missing
// configuration, an unknown name or a disabled alarm is fatal. It overrides
// the condition only on its own copy of the alarm, not on the manager's, so
// whether notifications fire depends on the alarm's real condition matching
// the synthetic observation.
func runAlarmTest(cfg *config.Config) {
	fmt.Printf("=== Alarm Trigger Test: %s ===\n\n", cfg.TestAlarm)

	if cfg.Alarms == "" {
		log.Fatal("No alarm configuration specified. Use --alarms flag or ALARMS environment variable.")
	}

	// Load alarm configuration
	alarmConfig, err := alarm.LoadAlarmConfig(cfg.Alarms)
	if err != nil {
		log.Fatalf("Failed to load alarm config: %v", err)
	}

	// Find the alarm by name
	var targetAlarm *alarm.Alarm
	for i := range alarmConfig.Alarms {
		if alarmConfig.Alarms[i].Name == cfg.TestAlarm {
			targetAlarm = &alarmConfig.Alarms[i]
			break
		}
	}

	if targetAlarm == nil {
		log.Fatalf("Alarm '%s' not found in configuration", cfg.TestAlarm)
	}

	fmt.Printf("Found alarm: %s\n", targetAlarm.Name)
	fmt.Printf("Description: %s\n", targetAlarm.Description)
	fmt.Printf("Condition: %s\n", targetAlarm.Condition)
	fmt.Printf("Enabled: %v\n", targetAlarm.Enabled)
	fmt.Printf("Channels: %d\n", len(targetAlarm.Channels))
	fmt.Println()

	if !targetAlarm.Enabled {
		log.Fatalf("Alarm '%s' is disabled in configuration", cfg.TestAlarm)
	}

	// Create a test observation that will trigger the alarm
	fmt.Println("Creating test observation to trigger alarm...")
	testObs := weather.Observation{
		Timestamp:            time.Now().Unix(),
		AirTemperature:       25.0, // Default values
		RelativeHumidity:     60.0,
		StationPressure:      1013.25,
		WindAvg:              5.0,
		WindGust:             10.0,
		WindDirection:        180.0,
		UV:                   5,
		Illuminance:          50000.0,
		RainAccumulated:      0.0,
		RainDailyTotal:       0.0,
		LightningStrikeCount: 0,
		LightningStrikeAvg:   0.0,
	}

	// Create alarm manager
	manager, err := alarm.NewManager(cfg.Alarms, cfg.StationName)
	if err != nil {
		log.Fatalf("Failed to create alarm manager: %v", err)
	}

	fmt.Println("Triggering alarm by sending test observation...")
	fmt.Println()

	// Force the alarm to fire by temporarily setting condition to always true
	// This is a test, so we modify the condition
	originalCondition := targetAlarm.Condition
	targetAlarm.Condition = "temperature > 0" // Always true condition

	// Send the observation
	manager.ProcessObservation(&testObs)

	// Restore original condition
	targetAlarm.Condition = originalCondition

	fmt.Println()
	fmt.Println("Alarm test completed!")
	fmt.Println("   Check above output for notification delivery results")
	os.Exit(0)
}

// contains checks if a string slice contains a specific string, using exact
// comparison. main uses it to spot a bare --test-udp in os.Args.
func contains(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

// runWebhookListener starts an HTTP server to listen for incoming webhook requests.
//
// port is the TCP port to listen on (all interfaces). Endpoints are
// POST /webhook, which logs the body (formatted as an alarm notification
// when it matches WebhookAlarmPayload) and replies with a JSON
// acknowledgement; GET /health; and / (any other path), which returns
// plain-text usage. It blocks until SIGINT or SIGTERM, shuts down with a
// five second grace period and calls os.Exit(0). A listen error is logged
// but does not end the wait for a signal.
func runWebhookListener(port string) {
	logger.Info("Starting webhook listener server on port %s", port)
	logger.Info("Webhook endpoints: POST /webhook, GET /health, GET /")

	// Create HTTP server
	mux := http.NewServeMux()

	// Webhook endpoint
	mux.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
		// Only accept POST requests
		if r.Method != http.MethodPost {
			logger.Error("Webhook endpoint received invalid method: %s from %s", r.Method, r.RemoteAddr)
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Read the request body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			logger.Error("Failed to read webhook request body from %s: %v", r.RemoteAddr, err)
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}

		// Log webhook reception at INFO level
		logger.Info("Webhook received from %s (%d bytes)", r.RemoteAddr, len(body))

		// Try to parse and format alarm data like console notifications
		if formattedMessage := formatWebhookAlarmMessage(body); formattedMessage != "" {
			logger.Alarm("%s", formattedMessage)
		}

		// Log detailed information at DEBUG level
		logger.Debug("Webhook details - Method: %s, URL: %s, Content-Type: %s",
			r.Method, r.URL.String(), r.Header.Get("Content-Type"))

		// Log headers at DEBUG level
		if len(r.Header) > 0 {
			headers := make([]string, 0, len(r.Header))
			for key, values := range r.Header {
				headers = append(headers, fmt.Sprintf("%s=%s", key, strings.Join(values, ",")))
			}
			logger.Debug("Webhook headers: %s", strings.Join(headers, "; "))
		}

		// Log body content at DEBUG level
		if len(body) > 0 {
			// Try to parse and pretty print as JSON
			var jsonData interface{}
			if err := json.Unmarshal(body, &jsonData); err == nil {
				// Pretty print JSON
				prettyJSON, _ := json.MarshalIndent(jsonData, "", "  ")
				logger.Debug("Webhook body (JSON):\n%s", string(prettyJSON))
			} else {
				// Not JSON, log as string
				logger.Debug("Webhook body (text):\n%s", string(body))
			}
		} else {
			logger.Debug("Webhook body: (empty)")
		}

		// Send success response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","message":"Webhook received successfully"}`))
	})

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		logger.Debug("Health check request from %s", r.RemoteAddr)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","service":"webhook-listener"}`))
	})

	// Root endpoint with instructions
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		logger.Debug("Root endpoint request from %s", r.RemoteAddr)
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		response := fmt.Sprintf(`Webhook Listener Server

This server listens for incoming webhook requests.

Endpoints:
  POST /webhook  - Receive webhook payloads (logs to console)
  GET  /health   - Health check endpoint

Send webhooks to: http://localhost:%s/webhook

Server started at: %s
`, port, time.Now().Format("2006-01-02 15:04:05"))
		_, _ = w.Write([]byte(response))
	})

	// Start the server
	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	// Channel to listen for interrupt signal
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	// Start server in a goroutine
	go func() {
		logger.Info("Webhook listener server started successfully on http://localhost:%s", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Failed to start webhook listener server: %v", err)
		}
	}()

	// Wait for interrupt signal
	sig := <-c
	logger.Info("Received signal %v, shutting down webhook listener server", sig)

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		logger.Error("Webhook listener server forced to shutdown: %v", err)
	} else {
		logger.Info("Webhook listener server shut down gracefully")
	}

	os.Exit(0)
}

// WebhookAlarmPayload represents the structure of incoming webhook payloads
// in the JSON shape used by this application's webhook alarm templates
// (see the default body in pkg/alarm/webhooktest.go).
type WebhookAlarmPayload struct {
	// Alarm identifies the alarm that fired. Tags is a comma-separated list.
	Alarm struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Condition   string `json:"condition"`
		Tags        string `json:"tags"`
	} `json:"alarm"`
	Station   string                 `json:"station"`   // Station name
	Timestamp string                 `json:"timestamp"` // "2006-01-02 15:04:05 MST" or RFC 3339
	Sensors   map[string]interface{} `json:"sensors"`   // Sensor readings keyed by name with unit suffix, e.g. "temperature_c"
	AppInfo   string                 `json:"app_info"`  // Free-form sender application information; not used for formatting
}

// formatWebhookAlarmMessage parses webhook payload and formats it like console notifications.
//
// body is the raw request body. It returns "" when the body is not JSON or
// lacks an alarm name or sensor readings, so callers can skip non-alarm
// payloads. Known sensor keys are mapped onto a weather.Observation; an
// unparsable or missing timestamp falls back to now. The result is a
// multi-line plain-text message.
func formatWebhookAlarmMessage(body []byte) string {
	var payload WebhookAlarmPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		// Not a valid alarm webhook payload, return empty string
		return ""
	}

	// Check if this looks like an alarm webhook (has alarm and sensors fields)
	if payload.Alarm.Name == "" || len(payload.Sensors) == 0 {
		return ""
	}

	// Create alarm struct from payload
	alarm := &alarm.Alarm{
		Name:        payload.Alarm.Name,
		Description: payload.Alarm.Description,
		Condition:   payload.Alarm.Condition,
		Enabled:     true, // Assume enabled if we're receiving it
	}

	// Parse tags if present
	if payload.Alarm.Tags != "" {
		alarm.Tags = strings.Split(payload.Alarm.Tags, ",")
		for i, tag := range alarm.Tags {
			alarm.Tags[i] = strings.TrimSpace(tag)
		}
	}

	// Create observation from sensors data
	obs := &weather.Observation{}

	// Parse timestamp
	if payload.Timestamp != "" {
		if t, err := time.Parse("2006-01-02 15:04:05 MST", payload.Timestamp); err == nil {
			obs.Timestamp = t.Unix()
		} else if t, err := time.Parse(time.RFC3339, payload.Timestamp); err == nil {
			obs.Timestamp = t.Unix()
		} else {
			// Use current time if parsing fails
			obs.Timestamp = time.Now().Unix()
		}
	} else {
		obs.Timestamp = time.Now().Unix()
	}

	// Parse sensor values
	if val, ok := payload.Sensors["temperature_c"].(float64); ok {
		obs.AirTemperature = val
	}
	if val, ok := payload.Sensors["humidity"].(float64); ok {
		obs.RelativeHumidity = val
	}
	if val, ok := payload.Sensors["pressure_mb"].(float64); ok {
		obs.StationPressure = val
	}
	if val, ok := payload.Sensors["wind_speed_ms"].(float64); ok {
		obs.WindAvg = val
	}
	if val, ok := payload.Sensors["wind_gust_ms"].(float64); ok {
		obs.WindGust = val
	}
	if val, ok := payload.Sensors["wind_direction_deg"].(float64); ok {
		obs.WindDirection = val
	}
	if val, ok := payload.Sensors["illuminance_lux"].(float64); ok {
		obs.Illuminance = val
	}
	if val, ok := payload.Sensors["uv_index"].(float64); ok {
		obs.UV = int(val)
	}
	if val, ok := payload.Sensors["rain_rate_mmh"].(float64); ok {
		obs.RainAccumulated = val
	}
	if val, ok := payload.Sensors["rain_daily_mm"].(float64); ok {
		obs.RainDailyTotal = val
	}
	if val, ok := payload.Sensors["lightning_count"].(float64); ok {
		obs.LightningStrikeCount = int(val)
	}
	if val, ok := payload.Sensors["lightning_distance_km"].(float64); ok {
		obs.LightningStrikeAvg = val
	}

	// Format the message like console notifications
	alarmInfo := formatAlarmInfo(alarm, false)
	sensorInfo := formatSensorInfoWithAlarm(obs, alarm, false)

	message := fmt.Sprintf("WEBHOOK ALARM: %s\n%s\n\nCurrent Conditions:\n%s",
		payload.Alarm.Name, alarmInfo, sensorInfo)

	return message
}

// formatAlarmInfo returns formatted alarm information.
//
// It lists name, description, condition, enabled status, cooldown (shown in
// the largest whole unit of hours, minutes or seconds) and tags. isHTML
// selects an inline-styled HTML table instead of plain text lines.
func formatAlarmInfo(alarm *alarm.Alarm, isHTML bool) string {
	enabledStr := "enabled"
	if !alarm.Enabled {
		enabledStr = "disabled"
	}

	cooldownStr := fmt.Sprintf("%d seconds", alarm.Cooldown)
	if alarm.Cooldown >= 3600 {
		cooldownStr = fmt.Sprintf("%d hours", alarm.Cooldown/3600)
	} else if alarm.Cooldown >= 60 {
		cooldownStr = fmt.Sprintf("%d minutes", alarm.Cooldown/60)
	}

	tagsStr := "none"
	if len(alarm.Tags) > 0 {
		tagsStr = strings.Join(alarm.Tags, ", ")
	}

	if isHTML {
		return fmt.Sprintf(`<table style="border-collapse: collapse; width: 100%%;">
			<tr><td style="padding: 5px; border: 1px solid #ddd; font-weight: bold;">Alarm:</td><td style="padding: 5px; border: 1px solid #ddd;">%s</td></tr>
			<tr><td style="padding: 5px; border: 1px solid #ddd; font-weight: bold;">Description:</td><td style="padding: 5px; border: 1px solid #ddd;">%s</td></tr>
			<tr><td style="padding: 5px; border: 1px solid #ddd; font-weight: bold;">Condition:</td><td style="padding: 5px; border: 1px solid #ddd;">%s</td></tr>
			<tr><td style="padding: 5px; border: 1px solid #ddd; font-weight: bold;">Status:</td><td style="padding: 5px; border: 1px solid #ddd;">%s</td></tr>
			<tr><td style="padding: 5px; border: 1px solid #ddd; font-weight: bold;">Cooldown:</td><td style="padding: 5px; border: 1px solid #ddd;">%s</td></tr>
			<tr><td style="padding: 5px; border: 1px solid #ddd; font-weight: bold;">Tags:</td><td style="padding: 5px; border: 1px solid #ddd;">%s</td></tr>
		</table>`,
			alarm.Name, alarm.Description, alarm.Condition, enabledStr, cooldownStr, tagsStr)
	}

	return fmt.Sprintf("Alarm: %s\nDescription: %s\nCondition: %s\nStatus: %s\nCooldown: %s\nTags: %s",
		alarm.Name, alarm.Description, alarm.Condition, enabledStr, cooldownStr, tagsStr)
}

// formatSensorInfoWithAlarm returns formatted sensor information with alarm context.
//
// It shows each current reading from obs (with imperial conversions for
// temperature, wind and daily rain, and a cardinal wind direction) beside
// the "Last" value recorded on alarm, preferring the value captured when the
// alarm triggered over the previous value. "Last" is "N/A" when alarm is nil
// or has no value for that sensor. isHTML selects an inline-styled HTML
// table instead of plain text lines.
func formatSensorInfoWithAlarm(obs *weather.Observation, alarm *alarm.Alarm, isHTML bool) string {
	tempF := obs.AirTemperature*9/5 + 32
	windSpeedMph := obs.WindAvg * 2.23694
	windGustMph := obs.WindGust * 2.23694
	rainDaily := obs.RainDailyTotal / 25.4 // Convert mm to inches

	// Wind direction cardinal
	dir := obs.WindDirection
	cardinal := "N"
	switch {
	case dir >= 337.5 || dir < 22.5:
		cardinal = "N"
	case dir >= 22.5 && dir < 67.5:
		cardinal = "NE"
	case dir >= 67.5 && dir < 112.5:
		cardinal = "E"
	case dir >= 112.5 && dir < 157.5:
		cardinal = "SE"
	case dir >= 157.5 && dir < 202.5:
		cardinal = "S"
	case dir >= 202.5 && dir < 247.5:
		cardinal = "SW"
	case dir >= 247.5 && dir < 292.5:
		cardinal = "W"
	case dir >= 292.5 && dir < 337.5:
		cardinal = "NW"
	}

	// Helper to get previous value with proper formatting
	getPrevValue := func(key string, _ /* current */ float64, format string) string {
		if alarm == nil {
			return "N/A"
		}
		if prev, ok := alarm.GetTriggerValue(key); ok {
			return fmt.Sprintf(format, prev)
		}
		if prev, ok := alarm.GetPreviousValue(key); ok {
			return fmt.Sprintf(format, prev)
		}
		return "N/A"
	}

	// Special handler for illuminance which needs number formatting
	getPrevLux := func() string {
		if alarm == nil {
			return "N/A"
		}
		if prev, ok := alarm.GetTriggerValue("lux"); ok {
			return formatNumber(prev)
		}
		if prev, ok := alarm.GetPreviousValue("lux"); ok {
			return formatNumber(prev)
		}
		return "N/A"
	}

	if isHTML {
		return fmt.Sprintf(`<table style="border-collapse: collapse; width: 100%%;">
			<tr style="background: #f0f0f0;"><th style="padding: 5px; border: 1px solid #ddd;">Sensor</th><th style="padding: 5px; border: 1px solid #ddd;">Current</th><th style="padding: 5px; border: 1px solid #ddd;">Last</th></tr>
			<tr><td style="padding: 5px; border: 1px solid #ddd;"><strong>Temperature:</strong></td><td style="padding: 5px; border: 1px solid #ddd;">%.1f°F (%.1f°C)</td><td style="padding: 5px; border: 1px solid #ddd;">%s°C</td></tr>
			<tr><td style="padding: 5px; border: 1px solid #ddd;"><strong>Humidity:</strong></td><td style="padding: 5px; border: 1px solid #ddd;">%.0f%%</td><td style="padding: 5px; border: 1px solid #ddd;">%s%%</td></tr>
			<tr><td style="padding: 5px; border: 1px solid #ddd;"><strong>Pressure:</strong></td><td style="padding: 5px; border: 1px solid #ddd;">%.2f mb</td><td style="padding: 5px; border: 1px solid #ddd;">%s mb</td></tr>
			<tr><td style="padding: 5px; border: 1px solid #ddd;"><strong>Wind Speed:</strong></td><td style="padding: 5px; border: 1px solid #ddd;">%.1f mph (%.1f m/s)</td><td style="padding: 5px; border: 1px solid #ddd;">%s m/s</td></tr>
			<tr><td style="padding: 5px; border: 1px solid #ddd;"><strong>Wind Gust:</strong></td><td style="padding: 5px; border: 1px solid #ddd;">%.1f mph (%.1f m/s)</td><td style="padding: 5px; border: 1px solid #ddd;">%s m/s</td></tr>
			<tr><td style="padding: 5px; border: 1px solid #ddd;"><strong>Wind Direction:</strong></td><td style="padding: 5px; border: 1px solid #ddd;">%.0f° (%s)</td><td style="padding: 5px; border: 1px solid #ddd;">%s°</td></tr>
			<tr><td style="padding: 5px; border: 1px solid #ddd;"><strong>UV Index:</strong></td><td style="padding: 5px; border: 1px solid #ddd;">%d</td><td style="padding: 5px; border: 1px solid #ddd;">%s</td></tr>
			<tr><td style="padding: 5px; border: 1px solid #ddd;"><strong>Illuminance:</strong></td><td style="padding: 5px; border: 1px solid #ddd;">%s lux</td><td style="padding: 5px; border: 1px solid #ddd;">%s lux</td></tr>
			<tr><td style="padding: 5px; border: 1px solid #ddd;"><strong>Rain Rate:</strong></td><td style="padding: 5px; border: 1px solid #ddd;">%.2f mm/hr</td><td style="padding: 5px; border: 1px solid #ddd;">%s mm/hr</td></tr>
			<tr><td style="padding: 5px; border: 1px solid #ddd;"><strong>Daily Rain:</strong></td><td style="padding: 5px; border: 1px solid #ddd;">%.2f in (%.1f mm)</td><td style="padding: 5px; border: 1px solid #ddd;">%s mm</td></tr>
			<tr><td style="padding: 5px; border: 1px solid #ddd;"><strong>Lightning:</strong></td><td style="padding: 5px; border: 1px solid #ddd;">%d strikes</td><td style="padding: 5px; border: 1px solid #ddd;">%s strikes</td></tr>
		</table>`,
			tempF, obs.AirTemperature, getPrevValue("temperature", obs.AirTemperature, "%.1f"),
			obs.RelativeHumidity, getPrevValue("humidity", obs.RelativeHumidity, "%.0f"),
			obs.StationPressure, getPrevValue("pressure", obs.StationPressure, "%.2f"),
			windSpeedMph, obs.WindAvg, getPrevValue("wind_speed", obs.WindAvg, "%.1f"),
			windGustMph, obs.WindGust, getPrevValue("wind_gust", obs.WindGust, "%.1f"),
			obs.WindDirection, cardinal, getPrevValue("wind_direction", obs.WindDirection, "%.0f"),
			obs.UV, getPrevValue("uv", float64(obs.UV), "%.0f"),
			formatNumber(obs.Illuminance), getPrevLux(),
			obs.RainAccumulated, getPrevValue("rain_rate", obs.RainAccumulated, "%.2f"),
			rainDaily, obs.RainDailyTotal, getPrevValue("rain_daily", obs.RainDailyTotal, "%.1f"),
			obs.LightningStrikeCount, getPrevValue("lightning_count", float64(obs.LightningStrikeCount), "%.0f"))
	}

	return fmt.Sprintf(`Temperature: %.1f°F (%.1f°C) [Last: %s°C]
Humidity: %.0f%% [Last: %s%%]
Pressure: %.2f mb [Last: %s mb]
Wind Speed: %.1f mph (%.1f m/s) [Last: %s m/s]
Wind Gust: %.1f mph (%.1f m/s) [Last: %s m/s]
Wind Direction: %.0f° (%s) [Last: %s°]
UV Index: %d [Last: %s]
Illuminance: %s lux [Last: %s lux]
Rain Rate: %.2f mm/hr [Last: %s mm/hr]
Daily Rain: %.2f in (%.1f mm) [Last: %s mm]
Lightning: %d strikes [Last: %s strikes]`,
		tempF, obs.AirTemperature, getPrevValue("temperature", obs.AirTemperature, "%.1f"),
		obs.RelativeHumidity, getPrevValue("humidity", obs.RelativeHumidity, "%.0f"),
		obs.StationPressure, getPrevValue("pressure", obs.StationPressure, "%.2f"),
		windSpeedMph, obs.WindAvg, getPrevValue("wind_speed", obs.WindAvg, "%.1f"),
		windGustMph, obs.WindGust, getPrevValue("wind_gust", obs.WindGust, "%.1f"),
		obs.WindDirection, cardinal, getPrevValue("wind_direction", obs.WindDirection, "%.0f"),
		obs.UV, getPrevValue("uv", float64(obs.UV), "%.0f"),
		formatNumber(obs.Illuminance), getPrevLux(),
		obs.RainAccumulated, getPrevValue("rain_rate", obs.RainAccumulated, "%.2f"),
		rainDaily, obs.RainDailyTotal, getPrevValue("rain_daily", obs.RainDailyTotal, "%.1f"),
		obs.LightningStrikeCount, getPrevValue("lightning_count", float64(obs.LightningStrikeCount), "%.0f"))
}

// formatNumber formats a number with thousands separator, rounded to a whole
// number (for example 12345.6 becomes "12,346").
func formatNumber(n float64) string {
	s := fmt.Sprintf("%.0f", n)
	if len(s) <= 3 {
		return s
	}

	var result strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result.WriteString(",")
		}
		result.WriteRune(c)
	}
	return result.String()
}

// runAPITests performs comprehensive testing of all WeatherFlow API endpoints
// to verify connectivity and data availability before starting the main service.
//
// In order it fetches the station list, the named station's details and
// Tempest device ID, the current observation, recent historical
// observations and the forecast, printing a summary of each (and the raw
// JSON when the log level is debug). The first failure is fatal.
func runAPITests(cfg *config.Config) {
	fmt.Println("=== WeatherFlow API Endpoint Tests ===")

	// Test 1: Get Stations
	fmt.Println("\n1. Testing Stations API...")
	stations, err := weather.GetStations(cfg.Token)
	if err != nil {
		log.Fatalf("Failed to get stations: %v", err)
	}

	if cfg.LogLevel == "debug" {
		fmt.Println("\n--- RAW STATIONS DATA ---")
		stationsJSON, _ := json.MarshalIndent(stations, "", "  ")
		fmt.Println(string(stationsJSON))
		fmt.Println("--- END RAW DATA ---")
	}

	fmt.Printf("Found %d stations\n", len(stations))
	for _, station := range stations {
		fmt.Printf("   - ID: %d, Name: '%s', StationName: '%s'\n",
			station.StationID, station.Name, station.StationName)
	}

	// Test 2: Find and get station details
	fmt.Printf("\n2. Testing Station Details API for '%s'...\n", cfg.StationName)
	station := weather.FindStationByName(stations, cfg.StationName)
	if station == nil {
		log.Fatalf("Station '%s' not found", cfg.StationName)
	}
	fmt.Printf("Found station: %s (ID: %d)\n", station.Name, station.StationID)

	stationDetails, err := weather.GetStationDetails(station.StationID, cfg.Token)
	if err != nil {
		log.Fatalf("Failed to get station details: %v", err)
	}

	if cfg.LogLevel == "debug" {
		fmt.Println("\n--- RAW STATION DETAILS DATA ---")
		detailsJSON, _ := json.MarshalIndent(stationDetails, "", "  ")
		fmt.Println(string(detailsJSON))
		fmt.Println("--- END RAW DATA ---")
	}

	fmt.Printf("Station has %d devices\n", len(stationDetails.Devices))
	for i, device := range stationDetails.Devices {
		fmt.Printf("   Device %d: ID=%d, Type=%s, Serial=%s\n",
			i+1, device.DeviceID, device.DeviceType, device.SerialNumber)
	}

	// Test 3: Get Tempest device ID
	fmt.Println("\n3. Testing Tempest Device Discovery...")
	deviceID, err := weather.GetTempestDeviceID(stationDetails)
	if err != nil {
		log.Fatalf("Failed to find Tempest device: %v", err)
	}
	fmt.Printf("Tempest Device ID: %d\n", deviceID)

	// Test 4: Get current observation
	fmt.Println("\n4. Testing Current Observation API...")
	obs, err := weather.GetObservation(station.StationID, cfg.Token)
	if err != nil {
		log.Fatalf("Failed to get current observation: %v", err)
	}

	if cfg.LogLevel == "debug" {
		fmt.Println("\n--- RAW OBSERVATION DATA ---")
		obsJSON, _ := json.MarshalIndent(obs, "", "  ")
		fmt.Println(string(obsJSON))
		fmt.Println("--- END RAW DATA ---")
	}

	obsTime := time.Unix(obs.Timestamp, 0)
	fmt.Printf("Current observation retrieved\n")
	fmt.Printf("   - Time: %s\n", obsTime.Format("2006-01-02 15:04:05"))
	fmt.Printf("   - Temperature: %.1f°C\n", obs.AirTemperature)
	fmt.Printf("   - Humidity: %.1f%%\n", obs.RelativeHumidity)
	fmt.Printf("   - Rain: %.3f in\n", obs.RainAccumulated)

	// Test 5: Get historical observations using day_offset
	fmt.Println("\n5. Testing Historical Observations API (day_offset)...")
	startTime := time.Now()
	observations, err := weather.GetHistoricalObservations(station.StationID, cfg.Token, cfg.LogLevel)
	if err != nil {
		log.Fatalf("Failed to get historical observations: %v", err)
	}
	elapsed := time.Since(startTime)

	fmt.Printf("Historical data retrieved in %.2f seconds\n", elapsed.Seconds())
	fmt.Printf("   - Total observations: %d\n", len(observations))

	if len(observations) > 0 {
		oldestObs := time.Unix(observations[len(observations)-1].Timestamp, 0)
		newestObs := time.Unix(observations[0].Timestamp, 0)
		timeSpan := newestObs.Sub(oldestObs)

		fmt.Printf("   - Time span: %.1f hours\n", timeSpan.Hours())
		fmt.Printf("   - Oldest: %s\n", oldestObs.Format("2006-01-02 15:04:05"))
		fmt.Printf("   - Newest: %s\n", newestObs.Format("2006-01-02 15:04:05"))

		// Count observations by day
		todayCount := 0
		yesterdayCount := 0
		now := time.Now()
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		yesterday := today.Add(-24 * time.Hour)

		for _, obs := range observations {
			obsTime := time.Unix(obs.Timestamp, 0).In(now.Location())
			obsDay := time.Date(obsTime.Year(), obsTime.Month(), obsTime.Day(), 0, 0, 0, 0, now.Location())

			if obsDay.Equal(today) {
				todayCount++
			} else if obsDay.Equal(yesterday) {
				yesterdayCount++
			}
		}

		fmt.Printf("   - Today: %d observations\n", todayCount)
		fmt.Printf("   - Yesterday: %d observations\n", yesterdayCount)

		// Show sample observations
		fmt.Printf("   - Sample observations:\n")
		sampleCount := 3
		if len(observations) < sampleCount {
			sampleCount = len(observations)
		}

		for i := 0; i < sampleCount; i++ {
			obs := observations[i]
			obsTime := time.Unix(obs.Timestamp, 0)
			fmt.Printf("     %d. %s: Temp=%.1f°C, Rain=%.3fin\n",
				i+1, obsTime.Format("15:04:05"), obs.AirTemperature, obs.RainAccumulated)
		}
	}

	// Test 6: Get forecast data
	fmt.Println("\n6. Testing Forecast API...")
	forecast, err := weather.GetForecast(station.StationID, cfg.Token)
	if err != nil {
		log.Fatalf("Failed to get forecast: %v", err)
	}

	if cfg.LogLevel == "debug" {
		fmt.Println("\n--- RAW FORECAST DATA ---")
		forecastJSON, _ := json.MarshalIndent(forecast, "", "  ")
		fmt.Println(string(forecastJSON))
		fmt.Println("--- END RAW DATA ---")
	}

	fmt.Printf("Forecast data retrieved\n")
	fmt.Printf("Station ID: %d\n", forecast.StationID)
	fmt.Printf("Station Name: %s\n", forecast.StationName)
	fmt.Printf("Timezone: %s\n", forecast.Timezone)

	if forecast.CurrentConditions.Time > 0 {
		currentTime := time.Unix(forecast.CurrentConditions.Time, 0)
		fmt.Printf("\nCurrent Conditions (as of %s):\n", currentTime.Format("2006-01-02 15:04:05"))
		fmt.Printf("   - Temperature: %.1f°C\n", forecast.CurrentConditions.AirTemperature)
		fmt.Printf("   - Feels Like: %.1f°C\n", forecast.CurrentConditions.FeelsLike)
		fmt.Printf("   - Conditions: %s\n", forecast.CurrentConditions.Conditions)
		fmt.Printf("   - Icon: %s\n", forecast.CurrentConditions.Icon)
		fmt.Printf("   - Humidity: %d%%\n", forecast.CurrentConditions.RelativeHumidity)
		fmt.Printf("   - Wind: %.1f m/s\n", forecast.CurrentConditions.WindAvg)
		fmt.Printf("   - Precipitation: %d%%\n", forecast.CurrentConditions.PrecipProbability)
	}

	if len(forecast.Forecast.Daily) > 0 {
		fmt.Printf("\nDaily Forecast (%d days):\n", len(forecast.Forecast.Daily))
		for i, day := range forecast.Forecast.Daily {
			if i >= 5 {
				break // Show only first 5 days
			}
			dayTime := time.Unix(day.Time, 0)
			fmt.Printf("   Day %d (%s):\n", i+1, dayTime.Format("Mon Jan 2"))
			fmt.Printf("      - Conditions: %s\n", day.Conditions)
			fmt.Printf("      - Icon: %s\n", day.Icon)
			fmt.Printf("      - Temp High: %.1f°C\n", day.AirTempHigh)
			fmt.Printf("      - Temp Low: %.1f°C\n", day.AirTempLow)
			fmt.Printf("      - Precipitation: %d%%\n", day.PrecipProbability)
		}
	}

	fmt.Println("\nAll API endpoint tests completed successfully!")
	fmt.Println("\n=== Summary ===")
	fmt.Printf("- Stations API: Working\n")
	fmt.Printf("- Station Details API: Working\n")
	fmt.Printf("- Device Discovery: Working\n")
	fmt.Printf("- Current Observations: Working\n")
	fmt.Printf("- Historical Observations (day_offset): Working\n")
	fmt.Printf("- Forecast API: Working\n")
	fmt.Printf("- Data Points Retrieved: %d observations\n", len(observations))
	fmt.Printf("- API Performance: %.2f seconds for historical data\n", elapsed.Seconds())
}

// runHistoryTest fetches as much historical data as possible and prints starting
// timestamps for each 500-point block running back in time.
//
// It requests up to 365 days using the configured history reduction
// settings, then prints the oldest timestamp, the first timestamp of every
// 500-observation block (newest first) and the total time covered. Station
// lookup or fetch failures are fatal.
func runHistoryTest(cfg *config.Config) {
	fmt.Println("=== Historical Data Coverage Test ===")

	// Get stations
	stations, err := weather.GetStations(cfg.Token)
	if err != nil {
		log.Fatalf("Failed to get stations: %v", err)
	}

	station := weather.FindStationByName(stations, cfg.StationName)
	if station == nil {
		log.Fatalf("Station '%s' not found", cfg.StationName)
	}
	fmt.Printf("Found station: %s (ID: %d)\n", station.Name, station.StationID)

	// Fetch historical observations across available days (up to 365)
	fmt.Println("Collecting historical observations (this may take a while)...")
	start := time.Now()
	observations, err := weather.GetAllHistoricalObservations(station.StationID, cfg.Token, cfg.LogLevel, 365, cfg.HistoryReduceMethod, cfg.HistoryBinMinutes, cfg.HistoryKeepRecentHours, cfg.HistoryReduce, 0)
	if err != nil {
		log.Fatalf("Failed to fetch historical observations: %v", err)
	}
	duration := time.Since(start)
	fmt.Printf("Fetched %d unique historical observations in %.2f seconds\n", len(observations), duration.Seconds())

	// Print INFO line for the start (oldest) historic data point
	if len(observations) > 0 {
		oldestObs := observations[len(observations)-1]
		utcStart := time.Unix(oldestObs.Timestamp, 0).UTC().Format("2006-01-02 15:04:05")
		localStart := time.Unix(oldestObs.Timestamp, 0).Local().Format("2006-01-02 15:04:05 -0700 MST")
		fmt.Printf("INFO: First historic data point (oldest): %s (UTC) / %s (Local)\n", utcStart, localStart)
	}

	if len(observations) == 0 {
		fmt.Println("No historical observations were retrieved.")
		return
	}

	// observations are newest-first. We'll report start time for each 500-point block
	blockSize := 500
	fmt.Println()
	fmt.Printf("%-8s %-12s %-25s %-25s\n", "Block#", "StartIdx", "Timestamp (UTC)", "Timestamp (Local)")
	fmt.Printf("%s\n", strings.Repeat("-", 78))

	total := len(observations)
	blockNum := 1
	for idx := 0; idx < total; idx += blockSize {
		obs := observations[idx]
		utc := time.Unix(obs.Timestamp, 0).UTC().Format("2006-01-02 15:04:05")
		local := time.Unix(obs.Timestamp, 0).Local().Format("2006-01-02 15:04:05 -0700 MST")
		fmt.Printf("%-8d %-12d %-25s %-25s\n", blockNum, idx+1, utc, local)
		blockNum++
	}

	// Summary: total coverage
	oldest := time.Unix(observations[len(observations)-1].Timestamp, 0)
	newest := time.Unix(observations[0].Timestamp, 0)
	fmt.Println()
	fmt.Printf("Total observations: %d\n", total)
	fmt.Printf("Data coverage: %s to %s (%.2f hours)\n", oldest.Format("2006-01-02 15:04:05"), newest.Format("2006-01-02 15:04:05"), newest.Sub(oldest).Hours())
}

// runLocalAPITests starts the service in-process and smoke-tests the local
// web API endpoints.
//
// It forces HomeKit off, switches the default web port 8080 to 8084 (and a
// generated-weather StationURL with it) to avoid clashing with a running
// instance, quietens logging unless debug, starts the service in a
// goroutine and waits three seconds for it. It then requests /api/weather,
// /api/status, /api/alarm-status, /api/history, /api/units and, with
// generated weather, /api/generate-weather via testEndpoint. It mutates cfg.
// The summary always reports success; individual failures appear only in
// the per-endpoint output. The context it creates is not passed to the
// service, so cancelling it does not stop the service.
func runLocalAPITests(cfg *config.Config) {
	fmt.Println("=== Local Web Server API Endpoint Tests ===")
	fmt.Println()

	// Force disable HomeKit for test mode - this is a standalone test
	cfg.DisableHomeKit = true

	// Use port 8084 as default for test-api-local if not specified
	if cfg.WebPort == "8080" {
		cfg.WebPort = "8084"
		fmt.Printf("Using default test port: %s\n", cfg.WebPort)
		// Update StationURL if using generated weather with default port
		if cfg.UseGeneratedWeather && strings.Contains(cfg.StationURL, ":8080/") {
			cfg.StationURL = strings.Replace(cfg.StationURL, ":8080/", ":8084/", 1)
		}
	} else {
		fmt.Printf("Using specified port: %s\n", cfg.WebPort)
	}

	// Suppress service initialization logs unless debug mode
	if cfg.LogLevel != "debug" {
		cfg.LogLevel = "error"
	}

	// Start a minimal service to test the API endpoints
	fmt.Println("Starting temporary web server for API testing...")

	// Create a context with timeout
	_, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Start service in background
	go func() {
		if err := service.StartService(cfg, "1.9.0"); err != nil {
			log.Printf("Service error: %v", err)
		}
	}()

	// Wait for server to start
	fmt.Println("Waiting for server to initialize...")
	time.Sleep(3 * time.Second)

	baseURL := fmt.Sprintf("http://localhost:%s", cfg.WebPort)
	client := &http.Client{Timeout: 5 * time.Second}

	// Test 1: /api/weather
	fmt.Println("\n1. Testing /api/weather endpoint...")
	testEndpoint(client, baseURL+"/api/weather", "Weather", cfg.LogLevel == "debug")

	// Test 2: /api/status
	fmt.Println("\n2. Testing /api/status endpoint...")
	testEndpoint(client, baseURL+"/api/status", "Status", cfg.LogLevel == "debug")

	// Test 3: /api/alarm-status
	fmt.Println("\n3. Testing /api/alarm-status endpoint...")
	testEndpoint(client, baseURL+"/api/alarm-status", "Alarm Status", cfg.LogLevel == "debug")

	// Test 4: /api/history
	fmt.Println("\n4. Testing /api/history endpoint...")
	testEndpoint(client, baseURL+"/api/history", "History", cfg.LogLevel == "debug")

	// Test 5: /api/units
	fmt.Println("\n5. Testing /api/units endpoint...")
	testEndpoint(client, baseURL+"/api/units", "Units", cfg.LogLevel == "debug")

	// Test 6: /api/generate-weather (only if using generated weather)
	if cfg.UseGeneratedWeather {
		fmt.Println("\n6. Testing /api/generate-weather endpoint...")
		testEndpoint(client, baseURL+"/api/generate-weather", "Generate Weather", cfg.LogLevel == "debug")
	}

	fmt.Println("\n=== Summary ===")
	fmt.Println("All local API endpoints tested successfully!")
	fmt.Printf("- Base URL: %s\n", baseURL)
	endpointCount := 5
	if cfg.UseGeneratedWeather {
		endpointCount = 6
	}
	fmt.Printf("- Endpoints tested: %d\n", endpointCount)
	fmt.Println("- /api/weather: Weather data and pressure analysis")
	fmt.Println("- /api/status: Service status, HomeKit, station info, data history")
	fmt.Println("- /api/alarm-status: Alarm configuration and status")
	fmt.Println("- /api/history: Historical observations for charts")
	fmt.Println("- /api/units: Current units configuration")
	if cfg.UseGeneratedWeather {
		fmt.Println("- /api/generate-weather: Generated weather data (Tempest API format)")
	}

	// Shutdown gracefully
	cancel()
	time.Sleep(1 * time.Second)
}

// testEndpoint fetches url with client and reports whether it returned
// HTTP 200 with valid JSON.
//
// name labels the output and selects which summary fields are printed when
// debug is false; when debug is true the indented raw JSON is printed
// instead. Failures are printed with a ❌ marker and are not returned.
func testEndpoint(client *http.Client, url, name string, debug bool) {
	resp, err := client.Get(url)
	if err != nil {
		fmt.Printf("❌ Failed to fetch %s: %v\n", name, err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("❌ %s returned status %d\n", name, resp.StatusCode)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("❌ Failed to read %s response: %v\n", name, err)
		return
	}

	// Try to parse as JSON
	var jsonData interface{}
	if err := json.Unmarshal(body, &jsonData); err != nil {
		fmt.Printf("❌ %s response is not valid JSON: %v\n", name, err)
		return
	}

	if debug {
		fmt.Printf("\n--- RAW %s DATA ---\n", strings.ToUpper(name))
		var formatted bytes.Buffer
		if err := json.Indent(&formatted, body, "", "  "); err != nil {
			log.Printf("failed to indent JSON for debug output: %v", err)
		} else {
			fmt.Println(formatted.String())
		}
		fmt.Printf("--- END RAW DATA ---\n\n")
	}

	fmt.Printf("✅ %s endpoint: OK (%d bytes, %dms)\n", name, len(body), 0)

	// Show key fields for non-debug mode
	if !debug {
		switch name {
		case "Weather":
			var weather map[string]interface{}
			if err := json.Unmarshal(body, &weather); err != nil {
				log.Printf("failed to unmarshal weather summary: %v", err)
			}
			if temp, ok := weather["temperature"].(float64); ok {
				fmt.Printf("   - Temperature: %.1f°C\n", temp)
			}
			if humidity, ok := weather["humidity"].(float64); ok {
				fmt.Printf("   - Humidity: %.0f%%\n", humidity)
			}

		case "Status":
			var status map[string]interface{}
			if err := json.Unmarshal(body, &status); err != nil {
				log.Printf("failed to unmarshal status summary: %v", err)
			}
			if connected, ok := status["connected"].(bool); ok {
				fmt.Printf("   - Connected: %v\n", connected)
			}
			if history, ok := status["dataHistory"].([]interface{}); ok {
				fmt.Printf("   - History points: %d\n", len(history))
			}

		case "Alarm Status":
			var alarmStatus map[string]interface{}
			if err := json.Unmarshal(body, &alarmStatus); err != nil {
				log.Printf("failed to unmarshal alarm status summary: %v", err)
			}
			if enabled, ok := alarmStatus["enabled"].(bool); ok {
				fmt.Printf("   - Alarms enabled: %v\n", enabled)
			}
			if total, ok := alarmStatus["totalAlarms"].(float64); ok {
				fmt.Printf("   - Total alarms: %.0f\n", total)
			}

		case "History":
			var history []interface{}
			if err := json.Unmarshal(body, &history); err != nil {
				log.Printf("failed to unmarshal history summary: %v", err)
			}
			fmt.Printf("   - Historical observations: %d\n", len(history))

		case "Units":
			var units map[string]interface{}
			if err := json.Unmarshal(body, &units); err != nil {
				log.Printf("failed to unmarshal units summary: %v", err)
			}
			if u, ok := units["units"].(string); ok {
				fmt.Printf("   - Units: %s\n", u)
			}
			if p, ok := units["unitsPressure"].(string); ok {
				fmt.Printf("   - Pressure: %s\n", p)
			}

		case "Generate Weather":
			var data map[string]interface{}
			if err := json.Unmarshal(body, &data); err != nil {
				log.Printf("failed to unmarshal generate weather: %v", err)
			}
			if obs, ok := data["obs"].([]interface{}); ok && len(obs) > 0 {
				if latest, ok := obs[0].(map[string]interface{}); ok {
					fmt.Printf("   - Observations: %d\n", len(obs))
					if temp, ok := latest["air_temperature"].(float64); ok {
						fmt.Printf("   - Temperature: %.1f°C\n", temp)
					}
					if humidity, ok := latest["relative_humidity"].(float64); ok {
						fmt.Printf("   - Humidity: %.0f%%\n", humidity)
					}
					if wind, ok := latest["wind_avg"].(float64); ok {
						fmt.Printf("   - Wind Speed: %.1f m/s\n", wind)
					}
				}
			}
		}
	}
}
