// Package config provides configuration management for the Tempest HomeKit service.
// It handles command-line flags, environment variables, and HomeKit database operations.
//
// LoadConfig is the entry point: it seeds a Config from environment variables
// (which the caller is expected to have loaded from the .env file first),
// lets command-line flags override them, validates the result, and resolves
// the station elevation. Invalid configuration terminates the process with
// exit status 2 after printing the usage text.
package config

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

// Config holds all configuration parameters for the Tempest HomeKit service.
//
// Defaults and environment variable names are applied by LoadConfig; flag
// values override environment values. Ports are kept as strings because
// they are passed straight into listen addresses; validateConfig checks that
// they are numeric.
type Config struct {
	Token                  string  // WeatherFlow API token (--token, TEMPEST_TOKEN)
	StationName            string  // Tempest station name (--station, TEMPEST_STATION_NAME)
	Pin                    string  // 8-digit HomeKit pairing PIN (--pin, HOMEKIT_PIN; default 00102003)
	LogLevel               string  // Log level: debug, info, warn/warning or error (--loglevel, LOG_LEVEL)
	LogFilter              string  // Filter log messages to only show those containing this string
	WebPort                string  // Web dashboard port, numeric string (--web-port, WEB_PORT; default 8080)
	ClearDB                bool    // Clear the HomeKit database and reset pairing (--cleardb)
	DisableHomeKit         bool    // Disable HomeKit services and run web console only
	DisableWebConsole      bool    // Disable web server (HomeKit only mode)
	DisableAlarms          bool    // Disable alarm initialization and processing
	Sensors                string  // Sensors to enable: "all", "min" or a comma list (see ParseSensorConfig)
	HistoryRead            bool    // Preload history from the Tempest API at startup (--history-read, READ_HISTORY)
	TestAPI                bool    // Test WeatherFlow API endpoints and data points (--test-api)
	TestAPILocal           bool    // Test local web API endpoints and exit
	TestHistory            bool    // Fetch as much historical data as possible and exit
	TestEmail              string  // Send test email to this address and exit
	TestSMS                string  // Send test SMS to this phone number and exit
	TestWebhook            string  // Send test webhook to this URL and exit
	TestConsole            bool    // Send test console notification and exit
	TestSyslog             bool    // Send test syslog notification and exit
	TestOSLog              bool    // Send test oslog notification and exit
	TestEventLog           bool    // Send test eventlog notification and exit
	TestUDP                int     // Listen for UDP broadcasts for N seconds and display received data (default: 120)
	TestHomeKit            bool    // Test HomeKit bridge setup and pairing without starting service
	TestWebStatus          bool    // Test web status scraping and exit
	TestAlarm              string  // Trigger a specific alarm by name for testing
	UseWebStatus           bool    // Enable headless browser scraping of TempestWX status
	UseGeneratedWeather    bool    // Use generated weather data for testing instead of Tempest API
	TestSensorRain         bool    // Test rain sensor with cycling pattern (requires --use-generated-weather)
	TestSensorWind         bool    // Test wind sensor with cycling pattern (requires --use-generated-weather)
	TestSensorTemp         bool    // Test temperature sensor with cycling pattern (requires --use-generated-weather)
	TestSensorHumidity     bool    // Test humidity sensor with cycling pattern (requires --use-generated-weather)
	TestSensorPressure     bool    // Test pressure sensor with cycling pattern (requires --use-generated-weather)
	TestSensorLux          bool    // Test lux sensor with cycling pattern (requires --use-generated-weather)
	TestSensorUV           bool    // Test UV sensor with cycling pattern (requires --use-generated-weather)
	TestSensorLightning    bool    // Test lightning sensor with cycling pattern (requires --use-generated-weather)
	UDPStream              bool    // Listen for UDP broadcasts from local Tempest station
	DisableInternet        bool    // Disable all internet access (no API, no status scraping)
	StationURL             string  // Custom station URL for weather data (overrides Tempest API)
	Elevation              float64 // elevation in meters
	Units                  string  // Units system: imperial, metric, or sae
	UnitsPressure          string  // Pressure units: inHg or mb
	HistoryPoints          int     // Number of data points to store in history (default: 1000, min: 10)
	ChartHistoryHours      int     // Number of hours of history to display in charts (default: 24, 0 = all)
	HistoryReduce          int     // Reduction factor for historical data (average N points into 1)
	HistoryReduceMethod    string  // Reduction method for historical data: timebin, factor, lttb
	HistoryBinMinutes      int     // Bin size in minutes for timebin reduction
	HistoryKeepRecentHours int     // Keep recent N hours at full resolution when reducing history
	Version                bool    // Show version and exit
	// GeneratedWeatherPath is the URL path portion used for the built-in generated
	// weather endpoint. Default: "/api/generate-weather". This can be overridden
	// via the GENERATE_WEATHER_PATH environment variable or the --generate-path flag.
	GeneratedWeatherPath string

	// Alarm configuration
	Alarms         string // Alarm configuration: @filename.json or inline JSON
	AlarmsEdit     string // Alarm editor mode: @filename.json to edit
	AlarmsEditPort string // Port for alarm editor (default: 8081)

	// Webhook listener
	WebhookListener    bool   // Enable webhook listener server (default port: 8082)
	WebhookListenPort  string // Port for webhook listener server (default: 8082)
	WebhookListenerSet bool   // Track if webhook-listener flag was explicitly set
	WebhookPortSet     bool   // Track if webhook-listener-port flag was explicitly set

	// Environment file
	EnvFile string // Custom environment file (default: .env)

	// Status console options
	Status          bool   // Enable curses-based status console
	StatusRefresh   int    // Status refresh interval in seconds (default: 5)
	StatusTimeout   int    // Status timeout in seconds (0 = never, default: 0)
	StatusTheme     string // Color theme name (default: "dark-ocean")
	StatusThemeList bool   // List available themes and exit
}

