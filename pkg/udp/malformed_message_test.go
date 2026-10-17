package udp

import "testing"

// TestProcessMessage_MalformedPacketsDoNotPanic checks that packets whose fields have
// the wrong JSON type (null where a number is expected) or are missing are dropped
// instead of panicking. processMessage runs on the listener goroutine, so a panic there
// used to take down the whole process on a single bad broadcast.
func TestProcessMessage_MalformedPacketsDoNotPanic(t *testing.T) {
	packets := map[string]string{
		"obs_st with null temperature": `{"serial_number":"ST-1","type":"obs_st","obs":[[1700000000,0,1,2,90,3,1013,null,50,100,1,10,0,0,0,0,2.6,1]]}`,
		"obs_air with null pressure":   `{"serial_number":"AR-1","type":"obs_air","obs":[[1700000000,null,20,50,0,0,3.4,1]]}`,
		"rapid_wind with null speed":   `{"serial_number":"ST-1","type":"rapid_wind","ob":[1700000000,null,90]}`,
		"evt_precip with empty evt":    `{"serial_number":"ST-1","type":"evt_precip","evt":[]}`,
		"evt_strike with null energy":  `{"serial_number":"ST-1","type":"evt_strike","evt":[1700000000,10,null]}`,
	}
	for name, pkt := range packets {
		t.Run(name, func(t *testing.T) {
			l := NewUDPListener(10)
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("processMessage panicked: %v", r)
				}
			}()
			l.processMessage([]byte(pkt))
			if n := len(l.GetObservations()); n != 0 {
				t.Fatalf("expected malformed packet to be dropped, got %d observations", n)
			}
		})
	}
}
