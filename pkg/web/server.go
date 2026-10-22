// Package web provides an HTTP server and web dashboard for monitoring Tempest weather data.
// It serves both API endpoints for weather data and a complete web interface with charts and controls.
//
// The central type is WebServer. The main program pushes observations, forecasts and
// HomeKit/UDP/data-source status into it through its Update*/Set* methods; HTTP handlers
// read that state under a RWMutex and render it as JSON (/api/weather, /api/status,
// /api/history, /api/alarm-status, /api/units) or HTML (the dashboard at "/" and the
// per-chart popout at /chart/<type>). Static assets are served from ./pkg/web/static/
// relative to the working directory.
//
// Files named testhelpers_ui.go and *_test.go in this package hold headless-browser UI
// test helpers and tests.
package web

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"tempest-homekit-go/pkg/alarm"
	"tempest-homekit-go/pkg/logger"
	"time"

	"tempest-homekit-go/pkg/generator"
	"tempest-homekit-go/pkg/udp"
	"tempest-homekit-go/pkg/weather"
)

// AlarmManagerInterface defines the methods we need from the alarm manager.
//
// It is the read-only view handleAlarmStatusAPI uses to build /api/alarm-status, kept as
// an interface so this package does not depend on the concrete alarm manager and tests can
// supply a fake.
//
// Methods:
//   - GetConfig returns the currently loaded alarm configuration.
//   - GetAlarmCount returns the total number of configured alarms.
//   - GetEnabledAlarmCount returns the number of alarms with Enabled set.
//   - GetConfigPath returns the path the alarm configuration was loaded from.
//   - GetLastLoadTime returns when the configuration was last (re)loaded; zero if never.
//   - GetLocation returns the station coordinates used to evaluate sunrise/sunset
//     schedules.
type AlarmManagerInterface interface {
	GetConfig() *alarm.AlarmConfig
	GetAlarmCount() int
	GetEnabledAlarmCount() int
	GetConfigPath() string
	GetLastLoadTime() time.Time
	GetLocation() (latitude, longitude float64)
}

// WebServer provides HTTP endpoints and a web dashboard for weather monitoring.
// It manages weather data, serves API endpoints, and provides real-time updates.
//
// All mutable state is guarded by mu; exported setters take the write lock and handlers
// take the read lock. Fields without an inline comment:
//   - port is the listen port (without colon); server is the underlying http.Server.
//   - weatherData is the most recent observation; nil until the first UpdateWeather.
//   - forecastData is the latest forecast passed to UpdateForecast.
//   - homekitStatus is merged key-by-key by UpdateHomeKitStatus and returned verbatim in
//     /api/status.
//   - dataHistory holds observations sorted ascending by Timestamp, trimmed to the newest
//     maxHistorySize entries by UpdateWeather.
//   - stationName is the display name set by SetStationName.
//   - startTime is when the server was constructed, used for the reported uptime.
//   - historicalDataLoaded and historicalDataCount are set by SetHistoricalDataStatus.
//   - historyLoadingProgress is the preload progress reported in /api/status, set by
//     SetHistoryLoadingProgress and cleared by SetHistoryLoadingComplete.
//   - mu guards every field above.
type WebServer struct {
	port                   string
	server                 *http.Server
	weatherData            *weather.Observation
	forecastData           *weather.ForecastResponse
	homekitStatus          map[string]interface{}
	dataHistory            []weather.Observation
	maxHistorySize         int
	chartHistoryHours      int // hours of data to show in charts (0 = all)
	stationName            string
	stationURL             string                // station URL for weather data
	stationID              int                   // station ID for TempestWX status scraping
	elevation              float64               // elevation in meters
	units                  string                // units system: imperial, metric, or sae
	unitsPressure          string                // pressure units: inHg or mb
	logLevel               string                // log level for filtering debug messages
	alarmManager           AlarmManagerInterface // alarm manager for status display
	alarmConfig            string                // alarm configuration path or content
	disableAlarms          bool                  // whether alarms are disabled via --disable-alarms flag
	startTime              time.Time
	historicalDataLoaded   bool
	historicalDataCount    int
	generatedWeather       *GeneratedWeatherInfo     // info about generated weather data
	weatherGenerator       WeatherGeneratorInterface // weather generator for regeneration
	historyLoadingProgress struct {
		isLoading   bool
		currentStep int
		totalSteps  int
		description string
	}
	statusManager    *weather.StatusManager    // Manages periodic status scraping
	version          string                    // application version
	udpListener      *udp.UDPListener          // UDP listener for local station monitoring
	dataSourceStatus *weather.DataSourceStatus // Unified data source status
	mu               sync.RWMutex
}

// logDebug prints debug messages only if log level is debug.
//
// The filter uses the server's own logLevel in addition to the global logger's level.
func (ws *WebServer) logDebug(format string, v ...interface{}) {
	if ws.logLevel == "debug" {
		logger.Debug(format, v...)
	}
}

// logInfo prints info and debug messages only if log level is debug or info.
func (ws *WebServer) logInfo(format string, v ...interface{}) {
	if ws.logLevel == "debug" || ws.logLevel == "info" {
		logger.Info(format, v...)
	}
}

// logError always prints error messages
// nolint:deadcode,unused // intentionally kept for future use and referenced via no-op assignments
func (ws *WebServer) logError(format string, v ...interface{}) {
	logger.Error(format, v...)
}

// _ references logError at package scope so staticcheck/gopls don't report it as unused.
//
// Reference logError at package scope so staticcheck/gopls don't report it as unused.
// The method is intentionally available for future use; keeping a reference here
// avoids IDE noise while preserving the method for callers.
var _ = (*WebServer).logError

// _ is an extra no-op closure to ensure static analyzers treat logError as referenced.
var _ = func() interface{} {
	var ws *WebServer
	// take method value; safe even with nil receiver
	_ = ws.logError
	return nil
}()

// WeatherResponse is the JSON body of /api/weather and the element type of
// StatusResponse.DataHistory.
//
// Numeric fields are passed through from weather.Observation in the units the data source
// supplies (temperature in Celsius, pressure in mb); conversion to the user's display
// units happens in the browser. Fields:
//   - RainAccum is the rain since the previous sample, never negative; RainRate is that
//     amount scaled to mm/hr; RainDailyTotal is the day's total (from the observation for
//     /api/weather, computed from history for /api/status entries).
//   - Pressure is station pressure; SeaLevelPressure, PressureCondition, PressureTrend and
//     WeatherForecast are derived from it and are only filled for /api/weather.
//   - LastUpdate is the observation time in RFC 3339.
//   - UnitHints names the units of the numeric fields; ObservationCount and MaxHistorySize
//     report history fill. All three are omitted in history entries.
type WeatherResponse struct {
	Temperature          float64           `json:"temperature"`
	Humidity             float64           `json:"humidity"`
	WindSpeed            float64           `json:"windSpeed"`
	WindGust             float64           `json:"windGust"`
	WindDirection        float64           `json:"windDirection"`
	RainAccum            float64           `json:"rainAccum"`
	RainRate             float64           `json:"rainRate"` // Rain intensity in mm/hr
	RainDailyTotal       float64           `json:"rainDailyTotal"`
	PrecipitationType    int               `json:"precipitationType"`
	Pressure             float64           `json:"pressure"`
	SeaLevelPressure     float64           `json:"seaLevelPressure"`
	PressureCondition    string            `json:"pressure_condition"`
	PressureTrend        string            `json:"pressure_trend"`
	WeatherForecast      string            `json:"weather_forecast"`
	Illuminance          float64           `json:"illuminance"`
	UV                   int               `json:"uv"`
	Battery              float64           `json:"battery"`
	LightningStrikeAvg   float64           `json:"lightningStrikeAvg"`
	LightningStrikeCount int               `json:"lightningStrikeCount"`
	LastUpdate           string            `json:"lastUpdate"`
	UnitHints            map[string]string `json:"unitHints,omitempty"`
	ObservationCount     int               `json:"observationCount,omitempty"`
	MaxHistorySize       int               `json:"maxHistorySize,omitempty"`
}

// StatusResponse is the JSON body of /api/status.
//
// It combines connection state, uptime (formatted "XhYmZs"), the full chart history,
// history-preload progress, forecast, TempestWX station status, generated-weather, UDP and
// data-source status. Optional sections are omitted or null when the corresponding source
// is not configured. Connected is true once any observation has been received.
type StatusResponse struct {
	Connected              bool                   `json:"connected"`
	LastUpdate             string                 `json:"lastUpdate"`
	Uptime                 string                 `json:"uptime"`
	StationName            string                 `json:"stationName,omitempty"`
	StationURL             string                 `json:"stationURL,omitempty"`
	Elevation              float64                `json:"elevation"`
	HomeKit                map[string]interface{} `json:"homekit"`
	DataHistory            []WeatherResponse      `json:"dataHistory"`
	ObservationCount       int                    `json:"observationCount"`
	MaxHistorySize         int                    `json:"maxHistorySize"`
	HistoricalDataLoaded   bool                   `json:"historicalDataLoaded"`
	HistoricalDataCount    int                    `json:"historicalDataCount"`
	HistoryLoadingProgress struct {
		IsLoading   bool   `json:"isLoading"`
		CurrentStep int    `json:"currentStep"`
		TotalSteps  int    `json:"totalSteps"`
		Description string `json:"description"`
	} `json:"historyLoadingProgress"`
	Forecast          *weather.ForecastResponse `json:"forecast,omitempty"`
	StationStatus     *weather.StationStatus    `json:"stationStatus,omitempty"`
	GeneratedWeather  *GeneratedWeatherInfo     `json:"generatedWeather,omitempty"`
	UDPStatus         *UDPStatusInfo            `json:"udpStatus,omitempty"`
	DataSource        *weather.DataSourceStatus `json:"dataSource,omitempty"` // Unified data source status
	UnitHints         map[string]string         `json:"unitHints,omitempty"`
	ChartHistoryHours int                       `json:"chartHistoryHours"` // Hours of data to display in charts (0=all)
}

// UDPStatusInfo contains information about UDP stream status.
//
// It is included in /api/status only when a UDP listener has been set; Enabled is then
// always true. LastPacketTime is RFC 3339 and omitted before the first packet.
type UDPStatusInfo struct {
	Enabled        bool   `json:"enabled"`
	ReceivingData  bool   `json:"receivingData"`
	PacketCount    int64  `json:"packetCount"`
	StationIP      string `json:"stationIP,omitempty"`
	SerialNumber   string `json:"serialNumber,omitempty"`
	LastPacketTime string `json:"lastPacketTime,omitempty"`
}

// GeneratedWeatherInfo contains information about generated weather data.
//
// It describes the synthetic location, season and climate zone currently used by the
// weather generator; handleRegenerateWeatherAPI updates it in place.
type GeneratedWeatherInfo struct {
	Enabled     bool   `json:"enabled"`
	Location    string `json:"location"`
	Season      string `json:"season"`
	ClimateZone string `json:"climateZone"`
}

