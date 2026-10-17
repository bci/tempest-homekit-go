// Package udp provides UDP broadcast listener for local Tempest weather station data.
// This enables offline weather monitoring when internet connectivity is unavailable.
//
// A Tempest hub broadcasts JSON datagrams on UDP port 50222 to the local network.
// UDPListener receives them, converts observation messages (obs_st, obs_air,
// obs_sky) into weather.Observation values kept in a bounded history, records the
// latest device_status and hub_status messages, and only logs rapid_wind,
// evt_precip and evt_strike events.
package udp

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"tempest-homekit-go/pkg/logger"
	"tempest-homekit-go/pkg/weather"
)

const (
	// UDPPort is the port that Tempest hubs broadcast on.
	UDPPort = 50222
)

// MessageType represents the type of UDP broadcast message (the JSON "type" field).
type MessageType string

// Message types sent by a Tempest hub, as they appear in UDPMessage.Type:
// observations from a Tempest (obs_st), the legacy AIR and SKY devices
// (obs_air, obs_sky), the 3-second rapid wind update, rain-start and
// lightning-strike events, and the periodic device_status and hub_status
// health reports.
const (
	TypeObservationST  MessageType = "obs_st"     // Tempest observation
	TypeObservationAir MessageType = "obs_air"    // AIR device observation
	TypeObservationSky MessageType = "obs_sky"    // SKY device observation
	TypeRapidWind      MessageType = "rapid_wind" // Rapid wind update
	TypeRainStart      MessageType = "evt_precip" // Rain start event
	TypeLightning      MessageType = "evt_strike" // Lightning strike event
	TypeDeviceStatus   MessageType = "device_status"
	TypeHubStatus      MessageType = "hub_status"
)

// FlexInt is a type that can unmarshal from either int or string.
// This is needed because Tempest UDP broadcasts sometimes send firmware_revision
// as an integer (e.g., 35) and sometimes as a string (e.g., "35").
type FlexInt int

// UnmarshalJSON implements json.Unmarshaler for FlexInt.
// It handles both integer and string representations of numbers. A string that
// does not start with an integer decodes as 0 rather than failing; any other
// JSON type returns an error.
func (fi *FlexInt) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as int first
	var i int
	if err := json.Unmarshal(data, &i); err == nil {
		*fi = FlexInt(i)
		return nil
	}

	// Try to unmarshal as string
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		// Try to parse string as int
		var parsed int
		if _, err := fmt.Sscanf(s, "%d", &parsed); err == nil {
			*fi = FlexInt(parsed)
			return nil
		}
		// If it's not a valid number, just use 0
		*fi = FlexInt(0)
		return nil
	}

	return fmt.Errorf("firmware_revision must be int or string")
}

// UDPMessage represents the generic structure of all UDP broadcast messages. It
// is the union of every message type's fields; which ones are populated depends
// on Type.
//
// Fields:
//
//   - SerialNumber is the sending device's serial (ST-..., AR-..., SK-... or the
//     hub's HB-...).
//   - Type is the message type, one of the Type* constants.
//   - HubSN is the serial of the hub that relayed the message.
//   - FirmwareRevision is the device or hub firmware revision, sent as either a
//     number or a string.
//   - Obs holds observation rows for obs_st, obs_air and obs_sky; each row is a
//     positional array of numbers (or null), see the process* methods for the
//     layouts. Only the first row is used.
//   - Ob is the single [timestamp, speed m/s, direction deg] row of a rapid_wind
//     message.
//   - Evt is the event payload of evt_precip ([timestamp]) and evt_strike
//     ([timestamp, distance km, energy]).
//   - Timestamp, Uptime (seconds), Voltage (battery volts), RSSI, HubRSSI,
//     SensorStatus (bit flags) and Debug come from device_status; Timestamp,
//     Uptime and RSSI are also sent in hub_status.
//   - ResetFlags, Seq, RadioStats and MqttStats come from hub_status.
type UDPMessage struct {
	SerialNumber     string          `json:"serial_number"`
	Type             MessageType     `json:"type"`
	HubSN            string          `json:"hub_sn"`
	FirmwareRevision FlexInt         `json:"firmware_revision,omitempty"`
	Obs              [][]interface{} `json:"obs,omitempty"`
	Ob               []interface{}   `json:"ob,omitempty"`  // For rapid_wind
	Evt              []interface{}   `json:"evt,omitempty"` // For events
	// Device status fields
	Timestamp    int64   `json:"timestamp,omitempty"`
	Uptime       int     `json:"uptime,omitempty"`
	Voltage      float64 `json:"voltage,omitempty"`
	RSSI         int     `json:"rssi,omitempty"`
	HubRSSI      int     `json:"hub_rssi,omitempty"`
	SensorStatus int     `json:"sensor_status,omitempty"`
	Debug        int     `json:"debug,omitempty"`
	// Hub status fields
	ResetFlags string        `json:"reset_flags,omitempty"`
	Seq        int           `json:"seq,omitempty"`
	RadioStats []interface{} `json:"radio_stats,omitempty"`
	MqttStats  []interface{} `json:"mqtt_stats,omitempty"`
}

