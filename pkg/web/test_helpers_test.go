package web

import (
	"tempest-homekit-go/pkg/generator"
	"tempest-homekit-go/pkg/weather"
	"testing"
)

// fakeGeneratorConfig configures deterministic behavior for the fake generator used in tests.
//
// Each field is returned verbatim by the matching fakeGenerator method: LocationName and
// ClimateZone by GetLocation, Season by GetSeason, DailyRainTotal by GetDailyRainTotal
// and Observation by GenerateObservation.
type fakeGeneratorConfig struct {
	LocationName   string
	Season         generator.Season
	DailyRainTotal float64
	ClimateZone    string
	Observation    *weather.Observation
}

// fakeGenerator is a configurable implementation of WeatherGeneratorInterface for tests.
type fakeGenerator struct {
	cfg *fakeGeneratorConfig
}

// newFakeGenerator returns a fakeGenerator using cfg, or a default "Test"/"Temperate"
// configuration with zero rain and an empty observation when cfg is nil.
func newFakeGenerator(cfg *fakeGeneratorConfig) *fakeGenerator {
	if cfg == nil {
		cfg = &fakeGeneratorConfig{
			LocationName:   "Test",
			Season:         generator.Season(0),
			DailyRainTotal: 0.0,
			ClimateZone:    "Temperate",
			Observation:    &weather.Observation{},
		}
	}
	return &fakeGenerator{cfg: cfg}
}

// GenerateNewSeason is a no-op so tests see a stable location and season.
//
// It and the adjacent methods below implement WeatherGeneratorInterface (they
// are kept without separating comments so gofmt keeps their layout):
// GetLocation returns a Location built from the configured name and climate
// zone, GetSeason returns the configured season, GetDailyRainTotal returns the
// configured rain total, SetCurrentWeatherMode is a no-op, and
// GenerateObservation returns the configured observation pointer (the same one
// on every call).
func (f *fakeGenerator) GenerateNewSeason() {
	// no-op for deterministic tests
}
func (f *fakeGenerator) GetLocation() generator.Location {
	return generator.Location{Name: f.cfg.LocationName, ClimateZone: f.cfg.ClimateZone}
}
func (f *fakeGenerator) GetSeason() generator.Season               { return f.cfg.Season }
func (f *fakeGenerator) GetDailyRainTotal() float64                { return f.cfg.DailyRainTotal }
func (f *fakeGenerator) SetCurrentWeatherMode()                    {}
func (f *fakeGenerator) GenerateObservation() *weather.Observation { return f.cfg.Observation }

// GetLocationWithClimate returns the same Location as GetLocation.
//
// Provide climate zone via Location if requested by tests
func (f *fakeGenerator) GetLocationWithClimate() generator.Location {
	loc := generator.Location{Name: f.cfg.LocationName}
	loc.ClimateZone = f.cfg.ClimateZone
	return loc
}

// testNewWebServer returns a preconfigured *WebServer for tests with sensible defaults.
// Tests can customize returned generator behavior by constructing their own fake and
// calling NewWebServer directly if needed; this helper covers the common case.
func testNewWebServer(t *testing.T) *WebServer {
	t.Helper()
	gw := &GeneratedWeatherInfo{Enabled: false}
	fg := newFakeGenerator(nil)
	// Use info log level for tests by default to match test expectations
	return NewWebServer("8080", 100.0, "info", 12345, false, "v1.3.0", "", gw, fg, "imperial", "mb", 1000, 24, "", false)
}
