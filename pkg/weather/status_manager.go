// This file provides status management for Tempest weather station data.
// The StatusManager handles periodic web scraping when enabled via CLI flag.

package weather

import (
	"fmt"
	"sync"
	"tempest-homekit-go/pkg/logger"
	"time"
)

// StatusManager handles periodic scraping and caching of station status for the
// dashboard. It always holds a status: a placeholder ("--" values) until real
// data arrives from web scraping (every 15 minutes when enabled), from UDP
// device/hub status messages (UpdateFromUDP) or from observation battery
// readings (UpdateBatteryFromObservation).
//
// Fields: stationID and logLevel parameterise scraping; useWebScraping enables
// it; cachedStatus is the current status, guarded by mutex; stopChan stops the
// scraping loop; scrapingActive records whether Start launched it.
type StatusManager struct {
	stationID      int
	logLevel       string
	useWebScraping bool
	cachedStatus   *StationStatus
	mutex          sync.RWMutex
	stopChan       chan bool
	scrapingActive bool
}

// NewStatusManager creates a new status manager for stationID, initialised with
// the fallback status. useWebScraping enables scraping of the TempestWX status
// page once Start is called; logLevel "debug" enables diagnostics.
func NewStatusManager(stationID int, logLevel string, useWebScraping bool) *StatusManager {
	manager := &StatusManager{
		stationID:      stationID,
		logLevel:       logLevel,
		useWebScraping: useWebScraping,
		stopChan:       make(chan bool),
	}

	// Initialize with fallback status
	manager.cachedStatus = manager.createFallbackStatus()

	return manager
}

// Start begins the periodic status scraping if web scraping is enabled: one
// scrape immediately and then one every 15 minutes, each in its own goroutine.
// It does nothing when scraping is disabled.
func (sm *StatusManager) Start() {
	if !sm.useWebScraping {
		if sm.logLevel == "debug" {
			logger.Debug("Web status scraping disabled, using API fallback only")
		}
		return
	}

	if sm.logLevel == "debug" {
		logger.Debug("Starting status manager with 15-minute web scraping interval")
	}

	sm.scrapingActive = true

	// Do initial scrape
	go sm.performScrape()

	// Start periodic scraping
	go sm.periodicScraping()
}

// Stop stops the periodic scraping. It blocks until the scraping loop receives
// the stop signal and does nothing if scraping was never started.
func (sm *StatusManager) Stop() {
	if sm.scrapingActive {
		sm.stopChan <- true
		sm.scrapingActive = false
		if sm.logLevel == "debug" {
			logger.Debug("Status manager stopped")
		}
	}
}

// GetStatus returns the current cached status as a copy that the caller may modify.
func (sm *StatusManager) GetStatus() *StationStatus {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	// Return a copy to avoid concurrent modification
	statusCopy := *sm.cachedStatus
	return &statusCopy
}

// periodicScraping runs the scraping loop every 15 minutes until a value is received on stopChan.
func (sm *StatusManager) periodicScraping() {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			go sm.performScrape()
		case <-sm.stopChan:
			return
		}
	}
}

