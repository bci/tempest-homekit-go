package service

import (
	"errors"
	"testing"
	"time"

	"tempest-homekit-go/pkg/config"
	"tempest-homekit-go/pkg/weather"
)

// fakeDataSource implements weather.DataSource for tests; its channel delivers
// two observations and then closes. Stop is a no-op and GetType reports
// DataSourceGenerated.
type fakeDataSource struct{}

// Start returns a channel that delivers two observations and then closes.
func (f *fakeDataSource) Start() (<-chan weather.Observation, error) {
	ch := make(chan weather.Observation)
	go func() {
		// send two observations then close
		ch <- weather.Observation{Timestamp: time.Now().Unix(), AirTemperature: 20.0}
		ch <- weather.Observation{Timestamp: time.Now().Add(-time.Minute).Unix(), AirTemperature: 21.0}
		close(ch)
	}()
	return ch, nil
}
func (f *fakeDataSource) Stop() error { return nil }
func (f *fakeDataSource) GetLatestObservation() *weather.Observation {
	return &weather.Observation{AirTemperature: 20.0}
}
func (f *fakeDataSource) GetForecast() *weather.ForecastResponse { return nil }
func (f *fakeDataSource) GetStatus() weather.DataSourceStatus {
	return weather.DataSourceStatus{Type: weather.DataSourceGenerated, Active: true}
}
func (f *fakeDataSource) GetType() weather.DataSourceType { return weather.DataSourceGenerated }

// failingStartDS returns an error on Start(); its other methods return zero values.
type failingStartDS struct{}

// Start always fails with "start failed".
func (f *failingStartDS) Start() (<-chan weather.Observation, error) {
	return nil, errors.New("start failed")
}
func (f *failingStartDS) Stop() error                                { return nil }
func (f *failingStartDS) GetLatestObservation() *weather.Observation { return nil }
func (f *failingStartDS) GetForecast() *weather.ForecastResponse     { return nil }
func (f *failingStartDS) GetStatus() weather.DataSourceStatus {
	return weather.DataSourceStatus{Type: weather.DataSourceGenerated}
}
func (f *failingStartDS) GetType() weather.DataSourceType { return weather.DataSourceGenerated }

// TestStartService_WithFakeDataSource checks that StartService processes a fake source in generated mode and returns nil when it closes.
func TestStartService_WithFakeDataSource(t *testing.T) {
	orig := DataSourceFactory
	defer func() { DataSourceFactory = orig }()

	DataSourceFactory = func(cfg *config.Config, station *weather.Station, udpListener interface{}, genParam interface{}) (weather.DataSource, error) {
		return &fakeDataSource{}, nil
	}

	cfg := &config.Config{
		DisableHomeKit:      true,
		DisableWebConsole:   true,
		UseGeneratedWeather: true,
		LogLevel:            "debug",
		HistoryPoints:       10,
	}

	if err := StartService(cfg, "vtest"); err != nil {
		t.Fatalf("StartService returned error: %v", err)
	}
}

// TestStartService_FactoryError checks that a data source factory error is returned by StartService.
func TestStartService_FactoryError(t *testing.T) {
	orig := DataSourceFactory
	defer func() { DataSourceFactory = orig }()

	DataSourceFactory = func(cfg *config.Config, station *weather.Station, udpListener interface{}, genParam interface{}) (weather.DataSource, error) {
		return nil, errors.New("factory failed")
	}

	cfg := &config.Config{DisableHomeKit: true, DisableWebConsole: true, UseGeneratedWeather: true}
	if err := StartService(cfg, "vtest"); err == nil {
		t.Fatalf("expected StartService to return error when factory fails")
	}
}

// TestStartService_DataSourceStartError checks that a data source Start error is returned by StartService.
func TestStartService_DataSourceStartError(t *testing.T) {
	orig := DataSourceFactory
	defer func() { DataSourceFactory = orig }()

	DataSourceFactory = func(cfg *config.Config, station *weather.Station, udpListener interface{}, genParam interface{}) (weather.DataSource, error) {
		return &failingStartDS{}, nil
	}

	cfg := &config.Config{DisableHomeKit: true, DisableWebConsole: true, UseGeneratedWeather: true}
	if err := StartService(cfg, "vtest"); err == nil {
		t.Fatalf("expected StartService to return error when datasource Start fails")
	}
}