// WeatherGeneratorInterface defines the interface for weather generators.
//
// It is satisfied by the generator package's synthetic weather source and lets tests
// substitute a fake.
//
// Methods:
//   - GenerateNewSeason picks a new random location and season.
//   - GetLocation returns the current synthetic location.
//   - GetSeason returns the current synthetic season.
//   - GetDailyRainTotal returns the generator's running rain total for the current day.
//   - SetCurrentWeatherMode switches the generator to producing "now" observations
//     rather than historical backfill.
//   - GenerateObservation returns a new synthetic observation.
type WeatherGeneratorInterface interface {
	GenerateNewSeason()
	GetLocation() generator.Location
	GetSeason() generator.Season
	GetDailyRainTotal() float64
	SetCurrentWeatherMode()
	GenerateObservation() *weather.Observation
}

// calculateDailyRainAccumulation returns today's rain total.
//
// Calculate daily rain accumulation from historical data. When generated weather is
// active, the generator's daily total is returned instead. Otherwise it takes the
// observations since local midnight and returns the difference between the latest and
// earliest RainAccumulated values, treating RainAccumulated as a cumulative counter.
// With a single observation that value itself is returned if it is at most 10. It
// returns 0 when there is no data, when the counter went backwards, or when the
// difference exceeds the sanity limit of 20. It takes the read lock.
func (ws *WebServer) calculateDailyRainAccumulation() float64 {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	// For generated weather, use the generator's daily total
	if ws.generatedWeather != nil && ws.generatedWeather.Enabled && ws.weatherGenerator != nil {
		dailyTotal := ws.weatherGenerator.GetDailyRainTotal()
		ws.logDebug("Using generated weather daily rain total: %.3f inches", dailyTotal)
		return dailyTotal
	}

	if len(ws.dataHistory) == 0 {
		ws.logDebug("No data history available for daily rain calculation")
		return 0.0
	}

	// Get the start of the current local day
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	// Find observations from today
	var dailyObservations []weather.Observation
	for _, obs := range ws.dataHistory {
		obsTime := time.Unix(obs.Timestamp, 0)
		if obsTime.After(startOfDay) || obsTime.Equal(startOfDay) {
			dailyObservations = append(dailyObservations, obs)
		}
	}

	ws.logDebug("Daily rain calculation - Total history: %d, Today's observations: %d, Start of day: %s",
		len(ws.dataHistory), len(dailyObservations), startOfDay.Format("2006-01-02 15:04:05"))

	if len(dailyObservations) == 0 {
		ws.logDebug("No observations found for today")
		return 0.0
	}

	// Sort by timestamp to ensure we process chronologically
	sort.Slice(dailyObservations, func(i, j int) bool {
		return dailyObservations[i].Timestamp < dailyObservations[j].Timestamp
	})

	// Calculate total rain for the day
	// The rain_accumulated field from Tempest represents cumulative rain since station started
	// To get daily total, we find the difference between current and earliest reading today
	if len(dailyObservations) == 1 {
		// Only one observation today, so we can't calculate a difference
		// Return the accumulated value if it seems reasonable for a daily total
		singleValue := dailyObservations[0].RainAccumulated
		ws.logDebug("Only one observation today, rain value: %.3f", singleValue)
		if singleValue <= 10.0 { // Reasonable daily limit in inches
			return singleValue
		}
		return 0.0
	}

	// Find the earliest and latest readings for today
	earliestToday := dailyObservations[0].RainAccumulated
	latestToday := dailyObservations[len(dailyObservations)-1].RainAccumulated

	ws.logDebug("Daily rain calculation - Earliest: %.3f, Latest: %.3f, Observations count: %d",
		earliestToday, latestToday, len(dailyObservations))

	// If latest is greater than earliest, we have rain accumulation for the day
	if latestToday >= earliestToday {
		dailyTotal := latestToday - earliestToday
		ws.logDebug("Daily rain total calculated: %.3f inches", dailyTotal)
		// Sanity check: daily total shouldn't exceed reasonable limits
		if dailyTotal <= 20.0 { // 20 inches would be extreme but possible
			return dailyTotal
		}
		ws.logDebug("Daily rain total exceeds sanity limit (%.3f > 20.0), returning 0", dailyTotal)
	}

	// If we can't calculate a reliable daily total, return 0
	ws.logDebug("Cannot calculate reliable daily total, returning 0")
	return 0.0
}

// calculateDailyRainForTime calculates the daily rain total for a specific time.
//
// It considers observations from startOfDay through targetTime inclusive and returns the
// RainAccumulated of the last one at or before targetTime minus that of the first,
// clamped at 0 (a single observation returns its own value, clamped at 0). It does not
// lock; the caller must hold ws.mu. It scans all of dataHistory on every call.
func (ws *WebServer) calculateDailyRainForTime(targetTime time.Time, startOfDay time.Time) float64 {
	// Find observations from the start of the day up to the target time
	var dayObservations []weather.Observation
	for _, obs := range ws.dataHistory {
		obsTime := time.Unix(obs.Timestamp, 0)
		if (obsTime.After(startOfDay) || obsTime.Equal(startOfDay)) && !obsTime.After(targetTime) {
			dayObservations = append(dayObservations, obs)
		}
	}

	if len(dayObservations) == 0 {
		return 0.0
	}

	// Sort by timestamp
	sort.Slice(dayObservations, func(i, j int) bool {
		return dayObservations[i].Timestamp < dayObservations[j].Timestamp
	})

	// Calculate rain since start of day
	if len(dayObservations) == 1 {
		return math.Max(0, dayObservations[0].RainAccumulated)
	}

	// Find the earliest reading at start of day and target time reading
	earliestReading := dayObservations[0].RainAccumulated
	var targetReading float64

	// Find the reading closest to or at the target time
	for _, obs := range dayObservations {
		obsTime := time.Unix(obs.Timestamp, 0)
		if obsTime.Equal(targetTime) || obsTime.Before(targetTime) {
			targetReading = obs.RainAccumulated
		}
	}

	return math.Max(0, targetReading-earliestReading)
}

// calculateSeaLevelPressure reduces station pressure to sea level.
//
// Pressure analysis functions. stationPressure is in mb (hPa), temperature in Celsius and
// elevation in meters; the result is in the same unit as stationPressure. It uses the
// barometric formula with the standard 0.0065 K/m lapse rate and an exponent of 5.257.
func calculateSeaLevelPressure(stationPressure, temperature, elevation float64) float64 {
	// Convert temperature from Celsius to Kelvin
	tempK := temperature + 273.15

	// Standard atmosphere lapse rate in K/m
	lapseRate := 0.0065

	// Calculate sea level pressure using barometric formula
	// P_sea = P_station * (1 - (L * h) / (T + L * h))^(-g*M/(R*L))
	// Where: L = lapse rate, h = elevation, T = temperature at station, g*M/(R*L) ≈ 5.257
	factor := (lapseRate * elevation) / (tempK + lapseRate*elevation)
	seaLevelPressure := stationPressure * math.Pow(1-factor, -5.257)

	return seaLevelPressure
}

// getPressureDescription classifies a sea-level pressure in mb as "Low" (< 980),
// "High" (> 1020) or "Normal".
func getPressureDescription(pressure float64) string {
	if pressure < 980 {
		return "Low"
	} else if pressure > 1020 {
		return "High"
	}
	return "Normal"
}

// getPressureTrend returns "Rising", "Falling" or "Stable" for the recent pressure history.
//
// It converts the last 60 observations (about an hour at one-minute intervals) to
// sea-level pressure using elevation and compares the newest with the oldest: a change of
// more than 1 mb either way is a trend. Fewer than two observations is "Stable".
// dataHistory must be in ascending time order.
func getPressureTrend(dataHistory []weather.Observation, elevation float64) string {
	if len(dataHistory) < 2 {
		return "Stable"
	}

	// Look at last hour of data for trend (using sea level pressure for accurate analysis)
	recentData := make([]float64, 0)
	for i := len(dataHistory) - 1; i >= 0 && len(recentData) < 60; i-- {
		// Calculate sea level pressure for each historical point
		seaLevelPressure := calculateSeaLevelPressure(dataHistory[i].StationPressure, dataHistory[i].AirTemperature, elevation)
		recentData = append([]float64{seaLevelPressure}, recentData...)
	}

	if len(recentData) < 2 {
		return "Stable"
	}

	pressureChange := recentData[len(recentData)-1] - recentData[0]

	if pressureChange > 1.0 {
		return "Rising"
	} else if pressureChange < -1.0 {
		return "Falling"
	}
	return "Stable"
}

// getPressureWeatherForecast returns a short barometer-style forecast from a sea-level
// pressure in mb and a trend from getPressureTrend.
//
// Rising: "Fair Weather" above 1013, else "Storm Clearing". Falling: "Stormy" below 1000,
// "Unsettled" below 1013, else "Change Coming". Any other trend is treated as Stable:
// "Fair Weather" above 1020, "Stormy" below 1000, else "Settled".
func getPressureWeatherForecast(pressure float64, trend string) string {
	switch trend {
	case "Rising":
		if pressure > 1013 {
			return "Fair Weather"
		} else {
			return "Storm Clearing"
		}
	case "Falling":
		if pressure < 1000 {
			return "Stormy"
		} else if pressure < 1013 {
			return "Unsettled"
		} else {
			return "Change Coming"
		}
	default: // Stable
		if pressure > 1020 {
			return "Fair Weather"
		} else if pressure < 1000 {
			return "Stormy"
		} else {
			return "Settled"
		}
	}
}

// NewWebServer creates a WebServer and registers its routes, without starting it.
//
// port is the listen port without a colon; elevation (meters) is used for sea-level
// pressure; logLevel filters this server's debug/info logging; stationID and
// useWebStatus configure the TempestWX status manager; version is shown in the UI;
// stationURL is reported in /api/status. generatedWeather and weatherGenerator are
// non-nil only when synthetic weather is in use. units and unitsPressure are returned
// by /api/units. historyPoints is the history capacity: values above 100000 are capped
// at 100000 and values below 10 are raised to 100. chartHistoryHours limits the hours
// shown in charts (0 = all). alarmConfig is the configured alarm path or content and
// disableAlarms reflects the --disable-alarms flag; both feed /api/alarm-status.
func NewWebServer(port string, elevation float64, logLevel string, stationID int, useWebStatus bool, version string, stationURL string, generatedWeather *GeneratedWeatherInfo, weatherGenerator WeatherGeneratorInterface, units string, unitsPressure string, historyPoints int, chartHistoryHours int, alarmConfig string, disableAlarms bool) *WebServer {
	// Validate history size to prevent excessive memory allocation
	if historyPoints > 100000 {
		logger.Warn("History size %d is very large, capping at 100000 to prevent memory issues", historyPoints)
		historyPoints = 100000
	}
	if historyPoints < 10 {
		logger.Warn("History size %d is too small, setting to minimum of 100", historyPoints)
		historyPoints = 100
	}

	ws := &WebServer{
		port:              port,
		elevation:         elevation,
		logLevel:          logLevel,
		stationID:         stationID,
		maxHistorySize:    historyPoints,
		chartHistoryHours: chartHistoryHours,
		dataHistory:       make([]weather.Observation, 0, historyPoints),
		startTime:         time.Now(),
		version:           version,
		stationURL:        stationURL,
		generatedWeather:  generatedWeather,
		weatherGenerator:  weatherGenerator,
		units:             units,
		unitsPressure:     unitsPressure,
		alarmConfig:       alarmConfig,
		disableAlarms:     disableAlarms,
		homekitStatus: map[string]interface{}{
			"bridge":      false,
			"accessories": 0,
			"pin":         "00102003",
		},
	}

	// Initialize status manager
	ws.statusManager = weather.NewStatusManager(stationID, logLevel, useWebStatus)

	mux := http.NewServeMux()
	mux.HandleFunc("/", ws.handleDashboard)
	mux.HandleFunc("/api/weather", ws.handleWeatherAPI)
	mux.HandleFunc("/api/status", ws.handleStatusAPI)
	mux.HandleFunc("/api/alarm-status", ws.handleAlarmStatusAPI)
	mux.HandleFunc("/api/history", ws.handleHistoryAPI)
	mux.HandleFunc("/chart/", ws.handleChartPage)
	mux.HandleFunc("/api/regenerate-weather", ws.handleRegenerateWeatherAPI)
	mux.HandleFunc("/api/generate-weather", ws.handleGenerateWeatherAPI)
	mux.HandleFunc("/api/units", ws.handleUnitsAPI)

	ws.server = &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	// ensure logError is considered used by analyzers: take method value here
	// (this is a no-op assignment and safe with the current ws instance)
	_ = ws.logError

	return ws
}