// customUsage prints a well-formatted help message with grouped flags and examples.
//
// It is installed as flag.Usage by LoadConfig and writes to os.Stderr,
// using a tabwriter to align flag, description and environment variable
// columns. Write errors are logged rather than returned.
func customUsage() {
	// helper to print and handle any write errors (satisfies errcheck)
	safeFprintln := func(w io.Writer, a ...interface{}) {
		if _, err := fmt.Fprintln(w, a...); err != nil {
			log.Printf("usage print error: %v", err)
		}
	}
	// Use tabwriter to create clean aligned columns for flags and env vars
	safeFprintln(os.Stderr, "Tempest HomeKit Bridge - HomeKit integration for WeatherFlow Tempest weather stations")
	safeFprintln(os.Stderr, "")
	safeFprintln(os.Stderr, "USAGE:")
	safeFprintln(os.Stderr, "  tempest-homekit-go [OPTIONS]")

	w := tabwriter.NewWriter(os.Stderr, 0, 8, 2, ' ', 0)

	// Data source options
	safeFprintln(w, "DATA SOURCE OPTIONS:")
	safeFprintln(w, "  --token <string>\tWeatherFlow API token (required for API mode)\tEnv: TEMPEST_TOKEN")
	safeFprintln(w, "  --station <string>\tTempest station name (required for API mode)\tEnv: TEMPEST_STATION_NAME")
	safeFprintln(w, "  --station-url <url>\tCustom station URL (overrides Tempest API)\tEnv: STATION_URL")
	safeFprintln(w, "  --use-generated-weather\tUse simulated weather data for testing (sets generate-path internally)\t")
	safeFprintln(w, "  --udp-stream\tListen for UDP broadcasts from local station (port 50222)\tEnv: UDP_STREAM=true")
	safeFprintln(w, "  --disable-internet\tDisable all internet access (offline mode)\tEnv: DISABLE_INTERNET=true")
	safeFprintln(w, "  --env <file>\tCustom environment file to load (default: .env)\t")
	safeFprintln(w, "  --elevation <value>\tStation elevation (e.g., 903ft, 275m) - auto-detected if omitted\t")
	safeFprintln(w)

	// HomeKit options
	safeFprintln(w, "HOMEKIT OPTIONS:")
	safeFprintln(w, "  --pin <string>\tHomeKit PIN for device pairing (default: \"00102003\")\tEnv: HOMEKIT_PIN")
	safeFprintln(w, "  --sensors <list>\tSensors to enable (default: \"temp,lux,humidity,uv\")\tEnv: SENSORS")
	safeFprintln(w, "  --disable-homekit\tRun web console only (no HomeKit services)\t")
	safeFprintln(w, "  --disable-alarms\tDisable alarm initialization and processing\t")
	safeFprintln(w, "  --cleardb\tClear HomeKit database and reset device pairing\t")
	safeFprintln(w)

	// HISTORY section (dedicated)
	safeFprintln(w, "HISTORY OPTIONS:")
	safeFprintln(w, "  --history <points>\tNumber of data points to store in history (default: 1000, min: 10)\tEnv: HISTORY_POINTS")
	safeFprintln(w, "  --history-read\tPreload historical observations from Tempest API up to HISTORY_POINTS\tEnv: READ_HISTORY")
	safeFprintln(w, "  --history-reduce <factor>\tReduce historical data by averaging N points into 1 (default: 1 = no reduction)\tEnv: HISTORY_REDUCE")
	safeFprintln(w, "  --history-reduce-method <str>\tMethod to reduce historical data: timebin (default), factor, lttb\tEnv: HISTORY_REDUCE_METHOD")
	safeFprintln(w, "  --history-bin-size <minutes>\tBin size in minutes for timebin reduction (default: 10)\tEnv: HISTORY_BIN_MINUTES")
	safeFprintln(w, "  --history-keep-recent-hours <hours>\tKeep recent N hours of data at full resolution (default: 24)\tEnv: HISTORY_KEEP_RECENT_HOURS")
	safeFprintln(w, "  --chart-history <hours>\tNumber of hours of data to show in charts (default: 24, 0=all)\tEnv: CHART_HISTORY_HOURS")
	safeFprintln(w, "  --generate-path <path>\tPath for generated weather endpoint (default: /api/generate-weather)\tEnv: GENERATE_WEATHER_PATH")
	safeFprintln(w)
	safeFprintln(w)

	// Web console and others (shortened for readability)
	safeFprintln(w, "WEB CONSOLE OPTIONS:")
	safeFprintln(w, "  --web-port <port>\tWeb dashboard port (default: \"8080\")\tEnv: WEB_PORT")
	safeFprintln(w, "  --disable-webconsole\tDisable web server (HomeKit only mode)\t")
	safeFprintln(w, "  --use-web-status\tEnable Chrome-based scraping of TempestWX status page\t")
	safeFprintln(w)

	safeFprintln(w, "ALARM & WEBHOOK OPTIONS:")
	safeFprintln(w, "  --alarms <file|json>\tAlarm configuration: @filename.json or inline JSON string\tEnv: ALARMS")
	safeFprintln(w, "  --alarms-edit <file>\tRun alarm editor for specified config file: @filename.json\tEnv: ALARMS_EDIT")
	safeFprintln(w, "  --alarms-edit-port <port>\tPort for alarm editor web UI (default: 8081)\tEnv: ALARMS_EDIT_PORT")
	safeFprintln(w, "  --webhook-listener\tStart webhook listener server (default port: 8082)\tEnv: WEBHOOK_LISTENER")
	safeFprintln(w, "  --webhook-listener-port <port>\tPort for webhook listener server (default: 8082)\tEnv: WEBHOOK_LISTEN_PORT")
	safeFprintln(w)

	safeFprintln(w, "STATUS OPTIONS:")
	safeFprintln(w, "  --status\tEnable curses-based status console (TUI mode)\tEnv: STATUS")
	safeFprintln(w, "  --status-refresh <sec>\tStatus refresh interval in seconds (default: 5)\tEnv: STATUS_REFRESH")
	safeFprintln(w, "  --status-timeout <sec>\tAuto-exit after N seconds (0 = never)\tEnv: STATUS_TIMEOUT")
	safeFprintln(w, "  --status-theme <name>\tColor theme for status console (default: dark-ocean)\tEnv: STATUS_THEME")
	safeFprintln(w, "  --status-theme-list\tList all available color themes and exit\t")
	safeFprintln(w)

	safeFprintln(w, "LOGGING & DEBUG OPTIONS:")
	safeFprintln(w, "  --loglevel <level>\tLog level: error (default), warn/warning, info, debug\tEnv: LOG_LEVEL")
	safeFprintln(w, "  --logfilter <string>\tFilter log messages (case-insensitive substring match)\tEnv: LOG_FILTER")
	safeFprintln(w)

	safeFprintln(w, "TESTING OPTIONS:")
	safeFprintln(w, "  --test-history\tFetch as much historical data as possible and print block start times, then exit\t")
	safeFprintln(w, "  --test-api\tTest WeatherFlow API endpoints and exit\t")
	safeFprintln(w, "  --test-api-local\tTest local web server API endpoints and exit\t")
	safeFprintln(w, "  --test-email <email>\tSend test email to specified address and exit\t")
	safeFprintln(w, "  --test-sms <phone>\tSend test SMS to specified phone number and exit\t")
	safeFprintln(w, "  --test-webhook <url>\tSend test webhook to specified URL and exit\t")
	safeFprintln(w, "  --test-console\tSend test console notification and exit\t")
	safeFprintln(w, "  --test-syslog\tSend test syslog notification and exit\t")
	safeFprintln(w, "  --test-oslog\tSend test oslog notification and exit (macOS only)\t")
	safeFprintln(w, "  --test-eventlog\tSend test eventlog notification and exit (Windows only)\t")
	safeFprintln(w, "  --test-udp [seconds]\tListen for UDP broadcasts for N seconds (default: 120) and exit\t")
	safeFprintln(w, "  --test-homekit\tTest HomeKit bridge setup and pairing info, then exit\t")
	safeFprintln(w, "  --test-web-status\tTest web status scraping from TempestWX and exit\t")
	safeFprintln(w, "  --test-alarm <name>\tTrigger a specific alarm by name for testing and exit\t")
	safeFprintln(w, "  --test-sensor-rain\tRun rain sensor cycling pattern (requires --use-generated-weather)\t")
	safeFprintln(w, "  --test-sensor-wind\tRun wind sensor cycling pattern (requires --use-generated-weather)\t")
	safeFprintln(w, "  --test-sensor-temp\tRun temperature sensor cycling pattern (requires --use-generated-weather)\t")
	safeFprintln(w, "  --test-sensor-humidity\tRun humidity sensor cycling pattern (requires --use-generated-weather)\t")
	safeFprintln(w, "  --test-sensor-pressure\tRun pressure sensor cycling pattern (requires --use-generated-weather)\t")
	safeFprintln(w, "  --test-sensor-lux\tRun lux sensor cycling pattern (requires --use-generated-weather)\t")
	safeFprintln(w, "  --test-sensor-uv\tRun UV sensor cycling pattern (requires --use-generated-weather)\t")
	safeFprintln(w, "  --test-sensor-lightning\tRun lightning sensor cycling pattern (requires --use-generated-weather)\t")
	safeFprintln(w)

	safeFprintln(w, "OTHER OPTIONS:")
	safeFprintln(w, "  --version\tShow version information and exit\t")
	safeFprintln(w, "  --help\tShow this help message\t")

	// Examples header printed directly to stderr for clarity
	if err := w.Flush(); err != nil {
		log.Printf("usage flush error: %v", err)
	}
	safeFprintln(os.Stderr, "")
	safeFprintln(os.Stderr, "EXAMPLES:")
	safeFprintln(os.Stderr, "  # Basic HomeKit bridge with API")
	safeFprintln(os.Stderr, "  tempest-homekit-go --token \"your-token\" --station \"My Station\"")
	safeFprintln(os.Stderr, "For full details, see: https://github.com/BEHRConsulting/tempest-homekit-go")
}

