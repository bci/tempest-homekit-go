package service_test

import (
	"testing"
	"time"

	"tempest-homekit-go/pkg/config"
	svc "tempest-homekit-go/pkg/service"
	"tempest-homekit-go/pkg/weather"
)

// fakeDataSource is a minimal implementation of weather.DataSource used for tests.
type fakeDataSource struct{}

// Start returns a channel that delivers two observations about 5 ms apart and then closes, so StartService returns.
func (f *fakeDataSource) Start() (<-chan weather.Observation, error) {
	ch := make(chan weather.Observation)
	go func() {
		// send a couple of observations then close the channel so StartService can exit
		for i := 0; i < 2; i++ {
			ch <- weather.Observation{
				Timestamp:        time.Now().Unix(),
				AirTemperature:   20.0 + float64(i),
				RelativeHumidity: 50.0,
				WindAvg:          1.2,
			}
			time.Sleep(5 * time.Millisecond)
		}
		close(ch)
	}()
	return ch, nil
}

// Stop is a no-op for the test.
func (f *fakeDataSource) Stop() error {
	// no-op for test
	return nil
}

// GetLatestObservation returns a fresh observation at 21 deg C.
func (f *fakeDataSource) GetLatestObservation() *weather.Observation {
	return &weather.Observation{Timestamp: time.Now().Unix(), AirTemperature: 21.0}
}

// GetForecast always returns nil.
func (f *fakeDataSource) GetForecast() *weather.ForecastResponse { return nil }

// GetStatus reports an active generated source with two observations.
func (f *fakeDataSource) GetStatus() weather.DataSourceStatus {
	return weather.DataSourceStatus{Type: weather.DataSourceGenerated, Active: true, LastUpdate: time.Now(), ObservationCount: 2}
}

// GetType reports DataSourceGenerated.
func (f *fakeDataSource) GetType() weather.DataSourceType { return weather.DataSourceGenerated }

// TestStartService_WithFakeDataSource checks, from outside the package, that StartService processes a fake source with HomeKit and the web console disabled and returns nil when it closes.
func TestStartService_WithFakeDataSource(t *testing.T) {
	// Override factory and restore after test
	orig := svc.DataSourceFactory
	defer func() { svc.DataSourceFactory = orig }()

	svc.DataSourceFactory = func(cfg *config.Config, station *weather.Station, udpListener interface{}, genParam interface{}) (weather.DataSource, error) {
		return &fakeDataSource{}, nil
	}

	cfg := &config.Config{
		Pin:                 "00102003",
		LogLevel:            "debug",
		UseGeneratedWeather: true,
		DisableHomeKit:      true,
		DisableWebConsole:   true,
	}

	// StartService should run, process the fake observations and return without error.
	if err := svc.StartService(cfg, "vtest"); err != nil {
		t.Fatalf("StartService returned error: %v", err)
	}
}