// Start starts the status manager's periodic scraping and then serves HTTP, blocking
// until the listener stops.
//
// It returns the ListenAndServe error, which after Stop is http.ErrServerClosed.
func (ws *WebServer) Start() error {
	ws.logInfo("Starting web server on port %s", ws.port)

	// Start status manager for periodic scraping
	ws.statusManager.Start()

	ws.logInfo("Web server calling ListenAndServe on :%s", ws.port)
	err := ws.server.ListenAndServe()
	if err != nil {
		ws.logError("Web server ListenAndServe failed: %v", err)
		fmt.Printf("WEB SERVER ERROR: ListenAndServe failed: %v\n", err)
		return err
	}
	return nil
}

// UpdateWeather records obs as the current observation and inserts it into the history.
//
// The history stays sorted by Timestamp: obs is placed by binary search, replaces any
// entry with the same Timestamp, and the oldest entries are dropped beyond maxHistorySize.
// Note that weatherData is always set to obs even if obs is older than the newest entry.
func (ws *WebServer) UpdateWeather(obs *weather.Observation) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	ws.weatherData = obs

	// Insert observation into dataHistory while keeping it sorted by Timestamp (ascending).
	// Use binary search to find insertion index. If a reading with the same timestamp exists,
	// replace it. After insertion, trim the slice to retain the most recent maxHistorySize entries.
	ts := obs.Timestamp
	n := len(ws.dataHistory)

	if n == 0 {
		ws.dataHistory = append(ws.dataHistory, *obs)
	} else {
		lo, hi := 0, n
		for lo < hi {
			mid := (lo + hi) / 2
			if ws.dataHistory[mid].Timestamp < ts {
				lo = mid + 1
			} else {
				hi = mid
			}
		}

		// lo is the insertion index
		if lo > 0 && ws.dataHistory[lo-1].Timestamp == ts {
			// Replace existing at lo-1
			ws.dataHistory[lo-1] = *obs
		} else if lo < n && ws.dataHistory[lo].Timestamp == ts {
			// Replace existing at lo
			ws.dataHistory[lo] = *obs
		} else {
			// Insert at position lo
			ws.dataHistory = append(ws.dataHistory, weather.Observation{})
			copy(ws.dataHistory[lo+1:], ws.dataHistory[lo:])
			ws.dataHistory[lo] = *obs
		}

		// Trim to most recent maxHistorySize entries (keep the latest entries)
		if len(ws.dataHistory) > ws.maxHistorySize {
			start := len(ws.dataHistory) - ws.maxHistorySize
			ws.dataHistory = ws.dataHistory[start:]
		}
	}
}

// UpdateHomeKitStatus merges status into the HomeKit status map reported by /api/status;
// existing keys are overwritten and others kept.
func (ws *WebServer) UpdateHomeKitStatus(status map[string]interface{}) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	for k, v := range status {
		ws.homekitStatus[k] = v
	}
}

// SetStationName sets the station display name reported by /api/status.
func (ws *WebServer) SetStationName(name string) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.stationName = name
}

// SetHistoricalDataStatus marks historical data as loaded and records how many
// observations were preloaded.
func (ws *WebServer) SetHistoricalDataStatus(count int) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.historicalDataLoaded = true
	ws.historicalDataCount = count
}

// SetHistoryLoadingProgress marks history preloading as in progress at step currentStep
// of totalSteps, with a human-readable description shown by the dashboard.
func (ws *WebServer) SetHistoryLoadingProgress(currentStep, totalSteps int, description string) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.historyLoadingProgress.isLoading = true
	ws.historyLoadingProgress.currentStep = currentStep
	ws.historyLoadingProgress.totalSteps = totalSteps
	ws.historyLoadingProgress.description = description
}

// SetHistoryLoadingComplete clears the history-loading progress state.
func (ws *WebServer) SetHistoryLoadingComplete() {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.historyLoadingProgress.isLoading = false
	ws.historyLoadingProgress.currentStep = 0
	ws.historyLoadingProgress.totalSteps = 0
	ws.historyLoadingProgress.description = ""
}

// SetUDPListener sets the UDP listener for local station monitoring.
//
// Once set, /api/status includes a UDPStatus section built from the listener's stats.
func (ws *WebServer) SetUDPListener(listener *udp.UDPListener) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.udpListener = listener
}

// UpdateDataSourceStatus updates the unified data source status.
//
// A copy of status is stored and returned as the DataSource section of /api/status.
func (ws *WebServer) UpdateDataSourceStatus(status weather.DataSourceStatus) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.dataSourceStatus = &status
	ws.logDebug("Data source status updated: type=%s, active=%v, observations=%d",
		status.Type, status.Active, status.ObservationCount)
}

// UpdateForecast stores the latest forecast, returned as the Forecast section of
// /api/status.
func (ws *WebServer) UpdateForecast(forecast *weather.ForecastResponse) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.forecastData = forecast
}

// SetAlarmManager sets the alarm manager for status display.
//
// Until it is set, /api/alarm-status reports only whether alarms are configured.
func (ws *WebServer) SetAlarmManager(manager AlarmManagerInterface) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.alarmManager = manager
	logger.Info("Alarm manager connected to web server")
}

// GetStatusManager returns the status manager for external use.
func (ws *WebServer) GetStatusManager() *weather.StatusManager {
	ws.mu.RLock()
	defer ws.mu.RUnlock()
	return ws.statusManager
}

// UpdateBatteryFromObservation updates the status manager with battery data from the latest observation.
//
// It is a no-op when there is no status manager.
func (ws *WebServer) UpdateBatteryFromObservation(obs *weather.Observation) {
	if ws.statusManager != nil {
		ws.statusManager.UpdateBatteryFromObservation(obs)
	}
}

// handleDashboard serves the dashboard page at "/" and is also the fallback route.
//
// It serves ./test-api.html for /test-api.html, and files under /static/ or
// /pkg/web/static/ from ./pkg/web/static/ (relative to the working directory, so the
// binary must run from the repository root), with no-cache headers for styles.css and
// script.js so edits show without a hard reload. Any other path except "/" is a 404.
func (ws *WebServer) handleDashboard(w http.ResponseWriter, r *http.Request) {
	ws.logDebug("Request: %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)

	// Handle test-api.html specifically
	if r.URL.Path == "/test-api.html" {
		http.ServeFile(w, r, "./test-api.html")
		return
	}

	// Handle static files - support both /static/ and /pkg/web/static/ paths
	if strings.HasPrefix(r.URL.Path, "/pkg/web/static/") || strings.HasPrefix(r.URL.Path, "/static/") {
		var filename string
		if strings.HasPrefix(r.URL.Path, "/pkg/web/static/") {
			filename = strings.TrimPrefix(r.URL.Path, "/pkg/web/static/")
		} else {
			filename = strings.TrimPrefix(r.URL.Path, "/static/")
		}

		ws.logDebug("Static file request: %s (path: %s)", filename, r.URL.Path)

		// Serve the file from the physical directory
		filePath := "./pkg/web/static/" + filename

		// Set appropriate content type and cache-busting headers
		switch filename {
		case "styles.css":
			w.Header().Set("Content-Type", "text/css")
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")
		case "script.js":
			w.Header().Set("Content-Type", "application/javascript")
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")
		case "date-fns.min.js":
			w.Header().Set("Content-Type", "application/javascript")
		}

		ws.logDebug("Serving static file: %s", filePath)

		// Try to serve the file
		http.ServeFile(w, r, filePath)
		return
	}

	// Default to dashboard for root path
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	tmpl := ws.getDashboardHTML()
	_, _ = w.Write([]byte(tmpl))
}

// handleWeatherAPI serves /api/weather: the current observation as a WeatherResponse.
//
// It responds 503 when no observation has been received. Sea-level pressure, pressure
// condition, trend and forecast are derived here; rain since the last sample and rain
// rate come from comparing the current observation with the second-newest history entry
// (the newest being the current observation itself). CORS is open to any origin.
func (ws *WebServer) handleWeatherAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ws.logDebug("Weather endpoint called from %s", r.RemoteAddr)

	ws.mu.RLock()
	defer ws.mu.RUnlock()

	if ws.weatherData == nil {
		ws.logDebug("No weather data available")
		http.Error(w, "No weather data available", http.StatusServiceUnavailable)
		return
	}

	// Calculate sea level pressure using configured station elevation
	seaLevelPressure := calculateSeaLevelPressure(ws.weatherData.StationPressure, ws.weatherData.AirTemperature, ws.elevation)

	// Calculate pressure analysis with debug logging (using sea level pressure for accurate forecasting)
	pressureCondition := getPressureDescription(seaLevelPressure)
	pressureTrend := getPressureTrend(ws.dataHistory, ws.elevation)
	weatherForecast := getPressureWeatherForecast(seaLevelPressure, pressureTrend)

	// Use the precip_accum_local_day field from the WeatherFlow API as the daily total
	// The WeatherFlow API provides this value in millimeters which resets at midnight local time
	// Keep in mm for now, convert to user's preferred units in the frontend
	dailyRainTotal := ws.weatherData.RainDailyTotal

	// Calculate incremental rain since last sample (in mm)
	// The RainAccumulated field is cumulative rain in mm
	var incrementalRainMm float64
	var rainRate float64 // Rain intensity in mm/hr
	if len(ws.dataHistory) > 1 {
		// Use a sorted copy of history to ensure we get the chronologically latest reading
		historyCopy := make([]weather.Observation, len(ws.dataHistory))
		copy(historyCopy, ws.dataHistory)
		sort.Slice(historyCopy, func(i, j int) bool { return historyCopy[i].Timestamp < historyCopy[j].Timestamp })
		// Use the SECOND-to-last reading to compare with current (weatherData is the same as last history item)
		lastReading := historyCopy[len(historyCopy)-2].RainAccumulated
		incrementalRainMm = math.Max(0, ws.weatherData.RainAccumulated-lastReading)

		// Calculate rain rate in mm/hr
		timeDiffSeconds := ws.weatherData.Timestamp - historyCopy[len(historyCopy)-2].Timestamp
		if timeDiffSeconds > 0 {
			rainRate = (incrementalRainMm / float64(timeDiffSeconds)) * 3600 // mm/hr
		}
	} else {
		incrementalRainMm = 0 // No previous data
		rainRate = 0
	}

	ws.logDebug("Pressure analysis calculated - Condition: %s, Trend: %s, Forecast: %s", pressureCondition, pressureTrend, weatherForecast)
	ws.logDebug("Pressure values - Station: %.2f mb, Sea Level: %.2f mb (used for forecasting)", ws.weatherData.StationPressure, seaLevelPressure)
	ws.logDebug("Rain data calculated - Incremental: %.3f mm, Daily Total: %.3f mm, Rate: %.2f mm/hr", incrementalRainMm, dailyRainTotal, rainRate)

	response := WeatherResponse{
		Temperature:          ws.weatherData.AirTemperature,
		Humidity:             ws.weatherData.RelativeHumidity,
		WindSpeed:            ws.weatherData.WindAvg,
		WindGust:             ws.weatherData.WindGust,
		WindDirection:        ws.weatherData.WindDirection,
		RainAccum:            incrementalRainMm, // Rain since last sample (mm)
		RainRate:             rainRate,          // Rain intensity in mm/hr
		RainDailyTotal:       dailyRainTotal,    // Total rain since 00:00 (mm)
		PrecipitationType:    ws.weatherData.PrecipitationType,
		Pressure:             ws.weatherData.StationPressure,
		SeaLevelPressure:     seaLevelPressure,
		PressureCondition:    pressureCondition,
		PressureTrend:        pressureTrend,
		WeatherForecast:      weatherForecast,
		Illuminance:          ws.weatherData.Illuminance,
		UV:                   ws.weatherData.UV,
		Battery:              ws.weatherData.Battery,
		LightningStrikeAvg:   ws.weatherData.LightningStrikeAvg,
		LightningStrikeCount: ws.weatherData.LightningStrikeCount,
		LastUpdate:           time.Unix(ws.weatherData.Timestamp, 0).Format(time.RFC3339),
	}

	// Provide explicit unit hints for the client. These describe the units used in the numeric
	// fields returned by this API so clients (like the popout) can perform deterministic
	// conversions when necessary. These are the units used internally by the server/data.
	response.UnitHints = map[string]string{
		"temperature": "celsius",
		"pressure":    "mb",
		"wind":        "mph",
		"rain":        "inches",
	}

	// Add observation count and max history size for real-time updates in UI
	response.ObservationCount = len(ws.dataHistory)
	response.MaxHistorySize = ws.maxHistorySize

	ws.logDebug("Weather API response prepared - Temperature: %.1f°C, Humidity: %.1f%%, UV: %d, Illuminance: %.0f lux, Observations: %d/%d",
		response.Temperature, response.Humidity, response.UV, response.Illuminance, response.ObservationCount, response.MaxHistorySize)

	// Marshal to JSON first so we can log the exact payload sent to clients
	if b, err := json.Marshal(response); err == nil {
		ws.logDebug("Weather API JSON payload: %s", string(b))
		_, _ = w.Write(b)
		return
	}
	// Fallback to encoder if marshalling unexpectedly fails
	_ = json.NewEncoder(w).Encode(response)
}

