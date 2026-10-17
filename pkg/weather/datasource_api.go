// This file provides the API data source implementation.

package weather

import (
	"net/url"
	"sync"
	"time"

	"tempest-homekit-go/pkg/logger"
)

// APIDataSource implements DataSource for WeatherFlow API polling. It fetches an
// observation immediately on Start and then every 60 seconds, and the forecast
// on Start and then every 30 minutes. With a custom URL it polls that URL
// instead of the WeatherFlow station endpoint, and if the URL's path is the
// generated-weather path it reports itself as DataSourceGenerated and skips
// forecasts. It is safe for concurrent use.
//
// Fields: stationID, token and stationName identify the WeatherFlow station;
// customURL, generated and generatedPath are described inline; mu guards the
// fields below it; latestObservation and latestForecast hold the most recent
// data; observationChan (buffer 100) delivers observations and is closed by
// Stop; stopChan is closed by Stop to end pollLoop; observationCount and
// lastUpdate count successful fetches; running is true between Start and Stop;
// wg lets Stop wait for pollLoop to exit.
type APIDataSource struct {
	stationID     int
	token         string
	stationName   string
	customURL     string // For custom station URLs (generated weather, etc.)
	generated     bool   // true when this API data source is using generated weather endpoint
	generatedPath string // configured path used to identify generated weather endpoint

	mu                sync.RWMutex
	latestObservation *Observation
	latestForecast    *ForecastResponse
	observationChan   chan Observation
	stopChan          chan struct{}
	observationCount  int64
	lastUpdate        time.Time
	running           bool
	wg                sync.WaitGroup
}

// APIDataSourceOptions holds optional parameters for creating APIDataSource.
//
// Fields:
//
//   - CustomURL, if non-empty, is polled instead of the WeatherFlow station
//     observations endpoint. It must serve the same JSON shape.
//   - GeneratedPath is the URL path of the local generated-weather endpoint
//     (default "/api/generate-weather"); a CustomURL with exactly this path
//     marks the source as generated.
type APIDataSourceOptions struct {
	CustomURL     string
	GeneratedPath string
}

// NewAPIDataSource creates a new API-based data source with options. token is
// the WeatherFlow access token (unused when opts.CustomURL is set, except for
// forecasts). Call Start to begin polling.
func NewAPIDataSource(stationID int, token, stationName string, opts APIDataSourceOptions) *APIDataSource {
	a := &APIDataSource{
		stationID:       stationID,
		token:           token,
		stationName:     stationName,
		customURL:       opts.CustomURL,
		generatedPath:   opts.GeneratedPath,
		observationChan: make(chan Observation, 100),
		stopChan:        make(chan struct{}),
	}

	// Default generatedPath when empty
	if a.generatedPath == "" {
		a.generatedPath = "/api/generate-weather"
	}

	// Determine if this data source points to the generated weather endpoint by
	// parsing the URL and comparing the path exactly to the configured generatedPath.
	if a.customURL != "" {
		if u, err := url.Parse(a.customURL); err == nil {
			if u.Path == a.generatedPath {
				a.generated = true
			}
		}
	}

	return a
}

// Start begins polling the API for weather data in a background goroutine and
// returns the observation channel. Calling it while already running returns the
// same channel. It never fails; fetch errors are logged.
func (a *APIDataSource) Start() (<-chan Observation, error) {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return a.observationChan, nil
	}
	a.running = true
	a.wg.Add(1)
	a.mu.Unlock()

	// Start polling goroutine
	go a.pollLoop()

	return a.observationChan, nil
}

// Stop gracefully shuts down the API data source: it stops the polling goroutine,
// waits for it to exit and then closes the observation channel. It is a no-op if
// not running and always returns nil. The source cannot be restarted afterwards.
func (a *APIDataSource) Stop() error {
	a.mu.Lock()
	if !a.running {
		a.mu.Unlock()
		return nil
	}

	a.running = false
	a.mu.Unlock()

	// Signal stop and wait for goroutine to finish
	close(a.stopChan)
	a.wg.Wait()

	// Now safe to close the channel
	a.mu.Lock()
	close(a.observationChan)
	a.mu.Unlock()

	logger.Info("API data source stopped")
	return nil
}