// UDPListener listens for UDP broadcasts from a Tempest weather station. Create
// it with NewUDPListener and call Start; all methods are safe for concurrent
// use.
//
// Fields:
//
//   - conn is the bound UDP socket; nil until Start succeeds.
//   - observations is the bounded observation history, oldest first.
//   - maxHistorySize caps len(observations).
//   - latestObs is the most recent observation, or nil before the first.
//   - mu guards the mutable fields below it.
//   - packetCount and lastPacketTime count every datagram received and when the
//     last one arrived, whether or not it parsed.
//   - stationIP is the source address of the first datagram received.
//   - serialNumber is the first non-empty serial_number seen.
//   - deviceStatus and hubStatus are the latest device_status and hub_status
//     reports, or nil before one arrives.
//   - observationChan receives each new observation; sends are non-blocking
//     and dropped when its 100-slot buffer is full.
//   - stopChan is closed by Stop to end the listen loop.
//   - running reports whether Start has been called without a matching Stop.
//   - packetCallback, if set, receives every raw datagram.
type UDPListener struct {
	conn            *net.UDPConn
	observations    []weather.Observation
	maxHistorySize  int
	latestObs       *weather.Observation
	mu              sync.RWMutex
	packetCount     int64
	lastPacketTime  time.Time
	stationIP       string
	serialNumber    string
	deviceStatus    *DeviceStatus
	hubStatus       *HubStatus
	observationChan chan weather.Observation
	stopChan        chan struct{}
	running         bool
	packetCallback  func([]byte) // Callback for raw packet data
}

// DeviceStatus holds device status information from the most recent
// device_status message.
//
// Fields:
//
//   - Timestamp is when the device sent the report.
//   - Uptime is the device uptime in seconds.
//   - Voltage is the battery voltage.
//   - RSSI is the device's radio signal strength in dBm and HubRSSI the
//     hub's, both as reported in the device_status message.
//   - SensorStatus is the sensor-failure bit field reported by the device.
//   - Debug is the device's debug flag.
type DeviceStatus struct {
	Timestamp    time.Time
	Uptime       int
	Voltage      float64
	RSSI         int
	HubRSSI      int
	SensorStatus int
	Debug        int
}

// HubStatus holds hub status information from the most recent hub_status
// message.
//
// Fields:
//
//   - Timestamp is when the hub sent the report.
//   - FirmwareRev is the firmware revision as decimal text.
//   - Uptime is the hub uptime in seconds.
//   - RSSI is the hub's Wi-Fi signal strength in dBm.
//   - ResetFlags lists the causes of the hub's last reset.
//   - Seq is the hub's status sequence number.
//   - SerialNumber is the hub serial.
type HubStatus struct {
	Timestamp    time.Time
	FirmwareRev  string
	Uptime       int
	RSSI         int
	ResetFlags   string
	Seq          int
	SerialNumber string
}