// handleStatusAPI serves /api/status: a StatusResponse with the whole chart history.
//
// Each history entry gets incremental rain and rain rate relative to the previous entry
// and a daily total from calculateDailyRainForTime, all computed while holding the read
// lock. The TempestWX station status comes from the status manager's cached value.
// CORS is open to any origin.
func (ws *WebServer) handleStatusAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ws.logDebug("Status endpoint called from %s", r.RemoteAddr)

	ws.mu.RLock()
	defer ws.mu.RUnlock()

	connected := ws.weatherData != nil
	ws.logDebug("Status check - weatherData exists: %t", connected)
	lastUpdate := ""
	if ws.weatherData != nil {
		lastUpdate = time.Unix(ws.weatherData.Timestamp, 0).Format(time.RFC3339)
	}

	// Calculate uptime
	uptime := time.Since(ws.startTime)
	uptimeStr := fmt.Sprintf("%dh%dm%ds", int(uptime.Hours()), int(uptime.Minutes())%60, int(uptime.Seconds())%60)

	// Convert data history to response format with incremental rain calculation
	history := make([]WeatherResponse, 0, len(ws.dataHistory))

	// Work from a time-sorted copy to guarantee chronological ordering (oldest -> newest)
	historyCopy := make([]weather.Observation, len(ws.dataHistory))
	copy(historyCopy, ws.dataHistory)
	sort.Slice(historyCopy, func(i, j int) bool { return historyCopy[i].Timestamp < historyCopy[j].Timestamp })

	for i, obs := range historyCopy {
		// Calculate incremental rain since last observation (in mm)
		var incrementalRainMm float64
		var rainRate float64
		if i > 0 {
			// obs.RainAccumulated is cumulative rain in mm
			incrementalRainMm = math.Max(0, obs.RainAccumulated-historyCopy[i-1].RainAccumulated)
			// Calculate rain rate (mm/hr) from incremental rain and time difference
			timeDiffSeconds := obs.Timestamp - historyCopy[i-1].Timestamp
			if timeDiffSeconds > 0 {
				rainRate = (incrementalRainMm / float64(timeDiffSeconds)) * 3600 // mm/hr
			}
		} else {
			incrementalRainMm = 0 // First observation, no previous data
			rainRate = 0
		}

		// Calculate daily total for this observation (in mm)
		obsTime := time.Unix(obs.Timestamp, 0)
		startOfDay := time.Date(obsTime.Year(), obsTime.Month(), obsTime.Day(), 0, 0, 0, 0, obsTime.Location())
		dailyTotal := ws.calculateDailyRainForTime(obsTime, startOfDay)

		history = append(history, WeatherResponse{
			Temperature:          obs.AirTemperature,
			Humidity:             obs.RelativeHumidity,
			WindSpeed:            obs.WindAvg,
			WindGust:             obs.WindGust,
			WindDirection:        obs.WindDirection,
			RainAccum:            incrementalRainMm, // Incremental rain since last sample (mm)
			RainRate:             rainRate,          // Rain intensity in mm/hr
			RainDailyTotal:       dailyTotal,        // Total rain since 00:00 (mm)
			PrecipitationType:    obs.PrecipitationType,
			Pressure:             obs.StationPressure,
			Illuminance:          obs.Illuminance,
			UV:                   obs.UV,
			Battery:              obs.Battery,
			LightningStrikeAvg:   obs.LightningStrikeAvg,
			LightningStrikeCount: obs.LightningStrikeCount,
			LastUpdate:           time.Unix(obs.Timestamp, 0).Format(time.RFC3339),
		})
	}

	response := StatusResponse{
		Connected:            connected,
		LastUpdate:           lastUpdate,
		Uptime:               uptimeStr,
		Elevation:            ws.elevation,
		HomeKit:              ws.homekitStatus,
		DataHistory:          history,
		ObservationCount:     len(ws.dataHistory),
		MaxHistorySize:       ws.maxHistorySize,
		HistoricalDataLoaded: ws.historicalDataLoaded,
		HistoricalDataCount:  ws.historicalDataCount,
		GeneratedWeather:     ws.generatedWeather,
	}

	// Provide explicit unit hints for the client to indicate the units used in the
	// DataHistory entries and other numeric fields. This helps the popout determine
	// whether a conversion is required when the user requests a different display unit.
	response.UnitHints = map[string]string{
		"temperature": "celsius",
		"pressure":    "mb",
		"wind":        "mph",
		"rain":        "inches",
	}

	// Add progress information
	response.HistoryLoadingProgress.IsLoading = ws.historyLoadingProgress.isLoading
	response.HistoryLoadingProgress.CurrentStep = ws.historyLoadingProgress.currentStep
	response.HistoryLoadingProgress.TotalSteps = ws.historyLoadingProgress.totalSteps
	response.HistoryLoadingProgress.Description = ws.historyLoadingProgress.description

	// Add forecast data if available
	response.Forecast = ws.forecastData

	// Add station name if available
	response.StationName = ws.stationName

	// Add station URL if available
	response.StationURL = ws.stationURL

	// Add generated weather information if available
	response.GeneratedWeather = ws.generatedWeather

	// Add UDP status if UDP listener is active
	if ws.udpListener != nil {
		packetCount, lastPacket, stationIP, serialNumber := ws.udpListener.GetStats()
		udpInfo := &UDPStatusInfo{
			Enabled:       true,
			ReceivingData: ws.udpListener.IsReceivingData(),
			PacketCount:   packetCount,
			StationIP:     stationIP,
			SerialNumber:  serialNumber,
		}
		if !lastPacket.IsZero() {
			udpInfo.LastPacketTime = lastPacket.Format(time.RFC3339)
		}
		response.UDPStatus = udpInfo
		ws.logDebug("UDP Status - Enabled: %t, Receiving: %t, Packets: %d, IP: %s, Serial: %s",
			udpInfo.Enabled, udpInfo.ReceivingData, udpInfo.PacketCount, udpInfo.StationIP, udpInfo.SerialNumber)
	}

	// Add unified data source status if available
	if ws.dataSourceStatus != nil {
		response.DataSource = ws.dataSourceStatus
		ws.logDebug("Data Source Status - Type: %s, Active: %t, Observations: %d",
			ws.dataSourceStatus.Type, ws.dataSourceStatus.Active, ws.dataSourceStatus.ObservationCount)
	}

	// Add chart history hours configuration
	response.ChartHistoryHours = ws.chartHistoryHours

	// Fetch station status from TempestWX (async, don't block on errors)
	// Get station status from status manager (handles both scraping and fallback)
	ws.logDebug("Retrieving station status from status manager")
	stationStatus := ws.statusManager.GetStatus()
	response.StationStatus = stationStatus

	ws.logDebug("Station status retrieved - Source: %s, Battery: %s, LastScraped: %s",
		stationStatus.DataSource, stationStatus.BatteryVoltage, stationStatus.LastScraped)

	// Marshal to JSON first so tests can inspect the exact payload and to provide
	// clearer debugging output when headless tests observe unexpected/missing fields.
	if b, err := json.Marshal(response); err == nil {
		ws.logDebug("Status API JSON payload: %s", string(b))
		_, _ = w.Write(b)
		return
	}
	// Fallback
	_ = json.NewEncoder(w).Encode(response)
}

// AlarmStatusResponse represents the alarm status API response.
//
// Enabled is true when alarms are configured (a config string or a manager is present)
// and not disabled by flag. ConfigPath and LastReadTime come from the manager when one is
// set ("N/A"/"Never" otherwise); TotalAlarms and EnabledAlarms are the manager's counts.
type AlarmStatusResponse struct {
	Enabled       bool          `json:"enabled"`
	Disabled      bool          `json:"disabled"` // True if explicitly disabled via --disable-alarms
	ConfigPath    string        `json:"configPath"`
	LastReadTime  string        `json:"lastReadTime"`
	TotalAlarms   int           `json:"totalAlarms"`
	EnabledAlarms int           `json:"enabledAlarms"`
	Alarms        []AlarmStatus `json:"alarms"`
}

// AlarmStatus represents individual alarm information.
//
// Channels lists the channel types only. LastTriggered is local time
// "2006-01-02 15:04:05" or "Never". Cooldown is the configured cooldown in seconds.
// ScheduleActive is always true for alarms without a schedule (or with type "always").
type AlarmStatus struct {
	Name              string   `json:"name"`
	Description       string   `json:"description"`
	Enabled           bool     `json:"enabled"`
	Condition         string   `json:"condition"`
	Tags              []string `json:"tags"`
	Channels          []string `json:"channels"`
	LastTriggered     string   `json:"lastTriggered"`
	Cooldown          int      `json:"cooldown"`
	CooldownRemaining int      `json:"cooldownRemaining"` // Seconds remaining in cooldown (0 if ready)
	InCooldown        bool     `json:"inCooldown"`        // True if currently in cooldown
	TriggeredCount    int      `json:"triggeredCount"`
	HasSchedule       bool     `json:"hasSchedule"`    // True if alarm has a schedule defined
	ScheduleActive    bool     `json:"scheduleActive"` // True if schedule allows alarm to be active now
}

