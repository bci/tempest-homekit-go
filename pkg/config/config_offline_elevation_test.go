package config

import (
	"flag"
	"os"
	"testing"
)

// TestLoadConfig_DisableInternetSkipsElevationLookup checks that --disable-internet
// (with --udp-stream as its required local source) never runs the elevation
// auto-lookup, which calls api.open-elevation.com. The lookup used to run in offline
// mode whenever --elevation was not given.
func TestLoadConfig_DisableInternetSkipsElevationLookup(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantLookup bool
	}{
		{"offline UDP", []string{"cmd", "--disable-internet", "--udp-stream", "--station", "Chino Hills"}, false},
		{"online UDP", []string{"cmd", "--udp-stream", "--station", "Chino Hills"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			oldArgs, oldLookup, oldFlags := os.Args, stationElevationLookup, flag.CommandLine
			defer func() { os.Args, stationElevationLookup, flag.CommandLine = oldArgs, oldLookup, oldFlags }()

			called := false
			stationElevationLookup = func(token, stationName string) (float64, error) {
				called = true
				return 100, nil
			}
			t.Setenv("HOMEKIT_PIN", "12345678")
			os.Args = tc.args
			flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

			_ = LoadConfig()
			if called != tc.wantLookup {
				t.Fatalf("elevation lookup called=%v, want %v", called, tc.wantLookup)
			}
		})
	}
}
