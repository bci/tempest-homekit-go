package udp

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// Clean, minimal tests for pkg/udp.

// sampleObsSTJSON returns a raw obs_st datagram with serial SN123 and a full 18-value observation row.
func sampleObsSTJSON() []byte {
	j := `{"serial_number":"SN123","type":"obs_st","obs":[[1600000000,0,1.2,2.3,180,3,1012.5,20.5,50,1000,2,0,0.5,0,5.0,1,3.7,60]]}`
	return []byte(j)
}

// TestFlexInt_UnmarshalInt checks that FlexInt decodes a JSON number.
func TestFlexInt_UnmarshalInt(t *testing.T) {
	var fi FlexInt
	if err := json.Unmarshal([]byte("35"), &fi); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if int(fi) != 35 {
		t.Fatalf("expected 35, got %d", int(fi))
	}
}

// TestFlexInt_UnmarshalStringNumber checks that FlexInt decodes a numeric JSON string.
func TestFlexInt_UnmarshalStringNumber(t *testing.T) {
	var fi FlexInt
	if err := json.Unmarshal([]byte("\"42\""), &fi); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if int(fi) != 42 {
		t.Fatalf("expected 42, got %d", int(fi))
	}
}

// TestPrettyPrintMessage_obs_st checks that the pretty-printed obs_st line includes the serial number.
func TestPrettyPrintMessage_obs_st(t *testing.T) {
	out := PrettyPrintMessage(sampleObsSTJSON())
	if out == "" {
		t.Fatal("expected non-empty pretty print output")
	}
	if !strings.Contains(out, "SN123") {
		t.Fatalf("expected serial in pretty output: %s", out)
	}
}

// TestNewUDPListenerAndProcessObservationST checks that an obs_st datagram produces a latest observation.
func TestNewUDPListenerAndProcessObservationST(t *testing.T) {
	l := NewUDPListener(50)
	if l == nil {
		t.Fatal("NewUDPListener returned nil")
	}

	obs := make([]interface{}, 18)
	// Fill all expected indices with numeric values (as float64) to match
	// what processObservationST expects after JSON unmarshalling.
	obs[0] = float64(time.Now().Unix()) // timestamp
	obs[1] = float64(0.1)               // wind_lull
	obs[2] = float64(1.2)               // wind_avg
	obs[3] = float64(2.3)               // wind_gust
	obs[4] = float64(180.0)             // wind_dir
	obs[5] = float64(3.0)               // wind_sample_interval
	obs[6] = float64(1013.25)           // pressure
	obs[7] = float64(20.0)              // temp
	obs[8] = float64(50.0)              // humidity
	obs[9] = float64(100.0)             // lux
	obs[10] = float64(2.0)              // uv
	obs[11] = float64(0.0)              // solar_rad
	obs[12] = float64(0.5)              // rain_1min
	obs[13] = float64(0.0)              // precip_type
	obs[14] = float64(5.0)              // lightning_dist
	obs[15] = float64(1.0)              // lightning_count
	obs[16] = float64(3.7)              // battery
	obs[17] = float64(60.0)             // report_interval
	msg := UDPMessage{SerialNumber: "SN-TEST", Type: TypeObservationST, Obs: [][]interface{}{obs}}
	data, _ := json.Marshal(msg)
	l.processMessage(data)
	if l.GetLatestObservation() == nil {
		t.Fatal("expected latest observation after processing message")
	}
}

// TestUDPListener_DeviceAndHubStatus checks that raw device_status and hub_status JSON populate the status maps.
func TestUDPListener_DeviceAndHubStatus(t *testing.T) {
	l := NewUDPListener(10)

	// craft device status JSON
	j := `{"serial_number":"DSN","type":"device_status","timestamp":1600000003,"uptime":1234,"voltage":3.9,"rssi":-50,"hub_rssi":-48,"sensor_status":1}`
	l.processMessage([]byte(j))
	ds := l.GetDeviceStatus()
	if ds == nil {
		t.Fatal("expected device status map")
	}

	// craft hub status
	j2 := `{"serial_number":"HSN","type":"hub_status","timestamp":1600000004,"uptime":4321,"rssi":-40,"firmware_revision":35,"reset_flags":"none","seq":7}`
	l.processMessage([]byte(j2))
	hs := l.GetHubStatus()
	if hs == nil {
		t.Fatal("expected hub status map")
	}
}