// NewUDPListener creates a new UDP listener that keeps up to maxHistorySize
// observations. Values below 10 are treated as invalid and replaced by 1000;
// values above 100000 are capped at 100000 (with a warning) to bound memory use.
// The listener is idle until Start is called.
func NewUDPListener(maxHistorySize int) *UDPListener {
	if maxHistorySize < 10 {
		maxHistorySize = 1000 // default if invalid
	}

	// Validate history size to prevent excessive memory allocation
	if maxHistorySize > 100000 {
		logger.Warn("History size %d is very large, capping at 100000 to prevent memory issues", maxHistorySize)
		maxHistorySize = 100000
	}

	return &UDPListener{
		observations:    make([]weather.Observation, 0, maxHistorySize),
		maxHistorySize:  maxHistorySize,
		observationChan: make(chan weather.Observation, 100),
		stopChan:        make(chan struct{}),
	}
}

// SetPacketCallback sets a callback function that will be called for each received packet
// The callback receives the raw packet data. It runs synchronously on the
// listener goroutine before the packet is parsed (used by --test-udp mode), and
// the slice aliases the read buffer, so it must be copied if retained. Pass nil
// to remove the callback.
func (l *UDPListener) SetPacketCallback(callback func([]byte)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.packetCallback = callback
}

// Start begins listening for UDP broadcasts on 0.0.0.0:UDPPort and processes them
// in a background goroutine. It returns an error if the listener is already
// running or the port cannot be bound (for example, another process holds it).
func (l *UDPListener) Start() error {
	l.mu.Lock()
	if l.running {
		l.mu.Unlock()
		return fmt.Errorf("UDP listener already running")
	}
	l.running = true
	l.mu.Unlock()

	addr := net.UDPAddr{
		Port: UDPPort,
		IP:   net.ParseIP("0.0.0.0"),
	}

	conn, err := net.ListenUDP("udp", &addr)
	if err != nil {
		l.mu.Lock()
		l.running = false
		l.mu.Unlock()
		return fmt.Errorf("failed to start UDP listener on port %d: %v", UDPPort, err)
	}

	l.conn = conn
	logger.Info("UDP listener started on port %d", UDPPort)

	// Start listening in a goroutine
	go l.listen()

	return nil
}

// Stop stops the UDP listener by signalling the listen loop and closing the
// socket. It is a no-op returning nil if the listener is not running; otherwise it
// returns any error from closing the socket.
func (l *UDPListener) Stop() error {
	l.mu.Lock()
	if !l.running {
		l.mu.Unlock()
		return nil
	}
	l.running = false
	l.mu.Unlock()

	close(l.stopChan)
	if l.conn != nil {
		return l.conn.Close()
	}
	return nil
}

// listen is the main listening loop. It reads datagrams with a one-second read
// deadline so it can notice stopChan being closed, updates the packet statistics
// (recording the first sender's IP as the station IP) and hands each datagram to
// processMessage. Read errors other than timeouts are logged and the loop
// continues.
func (l *UDPListener) listen() {
	buffer := make([]byte, 4096)

	for {
		select {
		case <-l.stopChan:
			logger.Info("UDP listener stopped")
			return
		default:
			// Set read deadline to allow checking stopChan periodically
			_ = l.conn.SetReadDeadline(time.Now().Add(1 * time.Second))

			n, remoteAddr, err := l.conn.ReadFromUDP(buffer)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					// Timeout is expected, continue
					continue
				}
				logger.Error("UDP read error: %v", err)
				continue
			}

			// Update packet statistics
			l.mu.Lock()
			l.packetCount++
			l.lastPacketTime = time.Now()
			if l.stationIP == "" && remoteAddr != nil {
				l.stationIP = remoteAddr.IP.String()
				logger.Info("Detected Tempest station at IP: %s", l.stationIP)
			}
			l.mu.Unlock()

			// Process the message
			l.processMessage(buffer[:n])
		}
	}
}