// GetLatestObservation returns the most recent observation, or nil before the first successful fetch.
func (a *APIDataSource) GetLatestObservation() *Observation {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.latestObservation
}

// GetForecast returns the latest forecast data, or nil if none has been fetched (always nil for generated weather).
func (a *APIDataSource) GetForecast() *ForecastResponse {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.latestForecast
}

// GetStatus returns the current status of the API data source; its Type is the
// same as GetType.
func (a *APIDataSource) GetStatus() DataSourceStatus {
	a.mu.RLock()
	defer a.mu.RUnlock()

	sourceType := DataSourceAPI
	if a.customURL != "" {
		if a.generated {
			sourceType = DataSourceGenerated
		} else {
			sourceType = DataSourceCustomURL
		}
	}

	return DataSourceStatus{
		Type:             sourceType,
		Active:           a.running,
		LastUpdate:       a.lastUpdate,
		ObservationCount: a.observationCount,
		StationName:      a.stationName,
		CustomURL:        a.customURL,
	}
}

// GetType returns the data source type: DataSourceGenerated for the generated
// weather endpoint, DataSourceCustomURL for any other custom URL, and
// DataSourceAPI otherwise.
func (a *APIDataSource) GetType() DataSourceType {
	if a.customURL != "" {
		if a.generated {
			return DataSourceGenerated
		}
		return DataSourceCustomURL
	}
	return DataSourceAPI
}

// pollLoop is the main polling loop that fetches data every 60 seconds, plus the
// forecast every 30 ticks, until stopChan is closed.
func (a *APIDataSource) pollLoop() {
	logger.Info("Starting API data source polling loop (60 second interval)")

	// Initial fetch
	a.fetchObservation()
	a.fetchForecast()

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	forecastCounter := 0

	for {
		select {
		case <-a.stopChan:
			logger.Info("API polling loop stopped")
			a.wg.Done()
			return

		case <-ticker.C:
			a.fetchObservation()

			// Update forecast every 30 minutes (30 ticks)
			forecastCounter++
			if forecastCounter >= 30 {
				a.fetchForecast()
				forecastCounter = 0
			}
		}
	}
}

// fetchObservation retrieves a single observation from the API (or the custom
// URL), records it as the latest and, while running, offers it to the channel
// without blocking. Errors are logged and the observation is skipped.
func (a *APIDataSource) fetchObservation() {
	logger.Debug("API data source: fetching observation")

	var obs *Observation
	var err error

	if a.customURL != "" {
		// Use custom station URL (generated weather, etc.)
		obs, err = GetObservationFromURL(a.customURL)
		if err != nil {
			logger.Error("Error getting observation from URL %s: %v", a.customURL, err)
			return
		}
		logger.Debug("Successfully fetched observation from custom URL: %s", a.customURL)
	} else {
		// Use real Tempest API
		obs, err = GetObservation(a.stationID, a.token)
		if err != nil {
			logger.Error("Error getting observation from API: %v", err)
			return
		}
		logger.Debug("Successfully fetched observation from WeatherFlow API")
	}

	if obs != nil {
		a.mu.Lock()
		a.latestObservation = obs
		a.lastUpdate = time.Now()
		a.observationCount++
		obsChan := a.observationChan
		isRunning := a.running
		a.mu.Unlock()

		// Only send if still running
		if isRunning {
			// Send to channel (non-blocking)
			select {
			case obsChan <- *obs:
				logger.Debug("Observation sent to channel")
			default:
				logger.Debug("Observation channel full, skipping")
			}
		}
	}
}

// fetchForecast retrieves forecast data from the API for stationID and stores it.
// It is skipped for generated weather; errors are logged and the previous
// forecast is kept.
func (a *APIDataSource) fetchForecast() {
	// Skip forecast for generated weather
	if a.generated {
		logger.Debug("Skipping forecast fetch for generated weather")
		return
	}

	logger.Debug("API data source: fetching forecast")

	forecast, err := GetForecast(a.stationID, a.token)
	if err != nil {
		logger.Error("Error getting forecast: %v", err)
		return
	}

	a.mu.Lock()
	a.latestForecast = forecast
	a.mu.Unlock()

	logger.Debug("Successfully fetched forecast data")
}
