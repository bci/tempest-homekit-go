// This file defines the unified data source interface for weather observations.
// All weather data sources (API, UDP, Generated) implement this interface.

package weather

import (
	"time"
)

// DataSource is the unified interface for all weather data sources.
// Implementations include: API polling, UDP stream, and generated weather.
// StartService consumes any of them the same way: Start once, range over the
// returned channel, and query status and forecast as observations arrive.
type DataSource interface {
	// Start begins the data source and returns a channel for observations
	Start() (<-chan Observation, error)

	// Stop gracefully shuts down the data source
	Stop() error

	// GetLatestObservation returns the most recent observation
	GetLatestObservation() *Observation

	// GetForecast returns forecast data (may return nil if not available)
	GetForecast() *ForecastResponse

	// GetStatus returns data source status information
	GetStatus() DataSourceStatus

	// GetType returns the type of data source
	GetType() DataSourceType
}

// DataSourceType identifies the type of weather data source; the values are reported in status JSON.
type DataSourceType string

const (
	// DataSourceAPI represents WeatherFlow API polling
	DataSourceAPI DataSourceType = "api"

	// DataSourceUDP represents local UDP broadcast listener
	DataSourceUDP DataSourceType = "udp"

	// DataSourceGenerated represents simulated weather data
	DataSourceGenerated DataSourceType = "generated"

	// DataSourceCustomURL represents a custom station URL
	DataSourceCustomURL DataSourceType = "custom-url"
)

// DataSourceStatus provides unified status information for any data source, for
// the web dashboard's /api/status response.
//
// Fields:
//
//   - Type identifies the data source.
//   - Active reports whether the source is running (and, for UDP, receiving).
//   - LastUpdate is when the last observation arrived (zero if none).
//   - ObservationCount is the number of observations received or stored.
//   - The remaining fields are optional and only filled by the source types
//     noted in their inline comments; StationName is set by every source.
type DataSourceStatus struct {
	Type             DataSourceType `json:"type"`
	Active           bool           `json:"active"`
	LastUpdate       time.Time      `json:"lastUpdate"`
	ObservationCount int64          `json:"observationCount"`

	// Optional fields depending on source type
	StationName  string `json:"stationName,omitempty"`
	StationIP    string `json:"stationIP,omitempty"`    // For UDP
	SerialNumber string `json:"serialNumber,omitempty"` // For UDP
	PacketCount  int64  `json:"packetCount,omitempty"`  // For UDP
	Location     string `json:"location,omitempty"`     // For Generated
	Season       string `json:"season,omitempty"`       // For Generated
	ClimateZone  string `json:"climateZone,omitempty"`  // For Generated
	CustomURL    string `json:"customURL,omitempty"`    // For Custom URL
}
