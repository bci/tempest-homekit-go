// Package generator provides synthetic weather data generation for UI testing.
// It creates realistic weather patterns based on different seasons and locations.
//
// A WeatherGenerator picks (or is given) a Location and Season, derives base
// temperature, pressure and humidity from them, and then produces
// types.Observation values with diurnal variation and random noise. Optional
// per-sensor TestPattern cycles override the random values with fixed,
// predictable steps so that UI and HomeKit behaviour can be checked by eye.
// A WeatherGenerator is not safe for concurrent use.
package generator

import (
	"math"
	"math/rand"
	"time"

	"tempest-homekit-go/pkg/types"
)

// Season represents different weather seasons. It biases the generated
// temperature, wind, light, UV, solar radiation and rain probability.
type Season int

// Season values. They are contiguous from zero so that a random season can
// be chosen with rand.Intn(4).
const (
	Spring Season = iota // Mild and windy, second-highest rain chance
	Summer               // Warmest and calmest, strongest light and UV
	Fall                 // Mild, moderate wind and rain
	Winter               // Coldest and windiest, highest rain chance
)

// String returns the season's name ("Spring", "Summer", "Fall" or "Winter").
// It panics if s is outside the defined range.
func (s Season) String() string {
	seasons := []string{"Spring", "Summer", "Fall", "Winter"}
	return seasons[s]
}

// Location represents different climate locations used to seed generated
// weather.
//
// ClimateZone values recognised by the generator are "Tropical",
// "Subtropical", "Desert", "Oceanic", "Continental", "Mediterranean" and
// "Subarctic"; other values get default behaviour.
type Location struct {
	Name        string  // Human-readable place name, e.g. "Denver, CO"
	Latitude    float64 // Decimal degrees; shifts base temperature and scales UV
	Longitude   float64 // Decimal degrees; informational, not used by the generator
	Elevation   float64 // Metres; reduces station pressure from sea level
	ClimateZone string  // Climate zone name selecting humidity, rain, wind and temperature adjustments
}

// WeatherGenerator generates synthetic weather data.
//
// Create one with NewWeatherGenerator or NewWeatherGeneratorWithParams; the
// zero value has no random source and will panic.
//
// Unexported state: history is the last series from GenerateHistoricalData
// (cleared by Regenerate); rng is the private clock-seeded random source;
// the testPattern* fields hold the per-sensor test pattern enabled by
// EnableTestPattern, nil meaning normal random generation for that sensor.
type WeatherGenerator struct {
	Location               Location           // Simulated station location
	Season                 Season             // Simulated season
	CurrentTime            time.Time          // Observation time; zero means time.Now(). Stepped and restored by GenerateHistoricalData
	BaseTemperature        float64            // Celsius
	BasePressure           float64            // mb
	BaseHumidity           float64            // %
	current                *types.Observation // Most recent observation from GenerateObservation
	history                []*types.Observation
	rng                    *rand.Rand
	cumulativeRain         float64 // Total accumulated rain since station start (like real Tempest)
	dailyRainTotal         float64 // Total rain for the current day (resets at midnight)
	lastDayCheck           int     // Day of year for checking when to reset daily total
	isGeneratingHistorical bool    // Flag to prevent historical generation from affecting daily totals
	testPatternRain        *TestPattern
	testPatternWind        *TestPattern
	testPatternTemp        *TestPattern
	testPatternHumidity    *TestPattern
	testPatternPressure    *TestPattern
	testPatternLux         *TestPattern
	testPatternUV          *TestPattern
	testPatternLightning   *TestPattern
}

// Locations lists predefined locations with different climates, from which
// NewWeatherGenerator and Regenerate choose at random.
var Locations = []Location{
	{
		Name: "Miami, FL", Latitude: 25.7617, Longitude: -80.1918, Elevation: 2.0,
		ClimateZone: "Tropical",
	},
	{
		Name: "Denver, CO", Latitude: 39.7392, Longitude: -104.9903, Elevation: 1609.0,
		ClimateZone: "Continental",
	},
	{
		Name: "Seattle, WA", Latitude: 47.6062, Longitude: -122.3321, Elevation: 56.0,
		ClimateZone: "Oceanic",
	},
	{
		Name: "Phoenix, AZ", Latitude: 33.4484, Longitude: -112.0740, Elevation: 331.0,
		ClimateZone: "Desert",
	},
	{
		Name: "Minneapolis, MN", Latitude: 44.9778, Longitude: -93.2650, Elevation: 264.0,
		ClimateZone: "Continental",
	},
	{
		Name: "San Diego, CA", Latitude: 32.7157, Longitude: -117.1611, Elevation: 19.0,
		ClimateZone: "Mediterranean",
	},
	{
		Name: "Anchorage, AK", Latitude: 61.2181, Longitude: -149.9003, Elevation: 35.0,
		ClimateZone: "Subarctic",
	},
	{
		Name: "New Orleans, LA", Latitude: 29.9511, Longitude: -90.0715, Elevation: -0.5,
		ClimateZone: "Subtropical",
	},
}