// handleAlarmStatusAPI serves /api/alarm-status: an AlarmStatusResponse built from the
// alarm manager, including per-alarm cooldown and schedule state evaluated at request
// time.
//
// Without a manager it returns only the enabled/disabled flags and the configured path,
// with an empty alarm list. CORS is open to any origin.
func (ws *WebServer) handleAlarmStatusAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ws.mu.RLock()
	alarmMgr := ws.alarmManager
	alarmConfig := ws.alarmConfig
	disableAlarms := ws.disableAlarms
	ws.mu.RUnlock()

	// Determine if alarms are enabled (configured, manager exists, and not disabled via flag)
	enabled := (alarmConfig != "" || alarmMgr != nil) && !disableAlarms

	// If no alarm manager, return basic status
	if alarmMgr == nil {
		_ = json.NewEncoder(w).Encode(AlarmStatusResponse{
			Enabled:       enabled,
			Disabled:      disableAlarms,
			ConfigPath:    alarmConfig,
			LastReadTime:  "N/A",
			TotalAlarms:   0,
			EnabledAlarms: 0,
			Alarms:        []AlarmStatus{},
		})
		return
	}

	// Get alarm configuration
	config := alarmMgr.GetConfig()
	totalAlarms := alarmMgr.GetAlarmCount()
	enabledAlarms := alarmMgr.GetEnabledAlarmCount()

	// Build alarm status list
	alarmStatuses := make([]AlarmStatus, 0, len(config.Alarms))
	for _, alm := range config.Alarms {
		// Get channel types
		channels := make([]string, 0, len(alm.Channels))
		for _, ch := range alm.Channels {
			channels = append(channels, ch.Type)
		}

		// Get last triggered time
		lastTriggered := "Never"
		if lastFired := alm.GetLastFired(); !lastFired.IsZero() {
			lastTriggered = lastFired.Format("2006-01-02 15:04:05")
		}

		// Get cooldown status
		cooldownRemaining := alm.GetCooldownRemaining()
		inCooldown := alm.IsInCooldown()

		// Check schedule status
		hasSchedule := alm.Schedule != nil && alm.Schedule.Type != "" && alm.Schedule.Type != "always"
		scheduleActive := true
		if hasSchedule {
			lat, lon := alarmMgr.GetLocation()
			scheduleActive = alm.Schedule.IsActive(time.Now(), lat, lon)
		}

		alarmStatuses = append(alarmStatuses, AlarmStatus{
			Name:              alm.Name,
			Description:       alm.Description,
			Enabled:           alm.Enabled,
			Condition:         alm.Condition,
			Tags:              alm.Tags,
			Channels:          channels,
			LastTriggered:     lastTriggered,
			Cooldown:          alm.Cooldown,
			CooldownRemaining: cooldownRemaining,
			InCooldown:        inCooldown,
			TriggeredCount:    alm.TriggeredCount,
			HasSchedule:       hasSchedule,
			ScheduleActive:    scheduleActive,
		})
	}

	// Get actual config path and load time
	configPath := alarmMgr.GetConfigPath()
	lastLoadTime := alarmMgr.GetLastLoadTime()
	lastReadTimeStr := "Never"
	if !lastLoadTime.IsZero() {
		lastReadTimeStr = lastLoadTime.Format("2006-01-02 15:04:05")
	}

	response := AlarmStatusResponse{
		Enabled:       enabled,
		Disabled:      disableAlarms,
		ConfigPath:    configPath,
		LastReadTime:  lastReadTimeStr,
		TotalAlarms:   totalAlarms,
		EnabledAlarms: enabledAlarms,
		Alarms:        alarmStatuses,
	}

	_ = json.NewEncoder(w).Encode(response)
}

// handleChartPage serves a dedicated chart page for a given weather type.
// URL format: /chart/<type>?config=<urlencoded-json>
//
// The same ./pkg/web/static/chart.html file is served for every type; the page's script
// reads the type and config from its own URL.
func (ws *WebServer) handleChartPage(w http.ResponseWriter, r *http.Request) {
	// Expected path: /chart/<type>
	ws.logDebug("Chart page requested: %s", r.URL.Path)

	// Serve the static chart.html template (script will read query params)
	if strings.HasPrefix(r.URL.Path, "/chart/") {
		http.ServeFile(w, r, "./pkg/web/static/chart.html")
		return
	}

	http.NotFound(w, r)
}

// handleRegenerateWeatherAPI handles requests to regenerate weather data for testing.
//
// It accepts POST only (405 otherwise) and responds 400 when no weather generator is
// configured. It asks the generator for a new random location and season, copies them
// into the GeneratedWeatherInfo reported by /api/status, and responds with
// {"success", "location", "season", "climateZone"}.
func (ws *WebServer) handleRegenerateWeatherAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Only allow POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if weather generation is enabled
	if ws.weatherGenerator == nil {
		http.Error(w, "Weather generation not enabled", http.StatusBadRequest)
		return
	}

	// Regenerate weather with new random location/season
	ws.weatherGenerator.GenerateNewSeason()

	// Update the generated weather info
	ws.mu.Lock()
	if ws.generatedWeather != nil {
		location := ws.weatherGenerator.GetLocation()
		ws.generatedWeather.Location = location.Name
		ws.generatedWeather.Season = ws.weatherGenerator.GetSeason().String()
		ws.generatedWeather.ClimateZone = location.ClimateZone
	}
	ws.mu.Unlock()

	// Return success response
	response := map[string]interface{}{
		"success":     true,
		"location":    ws.generatedWeather.Location,
		"season":      ws.generatedWeather.Season,
		"climateZone": ws.generatedWeather.ClimateZone,
	}

	_ = json.NewEncoder(w).Encode(response)
}

// handleUnitsAPI returns the current units configuration as
// {"units": ..., "unitsPressure": ...}. CORS is open to any origin.
func (ws *WebServer) handleUnitsAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ws.logDebug("Units endpoint called from %s", r.RemoteAddr)

	response := map[string]string{
		"units":         ws.units,
		"unitsPressure": ws.unitsPressure,
	}

	_ = json.NewEncoder(w).Encode(response)
}

// HistoryResponse represents a single historical observation with calculated incremental rain.
//
// It is the element type of /api/history and uses the Tempest API's snake_case field
// names (except rainAccum and rainRate) so the popout charts can treat it like raw API
// data. RainAccum and RainAccumulated both carry the observation's RainAccumulated value
// unchanged; RainRate is computed from the change since the previous entry.
type HistoryResponse struct {
	Timestamp            int64   `json:"timestamp"`
	AirTemperature       float64 `json:"air_temperature"`
	RelativeHumidity     float64 `json:"relative_humidity"`
	WindLull             float64 `json:"wind_lull"`
	WindAvg              float64 `json:"wind_avg"`
	WindGust             float64 `json:"wind_gust"`
	WindDirection        float64 `json:"wind_direction"`
	StationPressure      float64 `json:"station_pressure"`
	Illuminance          float64 `json:"illuminance"`
	UV                   int     `json:"uv"`
	SolarRadiation       float64 `json:"solar_radiation"`
	RainAccum            float64 `json:"rainAccum"`        // Incremental rain since last reading
	RainRate             float64 `json:"rainRate"`         // Rain intensity in mm/hr
	RainAccumulated      float64 `json:"rain_accumulated"` // API's cumulative rain from midnight
	PrecipitationType    int     `json:"precipitation_type"`
	LightningStrikeAvg   float64 `json:"lightning_strike_avg_distance"`
	LightningStrikeCount int     `json:"lightning_strike_count"`
	Battery              float64 `json:"battery"`
	ReportInterval       int     `json:"report_interval"`
}

// handleHistoryAPI returns historical weather observations for popout charts
// with calculated incremental rain values.
//
// It copies the history under the read lock, sorts it by time and returns it as a JSON
// array of HistoryResponse, oldest first. CORS is open to any origin.
func (ws *WebServer) handleHistoryAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ws.logDebug("History endpoint called from %s", r.RemoteAddr)

	ws.mu.RLock()
	history := make([]weather.Observation, len(ws.dataHistory))
	copy(history, ws.dataHistory)
	ws.mu.RUnlock()

	// Sort history by timestamp to ensure chronological order for rate calculations
	sort.Slice(history, func(i, j int) bool { return history[i].Timestamp < history[j].Timestamp })

	// Convert to response format
	// NOTE: We set rainAccum=0 for all historical observations because the WeatherFlow
	// historical API returns data from different time periods mixed together, causing
	// rain_accumulated values to jump around unpredictably. This makes it impossible to
	// calculate reliable incremental rain from historical data.
	// The "Window Total" line will only show meaningful data for observations collected
	// live going forward, not for preloaded historical data.
	response := make([]HistoryResponse, 0, len(history))

	for i, obs := range history {
		// Keep rain in mm (native units), convert to user's preferred units in frontend
		rainInMm := obs.RainAccumulated

		// Calculate rain rate in mm/hr
		var rainRate float64
		if i > 0 {
			prevObs := history[i-1]
			timeDiffSeconds := obs.Timestamp - prevObs.Timestamp
			if timeDiffSeconds > 0 {
				// Rain accumulated is in mm, calculate rate
				rainDiff := math.Max(0, obs.RainAccumulated-prevObs.RainAccumulated)
				rainRate = (rainDiff / float64(timeDiffSeconds)) * 3600 // mm/hr
			}
		}

		response = append(response, HistoryResponse{
			Timestamp:            obs.Timestamp,
			AirTemperature:       obs.AirTemperature,
			RelativeHumidity:     obs.RelativeHumidity,
			WindLull:             obs.WindLull,
			WindAvg:              obs.WindAvg,
			WindGust:             obs.WindGust,
			WindDirection:        obs.WindDirection,
			StationPressure:      obs.StationPressure,
			Illuminance:          obs.Illuminance,
			UV:                   obs.UV,
			SolarRadiation:       obs.SolarRadiation,
			RainAccum:            rainInMm, // Incremental rain per observation in mm
			RainRate:             rainRate, // Rain intensity in mm/hr
			RainAccumulated:      rainInMm, // Same value for backward compatibility
			PrecipitationType:    obs.PrecipitationType,
			LightningStrikeAvg:   obs.LightningStrikeAvg,
			LightningStrikeCount: obs.LightningStrikeCount,
			Battery:              obs.Battery,
			ReportInterval:       obs.ReportInterval,
		})
	}

	ws.logDebug("Returning %d historical observations with calculated incremental rain and rates", len(response))

	// Return the historical data with incremental rain
	_ = json.NewEncoder(w).Encode(response)
}

// getDashboardHTML returns the dashboard page's HTML.
//
// The page is built per request: Chart.js and its date adapter are loaded from
// ./pkg/web/static/ when CI or USE_LOCAL_CHARTJS is set (to avoid CDN flakiness in
// tests) and from unpkg otherwise, and script.js gets a nanosecond cache-busting query.
func (ws *WebServer) getDashboardHTML() string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Tempest Weather Dashboard</title>
    <link rel="stylesheet" href="pkg/web/static/styles.css">
    <link rel="stylesheet" href="pkg/web/static/themes.css">

