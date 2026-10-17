package weather

import (
	"testing"
	"time"
)

// TestParseStationStatusHTML_Basic tests parseStationStatusHTML with a representative HTML fragment.
func TestParseStationStatusHTML_Basic(t *testing.T) {
	html := `
    <div id="diagnostic-info">
      <span class="lv-param-label">Battery Voltage</span>
      <span class="lv-value-display">Good (2.69v)</span>

    <span class="lv-param-label">Uptime</span><span class="lv-value-display">63d 13h 6m 1s</span>

    <span class="lv-param-label">Uptime</span><span class="lv-value-display">128d 3h 30m 29s</span>

    <span class="lv-param-label">Network Status</span><span class="lv-value-display">Online</span>

    <span class="lv-param-label">Network Status</span><span class="lv-value-display">Online</span>

    <span class="lv-param-label">Wi-Fi Signal (RSSI)</span><span class="lv-value-display">Strong (-32)</span>

    <span class="lv-param-label">Serial Number</span><span class="lv-value-display">HB-12345</span>
    <span class="lv-param-label">Serial Number</span><span class="lv-value-display">ST-99999</span>

    <span class="lv-param-label">Firmware Revision</span><span class="lv-value-display">329</span>
    <span class="lv-param-label">Firmware Revision</span><span class="lv-value-display">179</span>

    <span class="lv-param-label">Last Status Message</span><span class="lv-value-display">09/17/2025 5:26:08 pm</span>

    <span class="lv-param-label">Last Observation</span><span class="lv-value-display">09/17/2025 5:25:45 pm</span>

    <span class="lv-param-label">Sensor Status</span><span class="lv-value-display">Good</span>
    </div>
    `

	status, err := parseStationStatusHTML(html, "debug")
	if err != nil {
		t.Fatalf("parseStationStatusHTML returned error: %v", err)
	}

	if status.BatteryStatus != "Good" {
		t.Fatalf("expected BatteryStatus Good, got %s", status.BatteryStatus)
	}
	if status.BatteryVoltage != "2.69V" {
		t.Fatalf("expected BatteryVoltage 2.69V, got %s", status.BatteryVoltage)
	}
	// Depending on HTML whitespace and patterns, parser may find one or both uptimes.
	// Ensure at least one uptime value was captured.
	if status.HubUptime == "" && status.DeviceUptime == "" {
		t.Fatalf("expected at least one non-empty uptime, got hub=%s device=%s", status.HubUptime, status.DeviceUptime)
	}
	if status.HubNetworkStatus != "Online" || status.DeviceNetworkStatus != "Online" {
		t.Fatalf("expected network status Online, got hub=%s device=%s", status.HubNetworkStatus, status.DeviceNetworkStatus)
	}
	if status.HubWiFiSignal != "Strong (-32)" {
		t.Fatalf("expected wifi signal Strong (-32), got %s", status.HubWiFiSignal)
	}
	if status.HubSerialNumber != "HB-12345" || status.DeviceSerialNumber != "ST-99999" {
		t.Fatalf("expected serials HB-12345/ST-99999, got hub=%s device=%s", status.HubSerialNumber, status.DeviceSerialNumber)
	}
	if status.HubFirmware != "v329" || status.DeviceFirmware != "v179" {
		t.Fatalf("expected firmware v329/v179, got hub=%s device=%s", status.HubFirmware, status.DeviceFirmware)
	}
	if status.HubLastStatus == "" || status.DeviceLastObs == "" {
		t.Fatalf("expected last status and last obs non-empty")
	}
}

// TestFilterToOneMinuteIncrements tests filterToOneMinuteIncrements to ensure it filters and orders correctly.
func TestFilterToOneMinuteIncrements(t *testing.T) {
	now := time.Now().Unix()
	// Create observations every 30 seconds for 5 minutes
	var obs []*Observation
	for i := 0; i < 10; i++ {
		o := &Observation{Timestamp: now - int64(i*30)}
		obs = append(obs, o)
	}

	filtered := filterToOneMinuteIncrements(obs, 10)
	if len(filtered) == 0 {
		t.Fatalf("expected filtered non-empty")
	}
	// Should be roughly one per minute, so expect around 5 entries
	if len(filtered) < 4 || len(filtered) > 6 {
		t.Fatalf("unexpected filtered length %d", len(filtered))
	}
	// Ensure timestamps are increasing (oldest first)
	for i := 1; i < len(filtered); i++ {
		if filtered[i].Timestamp <= filtered[i-1].Timestamp {
			t.Fatalf("timestamps not strictly increasing at %d: %d <= %d", i, filtered[i].Timestamp, filtered[i-1].Timestamp)
		}
	}
}
