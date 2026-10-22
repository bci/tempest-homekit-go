package udp

import (
	"encoding/json"
	"testing"
	"time"
)

// TestFlexInt_UnmarshalInt_Udp checks that FlexInt decodes a JSON number.
func TestFlexInt_UnmarshalInt_Udp(t *testing.T) {
	var fi FlexInt
	data := []byte("35")
	if err := json.Unmarshal(data, &fi); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if int(fi) != 35 {
		t.Fatalf("expected 35, got %d", fi)
	}
}

// TestFlexInt_UnmarshalString_Udp checks that FlexInt decodes a numeric JSON string.
func TestFlexInt_UnmarshalString_Udp(t *testing.T) {
	var fi FlexInt
	data := []byte("\"42\"")
	if err := json.Unmarshal(data, &fi); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if int(fi) != 42 {
		t.Fatalf("expected 42, got %d", fi)
	}
}

// TestPrettyPrintMessage_ObsST checks that a complete obs_st message pretty-prints to a non-empty line.
func TestPrettyPrintMessage_ObsST(t *testing.T) {
	// Construct minimal obs_st payload with expected indices
	payload := UDPMessage{
		SerialNumber: "ST-0001",
		Type:         TypeObservationST,
		Obs: [][]interface{}{
			{
				float64(time.Now().Unix()), // 0
				0.0,                        // 1 wind_lull
				1.2,                        // 2 wind_avg
				2.3,                        // 3 wind_gust
				180.0,                      // 4 wind_dir
				3.0,                        // 5 sample_interval
				1012.3,                     // 6 pressure
				20.5,                       // 7 temp
				50.0,                       // 8 humidity
				100.0,                      // 9 lux
				5.0,                        // 10 uv
				0.0,                        // 11 solar
				0.0,                        // 12 rain
				0.0,                        // 13 precip_type
				10.0,                       // 14 lightning_dist
				0.0,                        // 15 lightning_count
				3.7,                        // 16 battery
				60.0,                       // 17 report_interval
			},
		},
	}

	b, _ := json.Marshal(payload)
	s := PrettyPrintMessage(b)
	if s == "" {
		t.Fatalf("PrettyPrintMessage returned empty string")
	}
}
