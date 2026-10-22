//go:build ignore

package service

import (
	"testing"
	"time"

	"tempest-homekit-go/pkg/config"
	"tempest-homekit-go/pkg/weather"
)

// fakeUDPListener implements weather.UDPListener for tests with no-op methods and
// an already-closed observation channel. This file is excluded by its ignore
// build tag and predates the current four-argument CreateDataSource signature.
type fakeUDPListener struct{}

// Start is a no-op that returns nil.
func (f *fakeUDPListener) Start() error                               { return nil }
func (f *fakeUDPListener) Stop() error                                { return nil }
func (f *fakeUDPListener) GetLatestObservation() *weather.Observation { return nil }
func (f *fakeUDPListener) GetStats() (int64, time.Time, string, string) {
	return 0, time.Time{}, "", ""
}
func (f *fakeUDPListener) GetObservations() []weather.Observation { return nil }
func (f *fakeUDPListener) IsReceivingData() bool                  { return false }
func (f *fakeUDPListener) ObservationChannel() <-chan weather.Observation {
	ch := make(chan weather.Observation)
	close(ch)
	return ch
}

// TestCreateDataSource_UDP_Success checks that UDP mode with a listener yields a UDP data source.
func TestCreateDataSource_UDP_Success(t *testing.T) {
	cfg := &config.Config{UDPStream: true, DisableInternet: true}
	var st *weather.Station = &weather.Station{StationID: 123, StationName: "Test"}

	ds, err := CreateDataSource(cfg, st, &fakeUDPListener{}, nil)
	if err != nil {
		t.Fatalf("CreateDataSource UDP returned error: %v", err)
	}
	if ds.GetType() != weather.DataSourceUDP {
		t.Fatalf("expected UDP data source type, got: %v", ds.GetType())
	}
}

// TestCreateDataSource_CustomURL checks that a custom station URL yields a custom-URL data source.
func TestCreateDataSource_CustomURL(t *testing.T) {
	cfg := &config.Config{StationURL: "http://example/custom"}
	st := &weather.Station{StationID: 10, StationName: "S"}

	ds, err := CreateDataSource(cfg, st, nil)
	if err != nil {
		t.Fatalf("CreateDataSource custom URL returned error: %v", err)
	}
	if ds.GetType() != weather.DataSourceCustomURL {
		t.Fatalf("expected CustomURL data source type, got: %v", ds.GetType())
	}
}

// TestCreateDataSource_Generated checks that generated-weather mode yields a generated data source without a station.
func TestCreateDataSource_Generated(t *testing.T) {
	cfg := &config.Config{UseGeneratedWeather: true, WebPort: "7777", GeneratedWeatherPath: "/mygen"}
	ds, err := CreateDataSource(cfg, nil, nil)
	if err != nil {
		t.Fatalf("CreateDataSource generated returned error: %v", err)
	}
	if ds.GetType() != weather.DataSourceGenerated {
		t.Fatalf("expected Generated data source type, got: %v", ds.GetType())
	}
}

// TestCreateDataSource_DefaultNoStationError checks that the default API mode fails without a station.
func TestCreateDataSource_DefaultNoStationError(t *testing.T) {
	cfg := &config.Config{}
	ds, err := CreateDataSource(cfg, nil, nil)
	if err == nil {
		t.Fatalf("expected error when station is nil for default API data source, got ds: %v", ds)
	}
}
