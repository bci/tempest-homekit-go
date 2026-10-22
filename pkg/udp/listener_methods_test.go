package udp

import (
	"encoding/json"
	"testing"
	"time"

	"tempest-homekit-go/pkg/weather"
)

// TestProcessDeviceStatusSetsDeviceStatus checks that a device_status datagram populates GetDeviceStatus.
func TestProcessDeviceStatusSetsDeviceStatus(t *testing.T) {
	l := NewUDPListener(100)

	msg := UDPMessage{
		SerialNumber: "ST-DEV",
		Type:         TypeDeviceStatus,
		Timestamp:    time.Now().Unix(),
		Uptime:       1234,
		Voltage:      3.8,
		RSSI:         -42,
		HubRSSI:      -30,
		SensorStatus: 0x1,
	}

	b, _ := json.Marshal(msg)
	l.processMessage(b)

	ds := l.GetDeviceStatus()
	if ds == nil {
		t.Fatalf("expected device status map, got nil")
	}
}

// TestProcessHubStatusSetsHubStatus checks that a hub_status datagram populates GetHubStatus.
func TestProcessHubStatusSetsHubStatus(t *testing.T) {
	l := NewUDPListener(100)

	msg := UDPMessage{
		SerialNumber:     "HB-1",
		Type:             TypeHubStatus,
		Timestamp:        time.Now().Unix(),
		Uptime:           777,
		RSSI:             -50,
		ResetFlags:       "none",
		Seq:              42,
		FirmwareRevision: 179,
	}

	b, _ := json.Marshal(msg)
	l.processMessage(b)

	hs := l.GetHubStatus()
	if hs == nil {
		t.Fatalf("expected hub status map, got nil")
	}
}

// TestAddObservationRollOverAndGetLatestCopy checks history rollover at maxHistorySize and that GetLatestObservation returns a copy.
func TestAddObservationRollOverAndGetLatestCopy(t *testing.T) {
	l := NewUDPListener(100)
	// set a small maxHistorySize to test rollover
	l.maxHistorySize = 2

	// add three observations
	o1 := createObs(1)
	o2 := createObs(2)
	o3 := createObs(3)
	l.addObservation(*o1)
	l.addObservation(*o2)
	l.addObservation(*o3)

	obs := l.GetObservations()
	if len(obs) != 2 {
		t.Fatalf("expected 2 observations after rollover, got %d", len(obs))
	}

	latest := l.GetLatestObservation()
	if latest == nil {
		t.Fatalf("expected latest observation, got nil")
	}
	// modify returned latest and ensure internal latest is unaffected
	latest.AirTemperature = 999.0
	latest2 := l.GetLatestObservation()
	if latest2.AirTemperature == 999.0 {
		t.Fatalf("expected internal latest to be unchanged by external modification")
	}
}

// createObs returns an observation whose timestamp and air temperature are both ts.
func createObs(ts int64) *weather.Observation {
	return &weather.Observation{Timestamp: ts, AirTemperature: float64(ts)}
}