// PrettyPrintMessage formats a UDP message for human-readable output as a single
// line prefixed with the current local time. Observation and event messages show
// their key readings; messages too short to decode are reported as
// "(incomplete data)" and unparseable JSON as "Failed to parse: ...".
func PrettyPrintMessage(data []byte) string {
	var msg UDPMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return fmt.Sprintf("Failed to parse: %v", err)
	}

	ts := time.Now().Format("15:04:05")
	switch msg.Type {
	case TypeObservationST:
		if len(msg.Obs) > 0 && len(msg.Obs[0]) >= 18 {
			obs := msg.Obs[0]
			temp := obs[7].(float64)
			humidity := obs[8].(float64)
			pressure := obs[6].(float64)
			windAvg := obs[2].(float64)
			windGust := obs[3].(float64)
			windDir := obs[4].(float64)
			rain := obs[12].(float64)
			uv := int(obs[10].(float64))
			lux := obs[9].(float64)
			lightningCount := int(obs[15].(float64))
			lightningDist := obs[14].(float64)
			battery := obs[16].(float64)
			return fmt.Sprintf("[%s] 🌡️  obs_st | Temp: %.1f°C | Humidity: %.0f%% | Pressure: %.1fmb | Wind: %.1f/%.1fm/s@%.0f° | UV: %d | Lux: %.0f | Rain: %.2fmm | Lightning: %d@%.0fkm | Battery: %.2fV | Serial: %s",
				ts, temp, humidity, pressure, windAvg, windGust, windDir, uv, lux, rain, lightningCount, lightningDist, battery, msg.SerialNumber)
		}
	case TypeObservationAir:
		if len(msg.Obs) > 0 && len(msg.Obs[0]) >= 8 {
			obs := msg.Obs[0]
			temp := obs[2].(float64)
			humidity := obs[3].(float64)
			pressure := obs[1].(float64)
			lightningCount := int(obs[4].(float64))
			lightningDist := obs[5].(float64)
			battery := obs[6].(float64)
			return fmt.Sprintf("[%s] 🌡️  obs_air | Temp: %.1f°C | Humidity: %.0f%% | Pressure: %.1fmb | Lightning: %d@%.0fkm | Battery: %.2fV | Serial: %s",
				ts, temp, humidity, pressure, lightningCount, lightningDist, battery, msg.SerialNumber)
		}
	case TypeObservationSky:
		if len(msg.Obs) > 0 && len(msg.Obs[0]) >= 14 {
			obs := msg.Obs[0]
			windAvg := obs[5].(float64)
			windGust := obs[6].(float64)
			windDir := obs[7].(float64)
			rain := obs[3].(float64)
			uv := int(obs[2].(float64))
			lux := obs[1].(float64)
			solar := obs[10].(float64)
			battery := obs[8].(float64)
			return fmt.Sprintf("[%s] obs_sky | Wind: %.1f/%.1fm/s@%.0f° | UV: %d | Lux: %.0f | Solar: %.0fW/m² | Rain: %.2fmm | Battery: %.2fV | Serial: %s",
				ts, windAvg, windGust, windDir, uv, lux, solar, rain, battery, msg.SerialNumber)
		}
	case TypeRapidWind:
		if len(msg.Ob) >= 3 {
			windSpeed := msg.Ob[1].(float64)
			windDir := int(msg.Ob[2].(float64))
			return fmt.Sprintf("[%s] rapid_wind | Speed: %.1fm/s | Direction: %d° | Serial: %s",
				ts, windSpeed, windDir, msg.SerialNumber)
		}
	case TypeRainStart:
		if len(msg.Evt) > 0 {
			timestamp := int64(msg.Evt[0].(float64))
			return fmt.Sprintf("[%s] evt_precip | Rain started at %s | Serial: %s",
				ts, time.Unix(timestamp, 0).Format("15:04:05"), msg.SerialNumber)
		}
	case TypeLightning:
		if len(msg.Evt) >= 3 {
			timestamp := int64(msg.Evt[0].(float64))
			distance := msg.Evt[1].(float64)
			energy := msg.Evt[2].(float64)
			return fmt.Sprintf("[%s] evt_strike | Distance: %.1fkm | Energy: %.0f | Time: %s | Serial: %s",
				ts, distance, energy, time.Unix(timestamp, 0).Format("15:04:05"), msg.SerialNumber)
		}
	case TypeDeviceStatus:
		return fmt.Sprintf("[%s] device_status | Uptime: %ds | Battery: %.2fV | RSSI: %ddBm | Hub RSSI: %ddBm | Sensor Status: 0x%X | Serial: %s",
			ts, msg.Uptime, msg.Voltage, msg.RSSI, msg.HubRSSI, msg.SensorStatus, msg.SerialNumber)
	case TypeHubStatus:
		return fmt.Sprintf("[%s] hub_status | Uptime: %ds | RSSI: %ddBm | Firmware: %d | Reset Flags: %s | Seq: %d | Serial: %s",
			ts, msg.Uptime, msg.RSSI, msg.FirmwareRevision, msg.ResetFlags, msg.Seq, msg.SerialNumber)
	default:
		return fmt.Sprintf("[%s] %s | Serial: %s", ts, msg.Type, msg.SerialNumber)
	}
	return fmt.Sprintf("[%s] %s (incomplete data) | Serial: %s", ts, msg.Type, msg.SerialNumber)
}

