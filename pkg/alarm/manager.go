package alarm

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"tempest-homekit-go/pkg/logger"
	"tempest-homekit-go/pkg/weather"

	"github.com/fsnotify/fsnotify"
)

// Manager manages alarm evaluation and notifications.
//
// It owns the live [AlarmConfig] (whose Alarm values carry cooldown and
// change-detection state), an [Evaluator], and a [NotifierFactory] bound to
// the current credentials. When the config came from a file, it also watches
// that file with fsnotify and hot-reloads it. All methods are safe for
// concurrent use; mu guards config, notifierFactory, lastLoadTime and the
// location.
//
// Fields:
//   - config: the active configuration; replaced wholesale on reload.
//   - configPath: the watched file path, empty for inline JSON.
//   - lastLoadTime: when config was last (re)loaded.
//   - evaluator: shared stateless condition evaluator.
//   - notifierFactory: builds notifiers using config's credentials.
//   - watcher: fsnotify watcher on configPath's directory, nil if none.
//   - stationName: station name exposed to templates as {{station}}.
//   - latitude, longitude: station location passed to sun-based schedules
//     (0,0 until [Manager.SetLocation] is called).
//   - mu: guards the mutable fields above.
//   - stopChan: closed by [Manager.Stop] to end the watcher goroutine.
type Manager struct {
	config          *AlarmConfig
	configPath      string
	lastLoadTime    time.Time
	evaluator       *Evaluator
	notifierFactory *NotifierFactory
	watcher         *fsnotify.Watcher
	stationName     string
	latitude        float64 // Station latitude for sun calculations
	longitude       float64 // Station longitude for sun calculations
	mu              sync.RWMutex
	stopChan        chan struct{}
}

// NewManager creates a new alarm manager.
//
// configInput is either "@path" to a JSON file or an inline JSON string, as
// accepted by [LoadAlarmConfig]; stationName is used for the {{station}}
// template variable. For file input it starts watching the file's directory
// for changes; failure to set up the watcher is logged and not fatal. It logs
// the loaded alarms and warns (without failing) about channels whose provider
// credentials are missing. It returns an error only if the config cannot be
// loaded or validated.
func NewManager(configInput string, stationName string) (*Manager, error) {
	config, err := LoadAlarmConfig(configInput)
	if err != nil {
		return nil, err
	}

	m := &Manager{
		config:          config,
		evaluator:       NewEvaluator(),
		notifierFactory: NewNotifierFactory(config),
		stationName:     stationName,
		latitude:        0, // Will be set via SetLocation if available
		longitude:       0,
		stopChan:        make(chan struct{}),
		lastLoadTime:    time.Now(),
	}

	// If config is from file, set up file watching
	if strings.HasPrefix(configInput, "@") {
		m.configPath = strings.TrimPrefix(configInput, "@")
		if err := m.setupFileWatcher(); err != nil {
			logger.Error("Failed to set up file watcher: %v", err)
			// Non-fatal: continue without file watching
		}
	}

	logger.Info("Alarm manager initialized with %d alarms", len(config.Alarms))

	// Log active alarms
	enabledCount := 0
	for _, alarm := range config.Alarms {
		if alarm.Enabled {
			enabledCount++
			logger.Info("Loaded alarm: %s", alarm.Name)
			logger.Debug("  Condition: %s", alarm.Condition)
			logger.Debug("  Description: %s", alarm.Description)
			logger.Debug("  Cooldown: %ds", alarm.Cooldown)
			logger.Debug("  Channels: %d", len(alarm.Channels))
		} else {
			logger.Info("Loaded alarm (disabled): %s", alarm.Name)
		}
	}
	logger.Info("%d of %d alarms are enabled", enabledCount, len(config.Alarms))

	// Output pretty-formatted JSON of all alarms at debug level
	if prettyJSON, err := json.MarshalIndent(config.Alarms, "", "  "); err == nil {
		logger.Debug("Alarm configuration JSON:\n%s", string(prettyJSON))
	}

	// Validate that required provider configuration is present
	validateConfigProviders(config)

	return m, nil
}

