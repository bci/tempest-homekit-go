//go:build !darwin
// +build !darwin

package alarm

import (
	"fmt"

	"tempest-homekit-go/pkg/weather"
)

// OSLogNotifier is a placeholder for non-macOS platforms, so the "oslog"
// channel type still compiles everywhere.
type OSLogNotifier struct{}

// Send always returns an error, because oslog notification is only supported
// on macOS.
func (n *OSLogNotifier) Send(alarm *Alarm, channel *Channel, obs *weather.Observation, stationName string) error {
	return fmt.Errorf("oslog notification type is only supported on macOS")
}
