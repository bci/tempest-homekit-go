package service

import (
	"errors"
	"testing"

	"tempest-homekit-go/pkg/config"
	"tempest-homekit-go/pkg/weather"
)

// dsErrorFactory returns an error when called to simulate factory failure.
func dsErrorFactory(cfg *config.Config, station *weather.Station, udpListener interface{}, genParam interface{}) (weather.DataSource, error) {
	return nil, errors.New("factory failed")
}

// fakeDataSourceStartError implements weather.DataSource where Start returns an error; the other methods return zero values.
type fakeDataSourceStartError struct{}

// Start always fails with "start failed".
func (f *fakeDataSourceStartError) Start() (<-chan weather.Observation, error) {
	return nil, errors.New("start failed")
}
func (f *fakeDataSourceStartError) Stop() error                                { return nil }
func (f *fakeDataSourceStartError) GetLatestObservation() *weather.Observation { return nil }
func (f *fakeDataSourceStartError) GetForecast() *weather.ForecastResponse     { return nil }
func (f *fakeDataSourceStartError) GetStatus() weather.DataSourceStatus {
	return weather.DataSourceStatus{}
}
func (f *fakeDataSourceStartError) GetType() weather.DataSourceType { return "fake" }

// dsStartErrorFactory returns a fakeDataSourceStartError.
func dsStartErrorFactory(cfg *config.Config, station *weather.Station, udpListener interface{}, genParam interface{}) (weather.DataSource, error) {
	return &fakeDataSourceStartError{}, nil
}

// TestStartService_FactoryErrorPropagates expects StartService to fail when the
// factory fails. In API mode with no token the station lookup fails first, so
// the factory is not actually reached.
func TestStartService_FactoryErrorPropagates(t *testing.T) {
	orig := DataSourceFactory
	defer func() { DataSourceFactory = orig }()
	DataSourceFactory = dsErrorFactory

	cfg := &config.Config{
		DisableHomeKit:      true,
		DisableWebConsole:   true,
		UseGeneratedWeather: false,
	}

	if err := StartService(cfg, "vtest"); err == nil {
		t.Fatalf("expected StartService to return error when factory fails")
	}
}

// TestStartService_DataSourceStartErrorPropagates expects StartService to fail
// when the data source cannot start. With a fake token the WeatherFlow station
// lookup fails first, so Start is not actually reached.
func TestStartService_DataSourceStartErrorPropagates(t *testing.T) {
	orig := DataSourceFactory
	defer func() { DataSourceFactory = orig }()
	DataSourceFactory = dsStartErrorFactory

	cfg := &config.Config{
		DisableHomeKit:      true,
		DisableWebConsole:   true,
		UseGeneratedWeather: false,
		StationName:         "Test",
		Token:               "tok",
	}

	if err := StartService(cfg, "vtest"); err == nil {
		t.Fatalf("expected StartService to return error when data source Start fails")
	}
}