// processMessage parses and processes a UDP message: it passes the raw bytes to
// the packet callback, prints a pretty form when debug logging is enabled,
// records the first serial number seen, and dispatches on the message type.
// Unparseable messages are logged at debug level and dropped.
func (l *UDPListener) processMessage(data []byte) {
	// Call packet callback if set (for --test-udp mode)
	l.mu.RLock()
	callback := l.packetCallback
	l.mu.RUnlock()

	if callback != nil {
		callback(data)
	}

	// Pretty print packet if debug logging is enabled
	if logger.IsDebugEnabled() {
		fmt.Println(PrettyPrintMessage(data))
	}

	var msg UDPMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		logger.Debug("Failed to parse UDP message: %v", err)
		return
	}

	logger.Debug("Parsed UDP message - Type: %s, Serial: %s, Hub: %s", msg.Type, msg.SerialNumber, msg.HubSN)

	// Update serial number if not set
	if l.serialNumber == "" && msg.SerialNumber != "" {
		l.mu.Lock()
		l.serialNumber = msg.SerialNumber
		l.mu.Unlock()
		logger.Info("Detected Tempest device serial: %s", msg.SerialNumber)
	}

	switch msg.Type {
	case TypeObservationST:
		l.processObservationST(msg)
	case TypeObservationAir:
		l.processObservationAir(msg)
	case TypeObservationSky:
		l.processObservationSky(msg)
	case TypeRapidWind:
		l.processRapidWind(msg)
	case TypeDeviceStatus:
		l.processDeviceStatus(msg)
	case TypeHubStatus:
		l.processHubStatus(msg)
	case TypeRainStart:
		timestamp := int64(msg.Evt[0].(float64))
		logger.Debug("UDP evt_precip - Rain start event at timestamp=%d (%v)", timestamp, time.Unix(timestamp, 0))
	case TypeLightning:
		if len(msg.Evt) >= 3 {
			timestamp := int64(msg.Evt[0].(float64))
			distance := msg.Evt[1].(float64)
			energy := msg.Evt[2].(float64)
			logger.Debug("UDP evt_strike - Lightning strike at timestamp=%d, distance=%.1fkm, energy=%.0f", timestamp, distance, energy)
		}
	default:
		logger.Debug("Unknown UDP message type: %s", msg.Type)
	}
}

