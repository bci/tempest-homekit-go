// This file provides a generated data source for synthetic weather data.
// This data source directly uses the weather generator without HTTP calls.

package weather

import (
	"time"

	"tempest-homekit-go/pkg/generator"
	"tempest-homekit-go/pkg/logger"
	"tempest-homekit-go/pkg/types"
)

// GeneratedDataSource implements the DataSource interface for generated weather data.
// It directly uses a weather generator instead of making HTTP requests, producing
// one observation immediately on Start and then one every 60 seconds.
//
// It holds its own copy of the generator (passed by value). Its fields are not
// protected by a mutex, so status getters race with the generation goroutine.
//
// Fields: stationID, token and stationName identify the (fake) station;
// generator produces observations; running is true between Start and Stop;
// stopChan is closed by Stop; observationChan (buffer 10) delivers
// observations; latestObservation, lastUpdate and observationCount track the
// most recent generated observation.
type GeneratedDataSource struct {
	stationID         int
	token             string
	stationName       string
	generator         generator.WeatherGenerator
	running           bool
	stopChan          chan struct{}
	observationChan   chan types.Observation
	latestObservation *types.Observation
	lastUpdate        time.Time
	observationCount  int64
}

// NewGeneratedDataSource creates a new generated data source for the given station
// identity, using a copy of gen. token is stored but not used. Call Start to
// begin generating.
func NewGeneratedDataSource(stationID int, token, stationName string, gen generator.WeatherGenerator) *GeneratedDataSource {
	return &GeneratedDataSource{
		stationID:       stationID,
		token:           token,
		stationName:     stationName,
		generator:       gen,
		stopChan:        make(chan struct{}),
		observationChan: make(chan Observation, 10), // Buffer for observations
	}
}

// Start begins generating weather observations in a background goroutine and
// returns the observation channel. It never fails. The channel is not closed by
// Stop.
func (g *GeneratedDataSource) Start() (<-chan types.Observation, error) {
	g.running = true

	// Start the generation loop in a goroutine
	go g.generationLoop()

	return g.observationChan, nil
}

// Stop halts the data source by stopping the generation loop. It is safe to call
// more than once and always returns nil.
func (g *GeneratedDataSource) Stop() error {
	if g.running {
		g.running = false
		close(g.stopChan)
	}
	return nil
}

// GetLatestObservation returns the most recent observation, or nil before the
// first one. The pointer is shared with the data source.
func (g *GeneratedDataSource) GetLatestObservation() *Observation {
	return g.latestObservation
}

// GetForecast returns forecast data (generated data doesn't have forecasts)
func (g *GeneratedDataSource) GetForecast() *ForecastResponse {
	return nil
}

// GetStatus returns the current status, including the generator's location,
// season and climate zone.
func (g *GeneratedDataSource) GetStatus() DataSourceStatus {
	location := g.generator.GetLocation()
	season := g.generator.GetSeason()

	return DataSourceStatus{
		Type:             DataSourceGenerated,
		Active:           g.running,
		LastUpdate:       g.lastUpdate,
		ObservationCount: g.observationCount,
		StationName:      g.stationName,
		Location:         location.Name,
		Season:           season.String(),
		ClimateZone:      location.ClimateZone,
	}
}

// GetType returns the data source type
func (g *GeneratedDataSource) GetType() DataSourceType {
	return DataSourceGenerated
}

// generationLoop generates weather observations every 60 seconds, starting with
// one immediately, until stopChan is closed.
func (g *GeneratedDataSource) generationLoop() {
	logger.Info("Starting generated data source generation loop (60 second interval)")

	// Generate initial observation
	g.generateObservation()

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-g.stopChan:
			logger.Info("Generated data source generation loop stopped")
			return

		case <-ticker.C:
			g.generateObservation()
		}
	}
}

// generateObservation creates and sends a new weather observation: it records it
// as the latest and, while running, offers it to observationChan without
// blocking (dropping it if the channel is full).
func (g *GeneratedDataSource) generateObservation() {
	logger.Debug("Generated data source: generating observation")

	// Generate a fresh observation
	obs := g.generator.GenerateObservation()
	if obs == nil {
		logger.Error("Failed to generate weather observation")
		return
	}

	// Update internal state
	g.latestObservation = obs
	g.lastUpdate = time.Now()
	g.observationCount++

	// Send to channel if running (non-blocking)
	if g.running {
		select {
		case g.observationChan <- *obs:
			logger.Debug("Generated observation sent to channel")
		default:
			logger.Debug("Generated observation channel full, skipping")
		}
	}
}
