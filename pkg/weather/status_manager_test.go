package weather

import (
	"testing"
	"time"
)

// TestStatusManager_FallbackAndUpdateBattery checks the initial placeholder status and that an observation battery reading replaces it.
func TestStatusManager_FallbackAndUpdateBattery(t *testing.T) {
	sm := NewStatusManager(123, "debug", false)

	// initial status should be fallback
	status := sm.GetStatus()
	if status == nil {
		t.Fatalf("Expected non-nil status")
	}

	// Ensure fallback values are present
	if status.BatteryVoltage != "--" {
		t.Fatalf("Expected fallback battery voltage, got %s", status.BatteryVoltage)
	}

	// Update battery via observation
	obs := &Observation{Battery: 3.7}
	sm.UpdateBatteryFromObservation(obs)

	updated := sm.GetStatus()
	if updated.BatteryVoltage == "--" {
		t.Fatalf("Expected battery voltage to be updated from observation")
	}
}

// fakeListener implements UDPListener for testing UDPDataSource without network.
// Observations pushed into ch are delivered through ObservationChannel; obs backs
// GetLatestObservation, GetObservations and the count in GetStats (which also
// reports a fixed IP and serial). Stop closes ch, so it may be called only once;
// the other methods are no-ops.
type fakeListener struct {
	ch  chan Observation
	obs []Observation
}

// Start is a no-op that returns nil.
func (f *fakeListener) Start() error { return nil }
func (f *fakeListener) Stop() error  { close(f.ch); return nil }
func (f *fakeListener) GetLatestObservation() *Observation {
	if len(f.obs) == 0 {
		return nil
	}
	return &f.obs[len(f.obs)-1]
}
func (f *fakeListener) GetStats() (int64, time.Time, string, string) {
	return int64(len(f.obs)), time.Now(), "127.0.0.1", "ST-FAKE"
}
func (f *fakeListener) GetObservations() []Observation         { return f.obs }
func (f *fakeListener) IsReceivingData() bool                  { return true }
func (f *fakeListener) ObservationChannel() <-chan Observation { return f.ch }
func (f *fakeListener) GetDeviceStatus() interface{}           { return nil }
func (f *fakeListener) GetHubStatus() interface{}              { return nil }

// TestUDPDataSource_Forwarding checks that an observation from the listener is forwarded to the data source channel and that Stop succeeds.
func TestUDPDataSource_Forwarding(t *testing.T) {
	f := &fakeListener{ch: make(chan Observation, 10), obs: []Observation{}}
	ds := NewUDPDataSource(f, true, 0, "")

	// Start should call listener.Start (no-op) and return channel
	ch, err := ds.Start()
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	// push an observation into fake listener channel
	fObs := Observation{Timestamp: time.Now().Unix(), AirTemperature: 15.5}
	f.obs = append(f.obs, fObs)
	f.ch <- fObs

	select {
	case got := <-ch:
		if got.AirTemperature != fObs.AirTemperature {
			t.Fatalf("expected forwarded observation, got %+v", got)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timed out waiting for forwarded observation")
	}

	// Stop should close channels without panic
	if err := ds.Stop(); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
}