// LoadConfig initializes and returns a new Config struct with values from
// environment variables, command-line flags, and sensible defaults.
//
// It registers all flags on the global flag.CommandLine set and calls
// flag.Parse, so it must be called at most once per process. If validation
// fails it prints the error and usage to stderr and calls os.Exit(2).
//
// After validation it records whether --webhook-listener and
// --webhook-listener-port were given explicitly, and resolves elevation:
// when --elevation is omitted or "auto" (and generated weather is not in
// use) it looks the elevation up from the station's coordinates over the
// network; otherwise it parses the given value. Any failure falls back to
// 275.2 m (903 ft) with a logged warning.
func LoadConfig() *Config {
	cfg := &Config{
		Token:                  getEnvOrDefault("TEMPEST_TOKEN", ""),
		StationName:            getEnvOrDefault("TEMPEST_STATION_NAME", ""),
		Pin:                    getEnvOrDefault("HOMEKIT_PIN", "00102003"),
		LogLevel:               getEnvOrDefault("LOG_LEVEL", "error"),
		LogFilter:              getEnvOrDefault("LOG_FILTER", ""),
		WebPort:                getEnvOrDefault("WEB_PORT", "8080"),
		Sensors:                getEnvOrDefault("SENSORS", "temp,lux,humidity,uv"),
		HistoryRead:            getEnvOrDefault("READ_HISTORY", "") == "true",
		StationURL:             getEnvOrDefault("STATION_URL", ""),
		UDPStream:              getEnvOrDefault("UDP_STREAM", "") == "true",
		DisableInternet:        getEnvOrDefault("DISABLE_INTERNET", "") == "true",
		Elevation:              275.2, // 903ft default elevation in meters
		Units:                  getEnvOrDefault("UNITS", "imperial"),
		UnitsPressure:          getEnvOrDefault("UNITS_PRESSURE", "inHg"),
		HistoryPoints:          parseIntEnv("HISTORY_POINTS", 1000),
		ChartHistoryHours:      parseIntEnv("CHART_HISTORY_HOURS", 24),
		HistoryReduce:          parseIntEnv("HISTORY_REDUCE", 1),
		HistoryReduceMethod:    getEnvOrDefault("HISTORY_REDUCE_METHOD", "timebin"),
		HistoryBinMinutes:      parseIntEnv("HISTORY_BIN_MINUTES", 10),
		HistoryKeepRecentHours: parseIntEnv("HISTORY_KEEP_RECENT_HOURS", 24),
		GeneratedWeatherPath:   getEnvOrDefault("GENERATE_WEATHER_PATH", "/api/generate-weather"),
		Alarms:                 getEnvOrDefault("ALARMS", ""),
		AlarmsEdit:             getEnvOrDefault("ALARMS_EDIT", ""),
		AlarmsEditPort:         getEnvOrDefault("ALARMS_EDIT_PORT", "8081"),
		WebhookListener:        getEnvOrDefault("WEBHOOK_LISTENER", "") == "true",
		WebhookListenPort:      getEnvOrDefault("WEBHOOK_LISTEN_PORT", "8082"),
		EnvFile:                getEnvOrDefault("ENV_FILE", ".env"),
		Status:                 getEnvOrDefault("STATUS", "") == "true",
		StatusRefresh:          parseIntEnv("STATUS_REFRESH", 5),
		StatusTimeout:          parseIntEnv("STATUS_TIMEOUT", 0),
		StatusTheme:            getEnvOrDefault("STATUS_THEME", "dark-ocean"),
	}

	// Set custom usage function
	flag.Usage = customUsage

	var elevationStr string
	var elevationProvided bool
	flag.StringVar(&cfg.Token, "token", cfg.Token, "WeatherFlow API token")
	flag.StringVar(&cfg.StationName, "station", cfg.StationName, "Tempest station name")
	flag.StringVar(&cfg.Pin, "pin", cfg.Pin, "HomeKit PIN")
	flag.StringVar(&cfg.LogLevel, "loglevel", cfg.LogLevel, "Log level (debug, info, error)")
	flag.StringVar(&cfg.LogFilter, "logfilter", cfg.LogFilter, "Filter log messages to only show those containing this string (case-insensitive)")
	flag.StringVar(&cfg.WebPort, "web-port", cfg.WebPort, "Web dashboard port")
	flag.StringVar(&cfg.Sensors, "sensors", cfg.Sensors, "Sensors to enable: 'all', 'min' (temp,humidity,lux), or comma-delimited list (temp/temperature,humidity,lux/light,wind,rain,pressure,uv/uvi,lightning)")
	flag.StringVar(&elevationStr, "elevation", "", "Station elevation (e.g., 903ft, 275m). If not provided, elevation will be auto-detected from coordinates")
	flag.BoolVar(&cfg.ClearDB, "cleardb", false, "Clear HomeKit database and reset device pairing")
	flag.BoolVar(&cfg.DisableHomeKit, "disable-homekit", false, "Disable HomeKit services and run web console only")
	flag.BoolVar(&cfg.DisableAlarms, "disable-alarms", false, "Disable alarm initialization and processing")
	flag.BoolVar(&cfg.HistoryRead, "history-read", cfg.HistoryRead, "Preload historical observations from Tempest API up to HISTORY_POINTS")
	flag.BoolVar(&cfg.TestAPI, "test-api", false, "Test WeatherFlow API endpoints and data points")
	flag.BoolVar(&cfg.TestAPILocal, "test-api-local", false, "Test local web server API endpoints and exit")
	flag.BoolVar(&cfg.TestHistory, "test-history", false, "Fetch as much historical data as possible and print block start times, then exit")
	flag.StringVar(&cfg.TestEmail, "test-email", "", "Send a test email to the specified address and exit")
	flag.StringVar(&cfg.TestSMS, "test-sms", "", "Send a test SMS to the specified phone number (E.164 format) and exit")
	flag.StringVar(&cfg.TestWebhook, "test-webhook", "", "Send a test webhook to the specified URL and exit")
	flag.BoolVar(&cfg.TestConsole, "test-console", false, "Send a test console notification and exit")
	flag.BoolVar(&cfg.TestSyslog, "test-syslog", false, "Send a test syslog notification and exit")
	flag.BoolVar(&cfg.TestOSLog, "test-oslog", false, "Send a test oslog notification and exit (macOS only)")
	flag.BoolVar(&cfg.TestEventLog, "test-eventlog", false, "Send a test eventlog notification and exit (Windows only)")
	flag.IntVar(&cfg.TestUDP, "test-udp", 0, "Listen for UDP broadcasts for N seconds (default: 120) and exit")
	flag.BoolVar(&cfg.TestHomeKit, "test-homekit", false, "Test HomeKit bridge setup and pairing info, then exit")
	flag.BoolVar(&cfg.TestWebStatus, "test-web-status", false, "Test web status scraping from TempestWX and exit")
	flag.StringVar(&cfg.TestAlarm, "test-alarm", "", "Trigger a specific alarm by name for testing and exit")
	flag.BoolVar(&cfg.UseWebStatus, "use-web-status", false, "Enable headless browser scraping of TempestWX status page every 15 minutes")
	flag.StringVar(&cfg.StationURL, "station-url", cfg.StationURL, "Custom station URL for weather data (e.g., http://localhost:8080/api/generate-weather). Overrides Tempest API. Can also be set via STATION_URL environment variable")
	flag.BoolVar(&cfg.UseGeneratedWeather, "use-generated-weather", false, "Use generated weather data for UI testing instead of Tempest API")
	flag.BoolVar(&cfg.UDPStream, "udp-stream", cfg.UDPStream, "Listen for UDP broadcasts from local Tempest station (port 50222) for offline operation. Can also be set via UDP_STREAM environment variable")
	flag.BoolVar(&cfg.DisableInternet, "disable-internet", cfg.DisableInternet, "Disable all internet access (no WeatherFlow API calls, no status scraping). Requires --udp-stream or --use-generated-weather. Can also be set via DISABLE_INTERNET environment variable")
	flag.BoolVar(&cfg.DisableWebConsole, "disable-webconsole", false, "Disable web server (HomeKit only mode)")
	flag.StringVar(&cfg.Units, "units", cfg.Units, "Units system: imperial (default), metric, or sae. Can also be set via UNITS environment variable")
	flag.StringVar(&cfg.UnitsPressure, "units-pressure", cfg.UnitsPressure, "Pressure units: inHg (default) or mb. Can also be set via UNITS_PRESSURE environment variable")
	flag.IntVar(&cfg.HistoryPoints, "history", cfg.HistoryPoints, "Number of data points to store in history (default: 1000, min: 10). Can also be set via HISTORY_POINTS environment variable")
	flag.IntVar(&cfg.HistoryReduce, "history-reduce", cfg.HistoryReduce, "Reduce historical data by averaging N points into 1 (default: 1 = no reduction)")
	flag.StringVar(&cfg.HistoryReduceMethod, "history-reduce-method", cfg.HistoryReduceMethod, "Method to reduce historical data: timebin (default), factor, lttb")
	flag.IntVar(&cfg.HistoryBinMinutes, "history-bin-size", cfg.HistoryBinMinutes, "Bin size in minutes for timebin reduction (default: 10)")
	flag.IntVar(&cfg.HistoryKeepRecentHours, "history-keep-recent-hours", cfg.HistoryKeepRecentHours, "Keep recent N hours at full resolution when reducing history (default: 24)")
	flag.IntVar(&cfg.ChartHistoryHours, "chart-history", cfg.ChartHistoryHours, "Number of hours of data to display in charts (default: 24, 0=all). Can also be set via CHART_HISTORY_HOURS environment variable")
	flag.StringVar(&cfg.GeneratedWeatherPath, "generate-path", cfg.GeneratedWeatherPath, "Path for generated weather endpoint (default: /api/generate-weather). Can also be set via GENERATE_WEATHER_PATH environment variable")
	flag.StringVar(&cfg.Alarms, "alarms", cfg.Alarms, "Alarm configuration: @filename.json or inline JSON string")
	flag.StringVar(&cfg.AlarmsEdit, "alarms-edit", cfg.AlarmsEdit, "Run alarm editor for specified config file: @filename.json")
	flag.StringVar(&cfg.AlarmsEditPort, "alarms-edit-port", cfg.AlarmsEditPort, "Port for alarm editor web UI (default: 8081)")
	flag.BoolVar(&cfg.WebhookListener, "webhook-listener", cfg.WebhookListener, "Start webhook listener server (default port: 8082)")
	flag.StringVar(&cfg.WebhookListenPort, "webhook-listener-port", cfg.WebhookListenPort, "Port for webhook listener server (default: 8082)")
	flag.StringVar(&cfg.EnvFile, "env", cfg.EnvFile, "Custom environment file to load (default: .env)")
	flag.BoolVar(&cfg.Status, "status", cfg.Status, "Enable curses-based status console (TUI mode)")
	flag.IntVar(&cfg.StatusRefresh, "status-refresh", cfg.StatusRefresh, "Status refresh interval in seconds (default: 5)")
	flag.IntVar(&cfg.StatusTimeout, "status-timeout", cfg.StatusTimeout, "Auto-exit after N seconds (0 = never, default: 0)")
	flag.StringVar(&cfg.StatusTheme, "status-theme", cfg.StatusTheme, "Color theme for status console (default: dark-ocean)")
	flag.BoolVar(&cfg.StatusThemeList, "status-theme-list", false, "List all available color themes and exit")
	flag.BoolVar(&cfg.Version, "version", false, "Show version information and exit")
	flag.BoolVar(&cfg.TestSensorRain, "test-sensor-rain", false, "Test rain sensor with cycling pattern")
	flag.BoolVar(&cfg.TestSensorWind, "test-sensor-wind", false, "Test wind sensor with cycling pattern")
	flag.BoolVar(&cfg.TestSensorTemp, "test-sensor-temp", false, "Test temperature sensor with cycling pattern")
	flag.BoolVar(&cfg.TestSensorHumidity, "test-sensor-humidity", false, "Test humidity sensor with cycling pattern")
	flag.BoolVar(&cfg.TestSensorPressure, "test-sensor-pressure", false, "Test pressure sensor with cycling pattern")
	flag.BoolVar(&cfg.TestSensorLux, "test-sensor-lux", false, "Test lux sensor with cycling pattern")
	flag.BoolVar(&cfg.TestSensorUV, "test-sensor-uv", false, "Test UV sensor with cycling pattern")
	flag.BoolVar(&cfg.TestSensorLightning, "test-sensor-lightning", false, "Test lightning sensor with cycling pattern")

	// Parse flags but check if elevation was actually provided
	flag.Parse()

	// Handle station URL configuration. If a StationURL is provided and
	// generated weather is not requested, we leave it as-is. Do not set
	// StationURL when using --use-generated-weather so the generated data
	// source is used instead of an HTTP API.

	// Validate command line arguments
	if err := validateConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n\n", err)
		flag.Usage()
		os.Exit(2)
	}

	// Check if elevation was provided by user
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "elevation" {
			elevationProvided = true
		}
		if f.Name == "webhook-listener" {
			cfg.WebhookListenerSet = true
		}
		if f.Name == "webhook-listener-port" {
			cfg.WebhookPortSet = true
		}
	})

	// Handle elevation configuration - auto lookup by default
	if !elevationProvided || strings.ToLower(elevationStr) == "auto" {
		// Skip station elevation lookup if using generated weather - elevation will be set later from generated location
		if !cfg.UseGeneratedWeather {
			if elevation, err := lookupStationElevation(cfg.Token, cfg.StationName); err != nil {
				log.Printf("Warning: Failed to lookup elevation automatically: %v", err)
				log.Printf("INFO: Using fallback elevation 903ft (275.2m)")
			} else {
				cfg.Elevation = elevation
				// Don't log here - will be logged later in main.go after logger is set up
			}
		}
		// For generated weather, elevation will be set by the service from the generated location
	} else {
		// Parse manually provided elevation with units
		if elevation, err := parseElevation(elevationStr); err != nil {
			log.Printf("Warning: Invalid elevation format '%s', using fallback 903ft (275.2m): %v", elevationStr, err)
		} else {
			cfg.Elevation = elevation
			log.Printf("INFO: Using specified elevation: %.1f meters (%.0f feet)", elevation, elevation*3.28084)
		}
	}

	return cfg
}

