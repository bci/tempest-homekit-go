package udp

import (
	"encoding/json"
	"testing"
	"time"

	"tempest-homekit-go/pkg/weather"
)

// createObsST builds a minimal obs_st payload (18 fields) with predictable values and timestamp ts.
func createObsST(ts int64) [][]interface{} {
	obs := make([]interface{}, 18)
	obs[0] = float64(ts)
	obs[1] = 0.0
	obs[2] = 1.0
	obs[3] = 2.0
	obs[4] = 90.0
	obs[5] = 0.0
	obs[6] = 1012.3
	obs[7] = 20.5
	obs[8] = 50.0
	obs[9] = 100.0
	obs[10] = 3.0
	obs[11] = 0.0
	obs[12] = 0.1
	obs[13] = 0.0
	obs[14] = 5.0
	obs[15] = 1.0
	obs[16] = 3.7
	obs[17] = 60.0
	return [][]interface{}{obs}
}

// TestProcessObservationST_AddsObservationAndPrettyPrints checks that an obs_st message pretty-prints and is added to the history.
func TestProcessObservationST_AddsObservationAndPrettyPrints(t *testing.T) {
	l := &UDPListener{observations: make([]weather.Observation, 0, 10), maxHistorySize: 10}

	// Build UDPMessage for obs_st type
	msg := UDPMessage{Type: TypeObservationST, SerialNumber: "ST-TEST", Obs: createObsST(time.Now().Unix())}

	// Marshal to JSON and exercise PrettyPrintMessage
	data, _ := json.Marshal(msg)
	s := PrettyPrintMessage(data)
	if s == "" {
		t.Fatalf("PrettyPrintMessage returned empty string for obs_st")
	}

	// Call processObservationST via processMessage (which unmarshals JSON)
	l.processMessage(data)

	if len(l.GetObservations()) == 0 {
		t.Fatalf("expected observation added after processing obs_st")
	}
}

// TestPrettyPrintMessage_OtherTypes checks that every other message type pretty-prints to a non-empty line.
func TestPrettyPrintMessage_OtherTypes(t *testing.T) {
	now := float64(time.Now().Unix())

	cases := []UDPMessage{
		{Type: TypeObservationAir, SerialNumber: "AIR-1", Obs: [][]interface{}{{now, 1012.0, 22.0, 45.0, 0.0, 0.0, 3.8, 60.0}}},
		{Type: TypeObservationSky, SerialNumber: "SKY-1", Obs: [][]interface{}{{now, 200.0, 3.0, 0.0, 0.0, 3.0, 4.0, 180.0, 3.7, 60.0, 0.0, nil, 0.0, 60.0}}},
		{Type: TypeRapidWind, SerialNumber: "HB-1", Ob: []interface{}{now, 5.5, 120.0}},
		{Type: TypeRainStart, SerialNumber: "HB-1", Evt: []interface{}{now}},
		{Type: TypeLightning, SerialNumber: "HB-1", Evt: []interface{}{now, 2.3, 100.0}},
		{Type: TypeDeviceStatus, SerialNumber: "ST-1", Timestamp: int64(now), Uptime: 1234, Voltage: 3.7, RSSI: -50, HubRSSI: -60, SensorStatus: 0x1},
		{Type: TypeHubStatus, SerialNumber: "HB-1", Timestamp: int64(now), Uptime: 2345, RSSI: -55, FirmwareRevision: FlexInt(329), ResetFlags: "", Seq: 7},
	}

	for _, c := range cases {
		data, _ := json.Marshal(c)
		s := PrettyPrintMessage(data)
		if s == "" {
			t.Fatalf("PrettyPrintMessage returned empty for type %s", c.Type)
		}
	}
}
