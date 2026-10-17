package service

import (
	"testing"

	"tempest-homekit-go/pkg/config"
	"tempest-homekit-go/pkg/weather"
	"time"
)

// simpleFakeUDP implements weather.UDPListener for testing CreateDataSource UDP path.
// Every method is a no-op returning zero values; ObservationChannel returns an
// already-closed channel.
type simpleFakeUDP struct{}

// Start is a no-op that returns nil.
func (s *simpleFakeUDP) Start() error                                 { return nil }
func (s *simpleFakeUDP) Stop() error                                  { return nil }
func (s *simpleFakeUDP) GetLatestObservation() *weather.Observation   { return nil }
func (s *simpleFakeUDP) GetStats() (int64, time.Time, string, string) { return 0, time.Time{}, "", "" }
func (s *simpleFakeUDP) GetObservations() []weather.Observation       { return nil }
func (s *simpleFakeUDP) ObservationChannel() <-chan weather.Observation {
	ch := make(chan weather.Observation)
	close(ch)
	return ch
}
func (s *simpleFakeUDP) IsReceivingData() bool        { return false }
func (s *simpleFakeUDP) GetDeviceStatus() interface{} { return nil }
func (s *simpleFakeUDP) GetHubStatus() interface{}    { return nil }

// TestCreateDataSource_UDPWithListener checks that UDP mode with a listener yields a UDP data source.
func TestCreateDataSource_UDPWithListener(t *testing.T) {
	cfg := &config.Config{UDPStream: true, DisableInternet: true}
	station := &weather.Station{StationID: 123, StationName: "Test"}
	ds, err := CreateDataSource(cfg, station, &simpleFakeUDP{}, nil)
	if err != nil {
		t.Fatalf("CreateDataSource UDP failed: %v", err)
	}
	if ds.GetType() != weather.DataSourceUDP {
		t.Fatalf("expected UDP data source, got %s", ds.GetType())
	}
}

// TestCreateDataSource_UseGeneratedWeatherDefaults checks that generated-weather mode yields a generated data source.
func TestCreateDataSource_UseGeneratedWeatherDefaults(t *testing.T) {
	cfg := &config.Config{UseGeneratedWeather: true}
	station := &weather.Station{StationID: 0, StationName: "Gen"}
	ds, err := CreateDataSource(cfg, station, nil, nil)
	if err != nil {
		t.Fatalf("CreateDataSource generated failed: %v", err)
	}
	if ds.GetType() != weather.DataSourceGenerated {
		t.Fatalf("expected Generated data source, got %s", ds.GetType())
	}
}