// processObservationST processes a Tempest (ST) observation message, converting
// the first 18-element observation row into a weather.Observation and adding it
// to the history. Shorter rows are logged and ignored. Units are as broadcast
// (metric: m/s, mb, deg C, mm).
func (l *UDPListener) processObservationST(msg UDPMessage) {
	if len(msg.Obs) == 0 || len(msg.Obs[0]) < 18 {
		logger.Debug("Invalid Tempest observation data")
		return
	}

	obs := msg.Obs[0]

	// Parse observation according to Tempest UDP format
	// [0]=timestamp, [1]=wind_lull, [2]=wind_avg, [3]=wind_gust, [4]=wind_dir,
	// [5]=wind_sample_interval, [6]=pressure, [7]=temp, [8]=humidity, [9]=lux,
	// [10]=uv, [11]=solar_rad, [12]=rain_1min, [13]=precip_type, [14]=lightning_dist,
	// [15]=lightning_count, [16]=battery, [17]=report_interval

	observation := weather.Observation{
		Timestamp:            int64(obs[0].(float64)),
		WindLull:             obs[1].(float64),
		WindAvg:              obs[2].(float64),
		WindGust:             obs[3].(float64),
		WindDirection:        obs[4].(float64),
		StationPressure:      obs[6].(float64),
		AirTemperature:       obs[7].(float64),
		RelativeHumidity:     obs[8].(float64),
		Illuminance:          obs[9].(float64),
		UV:                   int(obs[10].(float64)),
		SolarRadiation:       obs[11].(float64),
		RainAccumulated:      obs[12].(float64),
		PrecipitationType:    int(obs[13].(float64)),
		LightningStrikeAvg:   obs[14].(float64),
		LightningStrikeCount: int(obs[15].(float64)),
		Battery:              obs[16].(float64),
		ReportInterval:       int(obs[17].(float64)),
	}

	logger.Debug("UDP obs_st - Timestamp=%d, Temp=%.1f°C, Humidity=%.0f%%, Pressure=%.1fmb, Wind=%.1f/%.1f/%.1fm/s@%.0f°, Lux=%.0f, UV=%d, Rain=%.2fmm, Lightning=%d@%.0fkm, Battery=%.2fV",
		observation.Timestamp, observation.AirTemperature, observation.RelativeHumidity, observation.StationPressure,
		observation.WindLull, observation.WindAvg, observation.WindGust, observation.WindDirection,
		observation.Illuminance, observation.UV, observation.RainAccumulated,
		observation.LightningStrikeCount, observation.LightningStrikeAvg, observation.Battery)

	l.addObservation(observation)
}

// processObservationAir processes an AIR device observation, adding it to the
// history. Rows shorter than 8 elements are silently ignored.
func (l *UDPListener) processObservationAir(msg UDPMessage) {
	if len(msg.Obs) == 0 || len(msg.Obs[0]) < 8 {
		return
	}

	obs := msg.Obs[0]
	// AIR format: [0]=timestamp, [1]=pressure, [2]=temp, [3]=humidity,
	// [4]=lightning_count, [5]=lightning_dist, [6]=battery, [7]=report_interval

	observation := weather.Observation{
		Timestamp:            int64(obs[0].(float64)),
		StationPressure:      obs[1].(float64),
		AirTemperature:       obs[2].(float64),
		RelativeHumidity:     obs[3].(float64),
		LightningStrikeCount: int(obs[4].(float64)),
		LightningStrikeAvg:   obs[5].(float64),
		Battery:              obs[6].(float64),
		ReportInterval:       int(obs[7].(float64)),
	}

	logger.Debug("UDP obs_air - Timestamp=%d, Temp=%.1f°C, Humidity=%.0f%%, Pressure=%.1fmb, Lightning=%d@%.0fkm, Battery=%.2fV",
		observation.Timestamp, observation.AirTemperature, observation.RelativeHumidity,
		observation.StationPressure, observation.LightningStrikeCount, observation.LightningStrikeAvg, observation.Battery)

	l.addObservation(observation)
}

