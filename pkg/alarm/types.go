// Package alarm implements rule-based weather alerting for the Tempest HomeKit
// bridge.
//
// An [AlarmConfig] holds a list of [Alarm] rules loaded from a JSON file
// ("@path.json") or an inline JSON string by [LoadAlarmConfig]. Each rule has a
// condition string (for example "temperature > 85F && humidity > 80", or the
// change-detection forms "*lightning_count", ">rain_rate" and
// "<lightning_distance"), an optional [Schedule] restricting when it is active,
// a cooldown, and one or more [Channel] destinations. The [Manager] evaluates
// every enabled rule against each incoming weather observation with an
// [Evaluator] and, when a rule fires, expands the channel's {{placeholder}}
// template and delivers it through the matching [Notifier] (console, syslog,
// oslog, eventlog, email, sms, webhook, csv or json).
//
// Delivery credentials (SMTP, Microsoft 365, Twilio, AWS SNS, syslog) are never
// read from the alarm JSON; they come only from environment variables via
// [LoadConfigFromEnv], so a rules file can be shared or committed without
// secrets.
//
// The Test*Configuration and Run*Test functions are command-line self-tests
// that push one message through a single channel type using the same
// [NotifierFactory] path as real alarms.
package alarm

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// AlarmConfig represents the alarm system configuration.
// This structure contains ONLY alarm rules. All credentials (email, SMS, syslog)
// are loaded exclusively from .env file via environment variables.
// See LoadConfigFromEnv() for credential loading from SMTP_*, MS365_*, TWILIO_*, AWS_*, SYSLOG_*
//
// The Email, SMS and Syslog fields are tagged `json:"-"`, so they are neither
// read from nor written to the rules JSON; [LoadAlarmConfig] and the manager's
// reload path fill them from [LoadConfigFromEnv] after parsing.
type AlarmConfig struct {
	// Alarms is the list of alarm rules. An empty list is valid so that a
	// [Manager] can start and wait for the watched file to gain rules.
	Alarms []Alarm `json:"alarms"`

	// Email holds the global email provider settings (loaded from .env, not JSON);
	// nil when no email provider is configured.
	Email *EmailGlobalConfig `json:"-"`
	// SMS holds the global SMS provider settings (loaded from .env, not JSON); nil
	// when no SMS provider is configured.
	SMS *SMSGlobalConfig `json:"-"`
	// Syslog holds remote syslog settings (loaded from .env, not JSON); nil means
	// the local syslog daemon with default priority.
	Syslog *SyslogConfig `json:"-"`
}

// EmailGlobalConfig contains global email configuration: the provider and its
// credentials. It is populated only by [LoadConfigFromEnv]; the JSON tags
// describe its shape but the alarm rules file never supplies it.
//
// Fields:
//   - Provider: "smtp" or "microsoft365" (EmailNotifier also accepts the
//     aliases "o365" and "exchange").
//   - SMTPHost, SMTPPort, Username, Password: SMTP server and PLAIN-auth
//     credentials (SMTP_HOST, SMTP_PORT default 587, SMTP_USERNAME,
//     SMTP_PASSWORD). Port 465 means implicit TLS; other ports use STARTTLS
//     when UseTLS is set.
//   - FromAddress: sender address. For Microsoft 365 it must also be a user
//     principal name in the tenant, because mail is sent as that user through
//     Microsoft Graph.
//   - FromName: optional display name for the From header (SMTP_FROM_NAME,
//     used by both providers).
//   - UseTLS: enable TLS for SMTP (SMTP_USE_TLS; true unless "false").
//   - UseOAuth2: send Microsoft 365 mail through Graph; without it the
//     Microsoft 365 provider falls back to SMTP.
//   - ClientID, ClientSecret, TenantID: Azure AD app registration
//     credentials (MS365_CLIENT_ID, MS365_CLIENT_SECRET, MS365_TENANT_ID).
type EmailGlobalConfig struct {
	Provider     string `json:"provider"` // "smtp", "microsoft365"
	SMTPHost     string `json:"smtp_host,omitempty"`
	SMTPPort     int    `json:"smtp_port,omitempty"`
	Username     string `json:"username,omitempty"`
	Password     string `json:"password,omitempty"`
	FromAddress  string `json:"from_address"`
	FromName     string `json:"from_name,omitempty"`
	UseTLS       bool   `json:"use_tls"`
	UseOAuth2    bool   `json:"use_oauth2,omitempty"`
	ClientID     string `json:"client_id,omitempty"`
	ClientSecret string `json:"client_secret,omitempty"`
	TenantID     string `json:"tenant_id,omitempty"`
}

