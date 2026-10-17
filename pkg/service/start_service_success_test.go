package service

import (
	"testing"
	"time"

	"tempest-homekit-go/pkg/config"
	"tempest-homekit-go/pkg/weather"
)

// fakeUDPListener2 implements weather.UDPListener for testing CreateDataSource UDP branch; every method is a no-op and ObservationChannel returns an already-closed channel.
type fakeUDPListener2 struct{}

// Start is a no-op that returns nil.
func (f *fakeUDPListener2) Start() error                               { return nil }
func (f *fakeUDPListener2) Stop() error                                { return nil }
func (f *fakeUDPListener2) GetLatestObservation() *weather.Observation { return nil }
func (f *fakeUDPListener2) GetStats() (int64, time.Time, string, string) {
	return 0, time.Time{}, "", ""
}
func (f *fakeUDPListener2) GetObservations() []weather.Observation { return nil }
func (f *fakeUDPListener2) IsReceivingData() bool                  { return false }
func (f *fakeUDPListener2) ObservationChannel() <-chan weather.Observation {
	ch := make(chan weather.Observation)
	close(ch)
	return ch
}
func (f *fakeUDPListener2) GetDeviceStatus() interface{} { return nil }
func (f *fakeUDPListener2) GetHubStatus() interface{}    { return nil }

// fakeDataSource2 emits one observation then closes the channel; its other methods return zero values.
type fakeDataSource2 struct{}

// Start returns a channel that delivers one observation and then closes.
func (f *fakeDataSource2) Start() (<-chan weather.Observation, error) {
	ch := make(chan weather.Observation)
	go func() {
		ch <- weather.Observation{Timestamp: time.Now().Unix(), AirTemperature: 21.0}
		close(ch)
	}()
	return ch, nil
}
func (f *fakeDataSource2) Stop() error                                { return nil }
func (f *fakeDataSource2) GetLatestObservation() *weather.Observation { return nil }
func (f *fakeDataSource2) GetForecast() *weather.ForecastResponse     { return nil }
func (f *fakeDataSource2) GetStatus() weather.DataSourceStatus        { return weather.DataSourceStatus{} }
func (f *fakeDataSource2) GetType() weather.DataSourceType            { return weather.DataSourceGenerated }

// TestCreateDataSource_UDPBranch checks that UDP mode with a listener yields a UDP data source.
func TestCreateDataSource_UDPBranch(t *testing.T) {
	cfg := &config.Config{UDPStream: true}
	listener := &fakeUDPListener2{}

	ds, err := CreateDataSource(cfg, &weather.Station{StationID: 1, StationName: "s"}, listener, nil)
	if err != nil {
		t.Fatalf("expected UDP datasource creation to succeed, got error: %v", err)
	}
	if ds.GetType() != weather.DataSourceUDP {
		t.Fatalf("expected DataSourceUDP type, got %v", ds.GetType())
	}
}

// TestStartService_SucceedsWithFakeDataSource checks that StartService returns nil after a one-observation fake source closes.
func TestStartService_SucceedsWithFakeDataSource(t *testing.T) {
	origFactory := DataSourceFactory
	DataSourceFactory = func(cfg *config.Config, station *weather.Station, udpListener interface{}, genParam interface{}) (weather.DataSource, error) {
		return &fakeDataSource2{}, nil
	}
	defer func() { DataSourceFactory = origFactory }()

	cfg := &config.Config{
		DisableHomeKit:      true,
		DisableWebConsole:   true,
		UseGeneratedWeather: true,
	}

	// StartService should return nil; the fake data source emits one observation then closes
	if err := StartService(cfg, "vtest"); err != nil {
		t.Fatalf("expected StartService to succeed with fake data source, got error: %v", err)
	}
}