// validateConfigProviders checks that required environment variables are set for delivery methods.
//
// It looks only at enabled alarms, and only for email and sms channels, and
// logs a warning listing missing variables for the configured provider. It
// never fails, so a misconfigured provider surfaces as a send error later.
func validateConfigProviders(config *AlarmConfig) {
	// Track which delivery methods are used
	usesEmail := false
	usesSMS := false

	for _, alarm := range config.Alarms {
		if !alarm.Enabled {
			continue
		}
		for _, channel := range alarm.Channels {
			switch channel.Type {
			case "email":
				usesEmail = true
			case "sms":
				usesSMS = true
			}
		}
	}

	// Validate email configuration
	if usesEmail {
		if config.Email == nil {
			logger.Info("⚠️  Email delivery is configured in alarms, but no email provider is configured.")
			logger.Info("    Set either SMTP_* or MS365_* environment variables in .env file.")
			logger.Info("    For SMTP: SMTP_HOST, SMTP_PORT, SMTP_USERNAME, SMTP_PASSWORD, SMTP_FROM_ADDRESS")
			logger.Info("    For MS365: MS365_CLIENT_ID, MS365_CLIENT_SECRET, MS365_TENANT_ID, MS365_FROM_ADDRESS")
		} else {
			// Validate specific provider requirements
			switch config.Email.Provider {
			case "smtp":
				missing := []string{}
				if config.Email.SMTPHost == "" {
					missing = append(missing, "SMTP_HOST")
				}
				if config.Email.Username == "" {
					missing = append(missing, "SMTP_USERNAME")
				}
				if config.Email.Password == "" {
					missing = append(missing, "SMTP_PASSWORD")
				}
				if config.Email.FromAddress == "" {
					missing = append(missing, "SMTP_FROM_ADDRESS")
				}
				if len(missing) > 0 {
					logger.Info("⚠️  SMTP email is configured but missing required environment variables: %s", strings.Join(missing, ", "))
				}
			case "microsoft365":
				missing := []string{}
				if config.Email.ClientID == "" {
					missing = append(missing, "MS365_CLIENT_ID")
				}
				if config.Email.ClientSecret == "" {
					missing = append(missing, "MS365_CLIENT_SECRET")
				}
				if config.Email.TenantID == "" {
					missing = append(missing, "MS365_TENANT_ID")
				}
				if config.Email.FromAddress == "" {
					missing = append(missing, "MS365_FROM_ADDRESS")
				}
				if len(missing) > 0 {
					logger.Info("⚠️  Microsoft 365 email is configured but missing required environment variables: %s", strings.Join(missing, ", "))
				}
			}
		}
	}

	// Validate SMS configuration
	if usesSMS {
		if config.SMS == nil {
			logger.Info("⚠️  SMS delivery is configured in alarms, but no SMS provider is configured.")
			logger.Info("    Set either TWILIO_* or AWS_* environment variables in .env file.")
			logger.Info("    For Twilio: TWILIO_ACCOUNT_SID, TWILIO_AUTH_TOKEN, TWILIO_FROM_NUMBER")
			logger.Info("    For AWS SNS: AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_REGION, AWS_SNS_TOPIC_ARN")
		} else {
			// Validate specific provider requirements
			switch config.SMS.Provider {
			case "twilio":
				missing := []string{}
				if config.SMS.AccountSID == "" {
					missing = append(missing, "TWILIO_ACCOUNT_SID")
				}
				if config.SMS.AuthToken == "" {
					missing = append(missing, "TWILIO_AUTH_TOKEN")
				}
				if config.SMS.FromNumber == "" {
					missing = append(missing, "TWILIO_FROM_NUMBER")
				}
				if len(missing) > 0 {
					logger.Info("⚠️  Twilio SMS is configured but missing required environment variables: %s", strings.Join(missing, ", "))
				}
			case "aws_sns":
				missing := []string{}
				if config.SMS.AWSAccessKey == "" {
					missing = append(missing, "AWS_ACCESS_KEY_ID")
				}
				if config.SMS.AWSSecretKey == "" {
					missing = append(missing, "AWS_SECRET_ACCESS_KEY")
				}
				if config.SMS.AWSRegion == "" {
					missing = append(missing, "AWS_REGION")
				}
				if len(missing) > 0 {
					logger.Info("⚠️  AWS SNS is configured but missing required environment variables: %s", strings.Join(missing, ", "))
				}
			}
		}
	}
}

// setupFileWatcher sets up cross-platform file watching for alarm config.
//
// It watches the file's directory rather than the file itself so that editors
// which save by replacing the file (rename/create) are still seen, then starts
// watchConfigFile in a goroutine. It returns an error if the watcher cannot be
// created or the directory cannot be watched.
func (m *Manager) setupFileWatcher() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create file watcher: %w", err)
	}

	m.watcher = watcher

	// Watch the config file directory (not the file directly, for better compatibility)
	configDir := filepath.Dir(m.configPath)
	if err := watcher.Add(configDir); err != nil {
		if cerr := watcher.Close(); cerr != nil {
			logger.Debug("failed to close watcher after Add error: %v", cerr)
		}
		return fmt.Errorf("failed to watch config directory: %w", err)
	}

	// Start watching in background
	go m.watchConfigFile()

	logger.Info("Watching alarm config file for changes: %s", m.configPath)
	return nil
}