// SMSGlobalConfig contains global SMS configuration: the provider and its
// credentials. Like [EmailGlobalConfig] it is populated only from environment
// variables by [LoadConfigFromEnv].
//
// Fields:
//   - Provider: "twilio" or "aws_sns" (SMSNotifier also accepts the aliases
//     "sns" and "aws").
//   - AccountSID, AuthToken, FromNumber: Twilio credentials and sending
//     number (TWILIO_ACCOUNT_SID, TWILIO_AUTH_TOKEN, TWILIO_FROM_NUMBER).
//   - AWSAccessKey, AWSSecretKey, AWSRegion: static AWS credentials and
//     region for SNS (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_REGION).
//   - AWSSNSTopicARN: when set, messages are published to this SNS topic
//     instead of being sent directly to each phone number
//     (AWS_SNS_TOPIC_ARN).
type SMSGlobalConfig struct {
	Provider       string `json:"provider"` // "twilio", "aws_sns"
	AccountSID     string `json:"account_sid,omitempty"`
	AuthToken      string `json:"auth_token,omitempty"`
	FromNumber     string `json:"from_number,omitempty"`
	AWSAccessKey   string `json:"aws_access_key,omitempty"`
	AWSSecretKey   string `json:"aws_secret_key,omitempty"`
	AWSRegion      string `json:"aws_region,omitempty"`
	AWSSNSTopicARN string `json:"aws_sns_topic_arn,omitempty"`
}

// SyslogConfig contains syslog configuration, populated from the SYSLOG_*
// environment variables by [LoadConfigFromEnv].
//
// SyslogNotifier dials a remote server only when both Network and Address are
// set; otherwise it writes to the local syslog daemon. Priority selects the
// severity name ("info", "warning" or "error", default "warning") and Tag the
// syslog tag for remote messages (default "tempest-weather").
type SyslogConfig struct {
	Network  string `json:"network,omitempty"`  // "tcp", "udp", "" for local
	Address  string `json:"address,omitempty"`  // "localhost:514" or empty for local
	Priority string `json:"priority,omitempty"` // "info", "warning", "error"
	Tag      string `json:"tag,omitempty"`
}

// Alarm represents a single alarm rule: a condition evaluated against each
// observation, the channels to notify when it holds, and the runtime state
// (cooldown timestamp, previous field values) the rule carries between
// evaluations.
//
// Fields:
//   - Name: unique identifier within a config; required.
//   - Description: free text, exposed to templates as {{alarm_description}}.
//   - Tags: free-form labels shown in {{alarm_info}}.
//   - Enabled: whether the alarm is evaluated at all.
//   - Condition: expression evaluated by [Evaluator.EvaluateWithAlarm];
//     required.
//   - Cooldown: minimum seconds between notifications; 0 means none.
//   - Schedule: optional window in which the alarm is evaluated; nil means
//     always.
//   - Channels: notification destinations; at least one is required.
//   - TriggeredCount: number of triggers since process start (or since the
//     last config reload, which replaces the Alarm values).
//
// The unexported fields are runtime state that is not serialized: lastFired
// drives the cooldown, previousValue is the per-field baseline for
// change-detection operators and the "Last" column in notifications, and
// triggerContext records the value a change-detection condition compared
// against when it fired (preferred over previousValue for {{last_*}}).
type Alarm struct {
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	Enabled     bool      `json:"enabled"`
	Condition   string    `json:"condition"`          // e.g., "temperature > 85", "humidity > 80 && temperature > 35", "*lightning_count"
	Cooldown    int       `json:"cooldown,omitempty"` // Seconds between repeated notifications
	Schedule    *Schedule `json:"schedule,omitempty"` // Optional schedule defining when alarm is active
	Channels    []Channel `json:"channels"`
	// TriggeredCount tracks how many times this alarm has been triggered since process start
	TriggeredCount int                `json:"triggered_count,omitempty"`
	lastFired      time.Time          // Internal: last trigger time
	previousValue  map[string]float64 // Internal: previous field values for change detection
	triggerContext map[string]float64 // Internal: field values at time of trigger (for notification display)
}