// validateConfig validates command line arguments and returns an error if invalid.
//
// It first fills defaults into zero-valued fields (Units, UnitsPressure,
// HistoryPoints, ChartHistoryHours) so that programmatically built Configs
// validate, which means it mutates cfg. It then checks the log level,
// sensor list, numeric ports, 8-digit PIN, the token and station name
// required in WeatherFlow API mode, offline-mode conflicts, mutually
// exclusive disable flags, that test-sensor flags have generated weather,
// units, and history limits. It returns the first problem found, phrased
// for display to the user, or nil.
func validateConfig(cfg *Config) error {
	// Ensure sensible defaults for fields when Config structs are created programmatically
	// Some tests construct Config with empty values and expect sensible defaults to be applied.
	if strings.TrimSpace(cfg.Units) == "" {
		cfg.Units = "imperial"
	}
	if strings.TrimSpace(cfg.UnitsPressure) == "" {
		cfg.UnitsPressure = "inHg"
	}
	// Default history/chart values when zero-valued Config is used in tests or programmatically
	if cfg.HistoryPoints == 0 {
		cfg.HistoryPoints = 1000
	}
	if cfg.ChartHistoryHours == 0 {
		cfg.ChartHistoryHours = 24
	}
	// Validate log level
	validLogLevels := []string{"debug", "info", "warn", "warning", "error"}
	validLevel := false
	for _, level := range validLogLevels {
		if cfg.LogLevel == level {
			validLevel = true
			break
		}
	}
	if !validLevel {
		return fmt.Errorf("invalid log level '%s'. Valid options: debug, info, warn/warning, error", cfg.LogLevel)
	}

	// Validate sensor configuration by testing parsing
	if cfg.Sensors != "" {
		// Test if sensor config is valid by attempting to parse it
		// This will help catch invalid sensor names early
		validSensorNames := []string{"temp", "temperature", "humidity", "lux", "light", "wind", "rain", "pressure", "uv", "uvi", "lightning"}
		validPresets := []string{"all", "min"}

		// Check if it's a preset
		isPreset := false
		for _, preset := range validPresets {
			if cfg.Sensors == preset {
				isPreset = true
				break
			}
		}

		if !isPreset {
			// Parse comma-separated sensor list
			sensors := strings.Split(strings.ToLower(cfg.Sensors), ",")
			for _, sensor := range sensors {
				sensor = strings.TrimSpace(sensor)
				if sensor == "" {
					continue
				}
				valid := false
				for _, validName := range validSensorNames {
					if sensor == validName {
						valid = true
						break
					}
				}
				if !valid {
					return fmt.Errorf("invalid sensor '%s'. Valid sensors: %s. Valid presets: %s",
						sensor, strings.Join(validSensorNames, ", "), strings.Join(validPresets, ", "))
				}
			}
		}
	}

	// Validate web port is numeric
	if _, err := strconv.Atoi(cfg.WebPort); err != nil {
		return fmt.Errorf("invalid web port '%s'. Port must be a number", cfg.WebPort)
	}

	// Validate webhook listen port is numeric
	if cfg.WebhookListenPort != "" {
		if _, err := strconv.Atoi(cfg.WebhookListenPort); err != nil {
			return fmt.Errorf("invalid webhook listen port '%s'. Port must be a number", cfg.WebhookListenPort)
		}
	}

	// Validate HomeKit PIN format (8 digits)
	if len(cfg.Pin) != 8 {
		return fmt.Errorf("invalid HomeKit PIN '%s'. PIN must be exactly 8 digits", cfg.Pin)
	}
	if _, err := strconv.Atoi(cfg.Pin); err != nil {
		return fmt.Errorf("invalid HomeKit PIN '%s'. PIN must contain only digits", cfg.Pin)
	}

	// Validate required fields for WeatherFlow API mode
	// The WeatherFlow API token is required only when using the WeatherFlow API as the
	// data source. If a custom station URL is provided via --station-url, the
	// --use-generated-weather flag is set, or --udp-stream is enabled, a WeatherFlow token is not necessary.
	// Also skip token requirement for alarm editor mode.
	usingWeatherFlowAPI := cfg.StationURL == "" && !cfg.UseGeneratedWeather && !cfg.UDPStream && cfg.AlarmsEdit == ""

	if usingWeatherFlowAPI {
		if cfg.Token == "" {
			return fmt.Errorf("WeatherFlow API token is required when using the WeatherFlow API as the data source. Set via --token flag or TEMPEST_TOKEN environment variable, or use --station-url/--use-generated-weather/--udp-stream for token-less modes")
		}
		if cfg.StationName == "" {
			return fmt.Errorf("both --token and --station are required when using the WeatherFlow API. Set station via --station flag or TEMPEST_STATION_NAME environment variable")
		}
	}

	// Validate DisableInternet mode requires a local data source (UDP or Generated Weather)
	if cfg.DisableInternet && !cfg.UDPStream && !cfg.UseGeneratedWeather {
		return fmt.Errorf("--disable-internet mode requires --udp-stream or --use-generated-weather (need a local data source)")
	}

	// Validate DisableInternet mode is incompatible with internet-dependent features
	if cfg.DisableInternet {
		if cfg.UseWebStatus {
			return fmt.Errorf("--use-web-status cannot be used with --disable-internet (requires internet access)")
		}
		if cfg.HistoryRead {
			return fmt.Errorf("--history-read cannot be used with --disable-internet (requires WeatherFlow API access)")
		}
	}

	// Validate DisableHomeKit and DisableWebConsole are mutually exclusive
	if cfg.DisableHomeKit && cfg.DisableWebConsole {
		return fmt.Errorf("--disable-homekit and --disable-webconsole cannot be used together (would disable everything)")
	}

	// Test sensor flags require --use-generated-weather
	if (cfg.TestSensorRain || cfg.TestSensorWind || cfg.TestSensorTemp || cfg.TestSensorHumidity ||
		cfg.TestSensorPressure || cfg.TestSensorLux || cfg.TestSensorUV || cfg.TestSensorLightning) &&
		!cfg.UseGeneratedWeather {
		return fmt.Errorf("test sensor flags require --use-generated-weather")
	}

	// Station name is required for non-alarm-editor modes (already checked above for API mode)
	if cfg.StationName == "" && cfg.AlarmsEdit == "" && !usingWeatherFlowAPI {
		return fmt.Errorf("station name is required. Set via --station flag or TEMPEST_STATION_NAME environment variable")
	}

	// Validate units
	validUnits := []string{"imperial", "metric", "sae"}
	validUnit := false
	for _, unit := range validUnits {
		if cfg.Units == unit {
			validUnit = true
			break
		}
	}
	if !validUnit {
		return fmt.Errorf("invalid units '%s'. Valid options: imperial, metric, sae", cfg.Units)
	}

	// Validate pressure units
	validPressureUnits := []string{"inHg", "mb"}
	validPressureUnit := false
	for _, unit := range validPressureUnits {
		if cfg.UnitsPressure == unit {
			validPressureUnit = true
			break
		}
	}
	if !validPressureUnit {
		return fmt.Errorf("invalid pressure units '%s'. Valid options: inHg, mb", cfg.UnitsPressure)
	}

	// Validate history points
	if cfg.HistoryPoints < 10 {
		return fmt.Errorf("history points must be at least 10 (got %d)", cfg.HistoryPoints)
	}
	// Validate chart history hours (0 means all, so only check if positive)
	if cfg.ChartHistoryHours < 0 {
		return fmt.Errorf("chart history hours must be 0 (all data) or positive (got %d)", cfg.ChartHistoryHours)
	}

	return nil
}