// performScrape attempts to scrape status data, first with the headless browser
// and then, if that fails, with a plain HTTP fetch. A result counts only if
// hasUsefulData accepts it, in which case it is tagged with DataSource
// "api, web status page"; otherwise the fallback status is cached instead.
func (sm *StatusManager) performScrape() {
	if sm.logLevel == "debug" {
		logger.Debug("Performing status scrape for station %d", sm.stationID)
	}

	var status *StationStatus
	var err error

	if sm.useWebScraping {
		// Try headless browser scraping first
		status, err = GetStationStatusWithBrowser(sm.stationID, sm.logLevel)
		if err != nil {
			if sm.logLevel == "debug" {
				logger.Debug("Browser scraping failed: %v", err)
			}
			// Fall back to regular HTTP scraping
			status, err = GetStationStatus(sm.stationID, sm.logLevel)
			if err == nil && sm.hasUsefulData(status) {
				status.DataSource = "api, web status page"
				status.LastScraped = time.Now().UTC().Format(time.RFC3339)
				status.ScrapingEnabled = true
				if sm.logLevel == "debug" {
					logger.Debug("HTTP scraping succeeded with useful data")
				}
			} else if err == nil && !sm.hasUsefulData(status) {
				if sm.logLevel == "debug" {
					logger.Debug("HTTP scraping succeeded but no useful data found")
				}
			}
		} else if sm.hasUsefulData(status) {
			// Browser scraping succeeded and got useful data
			status.DataSource = "api, web status page"
			status.LastScraped = time.Now().UTC().Format(time.RFC3339)
			status.ScrapingEnabled = true
			if sm.logLevel == "debug" {
				logger.Debug("Browser scraping succeeded with useful data")
			}
		} else if status != nil && !sm.hasUsefulData(status) {
			if sm.logLevel == "debug" {
				logger.Debug("Browser scraping succeeded but no useful data found - Battery: %s, DeviceUptime: %s, HubUptime: %s",
					status.BatteryVoltage, status.DeviceUptime, status.HubUptime)
			}
		}
	}

	// If scraping failed or disabled, or got no useful data, create fallback status
	if status == nil || err != nil || !sm.hasUsefulData(status) {
		status = sm.createFallbackStatus()
		if sm.logLevel == "debug" {
			logger.Debug("Using fallback status (scraping failed or no useful data)")
		}
	}

	// Update cached status
	sm.mutex.Lock()
	sm.cachedStatus = status
	sm.mutex.Unlock()

	if sm.logLevel == "debug" {
		logger.Debug("Status updated - Source: %s, Battery: %s, DeviceUptime: %s, LastScraped: %s",
			status.DataSource, status.BatteryVoltage, status.DeviceUptime, status.LastScraped)
	}
}

// hasUsefulData checks if the status contains any useful scraped data: any of
// battery voltage, device or hub uptime, network status or serial number set
// to something other than "" or the "--" placeholder.
func (sm *StatusManager) hasUsefulData(status *StationStatus) bool {
	if status == nil {
		return false
	}

	// Consider it successful if we got any of these key data points
	hasData := (status.BatteryVoltage != "" && status.BatteryVoltage != "--") ||
		(status.DeviceUptime != "" && status.DeviceUptime != "--") ||
		(status.HubUptime != "" && status.HubUptime != "--") ||
		(status.DeviceNetworkStatus != "" && status.DeviceNetworkStatus != "--") ||
		(status.HubNetworkStatus != "" && status.HubNetworkStatus != "--") ||
		(status.DeviceSerialNumber != "" && status.DeviceSerialNumber != "--") ||
		(status.HubSerialNumber != "" && status.HubSerialNumber != "--")

	if sm.logLevel == "debug" {
		logger.Debug("hasUsefulData check - Battery: '%s', DeviceUptime: '%s', HubUptime: '%s', DeviceNetwork: '%s', HubNetwork: '%s', DeviceSerial: '%s', HubSerial: '%s' -> %t",
			status.BatteryVoltage, status.DeviceUptime, status.HubUptime,
			status.DeviceNetworkStatus, status.HubNetworkStatus,
			status.DeviceSerialNumber, status.HubSerialNumber, hasData)
	}

	return hasData
}

// createFallbackStatus creates a status with fallback values and appropriate
// metadata: every display field is "--", DataSource is "api", LastScraped is
// now and ScrapingEnabled mirrors the configuration. (An earlier note said it
// could use the battery voltage from a latestObs argument; it takes no
// argument - UpdateBatteryFromObservation does that separately.)
func (sm *StatusManager) createFallbackStatus() *StationStatus {
	dataSource := "api"
	if sm.useWebScraping {
		dataSource = "api" // Even when scraping is enabled but fails, we still have API data
	}

	status := &StationStatus{
		BatteryVoltage:      "--",
		BatteryStatus:       "--",
		DeviceUptime:        "--",
		HubUptime:           "--",
		DeviceNetworkStatus: "--",
		HubNetworkStatus:    "--",
		DeviceSignal:        "--",
		HubWiFiSignal:       "--",
		SensorStatus:        "--",
		DeviceLastObs:       "--",
		DeviceSerialNumber:  "--",
		DeviceFirmware:      "--",
		HubLastStatus:       "--",
		HubSerialNumber:     "--",
		HubFirmware:         "--",
		DataSource:          dataSource,
		LastScraped:         time.Now().UTC().Format(time.RFC3339),
		ScrapingEnabled:     sm.useWebScraping,
	}

	return status
}

