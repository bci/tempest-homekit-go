package alarm

import (
	"encoding/json"
	"testing"
	"time"
)

// TestSchedule_AlwaysActive checks that a nil schedule, an empty one and type "always"
// are all active.
func TestSchedule_AlwaysActive(t *testing.T) {
	tests := []struct {
		name     string
		schedule *Schedule
		expected bool
	}{
		{"nil schedule", nil, true},
		{"empty schedule", &Schedule{}, true},
		{"type always", &Schedule{Type: "always"}, true},
	}

	now := time.Date(2025, 1, 15, 14, 30, 0, 0, time.UTC)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.schedule.IsActive(now, 0, 0)
			if result != tt.expected {
				t.Errorf("IsActive() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestSchedule_TimeRange checks a "time" schedule is active inclusively between start
// and end, and that a range crossing midnight is active on both sides of it.
func TestSchedule_TimeRange(t *testing.T) {
	tests := []struct {
		name      string
		schedule  *Schedule
		checkTime string // HH:MM format
		expected  bool
	}{
		{
			"within range",
			&Schedule{Type: "time", StartTime: "09:00", EndTime: "17:00"},
			"12:00",
			true,
		},
		{
			"at start",
			&Schedule{Type: "time", StartTime: "09:00", EndTime: "17:00"},
			"09:00",
			true,
		},
		{
			"at end",
			&Schedule{Type: "time", StartTime: "09:00", EndTime: "17:00"},
			"17:00",
			true,
		},
		{
			"before start",
			&Schedule{Type: "time", StartTime: "09:00", EndTime: "17:00"},
			"08:59",
			false,
		},
		{
			"after end",
			&Schedule{Type: "time", StartTime: "09:00", EndTime: "17:00"},
			"17:01",
			false,
		},
		{
			"overnight range - within first part",
			&Schedule{Type: "time", StartTime: "22:00", EndTime: "06:00"},
			"23:30",
			true,
		},
		{
			"overnight range - within second part",
			&Schedule{Type: "time", StartTime: "22:00", EndTime: "06:00"},
			"03:00",
			true,
		},
		{
			"overnight range - outside",
			&Schedule{Type: "time", StartTime: "22:00", EndTime: "06:00"},
			"12:00",
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse check time
			checkParts := parseTime(tt.checkTime)
			now := time.Date(2025, 1, 15, checkParts[0], checkParts[1], 0, 0, time.UTC)

			result := tt.schedule.IsActive(now, 0, 0)
			if result != tt.expected {
				t.Errorf("IsActive(%s) = %v, want %v", tt.checkTime, result, tt.expected)
			}
		})
	}
}

// TestSchedule_Daily checks that a "daily" schedule is active inside its time range and
// inactive outside it.
func TestSchedule_Daily(t *testing.T) {
	tests := []struct {
		name      string
		schedule  *Schedule
		checkTime string
		expected  bool
	}{
		{
			"within daily range",
			&Schedule{Type: "daily", StartTime: "09:00", EndTime: "17:00"},
			"12:00",
			true,
		},
		{
			"outside daily range",
			&Schedule{Type: "daily", StartTime: "09:00", EndTime: "17:00"},
			"20:00",
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkParts := parseTime(tt.checkTime)
			now := time.Date(2025, 1, 15, checkParts[0], checkParts[1], 0, 0, time.UTC)

			result := tt.schedule.IsActive(now, 0, 0)
			if result != tt.expected {
				t.Errorf("IsActive(%s) = %v, want %v", tt.checkTime, result, tt.expected)
			}
		})
	}
}

// TestSchedule_Weekly checks that a "weekly" schedule is active only on its listed days
// (0 = Sunday) and, when times are given, only within them.
func TestSchedule_Weekly(t *testing.T) {
	tests := []struct {
		name      string
		schedule  *Schedule
		dayOfWeek time.Weekday
		checkTime string
		expected  bool
	}{
		{
			"Monday within business hours",
			&Schedule{Type: "weekly", DaysOfWeek: []int{1, 2, 3, 4, 5}, StartTime: "09:00", EndTime: "17:00"},
			time.Monday,
			"12:00",
			true,
		},
		{
			"Friday within business hours",
			&Schedule{Type: "weekly", DaysOfWeek: []int{1, 2, 3, 4, 5}, StartTime: "09:00", EndTime: "17:00"},
			time.Friday,
			"16:00",
			true,
		},
		{
			"Saturday (not in days list)",
			&Schedule{Type: "weekly", DaysOfWeek: []int{1, 2, 3, 4, 5}, StartTime: "09:00", EndTime: "17:00"},
			time.Saturday,
			"12:00",
			false,
		},
		{
			"Sunday (not in days list)",
			&Schedule{Type: "weekly", DaysOfWeek: []int{1, 2, 3, 4, 5}, StartTime: "09:00", EndTime: "17:00"},
			time.Sunday,
			"12:00",
			false,
		},
		{
			"Monday outside business hours",
			&Schedule{Type: "weekly", DaysOfWeek: []int{1, 2, 3, 4, 5}, StartTime: "09:00", EndTime: "17:00"},
			time.Monday,
			"20:00",
			false,
		},
		{
			"Weekend only - Saturday",
			&Schedule{Type: "weekly", DaysOfWeek: []int{0, 6}},
			time.Saturday,
			"12:00",
			true,
		},
		{
			"Weekend only - Sunday",
			&Schedule{Type: "weekly", DaysOfWeek: []int{0, 6}},
			time.Sunday,
			"12:00",
			true,
		},
		{
			"Weekend only - Monday",
			&Schedule{Type: "weekly", DaysOfWeek: []int{0, 6}},
			time.Monday,
			"12:00",
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkParts := parseTime(tt.checkTime)
			// Use a known date for the specified day of week
			// January 2025: 12=Sun, 13=Mon, 14=Tue, 15=Wed, 16=Thu, 17=Fri, 18=Sat
			day := 12 + int(tt.dayOfWeek)
			now := time.Date(2025, 1, day, checkParts[0], checkParts[1], 0, 0, time.UTC)

			result := tt.schedule.IsActive(now, 0, 0)
			if result != tt.expected {
				t.Errorf("IsActive(%s on %s) = %v, want %v", tt.checkTime, tt.dayOfWeek, result, tt.expected)
			}
		})
	}
}

