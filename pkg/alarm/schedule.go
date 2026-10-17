package alarm

import (
	"encoding/json"
	"fmt"
	"math"
	"time"
)

// Schedule defines when an alarm is active. Outside its window the alarm is
// skipped entirely (not evaluated, so change-detection baselines are not
// updated by the evaluator either). A nil Schedule, or Type "" or "always",
// means always active.
type Schedule struct {
	// Type of schedule: "always" (default), "time", "daily", "weekly", "sun".
	// "time" and "daily" behave identically.
	Type string `json:"type,omitempty"`

	// StartTime and EndTime bound a time-of-day window in 24-hour "HH:MM" form,
	// inclusive of the end minute; an end earlier than the start wraps past
	// midnight (22:00 to 06:00). Used by "time"/"daily" and optionally "weekly".
	// Example JSON: {"start_time": "09:00", "end_time": "17:00"}
	StartTime string `json:"start_time,omitempty"`
	EndTime   string `json:"end_time,omitempty"`

	// DaysOfWeek lists the active days for "weekly" (0=Sunday, 1=Monday, ..., 6=Saturday).
	// Example: [1,2,3,4,5] for Monday-Friday
	DaysOfWeek []int `json:"days_of_week,omitempty"`

	// SunEvent and SunEventEnd define a "sun" schedule. Only "sunrise" and
	// "sunset" are accepted by [Schedule.Validate]. With no SunEventEnd the
	// schedule is active from SunEvent until midnight; with one it is active
	// between the two events on the same calendar day.
	SunEvent    string `json:"sun_event,omitempty"`     // "sunrise", "sunset"
	SunEventEnd string `json:"sun_event_end,omitempty"` // Optional: for ranges like "sunrise" to "sunset"

	// SunOffset and SunOffsetEnd shift SunEvent and SunEventEnd by minutes (can be
	// negative for "before" or positive for "after").
	// Example: -30 means 30 minutes before sunrise
	SunOffset    int `json:"sun_offset,omitempty"`
	SunOffsetEnd int `json:"sun_offset_end,omitempty"`

	// Latitude and Longitude give the location for sun calculations in decimal
	// degrees. When both are 0 (or UseStationLocation is set) the location passed
	// to [Schedule.IsActive] by the manager is used instead.
	Latitude  float64 `json:"latitude,omitempty"`
	Longitude float64 `json:"longitude,omitempty"`

	// UseStationLocation - if true, use the weather station's coordinates for sun calculations
	// instead of the latitude/longitude specified above or the manager's default location.
	// This is useful when you want sunrise/sunset times based on the actual station location.
	UseStationLocation bool `json:"use_station_location,omitempty"`

	// Timezone - IANA timezone name (e.g., "America/New_York", "Europe/London")
	// If not specified, uses system's local timezone
	// Examples: "America/Los_Angeles", "America/Chicago", "America/New_York", "UTC"
	Timezone string `json:"timezone,omitempty"`
}

// IsActive checks if the alarm should be active at the given time
// Returns true if no schedule is defined (always active) or if current time is within schedule
//
// now is converted to Timezone first; a timezone that fails to load is
// ignored and now is used as given. lat and lon (decimal degrees, typically
// the station location) are used only by "sun" schedules. The schedule fails
// open: an unknown type, unparsable times or an unknown location all yield
// true rather than silencing the alarm.
func (s *Schedule) IsActive(now time.Time, lat, lon float64) bool {
	// No schedule or type="always" means always active
	if s == nil || s.Type == "" || s.Type == "always" {
		return true
	}

	// Convert time to schedule's timezone if specified
	if s.Timezone != "" {
		loc, err := time.LoadLocation(s.Timezone)
		if err == nil {
			now = now.In(loc)
		}
		// If timezone load fails, continue with original time (system timezone)
	}

	switch s.Type {
	case "time":
		return s.isActiveTimeRange(now)
	case "daily":
		return s.isActiveDaily(now)
	case "weekly":
		return s.isActiveWeekly(now)
	case "sun":
		return s.isActiveSun(now, lat, lon)
	default:
		// Unknown type, default to active
		return true
	}
}

// isActiveTimeRange checks if current time is within start_time and end_time,
// at minute resolution and inclusive at both ends, wrapping past midnight when
// the end is before the start. Missing or unparsable times return true.
func (s *Schedule) isActiveTimeRange(now time.Time) bool {
	if s.StartTime == "" || s.EndTime == "" {
		return true // Missing required fields, default to active
	}

	start, err := parseTimeOfDay(s.StartTime)
	if err != nil {
		return true // Parse error, default to active
	}

	end, err := parseTimeOfDay(s.EndTime)
	if err != nil {
		return true
	}

	nowMinutes := now.Hour()*60 + now.Minute()

	// Handle overnight ranges (e.g., 22:00 to 06:00)
	if end < start {
		return nowMinutes >= start || nowMinutes <= end
	}

	return nowMinutes >= start && nowMinutes <= end
}

