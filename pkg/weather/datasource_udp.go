// This file provides the UDP data source implementation.

package weather

import (
	"sync"
	"time"

	"tempest-homekit-go/pkg/logger"
)

// UDPListener is an interface to avoid import cycle with pkg/udp. It is the subset
// of *udp.UDPListener that UDPDataSource needs; see that type for full
// semantics.
//
// Methods:
//
//   - Start begins receiving broadcasts.
//   - Stop stops receiving broadcasts.
//   - GetLatestObservation returns the most recent observation, or nil.
//   - GetStats returns packet count, last packet time, station IP and device
//     serial number.
//   - GetObservations returns the stored observation history.
//   - IsReceivingData reports whether data has arrived recently.
//   - ObservationChannel delivers each new observation.
//   - GetDeviceStatus and GetHubStatus return the latest device and hub status
//     as a map[string]interface{}, or nil if none has been received.
type UDPListener interface {
	Start() error
	Stop() error
	GetLatestObservation() *Observation
	GetStats() (packetCount int64, lastPacket time.Time, stationIP, serialNumber string)
	GetObservations() []Observation
	IsReceivingData() bool
	ObservationChannel() <-chan Observation
	GetDeviceStatus() interface{}
	GetHubStatus() interface{}
}

// UDPDataSource implements DataSource for local UDP broadcast listening. It
// forwards observations from a UDPListener, pushes the listener's device and hub
// status to an optional StatusManager, and, when internet access and a token are
// available, polls the WeatherFlow forecast every 30 minutes. It is safe for
// concurrent use.
//
// Fields: listener supplies the broadcasts; noInternet, stationID and token
// control forecast polling (it needs all three); statusManager receives status
// updates (nil to disable); mu guards the fields below it; latestObservation
// and latestForecast hold the most recent data; observationChan (buffer 100)
// delivers observations and is closed by Stop; stopChan is closed by Stop to
// end the background loops; running is true between Start and Stop.
type UDPDataSource struct {
	listener      UDPListener
	noInternet    bool
	stationID     int
	token         string
	statusManager *StatusManager

	mu                sync.RWMutex
	latestObservation *Observation
	latestForecast    *ForecastResponse
	observationChan   chan Observation
	stopChan          chan struct{}
	running           bool
}

// NewUDPDataSource creates a new UDP-based data source
// Pass in an already-created UDP listener to avoid import cycle. noInternet
// disables forecast polling; stationID and token are only used for forecasts.
// Call Start to begin.
func NewUDPDataSource(listener UDPListener, noInternet bool, stationID int, token string) *UDPDataSource {
	return &UDPDataSource{
		listener:        listener,
		noInternet:      noInternet,
		stationID:       stationID,
		token:           token,
		observationChan: make(chan Observation, 100),
		stopChan:        make(chan struct{}),
	}
}

// Start begins listening for UDP broadcasts by starting the listener and the
// forwarding loop (plus the forecast loop when online with a token), and returns
// the observation channel. Calling it while running returns the same channel.
// It returns the listener's error, for example when UDP port 50222 is in use.
func (u *UDPDataSource) Start() (<-chan Observation, error) {
	u.mu.Lock()
	if u.running {
		u.mu.Unlock()
		return u.observationChan, nil
	}
	u.running = true
	u.mu.Unlock()

	// Start the UDP listener (already created and passed in constructor)
	if err := u.listener.Start(); err != nil {
		u.mu.Lock()
		u.running = false
		u.mu.Unlock()
		return nil, err
	}

	logger.Info("UDP data source started on port 50222")

	// Start observation forwarding goroutine
	go u.forwardLoop()

	// Start optional forecast polling (if internet enabled)
	if !u.noInternet && u.token != "" {
		go u.forecastLoop()
	}

	return u.observationChan, nil
}

