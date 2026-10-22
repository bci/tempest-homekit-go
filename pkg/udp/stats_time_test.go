package udp

import (
	"testing"
	"time"

	"tempest-homekit-go/pkg/weather"
)

// TestIsReceivingDataAndGetStats checks GetStats values and that IsReceivingData is true only for a packet within the last 5 minutes.
func TestIsReceivingDataAndGetStats(t *testing.T) {
	l := &UDPListener{observations: make([]weather.Observation, 0, 10)}

	// Initially, no data
	if l.IsReceivingData() {
		t.Fatalf("expected IsReceivingData false when no packets")
	}

	// Update stats to recent time and packet count
	l.mu.Lock()
	l.packetCount = 7
	l.lastPacketTime = time.Now().Add(-1 * time.Minute)
	l.stationIP = "192.0.2.1"
	l.serialNumber = "ST-123"
	l.mu.Unlock()

	if !l.IsReceivingData() {
		t.Fatalf("expected IsReceivingData true for recent packet")
	}

	pc, last, ip, sn := l.GetStats()
	if pc != 7 {
		t.Fatalf("expected packet count 7, got %d", pc)
	}
	if ip != "192.0.2.1" {
		t.Fatalf("expected station ip, got %s", ip)
	}
	if sn != "ST-123" {
		t.Fatalf("expected serial number, got %s", sn)
	}
	if last.IsZero() {
		t.Fatalf("expected last packet time set")
	}

	// Set lastPacketTime to far past
	l.mu.Lock()
	l.lastPacketTime = time.Now().Add(-10 * time.Minute)
	l.mu.Unlock()

	if l.IsReceivingData() {
		t.Fatalf("expected IsReceivingData false for old packet")
	}
}