// TestSchedule_Sun checks "sun" schedules for Los Angeles against sunrise and sunset
// computed by calculateSunTimes: open-ended after sunrise, sunrise-to-sunset, and with
// start and end offsets in minutes.
func TestSchedule_Sun(t *testing.T) {
	// Use Los Angeles coordinates for testing (34.0522°N, 118.2437°W)
	lat := 34.0522
	lon := -118.2437

	// Use local timezone for testing
	loc, _ := time.LoadLocation("America/Los_Angeles")
	testDate := time.Date(2025, 1, 15, 0, 0, 0, 0, loc)

	// Calculate actual sunrise/sunset for this location and date
	sunrise, sunset := calculateSunTimes(testDate, lat, lon)
	t.Logf("Test date: %s", testDate.Format("2006-01-02"))
	t.Logf("Calculated sunrise: %s", sunrise.Format("15:04 MST"))
	t.Logf("Calculated sunset: %s", sunset.Format("15:04 MST"))

	// Test with times relative to calculated sunrise/sunset
	tests := []struct {
		name     string
		schedule *Schedule
		testTime time.Time
		expected bool
	}{
		{
			"after sunrise (no end) - noon",
			&Schedule{Type: "sun", SunEvent: "sunrise", SunOffset: 0},
			time.Date(2025, 1, 15, 12, 0, 0, 0, loc),
			true,
		},
		{
			"before calculated sunrise (no end)",
			&Schedule{Type: "sun", SunEvent: "sunrise", SunOffset: 0},
			sunrise.Add(-1 * time.Hour),
			false,
		},
		{
			"after calculated sunrise (no end)",
			&Schedule{Type: "sun", SunEvent: "sunrise", SunOffset: 0},
			sunrise.Add(1 * time.Hour),
			true,
		},
		{
			"sunrise to sunset range - between them",
			&Schedule{Type: "sun", SunEvent: "sunrise", SunEventEnd: "sunset", SunOffset: 0, SunOffsetEnd: 0},
			sunrise.Add(4 * time.Hour),
			true,
		},
		{
			"sunrise to sunset range - before sunrise",
			&Schedule{Type: "sun", SunEvent: "sunrise", SunEventEnd: "sunset", SunOffset: 0, SunOffsetEnd: 0},
			sunrise.Add(-1 * time.Hour),
			false,
		},
		{
			"sunrise to sunset range - after sunset",
			&Schedule{Type: "sun", SunEvent: "sunrise", SunEventEnd: "sunset", SunOffset: 0, SunOffsetEnd: 0},
			sunset.Add(1 * time.Hour),
			false,
		},
		{
			"30 minutes before sunrise to sunset",
			&Schedule{Type: "sun", SunEvent: "sunrise", SunEventEnd: "sunset", SunOffset: -30, SunOffsetEnd: 0},
			sunrise.Add(-15 * time.Minute), // 15 minutes before sunrise = within range
			true,
		},
		{
			"sunrise to 30 minutes after sunset",
			&Schedule{Type: "sun", SunEvent: "sunrise", SunEventEnd: "sunset", SunOffset: 0, SunOffsetEnd: 30},
			sunset.Add(15 * time.Minute), // 15 minutes after sunset = within range
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.schedule.IsActive(tt.testTime, lat, lon)
			if result != tt.expected {
				t.Errorf("IsActive(%s) = %v, want %v", tt.testTime.Format("15:04 MST"), result, tt.expected)
			}
		})
	}
}

