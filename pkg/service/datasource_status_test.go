package service

import (
	"testing"
	"time"

	"tempest-homekit-go/pkg/config"
	"tempest-homekit-go/pkg/weather"
)

// TestCreateDataSource_UDPStatusReflectsListener checks that a UDP data source status reports the listener packet count, serial, IP and observation count.
func TestCreateDataSource_UDPStatusReflectsListener(t *testing.T) {
	cfg := &config.Config{UDPStream: true, DisableInternet: true}
	station := mockStation()

	// Use the provided mockUDPListener and set stats
	m := newMockUDPListener()
	m.packetCount = 42
	m.lastPacket = time.Unix(1610000000, 0)
	m.stationIP = "10.0.0.5"
	m.serialNumber = "ST-FAKE"
	// populate observations
	m.observations = []weather.Observation{{Timestamp: 1}, {Timestamp: 2}, {Timestamp: 3}}

	ds, err := CreateDataSource(cfg, station, m, nil)
	if err != nil {
		t.Fatalf("unexpected error creating UDP data source: %v", err)
	}

	status := ds.GetStatus()
	if status.PacketCount != 42 {
		t.Fatalf("expected packet count 42, got %d", status.PacketCount)
	}
	if status.SerialNumber != "ST-FAKE" {
		t.Fatalf("expected serial ST-FAKE, got %s", status.SerialNumber)
	}
	if status.StationIP != "10.0.0.5" {
		t.Fatalf("expected station IP 10.0.0.5, got %s", status.StationIP)
	}
	if status.ObservationCount != int64(len(m.observations)) {
		t.Fatalf("expected observation count %d, got %d", len(m.observations), status.ObservationCount)
	}
}
