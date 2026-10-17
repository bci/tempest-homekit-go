// This file provides the data source factory for creating appropriate weather
// data sources (see the package documentation in service.go).

package service

import (
	"fmt"

	"tempest-homekit-go/pkg/config"
	"tempest-homekit-go/pkg/generator"
	"tempest-homekit-go/pkg/logger"
	"tempest-homekit-go/pkg/weather"
)

// DataSourceFactoryFunc is the signature used to create a DataSource. Tests can
// override the package-level DataSourceFactory variable to inject fake data
// sources without changing production code.
type DataSourceFactoryFunc func(cfg *config.Config, station *weather.Station, udpListener interface{}, genParam interface{}) (weather.DataSource, error)

// DataSourceFactory is the default factory used by StartService. It points to
// CreateDataSource but can be replaced in tests.
var DataSourceFactory DataSourceFactoryFunc = CreateDataSource

// CreateDataSource examines config flags and returns the appropriate DataSource implementation.
// This is the only place in the codebase that needs to know about different data source types.
// For UDP mode, the udpListener parameter must be provided as interface{} (pass nil for other modes).
// For generated weather, the generator parameter can be provided as interface{} (pass nil to create new).
//
// The first matching mode wins: cfg.UDPStream (UDP broadcast data source;
// the WeatherFlow token is passed only when internet access is allowed, for
// forecasts), then a non-empty cfg.StationURL (API data source polling that
// URL), then cfg.UseGeneratedWeather (generated data source with a fake
// station ID 99999; station is ignored), and otherwise the WeatherFlow API
// data source for station.
//
// udpListener must implement weather.UDPListener (normally a
// *udp.UDPListener); genParam, if non-nil, should be a
// *generator.WeatherGenerator and is replaced by a new generator (with a
// warning) if it is not. It returns an error if UDP mode has a nil or
// wrongly typed listener, or if API mode has a nil station.
func CreateDataSource(cfg *config.Config, station *weather.Station, udpListener interface{}, genParam interface{}) (weather.DataSource, error) {
	// Priority order:
	// 1. UDP Stream (if enabled)
	// 2. Custom Station URL (if provided)
	// 3. Generated Weather (if enabled)
	// 4. WeatherFlow API (default)

	if cfg.UDPStream {
		if udpListener == nil {
			return nil, fmt.Errorf("UDP listener required for UDP stream mode")
		}

		logger.Info("Creating UDP data source (offline mode: %v)", cfg.DisableInternet)

		// Create UDP data source wrapper
		var stationID int
		var token string
		if station != nil {
			stationID = station.StationID
		}
		if !cfg.DisableInternet {
			token = cfg.Token
		}

		// Type assert to the interface defined in weather package
		listener, ok := udpListener.(weather.UDPListener)
		if !ok {
			return nil, fmt.Errorf("invalid UDP listener type")
		}

		dataSource := weather.NewUDPDataSource(listener, cfg.DisableInternet, stationID, token)
		logger.Info("UDP data source created (port 50222)")
		return dataSource, nil
	}

	if cfg.StationURL != "" {
		logger.Info("Creating API data source with custom URL: %s", cfg.StationURL)

		// Custom station URL (generated weather, etc.)
		var stationID int
		var stationName string
		if station != nil {
			stationID = station.StationID
			stationName = station.StationName
		}

		dataSource := weather.NewAPIDataSource(stationID, cfg.Token, stationName, weather.APIDataSourceOptions{CustomURL: cfg.StationURL, GeneratedPath: cfg.GeneratedWeatherPath})
		logger.Info("API data source created with custom URL")
		return dataSource, nil
	}

	if cfg.UseGeneratedWeather {
		logger.Info("Creating generated data source")

		var gen *generator.WeatherGenerator
		if genParam != nil {
			// Use the provided generator
			if g, ok := genParam.(*generator.WeatherGenerator); ok {
				gen = g
			} else {
				logger.Warn("Invalid generator type provided, creating new one")
				gen = generator.NewWeatherGenerator()
			}
		} else {
			// Create a new generator
			gen = generator.NewWeatherGenerator()
		}

		// Create a fake station for the generated location
		location := gen.GetLocation()
		station = &weather.Station{
			StationID: 99999, // Fake station ID
			// Name field not needed for generated weather source
			StationName: location.Name,
		}

		dataSource := weather.NewGeneratedDataSource(station.StationID, cfg.Token, station.StationName, *gen)
		logger.Info("Generated data source created")
		return dataSource, nil
	}

	// Default: WeatherFlow API
	if station == nil {
		return nil, fmt.Errorf("station required for API data source")
	}

	logger.Info("Creating API data source for station: %s (ID: %d)", station.StationName, station.StationID)
	dataSource := weather.NewAPIDataSource(station.StationID, cfg.Token, station.StationName, weather.APIDataSourceOptions{CustomURL: "", GeneratedPath: cfg.GeneratedWeatherPath})
	logger.Info("WeatherFlow API data source created")
	return dataSource, nil
}