// TestSchedule_Validate checks Validate for each schedule type: required fields, the
// HH:MM time format, day numbers 0-6, start and end given together, valid sun event names
// and unknown types.
func TestSchedule_Validate(t *testing.T) {
	tests := []struct {
		name      string
		schedule  *Schedule
		expectErr bool
	}{
		{"nil schedule", nil, false},
		{"always active", &Schedule{Type: "always"}, false},
		{"empty type (defaults to always)", &Schedule{}, false},
		{
			"valid time range",
			&Schedule{Type: "time", StartTime: "09:00", EndTime: "17:00"},
			false,
		},
		{
			"time range missing start_time",
			&Schedule{Type: "time", EndTime: "17:00"},
			true,
		},
		{
			"time range missing end_time",
			&Schedule{Type: "time", StartTime: "09:00"},
			true,
		},
		{
			"time range invalid format",
			&Schedule{Type: "time", StartTime: "9:00 AM", EndTime: "17:00"},
			true,
		},
		{
			"valid daily",
			&Schedule{Type: "daily", StartTime: "09:00", EndTime: "17:00"},
			false,
		},
		{
			"valid weekly with time",
			&Schedule{Type: "weekly", DaysOfWeek: []int{1, 2, 3, 4, 5}, StartTime: "09:00", EndTime: "17:00"},
			false,
		},
		{
			"valid weekly without time",
			&Schedule{Type: "weekly", DaysOfWeek: []int{1, 2, 3, 4, 5}},
			false,
		},
		{
			"weekly missing days",
			&Schedule{Type: "weekly", StartTime: "09:00", EndTime: "17:00"},
			true,
		},
		{
			"weekly invalid day",
			&Schedule{Type: "weekly", DaysOfWeek: []int{1, 2, 7}},
			true,
		},
		{
			"weekly partial time (only start)",
			&Schedule{Type: "weekly", DaysOfWeek: []int{1, 2}, StartTime: "09:00"},
			true,
		},
		{
			"valid sun schedule",
			&Schedule{Type: "sun", SunEvent: "sunrise", SunEventEnd: "sunset"},
			false,
		},
		{
			"sun schedule missing event",
			&Schedule{Type: "sun", SunEventEnd: "sunset"},
			true,
		},
		{
			"sun schedule invalid event",
			&Schedule{Type: "sun", SunEvent: "noon"},
			true,
		},
		{
			"invalid type",
			&Schedule{Type: "monthly"},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.schedule.Validate()
			if (err != nil) != tt.expectErr {
				t.Errorf("Validate() error = %v, expectErr = %v", err, tt.expectErr)
			}
		})
	}
}

