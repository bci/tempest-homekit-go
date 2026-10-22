// Package service provides the main service orchestration for the Tempest HomeKit bridge.
// It coordinates between the WeatherFlow API client, HomeKit accessories, and web dashboard.
//
// StartService wires everything together and then runs a single observation loop
// that is the same for every data source. The package also provides the data
// source factory (CreateDataSource, overridable through DataSourceFactory) that
// picks the appropriate weather data source - UDP broadcast, custom station URL,
// generated weather or the WeatherFlow API - from the configuration.
package service

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"tempest-homekit-go/pkg/alarm"
	"tempest-homekit-go/pkg/config"
	"tempest-homekit-go/pkg/generator"
	"tempest-homekit-go/pkg/homekit"
	"tempest-homekit-go/pkg/logger"
	"tempest-homekit-go/pkg/udp"
	"tempest-homekit-go/pkg/weather"
	"tempest-homekit-go/pkg/web"
)

// StartService initializes and starts the Tempest HomeKit service with the provided configuration.
// It sets up HomeKit accessories, starts the web server, and begins weather data polling.
// Now uses unified data source architecture for clean separation of concerns.
//
// In order it: resolves the station (WeatherFlow API lookup, a placeholder in UDP
// mode, or a fake station for generated weather), starts the HomeKit bridge
// unless cfg.DisableHomeKit, starts the web dashboard unless
// cfg.DisableWebConsole, sets up the alarm manager when alarms are configured,
// optionally preloads history (cfg.HistoryRead), creates the data source through
// DataSourceFactory and starts it. It then blocks, feeding every observation to
// HomeKit, the web server and the alarm manager, until the data source closes its
// channel. cfg.Elevation is overwritten with the generated location's elevation
// in generated-weather mode.
//
// version is shown on the dashboard. It returns an error if the station cannot be
// found or fetched, HomeKit setup fails, the station URL cannot be parsed, or the
// data source cannot be created or started; it returns nil when the observation
// channel closes.
func StartService(cfg *config.Config, version string) error {
	// Set log level
	logger.SetLogLevel(cfg.LogLevel)

	logger.Info("Starting Tempest HomeKit service...")

	// Step 1: Get station information based on mode
	var station *weather.Station
	var weatherGen *generator.WeatherGenerator

	if cfg.UDPStream {
		// UDP mode - fetch station details if internet is available and we have credentials
		if !cfg.DisableInternet && cfg.Token != "" && cfg.StationName != "" {
			logger.Info("UDP stream mode - fetching station details for forecast and metadata")
			stations, err := weather.GetStations(cfg.Token)
			if err != nil {
				logger.Info("Failed to fetch station details: %v (continuing with placeholder)", err)
				// Continue with placeholder station
				station = &weather.Station{
					StationID:   0,
					Name:        cfg.StationName,
					StationName: cfg.StationName,
				}
			} else {
				station = weather.FindStationByName(stations, cfg.StationName)
				if station == nil {
					logger.Info("Available stations:")
					for _, s := range stations {
						logger.Info("  - ID: %d, Name: '%s', StationName: '%s'", s.StationID, s.Name, s.StationName)
					}
					logger.Info("Station '%s' not found - using placeholder (forecast disabled)", cfg.StationName)
					station = &weather.Station{
						StationID:   0,
						Name:        cfg.StationName,
						StationName: cfg.StationName,
					}
				} else {
					logger.Info("Found station: %s (ID: %d) - forecast enabled", station.Name, station.StationID)
				}
			}
		} else {
			// Offline mode or missing credentials - create placeholder station
			logger.Info("UDP stream mode - will create UDP data source later")
			station = &weather.Station{
				StationID:   0,
				Name:        cfg.StationName,
				StationName: cfg.StationName,
			}
			if cfg.DisableInternet {
				logger.Info("Running in offline mode (--disable-internet) - all internet access disabled")
			}
		}
	} else if cfg.UseGeneratedWeather {
		// Use generated weather data for testing
		logger.Info("Using generated weather data for testing")
		weatherGen = generator.NewWeatherGenerator()

		// Create a fake station for the generated location
		location := weatherGen.GetLocation()
		station = &weather.Station{
			StationID:   99999, // Fake station ID
			Name:        location.Name,
			StationName: location.Name,
		}

		// Update the config elevation to match the generated location
		cfg.Elevation = location.Elevation
		logger.Info("Using generated location elevation: %.1f meters (%.0f feet)", location.Elevation, location.Elevation*3.28084)

		logger.Info("Generated weather location: %s (%s, %s season)",
			location.Name, location.ClimateZone, weatherGen.GetSeason().String())

		// Enable test patterns if requested
		if cfg.TestSensorRain {
			weatherGen.EnableTestPattern("rain")
			logger.Info("Test pattern enabled for rain sensor: 2-min cycle (0→0.5→2→10 mm/hr)")
		}
		if cfg.TestSensorWind {
			weatherGen.EnableTestPattern("wind")
			logger.Info("Test pattern enabled for wind sensor: 2-min cycle (1→4→12→22 m/s)")
		}
		if cfg.TestSensorTemp {
			weatherGen.EnableTestPattern("temp")
			logger.Info("Test pattern enabled for temperature sensor: 2-min cycle (0→15→25→38 °C)")
		}
		if cfg.TestSensorHumidity {
			weatherGen.EnableTestPattern("humidity")
			logger.Info("Test pattern enabled for humidity sensor: 2-min cycle (30→50→70→95 %%)")
		}
		if cfg.TestSensorPressure {
			weatherGen.EnableTestPattern("pressure")
			logger.Info("Test pattern enabled for pressure sensor: 2-min cycle (980→1000→1020→1040 mb)")
		}
		if cfg.TestSensorLux {
			weatherGen.EnableTestPattern("lux")
			logger.Info("Test pattern enabled for lux sensor: 2-min cycle (0→100→10000→50000 lux)")
		}
		if cfg.TestSensorUV {
			weatherGen.EnableTestPattern("uv")
			logger.Info("Test pattern enabled for UV sensor: 2-min cycle (0→2→7→11 index)")
		}
		if cfg.TestSensorLightning {
			weatherGen.EnableTestPattern("lightning")
			logger.Info("Test pattern enabled for lightning sensor: 2-min cycle (0→1@20km→5@5km→10@1km)")
		}
	} else {
		// Use real Tempest API data
		logger.Debug("Fetching stations from WeatherFlow API")
		stations, err := weather.GetStations(cfg.Token)
		if err != nil {
			return fmt.Errorf("failed to get stations: %v", err)
		}

		station = weather.FindStationByName(stations, cfg.StationName)
		if station == nil {
			logger.Info("Available stations:")
			for _, s := range stations {
				logger.Info("  - ID: %d, Name: '%s', StationName: '%s'", s.StationID, s.Name, s.StationName)
			}
			return fmt.Errorf("station '%s' not found", cfg.StationName)
		}

		logger.Info("Found station: %s (ID: %d)", station.Name, station.StationID)
	}

	// Parse sensor configuration (needed for both HomeKit and web server)
	sensorConfig := config.ParseSensorConfig(cfg.Sensors)

	// Conditionally setup HomeKit based on configuration
	var ws *homekit.WeatherSystemModern
	if cfg.DisableHomeKit {
		logger.Info("HomeKit services disabled - running in web console only mode")
	} else {
		// Setup HomeKit with sensor configuration
		logger.Debug("Initializing HomeKit accessories with sensor config: %s", cfg.Sensors)
		var setupErr error
		ws, setupErr = homekit.NewWeatherSystemModern(cfg.Pin, &sensorConfig, cfg.LogLevel)
		if setupErr != nil {
			return fmt.Errorf("failed to setup HomeKit: %v", setupErr)
		}

		// Start the HomeKit server
		logger.Debug("Starting weather system server")
		go func() {
			if err := ws.Start(); err != nil {
				logger.Error("HomeKit server error: %v", err)
			}
		}()

		logger.Info("HomeKit server started successfully with PIN: %s", cfg.Pin)
		logger.Debug("HomeKit - Bridge ready to accept connections")
		logger.Debug("HomeKit - Listening for iOS/HomeKit client connections...")
	}

	// Setup web dashboard
	var generatedWeatherInfo *web.GeneratedWeatherInfo
	if cfg.UseGeneratedWeather {
		location := weatherGen.GetLocation()
		generatedWeatherInfo = &web.GeneratedWeatherInfo{
			Enabled:     true,
			Location:    location.Name,
			Season:      weatherGen.GetSeason().String(),
			ClimateZone: location.ClimateZone,
		}
	}

	// Determine the effective station URL that will be used for weather data
	effectiveStationURL := cfg.StationURL
	if cfg.UseGeneratedWeather {
		// For generated weather, use a dummy URL and station since we don't need real ones
		effectiveStationURL = "http://localhost:8080/api/generate-weather"
		if station == nil {
			station = &weather.Station{
				StationID: 999999, // Dummy ID for generated weather
				Name:      "Generated Weather",
			}
		}
		logger.Info("Using generated weather - no station URL needed")
	} else if effectiveStationURL == "" {
		// Get station information from WeatherFlow API
		stations, err := weather.GetStations(cfg.Token)
		if err != nil {
			return fmt.Errorf("failed to get stations: %v", err)
		}
		station = weather.FindStationByName(stations, cfg.StationName)
		if station == nil {
			return fmt.Errorf("station '%s' not found", cfg.StationName)
		}
		effectiveStationURL = fmt.Sprintf("https://swd.weatherflow.com/swd/rest/observations/station/%d?token=%s", station.StationID, cfg.Token)
		logger.Info("Found station: %s (ID: %d)", station.Name, station.StationID)
	} else {
		// Station URL provided, extract station ID for web server
		// Parse station ID from URL like: https://swd.weatherflow.com/swd/rest/observations/station/12345?token=...
		parts := strings.Split(effectiveStationURL, "/station/")
		if len(parts) != 2 {
			return fmt.Errorf("invalid station URL format: %s", effectiveStationURL)
		}
		idPart := parts[1]
		if idx := strings.Index(idPart, "?"); idx > 0 {
			idPart = idPart[:idx]
		}
		stationID, err := strconv.Atoi(idPart)
		if err != nil {
			return fmt.Errorf("failed to parse station ID from URL: %v", err)
		}
		station = &weather.Station{
			StationID: stationID,
			Name:      cfg.StationName, // Use configured name
		}
		logger.Info("Using provided station URL for station ID: %d", stationID)
	}

	// Initialize alarm manager if alarms are configured and not disabled
	var alarmManager *alarm.Manager
	if cfg.Alarms != "" && !cfg.DisableAlarms && !cfg.TestAPILocal {
		logger.Info("Initializing alarm manager with config: %s", cfg.Alarms)
		var err error
		// Use station Name if StationName is empty (API sometimes only populates Name field)
		stationDisplayName := station.StationName
		if stationDisplayName == "" {
			stationDisplayName = station.Name
		}
		alarmManager, err = alarm.NewManager(cfg.Alarms, stationDisplayName)
		if err != nil {
			logger.Error("Failed to initialize alarm manager: %v", err)
			logger.Error("Continuing without alarms - fix configuration to enable alarm notifications")
		} else {
			logger.Info("Alarm manager initialized with %d alarms (%d enabled)",
				alarmManager.GetAlarmCount(), alarmManager.GetEnabledAlarmCount())

			// Set station location for sunrise/sunset calculations if available
			if station.Latitude != 0 || station.Longitude != 0 {
				alarmManager.SetLocation(station.Latitude, station.Longitude)
				logger.Debug("Alarm manager location set to station coordinates: %.4f, %.4f",
					station.Latitude, station.Longitude)
			}
		}
	}
	if alarmManager != nil {
		defer alarmManager.Stop()
	}

	// Create web server only if not disabled
	var webServer *web.WebServer
	if !cfg.DisableWebConsole {
		webServer = web.NewWebServer(cfg.WebPort, cfg.Elevation, cfg.LogLevel, station.StationID, cfg.UseWebStatus, version, effectiveStationURL, generatedWeatherInfo, weatherGen, cfg.Units, cfg.UnitsPressure, cfg.HistoryPoints, cfg.ChartHistoryHours, cfg.Alarms, cfg.DisableAlarms)
		webServer.SetStationName(station.Name)
		if alarmManager != nil {
			webServer.SetAlarmManager(alarmManager)
		}
		go func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("Web server panic recovered: %v", r)
				}
			}()
			logger.Info("Starting web dashboard on port %s", cfg.WebPort)
			if err := webServer.Start(); err != nil {
				logger.Error("Web server error: %v", err)
			}
		}()
	} else {
		logger.Info("Web console disabled (--disable-webconsole)")
	}

	// Update HomeKit status in web server
	var enabledSensors []string
	if sensorConfig.Temperature {
		enabledSensors = append(enabledSensors, "Temperature")
	}
	if sensorConfig.Humidity {
		enabledSensors = append(enabledSensors, "Humidity")
	}
	if sensorConfig.Light {
		enabledSensors = append(enabledSensors, "Light")
	}
	if sensorConfig.Wind {
		enabledSensors = append(enabledSensors, "Wind Speed", "Wind Direction")
	}
	if sensorConfig.Rain {
		enabledSensors = append(enabledSensors, "Rain")
	}
	if sensorConfig.Pressure {
		enabledSensors = append(enabledSensors, "Pressure")
	}
	if sensorConfig.UV {
		enabledSensors = append(enabledSensors, "UV")
	}
	if sensorConfig.Lightning {
		enabledSensors = append(enabledSensors, "Lightning")
	}

	// Build complete sensor list (all possible sensors, regardless of enabled/disabled status)
	allSensorsList := []string{
		"Temperature",
		"Humidity",
		"Light",
		"UV",
		"Wind Speed",
		"Wind Direction",
		"Rain",
		"Pressure",
		"Lightning",
	}

	// Update HomeKit status in web server based on whether HomeKit is enabled
	var homekitStatus map[string]interface{}
	if cfg.DisableHomeKit {
		homekitStatus = map[string]interface{}{
			"bridge":         false,
			"name":           "HomeKit Disabled",
			"accessories":    0,
			"accessoryNames": []string{},
			"allSensors":     allSensorsList,
			"sensorConfig":   "Web Console Only",
			"pin":            "N/A",
			"status":         "Disabled by --disable-homekit flag",
		}
	} else {
		// Get detailed HomeKit info from weather system if available
		if ws != nil {
			homekitStatus = ws.GetDetailedInfo()
			homekitStatus["sensorConfig"] = cfg.Sensors
			homekitStatus["allSensors"] = allSensorsList
			homekitStatus["accessoryNames"] = enabledSensors // Override with actual enabled sensors from config
		} else {
			homekitStatus = map[string]interface{}{
				"bridge":         true,
				"name":           "Tempest HomeKit Bridge",
				"accessories":    len(enabledSensors),
				"accessoryNames": enabledSensors,
				"allSensors":     allSensorsList,
				"sensorConfig":   cfg.Sensors,
				"pin":            cfg.Pin,
			}
		}
	}
	if webServer != nil {
		webServer.UpdateHomeKitStatus(homekitStatus)
	}

	// Preload historical data if requested
	if cfg.HistoryRead {
		var dataSourceDesc string
		if cfg.UseGeneratedWeather {
			dataSourceDesc = "from generated weather"
		} else {
			dataSourceDesc = "from Tempest API"
		}

		if cfg.LogLevel == "info" || cfg.LogLevel == "debug" {
			logger.Info("--history-read flag detected, preloading historical observations (up to HISTORY_POINTS points) %s...", dataSourceDesc)
		}

		// Create a progress callback function
		progressCallback := func(currentStep, totalSteps int, description string) {
			if webServer != nil {
				webServer.SetHistoryLoadingProgress(currentStep, totalSteps, description)
			}
		}

		var historicalObs []*weather.Observation
		var err error

		if cfg.UseGeneratedWeather && weatherGen != nil {
			// Generate historical data
			logger.Info("Generating %d historical weather data points...", cfg.HistoryPoints)
			historicalObs = weatherGen.GenerateHistoricalData(cfg.HistoryPoints)
			logger.Debug("Successfully generated %d historical observations", len(historicalObs))
		} else {
			// Use real historical data from API
			historicalObs, err = weather.GetHistoricalObservationsWithProgress(station.StationID, cfg.Token, cfg.LogLevel, progressCallback, cfg.HistoryPoints)
			if err != nil {
				logger.Error("Failed to fetch historical data: %v", err)
				if webServer != nil {
					webServer.SetHistoryLoadingComplete()
				}
			} else {
				logger.Debug("Successfully fetched %d historical observations", len(historicalObs))
			}
		}

		if err == nil && webServer != nil {
			webServer.SetHistoryLoadingProgress(2, 3, "Processing historical data...")

			// Send historical data to web server for charts
			for _, obs := range historicalObs {
				webServer.UpdateWeather(obs)
				logger.Debug("Added historical observation from %v", time.Unix(obs.Timestamp, 0))
			}

			// Complete the loading process
			webServer.SetHistoryLoadingComplete()

			webServer.SetHistoricalDataStatus(len(historicalObs))

			if cfg.LogLevel == "info" || cfg.LogLevel == "debug" {
				logger.Info("Historical data preload completed - loaded %d observations (up to configured HISTORY_POINTS)", len(historicalObs))
			}

			// NOTE: No need to reset test pattern rain anymore because we now preserve
			// cumulativeRain during historical generation instead of restoring it
		}
	}

	// UNIFIED DATA SOURCE APPROACH
	// Create UDP listener if needed (service layer handles this to avoid import cycles)
	var udpListener *udp.UDPListener
	if cfg.UDPStream {
		logger.Info("Creating UDP listener for UDP stream mode")
		udpListener = udp.NewUDPListener(cfg.HistoryPoints)
	}

	// Create appropriate data source using factory pattern. Use the
	// injectable DataSourceFactory so tests can override behavior.
	logger.Info("Creating data source...")
	var generatorPtr *generator.WeatherGenerator
	if cfg.UseGeneratedWeather && weatherGen != nil {
		generatorPtr = weatherGen
	}
	dataSource, err := DataSourceFactory(cfg, station, udpListener, generatorPtr)
	if err != nil {
		return fmt.Errorf("failed to create data source: %v", err)
	}
	defer func() {
		if err := dataSource.Stop(); err != nil {
			logger.Error("dataSource stop error: %v", err)
		}
	}()

	// Wire up status manager for UDP data source if web server is enabled
	if webServer != nil && cfg.UDPStream {
		if udpDataSource, ok := dataSource.(*weather.UDPDataSource); ok {
			statusManager := webServer.GetStatusManager()
			if statusManager != nil {
				udpDataSource.SetStatusManager(statusManager)
				logger.Info("Status manager connected to UDP data source")
			}
		}
	}

	// Start the data source
	logger.Info("Starting data source: %s", dataSource.GetType())
	obsChan, err := dataSource.Start()
	if err != nil {
		return fmt.Errorf("failed to start data source: %v", err)
	}

	// Set initial data source status in web server (before any observations arrive)
	if webServer != nil {
		initialStatus := dataSource.GetStatus()
		webServer.UpdateDataSourceStatus(initialStatus)
		logger.Debug("Initial data source status set: type=%s", initialStatus.Type)
	}

	// Main observation processing loop - unified for all data sources!
	logger.Info("Starting unified observation processing loop")
	for obs := range obsChan {
		logger.Debug("Processing observation from %s data source", dataSource.GetType())

		// Update HomeKit sensors (if enabled)
		if ws != nil {
			ws.UpdateSensor("Wind Speed", obs.WindAvg)
			ws.UpdateSensor("Wind Gust", obs.WindGust)
			ws.UpdateSensor("Wind Direction", obs.WindDirection)
			ws.UpdateSensor("Air Temperature", obs.AirTemperature)
			ws.UpdateSensor("Relative Humidity", obs.RelativeHumidity)
			ws.UpdateSensor("Ambient Light", obs.Illuminance)
			ws.UpdateSensor("UV Index", float64(obs.UV))
			ws.UpdateSensor("Rain Accumulation", obs.RainAccumulated)
			ws.UpdateSensor("Precipitation Type", float64(obs.PrecipitationType))
			ws.UpdateSensor("Lightning Count", float64(obs.LightningStrikeCount))
			ws.UpdateSensor("Lightning Distance", obs.LightningStrikeAvg)
			logger.Debug("HomeKit sensors updated")
		}

		// Update web server
		if webServer != nil {
			webServer.UpdateWeather(&obs)
			logger.Debug("Web server updated")

			// Update forecast from data source (if available)
			if forecast := dataSource.GetForecast(); forecast != nil {
				webServer.UpdateForecast(forecast)
				logger.Debug("Forecast updated")
			}

			// Update data source status in web server
			status := dataSource.GetStatus()
			webServer.UpdateDataSourceStatus(status)
			logger.Debug("Data source status updated")
		}

		// Process alarms if alarm manager is initialized
		if alarmManager != nil {
			alarmManager.ProcessObservation(&obs)
		}

		// Log observation details
		if cfg.LogLevel == "info" || cfg.LogLevel == "debug" {
			logger.Info("%s data - Temp: %.1f°C, Humidity: %.1f%%, Wind: %.1f m/s, Lux: %.0f",
				dataSource.GetType(), obs.AirTemperature, obs.RelativeHumidity, obs.WindAvg, obs.Illuminance)
		}
	}

	logger.Info("Observation processing loop ended")
	return nil
}