// Channel represents a notification channel: a destination Type plus the
// type-specific settings and message template. Only the sub-config matching
// Type is used.
//
// Fields:
//   - Type: one of console, email, sms, syslog, oslog, eventlog, webhook,
//     csv or json.
//   - Template: message template for console, syslog, oslog and eventlog
//     (required for those), and the fallback email body when Email.Body is
//     empty.
//   - Email, SMS, Webhook, CSV, JSON: settings for the channel of that type;
//     the matching one is required by [Channel.Validate].
type Channel struct {
	Type     string         `json:"type"`
	Template string         `json:"template,omitempty"`
	Email    *EmailConfig   `json:"email,omitempty"`
	SMS      *SMSConfig     `json:"sms,omitempty"`
	Webhook  *WebhookConfig `json:"webhook,omitempty"`
	CSV      *CSVConfig     `json:"csv,omitempty"`
	JSON     *JSONConfig    `json:"json,omitempty"`
}

// EmailConfig holds email-specific configuration for a channel.
//
// Subject and Body are templates expanded per notification; both are required
// by [Channel.Validate]. To lists the header and envelope recipients (at least
// one required), CC adds Cc header recipients, and BCC recipients are added to
// the envelope only. Html sends the body as text/html instead of text/plain.
type EmailConfig struct {
	Subject string   `json:"subject,omitempty"`
	Body    string   `json:"body,omitempty"`
	To      []string `json:"to,omitempty"`
	CC      []string `json:"cc,omitempty"`
	BCC     []string `json:"bcc,omitempty"`
	Html    bool     `json:"html,omitempty"`
}

// SMSConfig holds SMS-specific configuration for a channel.
//
// Message is the SMS text template and To the recipient phone numbers in E.164
// form ("+15551234567"); both are required by [Channel.Validate].
type SMSConfig struct {
	Message string   `json:"message,omitempty"`
	To      []string `json:"to,omitempty"`
}

