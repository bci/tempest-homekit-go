//go:build !windows && !plan9

package alarm

import (
	"net"
	"strings"
	"testing"
	"time"

	"tempest-homekit-go/pkg/weather"
)

// TestSyslogNotifier_UsesConfiguredPriority sends one message per configured priority
// to a local UDP "syslog server" and checks the <PRI> header of what arrives. The
// notifier used to write every message with writer.Warning, so "error" and "info"
// both arrived as warning (<4>). Facility is 0 (kern) because the notifier passes a
// bare severity to syslog.Dial.
func TestSyslogNotifier_UsesConfiguredPriority(t *testing.T) {
	cases := []struct {
		priority string
		wantPRI  string
	}{
		{"error", "<3>"},
		{"warning", "<4>"},
		{"info", "<6>"},
		{"", "<4>"}, // default
	}
	for _, tc := range cases {
		t.Run("priority="+tc.priority, func(t *testing.T) {
			pc, err := net.ListenPacket("udp", "127.0.0.1:0")
			if err != nil {
				t.Fatalf("listen: %v", err)
			}
			defer func() { _ = pc.Close() }()

			n := &SyslogNotifier{config: &SyslogConfig{
				Network:  "udp",
				Address:  pc.LocalAddr().String(),
				Priority: tc.priority,
				Tag:      "tempest-test",
			}}
			alarm := &Alarm{Name: "test"}
			channel := &Channel{Type: "syslog", Template: "hello"}
			if err := n.Send(alarm, channel, &weather.Observation{}, "station"); err != nil {
				t.Fatalf("Send: %v", err)
			}

			buf := make([]byte, 2048)
			_ = pc.SetReadDeadline(time.Now().Add(2 * time.Second))
			nr, _, err := pc.ReadFrom(buf)
			if err != nil {
				t.Fatalf("no syslog packet received: %v", err)
			}
			got := string(buf[:nr])
			if !strings.HasPrefix(got, tc.wantPRI) {
				t.Fatalf("expected packet to start with %s, got %q", tc.wantPRI, got)
			}
		})
	}
}