// Stop gracefully shuts down the UDP data source: it stops the background loops
// and the listener and closes the observation channel. It is a no-op if not
// running and always returns nil (listener errors are ignored).
func (u *UDPDataSource) Stop() error {
	u.mu.Lock()
	defer u.mu.Unlock()

	if !u.running {
		return nil
	}

	close(u.stopChan)
	u.running = false

	if u.listener != nil {
		_ = u.listener.Stop()
	}

	close(u.observationChan)

	logger.Info("UDP data source stopped")
	return nil
}

// GetLatestObservation returns the most recent observation forwarded by this data
// source, falling back to the listener's latest observation, or nil.
func (u *UDPDataSource) GetLatestObservation() *Observation {
	u.mu.RLock()
	defer u.mu.RUnlock()

	if u.latestObservation != nil {
		return u.latestObservation
	}

	// Fall back to UDP listener's latest observation
	if u.listener != nil {
		return u.listener.GetLatestObservation()
	}

	return nil
}

// GetForecast returns the latest forecast data (may be nil in offline mode)
func (u *UDPDataSource) GetForecast() *ForecastResponse {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.latestForecast
}

// GetStatus returns the current status of the UDP data source, taking packet
// count, last packet time, station IP, serial number and observation count from
// the listener.
func (u *UDPDataSource) GetStatus() DataSourceStatus {
	u.mu.RLock()
	defer u.mu.RUnlock()

	status := DataSourceStatus{
		Type:   DataSourceUDP,
		Active: u.running,
	}

	if u.listener != nil {
		packetCount, lastPacket, stationIP, serialNumber := u.listener.GetStats()
		status.PacketCount = packetCount
		status.LastUpdate = lastPacket
		status.StationIP = stationIP
		status.SerialNumber = serialNumber

		// Count observations
		obs := u.listener.GetObservations()
		status.ObservationCount = int64(len(obs))
	}

	return status
}

// GetType returns the data source type
func (u *UDPDataSource) GetType() DataSourceType {
	return DataSourceUDP
}

// SetStatusManager sets the status manager for UDP status updates; pass nil to stop updates.
func (u *UDPDataSource) SetStatusManager(sm *StatusManager) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.statusManager = sm
}

// forwardLoop forwards observations from UDP listener to the data source channel
// (non-blocking; dropped if full), recording each as the latest and pushing UDP
// status to the status manager. It also refreshes status every 30 seconds, and
// exits when stopChan or the listener's channel is closed.
func (u *UDPDataSource) forwardLoop() {
	logger.Info("Starting UDP observation forwarding loop with periodic status updates (30s interval)")

	udpChan := u.listener.ObservationChannel()

	// Create ticker for periodic status updates (every 30 seconds)
	statusTicker := time.NewTicker(30 * time.Second)
	defer statusTicker.Stop()

	for {
		select {
		case <-u.stopChan:
			logger.Info("UDP forwarding loop stopped")
			return

		case <-statusTicker.C:
			// Periodic status update check
			u.updateStatusFromUDP()

		case obs, ok := <-udpChan:
			if !ok {
				logger.Info("UDP observation channel closed")
				return
			}

			logger.Debug("Received observation from UDP listener")

			u.mu.Lock()
			u.latestObservation = &obs
			u.mu.Unlock()

			// Update status manager with latest UDP status
			u.updateStatusFromUDP()

			// Forward to data source channel (non-blocking)
			select {
			case u.observationChan <- obs:
				logger.Debug("Observation forwarded to data source channel")
			default:
				logger.Debug("Data source channel full, skipping observation")
			}
		}
	}
}

// forecastLoop periodically fetches forecast data (only if internet enabled): once at start and then every 30 minutes until stopChan is closed.
func (u *UDPDataSource) forecastLoop() {
	logger.Info("Starting forecast polling loop (30 minute interval)")

	// Initial fetch
	u.fetchForecast()

	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-u.stopChan:
			logger.Debug("Forecast polling loop stopped")
			return

		case <-ticker.C:
			u.fetchForecast()
		}
	}
}