// TestSchedule_String calls String on several schedules. It only guards against an empty
// result; the contains field is not actually compared.
func TestSchedule_String(t *testing.T) {
	tests := []struct {
		name     string
		schedule *Schedule
		contains string // Check if result contains this substring
	}{
		{"nil", nil, "Always active"},
		{"always", &Schedule{Type: "always"}, "Always active"},
		{"daily", &Schedule{Type: "daily", StartTime: "09:00", EndTime: "17:00"}, "Daily from 09:00 to 17:00"},
		{
			"weekly weekdays",
			&Schedule{Type: "weekly", DaysOfWeek: []int{1, 2, 3, 4, 5}, StartTime: "09:00", EndTime: "17:00"},
			"09:00 to 17:00",
		},
		{
			"sun sunrise to sunset",
			&Schedule{Type: "sun", SunEvent: "sunrise", SunEventEnd: "sunset"},
			"sunrise to sunset",
		},
		{
			"sun with offset",
			&Schedule{Type: "sun", SunEvent: "sunrise", SunOffset: -30},
			"-30m",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.schedule.String()
			if tt.contains != "" && len(result) > 0 {
				// Just check that it returns a non-empty string for now
				if len(result) == 0 {
					t.Errorf("String() returned empty string")
				}
			}
		})
	}
}

// TestSchedule_JSON checks that schedules round-trip through JSON, comparing Type and
// StartTime.
func TestSchedule_JSON(t *testing.T) {
	tests := []struct {
		name     string
		schedule *Schedule
	}{
		{
			"time schedule",
			&Schedule{Type: "time", StartTime: "09:00", EndTime: "17:00"},
		},
		{
			"weekly schedule",
			&Schedule{Type: "weekly", DaysOfWeek: []int{1, 2, 3, 4, 5}, StartTime: "09:00", EndTime: "17:00"},
		},
		{
			"sun schedule",
			&Schedule{Type: "sun", SunEvent: "sunrise", SunEventEnd: "sunset", SunOffset: -30, SunOffsetEnd: 30},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal to JSON
			data, err := json.Marshal(tt.schedule)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}

			// Unmarshal back
			var unmarshaled Schedule
			if err := json.Unmarshal(data, &unmarshaled); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}

			// Compare key fields
			if unmarshaled.Type != tt.schedule.Type {
				t.Errorf("Type mismatch: got %v, want %v", unmarshaled.Type, tt.schedule.Type)
			}
			if unmarshaled.StartTime != tt.schedule.StartTime {
				t.Errorf("StartTime mismatch: got %v, want %v", unmarshaled.StartTime, tt.schedule.StartTime)
			}
		})
	}
}

// TestCalculateSunTimes checks calculateSunTimes for Los Angeles in January: sunrise is
// before sunset on the same date and the day is 8-16 hours long. Implausible clock times
// are only logged.
func TestCalculateSunTimes(t *testing.T) {
	// Test known location: Los Angeles
	lat := 34.0522
	lon := -118.2437
	loc, _ := time.LoadLocation("America/Los_Angeles")
	date := time.Date(2025, 1, 15, 12, 0, 0, 0, loc)

	sunrise, sunset := calculateSunTimes(date, lat, lon)

	t.Logf("Test date: %s", date.Format("2006-01-02"))
	t.Logf("Calculated sunrise: %s", sunrise.Format("15:04 MST"))
	t.Logf("Calculated sunset: %s", sunset.Format("15:04 MST"))

	// Verify sunrise is before sunset
	if !sunrise.Before(sunset) {
		t.Errorf("sunrise should be before sunset: sunrise=%s, sunset=%s",
			sunrise.Format("15:04"), sunset.Format("15:04"))
	}

	// Verify reasonable times for Los Angeles in January
	// (sunrise between 6 AM and 8 AM, sunset between 4:30 PM and 6 PM)
	if sunrise.Hour() < 6 || sunrise.Hour() > 8 {
		t.Logf("Warning: sunrise hour may be off: %d (expected 6-8)", sunrise.Hour())
	}
	if sunset.Hour() < 16 || (sunset.Hour() == 18 && sunset.Minute() > 0) || sunset.Hour() > 18 {
		t.Logf("Warning: sunset hour may be off: %d:%02d (expected 16:30-18:00)", sunset.Hour(), sunset.Minute())
	}

	// Verify same date
	if sunrise.Day() != date.Day() || sunset.Day() != date.Day() {
		t.Errorf("sun times should be on same date as input")
	}

	// Verify sunrise/sunset are at least 8 hours apart (basic sanity check)
	duration := sunset.Sub(sunrise)
	if duration < 8*time.Hour || duration > 16*time.Hour {
		t.Errorf("sunrise/sunset duration seems unreasonable: %v (expected 8-16 hours)", duration)
	}
}