// WebhookConfig holds webhook-specific configuration for a channel.
//
// URL and the Body template are required. Method defaults to POST and
// ContentType to application/json (both defaults are filled in by
// [Channel.Validate]). Headers are applied after Content-Type, so a
// "Content-Type" entry there overrides ContentType.
type WebhookConfig struct {
	URL         string            `json:"url,omitempty"`
	Method      string            `json:"method,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Body        string            `json:"body,omitempty"`
	ContentType string            `json:"content_type,omitempty"`
}

// CSVConfig holds CSV file-specific configuration for a channel.
//
// Path is the file to append to (required). MaxDays rotates the file to a
// timestamped .bak once its modification time is older than that many days;
// 0 disables rotation and negative values are rejected. Message is the row
// template; a template containing commas is split into columns, and
// [Channel.Validate] supplies a default multi-column template when empty.
type CSVConfig struct {
	Path    string `json:"path,omitempty"`
	MaxDays int    `json:"max_days,omitempty"`
	Message string `json:"message,omitempty"`
}

// JSONConfig holds JSON file-specific configuration for a channel.
//
// Path is the JSON array file to append to (required). MaxDays rotates the
// file as for [CSVConfig]. Message is the record template; its expanded text is
// stored as the "message" string of each array element, and [Channel.Validate]
// supplies a default when empty.
type JSONConfig struct {
	Path    string `json:"path,omitempty"`
	MaxDays int    `json:"max_days,omitempty"`
	Message string `json:"message,omitempty"`
}

// LoadConfigFromEnv loads email/SMS configuration from environment variables.
// All credentials must be explicitly set in .env file - no fallback to OS credentials.
// For AWS SNS: Requires AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_REGION in .env
// (Does NOT use ~/.aws/credentials or IAM roles)
//
// Provider precedence: MS365_CLIENT_ID selects Microsoft 365 over SMTP_HOST,
// and TWILIO_ACCOUNT_SID selects Twilio over AWS_ACCESS_KEY_ID. Syslog is
// configured only when SYSLOG_ADDRESS is set, with priority defaulting to
// "warning" and tag to "tempest-weather". A section whose selecting variable
// is unset is left nil, and an unparsable SMTP_PORT silently falls back to
// 587. The returned config has no Alarms; the error is currently always nil.
func LoadConfigFromEnv() (*AlarmConfig, error) {
	config := &AlarmConfig{}

	// Load email configuration from environment
	if clientID := os.Getenv("MS365_CLIENT_ID"); clientID != "" {
		// Microsoft 365 OAuth2
		config.Email = &EmailGlobalConfig{
			Provider:     "microsoft365",
			UseOAuth2:    true,
			ClientID:     clientID,
			ClientSecret: os.Getenv("MS365_CLIENT_SECRET"),
			TenantID:     os.Getenv("MS365_TENANT_ID"),
			FromAddress:  os.Getenv("MS365_FROM_ADDRESS"),
			FromName:     os.Getenv("SMTP_FROM_NAME"), // Can use same FROM_NAME
		}
	} else if smtpHost := os.Getenv("SMTP_HOST"); smtpHost != "" {
		// Generic SMTP
		smtpPort := 587 // default
		if portStr := os.Getenv("SMTP_PORT"); portStr != "" {
			if port, err := strconv.Atoi(portStr); err == nil {
				smtpPort = port
			}
		}
		useTLS := true // default
		if tlsStr := os.Getenv("SMTP_USE_TLS"); tlsStr == "false" {
			useTLS = false
		}
		config.Email = &EmailGlobalConfig{
			Provider:    "smtp",
			SMTPHost:    smtpHost,
			SMTPPort:    smtpPort,
			Username:    os.Getenv("SMTP_USERNAME"),
			Password:    os.Getenv("SMTP_PASSWORD"),
			FromAddress: os.Getenv("SMTP_FROM_ADDRESS"),
			FromName:    os.Getenv("SMTP_FROM_NAME"),
			UseTLS:      useTLS,
		}
	}

	// Load SMS configuration from environment
	if twilioSID := os.Getenv("TWILIO_ACCOUNT_SID"); twilioSID != "" {
		// Twilio
		config.SMS = &SMSGlobalConfig{
			Provider:   "twilio",
			AccountSID: twilioSID,
			AuthToken:  os.Getenv("TWILIO_AUTH_TOKEN"),
			FromNumber: os.Getenv("TWILIO_FROM_NUMBER"),
		}
	} else if awsKey := os.Getenv("AWS_ACCESS_KEY_ID"); awsKey != "" {
		// AWS SNS
		config.SMS = &SMSGlobalConfig{
			Provider:       "aws_sns",
			AWSAccessKey:   awsKey,
			AWSSecretKey:   os.Getenv("AWS_SECRET_ACCESS_KEY"),
			AWSRegion:      os.Getenv("AWS_REGION"),
			AWSSNSTopicARN: os.Getenv("AWS_SNS_TOPIC_ARN"),
		}
	}

	// Load syslog configuration from environment (optional)
	if syslogAddr := os.Getenv("SYSLOG_ADDRESS"); syslogAddr != "" {
		config.Syslog = &SyslogConfig{
			Network:  os.Getenv("SYSLOG_NETWORK"),
			Address:  syslogAddr,
			Priority: os.Getenv("SYSLOG_PRIORITY"),
			Tag:      os.Getenv("SYSLOG_TAG"),
		}
		if config.Syslog.Priority == "" {
			config.Syslog.Priority = "warning"
		}
		if config.Syslog.Tag == "" {
			config.Syslog.Tag = "tempest-weather"
		}
	}

	return config, nil
}

// LoadAlarmConfig loads alarm configuration from file or JSON string.
//
// An input starting with "@" is treated as a file path ("@alarms.json");
// anything else must be inline JSON. Credentials are then taken from
// [LoadConfigFromEnv], overwriting anything the JSON might have held, and the
// result is checked with [AlarmConfig.Validate] (which also fills channel
// defaults).
//
// It returns an error if the file cannot be read, if inline input is not valid
// JSON (reporting line and column, and hinting at a missing "@" when the input
// looks like a path), if the JSON does not match AlarmConfig, or if validation
// fails.
func LoadAlarmConfig(input string) (*AlarmConfig, error) {
	var data []byte
	var err error
	isFile := false

	// Check if input is a file reference (@filename.json)
	if strings.HasPrefix(input, "@") {
		isFile = true
		filename := strings.TrimPrefix(input, "@")
		data, err = os.ReadFile(filename)
		if err != nil {
			return nil, fmt.Errorf("failed to read alarm config file %s: %w", filename, err)
		}
	} else {
		// Treat as inline JSON string - first validate it's JSON
		data = []byte(input)

		// Test if the string is valid JSON before attempting to parse into AlarmConfig
		var jsonTest interface{}
		if err := json.Unmarshal(data, &jsonTest); err != nil {
			// Detect if they forgot the @ prefix
			if !strings.HasPrefix(input, "{") && (strings.HasSuffix(input, ".json") || strings.Contains(input, "/")) {
				return nil, fmt.Errorf("invalid JSON string: %w\nHint: Did you mean to use '@%s'? File paths must be prefixed with @", err, input)
			}

			// Provide detailed error for invalid JSON
			if syntaxErr, ok := err.(*json.SyntaxError); ok {
				// Calculate line and column
				lines := strings.Split(input, "\n")
				var line, col int
				offset := syntaxErr.Offset
				currentOffset := int64(0)

				for i, l := range lines {
					lineLen := int64(len(l) + 1) // +1 for newline
					if currentOffset+lineLen > offset {
						line = i + 1
						col = int(offset - currentOffset + 1)
						break
					}
					currentOffset += lineLen
				}

				if line == 0 {
					line = 1
					col = int(offset)
				}

				return nil, fmt.Errorf("invalid JSON syntax at line %d, column %d: %v\nProvide valid JSON string or use @filename.json to load from file", line, col, syntaxErr)
			}

			return nil, fmt.Errorf("invalid JSON string: %w\nProvide valid JSON string or use @filename.json to load from file", err)
		}
	}

	var config AlarmConfig
	if err := json.Unmarshal(data, &config); err != nil {
		if isFile {
			return nil, fmt.Errorf("failed to parse alarm config from file: %w", err)
		}
		return nil, fmt.Errorf("failed to parse alarm config from JSON string: %w\nEnsure your JSON matches the AlarmConfig structure", err)
	}

	// Load credentials from environment variables (ONLY source for Email/SMS/Syslog config)
	// JSON files should contain ONLY alarm rules, never credentials
	envConfig, _ := LoadConfigFromEnv()
	config.Email = envConfig.Email
	config.SMS = envConfig.SMS
	config.Syslog = envConfig.Syslog

	// Validate config
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid alarm config: %w", err)
	}

	return &config, nil
}

// Validate checks if the alarm configuration is valid.
//
// Every alarm needs a unique non-empty name, a condition, a valid schedule (if
// any) and at least one valid channel. An empty alarm list is accepted. The
// error names the offending alarm (and channel index). Because
// [Channel.Validate] fills defaults through the channel's sub-config pointers,
// a successful Validate also applies those defaults to c.
func (c *AlarmConfig) Validate() error {
	// Allow empty alarm list - manager can start and watch for file changes
	if len(c.Alarms) == 0 {
		return nil
	}

	names := make(map[string]bool)
	for i, alarm := range c.Alarms {
		if alarm.Name == "" {
			return fmt.Errorf("alarm at index %d: name is required", i)
		}
		if names[alarm.Name] {
			return fmt.Errorf("duplicate alarm name: %s", alarm.Name)
		}
		names[alarm.Name] = true

		if alarm.Condition == "" {
			return fmt.Errorf("alarm %s: condition is required", alarm.Name)
		}

		// Validate schedule if present
		if alarm.Schedule != nil {
			if err := alarm.Schedule.Validate(); err != nil {
				return fmt.Errorf("alarm %s: invalid schedule: %w", alarm.Name, err)
			}
		}

		if len(alarm.Channels) == 0 {
			return fmt.Errorf("alarm %s: at least one channel is required", alarm.Name)
		}

		for j, channel := range alarm.Channels {
			if err := channel.Validate(); err != nil {
				return fmt.Errorf("alarm %s, channel %d: %w", alarm.Name, j, err)
			}
		}
	}

	return nil
}

// Validate checks if a channel configuration is valid.
//
// It rejects unknown types and missing required fields for the channel's type.
// It is not read-only: it fills defaults for webhook Method (POST) and
// ContentType (application/json), and for an empty CSV or JSON Message
// template. Note the error text for an unknown type does not list csv and
// json, although both are accepted.
func (c *Channel) Validate() error {
	validTypes := map[string]bool{
		"console":  true,
		"email":    true,
		"sms":      true,
		"syslog":   true,
		"oslog":    true,
		"eventlog": true,
		"webhook":  true,
		"csv":      true,
		"json":     true,
	}

	if !validTypes[c.Type] {
		return fmt.Errorf("invalid channel type: %s (must be console, email, sms, syslog, oslog, eventlog, or webhook)", c.Type)
	}

	switch c.Type {
	case "console", "syslog", "oslog", "eventlog":
		if c.Template == "" {
			return fmt.Errorf("template is required for %s channel", c.Type)
		}
	case "email":
		if c.Email == nil {
			return fmt.Errorf("email configuration is required for email channel")
		}
		if len(c.Email.To) == 0 {
			return fmt.Errorf("at least one recipient is required for email channel")
		}
		if c.Email.Subject == "" {
			return fmt.Errorf("subject is required for email channel")
		}
		if c.Email.Body == "" {
			return fmt.Errorf("body template is required for email channel")
		}
	case "sms":
		if c.SMS == nil {
			return fmt.Errorf("sms configuration is required for sms channel")
		}
		if len(c.SMS.To) == 0 {
			return fmt.Errorf("at least one phone number is required for sms channel")
		}
		if c.SMS.Message == "" {
			return fmt.Errorf("message template is required for sms channel")
		}
	case "webhook":
		if c.Webhook == nil {
			return fmt.Errorf("webhook configuration is required for webhook channel")
		}
		if c.Webhook.URL == "" {
			return fmt.Errorf("url is required for webhook channel")
		}
		if c.Webhook.Body == "" {
			return fmt.Errorf("body template is required for webhook channel")
		}
		if c.Webhook.Method == "" {
			c.Webhook.Method = "POST" // Default to POST
		}
		if c.Webhook.ContentType == "" {
			c.Webhook.ContentType = "application/json" // Default content type
		}
	case "csv":
		if c.CSV == nil {
			return fmt.Errorf("csv configuration is required for csv channel")
		}
		if c.CSV.Path == "" {
			return fmt.Errorf("path is required for csv channel")
		}
		if c.CSV.MaxDays < 0 {
			return fmt.Errorf("max_days must be 0 (unlimited) or positive for csv channel")
		}
		if c.CSV.Message == "" {
			c.CSV.Message = `{{timestamp}},{{alarm_name}},{{alarm_description}},{{temperature}},{{humidity}},{{pressure}},{{wind_speed}},{{lux}},{{uv}},{{rain_daily}}`
		}
	case "json":
		if c.JSON == nil {
			return fmt.Errorf("json configuration is required for json channel")
		}
		if c.JSON.Path == "" {
			return fmt.Errorf("path is required for json channel")
		}
		if c.JSON.MaxDays < 0 {
			return fmt.Errorf("max_days must be 0 (unlimited) or positive for json channel")
		}
		if c.JSON.Message == "" {
			c.JSON.Message = `{"timestamp": "{{timestamp}}", "message": "ALARM: {{alarm_name}} triggered", "alarm": {{alarm_info}}, "sensors": {{sensor_info}}}`
		}
	}

	return nil
}

// CanFire checks if the alarm can fire based on cooldown. It reports false for
// a disabled alarm, true when Cooldown is 0, and otherwise whether at least
// Cooldown seconds have passed since [Alarm.MarkFired] was last called.
func (a *Alarm) CanFire() bool {
	if !a.Enabled {
		return false
	}
	if a.Cooldown == 0 {
		return true
	}
	return time.Since(a.lastFired) >= time.Duration(a.Cooldown)*time.Second
}

// MarkFired updates the last fired timestamp to now, starting the cooldown.
func (a *Alarm) MarkFired() {
	a.lastFired = time.Now()
}

// GetLastFired returns the last fired timestamp, or the zero time if the alarm
// has not fired.
func (a *Alarm) GetLastFired() time.Time {
	return a.lastFired
}

// GetCooldownRemaining returns the remaining cooldown time in seconds (0 if can fire).
// It also returns 0 for a disabled alarm or one with no cooldown; partial
// seconds are truncated.
func (a *Alarm) GetCooldownRemaining() int {
	if !a.Enabled || a.Cooldown == 0 {
		return 0
	}
	elapsed := time.Since(a.lastFired)
	cooldownDuration := time.Duration(a.Cooldown) * time.Second
	if elapsed >= cooldownDuration {
		return 0
	}
	remaining := cooldownDuration - elapsed
	return int(remaining.Seconds())
}

// IsInCooldown returns true if the alarm is currently in cooldown, i.e. when
// [Alarm.GetCooldownRemaining] is positive.
func (a *Alarm) IsInCooldown() bool {
	return a.GetCooldownRemaining() > 0
}

// GetPreviousValue returns the previous value for a field and whether one has
// been recorded.
func (a *Alarm) GetPreviousValue(field string) (float64, bool) {
	if a.previousValue == nil {
		return 0, false
	}
	val, ok := a.previousValue[field]
	return val, ok
}

// SetPreviousValue stores the previous value for a field, creating the map on
// first use.
func (a *Alarm) SetPreviousValue(field string, value float64) {
	if a.previousValue == nil {
		a.previousValue = make(map[string]float64)
	}
	a.previousValue[field] = value
}

// GetTriggerValue returns the trigger context value for a field and whether
// one was recorded by the last change-detection trigger.
func (a *Alarm) GetTriggerValue(field string) (float64, bool) {
	if a.triggerContext == nil {
		return 0, false
	}
	val, ok := a.triggerContext[field]
	return val, ok
}

// SetTriggerContext stores the field values at time of trigger, replacing any
// earlier context. The map is kept, not copied.
func (a *Alarm) SetTriggerContext(values map[string]float64) {
	a.triggerContext = values
}
