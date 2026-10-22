// Package types defines common types used across the application.
// This package helps break import cycles by providing shared type definitions.
package types

// Observation represents a weather observation from a Tempest station.
//
// It is populated from the WeatherFlow REST API (pkg/weather), from UDP
// broadcasts, or synthesised by pkg/generator. Values use the API's metric
// units as noted per field (rain amounts are in mm); conversion to imperial happens at display time.
// Fields absent from a source are left at their zero value.
type Observation struct {
	Timestamp            int64   `json:"timestamp"`                     // Observation time, Unix epoch seconds
	WindLull             float64 `json:"wind_lull"`                     // Minimum wind speed over the interval, m/s
	WindAvg              float64 `json:"wind_avg"`                      // Average wind speed over the interval, m/s
	WindGust             float64 `json:"wind_gust"`                     // Maximum wind speed over the interval, m/s
	WindDirection        float64 `json:"wind_direction"`                // Wind direction, degrees from north (0-360)
	StationPressure      float64 `json:"station_pressure"`              // Pressure at station elevation (not sea-level), mb
	AirTemperature       float64 `json:"air_temperature"`               // Air temperature, °C
	RelativeHumidity     float64 `json:"relative_humidity"`             // Relative humidity, percent
	Illuminance          float64 `json:"illuminance"`                   // Light level, lux (REST API field "brightness")
	UV                   int     `json:"uv"`                            // UV index
	SolarRadiation       float64 `json:"solar_radiation"`               // Solar irradiance, W/m²
	RainAccumulated      float64 `json:"rain_accumulated"`              // Incremental rain since last obs (from "precip" field)
	RainDailyTotal       float64 `json:"rain_daily_total"`              // Total rain since midnight (from "precip_accum_local_day" field)
	PrecipitationType    int     `json:"precipitation_type"`            // Tempest code: 0 none, 1 rain, 2 hail, 3 rain plus hail
	LightningStrikeAvg   float64 `json:"lightning_strike_avg_distance"` // Average strike distance over the interval, km
	LightningStrikeCount int     `json:"lightning_strike_count"`        // Strikes detected over the interval
	Battery              float64 `json:"battery"`                       // Station battery voltage, volts
	ReportInterval       int     `json:"report_interval"`               // Reporting interval, minutes
}