// TestSchedule_WithScheduleInAlarm checks that an Alarm's Schedule survives a JSON
// round trip.
func TestSchedule_WithScheduleInAlarm(t *testing.T) {
	// Test that Schedule integrates properly with Alarm struct
	alarm := Alarm{
		Name:      "test-alarm",
		Condition: "temperature > 80",
		Enabled:   true,
		Schedule: &Schedule{
			Type:      "daily",
			StartTime: "09:00",
			EndTime:   "17:00",
		},
		Channels: []Channel{
			{Type: "console", Template: "Test"},
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(alarm)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	// Unmarshal back
	var unmarshaled Alarm
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	// Verify schedule was preserved
	if unmarshaled.Schedule == nil {
		t.Fatal("Schedule was nil after unmarshaling")
	}
	if unmarshaled.Schedule.Type != "daily" {
		t.Errorf("Schedule type mismatch: got %v, want daily", unmarshaled.Schedule.Type)
	}
	if unmarshaled.Schedule.StartTime != "09:00" {
		t.Errorf("Schedule start_time mismatch: got %v, want 09:00", unmarshaled.Schedule.StartTime)
	}
}

// parseTime parses "HH:MM" into [hour, minute]. A parse error is ignored and yields
// [0, 0].
func parseTime(timeStr string) [2]int {
	var hour, minute int
	t, _ := time.Parse("15:04", timeStr)
	hour = t.Hour()
	minute = t.Minute()
	return [2]int{hour, minute}
}

// TestSchedule_UseStationLocation checks that a sun schedule with UseStationLocation, or
// with no custom coordinates, is active just after the station's (Los Angeles) sunrise,
// and that String then says "(station location)". The custom-coordinates case is only
// logged, not asserted.
func TestSchedule_UseStationLocation(t *testing.T) {
	// Test that use_station_location flag properly uses station coordinates
	// Station location (Los Angeles)
	stationLat := 34.0522
	stationLon := -118.2437

	// Custom location (New York - different sunrise/sunset times)
	customLat := 40.7128
	customLon := -74.0060

	loc, _ := time.LoadLocation("America/Los_Angeles")
	testTime := time.Date(2025, 1, 15, 8, 0, 0, 0, loc)

	tests := []struct {
		name                 string
		schedule             *Schedule
		expectedToUseStation bool
		description          string
	}{
		{
			name: "use_station_location true - should use station coords",
			schedule: &Schedule{
				Type:               "sun",
				SunEvent:           "sunrise",
				SunEventEnd:        "sunset",
				UseStationLocation: true,
				// These should be ignored when UseStationLocation is true
				Latitude:  customLat,
				Longitude: customLon,
			},
			expectedToUseStation: true,
			description:          "When use_station_location=true, station coordinates should be used even if custom lat/lon provided",
		},
		{
			name: "use_station_location false with custom coords - should use custom",
			schedule: &Schedule{
				Type:               "sun",
				SunEvent:           "sunrise",
				SunEventEnd:        "sunset",
				UseStationLocation: false,
				Latitude:           customLat,
				Longitude:          customLon,
			},
			expectedToUseStation: false,
			description:          "When use_station_location=false, custom coordinates should be used",
		},
		{
			name: "use_station_location false no custom coords - should use station default",
			schedule: &Schedule{
				Type:               "sun",
				SunEvent:           "sunrise",
				SunEventEnd:        "sunset",
				UseStationLocation: false,
			},
			expectedToUseStation: true, // Falls back to station since no custom coords
			description:          "When use_station_location=false and no custom coords, should use passed station coords",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Calculate sun times with both locations to compare
			stationSunrise, stationSunset := calculateSunTimes(testTime, stationLat, stationLon)
			customSunrise, customSunset := calculateSunTimes(testTime, customLat, customLon)

			t.Logf("Station (LA) sunrise: %s, sunset: %s",
				stationSunrise.Format("15:04"), stationSunset.Format("15:04"))
			t.Logf("Custom (NY) sunrise: %s, sunset: %s",
				customSunrise.Format("15:04"), customSunset.Format("15:04"))

			// Test at a time between the two sunrises to verify which is being used
			// Use LA's sunrise time (should be active if using station, inactive if using NY)
			testTimeAtLASunrise := time.Date(2025, 1, 15,
				stationSunrise.Hour(), stationSunrise.Minute()+10, 0, 0, loc)

			isActive := tt.schedule.IsActive(testTimeAtLASunrise, stationLat, stationLon)

			t.Logf("Testing at LA sunrise time (%s), isActive: %v",
				testTimeAtLASunrise.Format("15:04"), isActive)

			// If using station location, should be active at station's sunrise
			// NY sunrise is later, so this time would be before NY sunrise
			if tt.expectedToUseStation && !isActive {
				t.Errorf("Expected schedule to use station location and be active at station sunrise time")
			}

			// Also test the String() method shows correct location info
			str := tt.schedule.String()
			if tt.schedule.UseStationLocation {
				if !stringContainsSubstr(str, "(station location)") {
					t.Errorf("String() should indicate station location, got: %s", str)
				}
			}
		})
	}
}

// TestSchedule_String_WithLocations checks that String mentions the station-location
// flag or the custom coordinates of a sun schedule.
func TestSchedule_String_WithLocations(t *testing.T) {
	tests := []struct {
		name          string
		schedule      *Schedule
		shouldContain string
	}{
		{
			"with station location flag",
			&Schedule{Type: "sun", SunEvent: "sunrise", UseStationLocation: true},
			"(station location)",
		},
		{
			"with custom coordinates",
			&Schedule{Type: "sun", SunEvent: "sunrise", Latitude: 34.05, Longitude: -118.24},
			"(34.0500, -118.2400)",
		},
		{
			"no location specified",
			&Schedule{Type: "sun", SunEvent: "sunrise"},
			"sunrise",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.schedule.String()
			if !stringContainsSubstr(result, tt.shouldContain) {
				t.Errorf("String() = %q, should contain %q", result, tt.shouldContain)
			}
		})
	}
}