// ClearDatabase removes all files in the HomeKit database directory.
//
// dbPath is the directory holding HomeKit pairing state. A missing directory
// is not an error. Entries that cannot be removed (for example non-empty
// subdirectories) are logged as warnings and skipped. It returns an error
// only if the directory listing glob fails.
func ClearDatabase(dbPath string) error {
	log.Printf("INFO: Clearing HomeKit database at: %s", dbPath)

	// Check if directory exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		log.Printf("INFO: Database directory does not exist: %s", dbPath)
		return nil
	}

	// Remove all files in the directory
	files, err := filepath.Glob(filepath.Join(dbPath, "*"))
	if err != nil {
		return err
	}

	for _, file := range files {
		if err := os.Remove(file); err != nil {
			log.Printf("Warning: Failed to remove %s: %v", file, err)
		} else {
			log.Printf("INFO: Removed: %s", filepath.Base(file))
		}
	}

	log.Printf("INFO: HomeKit database cleared successfully")
	return nil
}

// SensorConfig represents which sensors should be enabled.
//
// Each field is true when the matching HomeKit sensor should be exposed.
type SensorConfig struct {
	Temperature bool // Air temperature sensor
	Humidity    bool // Relative humidity sensor
	Light       bool // Ambient light (lux) sensor
	Wind        bool // Wind speed sensor
	Rain        bool // Rain sensor
	Pressure    bool // Barometric pressure sensor
	UV          bool // UV index sensor
	Lightning   bool // Lightning sensor
}