// NewWeatherGenerator creates a new weather generator with random location and season.
//
// The random source is seeded from the current time, so results are not
// reproducible. CurrentTime is set to time.Now().
func NewWeatherGenerator() *WeatherGenerator {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	// Randomly select location and season
	location := Locations[rng.Intn(len(Locations))]
	season := Season(rng.Intn(4))

	wg := &WeatherGenerator{
		Location:    location,
		Season:      season,
		CurrentTime: time.Now(),
		rng:         rng,
	}

	wg.initializeBaseValues()
	return wg
}

// NewWeatherGeneratorWithParams creates a weather generator with specific location and season.
//
// location and season fix the climate; base values are still randomised from
// a clock-seeded source. CurrentTime is set to time.Now().
func NewWeatherGeneratorWithParams(location Location, season Season) *WeatherGenerator {
	wg := &WeatherGenerator{
		Location:    location,
		Season:      season,
		CurrentTime: time.Now(),
		rng:         rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	wg.initializeBaseValues()
	return wg
}

// initializeBaseValues sets realistic base values based on location and season.
//
// It sets BaseTemperature, BasePressure and BaseHumidity, seeds the
// cumulative rain counter with 1.5-9.5 inches (stored in mm) of pre-existing
// accumulation, zeroes the daily rain total and records CurrentTime's day
// of year for the midnight reset.
func (wg *WeatherGenerator) initializeBaseValues() {
	// Set base temperature based on location and season
	wg.BaseTemperature = wg.getSeasonalTemperature()

	// Set base pressure (adjusted to sea level, then we'll adjust for elevation)
	wg.BasePressure = 1013.25 + wg.rng.Float64()*40 - 20 // 993-1033 mb range

	// Set base humidity based on climate zone
	wg.BaseHumidity = wg.getClimateHumidity()

	// Initialize cumulative rain and daily total (in millimeters)
	wg.cumulativeRain = (1.5 + wg.rng.Float64()*8.0) * 25.4 // Start with some pre-existing accumulation (1.5-9.5 inches converted to mm)
	wg.dailyRainTotal = 0.0                                 // Start daily total at 0
	wg.lastDayCheck = wg.CurrentTime.YearDay()              // Track current day
}

// getSeasonalTemperature returns realistic temperatures for location and season.
//
// The result is in °C: a 15 °C baseline adjusted for latitude, season (with
// randomness) and climate zone, then clamped to -25..42 °C.
func (wg *WeatherGenerator) getSeasonalTemperature() float64 {
	baseTemp := 15.0 // Default 15°C (59°F)

	// Adjust for latitude (rough approximation)
	latAdjust := (45.0 - math.Abs(wg.Location.Latitude)) * 0.3
	baseTemp += latAdjust

	// Seasonal adjustments
	switch wg.Season {
	case Spring:
		baseTemp += wg.rng.Float64()*8 + 3 // 3-11°C adjustment
	case Summer:
		baseTemp += wg.rng.Float64()*10 + 8 // 8-18°C adjustment
	case Fall:
		baseTemp += wg.rng.Float64() * 6 // 0-6°C adjustment
	case Winter:
		baseTemp -= wg.rng.Float64()*10 + 2 // -12 to -2°C adjustment
	}

	// Climate zone adjustments (more moderate)
	switch wg.Location.ClimateZone {
	case "Tropical":
		baseTemp += 5 // Reduced from 8
	case "Desert":
		switch wg.Season {
		case Summer:
			baseTemp += 8 // Reduced from 15
		case Winter:
			baseTemp += 2 // Reduced from 5
		}
	case "Subarctic":
		baseTemp -= 15 // Reduced from 20
	case "Mediterranean":
		baseTemp += 2 // Reduced from 3
	}

	// Ensure temperatures stay within reasonable bounds
	baseTemp = math.Max(-25, math.Min(42, baseTemp)) // -13°F to 107.6°F range

	return baseTemp
}

// getClimateHumidity returns realistic humidity for the climate zone as a
// random percentage within a zone-specific range (50-80% for unknown zones).
func (wg *WeatherGenerator) getClimateHumidity() float64 {
	switch wg.Location.ClimateZone {
	case "Tropical", "Subtropical":
		return 70 + wg.rng.Float64()*25 // 70-95%
	case "Desert":
		return 10 + wg.rng.Float64()*30 // 10-40%
	case "Oceanic":
		return 60 + wg.rng.Float64()*30 // 60-90%
	case "Continental":
		return 40 + wg.rng.Float64()*40 // 40-80%
	case "Mediterranean":
		return 45 + wg.rng.Float64()*35 // 45-80%
	case "Subarctic":
		return 60 + wg.rng.Float64()*25 // 60-85%
	default:
		return 50 + wg.rng.Float64()*30 // 50-80%
	}
}

// SetCurrentWeatherMode ensures the generator is in current weather mode (not historical).
//
// In current mode, rain updates the daily rain total and test patterns use
// the wall clock.
func (wg *WeatherGenerator) SetCurrentWeatherMode() {
	wg.isGeneratingHistorical = false
}

// GenerateObservation creates a single realistic weather observation.
//
// It uses CurrentTime (or time.Now() when CurrentTime is zero) for diurnal
// effects and the timestamp, then applies any enabled test patterns, which
// overwrite the corresponding random values. RainAccumulated is set to the
// generator's cumulative rain since start, not the per-observation
// increment. The observation is stored as the generator's current value and
// returned.
func (wg *WeatherGenerator) GenerateObservation() *types.Observation {
	// Use CurrentTime if set (for historical data), otherwise use current time
	observationTime := wg.CurrentTime
	if observationTime.IsZero() {
		observationTime = time.Now()
	}

	// Generate temperature with daily variation
	hourOfDay := float64(observationTime.Hour())
	tempVariation := math.Sin((hourOfDay-6)*math.Pi/12) * 4                      // Reduced from 8 to 4 degrees variation, peak at 2 PM, minimum at 6 AM
	temperature := wg.BaseTemperature + tempVariation + (wg.rng.Float64()-0.5)*2 // Reduced random variation from 4 to 2

	// Generate humidity (inversely related to temperature)
	humidity := wg.BaseHumidity - tempVariation*2 + (wg.rng.Float64()-0.5)*15
	humidity = math.Max(10, math.Min(98, humidity))

	// Generate pressure with realistic variation
	pressure := wg.BasePressure + (wg.rng.Float64()-0.5)*10
	// Adjust for elevation
	pressure = pressure * math.Pow(1-0.0065*wg.Location.Elevation/288.15, 5.255)

	// Generate wind based on season and location
	windSpeed := wg.generateWind()
	windDirection := wg.rng.Float64() * 360
	windGust := windSpeed * (1.2 + wg.rng.Float64()*0.8)

	// Generate illuminance based on time of day
	illuminance := wg.generateIlluminance(observationTime)

	// Generate UV based on time of day and season
	uv := wg.generateUV(observationTime)

	// Generate rain based on season and climate (skip if test pattern is active)
	var rain float64
	if wg.testPatternRain == nil {
		rain = wg.generateRain()
	}

	// Generate solar radiation
	solar := wg.generateSolar(observationTime)

	obs := &types.Observation{
		Timestamp:            observationTime.Unix(),
		WindLull:             math.Max(0, windSpeed-wg.rng.Float64()*2),
		WindAvg:              windSpeed,
		WindGust:             windGust,
		WindDirection:        windDirection,
		StationPressure:      pressure,
		AirTemperature:       temperature,
		RelativeHumidity:     humidity,
		Illuminance:          illuminance,
		UV:                   int(uv),
		SolarRadiation:       solar,
		RainAccumulated:      wg.cumulativeRain, // Use cumulative rain, not incremental
		RainDailyTotal:       wg.dailyRainTotal,
		PrecipitationType:    wg.generatePrecipitationType(temperature, rain),
		LightningStrikeAvg:   wg.generateLightning(),
		LightningStrikeCount: wg.generateLightningCount(),
		Battery:              3.6 + wg.rng.Float64()*0.3, // 3.6-3.9V
		ReportInterval:       60,
	}

	wg.current = obs

	// Apply test patterns if enabled
	// For historical generation, pass the historical time; for real-time, pass nil
	var obsTimePtr *time.Time
	if wg.isGeneratingHistorical {
		obsTimePtr = &wg.CurrentTime
	}

	if wg.testPatternRain != nil {
		incrementalRain := ApplyTestRainPattern(obs, wg.testPatternRain, obsTimePtr)
		// Update daily total for real-time observations (not during historical generation)
		if !wg.isGeneratingHistorical {
			// Check if it's a new day and reset daily total if needed
			currentDay := time.Now().YearDay()
			if currentDay != wg.lastDayCheck {
				wg.dailyRainTotal = 0.0 // Reset daily total at midnight
				wg.lastDayCheck = currentDay
			}
			wg.dailyRainTotal += incrementalRain
			obs.RainDailyTotal = wg.dailyRainTotal
		}
	}
	if wg.testPatternWind != nil {
		ApplyTestWindPattern(obs, wg.testPatternWind)
	}
	if wg.testPatternTemp != nil {
		ApplyTestTempPattern(obs, wg.testPatternTemp)
	}
	if wg.testPatternHumidity != nil {
		ApplyTestHumidityPattern(obs, wg.testPatternHumidity)
	}
	if wg.testPatternPressure != nil {
		ApplyTestPressurePattern(obs, wg.testPatternPressure)
	}
	if wg.testPatternLux != nil {
		ApplyTestLuxPattern(obs, wg.testPatternLux)
	}
	if wg.testPatternUV != nil {
		ApplyTestUVPattern(obs, wg.testPatternUV)
	}
	if wg.testPatternLightning != nil {
		ApplyTestLightningPattern(obs, wg.testPatternLightning)
	}

	return obs
}

// generateWind creates realistic wind patterns.
//
// It returns an average wind speed: a random 2-10 base scaled by season and
// climate zone. GenerateObservation stores it in WindAvg, which is m/s.
func (wg *WeatherGenerator) generateWind() float64 {
	baseWind := 2.0 + wg.rng.Float64()*8 // 2-10 mph base

	// Seasonal adjustments
	switch wg.Season {
	case Spring:
		baseWind *= 1.3 // Windier in spring
	case Summer:
		baseWind *= 0.8 // Calmer in summer
	case Fall:
		baseWind *= 1.2 // Moderate wind in fall
	case Winter:
		baseWind *= 1.5 // Windier in winter
	}

	// Climate adjustments
	switch wg.Location.ClimateZone {
	case "Oceanic":
		baseWind *= 1.4
	case "Desert":
		baseWind *= 0.9
	case "Continental":
		baseWind *= 1.1
	}

	return baseWind
}

// generateIlluminance creates realistic light levels.
//
// It returns lux for the hour of t: 0-10 at night (before 06:00 or after
// 20:59), 50-250 at dawn and dusk (06, 07, 19 and 20 h), and a seasonal
// clear-sky baseline scaled by a random cloud factor during the day.
func (wg *WeatherGenerator) generateIlluminance(t time.Time) float64 {
	hour := t.Hour()

	// Night time - low light levels
	if hour < 6 || hour > 20 {
		return wg.rng.Float64() * 10 // 0-10 lux at night (moon/stars/artificial light)
	}

	// Dawn/dusk
	if hour == 6 || hour == 7 || hour == 19 || hour == 20 {
		return 50 + wg.rng.Float64()*200 // 50-250 lux
	}

	// Daytime - varies by season and weather
	baseLux := 10000.0 // Clear day baseline

	// Seasonal adjustments
	switch wg.Season {
	case Summer:
		baseLux *= 1.2
	case Winter:
		baseLux *= 0.6
	case Spring, Fall:
		baseLux *= 0.9
	}

	// Add some cloud variation
	cloudFactor := 0.3 + wg.rng.Float64()*0.7 // 30-100% of clear sky

	return baseLux*cloudFactor + wg.rng.Float64()*5000
}

// generateUV creates realistic UV index.
//
// It returns 0 outside 08:00-18:59, otherwise a sine-shaped value scaled by a
// seasonal maximum, adjusted for latitude and noise, and never negative.
func (wg *WeatherGenerator) generateUV(t time.Time) float64 {
	hour := t.Hour()

	// No UV at night
	if hour < 8 || hour > 18 {
		return 0
	}

	// Peak UV around noon
	uvFactor := math.Sin((float64(hour) - 6) * math.Pi / 12)
	if uvFactor < 0 {
		uvFactor = 0
	}

	maxUV := 3.0 // Base maximum UV

	// Seasonal adjustments
	switch wg.Season {
	case Summer:
		maxUV = 9.0
	case Spring, Fall:
		maxUV = 6.0
	case Winter:
		maxUV = 3.0
	}

	// Latitude adjustments
	if math.Abs(wg.Location.Latitude) < 30 {
		maxUV *= 1.3 // Higher UV near equator
	} else if math.Abs(wg.Location.Latitude) > 50 {
		maxUV *= 0.7 // Lower UV at high latitudes
	}

	return math.Max(0, maxUV*uvFactor+(wg.rng.Float64()-0.5)*2)
}

// GetDailyRainTotal returns the daily rain total for generated weather, in
// mm since the last midnight reset.
func (wg *WeatherGenerator) GetDailyRainTotal() float64 {
	return wg.dailyRainTotal
}

// generateRain creates realistic precipitation and updates cumulative total.
//
// In real-time mode without a rain test pattern it first resets the daily
// total when the day of year has changed. It then rolls against a
// season- and climate-adjusted probability and, on a hit, adds 0-2.54 mm to
// both the cumulative and the daily totals. It returns the incremental rain
// in mm, or 0 when no rain fell.
func (wg *WeatherGenerator) generateRain() float64 {
	// Only update daily totals if not generating historical data and not using test patterns
	if !wg.isGeneratingHistorical && wg.testPatternRain == nil {
		// Check if it's a new day and reset daily total if needed
		currentDay := time.Now().YearDay()
		if currentDay != wg.lastDayCheck {
			wg.dailyRainTotal = 0.0 // Reset daily total at midnight
			wg.lastDayCheck = currentDay
		}
	}

	// Base probability of rain
	rainChance := 0.1 // 10% base chance

	// Seasonal adjustments
	switch wg.Season {
	case Spring:
		rainChance = 0.25
	case Summer:
		rainChance = 0.15
	case Fall:
		rainChance = 0.2
	case Winter:
		rainChance = 0.3
	}

	// Climate adjustments
	switch wg.Location.ClimateZone {
	case "Tropical", "Oceanic":
		rainChance *= 2
	case "Desert":
		rainChance *= 0.2
	case "Subtropical":
		rainChance *= 1.5
	}

	var incrementalRain float64
	if wg.rng.Float64() < rainChance {
		// Light to moderate rain (per minute/observation)
		incrementalRain = wg.rng.Float64() * 2.54 // 0-2.54 mm per observation (equivalent to 0-0.1 inches)
		wg.cumulativeRain += incrementalRain

		// Add to daily total (even during historical generation for test patterns)
		wg.dailyRainTotal += incrementalRain
	}

	return incrementalRain
}

// generateSolar creates realistic solar radiation.
//
// It returns W/m²: 0 outside 06:00-19:59, otherwise a sine-shaped value
// scaled by a seasonal maximum and a random cloud factor.
func (wg *WeatherGenerator) generateSolar(t time.Time) float64 {
	hour := t.Hour()

	if hour < 6 || hour > 19 {
		return 0
	}

	// Peak solar around noon
	solarFactor := math.Sin((float64(hour) - 6) * math.Pi / 13)
	if solarFactor < 0 {
		solarFactor = 0
	}

	maxSolar := 800.0 // W/m²

	// Seasonal adjustments
	switch wg.Season {
	case Summer:
		maxSolar = 1000.0
	case Winter:
		maxSolar = 400.0
	case Spring, Fall:
		maxSolar = 700.0
	}

	return maxSolar * solarFactor * (0.5 + wg.rng.Float64()*0.5)
}

// generatePrecipitationType determines precipitation type.
//
// It returns 0 when rain is zero; otherwise 3 below 0 °C, 2 below 2 °C and 1
// above. The code comments label 2 and 3 as ice pellets and snow, which
// differs from the Tempest API meaning of those codes (see
// types.Observation.PrecipitationType).
func (wg *WeatherGenerator) generatePrecipitationType(temp, rain float64) int {
	if rain == 0 {
		return 0 // None
	}

	if temp < 0 {
		return 3 // Snow
	} else if temp < 2 {
		return 2 // Ice pellets
	} else {
		return 1 // Rain
	}
}

// generateLightning creates lightning distance: 5-45 km with a 5% chance,
// otherwise 0. It is drawn independently of generateLightningCount.
func (wg *WeatherGenerator) generateLightning() float64 {
	// Lightning is rare
	if wg.rng.Float64() < 0.05 { // 5% chance
		return 5 + wg.rng.Float64()*40 // 5-45 km
	}
	return 0
}

// generateLightningCount creates lightning count: 0-4 strikes with a 5%
// chance, otherwise 0.
func (wg *WeatherGenerator) generateLightningCount() int {
	if wg.rng.Float64() < 0.05 { // 5% chance
		return int(wg.rng.Float64() * 5) // 0-4 strikes
	}
	return 0
}

// GenerateHistoricalData creates a series of historical observations.
//
// It returns count observations evenly spaced over the 24 hours before
// CurrentTime (or before now when CurrentTime is zero), oldest first. Test
// pattern start times are moved to the start of that window. RainDailyTotal
// is computed only for observations at or after local midnight of the
// reference day and is 0 for earlier ones. Base values drift slightly
// between observations and that drift persists afterwards. The daily rain
// total and CurrentTime are restored when it returns; cumulative rain is
// restored too unless a rain test pattern is active. The series is also
// kept as the generator's history. count must be positive: zero divides
// by zero when computing the interval.
func (wg *WeatherGenerator) GenerateHistoricalData(count int) []*types.Observation {
	observations := make([]*types.Observation, count)

	// Save the current state to restore later (historical generation should not affect current day)
	originalCumulativeRain := wg.cumulativeRain
	originalDailyRain := wg.dailyRainTotal
	originalTime := wg.CurrentTime

	// Start from 24 hours ago and work forward. If `wg.CurrentTime` is set (test-provided),
	// base the historical window on that to allow deterministic generation in tests.
	var startTime time.Time
	if !wg.CurrentTime.IsZero() {
		startTime = wg.CurrentTime.Add(-24 * time.Hour)
	} else {
		startTime = time.Now().Add(-24 * time.Hour)
	}
	interval := 24 * time.Hour / time.Duration(count)

	// Adjust test pattern start times to historical start
	// This ensures phase calculations work correctly during historical generation
	if wg.testPatternRain != nil {
		wg.testPatternRain.startTime = startTime
	}
	if wg.testPatternWind != nil {
		wg.testPatternWind.startTime = startTime
	}
	if wg.testPatternTemp != nil {
		wg.testPatternTemp.startTime = startTime
	}
	if wg.testPatternHumidity != nil {
		wg.testPatternHumidity.startTime = startTime
	}
	if wg.testPatternPressure != nil {
		wg.testPatternPressure.startTime = startTime
	}
	if wg.testPatternLux != nil {
		wg.testPatternLux.startTime = startTime
	}
	if wg.testPatternUV != nil {
		wg.testPatternUV.startTime = startTime
	}
	if wg.testPatternLightning != nil {
		wg.testPatternLightning.startTime = startTime
	}

	// Set a flag to prevent rain generation from affecting daily totals during historical generation
	wg.isGeneratingHistorical = true

	// Calculate midnight of today for daily rain total calculation. Use the original
	// current time if provided (for deterministic/historical runs), otherwise use now.
	var referenceNow time.Time
	if !originalTime.IsZero() {
		referenceNow = originalTime
	} else {
		referenceNow = time.Now()
	}
	midnightToday := time.Date(referenceNow.Year(), referenceNow.Month(), referenceNow.Day(), 0, 0, 0, 0, referenceNow.Location())

	// Track daily total for historical generation
	var historicalDailyTotal float64

	for i := 0; i < count; i++ {
		// Set the current time for this observation
		wg.CurrentTime = startTime.Add(time.Duration(i) * interval)

		// Generate observation for this time
		obs := wg.GenerateObservation()
		obs.Timestamp = wg.CurrentTime.Unix()

		// Calculate daily total (rain since midnight today)
		// Daily total should only include observations from today (after midnight)
		if !wg.CurrentTime.Before(midnightToday) {
			// This observation is from today - include in daily total
			if i > 0 {
				prevObs := observations[i-1]
				// Calculate incremental rain between this obs and previous
				incrementalRain := obs.RainAccumulated - prevObs.RainAccumulated
				historicalDailyTotal += incrementalRain
			} else if wg.CurrentTime.Equal(midnightToday) || wg.CurrentTime.After(midnightToday) {
				// First observation of today - its accumulated value is the incremental
				historicalDailyTotal += obs.RainAccumulated
			}
			obs.RainDailyTotal = historicalDailyTotal
		} else {
			// This observation is from yesterday or earlier - daily total should be 0
			// (we're not tracking previous days' totals)
			obs.RainDailyTotal = 0.0
		}

		observations[i] = obs

		// Add some continuity - slight drift in base values
		wg.BaseTemperature += (wg.rng.Float64() - 0.5) * 0.2
		wg.BasePressure += (wg.rng.Float64() - 0.5) * 0.5
		wg.BaseHumidity += (wg.rng.Float64() - 0.5) * 1.0

		// Keep values in reasonable ranges
		wg.BaseTemperature = math.Max(-20, math.Min(50, wg.BaseTemperature))
		wg.BasePressure = math.Max(980, math.Min(1040, wg.BasePressure))
		wg.BaseHumidity = math.Max(20, math.Min(95, wg.BaseHumidity))
	}

	// Clear the historical generation flag
	wg.isGeneratingHistorical = false

	// Restore the original state (historical generation should not corrupt current day)
	// Restore the current day's daily total to what it was before generating history
	wg.dailyRainTotal = originalDailyRain
	// NOTE: Don't restore cumulative rain if test patterns are active - let them continue from historical end
	if wg.testPatternRain == nil {
		wg.cumulativeRain = originalCumulativeRain
	}
	wg.CurrentTime = originalTime

	wg.history = observations
	return observations
}

// GetLocation returns the current location.
func (wg *WeatherGenerator) GetLocation() Location {
	return wg.Location
}

// GetSeason returns the current season.
func (wg *WeatherGenerator) GetSeason() Season {
	return wg.Season
}

// Regenerate creates a new random location and season combination.
//
// It re-derives base values (resetting cumulative and daily rain) and clears
// the stored history. Enabled test patterns are kept.
func (wg *WeatherGenerator) Regenerate() {
	// Select new random location and season
	wg.Location = Locations[wg.rng.Intn(len(Locations))]
	wg.Season = Season(wg.rng.Intn(4))

	// Reinitialize base values
	wg.initializeBaseValues()

	// Clear history to force regeneration
	wg.history = nil
}

// GenerateNewSeason generates a new random location and season (alias for Regenerate).
func (wg *WeatherGenerator) GenerateNewSeason() {
	wg.Regenerate()
}

// EnableTestPattern enables a test pattern for a specific sensor.
//
// sensor is one of "rain", "wind", "temp", "humidity", "pressure", "lux",
// "uv" or "lightning"; other values are silently ignored. The pattern's
// cycle starts now. The rain pattern starts its cumulative total from the
// generator's current cumulative rain so the value keeps increasing.
// Calling it again for the same sensor restarts that pattern.
func (wg *WeatherGenerator) EnableTestPattern(sensor string) {
	pattern := &TestPattern{
		cycleCount: 0,
		startTime:  time.Now(),
	}

	switch sensor {
	case "rain":
		// Initialize with current cumulative rain so test pattern continues from there
		pattern.lastRainAccum = wg.cumulativeRain
		wg.testPatternRain = pattern
	case "wind":
		wg.testPatternWind = pattern
	case "temp":
		wg.testPatternTemp = pattern
	case "humidity":
		wg.testPatternHumidity = pattern
	case "pressure":
		wg.testPatternPressure = pattern
	case "lux":
		wg.testPatternLux = pattern
	case "uv":
		wg.testPatternUV = pattern
	case "lightning":
		wg.testPatternLightning = pattern
	}
}

// ResetTestPatternRain resets the rain test pattern accumulation to current cumulative rain.
//
// This is needed after historical data generation which resets cumulativeRain.
// It also restarts the pattern's rate clock at now. It does nothing when no
// rain pattern is enabled.
func (wg *WeatherGenerator) ResetTestPatternRain() {
	if wg.testPatternRain != nil {
		wg.testPatternRain.lastRainAccum = wg.cumulativeRain
		wg.testPatternRain.lastUpdateTime = time.Now()
	}
}

// TestPattern represents a cycling test pattern for sensor simulation.
//
// Every pattern steps through four fixed phases of 30 seconds each; see
// GetCyclePhase and the ApplyTest*Pattern functions for the values.
type TestPattern struct {
	cycleCount     int       // Initialised to zero; currently unused
	startTime      time.Time // Reference time from which phases are counted
	lastRainAccum  float64   // Track cumulative rain for rain pattern
	lastUpdateTime time.Time // Track last update for rain rate calculation
}

// GetCyclePhase returns the current phase (0-3) based on elapsed time.
//
// Each complete cycle is 120 seconds (2 minutes), divided into 4 phases of 30 seconds each.
// When obsTime is provided (for historical generation), uses that instead of real time.
// Elapsed time is measured from the pattern's start time.
func (p *TestPattern) GetCyclePhase(obsTime *time.Time) int {
	var elapsed float64
	if obsTime != nil {
		elapsed = obsTime.Sub(p.startTime).Seconds()
	} else {
		elapsed = time.Since(p.startTime).Seconds()
	}
	return (int(elapsed) / 30) % 4
}

// ApplyTestRainPattern applies a cycling rain pattern: 2.0 -> 0.5 -> 2.0 -> 10.0 mm/hr.
//
// RainAccumulated should be cumulative (always increasing) like real Tempest API.
// When obsTime is provided (for historical generation), uses that instead of real time,
// both for the phase and for the elapsed time since the previous call.
// Historical generation uses the same cycling phases, keyed to each observation's
// timestamp. The first call assumes 60 seconds have elapsed. The phase rate times
// the elapsed time is added to pattern's running total, which is written to
// obs.RainAccumulated; obs.PrecipitationType is set to 1 (rain).
// Returns the incremental rain amount in mm so the generator can update dailyRainTotal.
func ApplyTestRainPattern(obs *types.Observation, pattern *TestPattern, obsTime *time.Time) float64 {
	var rainRate float64

	// Calculate time elapsed since last update (for first call, use 60 seconds)
	var elapsedSeconds float64
	if pattern.lastUpdateTime.IsZero() {
		elapsedSeconds = 60 // Assume 60 second report interval for first call
	} else {
		if obsTime != nil {
			// Historical generation: use the observation time
			elapsedSeconds = obsTime.Sub(pattern.lastUpdateTime).Seconds()
		} else {
			// Real-time generation: use current time
			elapsedSeconds = time.Since(pattern.lastUpdateTime).Seconds()
		}
	}

	// For historical generation with intervals > 60 seconds, still use cycling phases
	// based on the observation timestamp to show pattern variation in history
	phase := pattern.GetCyclePhase(obsTime)
	switch phase {
	case 0:
		rainRate = 2.0
		obs.PrecipitationType = 1
	case 1:
		rainRate = 0.5
		obs.PrecipitationType = 1
	case 2:
		rainRate = 2.0
		obs.PrecipitationType = 1
	case 3:
		rainRate = 10.0
		obs.PrecipitationType = 1
	}

	// Calculate incremental rain: (mm/hr) * (seconds/3600) = mm
	// This is the rain that fell during this observation period
	incrementalRain := rainRate * (elapsedSeconds / 3600.0)

	// Accumulate rain (make it cumulative)
	pattern.lastRainAccum += incrementalRain
	obs.RainAccumulated = pattern.lastRainAccum

	// Update last update time
	if obsTime != nil {
		pattern.lastUpdateTime = *obsTime
	} else {
		pattern.lastUpdateTime = time.Now()
	}

	return incrementalRain
}

// ApplyTestWindPattern applies a cycling wind pattern: 1 -> 4 -> 12 -> 22 m/s.
//
// It sets obs.WindAvg, sets obs.WindGust to 1.5x the average and rotates
// obs.WindDirection through 0, 90, 180 and 270 degrees. The phase always
// uses the wall clock. WindLull is left unchanged.
func ApplyTestWindPattern(obs *types.Observation, pattern *TestPattern) {
	phase := pattern.GetCyclePhase(nil)
	switch phase {
	case 0:
		obs.WindAvg = 1.0
		obs.WindGust = 1.5
	case 1:
		obs.WindAvg = 4.0
		obs.WindGust = 6.0
	case 2:
		obs.WindAvg = 12.0
		obs.WindGust = 18.0
	case 3:
		obs.WindAvg = 22.0
		obs.WindGust = 33.0
	}
	obs.WindDirection = float64((phase * 90) % 360)
}

// ApplyTestTempPattern applies a cycling temperature pattern: 0 -> 15 -> 25 -> 38 °C.
//
// It sets obs.AirTemperature for the current phase, which always uses the wall clock.
func ApplyTestTempPattern(obs *types.Observation, pattern *TestPattern) {
	phase := pattern.GetCyclePhase(nil)
	switch phase {
	case 0:
		obs.AirTemperature = 0.0
	case 1:
		obs.AirTemperature = 15.0
	case 2:
		obs.AirTemperature = 25.0
	case 3:
		obs.AirTemperature = 38.0
	}
}

// ApplyTestHumidityPattern applies a cycling humidity pattern: 30 -> 50 -> 70 -> 95 %.
//
// It sets obs.RelativeHumidity for the current phase, which always uses the wall clock.
func ApplyTestHumidityPattern(obs *types.Observation, pattern *TestPattern) {
	phase := pattern.GetCyclePhase(nil)
	switch phase {
	case 0:
		obs.RelativeHumidity = 30.0
	case 1:
		obs.RelativeHumidity = 50.0
	case 2:
		obs.RelativeHumidity = 70.0
	case 3:
		obs.RelativeHumidity = 95.0
	}
}

// ApplyTestPressurePattern applies a cycling pressure pattern: 980 -> 1000 -> 1020 -> 1040 mb.
//
// It sets obs.StationPressure for the current phase, which always uses the wall clock.
func ApplyTestPressurePattern(obs *types.Observation, pattern *TestPattern) {
	phase := pattern.GetCyclePhase(nil)
	switch phase {
	case 0:
		obs.StationPressure = 980.0
	case 1:
		obs.StationPressure = 1000.0
	case 2:
		obs.StationPressure = 1020.0
	case 3:
		obs.StationPressure = 1040.0
	}
}

// ApplyTestLuxPattern applies a cycling lux pattern: 0 -> 100 -> 10000 -> 50000 lux.
//
// obs.SolarRadiation is set alongside to 0, 10, 500 and 1000 W/m². The phase
// always uses the wall clock.
func ApplyTestLuxPattern(obs *types.Observation, pattern *TestPattern) {
	phase := pattern.GetCyclePhase(nil)
	switch phase {
	case 0:
		obs.Illuminance = 0.0
		obs.SolarRadiation = 0
	case 1:
		obs.Illuminance = 100.0
		obs.SolarRadiation = 10
	case 2:
		obs.Illuminance = 10000.0
		obs.SolarRadiation = 500
	case 3:
		obs.Illuminance = 50000.0
		obs.SolarRadiation = 1000
	}
}

// ApplyTestUVPattern applies a cycling UV pattern: 0 -> 2 -> 7 -> 11 index.
//
// It sets obs.UV for the current phase, which always uses the wall clock.
func ApplyTestUVPattern(obs *types.Observation, pattern *TestPattern) {
	phase := pattern.GetCyclePhase(nil)
	switch phase {
	case 0:
		obs.UV = 0
	case 1:
		obs.UV = 2
	case 2:
		obs.UV = 7
	case 3:
		obs.UV = 11
	}
}

// ApplyTestLightningPattern applies a cycling lightning pattern: none -> 1@20km -> 5@5km -> 10@1km.
//
// It sets obs.LightningStrikeCount and obs.LightningStrikeAvg; the phase
// always uses the wall clock.
func ApplyTestLightningPattern(obs *types.Observation, pattern *TestPattern) {
	phase := pattern.GetCyclePhase(nil)
	switch phase {
	case 0:
		obs.LightningStrikeCount = 0
		obs.LightningStrikeAvg = 0
	case 1:
		obs.LightningStrikeCount = 1
		obs.LightningStrikeAvg = 20.0
	case 2:
		obs.LightningStrikeCount = 5
		obs.LightningStrikeAvg = 5.0
	case 3:
		obs.LightningStrikeCount = 10
		obs.LightningStrikeAvg = 1.0
	}
}