// watchConfigFile monitors for config file changes.
//
// It runs until stopChan is closed or the watcher's channels close, reloading
// the config on every Write or Create event for the config's base name. There
// is no debouncing, so one save can trigger several reloads. Reload failures
// are logged and the previous config stays active.
func (m *Manager) watchConfigFile() {
	configFileName := filepath.Base(m.configPath)

	for {
		select {
		case <-m.stopChan:
			return
		case event, ok := <-m.watcher.Events:
			if !ok {
				return
			}

			// Check if this event is for our config file
			if filepath.Base(event.Name) != configFileName {
				continue
			}

			if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
				logger.Info("Alarm config file changed, reloading: %s", m.configPath)
				if err := m.reloadConfig(); err != nil {
					logger.Error("Failed to reload alarm config: %v", err)
				} else {
					logger.Info("Alarm config reloaded successfully")
				}
			}
		case err, ok := <-m.watcher.Errors:
			if !ok {
				return
			}
			logger.Error("File watcher error: %v", err)
		}
	}
}

// reloadConfig reloads the alarm configuration from file.
//
// It re-reads and validates configPath, re-applies credentials from the
// environment, and swaps in the new config and a new notifier factory under
// the lock. The new Alarm values start fresh, so cooldown timers,
// change-detection baselines and TriggeredCount are reset by a reload. On any
// read, parse or validation error the current config is kept and the error
// returned.
func (m *Manager) reloadConfig() error {
	data, err := os.ReadFile(m.configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	var newConfig AlarmConfig
	if err := json.Unmarshal(data, &newConfig); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	// Load credentials from environment variables (ONLY source)
	envConfig, _ := LoadConfigFromEnv()
	newConfig.Email = envConfig.Email
	newConfig.SMS = envConfig.SMS
	newConfig.Syslog = envConfig.Syslog

	if err := newConfig.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	m.mu.Lock()
	m.config = &newConfig
	m.notifierFactory = NewNotifierFactory(&newConfig)
	m.lastLoadTime = time.Now()
	m.mu.Unlock()

	// Log detailed information about the reloaded alarms (same as initial load)
	logger.Info("Alarm manager initialized with %d alarms", len(newConfig.Alarms))

	enabledCount := 0
	for _, alarm := range newConfig.Alarms {
		if alarm.Enabled {
			enabledCount++
			logger.Info("Loaded alarm: %s", alarm.Name)
			logger.Debug("  Condition: %s", alarm.Condition)
			logger.Debug("  Description: %s", alarm.Description)
			logger.Debug("  Cooldown: %ds", alarm.Cooldown)
			logger.Debug("  Channels: %d", len(alarm.Channels))
		} else {
			logger.Info("Loaded alarm (disabled): %s", alarm.Name)
		}
	}
	logger.Info("%d of %d alarms are enabled", enabledCount, len(newConfig.Alarms))

	// Output pretty-formatted JSON of all alarms at debug level
	if prettyJSON, err := json.MarshalIndent(newConfig.Alarms, "", "  "); err == nil {
		logger.Debug("Alarm configuration JSON:\n%s", string(prettyJSON))
	}

	// Validate that required provider configuration is present
	validateConfigProviders(&newConfig)

	return nil
}

// ProcessObservation evaluates all alarms against a new weather observation.
//
// For each enabled alarm that is inside its schedule and out of cooldown, it
// evaluates the condition and, if true, sends notifications on every channel,
// increments TriggeredCount and starts the cooldown. Afterwards it records the
// observation's values as the alarm's previous values for change detection.
// Evaluation and delivery errors are logged, not returned. A nil obs is
// ignored. The manager lock is held for the whole call, including network
// delivery, so slow channels delay other callers.
func (m *Manager) ProcessObservation(obs *weather.Observation) {
	if obs == nil {
		return
	}

	// Work with the original alarms directly to preserve state (previousValue map)
	// We lock for the entire duration to ensure consistent state
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.config.Alarms {
		alarm := &m.config.Alarms[i]

		if !alarm.Enabled {
			logger.Debug("Skipping disabled alarm: %s", alarm.Name)
			continue
		}

		// Check if alarm is within its schedule
		if alarm.Schedule != nil {
			now := time.Now()
			if !alarm.Schedule.IsActive(now, m.latitude, m.longitude) {
				logger.Debug("Alarm %s outside scheduled time: %s", alarm.Name, alarm.Schedule.String())
				continue
			}
		}

		if !alarm.CanFire() {
			logger.Debug("Alarm %s in cooldown, skipping (last fired: %v)", alarm.Name, alarm.lastFired)
			continue
		}

		logger.Debug("Evaluating alarm: '%s'", alarm.Name)
		logger.Debug("  Condition: %s", alarm.Condition)
		logger.Debug("  Current observation: temp=%.1f°C, humidity=%.0f%%, pressure=%.2f, wind=%.1fm/s, lux=%.0f, uv=%d",
			obs.AirTemperature, obs.RelativeHumidity, obs.StationPressure, obs.WindAvg, obs.Illuminance, obs.UV)

		// Evaluate condition (pass alarm for change detection support)
		triggered, err := m.evaluator.EvaluateWithAlarm(alarm.Condition, obs, alarm)
		if err != nil {
			logger.Error("Failed to evaluate alarm %s: %v", alarm.Name, err)
			continue
		}

		logger.Debug("  Result: %v", triggered)

		if triggered {
			logger.Info("🚨 Alarm triggered: %s (condition: %s)", alarm.Name, alarm.Condition)
			m.sendNotifications(alarm, obs)
			// Increment triggered count and mark as fired
			alarm.TriggeredCount++
			alarm.MarkFired()
		}

		// Store all sensor values for next evaluation
		// This happens after evaluation so they become "previous" values on next run
		alarm.SetPreviousValue("temperature", obs.AirTemperature)
		alarm.SetPreviousValue("humidity", obs.RelativeHumidity)
		alarm.SetPreviousValue("pressure", obs.StationPressure)
		alarm.SetPreviousValue("wind_speed", obs.WindAvg)
		alarm.SetPreviousValue("wind_gust", obs.WindGust)
		alarm.SetPreviousValue("wind_direction", obs.WindDirection)
		alarm.SetPreviousValue("uv", float64(obs.UV))
		alarm.SetPreviousValue("lux", obs.Illuminance)
		alarm.SetPreviousValue("rain_rate", obs.RainAccumulated)
		alarm.SetPreviousValue("rain_daily", obs.RainDailyTotal)
		alarm.SetPreviousValue("lightning_count", float64(obs.LightningStrikeCount))
	}
}

// sendNotifications sends notifications through all configured channels for an alarm.
// Each channel is attempted independently; failures are logged and do not stop
// the remaining channels. Callers must hold m.mu.
func (m *Manager) sendNotifications(alarm *Alarm, obs *weather.Observation) {
	logger.Debug("Sending notifications for alarm '%s' through %d channels", alarm.Name, len(alarm.Channels))
	for i := range alarm.Channels {
		channel := &alarm.Channels[i]
		logger.Debug("Processing channel %d: type=%s", i, channel.Type)

		notifier, err := m.notifierFactory.GetNotifier(channel.Type)
		if err != nil {
			logger.Error("Failed to get notifier for %s: %v", channel.Type, err)
			continue
		}

		logger.Debug("Attempting to send %s notification for alarm %s", channel.Type, alarm.Name)
		if err := notifier.Send(alarm, channel, obs, m.stationName); err != nil {
			logger.Error("Failed to send %s notification for alarm %s: %v",
				channel.Type, alarm.Name, err)
		} else {
			logger.Info("Sent %s notification for alarm %s", channel.Type, alarm.Name)
		}
	}
	logger.Debug("Finished sending notifications for alarm '%s'", alarm.Name)
}

// Stop stops the alarm manager and file watcher. It must be called at most
// once: a second call panics on closing stopChan again.
func (m *Manager) Stop() {
	close(m.stopChan)
	if m.watcher != nil {
		if err := m.watcher.Close(); err != nil {
			logger.Debug("failed to close watcher: %v", err)
		}
	}
	logger.Info("Alarm manager stopped")
}

// GetConfig returns a copy of the current alarm configuration.
// The copy is shallow: the Alarms slice and the credential pointers are shared
// with the live config, so callers must treat it as read-only.
func (m *Manager) GetConfig() *AlarmConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy to prevent external modifications
	configCopy := *m.config
	return &configCopy
}

