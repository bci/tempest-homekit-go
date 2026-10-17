package udp

import (
	"encoding/json"
	"testing"
)

// TestFlexIntParsing checks that firmware_revision decodes from both JSON numbers and numeric strings.
func TestFlexIntParsing(t *testing.T) {
	tests := []struct {
		name     string
		jsonData string
		wantErr  bool
		wantVal  int
	}{
		{
			name:     "firmware_revision as integer",
			jsonData: `{"serial_number":"HB-00000001","type":"hub_status","firmware_revision":35}`,
			wantErr:  false,
			wantVal:  35,
		},
		{
			name:     "firmware_revision as string",
			jsonData: `{"serial_number":"HB-00000001","type":"hub_status","firmware_revision":"35"}`,
			wantErr:  false,
			wantVal:  35,
		},
		{
			name:     "firmware_revision as string with large number",
			jsonData: `{"serial_number":"ST-00000512","type":"obs_st","firmware_revision":"129"}`,
			wantErr:  false,
			wantVal:  129,
		},
		{
			name:     "firmware_revision zero",
			jsonData: `{"serial_number":"AR-00004049","type":"obs_air","firmware_revision":0}`,
			wantErr:  false,
			wantVal:  0,
		},
		{
			name:     "firmware_revision as string zero",
			jsonData: `{"serial_number":"AR-00004049","type":"obs_air","firmware_revision":"0"}`,
			wantErr:  false,
			wantVal:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var msg UDPMessage
			err := json.Unmarshal([]byte(tt.jsonData), &msg)

			if (err != nil) != tt.wantErr {
				t.Errorf("json.Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if int(msg.FirmwareRevision) != tt.wantVal {
					t.Errorf("FirmwareRevision = %d, want %d", msg.FirmwareRevision, tt.wantVal)
				}
			}
		})
	}
}

// TestFlexIntInvalidValues checks that non-numeric and empty firmware_revision strings decode as 0 without error.
func TestFlexIntInvalidValues(t *testing.T) {
	tests := []struct {
		name     string
		jsonData string
	}{
		{
			name:     "firmware_revision as non-numeric string",
			jsonData: `{"serial_number":"HB-00000001","type":"hub_status","firmware_revision":"abc"}`,
		},
		{
			name:     "firmware_revision as empty string",
			jsonData: `{"serial_number":"HB-00000001","type":"hub_status","firmware_revision":""}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var msg UDPMessage
			err := json.Unmarshal([]byte(tt.jsonData), &msg)

			// Should not error, just use 0 for invalid values
			if err != nil {
				t.Errorf("json.Unmarshal() unexpected error = %v", err)
				return
			}

			// Invalid values should default to 0
			if int(msg.FirmwareRevision) != 0 {
				t.Errorf("FirmwareRevision = %d, want 0 for invalid value", msg.FirmwareRevision)
			}
		})
	}
}