// isActiveDaily checks both time range and applies it daily; it is identical
// to isActiveTimeRange.
func (s *Schedule) isActiveDaily(now time.Time) bool {
	return s.isActiveTimeRange(now)
}

// isActiveWeekly checks if current day of week is in the allowed list and time is in range.
// An empty DaysOfWeek means every day; the time range is checked only when
// both StartTime and EndTime are set.
func (s *Schedule) isActiveWeekly(now time.Time) bool {
	if len(s.DaysOfWeek) == 0 {
		return true // No day restriction, always active
	}

	// Get current day of week (0=Sunday, 1=Monday, ..., 6=Saturday)
	currentDay := int(now.Weekday())

	// Check if current day is in the allowed list
	dayAllowed := false
	for _, day := range s.DaysOfWeek {
		if day == currentDay {
			dayAllowed = true
			break
		}
	}

	if !dayAllowed {
		return false
	}

	// If time range is specified, check that too
	if s.StartTime != "" && s.EndTime != "" {
		return s.isActiveTimeRange(now)
	}

	return true
}

// isActiveSun checks if current time is within sunrise/sunset based schedule.
//
// Location: the schedule's own Latitude/Longitude are used unless
// UseStationLocation is set or both are 0, in which case lat/lon from the
// caller are used; if the result is still 0,0 the schedule is active. Without
// SunEventEnd it is active from the start event until the end of the day;
// with it, between the two events on the same day, so a range such as
// "sunset" to "sunrise" never matches.
func (s *Schedule) isActiveSun(now time.Time, lat, lon float64) bool {
	// Priority order for location:
	// 1. If UseStationLocation is true, use lat/lon passed from manager (station location)
	// 2. If schedule has explicit lat/lon, use those
	// 3. Otherwise use manager's default lat/lon

	if !s.UseStationLocation {
		// Only override if not using station location and schedule has explicit coordinates
		if s.Latitude != 0 || s.Longitude != 0 {
			lat = s.Latitude
			lon = s.Longitude
		}
	}
	// If UseStationLocation is true, we use the lat/lon passed in (from manager/station)

	// If no location provided, can't calculate sun times
	if lat == 0 && lon == 0 {
		return true // Default to active if no location
	}

	// Calculate sunrise and sunset for today
	sunrise, sunset := calculateSunTimes(now, lat, lon)

	// Apply offsets
	var startTime, endTime time.Time

	switch s.SunEvent {
	case "sunrise":
		startTime = sunrise.Add(time.Duration(s.SunOffset) * time.Minute)
	case "sunset":
		startTime = sunset.Add(time.Duration(s.SunOffset) * time.Minute)
	default:
		return true // Unknown sun event, default to active
	}

	// If no end event, check if we're past the start
	if s.SunEventEnd == "" {
		return now.After(startTime) || now.Equal(startTime)
	}

	// Calculate end time
	switch s.SunEventEnd {
	case "sunrise":
		endTime = sunrise.Add(time.Duration(s.SunOffsetEnd) * time.Minute)
	case "sunset":
		endTime = sunset.Add(time.Duration(s.SunOffsetEnd) * time.Minute)
	default:
		return true
	}

	// Check if current time is between start and end
	return (now.After(startTime) || now.Equal(startTime)) && (now.Before(endTime) || now.Equal(endTime))
}

// parseTimeOfDay parses "HH:MM" format and returns minutes since midnight. It
// returns an error when timeStr is not a valid 24-hour time.
func parseTimeOfDay(timeStr string) (int, error) {
	t, err := time.Parse("15:04", timeStr)
	if err != nil {
		return 0, fmt.Errorf("invalid time format (use HH:MM): %w", err)
	}
	return t.Hour()*60 + t.Minute(), nil
}