// processObservationSky processes a SKY device observation, adding it to the
// history. Rows shorter than 14 elements are silently ignored.
func (l *UDPListener) processObservationSky(msg UDPMessage) {
	if len(msg.Obs) == 0 || len(msg.Obs[0]) < 14 {
		return
	}

	obs := msg.Obs[0]
	// SKY format: [0]=timestamp, [1]=lux, [2]=uv, [3]=rain_1min, [4]=wind_lull,
	// [5]=wind_avg, [6]=wind_gust, [7]=wind_dir, [8]=battery, [9]=report_interval,
	// [10]=solar_rad, [11]=local_rain (null), [12]=precip_type, [13]=wind_sample_interval

	observation := weather.Observation{
		Timestamp:         int64(obs[0].(float64)),
		Illuminance:       obs[1].(float64),
		UV:                int(obs[2].(float64)),
		RainAccumulated:   obs[3].(float64),
		WindLull:          obs[4].(float64),
		WindAvg:           obs[5].(float64),
		WindGust:          obs[6].(float64),
		WindDirection:     obs[7].(float64),
		Battery:           obs[8].(float64),
		ReportInterval:    int(obs[9].(float64)),
		SolarRadiation:    obs[10].(float64),
		PrecipitationType: int(obs[12].(float64)),
	}

	logger.Debug("UDP obs_sky - Timestamp=%d, Wind=%.1f/%.1f/%.1fm/s@%.0f°, Lux=%.0f, UV=%d, Solar=%.0fW/m², Rain=%.2fmm, Battery=%.2fV",
		observation.Timestamp, observation.WindLull, observation.WindAvg, observation.WindGust, observation.WindDirection,
		observation.Illuminance, observation.UV, observation.SolarRadiation, observation.RainAccumulated, observation.Battery)

	l.addObservation(observation)
}

// processRapidWind processes rapid wind updates (every 3 seconds). It only logs
// them; wind in the history is updated when the next full observation arrives.
func (l *UDPListener) processRapidWind(msg UDPMessage) {
	if len(msg.Ob) < 3 {
		return
	}

	// Rapid wind: [0]=timestamp, [1]=wind_speed, [2]=wind_direction
	timestamp := int64(msg.Ob[0].(float64))
	windSpeed := msg.Ob[1].(float64)
	windDir := int(msg.Ob[2].(float64))
	logger.Debug("UDP rapid_wind - Timestamp=%d, Speed=%.1fm/s, Direction=%d°", timestamp, windSpeed, windDir)

	// We could update wind in real-time here, but for now just log it
	// The full observation will be processed when obs_st arrives
}

// processDeviceStatus processes device status messages, replacing the stored
// DeviceStatus.
func (l *UDPListener) processDeviceStatus(msg UDPMessage) {
	status := &DeviceStatus{
		Timestamp:    time.Unix(msg.Timestamp, 0),
		Uptime:       msg.Uptime,
		Voltage:      msg.Voltage,
		RSSI:         msg.RSSI,
		HubRSSI:      msg.HubRSSI,
		SensorStatus: msg.SensorStatus,
		Debug:        msg.Debug,
	}

	l.mu.Lock()
	l.deviceStatus = status
	l.mu.Unlock()

	logger.Debug("UDP device_status - Serial=%s, Timestamp=%d, Battery=%.2fV, Uptime=%ds, RSSI=%ddBm, Hub RSSI=%ddBm, Sensor Status=0x%X",
		msg.SerialNumber, msg.Timestamp, status.Voltage, status.Uptime, status.RSSI, status.HubRSSI, status.SensorStatus)
}

// processHubStatus processes hub status messages, replacing the stored
// HubStatus.
func (l *UDPListener) processHubStatus(msg UDPMessage) {
	status := &HubStatus{
		Timestamp:    time.Unix(msg.Timestamp, 0),
		FirmwareRev:  fmt.Sprintf("%d", msg.FirmwareRevision),
		Uptime:       msg.Uptime,
		RSSI:         msg.RSSI,
		ResetFlags:   msg.ResetFlags,
		Seq:          msg.Seq,
		SerialNumber: msg.SerialNumber,
	}

	l.mu.Lock()
	l.hubStatus = status
	l.mu.Unlock()

	logger.Debug("UDP hub_status - Serial=%s, Timestamp=%d, Firmware=%s, Uptime=%ds, RSSI=%ddBm, ResetFlags=%s, Seq=%d",
		msg.SerialNumber, msg.Timestamp, status.FirmwareRev, status.Uptime, status.RSSI, status.ResetFlags, status.Seq)
}

