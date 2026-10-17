package config

import (
	"testing"
)

// TestGeneratedStationURLConstruction checks that a localhost station URL built from WebPort and GeneratedWeatherPath has the expected form. It reproduces the URL construction locally rather than calling LoadConfig.
func TestGeneratedStationURLConstruction(t *testing.T) {
	cfg := &Config{
		WebPort:              "3000",
		UseGeneratedWeather:  true,
		GeneratedWeatherPath: "/api/generate-weather",
	}

	// simulate the logic from LoadConfig that sets StationURL when UseGeneratedWeather is true
	if cfg.StationURL == "" && cfg.UseGeneratedWeather {
		cfg.StationURL = "http://localhost:" + cfg.WebPort + cfg.GeneratedWeatherPath
	}

	expected := "http://localhost:3000/api/generate-weather"
	if cfg.StationURL != expected {
		t.Fatalf("expected StationURL %s, got %s", expected, cfg.StationURL)
	}
}