// UpdateBatteryFromObservation updates the cached status with battery data from the latest observation.
// It only applies while the status comes from the API fallback (DataSource
// "api" or "fallback") and obs has a positive battery voltage; the status is
// then reported as "Good".
func (sm *StatusManager) UpdateBatteryFromObservation(obs *Observation) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	if sm.cachedStatus == nil {
		return
	}

	// Only update battery data if we're using API-only data (no scraping or scraping failed)
	if sm.cachedStatus.DataSource == "api" || sm.cachedStatus.DataSource == "fallback" {
		if obs != nil && obs.Battery > 0 {
			sm.cachedStatus.BatteryVoltage = fmt.Sprintf("%.1fV", obs.Battery)
			sm.cachedStatus.BatteryStatus = "Good" // Assume good status if we have battery data
			if sm.logLevel == "debug" {
				logger.Debug("Updated battery data from observation: %s", sm.cachedStatus.BatteryVoltage)
			}
		}
	}
}

// UDPDeviceStatus represents device status from UDP broadcasts (a device_status
// message), in the form StatusManager consumes.
//
// Fields: Timestamp is Unix seconds; Uptime is in seconds; Voltage is the
// battery voltage; RSSI and HubRSSI are signal strengths in dBm; SensorStatus is
// the sensor-failure bit field (0 means all OK); SerialNumber is the device
// serial.
type UDPDeviceStatus struct {
	Timestamp    int64
	Uptime       int
	Voltage      float64
	RSSI         int
	HubRSSI      int
	SensorStatus int
	SerialNumber string
}

// UDPHubStatus represents hub status from UDP broadcasts (a hub_status message),
// in the form StatusManager consumes.
//
// Fields: Timestamp is Unix seconds; FirmwareRev is the firmware revision as
// decimal text; Uptime is in seconds; RSSI is the hub Wi-Fi signal in dBm;
// ResetFlags lists the causes of the last reset; SerialNumber is the hub
// serial.
type UDPHubStatus struct {
	Timestamp    int64
	FirmwareRev  string
	Uptime       int
	RSSI         int
	ResetFlags   string
	SerialNumber string
}

