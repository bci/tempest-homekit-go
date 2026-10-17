//go:build darwin
// +build darwin

package alarm

/*
#cgo LDFLAGS: -framework Foundation
#include <os/log.h>
#include <stdlib.h>

void log_message(const char *subsystem, const char *category, const char *message) {
    os_log_t log = os_log_create(subsystem, category);
    os_log_with_type(log, OS_LOG_TYPE_DEFAULT, "%{public}s", message);
}
*/
import "C"
import (
	"unsafe"

	"tempest-homekit-go/pkg/weather"
)

// OSLogNotifier sends notifications to macOS unified logging system (this
// darwin build uses cgo and os_log).
type OSLogNotifier struct{}

// Send expands channel.Template and logs it via os_log with subsystem
// "com.bci.tempest-homekit", category "alarm", default level and the message
// marked public so it is not redacted. It never returns an error.
func (n *OSLogNotifier) Send(alarm *Alarm, channel *Channel, obs *weather.Observation, stationName string) error {
	message := expandTemplate(channel.Template, alarm, obs, stationName)

	// Use tempest-homekit as subsystem and alarm as category
	subsystem := C.CString("com.bci.tempest-homekit")
	category := C.CString("alarm")
	cMessage := C.CString(message)

	defer C.free(unsafe.Pointer(subsystem))
	defer C.free(unsafe.Pointer(category))
	defer C.free(unsafe.Pointer(cMessage))

	C.log_message(subsystem, category, cMessage)
	return nil
}