// updateStatusFromUDP retrieves device and hub status from UDP listener and updates status manager.
// It converts the listener's map form into UDPDeviceStatus/UDPHubStatus and does
// nothing if there is no status manager or neither status is available.
func (u *UDPDataSource) updateStatusFromUDP() {
	u.mu.RLock()
	statusManager := u.statusManager
	listener := u.listener
	u.mu.RUnlock()

	if statusManager == nil || listener == nil {
		return
	}

	var deviceStatus *UDPDeviceStatus
	var hubStatus *UDPHubStatus

	// Get device status from UDP listener
	if ds := listener.GetDeviceStatus(); ds != nil {
		// Convert from udp.DeviceStatus to weather.UDPDeviceStatus
		if dsMap, ok := ds.(map[string]interface{}); ok {
			deviceStatus = &UDPDeviceStatus{
				Timestamp:    getMapInt64(dsMap, "timestamp"),
				Uptime:       getMapInt(dsMap, "uptime"),
				Voltage:      getMapFloat64(dsMap, "voltage"),
				RSSI:         getMapInt(dsMap, "rssi"),
				HubRSSI:      getMapInt(dsMap, "hub_rssi"),
				SensorStatus: getMapInt(dsMap, "sensor_status"),
				SerialNumber: getMapString(dsMap, "serial_number"),
			}
		}
	}

	// Get hub status from UDP listener
	if hs := listener.GetHubStatus(); hs != nil {
		// Convert from udp.HubStatus to weather.UDPHubStatus
		if hsMap, ok := hs.(map[string]interface{}); ok {
			hubStatus = &UDPHubStatus{
				Timestamp:    getMapInt64(hsMap, "timestamp"),
				FirmwareRev:  getMapString(hsMap, "firmware_rev"),
				Uptime:       getMapInt(hsMap, "uptime"),
				RSSI:         getMapInt(hsMap, "rssi"),
				ResetFlags:   getMapString(hsMap, "reset_flags"),
				SerialNumber: getMapString(hsMap, "serial_number"),
			}
		}
	}

	// Update status manager with UDP data
	if deviceStatus != nil || hubStatus != nil {
		statusManager.UpdateFromUDP(deviceStatus, hubStatus)
		logger.Debug("Updated status manager from UDP: device=%v, hub=%v", deviceStatus != nil, hubStatus != nil)
	}
}

// fetchForecast retrieves forecast data from the API and stores it. It is skipped
// in offline mode or without a token or station ID; errors are logged and the
// previous forecast is kept.
func (u *UDPDataSource) fetchForecast() {
	if u.noInternet || u.token == "" || u.stationID == 0 {
		logger.Debug("Skipping forecast fetch (offline mode, no token, or no station ID)")
		return
	}

	logger.Debug("UDP data source: fetching forecast from API")

	forecast, err := GetForecast(u.stationID, u.token)
	if err != nil {
		logger.Error("Error getting forecast: %v", err)
		return
	}

	u.mu.Lock()
	u.latestForecast = forecast
	u.mu.Unlock()

	logger.Debug("Successfully fetched forecast data")
}

// Helper functions for converting map values (prefixed to avoid name conflicts).

// getMapInt64 returns m[key] as an int64 if it holds an int64, int or float64,
// and 0 if the key is missing or of another type.
func getMapInt64(m map[string]interface{}, key string) int64 {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case int64:
			return val
		case int:
			return int64(val)
		case float64:
			return int64(val)
		}
	}
	return 0
}

// getMapInt returns m[key] as an int if it holds an int, int64 or float64, and 0 otherwise.
func getMapInt(m map[string]interface{}, key string) int {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case int:
			return val
		case int64:
			return int(val)
		case float64:
			return int(val)
		}
	}
	return 0
}

// getMapFloat64 returns m[key] as a float64 if it holds a float64, int or int64, and 0 otherwise.
func getMapFloat64(m map[string]interface{}, key string) float64 {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case float64:
			return val
		case int:
			return float64(val)
		case int64:
			return float64(val)
		}
	}
	return 0
}

// getMapString returns m[key] if it holds a string, and "" otherwise.
func getMapString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
