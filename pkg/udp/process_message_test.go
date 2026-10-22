package udp

import (
	"encoding/json"
	"testing"
	"time"
)

// TestProcessObservationAirAddsObservation checks that one obs_air datagram adds one observation with the sent temperature.
func TestProcessObservationAirAddsObservation(t *testing.T) {
	l := NewUDPListener(10)

	// craft obs_air message
	msg := UDPMessage{
		SerialNumber: "ST-0001",
		Type:         TypeObservationAir,
		Obs: [][]interface{}{
			{
				float64(time.Now().Unix()), // 0 timestamp
				1012.5,                     // 1 pressure
				21.0,                       // 2 temp
				55.0,                       // 3 humidity
				0.0,                        // 4 lightning_count
				0.0,                        // 5 lightning_dist
				3.8,                        // 6 battery
				60.0,                       // 7 report_interval
			},
		},
	}

	b, _ := json.Marshal(msg)
	l.processMessage(b)

	obs := l.GetObservations()
	if len(obs) != 1 {
		t.Fatalf("expected 1 observation, got %d", len(obs))
	}
	if obs[0].AirTemperature != 21.0 {
		t.Fatalf("unexpected temp: %v", obs[0].AirTemperature)
	}
}

// TestProcessObservationSkyAddsObservation checks that an obs_sky row with a null local_rain field still adds an observation with the sent lux and UV.
func TestProcessObservationSkyAddsObservation(t *testing.T) {
	l := NewUDPListener(10)

	msg := UDPMessage{
		SerialNumber: "SK-0001",
		Type:         TypeObservationSky,
		Obs: [][]interface{}{
			{
				float64(time.Now().Unix()), // 0 timestamp
				500.0,                      // 1 lux
				4.0,                        // 2 uv
				0.0,                        // 3 rain
				0.1,                        // 4 wind_lull
				1.5,                        // 5 wind_avg
				2.0,                        // 6 wind_gust
				270.0,                      // 7 wind_dir
				3.9,                        // 8 battery
				60.0,                       // 9 report_interval
				200.0,                      // 10 solar_rad
				nil,                        // 11 local_rain
				0.0,                        // 12 precip_type
				3.0,                        // 13 wind_sample_interval
			},
		},
	}

	b, _ := json.Marshal(msg)
	l.processMessage(b)

	obs := l.GetObservations()
	if len(obs) != 1 {
		t.Fatalf("expected 1 observation, got %d", len(obs))
	}
	if obs[0].Illuminance != 500.0 {
		t.Fatalf("unexpected lux: %v", obs[0].Illuminance)
	}
	if obs[0].UV != 4 {
		t.Fatalf("unexpected uv: %d", obs[0].UV)
	}
}