// UpdateFromUDP updates the cached status with data from UDP broadcasts.
// Either argument may be nil. Battery voltage is rated Good (>= 2.5 V), Fair
// (>= 2.3 V) or Low; RSSI is rated Excellent (>= -60), Good (>= -70), Fair
// (>= -80) or Poor; uptimes are formatted as "Nd Nh Nm Ns". The update only
// applies while the status comes from the API fallback (DataSource "api" or
// "fallback"), and it then sets DataSource to "udp".
func (sm *StatusManager) UpdateFromUDP(deviceStatus *UDPDeviceStatus, hubStatus *UDPHubStatus) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	if sm.cachedStatus == nil {
		return
	}

	// Only update if not using web scraping or if scraping failed
	if sm.cachedStatus.DataSource == "api" || sm.cachedStatus.DataSource == "fallback" {
		if deviceStatus != nil {
			// Update battery voltage and status
			if deviceStatus.Voltage > 0 {
				sm.cachedStatus.BatteryVoltage = fmt.Sprintf("%.2fV", deviceStatus.Voltage)
				if deviceStatus.Voltage >= 2.5 {
					sm.cachedStatus.BatteryStatus = "Good"
				} else if deviceStatus.Voltage >= 2.3 {
					sm.cachedStatus.BatteryStatus = "Fair"
				} else {
					sm.cachedStatus.BatteryStatus = "Low"
				}
			}

			// Update device uptime
			if deviceStatus.Uptime > 0 {
				days := deviceStatus.Uptime / 86400
				hours := (deviceStatus.Uptime % 86400) / 3600
				minutes := (deviceStatus.Uptime % 3600) / 60
				seconds := deviceStatus.Uptime % 60
				sm.cachedStatus.DeviceUptime = fmt.Sprintf("%dd %dh %dm %ds", days, hours, minutes, seconds)
			}

			// Update device signal (RSSI)
			if deviceStatus.RSSI != 0 {
				if deviceStatus.RSSI >= -60 {
					sm.cachedStatus.DeviceSignal = fmt.Sprintf("Excellent (%d)", deviceStatus.RSSI)
				} else if deviceStatus.RSSI >= -70 {
					sm.cachedStatus.DeviceSignal = fmt.Sprintf("Good (%d)", deviceStatus.RSSI)
				} else if deviceStatus.RSSI >= -80 {
					sm.cachedStatus.DeviceSignal = fmt.Sprintf("Fair (%d)", deviceStatus.RSSI)
				} else {
					sm.cachedStatus.DeviceSignal = fmt.Sprintf("Poor (%d)", deviceStatus.RSSI)
				}
				sm.cachedStatus.DeviceNetworkStatus = "Connected"
			}

			// Update sensor status
			if deviceStatus.SensorStatus == 0 {
				sm.cachedStatus.SensorStatus = "All OK"
			} else {
				sm.cachedStatus.SensorStatus = fmt.Sprintf("0x%X", deviceStatus.SensorStatus)
			}

			// Update device serial number
			if deviceStatus.SerialNumber != "" {
				sm.cachedStatus.DeviceSerialNumber = deviceStatus.SerialNumber
			}

			// Update device last observation time
			if deviceStatus.Timestamp > 0 {
				sm.cachedStatus.DeviceLastObs = time.Unix(deviceStatus.Timestamp, 0).Format("2006-01-02 15:04:05")
			}
		}

		if hubStatus != nil {
			// Update hub uptime
			if hubStatus.Uptime > 0 {
				days := hubStatus.Uptime / 86400
				hours := (hubStatus.Uptime % 86400) / 3600
				minutes := (hubStatus.Uptime % 3600) / 60
				seconds := hubStatus.Uptime % 60
				sm.cachedStatus.HubUptime = fmt.Sprintf("%dd %dh %dm %ds", days, hours, minutes, seconds)
			}

			// Update hub WiFi signal (RSSI)
			if hubStatus.RSSI != 0 {
				if hubStatus.RSSI >= -60 {
					sm.cachedStatus.HubWiFiSignal = fmt.Sprintf("Excellent (%d)", hubStatus.RSSI)
				} else if hubStatus.RSSI >= -70 {
					sm.cachedStatus.HubWiFiSignal = fmt.Sprintf("Good (%d)", hubStatus.RSSI)
				} else if hubStatus.RSSI >= -80 {
					sm.cachedStatus.HubWiFiSignal = fmt.Sprintf("Fair (%d)", hubStatus.RSSI)
				} else {
					sm.cachedStatus.HubWiFiSignal = fmt.Sprintf("Poor (%d)", hubStatus.RSSI)
				}
				sm.cachedStatus.HubNetworkStatus = "Connected"
			}

			// Update hub firmware
			if hubStatus.FirmwareRev != "" {
				sm.cachedStatus.HubFirmware = "v" + hubStatus.FirmwareRev
			}

			// Update hub serial number
			if hubStatus.SerialNumber != "" {
				sm.cachedStatus.HubSerialNumber = hubStatus.SerialNumber
			}

			// Update hub last status time
			if hubStatus.Timestamp > 0 {
				sm.cachedStatus.HubLastStatus = time.Unix(hubStatus.Timestamp, 0).Format("2006-01-02 15:04:05")
			}
		}

		// Update data source metadata
		sm.cachedStatus.DataSource = "udp"
		sm.cachedStatus.LastScraped = time.Now().UTC().Format(time.RFC3339)

		if sm.logLevel == "debug" {
			logger.Debug("Updated status from UDP - Battery: %s, DeviceUptime: %s, HubUptime: %s",
				sm.cachedStatus.BatteryVoltage, sm.cachedStatus.DeviceUptime, sm.cachedStatus.HubUptime)
		}
	}
}