</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🌤️ Tempest Weather Dashboard</h1>
            <div class="status" id="status">
                Connecting to weather station...
            </div>
        </div>

        <div class="grid">
            <div class="card" id="temperature-card">
                <div class="card-header">
                    <span class="card-icon">🌡️</span>
                    <span class="card-title">Temperature</span>
                </div>
                <div class="card-value" id="temperature">--</div>
                <div class="card-unit" id="temperature-unit" onclick="toggleUnit('temperature')">°C</div>
                <div class="chart-container">
                    <canvas id="temperature-chart"></canvas>
                </div>
            </div>

			<!-- Tempest Station Tooltip (hidden, toggled by JS) -->
			<div class="tempest-tooltip hidden" id="station-tooltip" role="dialog" aria-hidden="true">
				<div class="tempest-tooltip-header">
					<div>Tempest Station Details</div>
					<div class="tempest-tooltip-close" id="station-tooltip-close" title="Close">✕</div>
				</div>
				<div class="tempest-tooltip-body" style="padding:12px; font-size:0.9rem;">
					<p style="margin:0 0 8px 0;">Detailed device and hub status (battery, uptime, signal strength, firmware, serial numbers) are only available when the service is receiving UDP broadcasts from the Tempest station (<code>--udp-stream</code>) or when headless web-status scraping is enabled (<code>--use-web-status</code>).</p>
					<p style="margin:0;">If neither is enabled the API does not provide these details and the dashboard will show summary status only.</p>
				</div>
			</div>

            <div class="card" id="humidity-card">
                <div class="card-header">
                    <span class="card-icon">💧</span>
                    <span class="card-title">Humidity</span>
                </div>
                <div class="card-value" id="humidity">--</div>
                <div class="card-unit">% <span class="info-icon" id="humidity-info-icon" title="Click for humidity reference information">ℹ️</span></div>
                <div class="humidity-description" id="humidity-description">--</div>
                <div class="humidity-context" id="humidity-context">
                    <div class="humidity-tooltip" id="humidity-tooltip">
                        <div class="humidity-tooltip-header">
                            <strong>Humidity Comfort Levels:</strong>
                            <span class="humidity-tooltip-close" id="humidity-tooltip-close" title="Close">×</span>
                        </div>
                        <table class="humidity-table">
                            <thead>
                                <tr>
                                    <th>Humidity Range</th>
                                    <th>Comfort Level</th>
                                    <th>Effects</th>
                                </tr>
                            </thead>
                            <tbody>
                                <tr><td>0-30%</td><td>Very Dry</td><td>Static electricity, dry skin, respiratory discomfort</td></tr>
                                <tr><td>30-40%</td><td>Dry</td><td>Comfortable for most people, minimal static</td></tr>
                                <tr><td>40-60%</td><td>Ideal</td><td>Most comfortable range for humans</td></tr>
                                <tr><td>60-70%</td><td>Humid</td><td>Slightly sticky feeling, still comfortable</td></tr>
                                <tr><td>70-80%</td><td>Very Humid</td><td>Sticky, harder to cool down, mold risk</td></tr>
                                <tr><td>80%+</td><td>Extremely Humid</td><td>Very uncomfortable, high mold/mildew risk</td></tr>
                            </tbody>
                        </table>
                    </div>
                </div>
                <div class="feels-like-info">
                    <div class="flex-row">
                        <span>Heat Index (feels like):</span>
                        <span id="heat-index" class="heat-index-value">--</span>
                        <span class="info-icon" id="heat-index-info-icon" title="Click for heat index information">ℹ️</span>
                    </div>
                    <div class="heat-index-context" id="heat-index-context">
                        <div class="heat-index-tooltip" id="heat-index-tooltip">
                            <div class="heat-index-tooltip-header">
                                <strong>Heat Index Calculation:</strong>
                                <span class="heat-index-tooltip-close" id="heat-index-tooltip-close" title="Close">×</span>
                            </div>
                            <div class="heat-index-details">
                                <p><strong>What is Heat Index?</strong><br>
                                The heat index combines air temperature and relative humidity to determine the human-perceived equivalent temperature.</p>
                                
                                <p><strong>Calculation:</strong><br>
                                Uses the official NOAA formula with temperature ≥80°F (26.7°C) and humidity ≥40%.</p>
                                
                                <p><strong>Heat Index Categories:</strong></p>
                                <table class="heat-index-table">
                                    <tr><td>80-90°F (27-32°C)</td><td>Caution - Fatigue possible</td></tr>
                                    <tr><td>90-105°F (32-41°C)</td><td>Extreme caution - Heat cramps possible</td></tr>
                                    <tr><td>105-130°F (41-54°C)</td><td>Danger - Heat exhaustion likely</td></tr>
                                    <tr><td>130°F+ (54°C+)</td><td>Extreme danger - Heat stroke imminent</td></tr>
                                </table>
                                
                                <p class="heat-index-note">
                                Note: If conditions don't meet the threshold, actual temperature is displayed.
                                </p>
                            </div>
                        </div>
                    </div>
                </div>
                <div class="chart-container">
                    <canvas id="humidity-chart"></canvas>
                </div>
            </div>

            <div class="card" id="wind-card">
                <div class="card-header">
                    <span class="card-icon">🌬️</span>
                    <span class="card-title">Wind</span>
                </div>
                <div class="card-value" id="wind-speed">--</div>
                <div class="card-unit" id="wind-unit" onclick="toggleUnit('wind')">mph</div>
                <div class="wind-direction">
                    <span class="direction-arrow" id="wind-arrow">↑</span>
                    <span id="wind-direction">--</span>
                </div>
                <div class="wind-gust">
                    <span id="wind-gust-info">--</span>
                </div>
                <div class="chart-container">
                    <canvas id="wind-chart"></canvas>
                </div>
            </div>

            <div class="card" id="rain-card">
                <div class="card-header">
                    <span class="card-icon">🌧️</span>
                    <span class="card-title">Rain & Lightning</span>
                </div>
                <div class="card-value" id="rain">--</div>
                <div class="card-unit" id="rain-unit" onclick="toggleUnit('rain')">in <span class="info-icon" id="rain-info-icon" title="Click for rain intensity reference table">ℹ️</span></div>
                <div class="rain-context" id="rain-context">
                    <div class="rain-tooltip" id="rain-tooltip">
                        <div class="rain-tooltip-header">
                            <strong>Rain Intensity Reference Table:</strong>
                            <span class="rain-tooltip-close" id="rain-tooltip-close" title="Close">×</span>
                        </div>
                        <table class="rain-table">
                            <thead>
                                <tr>
                                    <th>Amount (mm)</th>
                                    <th>Amount (in)</th>
                                    <th>Description</th>
                                    <th>Intensity</th>
                                </tr>
                            </thead>
                            <tbody>
                                <tr><td>0.0 - 0.25</td><td>0.00 - 0.01</td><td>No rain to very light drizzle</td><td>None/Trace</td></tr>
                                <tr><td>0.25 - 1.0</td><td>0.01 - 0.04</td><td>Light drizzle</td><td>Very Light</td></tr>
                                <tr><td>1.0 - 2.5</td><td>0.04 - 0.10</td><td>Light rain</td><td>Light</td></tr>
                                <tr><td>2.5 - 7.5</td><td>0.10 - 0.30</td><td>Moderate rain</td><td>Moderate</td></tr>
                                <tr><td>7.5 - 20</td><td>0.30 - 0.79</td><td>Heavy rain</td><td>Heavy</td></tr>
                                <tr><td>20 - 50</td><td>0.79 - 1.97</td><td>Very heavy rain</td><td>Very Heavy</td></tr>
                                <tr><td>50+</td><td>1.97+</td><td>Extreme rainfall</td><td>Extreme</td></tr>
                            </tbody>
                        </table>
                    </div>
                </div>
                <div class="rain-description">
                    <span id="rain-description">--</span>
                </div>
                <div class="daily-rain-info">
                    <div class="daily-rain-content">
                        <span class="daily-rain-label">Today Total:</span>
                        <span id="daily-rain-total" class="daily-rain-value">--</span>
                    </div>
                </div>
                <div class="precipitation-type">
                    <div class="precipitation-info">💧 Type: <span id="precipitation-type">--</span></div>
                </div>
                <div class="lightning-info">
                    <div class="lightning-strikes">⚡ <span id="lightning-count">--</span> strikes</div>
                    <div class="lightning-distance">📏 <span id="lightning-distance">--</span> <span id="lightning-distance-unit">km</span></div>
                </div>
                <div class="chart-container">
                    <canvas id="rain-chart"></canvas>
                </div>
            </div>

            <div class="card" id="pressure-card">
                <div class="card-header">
                    <span class="card-icon">📊</span>
                    <span class="card-title">Pressure</span>
                </div>
                <div class="card-value" id="pressure">--</div>
                <div class="card-unit" id="pressure-unit" onclick="toggleUnit('pressure')">mb <span class="info-icon" id="pressure-info-icon" title="Click for pressure interpretation table">ℹ️</span></div>
                <div class="pressure-info-box">
                    <div class="pressure-info-row">
                        <span class="pressure-label">Condition:</span>
                        <span id="pressure-condition" class="pressure-value">--</span>
                    </div>
                    <div class="pressure-info-row-spaced">
                        <span class="pressure-label">Trend:</span>
                        <span id="pressure-trend" class="pressure-value">--</span>
                    </div>
                    <div class="pressure-info-row-spaced">
                        <span class="pressure-label">Forecast:</span>
                        <span id="pressure-forecast" class="pressure-value">--</span>
                    </div>
                    <div class="pressure-info-row-spaced">
                        <span class="pressure-label">Sea Level:</span>
                        <span id="pressure-sea-level" class="pressure-value">--</span>
                    </div>
                </div>
                <div class="pressure-context" id="pressure-context">
                    <div class="pressure-tooltip" id="pressure-tooltip">
                        <div class="pressure-tooltip-header">
                            <strong>Barometric Pressure Interpretation:</strong>
                            <span class="pressure-tooltip-close" id="pressure-tooltip-close" title="Close">×</span>
                        </div>
                        <table class="pressure-table">
                            <thead>
                                <tr>
                                    <th>Pressure (mb)</th>
                                    <th>Pressure (inHg)</th>
                                    <th>Condition</th>
                                    <th>Weather Trend</th>
                                </tr>
                            </thead>
                            <tbody>
                                <tr class="pressure-row-very-low"><td>&lt; 980</td><td>&lt; 28.94</td><td><strong>Very Low</strong></td><td>Stormy weather likely</td></tr>
                                <tr class="pressure-row-low"><td>980-1000</td><td>28.94-29.53</td><td><strong>Low</strong></td><td>Unsettled weather, possible storms</td></tr>
                                <tr class="pressure-row-normal"><td>1000-1020</td><td>29.53-30.12</td><td><strong>Normal</strong></td><td>Fair weather conditions</td></tr>
                                <tr class="pressure-row-high"><td>1020-1040</td><td>30.12-30.71</td><td><strong>High</strong></td><td>Generally clear and stable</td></tr>
                                <tr class="pressure-row-very-high"><td>&gt; 1040</td><td>&gt; 30.71</td><td><strong>Very High</strong></td><td>Very stable, clear conditions</td></tr>
                            </tbody>
                        </table>
                        <div class="pressure-trends-box">
                            <strong>Pressure Trend Analysis & Weather Forecast:</strong>
                            <ul class="pressure-trends-list">
                                <li><strong>Rising Rapidly:</strong> Quick improvement, clearing skies</li>
                                <li><strong>Rising:</strong> Improving weather, fair conditions ahead</li>
                                <li><strong>Steady:</strong> Current weather conditions will continue</li>
                                <li><strong>Falling:</strong> Weather deteriorating, clouds/rain possible</li>
                                <li><strong>Falling Rapidly:</strong> Storm approaching quickly, take precautions</li>
                            </ul>
                            <div class="pressure-wind-note-text">
                                <strong>Combined Forecast:</strong> The condition shown combines current pressure with trend analysis for more accurate weather prediction.
                            </div>
                        </div>
                        <p class="pressure-note">
                        Note: Weather predictions based on pressure require considering local conditions and trends over time.
                        </p>
                    </div>
                </div>
                <div class="chart-container">
                    <canvas id="pressure-chart"></canvas>
                </div>
            </div>

            <div class="card" id="light-card">
                <div class="card-header">
                    <span class="card-icon">☀️</span>
                    <span class="card-title">Light</span>
                </div>
                <div class="card-value" id="illuminance">--</div>
                <div class="card-unit">lux <span class="info-icon" id="lux-info-icon" title="Click for lux reference table">ℹ️</span></div>
                <div class="lux-description" id="lux-description">--</div>
                <div class="lux-context" id="lux-context">
                    <div class="lux-tooltip" id="lux-tooltip">
                        <div class="lux-tooltip-header">
                            <strong>Lux Reference Table:</strong>
                            <span class="lux-tooltip-close" id="lux-tooltip-close" title="Close">×</span>
                        </div>
                        <table class="lux-table">
                            <thead>
                                <tr>
                                    <th>Lux</th>
                                    <th>Condition</th>
                                </tr>
                            </thead>
                            <tbody>
                                <tr><td>0.0001</td><td>Moonless, overcast night sky (starlight)</td></tr>
                                <tr><td>0.002</td><td>Moonless clear night sky with airglow</td></tr>
                                <tr><td>0.01</td><td>Quarter moon on a clear night</td></tr>
                                <tr><td>0.05–0.3</td><td>Full moon on a clear night</td></tr>
                                <tr><td>3.4</td><td>Dark limit of civil twilight under a clear sky</td></tr>
                                <tr><td>20–50</td><td>Public areas with dark surroundings</td></tr>
                                <tr><td>50</td><td>Family living room lights</td></tr>
                                <tr><td>80</td><td>Office building hallway/toilet lighting</td></tr>
                                <tr><td>100</td><td>Very dark overcast day</td></tr>
                                <tr><td>150</td><td>Train station platforms</td></tr>
                                <tr><td>320–500</td><td>Office lighting</td></tr>
                                <tr><td>400</td><td>Sunrise or sunset on a clear day</td></tr>
                                <tr><td>1000</td><td>Overcast day; typical TV studio lighting</td></tr>
                                <tr><td>10,000–25,000</td><td>Full daylight (not direct sun)</td></tr>
                                <tr><td>32,000–100,000</td><td>Direct sunlight</td></tr>
                            </tbody>
                        </table>
                    </div>
                </div>
                <div class="chart-container">
                    <canvas id="light-chart"></canvas>
                </div>
            </div>

            <div class="card" id="uv-card">
                <div class="card-header">
                    <span class="card-icon">🌞</span>
                    <span class="card-title">UV Index</span>
                </div>
                <div class="card-value" id="uv-index">--</div>
                <div class="card-unit">UVI <span class="info-icon" id="uv-info-icon" title="Click for UV Index exposure categories">ℹ️</span></div>
                <div class="uv-description" id="uv-description">--</div>
                <div class="uv-context" id="uv-context">
                    <div class="uv-tooltip" id="uv-tooltip">
                        <div class="uv-tooltip-header">
                            <strong>UV Index Exposure Categories:</strong>
                            <span class="uv-tooltip-close" id="uv-tooltip-close" title="Close">×</span>
                        </div>
                        <table class="uv-table">
                            <thead>
                                <tr>
                                    <th>UV Index</th>
                                    <th>Category</th>
                                    <th>Protection Advice</th>
                                </tr>
                            </thead>
                            <tbody>
                                <tr class="uv-row-low"><td>0–2</td><td><strong>Low</strong></td><td>Low danger from the sun's UV rays for the average person.</td></tr>
                                <tr class="uv-row-moderate"><td>3–5</td><td><strong>Moderate</strong></td><td>Moderate risk of harm from unprotected sun exposure.</td></tr>
                                <tr class="uv-row-high"><td>6–7</td><td><strong>High</strong></td><td>High risk of harm from unprotected sun exposure. Protection against skin and eye damage is needed.</td></tr>
                                <tr class="uv-row-very-high"><td>8–10</td><td><strong>Very High</strong></td><td>Very high risk of harm from unprotected sun exposure. Take extra precautions because unprotected skin and eyes will be damaged and can burn quickly.</td></tr>
                                <tr class="uv-row-extreme"><td>11+</td><td><strong>Extreme</strong></td><td>Extreme risk of harm from unprotected sun exposure. Take all precautions because unprotected skin and eyes can burn in minutes.</td></tr>
                            </tbody>
                        </table>
                        <p class="uv-note">
                        Source: U.S. Environmental Protection Agency UV Index scale
                        </p>
                    </div>
                </div>
                <div class="chart-container">
                    <canvas id="uv-chart"></canvas>
                </div>
            </div>
        </div>

        <!-- Information Cards -->
        <div class="grid">
            <!-- Tempest Forecast Card -->
            <div class="card" id="forecast-card">
                <div class="card-header">
                    <span class="card-icon">📅</span>
                    <span class="card-title">Tempest Forecast</span>
                </div>
                <div class="card-content">
                    <div class="forecast-current">
                        <div class="forecast-current-main">
                            <div class="forecast-icon" id="forecast-current-icon">--</div>
                            <div class="forecast-temp-container">
                                <div class="forecast-temp" id="forecast-current-temp">--°</div>
                                <div class="forecast-feels-like">Feels like <span id="forecast-current-feels-like">--°</span></div>
                            </div>
                            <div class="forecast-conditions" id="forecast-current-conditions">--</div>
                        </div>
                        <div class="forecast-current-details">
                            <div class="forecast-detail">
                                <span class="forecast-label">Humidity:</span>
                                <span class="forecast-value" id="forecast-current-humidity">--%</span>
                            </div>
                            <div class="forecast-detail">
                                <span class="forecast-label">Wind:</span>
                                <span class="forecast-value" id="forecast-current-wind">-- mph</span>
                            </div>
                            <div class="forecast-detail">
                                <span class="forecast-label">Pressure:</span>
                                <span class="forecast-value" id="forecast-current-pressure">-- mb</span>
                            </div>
                            <div class="forecast-detail">
                                <span class="forecast-label">Precip:</span>
                                <span class="forecast-value" id="forecast-current-precip">--%</span>
                            </div>
                        </div>
                    </div>
                    <div class="forecast-daily" id="forecast-daily-container">
                        <!-- Daily forecast items will be populated by JavaScript -->
                    </div>
                </div>
            </div>

            <div class="card" id="tempest-card">
                <div class="card-header">
                    <span class="card-icon">🌤️</span>
                    <span class="card-title">Tempest Station</span>
                    <button class="compact-toggle" id="tempest-compact-toggle" title="Toggle compact/detailed view">⚙️</button>
                </div>
                <div class="card-content">
                    <!-- General Status -->
                    <div class="info-row">
                        <span class="info-label">Status:</span>
                        <span class="info-value" id="tempest-status">Disconnected</span>
                    </div>
					<div class="info-row">
						<span class="info-label">Data Source:</span>
						<span class="info-value" id="tempest-data-source">--</span>
						<span class="info-icon" id="station-info-icon" role="button" aria-label="More info about Tempest Station status" title="More info about Tempest Station status">ℹ️</span>
					</div>
                    <div class="info-row">
                        <span class="info-label">Station:</span>
                        <span class="info-value" id="tempest-station">--</span>
                    </div>
                    <div class="info-row" id="tempest-station-url-row">
                        <span class="info-label" id="tempest-station-url-label">Station URL:</span>
                        <span class="info-value" id="tempest-station-url">--</span>
                    </div>
                    <div class="info-row">
                        <span class="info-label">Elevation:</span>
                        <span class="info-value" id="tempest-elevation">--</span>
                    </div>
                    <div class="info-row">
                        <span class="info-label">Last Update:</span>
                        <span class="info-value" id="tempest-last-update">--</span>
                    </div>
                    <div class="info-row">
                        <span class="info-label">Data Points:</span>
                        <span class="info-value" id="tempest-data-count">--</span>
                    </div>
                    <div class="info-row hidden" id="tempest-historical-row">
                        <span class="info-label">Historical:</span>
                        <span class="info-value" id="tempest-historical-count">--</span>
                    </div>
                    
                    <!-- Device Status -->
                    <div class="status-section">
                        <div class="info-row clickable" id="device-status-row">
                            <span class="info-label section-header">📡 Device Status</span>
                            <span class="expand-icon" id="device-status-expand-icon">▶</span>
                        </div>
                        <div class="status-expanded hidden" id="device-status-expanded">
                            <div class="info-row">
                                <span class="info-label">Battery Level:</span>
                                <span class="info-value">
                                    <span class="battery-indicator" id="tempest-battery-indicator"></span>
                                    <span id="tempest-battery">--</span>
                                </span>
                            </div>
                            <div class="info-row">
                                <span class="info-label">Device Uptime:</span>
                                <span class="info-value" id="tempest-device-uptime">--</span>
                            </div>
                            <div class="info-row">
                                <span class="info-label">Network Status:</span>
                                <span class="info-value" id="tempest-device-network">--</span>
                            </div>
                            <div class="info-row">
                                <span class="info-label">Signal Strength:</span>
                                <span class="info-value">
                                    <span class="signal-bars" id="tempest-device-signal-bars"></span>
                                    <span id="tempest-device-signal">--</span>
                                </span>
                            </div>
                            <div class="info-row">
                                <span class="info-label">Last Observation:</span>
                                <span class="info-value" id="tempest-device-last-obs">--</span>
                            </div>
                            <div class="info-row">
                                <span class="info-label">Serial Number:</span>
                                <span class="info-value" id="tempest-device-serial">--</span>
                            </div>
                            <div class="info-row">
                                <span class="info-label">Firmware:</span>
                                <span class="info-value" id="tempest-device-firmware">--</span>
                            </div>
                            <div class="info-row">
                                <span class="info-label">Sensor Status:</span>
                                <span class="info-value" id="tempest-sensor-status">--</span>
                            </div>
                            <div class="info-row">
                                <span class="info-label">Battery Status:</span>
                                <span class="info-value" id="tempest-battery-status">--</span>
                            </div>
                        </div>
                    </div>
                    
                    <!-- Hub Status -->
                    <div class="status-section">
                        <div class="info-row clickable" id="hub-status-row">
                            <span class="info-label section-header">🏠 Hub Status</span>
                            <span class="expand-icon" id="hub-status-expand-icon">▶</span>
                        </div>
                        <div class="status-expanded hidden" id="hub-status-expanded">
                            <div class="info-row">
                                <span class="info-label">Hub Uptime:</span>
                                <span class="info-value" id="tempest-hub-uptime">--</span>
                            </div>
                            <div class="info-row">
                                <span class="info-label">Network Status:</span>
                                <span class="info-value" id="tempest-hub-network">--</span>
                            </div>
                            <div class="info-row">
                                <span class="info-label">WiFi Signal:</span>
                                <span class="info-value">
                                    <span class="signal-bars" id="tempest-hub-signal-bars"></span>
                                    <span id="tempest-hub-wifi">--</span>
                                </span>
                            </div>
                            <div class="info-row">
                                <span class="info-label">Last Status:</span>
                                <span class="info-value" id="tempest-hub-last-status">--</span>
                            </div>
                            <div class="info-row">
                                <span class="info-label">Serial Number:</span>
                                <span class="info-value" id="tempest-hub-serial">--</span>
                            </div>
                            <div class="info-row">
                                <span class="info-label">Firmware:</span>
                                <span class="info-value" id="tempest-hub-firmware">--</span>
                            </div>
                        </div>
                    </div>
                </div>
            </div>

            <div class="card" id="homekit-card">
                <div class="card-header">
                    <span class="card-icon">🏠</span>
                    <span class="card-title">HomeKit Bridge</span>
                </div>
                <div class="card-content">
                    <!-- General Status -->
                    <div class="info-row">
                        <span class="info-label">Status:</span>
                        <span class="info-value" id="homekit-status">Inactive</span>
                    </div>
                    <div class="info-row">
                        <span class="info-label">Bridge Name:</span>
                        <span class="info-value" id="homekit-bridge">--</span>
                    </div>
                    <div class="info-row clickable" id="accessories-row">
                        <span class="info-label">Accessories:</span>
                        <span class="info-value" id="homekit-accessories">--</span>
                        <span class="expand-icon" id="accessories-expand-icon">▶</span>
                    </div>
                    <div class="accessories-expanded hidden" id="accessories-expanded">
                        <div id="accessories-list">
                            <!-- Accessories will be populated here -->
                        </div>
                    </div>
                    
                    <!-- Connection Info -->
                    <div class="status-section">
                        <div class="info-row clickable" id="homekit-connection-row">
                            <span class="info-label section-header">🔗 Connection Info</span>
                            <span class="expand-icon" id="homekit-connection-expand-icon">▶</span>
                        </div>
                        <div class="status-expanded hidden" id="homekit-connection-expanded">
                            <div class="info-row">
                                <span class="info-label">Setup PIN:</span>
                                <span class="info-value" id="homekit-pin">--</span>
                            </div>
                            <div class="info-row">
                                <span class="info-label">Setup Code:</span>
                                <span class="info-value" id="homekit-setup-code">--</span>
                            </div>
                            <div class="info-row" style="flex-direction: column; align-items: center; padding: 10px 0;">
                                <span class="info-label" style="margin-bottom: 10px;">Setup QR Code:</span>
                                <canvas id="homekit-qr-code" style="border: 2px solid #ddd; border-radius: 8px; padding: 10px; background: white;"></canvas>
                            </div>
                            <div class="info-row">
                                <span class="info-label">Paired Devices:</span>
                                <span class="info-value" id="homekit-paired-devices">--</span>
                            </div>
                            <div class="info-row">
                                <span class="info-label">Reachability:</span>
                                <span class="info-value" id="homekit-reachability">--</span>
                            </div>
                            <div class="info-row">
                                <span class="info-label">Last Request:</span>
                                <span class="info-value" id="homekit-last-request">--</span>
                            </div>
                        </div>
                    </div>
                    
                    <!-- Technical Details -->
                    <div class="status-section">
                        <div class="info-row clickable" id="homekit-technical-row">
                            <span class="info-label section-header">⚙️ Technical Details</span>
                            <span class="expand-icon" id="homekit-technical-expand-icon">▶</span>
                        </div>
                        <div class="status-expanded hidden" id="homekit-technical-expanded">
                            <div class="info-row">
                                <span class="info-label">Bridge ID:</span>
                                <span class="info-value" id="homekit-bridge-id">--</span>
                            </div>
                            <div class="info-row">
                                <span class="info-label">Manufacturer:</span>
                                <span class="info-value" id="homekit-manufacturer">--</span>
                            </div>
                            <div class="info-row">
                                <span class="info-label">Model:</span>
                                <span class="info-value" id="homekit-model">--</span>
                            </div>
                            <div class="info-row">
                                <span class="info-label">Firmware:</span>
                                <span class="info-value" id="homekit-firmware">--</span>
                            </div>
                            <div class="info-row">
                                <span class="info-label">Bridge Port:</span>
                                <span class="info-value" id="homekit-port">--</span>
                            </div>
                            <div class="info-row">
                                <span class="info-label">HAP Version:</span>
                                <span class="info-value" id="homekit-hap-version">--</span>
                            </div>
                            <div class="info-row">
                                <span class="info-label">Configuration #:</span>
                                <span class="info-value" id="homekit-config-number">--</span>
                            </div>
                            <div class="info-row">
                                <span class="info-label">Category:</span>
                                <span class="info-value" id="homekit-category">--</span>
                            </div>
                            <div class="info-row">
                                <span class="info-label">Uptime (Paired):</span>
                                <span class="info-value" id="homekit-paired-uptime">--</span>
                            </div>
                        </div>
                    </div>
                </div>
            </div>

            <div class="card" id="alarm-card">
                <div class="card-header">
                    <span class="card-icon">🚨</span>
                    <span class="card-title">Alarm Status</span>
                    <button class="alarm-compact-toggle" id="alarm-compact-toggle" title="Toggle compact/detailed view">⚙️</button>
                </div>
                <div class="alarm-status-content">
                    <div class="alarm-info-row">
                        <span class="alarm-label">Status:</span>
                        <span class="alarm-value" id="alarm-status">Loading...</span>
                    </div>
                    <div class="alarm-info-row">
                        <span class="alarm-label">Config:</span>
                        <span class="alarm-value" id="alarm-config-path">--</span>
                    </div>
                    <div class="alarm-info-row">
                        <span class="alarm-label">Last Read:</span>
                        <span class="alarm-value" id="alarm-last-read">--</span>
                    </div>
                    <div class="alarm-info-row">
                        <span class="alarm-label">Total Alarms:</span>
                        <span class="alarm-value"><span id="alarm-enabled-count">--</span> / <span id="alarm-total-count">--</span> enabled</span>
                    </div>
                    <div class="alarm-list" id="alarm-list">
                        <div class="alarm-list-header">Active Alarms:</div>
                        <!-- Alarm items will be inserted here by JavaScript -->
                    </div>
                </div>
            </div>
        </div>

        <div class="footer">
            <p>Last updated: <span id="last-update">--</span></p>
            <p>Tempest HomeKit Service v` + ws.version + `</p>
            <div class="theme-selector">
                <label for="theme-select">🎨 Theme:</label>
                <select id="theme-select">
                    <option value="default">Default (Purple)</option>
                    <option value="ocean">Ocean Blue</option>
                    <option value="sunset">Sunset Orange</option>
                    <option value="forest">Forest Green</option>
                    <option value="midnight">Midnight Dark</option>
                    <option value="arctic">Arctic Light</option>
                    <option value="autumn">Autumn Earth</option>
                </select>
            </div>
        </div>
    <!-- External JavaScript Libraries -->
    ` + func() string {
		// If running under CI or explicit flag, prefer local static copies to avoid CDN/network flakiness
		if os.Getenv("CI") != "" || os.Getenv("USE_LOCAL_CHARTJS") != "" {
			return `
    <script src="/pkg/web/static/chart.umd.js"></script>
    <script src="/pkg/web/static/chartjs-adapter-date-fns.bundle.min.js"></script>
    `
		}
		return `
    <script src="https://unpkg.com/chart.js@4.4.4/dist/chart.umd.js"></script>
    <script src="https://unpkg.com/chartjs-adapter-date-fns@3.0.0/dist/chartjs-adapter-date-fns.bundle.min.js"></script>
    `
	}() + `
    
	<!-- QR Code Library -->
	<script src="/pkg/web/static/qrcode.min.js"></script>
	
	<!-- Main Application Script -->
	<script src="/pkg/web/static/alarm-utils.js"></script>
	<script src="pkg/web/static/script.js?v=` + fmt.Sprintf("%d", time.Now().UnixNano()) + `"></script>