// ParseSensorConfig parses the sensor configuration string and returns a SensorConfig
// with appropriate sensor types enabled based on the input string.
// Supported values: "all", "min", or comma-separated sensor names.
//
// Matching is case-insensitive and ignores surrounding spaces. "min" enables
// temperature, humidity and light. Accepted names and aliases are temp or
// temperature, humidity, light or lux, wind, rain, pressure, uv or uvi, and
// lightning. Unknown names are ignored here; validateConfig rejects them.
func ParseSensorConfig(sensorsFlag string) SensorConfig {
	switch strings.ToLower(sensorsFlag) {
	case "all":
		return SensorConfig{
			Temperature: true,
			Humidity:    true,
			Light:       true,
			Wind:        true,
			Rain:        true,
			Pressure:    true,
			UV:          true,
			Lightning:   true,
		}
	case "min":
		return SensorConfig{
			Temperature: true,
			Humidity:    true,
			Light:       true,
			// Minimal sensors: temperature, humidity, and light for basic weather monitoring
		}
	default:
		// Parse comma-delimited sensor list
		sensors := strings.Split(strings.ToLower(sensorsFlag), ",")
		config := SensorConfig{}
		for _, sensor := range sensors {
			sensor = strings.TrimSpace(sensor)
			switch sensor {
			case "temp", "temperature":
				config.Temperature = true
			case "humidity":
				config.Humidity = true
			case "light", "lux":
				config.Light = true
			case "wind":
				config.Wind = true
			case "rain":
				config.Rain = true
			case "pressure":
				config.Pressure = true
			case "uv", "uvi":
				config.UV = true
			case "lightning":
				config.Lightning = true
			}
		}
		return config
	}
}