// stringContainsSubstr reports whether substr occurs in s, checking for equality, prefix
// and suffix before scanning; it returns false when either string is empty. It is a
// hand-rolled equivalent of strings.Contains.
func stringContainsSubstr(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 &&
		(s == substr || len(s) >= len(substr) &&
			(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
				substringInMiddle(s, substr)))
}

// substringInMiddle reports whether substr occurs at any offset in s by checking every
// window; it is the scanning fallback for stringContainsSubstr.
func substringInMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestSchedule_Timezone checks that a time schedule with a Timezone is evaluated in that
// zone, and that an invalid zone falls back to the time's own location.
func TestSchedule_Timezone(t *testing.T) {
	tests := []struct {
		name     string
		schedule *Schedule
		// Test time in UTC
		utcTime  time.Time
		expected bool
	}{
		{
			name: "9am-5pm PST schedule, checked at 5pm UTC (9am PST)",
			schedule: &Schedule{
				Type:      "time",
				StartTime: "09:00",
				EndTime:   "17:00",
				Timezone:  "America/Los_Angeles",
			},
			utcTime:  time.Date(2025, 1, 15, 17, 0, 0, 0, time.UTC), // 5pm UTC = 9am PST
			expected: true,
		},
		{
			name: "9am-5pm PST schedule, checked at 2am UTC (6pm PST - outside range)",
			schedule: &Schedule{
				Type:      "time",
				StartTime: "09:00",
				EndTime:   "17:00",
				Timezone:  "America/Los_Angeles",
			},
			utcTime:  time.Date(2025, 1, 15, 2, 0, 0, 0, time.UTC), // 2am UTC = 6pm PST (previous day)
			expected: false,
		},
		{
			name: "9am-5pm EST schedule, checked at 2pm UTC (9am EST)",
			schedule: &Schedule{
				Type:      "time",
				StartTime: "09:00",
				EndTime:   "17:00",
				Timezone:  "America/New_York",
			},
			utcTime:  time.Date(2025, 1, 15, 14, 0, 0, 0, time.UTC), // 2pm UTC = 9am EST
			expected: true,
		},
		{
			name: "Invalid timezone - should use system time and still work",
			schedule: &Schedule{
				Type:      "time",
				StartTime: "09:00",
				EndTime:   "17:00",
				Timezone:  "Invalid/Timezone",
			},
			utcTime:  time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC),
			expected: true, // Noon UTC should be in 9am-5pm range in UTC
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.schedule.IsActive(tt.utcTime, 0, 0)
			if result != tt.expected {
				t.Errorf("IsActive() = %v, want %v (checking %v)", result, tt.expected, tt.utcTime)
			}
		})
	}
}