</body>
</html>`
}

// handleGenerateWeatherAPI returns Tempest API-compatible JSON format for generated weather data.
//
// It accepts GET only (405 otherwise) and responds 503 when no weather generator is
// configured or 500 if it produces no observation. It switches the generator to
// current-weather mode and returns one fresh observation shaped like the Tempest REST
// API, so the weather client can be pointed at it (for example with --station-url) as
// if it were the real station API.
func (ws *WebServer) handleGenerateWeatherAPI(w http.ResponseWriter, r *http.Request) {
	ws.logDebug("Generate weather API endpoint called from %s", r.RemoteAddr)

	// Only allow GET requests
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ws.mu.RLock()
	generator := ws.weatherGenerator
	ws.mu.RUnlock()

	// Check if we have a weather generator
	if generator == nil {
		ws.logDebug("No weather generator available - cannot generate weather data")
		http.Error(w, "Weather generator not available", http.StatusServiceUnavailable)
		return
	}

	// Ensure we're in current weather mode (not historical)
	generator.SetCurrentWeatherMode()

	// Generate a fresh observation
	obs := generator.GenerateObservation()
	if obs == nil {
		ws.logDebug("Failed to generate weather observation")
		http.Error(w, "Failed to generate weather data", http.StatusInternalServerError)
		return
	}

	// Return in Tempest API format - single observation wrapped in obs array
	// This matches the format expected by the weather client
	response := map[string]interface{}{
		"obs": []map[string]interface{}{
			{
				"timestamp":                     obs.Timestamp,
				"wind_lull":                     obs.WindLull,
				"wind_avg":                      obs.WindAvg,
				"wind_gust":                     obs.WindGust,
				"wind_direction":                obs.WindDirection,
				"station_pressure":              obs.StationPressure,
				"air_temperature":               obs.AirTemperature,
				"relative_humidity":             obs.RelativeHumidity,
				"illuminance":                   obs.Illuminance,
				"uv":                            obs.UV,
				"solar_radiation":               obs.SolarRadiation,
				"rain_accumulated":              obs.RainAccumulated,
				"precipitation_type":            obs.PrecipitationType,
				"lightning_strike_avg_distance": obs.LightningStrikeAvg,
				"lightning_strike_count":        obs.LightningStrikeCount,
				"battery":                       obs.Battery,
				"report_interval":               obs.ReportInterval,
			},
		},
	}

	ws.logDebug("Generated weather API response - Temp: %.1f°C, Rain: %.3f in, Battery: %.1fV",
		obs.AirTemperature, obs.RainAccumulated, obs.Battery)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

// Stop closes the HTTP server immediately (without draining in-flight requests),
// causing Start to return. It does not stop the status manager.
func (ws *WebServer) Stop() error {
	if ws.server != nil {
		return ws.server.Close()
	}
	return nil
}