// StationLocation represents station coordinates from WeatherFlow API.
// It is not currently used by this package.
type StationLocation struct {
	StationID int     `json:"station_id"`          // WeatherFlow station ID
	Name      string  `json:"name"`                // Station name
	Latitude  float64 `json:"latitude"`            // Latitude in decimal degrees
	Longitude float64 `json:"longitude"`           // Longitude in decimal degrees
	Timezone  string  `json:"timezone"`            // IANA time zone name
	Elevation float64 `json:"elevation,omitempty"` // May be provided directly
}

// ElevationResponse represents response from elevation API
// (api.open-elevation.com lookup endpoint).
type ElevationResponse struct {
	// Results holds one entry per requested location; Elevation is in metres.
	Results []struct {
		Elevation float64 `json:"elevation"`
	} `json:"results"`
}

// lookupStationElevation attempts to get elevation from station coordinates.
//
// It resolves coordinates for stationName with getStationCoordinates, then
// queries the Open Elevation API. It returns the elevation in metres, or an
// error describing which step failed.
func lookupStationElevation(token, stationName string) (float64, error) {
	// First try to get station coordinates from WeatherFlow API
	lat, lon, err := getStationCoordinates(token, stationName)
	if err != nil {
		return 0, fmt.Errorf("failed to get station coordinates: %v", err)
	}

	// Then lookup elevation from coordinates
	elevation, err := getElevationFromCoordinates(lat, lon)
	if err != nil {
		return 0, fmt.Errorf("failed to lookup elevation for coordinates (%.4f, %.4f): %v", lat, lon, err)
	}

	return elevation, nil
}

