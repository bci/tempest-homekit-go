package udp

import (
	"encoding/json"
	"testing"
	"time"

	"tempest-homekit-go/pkg/weather"
)

// TestProcessObservationAir checks that an obs_air datagram yields an observation with the sent temperature and pressure.
func TestProcessObservationAir(t *testing.T) {
	l := NewUDPListener(50)

	obs := make([]interface{}, 8)
	obs[0] = float64(time.Now().Unix())
	obs[1] = 1012.5 // pressure
	obs[2] = 19.0   // temp
	obs[3] = 55.0   // humidity
	obs[4] = 0.0    // lightning_count
	obs[5] = 0.0    // lightning_dist
	obs[6] = 3.6    // battery
	obs[7] = 60.0   // report interval

	msg := UDPMessage{
		SerialNumber: "SN-AIR",
		Type:         TypeObservationAir,
		Obs:          [][]interface{}{obs},
	}
	data, _ := json.Marshal(msg)

	l.processMessage(data)

	latest := l.GetLatestObservation()
	if latest == nil {
		t.Fatal("expected latest observation after AIR message")
	}
	if latest.AirTemperature != 19.0 {
		t.Fatalf("unexpected temperature from AIR: %v", latest.AirTemperature)
	}
	if latest.StationPressure != 1012.5 {
		t.Fatalf("unexpected pressure from AIR: %v", latest.StationPressure)
	}
}

// TestProcessObservationSky checks that an obs_sky datagram yields an observation with the sent illuminance and UV.
func TestProcessObservationSky(t *testing.T) {
	l := NewUDPListener(50)

	obs := make([]interface{}, 14)
	obs[0] = float64(time.Now().Unix())
	obs[1] = 200.0  // lux
	obs[2] = 5.0    // uv
	obs[3] = 0.1    // rain
	obs[4] = 0.1    // wind_lull
	obs[5] = 3.0    // wind_avg
	obs[6] = 5.0    // wind_gust
	obs[7] = 270.0  // wind_dir
	obs[8] = 3.7    // battery
	obs[9] = 60.0   // report interval
	obs[10] = 120.0 // solar
	obs[12] = 0.0   // precip_type
	obs[13] = 1.0   // wind_sample_interval

	msg := UDPMessage{
		SerialNumber: "SN-SKY",
		Type:         TypeObservationSky,
		Obs:          [][]interface{}{obs},
	}
	data, _ := json.Marshal(msg)

	l.processMessage(data)

	latest := l.GetLatestObservation()
	if latest == nil {
		t.Fatal("expected latest observation after SKY message")
	}
	if latest.Illuminance != 200.0 {
		t.Fatalf("unexpected illuminance from SKY: %v", latest.Illuminance)
	}
	if latest.UV != 5 {
		t.Fatalf("unexpected UV from SKY: %v", latest.UV)
	}
}

// TestProcessDeviceAndHubStatus checks device and hub status maps, serial number capture and a directly added observation.
func TestProcessDeviceAndHubStatus(t *testing.T) {
	l := NewUDPListener(50)

	// device status
	dev := UDPMessage{
		SerialNumber: "SN-DEV",
		Type:         TypeDeviceStatus,
		Timestamp:    time.Now().Unix(),
		Uptime:       3600,
		Voltage:      3.8,
		RSSI:         -40,
		HubRSSI:      -30,
		SensorStatus: 1,
	}
	b1, _ := json.Marshal(dev)
	l.processMessage(b1)

	ds := l.GetDeviceStatus()
	if ds == nil {
		t.Fatal("expected device status to be set")
	}
	dsMap, ok := ds.(map[string]interface{})
	if !ok {
		t.Fatal("expected device status to be a map")
	}
	voltage, _ := dsMap["voltage"].(float64)
	if voltage != 3.8 {
		t.Fatalf("unexpected device voltage: %v", voltage)
	}

	// hub status
	hub := UDPMessage{
		SerialNumber:     "HUB-1",
		Type:             TypeHubStatus,
		Timestamp:        time.Now().Unix(),
		FirmwareRevision: 123,
		Uptime:           7200,
		RSSI:             -20,
		ResetFlags:       "",
		Seq:              42,
	}
	b2, _ := json.Marshal(hub)
	l.processMessage(b2)

	hs := l.GetHubStatus()
	if hs == nil {
		t.Fatal("expected hub status to be set")
	}
	hsMap, ok := hs.(map[string]interface{})
	if !ok {
		t.Fatal("expected hub status to be a map")
	}
	// Note: Seq is not included in the map returned by GetHubStatus
	// Just verify we got a valid map with expected fields
	if _, ok := hsMap["firmware_rev"]; !ok {
		t.Fatal("expected firmware_rev in hub status map")
	}

	// Ensure serial number captured
	if l.serialNumber == "" {
		t.Fatal("expected serial number to be set")
	}

	// Add observation via addObservation and validate
	l.addObservation(weather.Observation{AirTemperature: 25.0, Timestamp: time.Now().Unix()})
	if l.GetLatestObservation().AirTemperature != 25.0 {
		t.Fatalf("expected latest temp 25.0, got %v", l.GetLatestObservation().AirTemperature)
	}
}