// addObservation adds an observation to the history and notifies observers. The
// oldest entry is dropped once maxHistorySize is reached, obs becomes the latest
// observation, and it is offered to observationChan without blocking (dropped
// if the channel is full).
func (l *UDPListener) addObservation(obs weather.Observation) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Add to history (circular buffer)
	if len(l.observations) >= l.maxHistorySize {
		// Remove oldest observation
		l.observations = l.observations[1:]
	}
	l.observations = append(l.observations, obs)

	// Update latest
	l.latestObs = &obs

	// Send to channel (non-blocking)
	select {
	case l.observationChan <- obs:
	default:
		logger.Debug("Observation channel full, skipping")
	}
}

// GetLatestObservation returns the most recent observation as a copy, or nil if
// none has been received.
func (l *UDPListener) GetLatestObservation() *weather.Observation {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.latestObs == nil {
		return nil
	}
	obs := *l.latestObs
	return &obs
}

// GetObservations returns all stored observations as a copy, oldest first.
func (l *UDPListener) GetObservations() []weather.Observation {
	l.mu.RLock()
	defer l.mu.RUnlock()

	result := make([]weather.Observation, len(l.observations))
	copy(result, l.observations)
	return result
}

// GetStats returns UDP listener statistics: the number of datagrams received,
// when the last one arrived (zero if none), the station IP and the device serial
// number (empty until detected).
func (l *UDPListener) GetStats() (packetCount int64, lastPacket time.Time, stationIP, serialNumber string) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.packetCount, l.lastPacketTime, l.stationIP, l.serialNumber
}

// GetDeviceStatus returns the latest device status as a map (for interface compatibility)
// with JSON-style keys (timestamp in Unix seconds, uptime, voltage, rssi,
// hub_rssi, sensor_status, serial_number), or nil if no device_status message
// has been received.
func (l *UDPListener) GetDeviceStatus() interface{} {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.deviceStatus == nil {
		return nil
	}
	return map[string]interface{}{
		"timestamp":     l.deviceStatus.Timestamp.Unix(),
		"uptime":        l.deviceStatus.Uptime,
		"voltage":       l.deviceStatus.Voltage,
		"rssi":          l.deviceStatus.RSSI,
		"hub_rssi":      l.deviceStatus.HubRSSI,
		"sensor_status": l.deviceStatus.SensorStatus,
		"serial_number": l.serialNumber,
	}
}

// GetHubStatus returns the latest hub status as a map (for interface compatibility)
// with keys timestamp (Unix seconds), firmware_rev, uptime, rssi, reset_flags
// and serial_number, or nil if no hub_status message has been received.
func (l *UDPListener) GetHubStatus() interface{} {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.hubStatus == nil {
		return nil
	}
	return map[string]interface{}{
		"timestamp":     l.hubStatus.Timestamp.Unix(),
		"firmware_rev":  l.hubStatus.FirmwareRev,
		"uptime":        l.hubStatus.Uptime,
		"rssi":          l.hubStatus.RSSI,
		"reset_flags":   l.hubStatus.ResetFlags,
		"serial_number": l.hubStatus.SerialNumber,
	}
}

// ObservationChannel returns the channel for receiving new observations. The
// channel is never closed.
func (l *UDPListener) ObservationChannel() <-chan weather.Observation {
	return l.observationChan
}

// IsReceivingData returns true if we've received data recently: any datagram
// within the last 5 minutes.
func (l *UDPListener) IsReceivingData() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.lastPacketTime.IsZero() {
		return false
	}

	return time.Since(l.lastPacketTime) < 5*time.Minute
}