// getStationCoordinates fetches station coordinates from WeatherFlow API.
//
// Because fetchWeatherFlowStationCoords is not implemented yet, in practice
// it falls back to a small table of known city names matched exactly
// against stationName. It returns latitude and longitude in decimal degrees,
// or an error when the station is not in the table.
func getStationCoordinates(token, stationName string) (lat, lon float64, err error) {
	// First try to get actual station coordinates from WeatherFlow API
	if coords, err := fetchWeatherFlowStationCoords(token, stationName); err == nil {
		return coords[0], coords[1], nil
	}

	// Fallback to known coordinates for common locations
	knownLocations := map[string][2]float64{
		"Chino Hills": {33.9898, -117.7326},
		"Los Angeles": {34.0522, -118.2437},
		"San Diego":   {32.7157, -117.1611},
		"Phoenix":     {33.4484, -112.0740},
		"Denver":      {39.7392, -104.9903},
		"Seattle":     {47.6062, -122.3321},
		"Portland":    {45.5152, -122.6784},
		"Austin":      {30.2672, -97.7431},
		"Dallas":      {32.7767, -96.7970},
		"Miami":       {25.7617, -80.1918},
	}

	if coords, found := knownLocations[stationName]; found {
		return coords[0], coords[1], nil
	}

	return 0, 0, fmt.Errorf("coordinates not available for station '%s' (consider adding coordinates to known locations)", stationName)
}

// fetchWeatherFlowStationCoords attempts to get coordinates from WeatherFlow API.
//
// It is a stub: it ignores its arguments and always returns an error so that
// callers fall back to known locations. On success it would return
// [latitude, longitude].
func fetchWeatherFlowStationCoords(_token, _stationName string) (coords [2]float64, err error) {
	// Explicitly ignore unused parameters to satisfy linter
	_ = _token
	_ = _stationName

	// This would query the WeatherFlow API stations endpoint for detailed station info
	// The API might have an endpoint like: /stations/:station_id/details that includes lat/lon
	// For now, we return an error to fall back to known locations

	// TODO: Implement actual WeatherFlow API call when station details endpoint is available
	// Example implementation would be:
	/*
		url := fmt.Sprintf("https://swd.weatherflow.com/swd/rest/stations/%s/details?token=%s", stationID, token)
		resp, err := http.Get(url)
		if err != nil {
			return coords, err
		}
		defer func() { _ = resp.Body.Close() }()

		var stationDetails StationDetailsResponse
		if err := json.NewDecoder(resp.Body).Decode(&stationDetails); err != nil {
			return coords, err
		}

		if len(stationDetails.Stations) > 0 {
			station := stationDetails.Stations[0]
			coords[0] = station.Latitude
			coords[1] = station.Longitude
			return coords, nil
		}
	*/

	return coords, fmt.Errorf("WeatherFlow station coordinates API not implemented")
}

// getElevationFromCoordinates uses Open Elevation API to get elevation.
//
// lat and lon are decimal degrees. It makes one HTTP request with a 10 second
// timeout and returns the elevation in metres. It returns an error on
// network failure, a non-200 status, an unreadable or unparsable body, or an
// empty result list.
func getElevationFromCoordinates(lat, lon float64) (float64, error) {
	// Use Open Elevation API (free, no API key required)
	url := fmt.Sprintf("https://api.open-elevation.com/api/v1/lookup?locations=%.4f,%.4f", lat, lon)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return 0, fmt.Errorf("elevation API request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("elevation API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read elevation API response: %v", err)
	}

	var elevResp ElevationResponse
	if err := json.Unmarshal(body, &elevResp); err != nil {
		return 0, fmt.Errorf("failed to parse elevation API response: %v", err)
	}

	if len(elevResp.Results) == 0 {
		return 0, fmt.Errorf("no elevation data returned")
	}

	return elevResp.Results[0].Elevation, nil
}

// parseElevation parses elevation string with units (e.g., "903ft", "275m") and returns meters.
//
// Input is case-insensitive and trimmed; a bare number is taken as metres.
// It returns the strconv parse error for malformed numbers, and an error if
// the result is outside Earth's surface range of about -430 m to 8848 m.
func parseElevation(elevationStr string) (float64, error) {
	elevationStr = strings.TrimSpace(strings.ToLower(elevationStr))

	var meters float64
	var err error

	if strings.HasSuffix(elevationStr, "ft") {
		// Parse feet and convert to meters
		valueStr := strings.TrimSuffix(elevationStr, "ft")
		feet, parseErr := strconv.ParseFloat(valueStr, 64)
		if parseErr != nil {
			return 0, parseErr
		}
		meters = feet * 0.3048 // 1 foot = 0.3048 meters
	} else if strings.HasSuffix(elevationStr, "m") {
		// Parse meters directly
		valueStr := strings.TrimSuffix(elevationStr, "m")
		meters, err = strconv.ParseFloat(valueStr, 64)
		if err != nil {
			return 0, err
		}
	} else {
		// Try to parse as number without unit, assume meters
		meters, err = strconv.ParseFloat(elevationStr, 64)
		if err != nil {
			return 0, err
		}
	}

	// Validate elevation range: -1411ft to 29029ft (-430m to 8848m)
	// Dead Sea area is the lowest at -430m, Mount Everest is highest at 8848m
	// Add small tolerance for floating point precision
	const minElevationMeters = -430.1 // -1411 feet with tolerance
	const maxElevationMeters = 8848.1 // 29029 feet with tolerance

	if meters < minElevationMeters {
		return 0, fmt.Errorf("elevation %.1fm is below Earth's lowest point (%.1fm, Dead Sea area)", meters, minElevationMeters)
	}
	if meters > maxElevationMeters {
		return 0, fmt.Errorf("elevation %.1fm is above Earth's highest point (%.1fm, Mount Everest)", meters, maxElevationMeters)
	}

	return meters, nil
}

// getEnvOrDefault returns the value of environment variable key, or
// defaultValue when it is unset or empty.
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// parseIntEnv parses an integer from environment variable or returns default.
//
// defaultValue is returned when key is unset, empty or not a valid integer;
// invalid values are silently ignored.
func parseIntEnv(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}
