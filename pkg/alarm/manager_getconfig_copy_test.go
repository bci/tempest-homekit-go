package alarm

import (
	"sync"
	"testing"

	"tempest-homekit-go/pkg/weather"
)

// newCopyTestManager builds a Manager with one always-true, channel-less alarm, so
// ProcessObservation mutates the alarm's runtime state without sending anything.
func newCopyTestManager() *Manager {
	config := &AlarmConfig{
		Alarms: []Alarm{{Name: "always", Condition: "temperature > -100", Enabled: true}},
	}
	return &Manager{
		config:          config,
		evaluator:       NewEvaluator(),
		notifierFactory: NewNotifierFactory(config),
	}
}

// TestManager_GetConfig_AlarmsAreCopied checks that GetConfig returns its own copy of
// the Alarms slice, so a caller changing it does not change the live alarms. Before
// the 2026-10-16 fix the slice was shared with the manager.
func TestManager_GetConfig_AlarmsAreCopied(t *testing.T) {
	m := newCopyTestManager()

	cfg := m.GetConfig()
	cfg.Alarms[0].TriggeredCount = 99
	cfg.Alarms[0].Enabled = false

	live := m.config.Alarms[0]
	if live.TriggeredCount != 0 || !live.Enabled {
		t.Fatalf("mutating GetConfig() result changed the live alarm: %+v", live)
	}
}

// TestManager_GetConfig_ConcurrentWithProcessObservation reads the alarm runtime state
// through GetConfig (as the web alarm-status endpoint does) while observations are
// processed. Run with -race: the shared slice used to be a data race.
func TestManager_GetConfig_ConcurrentWithProcessObservation(t *testing.T) {
	m := newCopyTestManager()
	obs := &weather.Observation{AirTemperature: 20}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			m.ProcessObservation(obs)
		}
	}()
	for i := 0; i < 200; i++ {
		for _, a := range m.GetConfig().Alarms {
			_ = a.TriggeredCount
			_ = a.GetLastFired()
		}
	}
	wg.Wait()

	if got := m.GetConfig().Alarms[0].TriggeredCount; got != 200 {
		t.Fatalf("expected 200 triggers, got %d", got)
	}
}
