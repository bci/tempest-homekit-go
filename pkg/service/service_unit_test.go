package service

import (
	"testing"

	"tempest-homekit-go/pkg/config"
	"tempest-homekit-go/pkg/weather"
)

// fakeDS implements the minimal subset of weather.DataSource used in tests; Start returns an already-closed channel and the other methods return zero values.
type fakeDS struct{}

// Start returns an already-closed observation channel.
func (f *fakeDS) Start() (<-chan weather.Observation, error) {
	ch := make(chan weather.Observation)
	close(ch)
	return ch, nil
}
func (f *fakeDS) Stop() error                                { return nil }
func (f *fakeDS) GetLatestObservation() *weather.Observation { return nil }
func (f *fakeDS) GetForecast() *weather.ForecastResponse     { return nil }
func (f *fakeDS) GetStatus() weather.DataSourceStatus {
	return weather.DataSourceStatus{Type: weather.DataSourceGenerated}
}
func (f *fakeDS) GetType() weather.DataSourceType { return weather.DataSourceGenerated }

// TestDataSourceFactoryOverride checks that DataSourceFactory can be replaced and the replacement is called.
func TestDataSourceFactoryOverride(t *testing.T) {
	orig := DataSourceFactory
	defer func() { DataSourceFactory = orig }()

	DataSourceFactory = func(cfg *config.Config, station *weather.Station, udpListener interface{}, genParam interface{}) (weather.DataSource, error) {
		return &fakeDS{}, nil
	}

	ds, err := DataSourceFactory(nil, nil, nil, nil)
	if err != nil || ds == nil {
		t.Fatalf("factory override failed: %v", err)
	}
}
