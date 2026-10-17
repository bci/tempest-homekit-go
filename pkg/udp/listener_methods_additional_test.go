package udp

import (
	"fmt"
	"testing"

	"tempest-homekit-go/pkg/weather"
)

// TestAddObservationRolloverAndGetLatestCopy tests that addObservation rolls over
// when capacity is reached and that GetLatestObservation returns a copy
// (modifying returned slice/item does not affect internal storage).
func TestAddObservationRolloverAndGetLatestCopy(t *testing.T) {
	l := &UDPListener{
		observations:   make([]weather.Observation, 0, 3),
		maxHistorySize: 3,
	}

	// Add 4 observations; the first should be dropped and the last three kept.
	for i := 0; i < 4; i++ {
		o := weather.Observation{Timestamp: int64(100 + i), AirTemperature: float64(60 + i)}
		l.addObservation(o)
	}

	obs := l.GetObservations()
	if len(obs) != 3 {
		t.Fatalf("expected 3 observations after rollover, got %d", len(obs))
	}

	// Ensure the earliest (100) was dropped and remaining times are 101,102,103
	if obs[0].Timestamp != 101 {
		t.Fatalf("expected earliest retained timestamp 101, got %d", obs[0].Timestamp)
	}

	// GetLatestObservation should return a copy; mutate it and ensure the internal
	// latest stored observation is unchanged.
	latest := l.GetLatestObservation()
	if latest == nil {
		t.Fatal("expected latest observation, got nil")
	}
	// mutate returned value
	latest.AirTemperature = 9999

	internalLatest := l.GetLatestObservation()
	if internalLatest.AirTemperature == 9999 {
		t.Fatalf("modifying returned latest should not change internal storage")
	}
}

// TestDeviceAndHubStatusParsing tests parsing of device and hub status fields from
// sample messages, and that processObservationAir adds an observation.
func TestDeviceAndHubStatusParsing(t *testing.T) {
	// Construct a UDP message and ensure device/hub status processing stores values
	l := &UDPListener{}

	msg := UDPMessage{
		Timestamp:        1610000000,
		Uptime:           12345,
		Voltage:          3.7,
		RSSI:             -45,
		HubRSSI:          -60,
		SensorStatus:     0x1,
		FirmwareRevision: FlexInt(329),
		ResetFlags:       "",
		Seq:              42,
		SerialNumber:     "ST-TEST",
	}

	// Call device and hub status processors
	l.processDeviceStatus(msg)
	l.processHubStatus(msg)

	dsMap := l.GetDeviceStatus()
	if dsMap == nil {
		t.Fatalf("expected device status map, got nil")
	}
	if m, ok := dsMap.(map[string]interface{}); ok {
		if m["uptime"] != msg.Uptime {
			t.Fatalf("expected uptime %d, got %v", msg.Uptime, m["uptime"])
		}
		if m["rssi"] != msg.RSSI {
			t.Fatalf("expected rssi %d, got %v", msg.RSSI, m["rssi"])
		}
	} else {
		t.Fatalf("device status not a map, got %T", dsMap)
	}

	hsMap := l.GetHubStatus()
	if hsMap == nil {
		t.Fatalf("expected hub status map, got nil")
	}
	if hm, ok := hsMap.(map[string]interface{}); ok {
		// Firmware rev may be returned as string or numeric; accept either representation
		if str, ok := hm["firmware_rev"].(string); ok {
			if str != fmt.Sprintf("%d", int(msg.FirmwareRevision)) {
				t.Fatalf("unexpected firmware rev string: %s", str)
			}
		}
		if hm["serial_number"] != msg.SerialNumber {
			t.Fatalf("expected hub serial %s, got %v", msg.SerialNumber, hm["serial_number"])
		}
	} else {
		t.Fatalf("hub status not a map, got %T", hsMap)
	}

	// Also ensure that processObservationAir accepts a UDPMessage with Obs and doesn't panic
	l2 := &UDPListener{observations: make([]weather.Observation, 0, 10), maxHistorySize: 10}
	airObs := []interface{}{float64(1610000001), 1012.3, 20.5, 50.0, float64(0), 0.0, 3.8, float64(60)}
	msg2 := UDPMessage{Obs: [][]interface{}{airObs}}
	l2.processObservationAir(msg2)
	if len(l2.GetObservations()) == 0 {
		t.Fatalf("expected observation added from AIR message")
	}
}