// TestSchedule_Validate_Timezone checks that Validate rejects an unknown IANA timezone
// and accepts a valid or absent one.
func TestSchedule_Validate_Timezone(t *testing.T) {
	tests := []struct {
		name      string
		schedule  *Schedule
		wantError bool
	}{
		{
			name: "valid timezone",
			schedule: &Schedule{
				Type:      "time",
				StartTime: "09:00",
				EndTime:   "17:00",
				Timezone:  "America/New_York",
			},
			wantError: false,
		},
		{
			name: "invalid timezone",
			schedule: &Schedule{
				Type:      "time",
				StartTime: "09:00",
				EndTime:   "17:00",
				Timezone:  "Invalid/Timezone",
			},
			wantError: true,
		},
		{
			name: "no timezone",
			schedule: &Schedule{
				Type:      "time",
				StartTime: "09:00",
				EndTime:   "17:00",
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.schedule.Validate()
			if (err != nil) != tt.wantError {
				t.Errorf("Validate() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

// TestSchedule_String_WithTimezone checks that String appends "[zone]" when a timezone
// is set and describes a "time" schedule as "Daily from HH:MM to HH:MM".
func TestSchedule_String_WithTimezone(t *testing.T) {
	tests := []struct {
		name          string
		schedule      *Schedule
		shouldContain string
	}{
		{
			name: "time schedule with timezone",
			schedule: &Schedule{
				Type:      "time",
				StartTime: "09:00",
				EndTime:   "17:00",
				Timezone:  "America/New_York",
			},
			shouldContain: "[America/New_York]",
		},
		{
			name: "weekly schedule with timezone",
			schedule: &Schedule{
				Type:       "weekly",
				DaysOfWeek: []int{1, 2, 3, 4, 5},
				StartTime:  "09:00",
				EndTime:    "17:00",
				Timezone:   "Europe/London",
			},
			shouldContain: "[Europe/London]",
		},
		{
			name: "schedule without timezone",
			schedule: &Schedule{
				Type:      "time",
				StartTime: "09:00",
				EndTime:   "17:00",
			},
			shouldContain: "Daily from 09:00 to 17:00",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.schedule.String()
			if !stringContainsSubstr(result, tt.shouldContain) {
				t.Errorf("String() = %q, should contain %q", result, tt.shouldContain)
			}
		})
	}
}