// GetAlarmCount returns the number of configured alarms, enabled or not.
func (m *Manager) GetAlarmCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.config.Alarms)
}

// GetEnabledAlarmCount returns the number of enabled alarms.
func (m *Manager) GetEnabledAlarmCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, alarm := range m.config.Alarms {
		if alarm.Enabled {
			count++
		}
	}
	return count
}

// GetConfigPath returns the alarm configuration file path, or the literal
// "Inline configuration" when the config was given as a JSON string.
func (m *Manager) GetConfigPath() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.configPath == "" {
		return "Inline configuration"
	}
	return m.configPath
}

// GetLastLoadTime returns when the configuration was last loaded or reloaded.
func (m *Manager) GetLastLoadTime() time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastLoadTime
}

// SetLocation sets the geographic location for sunrise/sunset calculations in schedules.
// latitude and longitude are in decimal degrees; until it is called the
// location is 0,0, which sun schedules without their own coordinates treat
// as unknown (always active).
func (m *Manager) SetLocation(latitude, longitude float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.latitude = latitude
	m.longitude = longitude
	logger.Debug("Alarm manager location set to: lat=%.4f, lon=%.4f", latitude, longitude)
}

// GetLocation returns the current location settings in decimal degrees.
func (m *Manager) GetLocation() (latitude, longitude float64) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.latitude, m.longitude
}
