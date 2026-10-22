package homekit

import (
	"testing"

	"tempest-homekit-go/pkg/config"
)

// TestNewWeatherSystemModern_Basic checks that a bridge with all standard sensors
// builds, lists sensors, and accepts updates for known and unknown sensor names
// without panicking.
func TestNewWeatherSystemModern_Basic(t *testing.T) {
	cfg := config.SensorConfig{
		Temperature: true,
		Humidity:    true,
		Light:       true,
		UV:          true,
		Pressure:    true,
	}

	ws, err := NewWeatherSystemModern("00102003", &cfg, "debug")
	if err != nil {
		t.Fatalf("NewWeatherSystemModern returned error: %v", err)
	}
	if ws == nil {
		t.Fatalf("Expected non-nil WeatherSystemModern")
	}

	sensors := ws.GetAvailableSensors()
	if len(sensors) == 0 {
		t.Fatalf("Expected available sensors, got none")
	}

	// Attempt to update a known sensor that should exist
	ws.UpdateSensor("Air Temperature", 22.5)
	ws.UpdateSensor("Relative Humidity", 55.0)

	// Updating a non-existent sensor should not panic
	ws.UpdateSensor("Non Existent Sensor", 1.0)
}
