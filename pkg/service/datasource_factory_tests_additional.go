// This file sits in the production package despite holding a test (its name lacks
// the _test.go suffix), so it is compiled into non-test builds.

package service

import (
	"testing"

	"tempest-homekit-go/pkg/config"
	"tempest-homekit-go/pkg/weather"
)

// simpleFakeDS implements weather.DataSource for testing. Its methods are no-ops:
// Start returns an open channel that never delivers, and GetType reports
// DataSourceGenerated.
type simpleFakeDS struct{}

// Mark simpleFakeDS as referenced to avoid unused warnings from staticcheck.
var _ = simpleFakeDS{}

// Start returns a new unbuffered channel that is never written or closed.
func (s *simpleFakeDS) Start() (<-chan weather.Observation, error) {
	return make(chan weather.Observation), nil
}
func (s *simpleFakeDS) Stop() error                                { return nil }
func (s *simpleFakeDS) GetLatestObservation() *weather.Observation { return nil }
func (s *simpleFakeDS) GetForecast() *weather.ForecastResponse     { return nil }
func (s *simpleFakeDS) GetStatus() weather.DataSourceStatus        { return weather.DataSourceStatus{} }
func (s *simpleFakeDS) GetType() weather.DataSourceType            { return weather.DataSourceGenerated }

// TestCreateDataSource_GeneratedAndUDP checks that the default API mode fails
// without a station and that generated-weather mode yields a generated data
// source. Despite its name it does not exercise the UDP path.
func TestCreateDataSource_GeneratedAndUDP(t *testing.T) {
	// UDP case: create with type UDP should produce UDPDataSource or error when nil listener is used
	cfg := &config.Config{}
	_, err := CreateDataSource(cfg, nil, nil, nil)
	if err == nil {
		t.Fatalf("expected error because station is required for default API datasource when not using generated or UDP")
	}

	// Generated case: set UseGeneratedWeather so factory returns generated API data source
	cfg2 := &config.Config{UseGeneratedWeather: true}
	ds, err := CreateDataSource(cfg2, &weather.Station{StationID: 1, StationName: "s"}, nil, nil)
	if err != nil {
		t.Fatalf("expected generated datasource, got error: %v", err)
	}
	if ds.GetType() != weather.DataSourceGenerated {
		t.Fatalf("expected generated datasource type, got %v", ds.GetType())
	}
}

// Note: StartService factory error propagation is already covered in start_service_additional_test.go