// calculateSunTimes calculates sunrise and sunset times for a given date and location
// Uses simplified algorithm (adequate for scheduling purposes, not astronomical precision)
// Algorithm based on NOAA solar calculator
//
// latitude and longitude are decimal degrees (east positive). The results are
// on date's calendar day in date's location. In polar night both are midnight;
// in polar day sunrise is 00:00:00 and sunset 23:59:59. Note the algorithm
// yields UTC hours, but they are placed into date's location without
// conversion, so results are only correct when date is in UTC.
func calculateSunTimes(date time.Time, latitude, longitude float64) (sunrise, sunset time.Time) {
	// Convert to Julian day
	y := date.Year()
	m := int(date.Month())
	d := date.Day()

	if m <= 2 {
		y--
		m += 12
	}

	a := y / 100
	b := 2 - a + a/4
	jd := math.Floor(365.25*float64(y+4716)) + math.Floor(30.6001*float64(m+1)) + float64(d) + float64(b) - 1524.5

	// Days since J2000.0
	n := jd - 2451545.0

	// Mean solar time
	j := n - longitude/360.0

	// Solar mean anomaly
	m0 := 357.5291 + 0.98560028*j
	m0 = math.Mod(m0, 360.0)

	// Equation of center
	c := 1.9148*math.Sin(m0*math.Pi/180.0) + 0.0200*math.Sin(2*m0*math.Pi/180.0) + 0.0003*math.Sin(3*m0*math.Pi/180.0)

	// Ecliptic longitude
	lambda := math.Mod(m0+c+180.0+102.9372, 360.0)

	// Solar transit
	jTransit := 2451545.0 + j + 0.0053*math.Sin(m0*math.Pi/180.0) - 0.0069*math.Sin(2*lambda*math.Pi/180.0)

	// Declination of the sun
	delta := math.Asin(math.Sin(lambda*math.Pi/180.0) * math.Sin(23.44*math.Pi/180.0))

	// Hour angle
	cosOmega := (math.Sin(-0.833*math.Pi/180.0) - math.Sin(latitude*math.Pi/180.0)*math.Sin(delta)) /
		(math.Cos(latitude*math.Pi/180.0) * math.Cos(delta))

	// Handle polar day/night
	if cosOmega > 1 {
		// Sun never rises
		sunrise = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
		sunset = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
		return
	}
	if cosOmega < -1 {
		// Sun never sets
		sunrise = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
		sunset = time.Date(date.Year(), date.Month(), date.Day(), 23, 59, 59, 0, date.Location())
		return
	}

	omega := math.Acos(cosOmega) * 180.0 / math.Pi

	// Julian dates for sunrise and sunset
	jRise := jTransit - omega/360.0
	jSet := jTransit + omega/360.0

	// Convert back to UTC time
	sunriseUTC := (jRise - jd) * 24.0 // Hours since midnight
	sunsetUTC := (jSet - jd) * 24.0   // Hours since midnight

	// Normalize
	for sunriseUTC < 0 {
		sunriseUTC += 24
	}
	for sunriseUTC >= 24 {
		sunriseUTC -= 24
	}
	for sunsetUTC < 0 {
		sunsetUTC += 24
	}
	for sunsetUTC >= 24 {
		sunsetUTC -= 24
	}

	// Convert to time.Time in local timezone
	sunriseHour := int(sunriseUTC)
	sunriseMin := int((sunriseUTC - float64(sunriseHour)) * 60)
	sunsetHour := int(sunsetUTC)
	sunsetMin := int((sunsetUTC - float64(sunsetHour)) * 60)

	sunrise = time.Date(date.Year(), date.Month(), date.Day(), sunriseHour, sunriseMin, 0, 0, date.Location())
	sunset = time.Date(date.Year(), date.Month(), date.Day(), sunsetHour, sunsetMin, 0, 0, date.Location())

	return sunrise, sunset
}

// Validate checks if the schedule configuration is valid.
//
// A nil schedule or Type "" / "always" is valid. Otherwise the timezone must
// load, and the type-specific fields must be present and well formed: start
// and end times for "time"/"daily", days 0-6 (and both or neither times) for
// "weekly", and sunrise/sunset events for "sun". Unknown types are rejected.
func (s *Schedule) Validate() error {
	if s == nil || s.Type == "" || s.Type == "always" {
		return nil
	}

	// Validate timezone if specified
	if s.Timezone != "" {
		if _, err := time.LoadLocation(s.Timezone); err != nil {
			return fmt.Errorf("invalid timezone: %w", err)
		}
	}

	switch s.Type {
	case "time", "daily":
		if s.StartTime == "" || s.EndTime == "" {
			return fmt.Errorf("start_time and end_time are required for type '%s'", s.Type)
		}
		if _, err := parseTimeOfDay(s.StartTime); err != nil {
			return fmt.Errorf("invalid start_time: %w", err)
		}
		if _, err := parseTimeOfDay(s.EndTime); err != nil {
			return fmt.Errorf("invalid end_time: %w", err)
		}

	case "weekly":
		if len(s.DaysOfWeek) == 0 {
			return fmt.Errorf("days_of_week is required for type 'weekly'")
		}
		for _, day := range s.DaysOfWeek {
			if day < 0 || day > 6 {
				return fmt.Errorf("invalid day of week: %d (must be 0-6, where 0=Sunday)", day)
			}
		}
		// Time range is optional for weekly
		if s.StartTime != "" || s.EndTime != "" {
			if s.StartTime == "" || s.EndTime == "" {
				return fmt.Errorf("both start_time and end_time must be specified if using time range with weekly schedule")
			}
			if _, err := parseTimeOfDay(s.StartTime); err != nil {
				return fmt.Errorf("invalid start_time: %w", err)
			}
			if _, err := parseTimeOfDay(s.EndTime); err != nil {
				return fmt.Errorf("invalid end_time: %w", err)
			}
		}

	case "sun":
		if s.SunEvent == "" {
			return fmt.Errorf("sun_event is required for type 'sun' (e.g., 'sunrise' or 'sunset')")
		}
		if s.SunEvent != "sunrise" && s.SunEvent != "sunset" {
			return fmt.Errorf("invalid sun_event: %s (must be 'sunrise' or 'sunset')", s.SunEvent)
		}
		if s.SunEventEnd != "" {
			if s.SunEventEnd != "sunrise" && s.SunEventEnd != "sunset" {
				return fmt.Errorf("invalid sun_event_end: %s (must be 'sunrise' or 'sunset')", s.SunEventEnd)
			}
		}
		// Note: Latitude/Longitude can be provided here or passed at evaluation time

	default:
		return fmt.Errorf("invalid schedule type: %s (must be 'always', 'time', 'daily', 'weekly', or 'sun')", s.Type)
	}

	return nil
}

// String returns a human-readable description of the schedule, such as
// "Daily from 09:00 to 17:00 [UTC]" or "After sunset -30m (station location)".
// It is safe to call on a nil Schedule.
func (s *Schedule) String() string {
	if s == nil || s.Type == "" || s.Type == "always" {
		return "Always active (24/7)"
	}

	// Add timezone suffix if specified
	tzSuffix := ""
	if s.Timezone != "" {
		tzSuffix = fmt.Sprintf(" [%s]", s.Timezone)
	}

	switch s.Type {
	case "time", "daily":
		return fmt.Sprintf("Daily from %s to %s%s", s.StartTime, s.EndTime, tzSuffix)

	case "weekly":
		dayNames := []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
		var days []string
		for _, day := range s.DaysOfWeek {
			if day >= 0 && day <= 6 {
				days = append(days, dayNames[day])
			}
		}
		daysStr := fmt.Sprintf("%v", days)
		if s.StartTime != "" && s.EndTime != "" {
			return fmt.Sprintf("%s from %s to %s%s", daysStr, s.StartTime, s.EndTime, tzSuffix)
		}
		return fmt.Sprintf("%s (all day)%s", daysStr, tzSuffix)

	case "sun":
		offsetStr := ""
		if s.SunOffset != 0 {
			if s.SunOffset > 0 {
				offsetStr = fmt.Sprintf(" +%dm", s.SunOffset)
			} else {
				offsetStr = fmt.Sprintf(" %dm", s.SunOffset)
			}
		}

		locationStr := ""
		if s.UseStationLocation {
			locationStr = " (station location)"
		} else if s.Latitude != 0 || s.Longitude != 0 {
			locationStr = fmt.Sprintf(" (%.4f, %.4f)", s.Latitude, s.Longitude)
		}

		if s.SunEventEnd != "" {
			endOffsetStr := ""
			if s.SunOffsetEnd != 0 {
				if s.SunOffsetEnd > 0 {
					endOffsetStr = fmt.Sprintf(" +%dm", s.SunOffsetEnd)
				} else {
					endOffsetStr = fmt.Sprintf(" %dm", s.SunOffsetEnd)
				}
			}
			return fmt.Sprintf("%s%s to %s%s%s", s.SunEvent, offsetStr, s.SunEventEnd, endOffsetStr, locationStr)
		}

		return fmt.Sprintf("After %s%s%s", s.SunEvent, offsetStr, locationStr)

	default:
		return fmt.Sprintf("Unknown schedule type: %s", s.Type)
	}
}

// MarshalJSON implements custom JSON marshaling to handle nil schedules,
// encoding a nil *Schedule as null. The local Alias type drops the methods so
// the call does not recurse.
func (s *Schedule) MarshalJSON() ([]byte, error) {
	if s == nil {
		return json.Marshal(nil)
	}

	type Alias Schedule
	return json.Marshal((*Alias)(s))
}

// UnmarshalJSON implements custom JSON unmarshaling. It decodes through an
// Alias type (avoiding recursion) with the default field mapping and returns
// any decoding error.
func (s *Schedule) UnmarshalJSON(data []byte) error {
	type Alias Schedule
	aux := (*Alias)(s)
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	return nil
}
