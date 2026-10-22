package alarm

import (
	"context"
	"crypto/tls"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log/syslog"
	"net/http"
	"net/smtp"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	msgraphsdkgo "github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/users"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awscredentials "github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sns"

	"tempest-homekit-go/pkg/logger"
	"tempest-homekit-go/pkg/weather"
)

// Package-level state shared by the notifiers.
var (
	// appStartTime tracks when the application started (strictly, when this
	// package was initialised); it is the basis of the uptime shown by
	// {{app_info}}.
	appStartTime = time.Now()
)

// Notifier interface for sending notifications. Send expands the channel's
// template(s) for alarm, obs and stationName and delivers the result to one
// destination, returning an error if delivery fails.
type Notifier interface {
	Send(alarm *Alarm, channel *Channel, obs *weather.Observation, stationName string) error
}

// NotifierFactory creates notifiers for different channel types, handing
// each one the global provider settings it needs from config.
type NotifierFactory struct {
	config *AlarmConfig
}

// NewNotifierFactory creates a new notifier factory bound to config. config
// must be non-nil; [NotifierFactory.GetNotifier] reads its Email, SMS and
// Syslog fields.
func NewNotifierFactory(config *AlarmConfig) *NotifierFactory {
	return &NotifierFactory{config: config}
}

// GetNotifier returns a notifier for the given channel type (console, syslog,
// oslog, eventlog, email, sms, webhook, csv or json). A fresh notifier is
// built on each call. It returns an error for any other type.
func (f *NotifierFactory) GetNotifier(channelType string) (Notifier, error) {
	switch channelType {
	case "console":
		return &ConsoleNotifier{}, nil
	case "syslog":
		return &SyslogNotifier{config: f.config.Syslog}, nil
	case "oslog":
		return &OSLogNotifier{}, nil
	case "eventlog":
		return &EventLogNotifier{}, nil
	case "email":
		return &EmailNotifier{config: f.config.Email}, nil
	case "sms":
		return &SMSNotifier{config: f.config.SMS}, nil
	case "webhook":
		return &WebhookNotifier{}, nil
	case "csv":
		return &CSVNotifier{}, nil
	case "json":
		return &JSONNotifier{}, nil
	default:
		return nil, fmt.Errorf("unsupported notifier type: %s", channelType)
	}
}

// ConsoleNotifier sends notifications to console/stdout through logger.Alarm.
type ConsoleNotifier struct{}

// Send expands channel.Template and writes it with logger.Alarm. It never
// returns an error.
func (n *ConsoleNotifier) Send(alarm *Alarm, channel *Channel, obs *weather.Observation, stationName string) error {
	message := expandTemplate(channel.Template, alarm, obs, stationName)
	logger.Alarm("%s", message)
	return nil
}

// SyslogNotifier sends notifications to syslog, either the local daemon or a
// remote server described by config (which may be nil).
type SyslogNotifier struct {
	config *SyslogConfig
}

// Send expands channel.Template and writes it to syslog. It dials the remote
// server when config has both Network and Address, otherwise the local daemon
// with tag "tempest-weather". A new connection is opened and closed per
// message. The configured Priority is passed when opening the writer, but the
// message itself is always written at warning severity. It returns an error
// if syslog cannot be reached or the write fails.
func (n *SyslogNotifier) Send(alarm *Alarm, channel *Channel, obs *weather.Observation, stationName string) error {
	message := expandTemplate(channel.Template, alarm, obs, stationName)

	var priority syslog.Priority
	if n.config != nil {
		switch strings.ToLower(n.config.Priority) {
		case "error":
			priority = syslog.LOG_ERR
		case "warning":
			priority = syslog.LOG_WARNING
		case "info":
			priority = syslog.LOG_INFO
		default:
			priority = syslog.LOG_WARNING
		}
	} else {
		priority = syslog.LOG_WARNING
	}

	var writer *syslog.Writer
	var err error

	if n.config != nil && n.config.Network != "" && n.config.Address != "" {
		writer, err = syslog.Dial(n.config.Network, n.config.Address, priority, n.config.Tag)
	} else {
		writer, err = syslog.New(priority, "tempest-weather")
	}

	if err != nil {
		return fmt.Errorf("failed to connect to syslog: %w", err)
	}
	defer func() { _ = writer.Close() }()

	return writer.Warning(message)
}

// EventLogNotifier sends notifications to system event log (Windows) or syslog (Unix).
type EventLogNotifier struct{}

// Send expands channel.Template and delivers it. On Windows it currently only
// logs the message through logger.Info (no real Event Log write); elsewhere it
// writes to the local syslog at warning severity with tag "tempest-weather".
// It returns an error if syslog cannot be reached or the write fails.
func (n *EventLogNotifier) Send(alarm *Alarm, channel *Channel, obs *weather.Observation, stationName string) error {
	message := expandTemplate(channel.Template, alarm, obs, stationName)

	if runtime.GOOS == "windows" {
		// On Windows, use event log (simplified - would need golang.org/x/sys/windows for full implementation)
		logger.Info("[EventLog] %s", message)
		return nil
	}

	// On Unix systems, fall back to syslog
	writer, err := syslog.New(syslog.LOG_WARNING, "tempest-weather")
	if err != nil {
		return fmt.Errorf("failed to connect to syslog: %w", err)
	}
	defer func() { _ = writer.Close() }()

	return writer.Warning(message)
}

// EmailNotifier sends email notifications using the global provider settings
// in config (SMTP or Microsoft 365).
type EmailNotifier struct {
	config *EmailGlobalConfig
}

// Send expands the subject and body templates (falling back to
// channel.Template when Email.Body is empty), prepends a "To: ..." line to the
// body, and sends it via SMTP or, for Microsoft 365 with OAuth2, the Graph
// API; Microsoft 365 without OAuth2 falls back to SMTP. It returns an error if
// the channel or global email config is missing, the provider is unknown, or
// delivery fails.
func (n *EmailNotifier) Send(alarm *Alarm, channel *Channel, obs *weather.Observation, stationName string) error {
	if channel.Email == nil {
		return fmt.Errorf("email configuration missing for channel")
	}

	if n.config == nil {
		return fmt.Errorf("global email configuration not set")
	}

	// Expand templates - use channel.Template if email.Body is empty
	subject := expandTemplate(channel.Email.Subject, alarm, obs, stationName)
	bodyTemplate := channel.Email.Body
	if bodyTemplate == "" {
		bodyTemplate = channel.Template
	}
	body := expandTemplate(bodyTemplate, alarm, obs, stationName)

	// Prepend recipient information to body for better context
	toList := strings.Join(channel.Email.To, ", ")
	body = fmt.Sprintf("To: %s\n\n%s", toList, body)

	// Build email message
	from := n.config.FromAddress
	if n.config.FromName != "" {
		from = fmt.Sprintf("%s <%s>", n.config.FromName, n.config.FromAddress)
	}

	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("From: %s\r\n", from))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(channel.Email.To, ", ")))
	if len(channel.Email.CC) > 0 {
		msg.WriteString(fmt.Sprintf("Cc: %s\r\n", strings.Join(channel.Email.CC, ", ")))
	}
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))

	// Set content type based on Html flag
	if channel.Email.Html {
		msg.WriteString("MIME-Version: 1.0\r\n")
		msg.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	} else {
		msg.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	}

	msg.WriteString("\r\n")
	msg.WriteString(body)

	// Prepare recipients list
	recipients := append([]string{}, channel.Email.To...)
	recipients = append(recipients, channel.Email.CC...)
	recipients = append(recipients, channel.Email.BCC...)

	// Send email based on provider
	switch n.config.Provider {
	case "smtp":
		return n.sendSMTP(recipients, []byte(msg.String()))
	case "microsoft365", "o365", "exchange":
		if n.config.UseOAuth2 {
			return n.sendMicrosoft365(channel.Email, subject, body)
		}
		// Fall back to SMTP for M365 without OAuth2
		logger.Info("Microsoft 365 OAuth2 not configured, using SMTP for Exchange")
		return n.sendSMTP(recipients, []byte(msg.String()))
	default:
		return fmt.Errorf("unsupported email provider: %s", n.config.Provider)
	}
}

// sendSMTP delivers a pre-built RFC 5322 message to the envelope recipients in
// to, using PLAIN auth. With UseTLS, port 465 uses implicit TLS and any other
// port uses STARTTLS (which must succeed); without UseTLS it uses
// smtp.SendMail, which still upgrades opportunistically. Username and password
// undergo environment-variable expansion. Errors from any SMTP step are
// returned wrapped with that step's name.
func (n *EmailNotifier) sendSMTP(to []string, msg []byte) error {
	// Get credentials from config (with environment variable expansion)
	username := os.ExpandEnv(n.config.Username)
	password := os.ExpandEnv(n.config.Password)

	auth := smtp.PlainAuth("", username, password, n.config.SMTPHost)

	addr := fmt.Sprintf("%s:%d", n.config.SMTPHost, n.config.SMTPPort)

	if n.config.UseTLS {
		// Determine if we should use implicit TLS (port 465) or STARTTLS (port 587)
		useImplicitTLS := n.config.SMTPPort == 465

		tlsConfig := &tls.Config{
			ServerName: n.config.SMTPHost,
		}

		if useImplicitTLS {
			// Implicit TLS: Connect with TLS from the start (port 465)
			conn, err := tls.Dial("tcp", addr, tlsConfig)
			if err != nil {
				return fmt.Errorf("failed to dial TLS: %w", err)
			}
			defer func() { _ = conn.Close() }()

			client, err := smtp.NewClient(conn, n.config.SMTPHost)
			if err != nil {
				return fmt.Errorf("failed to create SMTP client: %w", err)
			}
			defer func() { _ = client.Close() }()

			if err = client.Auth(auth); err != nil {
				return fmt.Errorf("SMTP auth failed: %w", err)
			}

			if err = client.Mail(n.config.FromAddress); err != nil {
				return fmt.Errorf("SMTP MAIL failed: %w", err)
			}

			for _, addr := range to {
				if err = client.Rcpt(addr); err != nil {
					return fmt.Errorf("SMTP RCPT failed for %s: %w", addr, err)
				}
			}

			w, err := client.Data()
			if err != nil {
				return fmt.Errorf("SMTP DATA failed: %w", err)
			}

			_, err = w.Write(msg)
			if err != nil {
				return fmt.Errorf("failed to write message: %w", err)
			}

			err = w.Close()
			if err != nil {
				return fmt.Errorf("failed to close data writer: %w", err)
			}

			return client.Quit()
		} else {
			// STARTTLS: Connect plain, then upgrade to TLS (port 587)
			client, err := smtp.Dial(addr)
			if err != nil {
				return fmt.Errorf("failed to dial SMTP: %w", err)
			}
			defer func() { _ = client.Close() }()

			// Send STARTTLS command
			if err = client.StartTLS(tlsConfig); err != nil {
				return fmt.Errorf("STARTTLS failed: %w", err)
			}

			if err = client.Auth(auth); err != nil {
				return fmt.Errorf("SMTP auth failed: %w", err)
			}

			if err = client.Mail(n.config.FromAddress); err != nil {
				return fmt.Errorf("SMTP MAIL failed: %w", err)
			}

			for _, addr := range to {
				if err = client.Rcpt(addr); err != nil {
					return fmt.Errorf("SMTP RCPT failed for %s: %w", addr, err)
				}
			}

			w, err := client.Data()
			if err != nil {
				return fmt.Errorf("SMTP DATA failed: %w", err)
			}

			_, err = w.Write(msg)
			if err != nil {
				return fmt.Errorf("failed to write message: %w", err)
			}

			err = w.Close()
			if err != nil {
				return fmt.Errorf("failed to close data writer: %w", err)
			}

			return client.Quit()
		}
	}

	// Non-TLS SMTP
	return smtp.SendMail(addr, auth, n.config.FromAddress, to, msg)
}

// sendMicrosoft365 sends the message through the Microsoft Graph sendMail API
// using client-credential (app-only) auth, so the app registration needs the
// Mail.Send application permission. The mail is sent as the user named by
// FromAddress, which must be a user principal name in the tenant, and is not
// saved to Sent Items. Credentials undergo environment-variable expansion. It
// returns an error if credentials or the from address are missing, or if
// authentication or sending fails.
func (n *EmailNotifier) sendMicrosoft365(emailConfig *EmailConfig, subject, body string) error {
	// Get credentials from environment (expand environment variables)
	clientID := os.ExpandEnv(n.config.ClientID)
	clientSecret := os.ExpandEnv(n.config.ClientSecret)
	tenantID := os.ExpandEnv(n.config.TenantID)
	fromAddress := os.ExpandEnv(n.config.FromAddress)

	if clientID == "" || clientSecret == "" || tenantID == "" {
		return fmt.Errorf("microsoft 365 OAuth2 credentials missing (CLIENT_ID, CLIENT_SECRET, TENANT_ID required)")
	}

	if fromAddress == "" {
		return fmt.Errorf("FROM_ADDRESS is required for Microsoft 365 email")
	}

	logger.Debug("Sending email via Microsoft 365 Graph API")
	logger.Debug("  Tenant ID: %s", tenantID)
	logger.Debug("  Client ID: %s", clientID)
	logger.Debug("  From: %s", fromAddress)
	logger.Debug("  To: %v", emailConfig.To)

	// Create client credentials
	cred, err := azidentity.NewClientSecretCredential(tenantID, clientID, clientSecret, nil)
	if err != nil {
		return fmt.Errorf("failed to create Azure credentials: %w", err)
	}

	// Create Graph client
	client, err := msgraphsdkgo.NewGraphServiceClientWithCredentials(cred, []string{"https://graph.microsoft.com/.default"})
	if err != nil {
		return fmt.Errorf("failed to create Graph client: %w", err)
	}

	// Build message
	message := models.NewMessage()
	message.SetSubject(&subject)

	// Set body
	bodyContent := models.NewItemBody()
	contentType := models.TEXT_BODYTYPE
	if emailConfig.Html {
		contentType = models.HTML_BODYTYPE
	}
	bodyContent.SetContentType(&contentType)
	bodyContent.SetContent(&body)
	message.SetBody(bodyContent)

	// Set recipients
	toRecipients := make([]models.Recipientable, 0, len(emailConfig.To))
	for _, addr := range emailConfig.To {
		recipient := models.NewRecipient()
		emailAddr := models.NewEmailAddress()
		emailAddr.SetAddress(&addr)
		recipient.SetEmailAddress(emailAddr)
		toRecipients = append(toRecipients, recipient)
	}
	message.SetToRecipients(toRecipients)

	// Set CC if provided
	if len(emailConfig.CC) > 0 {
		ccRecipients := make([]models.Recipientable, 0, len(emailConfig.CC))
		for _, addr := range emailConfig.CC {
			recipient := models.NewRecipient()
			emailAddr := models.NewEmailAddress()
			emailAddr.SetAddress(&addr)
			recipient.SetEmailAddress(emailAddr)
			ccRecipients = append(ccRecipients, recipient)
		}
		message.SetCcRecipients(ccRecipients)
	}

	// Set BCC if provided
	if len(emailConfig.BCC) > 0 {
		bccRecipients := make([]models.Recipientable, 0, len(emailConfig.BCC))
		for _, addr := range emailConfig.BCC {
			recipient := models.NewRecipient()
			emailAddr := models.NewEmailAddress()
			emailAddr.SetAddress(&addr)
			recipient.SetEmailAddress(emailAddr)
			bccRecipients = append(bccRecipients, recipient)
		}
		message.SetBccRecipients(bccRecipients)
	}

	// Set from address
	fromRecipient := models.NewRecipient()
	fromEmailAddr := models.NewEmailAddress()
	fromEmailAddr.SetAddress(&fromAddress)
	if n.config.FromName != "" {
		fromEmailAddr.SetName(&n.config.FromName)
	}
	fromRecipient.SetEmailAddress(fromEmailAddr)
	message.SetFrom(fromRecipient)

	// Create send mail request body
	sendMailBody := users.NewItemSendMailPostRequestBody()
	sendMailBody.SetMessage(message)
	saveToSentItems := false
	sendMailBody.SetSaveToSentItems(&saveToSentItems)

	// Send the email using the from address as the user principal
	// Note: The from address must be a valid user principal name (UPN) in the tenant
	ctx := context.Background()

	// Extract the user principal - ensure it's properly formatted
	userPrincipal := fromAddress
	logger.Debug("  Sending as user principal: %s", userPrincipal)

	err = client.Users().ByUserId(userPrincipal).SendMail().Post(ctx, sendMailBody, nil)
	if err != nil {
		return fmt.Errorf("failed to send email via Microsoft Graph API (user: %s): %w", userPrincipal, err)
	}

	logger.Info("Email sent successfully via Microsoft 365 to %v", emailConfig.To)
	return nil
}

// SMSNotifier sends SMS notifications using the global provider settings in
// config (Twilio or AWS SNS).
type SMSNotifier struct {
	config *SMSGlobalConfig
}

// Send expands channel.SMS.Message and sends it to every recipient via the
// configured provider. It returns an error if the channel or global SMS config
// is missing, the provider is unknown, or every recipient failed.
func (n *SMSNotifier) Send(alarm *Alarm, channel *Channel, obs *weather.Observation, stationName string) error {
	if channel.SMS == nil {
		return fmt.Errorf("SMS configuration missing for channel")
	}

	if n.config == nil {
		return fmt.Errorf("global SMS configuration not set")
	}

	message := expandTemplate(channel.SMS.Message, alarm, obs, stationName)

	// Send based on provider
	switch n.config.Provider {
	case "aws_sns", "sns", "aws":
		return n.sendAWSSNS(channel.SMS, message)
	case "twilio":
		return n.sendTwilio(channel.SMS, message)
	default:
		return fmt.Errorf("unsupported SMS provider: %s", n.config.Provider)
	}
}

// sendAWSSNS sends message through AWS SNS using only the static credentials
// from config (no shared-credentials file or IAM role fallback). Without a
// topic ARN it sends a direct SMS to each number in smsConfig.To; with one it
// publishes to the topic once per entry in To. It returns an error only if
// credentials are missing, the AWS config cannot load, or every publish
// failed; partial failures are logged.
func (n *SMSNotifier) sendAWSSNS(smsConfig *SMSConfig, message string) error {
	// Get credentials from config (with environment variable expansion)
	accessKey := os.ExpandEnv(n.config.AWSAccessKey)
	secretKey := os.ExpandEnv(n.config.AWSSecretKey)
	region := os.ExpandEnv(n.config.AWSRegion)
	topicARN := os.ExpandEnv(n.config.AWSSNSTopicARN)

	if accessKey == "" || secretKey == "" || region == "" {
		return fmt.Errorf("AWS SNS credentials missing (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_REGION required)")
	}

	logger.Debug("Sending SMS via AWS SNS")
	logger.Debug("  Region: %s", region)
	logger.Debug("  To: %v", smsConfig.To)

	// Create AWS config with ONLY the credentials from .env (no OS credentials)
	// Using StaticCredentialsProvider prevents fallback to ~/.aws/credentials or IAM roles
	ctx := context.Background()
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(awscredentials.NewStaticCredentialsProvider(
			accessKey,
			secretKey,
			"",
		)),
	)
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create SNS client
	snsClient := sns.NewFromConfig(cfg)

	// Send to each recipient
	var lastErr error
	successCount := 0

	for _, phoneNumber := range smsConfig.To {
		// If topic ARN is provided, publish to topic
		if topicARN != "" {
			logger.Debug("Publishing to SNS topic: %s", topicARN)
			publishInput := &sns.PublishInput{
				Message:  &message,
				TopicArn: &topicARN,
			}
			_, err := snsClient.Publish(ctx, publishInput)
			if err != nil {
				lastErr = err
				logger.Error("Failed to publish to SNS topic %s: %v", topicARN, err)
			} else {
				successCount++
				logger.Info("SMS published to SNS topic %s", topicARN)
			}
		} else {
			// Direct SMS to phone number
			logger.Debug("Sending direct SMS to: %s", phoneNumber)
			publishInput := &sns.PublishInput{
				Message:     &message,
				PhoneNumber: &phoneNumber,
			}
			_, err := snsClient.Publish(ctx, publishInput)
			if err != nil {
				lastErr = err
				logger.Error("Failed to send SMS to %s via AWS SNS: %v", phoneNumber, err)
			} else {
				successCount++
				logger.Info("SMS sent successfully to %s via AWS SNS", phoneNumber)
			}
		}
	}

	if successCount == 0 && lastErr != nil {
		return fmt.Errorf("failed to send any SMS messages: %w", lastErr)
	}

	return nil
}

// sendTwilio sends message to each number in smsConfig.To through the Twilio
// Messages REST API with a 10-second timeout per request. It returns an error
// only if credentials are missing or every recipient failed; partial failures
// are logged with a warning.
func (n *SMSNotifier) sendTwilio(smsConfig *SMSConfig, message string) error {
	// Get credentials from config (with environment variable expansion)
	accountSID := os.ExpandEnv(n.config.AccountSID)
	authToken := os.ExpandEnv(n.config.AuthToken)
	fromNumber := os.ExpandEnv(n.config.FromNumber)

	if accountSID == "" || authToken == "" || fromNumber == "" {
		return fmt.Errorf("twilio credentials missing (TWILIO_ACCOUNT_SID, TWILIO_AUTH_TOKEN, TWILIO_FROM_NUMBER required)")
	}

	logger.Debug("Sending SMS via Twilio")
	logger.Debug("  Account SID: %s", accountSID[:10]+"...")
	logger.Debug("  From: %s", fromNumber)
	logger.Debug("  To: %v", smsConfig.To)

	// Twilio API endpoint
	urlStr := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Messages.json", accountSID)

	// Send to each recipient
	var lastErr error
	successCount := 0

	for _, phoneNumber := range smsConfig.To {
		// Prepare form data
		data := url.Values{}
		data.Set("To", phoneNumber)
		data.Set("From", fromNumber)
		data.Set("Body", message)

		// Create HTTP request
		req, err := http.NewRequest("POST", urlStr, strings.NewReader(data.Encode()))
		if err != nil {
			lastErr = err
			logger.Error("Failed to create Twilio request: %v", err)
			continue
		}

		// Set authentication and headers
		req.SetBasicAuth(accountSID, authToken)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		// Send request
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			logger.Error("Failed to send SMS to %s via Twilio: %v", phoneNumber, err)
			continue
		}
		defer func() { _ = resp.Body.Close() }()

		// Check response
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			successCount++
			logger.Info("SMS sent successfully via Twilio to [%s]", phoneNumber)
		} else {
			body, _ := io.ReadAll(resp.Body)
			lastErr = fmt.Errorf("twilio api error (status %d): %s", resp.StatusCode, string(body))
			logger.Error("Failed to send SMS to %s via Twilio: %v", phoneNumber, lastErr)
		}
	}

	if successCount == 0 && lastErr != nil {
		return fmt.Errorf("failed to send any SMS via Twilio: %w", lastErr)
	}

	if successCount < len(smsConfig.To) {
		logger.Warn("Sent %d/%d SMS messages successfully", successCount, len(smsConfig.To))
	}

	return nil
}

// WebhookNotifier sends webhook notifications as HTTP requests.
type WebhookNotifier struct{}

// Send expands channel.Webhook.Body and sends it to the configured URL with
// the configured method, Content-Type and extra headers, with a 10-second
// timeout. It returns an error if the request cannot be built or sent, or if
// the response status is not 2xx (including the response body in the error).
func (n *WebhookNotifier) Send(alarm *Alarm, channel *Channel, obs *weather.Observation, stationName string) error {
	if channel.Webhook == nil {
		return fmt.Errorf("webhook configuration missing for channel")
	}

	// Expand the body template
	body := expandTemplate(channel.Webhook.Body, alarm, obs, stationName)

	// Create HTTP request
	req, err := http.NewRequest(channel.Webhook.Method, channel.Webhook.URL, strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create webhook request: %w", err)
	}

	// Set content type
	req.Header.Set("Content-Type", channel.Webhook.ContentType)

	// Set custom headers
	for key, value := range channel.Webhook.Headers {
		req.Header.Set(key, value)
	}

	// Send request
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("webhook request failed with status %d: %s", resp.StatusCode, string(body))
	}

	logger.Info("Webhook sent successfully to %s", channel.Webhook.URL)
	return nil
}

// CSVNotifier writes alarm notifications to CSV files.
type CSVNotifier struct{}

// Send expands channel.CSV.Message and appends it as a row to the configured
// CSV file. It returns an error if the CSV config is missing or the write
// fails.
func (n *CSVNotifier) Send(alarm *Alarm, channel *Channel, obs *weather.Observation, stationName string) error {
	if channel.CSV == nil {
		return fmt.Errorf("CSV configuration missing for channel")
	}

	// Expand the message template
	message := expandTemplate(channel.CSV.Message, alarm, obs, stationName)

	return n.appendToCSVFile(channel.CSV.Path, message, channel.CSV.MaxDays)
}

// JSONNotifier writes alarm notifications to JSON files.
type JSONNotifier struct{}

// Send expands channel.JSON.Message and appends it as a record to the
// configured JSON array file. It returns an error if the JSON config is
// missing or the write fails.
func (n *JSONNotifier) Send(alarm *Alarm, channel *Channel, obs *weather.Observation, stationName string) error {
	if channel.JSON == nil {
		return fmt.Errorf("JSON configuration missing for channel")
	}

	// Expand the message template
	message := expandTemplate(channel.JSON.Message, alarm, obs, stationName)

	return n.appendToJSONFile(channel.JSON.Path, message, channel.JSON.MaxDays)
}

// appendToCSVFile appends a message to a CSV file with rotation.
//
// The file is first rotated if older than maxDays (rotation errors are only
// logged). If filePath cannot be opened, filePath+".tmp" is used instead. A
// header row is written to an empty file: the 11-column default header when
// the message contains a comma, otherwise "timestamp,message". Each row starts
// with the local time; a comma-separated message is parsed as one CSV record
// and its fields appended, otherwise the message is a single field. It returns
// an error if neither file can be opened or a write fails.
func (n *CSVNotifier) appendToCSVFile(filePath string, message string, maxDays int) error {
	// Check if file needs rotation
	if maxDays > 0 {
		if err := rotateFileIfNeeded(filePath, maxDays); err != nil {
			logger.Warn("Failed to rotate CSV file %s: %v", filePath, err)
		}
	}

	// Open file for appending (create if doesn't exist)
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		// Try to create temp file as fallback
		tempPath := filePath + ".tmp"
		logger.Warn("Failed to open CSV file %s: %v, using temp file %s", filePath, err, tempPath)
		file, err = os.OpenFile(tempPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("failed to open temp CSV file %s: %w", tempPath, err)
		}
		filePath = tempPath
	}
	defer func() { _ = file.Close() }()

	// Check if file is empty to write headers
	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}

	isEmpty := fileInfo.Size() == 0

	// Prepare CSV writer
	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write headers if file is empty
	if isEmpty {
		// Check if message contains commas (multi-column format)
		if strings.Contains(message, ",") {
			headers := []string{"timestamp", "alarm_name", "alarm_description", "temperature", "humidity", "pressure", "wind_speed", "lux", "uv", "rain_daily", "message"}
			if err := writer.Write(headers); err != nil {
				return fmt.Errorf("failed to write CSV headers: %w", err)
			}
		} else {
			headers := []string{"timestamp", "message"}
			if err := writer.Write(headers); err != nil {
				return fmt.Errorf("failed to write CSV headers: %w", err)
			}
		}
	}

	// Write record with current timestamp
	timestamp := time.Now().Format("2006-01-02 15:04:05")

	// Check if message contains commas (multi-column format)
	if strings.Contains(message, ",") {
		// Parse the message as CSV to handle quoted fields properly
		reader := csv.NewReader(strings.NewReader(message))
		fields, err := reader.Read()
		if err != nil {
			// If parsing fails, treat as single field
			row := []string{timestamp, message}
			if err := writer.Write(row); err != nil {
				return fmt.Errorf("failed to write CSV record: %w", err)
			}
		} else {
			// Prepend timestamp to the fields
			row := append([]string{timestamp}, fields...)
			if err := writer.Write(row); err != nil {
				return fmt.Errorf("failed to write CSV record: %w", err)
			}
		}
	} else {
		row := []string{timestamp, message}
		if err := writer.Write(row); err != nil {
			return fmt.Errorf("failed to write CSV record: %w", err)
		}
	}

	logger.Info("Alarm message written to CSV file %s", filePath)
	return nil
}

// appendToJSONFile appends a message to a JSON file with rotation.
//
// The file holds a JSON array of {"timestamp", "message"} objects. It is read
// whole, its closing "\n]" stripped, and rewritten with the new record added,
// so the write is not atomic. It returns an error if the file cannot be read
// or written, or if existing content does not end with "\n]" (the format this
// function writes).
func (n *JSONNotifier) appendToJSONFile(filePath string, message string, maxDays int) error {
	// Check if file needs rotation
	if maxDays > 0 {
		if err := rotateFileIfNeeded(filePath, maxDays); err != nil {
			logger.Warn("Failed to rotate JSON file %s: %v", filePath, err)
		}
	}

	// Read existing content
	var existingContent []byte
	file, err := os.Open(filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to open JSON file for reading: %w", err)
		}
		// File doesn't exist, existingContent remains empty
	} else {
		defer func() { _ = file.Close() }()
		existingContent, err = io.ReadAll(file)
		if err != nil {
			return fmt.Errorf("failed to read JSON file: %w", err)
		}
	}

	// Create JSON record with timestamp and message
	record := map[string]interface{}{
		"timestamp": time.Now().Format("2006-01-02 15:04:05"),
		"message":   message,
	}

	// Marshal record to JSON
	jsonData, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON record: %w", err)
	}

	// Prepare new content
	var newContent []byte
	if len(existingContent) == 0 {
		// Empty file, start new array
		newContent = append([]byte("[\n"), jsonData...)
		newContent = append(newContent, []byte("\n]")...)
	} else {
		// File exists, check if it ends with \n]
		contentStr := string(existingContent)
		if !strings.HasSuffix(contentStr, "\n]") {
			return fmt.Errorf("JSON file %s does not end with expected array closure", filePath)
		}
		// Remove the closing \n]
		contentStr = contentStr[:len(contentStr)-2]
		// Add comma, newline, record, and closing bracket
		newContent = []byte(contentStr + ",\n" + string(jsonData) + "\n]")
	}

	// Write the new content back to file
	err = os.WriteFile(filePath, newContent, 0644)
	if err != nil {
		return fmt.Errorf("failed to write JSON file: %w", err)
	}

	logger.Info("Alarm message written to JSON file %s", filePath)
	return nil
}

// rotateFileIfNeeded rotates a file if it's older than maxDays, renaming it to
// filePath.<YYYY-MM-DD_HH-MM-SS>.bak. Age is measured from the file's
// modification time, which every append refreshes. A missing file or
// maxDays <= 0 is a no-op. It returns an error if stat or rename fails.
func rotateFileIfNeeded(filePath string, maxDays int) error {
	if maxDays <= 0 {
		return nil // No rotation needed
	}

	fileInfo, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist, no rotation needed
		}
		return err
	}

	// Check if file is older than maxDays
	fileAge := time.Since(fileInfo.ModTime())
	maxAge := time.Duration(maxDays) * 24 * time.Hour

	if fileAge > maxAge {
		// Create backup with timestamp
		timestamp := time.Now().Format("2006-01-02_15-04-05")
		backupPath := filePath + "." + timestamp + ".bak"

		if err := os.Rename(filePath, backupPath); err != nil {
			return fmt.Errorf("failed to rotate file: %w", err)
		}

		logger.Info("Rotated file %s to %s (age: %v)", filePath, backupPath, fileAge)
	}

	return nil
}

// formatNumber formats a number with thousands separator, rounding to an
// integer (1234567.8 becomes "1,234,568"). Negative values are not handled
// correctly (for example -123 becomes "-,123").
func formatNumber(n float64) string {
	s := fmt.Sprintf("%.0f", n)
	if len(s) <= 3 {
		return s
	}

	var result strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result.WriteString(",")
		}
		result.WriteRune(c)
	}
	return result.String()
}

// formatAppInfo returns formatted application information for {{app_info}}:
// app version, uptime since appStartTime and Go version, as an HTML <div> when
// isHTML is true or a single plain-text line otherwise.
func formatAppInfo(isHTML bool) string {
	uptime := time.Since(appStartTime)
	days := int(uptime.Hours() / 24)
	hours := int(uptime.Hours()) % 24
	minutes := int(uptime.Minutes()) % 60

	uptimeStr := ""
	if days > 0 {
		uptimeStr = fmt.Sprintf("%d days, %d hours, %d minutes", days, hours, minutes)
	} else if hours > 0 {
		uptimeStr = fmt.Sprintf("%d hours, %d minutes", hours, minutes)
	} else {
		uptimeStr = fmt.Sprintf("%d minutes", minutes)
	}

	if isHTML {
		return fmt.Sprintf(`<div style="font-size: 11px; color: #666; font-family: monospace;">
			<strong>Tempest HomeKit Bridge</strong> %s | Uptime: %s | Go %s
		</div>`, appVersion, uptimeStr, runtime.Version())
	}

	return fmt.Sprintf("Tempest HomeKit Bridge %s | Uptime: %s | Go %s",
		appVersion, uptimeStr, runtime.Version())
}

// formatAlarmInfo returns formatted alarm information for {{alarm_info}}:
// name, description, condition, status, cooldown (whole hours or minutes when
// large enough), trigger count and tags, as an HTML table when isHTML is true
// or newline-separated text otherwise. Values are not HTML-escaped.
func formatAlarmInfo(alarm *Alarm, isHTML bool) string {
	enabledStr := "enabled"
	if !alarm.Enabled {
		enabledStr = "disabled"
	}

	cooldownStr := fmt.Sprintf("%d seconds", alarm.Cooldown)
	if alarm.Cooldown >= 3600 {
		cooldownStr = fmt.Sprintf("%d hours", alarm.Cooldown/3600)
	} else if alarm.Cooldown >= 60 {
		cooldownStr = fmt.Sprintf("%d minutes", alarm.Cooldown/60)
	}

	tagsStr := "none"
	if len(alarm.Tags) > 0 {
		tagsStr = strings.Join(alarm.Tags, ", ")
	}

	if isHTML {
		return fmt.Sprintf(`<table style="border-collapse: collapse; width: 100%%;">
			<tr><td style="padding: 5px; border: 1px solid #ddd; font-weight: bold;">Alarm:</td><td style="padding: 5px; border: 1px solid #ddd;">%s</td></tr>
			<tr><td style="padding: 5px; border: 1px solid #ddd; font-weight: bold;">Description:</td><td style="padding: 5px; border: 1px solid #ddd;">%s</td></tr>
			<tr><td style="padding: 5px; border: 1px solid #ddd; font-weight: bold;">Condition:</td><td style="padding: 5px; border: 1px solid #ddd;">%s</td></tr>
			<tr><td style="padding: 5px; border: 1px solid #ddd; font-weight: bold;">Status:</td><td style="padding: 5px; border: 1px solid #ddd;">%s</td></tr>
			<tr><td style="padding: 5px; border: 1px solid #ddd; font-weight: bold;">Cooldown:</td><td style="padding: 5px; border: 1px solid #ddd;">%s</td></tr>
			<tr><td style="padding: 5px; border: 1px solid #ddd; font-weight: bold;">Triggered:</td><td style="padding: 5px; border: 1px solid #ddd;">%d</td></tr>
			<tr><td style="padding: 5px; border: 1px solid #ddd; font-weight: bold;">Tags:</td><td style="padding: 5px; border: 1px solid #ddd;">%s</td></tr>
		</table>`,
			alarm.Name, alarm.Description, alarm.Condition, enabledStr, cooldownStr, alarm.TriggeredCount, tagsStr)
	}

	return fmt.Sprintf("Alarm: %s\nDescription: %s\nCondition: %s\nStatus: %s\nCooldown: %s\nTriggered: %d\nTags: %s",
		alarm.Name, alarm.Description, alarm.Condition, enabledStr, cooldownStr, alarm.TriggeredCount, tagsStr)
}

// formatSensorInfo returns formatted sensor information without previous
// values (every "Last" column shows N/A).
func formatSensorInfo(obs *weather.Observation, isHTML bool) string {
	return formatSensorInfoWithAlarm(obs, nil, isHTML)
}

// formatSensorInfoWithAlarm returns the {{sensor_info}} block: current
// readings (with imperial conversions and a cardinal wind direction) plus each
// field's last value from alarm, taken from its trigger context when present
// and otherwise from its previous values. alarm may be nil, which shows N/A.
// In HTML mode it renders a table and highlights rows whose value moved by
// more than a per-field threshold. obs must be non-nil.
func formatSensorInfoWithAlarm(obs *weather.Observation, alarm *Alarm, isHTML bool) string {
	tempF := obs.AirTemperature*9/5 + 32
	windSpeedMph := obs.WindAvg * 2.23694
	windGustMph := obs.WindGust * 2.23694
	rainDaily := obs.RainDailyTotal / 25.4 // Convert mm to inches

	// Wind direction cardinal
	dir := obs.WindDirection
	cardinal := "N"
	switch {
	case dir >= 337.5 || dir < 22.5:
		cardinal = "N"
	case dir >= 22.5 && dir < 67.5:
		cardinal = "NE"
	case dir >= 67.5 && dir < 112.5:
		cardinal = "E"
	case dir >= 112.5 && dir < 157.5:
		cardinal = "SE"
	case dir >= 157.5 && dir < 202.5:
		cardinal = "S"
	case dir >= 202.5 && dir < 247.5:
		cardinal = "SW"
	case dir >= 247.5 && dir < 292.5:
		cardinal = "W"
	case dir >= 292.5 && dir < 337.5:
		cardinal = "NW"
	}

	// Helper to get previous value with proper formatting
	getPrevValue := func(key string, _ /* current */ float64, format string) string {
		if alarm == nil {
			return "N/A"
		}
		if prev, ok := alarm.GetTriggerValue(key); ok {
			return fmt.Sprintf(format, prev)
		}
		if prev, ok := alarm.GetPreviousValue(key); ok {
			return fmt.Sprintf(format, prev)
		}
		return "N/A"
	}

	// Special handler for illuminance which needs number formatting
	getPrevLux := func() string {
		if alarm == nil {
			return "N/A"
		}
		if prev, ok := alarm.GetTriggerValue("lux"); ok {
			return formatNumber(prev)
		}
		if prev, ok := alarm.GetPreviousValue("lux"); ok {
			return formatNumber(prev)
		}
		return "N/A"
	}

	if isHTML {
		// Helper to check if value changed
		hasChanged := func(key string, current float64, threshold float64) bool {
			if alarm == nil {
				return false
			}
			var prev float64
			var ok bool
			if prev, ok = alarm.GetTriggerValue(key); !ok {
				if prev, ok = alarm.GetPreviousValue(key); !ok {
					return false
				}
			}
			// Check if difference exceeds threshold
			diff := current - prev
			if diff < 0 {
				diff = -diff
			}
			return diff > threshold
		}

		// Helper to get row style based on whether value changed
		getRowStyle := func(changed bool) string {
			if changed {
				return ` style="background: #fff3cd;"`
			}
			return ""
		}

		htmlTemplate := `<table style="border-collapse: collapse; width: 100%%;">
			<tr style="background: #f0f0f0;"><th style="padding: 5px; border: 1px solid #ddd;">Sensor</th><th style="padding: 5px; border: 1px solid #ddd;">Current</th><th style="padding: 5px; border: 1px solid #ddd;">Last</th></tr>
			<tr%s><td style="padding: 5px; border: 1px solid #ddd;"><strong>Temperature:</strong></td><td style="padding: 5px; border: 1px solid #ddd;">%.1f°F (%.1f°C)</td><td style="padding: 5px; border: 1px solid #ddd;">%s°C</td></tr>
			<tr%s><td style="padding: 5px; border: 1px solid #ddd;"><strong>Humidity:</strong></td><td style="padding: 5px; border: 1px solid #ddd;">%.0f%%</td><td style="padding: 5px; border: 1px solid #ddd;">%s%%</td></tr>
			<tr%s><td style="padding: 5px; border: 1px solid #ddd;"><strong>Pressure:</strong></td><td style="padding: 5px; border: 1px solid #ddd;">%.2f mb</td><td style="padding: 5px; border: 1px solid #ddd;">%s mb</td></tr>
			<tr%s><td style="padding: 5px; border: 1px solid #ddd;"><strong>Wind Speed:</strong></td><td style="padding: 5px; border: 1px solid #ddd;">%.1f mph (%.1f m/s)</td><td style="padding: 5px; border: 1px solid #ddd;">%s m/s</td></tr>
			<tr%s><td style="padding: 5px; border: 1px solid #ddd;"><strong>Wind Gust:</strong></td><td style="padding: 5px; border: 1px solid #ddd;">%.1f mph (%.1f m/s)</td><td style="padding: 5px; border: 1px solid #ddd;">%s m/s</td></tr>
			<tr%s><td style="padding: 5px; border: 1px solid #ddd;"><strong>Wind Direction:</strong></td><td style="padding: 5px; border: 1px solid #ddd;">%.0f° (%s)</td><td style="padding: 5px; border: 1px solid #ddd;">%s°</td></tr>
			<tr%s><td style="padding: 5px; border: 1px solid #ddd;"><strong>UV Index:</strong></td><td style="padding: 5px; border: 1px solid #ddd;">%d</td><td style="padding: 5px; border: 1px solid #ddd;">%s</td></tr>
			<tr%s><td style="padding: 5px; border: 1px solid #ddd;"><strong>Illuminance:</strong></td><td style="padding: 5px; border: 1px solid #ddd;">%s lux</td><td style="padding: 5px; border: 1px solid #ddd;">%s lux</td></tr>
			<tr%s><td style="padding: 5px; border: 1px solid #ddd;"><strong>Rain Rate:</strong></td><td style="padding: 5px; border: 1px solid #ddd;">%.2f mm/hr</td><td style="padding: 5px; border: 1px solid #ddd;">%s mm/hr</td></tr>
			<tr%s><td style="padding: 5px; border: 1px solid #ddd;"><strong>Daily Rain:</strong></td><td style="padding: 5px; border: 1px solid #ddd;">%.2f in (%.1f mm)</td><td style="padding: 5px; border: 1px solid #ddd;">%s mm</td></tr>
			<tr%s><td style="padding: 5px; border: 1px solid #ddd;"><strong>Lightning:</strong></td><td style="padding: 5px; border: 1px solid #ddd;">%d strikes</td><td style="padding: 5px; border: 1px solid #ddd;">%s strikes</td></tr>
		</table>`
		return fmt.Sprintf(htmlTemplate,
			getRowStyle(hasChanged("temperature", obs.AirTemperature, 0.1)),
			tempF, obs.AirTemperature, getPrevValue("temperature", obs.AirTemperature, "%.1f"),
			getRowStyle(hasChanged("humidity", obs.RelativeHumidity, 1.0)),
			obs.RelativeHumidity, getPrevValue("humidity", obs.RelativeHumidity, "%.0f"),
			getRowStyle(hasChanged("pressure", obs.StationPressure, 0.1)),
			obs.StationPressure, getPrevValue("pressure", obs.StationPressure, "%.2f"),
			getRowStyle(hasChanged("wind_speed", obs.WindAvg, 0.1)),
			windSpeedMph, obs.WindAvg, getPrevValue("wind_speed", obs.WindAvg, "%.1f"),
			getRowStyle(hasChanged("wind_gust", obs.WindGust, 0.1)),
			windGustMph, obs.WindGust, getPrevValue("wind_gust", obs.WindGust, "%.1f"),
			getRowStyle(hasChanged("wind_direction", obs.WindDirection, 5.0)),
			obs.WindDirection, cardinal, getPrevValue("wind_direction", obs.WindDirection, "%.0f"),
			getRowStyle(hasChanged("uv", float64(obs.UV), 0.5)),
			obs.UV, getPrevValue("uv", float64(obs.UV), "%.0f"),
			getRowStyle(hasChanged("lux", obs.Illuminance, 100.0)),
			formatNumber(obs.Illuminance), getPrevLux(),
			getRowStyle(hasChanged("rain_rate", obs.RainAccumulated, 0.01)),
			obs.RainAccumulated, getPrevValue("rain_rate", obs.RainAccumulated, "%.2f"),
			getRowStyle(hasChanged("rain_daily", obs.RainDailyTotal, 0.1)),
			rainDaily, obs.RainDailyTotal, getPrevValue("rain_daily", obs.RainDailyTotal, "%.1f"),
			getRowStyle(hasChanged("lightning_count", float64(obs.LightningStrikeCount), 0.5)),
			obs.LightningStrikeCount, getPrevValue("lightning_count", float64(obs.LightningStrikeCount), "%.0f"))
	}

	return fmt.Sprintf(`Temperature: %.1f°F (%.1f°C) [Last: %s°C]
Humidity: %.0f%% [Last: %s%%]
Pressure: %.2f mb [Last: %s mb]
Wind Speed: %.1f mph (%.1f m/s) [Last: %s m/s]
Wind Gust: %.1f mph (%.1f m/s) [Last: %s m/s]
Wind Direction: %.0f° (%s) [Last: %s°]
UV Index: %d [Last: %s]
Illuminance: %s lux [Last: %s lux]
Rain Rate: %.2f mm/hr [Last: %s mm/hr]
Daily Rain: %.2f in (%.1f mm) [Last: %s mm]
Lightning: %d strikes [Last: %s strikes]`,
		tempF, obs.AirTemperature, getPrevValue("temperature", obs.AirTemperature, "%.1f"),
		obs.RelativeHumidity, getPrevValue("humidity", obs.RelativeHumidity, "%.0f"),
		obs.StationPressure, getPrevValue("pressure", obs.StationPressure, "%.2f"),
		windSpeedMph, obs.WindAvg, getPrevValue("wind_speed", obs.WindAvg, "%.1f"),
		windGustMph, obs.WindGust, getPrevValue("wind_gust", obs.WindGust, "%.1f"),
		obs.WindDirection, cardinal, getPrevValue("wind_direction", obs.WindDirection, "%.0f"),
		obs.UV, getPrevValue("uv", float64(obs.UV), "%.0f"),
		formatNumber(obs.Illuminance), getPrevLux(),
		obs.RainAccumulated, getPrevValue("rain_rate", obs.RainAccumulated, "%.2f"),
		rainDaily, obs.RainDailyTotal, getPrevValue("rain_daily", obs.RainDailyTotal, "%.1f"),
		obs.LightningStrikeCount, getPrevValue("lightning_count", float64(obs.LightningStrikeCount), "%.0f"))
}

// expandTemplate replaces template variables with actual values.
//
// Placeholders are {{name}} tokens: current readings (temperature,
// temperature_f, temperature_c, humidity, pressure, wind_speed, wind_gust,
// wind_direction, lux, uv, rain_rate, rain_daily, lightning_count,
// lightning_distance), context (timestamp, station, alarm_name,
// alarm_description, alarm_condition, message), composite blocks (app_info,
// alarm_info, sensor_info) and last_<field> values for change comparisons
// ("N/A" when unknown). Composite blocks render as HTML when the template
// contains common HTML tags. Unknown placeholders are left as-is. Values are
// inserted verbatim (no JSON, CSV or HTML escaping), and {{rain_daily}}
// currently shows RainAccumulated. alarm and obs must be non-nil.
func expandTemplate(template string, alarm *Alarm, obs *weather.Observation, stationName string) string {
	result := template

	// Detect if this is an HTML template
	isHTML := strings.Contains(template, "<html>") || strings.Contains(template, "<table>") ||
		strings.Contains(template, "<div") || strings.Contains(template, "<h1>") ||
		strings.Contains(template, "<h2>") || strings.Contains(template, "<p>")

	// Replace observation values (current)
	replacements := map[string]string{
		"{{temperature}}":        fmt.Sprintf("%.1f", obs.AirTemperature),
		"{{temperature_f}}":      fmt.Sprintf("%.1f", obs.AirTemperature*9/5+32),
		"{{temperature_c}}":      fmt.Sprintf("%.1f", obs.AirTemperature),
		"{{humidity}}":           fmt.Sprintf("%.0f", obs.RelativeHumidity),
		"{{pressure}}":           fmt.Sprintf("%.2f", obs.StationPressure),
		"{{wind_speed}}":         fmt.Sprintf("%.1f", obs.WindAvg),
		"{{wind_gust}}":          fmt.Sprintf("%.1f", obs.WindGust),
		"{{wind_direction}}":     fmt.Sprintf("%.0f", obs.WindDirection),
		"{{lux}}":                fmt.Sprintf("%.0f", obs.Illuminance),
		"{{uv}}":                 fmt.Sprintf("%d", obs.UV),
		"{{rain_rate}}":          fmt.Sprintf("%.2f", obs.RainAccumulated),
		"{{rain_daily}}":         fmt.Sprintf("%.2f", obs.RainAccumulated),
		"{{lightning_count}}":    fmt.Sprintf("%d", obs.LightningStrikeCount),
		"{{lightning_distance}}": fmt.Sprintf("%.1f", obs.LightningStrikeAvg),
		"{{timestamp}}":          time.Unix(obs.Timestamp, 0).Format("2006-01-02 15:04:05 MST"),
		"{{station}}":            stationName,
		"{{alarm_name}}":         alarm.Name,
		"{{alarm_description}}":  alarm.Description,
		"{{alarm_condition}}":    alarm.Condition,
		"{{message}}":            fmt.Sprintf("ALARM: %s triggered", alarm.Name),
		// New composite variables
		"{{app_info}}":    formatAppInfo(isHTML),
		"{{alarm_info}}":  formatAlarmInfo(alarm, isHTML),
		"{{sensor_info}}": formatSensorInfoWithAlarm(obs, alarm, isHTML),
	}

	// Add previous values for change detection comparisons
	// These show the value that was compared against to trigger the alarm
	// Use trigger context if available (more accurate), otherwise fall back to previousValue
	if lastTemp, ok := alarm.GetTriggerValue("temperature"); ok {
		replacements["{{last_temperature}}"] = fmt.Sprintf("%.1f", lastTemp)
	} else if lastTemp, ok := alarm.GetPreviousValue("temperature"); ok {
		replacements["{{last_temperature}}"] = fmt.Sprintf("%.1f", lastTemp)
	} else {
		replacements["{{last_temperature}}"] = "N/A"
	}
	if lastHumidity, ok := alarm.GetTriggerValue("humidity"); ok {
		replacements["{{last_humidity}}"] = fmt.Sprintf("%.0f", lastHumidity)
	} else if lastHumidity, ok := alarm.GetPreviousValue("humidity"); ok {
		replacements["{{last_humidity}}"] = fmt.Sprintf("%.0f", lastHumidity)
	} else {
		replacements["{{last_humidity}}"] = "N/A"
	}
	if lastPressure, ok := alarm.GetTriggerValue("pressure"); ok {
		replacements["{{last_pressure}}"] = fmt.Sprintf("%.2f", lastPressure)
	} else if lastPressure, ok := alarm.GetPreviousValue("pressure"); ok {
		replacements["{{last_pressure}}"] = fmt.Sprintf("%.2f", lastPressure)
	} else {
		replacements["{{last_pressure}}"] = "N/A"
	}
	if lastWindSpeed, ok := alarm.GetTriggerValue("wind_speed"); ok {
		replacements["{{last_wind_speed}}"] = fmt.Sprintf("%.1f", lastWindSpeed)
	} else if lastWindSpeed, ok := alarm.GetPreviousValue("wind_speed"); ok {
		replacements["{{last_wind_speed}}"] = fmt.Sprintf("%.1f", lastWindSpeed)
	} else {
		replacements["{{last_wind_speed}}"] = "N/A"
	}
	if lastWindGust, ok := alarm.GetTriggerValue("wind_gust"); ok {
		replacements["{{last_wind_gust}}"] = fmt.Sprintf("%.1f", lastWindGust)
	} else if lastWindGust, ok := alarm.GetPreviousValue("wind_gust"); ok {
		replacements["{{last_wind_gust}}"] = fmt.Sprintf("%.1f", lastWindGust)
	} else {
		replacements["{{last_wind_gust}}"] = "N/A"
	}
	if lastWindDir, ok := alarm.GetTriggerValue("wind_direction"); ok {
		replacements["{{last_wind_direction}}"] = fmt.Sprintf("%.0f", lastWindDir)
	} else if lastWindDir, ok := alarm.GetPreviousValue("wind_direction"); ok {
		replacements["{{last_wind_direction}}"] = fmt.Sprintf("%.0f", lastWindDir)
	} else {
		replacements["{{last_wind_direction}}"] = "N/A"
	}
	if lastLux, ok := alarm.GetTriggerValue("lux"); ok {
		replacements["{{last_lux}}"] = fmt.Sprintf("%.0f", lastLux)
	} else if lastLux, ok := alarm.GetPreviousValue("lux"); ok {
		replacements["{{last_lux}}"] = fmt.Sprintf("%.0f", lastLux)
	} else {
		replacements["{{last_lux}}"] = "N/A"
	}
	if lastUV, ok := alarm.GetTriggerValue("uv"); ok {
		replacements["{{last_uv}}"] = fmt.Sprintf("%d", int(lastUV))
	} else if lastUV, ok := alarm.GetPreviousValue("uv"); ok {
		replacements["{{last_uv}}"] = fmt.Sprintf("%d", int(lastUV))
	} else {
		replacements["{{last_uv}}"] = "N/A"
	}
	if lastRainRate, ok := alarm.GetTriggerValue("rain_rate"); ok {
		replacements["{{last_rain_rate}}"] = fmt.Sprintf("%.2f", lastRainRate)
	} else if lastRainRate, ok := alarm.GetPreviousValue("rain_rate"); ok {
		replacements["{{last_rain_rate}}"] = fmt.Sprintf("%.2f", lastRainRate)
	} else {
		replacements["{{last_rain_rate}}"] = "N/A"
	}
	if lastRainDaily, ok := alarm.GetTriggerValue("rain_daily"); ok {
		replacements["{{last_rain_daily}}"] = fmt.Sprintf("%.2f", lastRainDaily)
	} else if lastRainDaily, ok := alarm.GetPreviousValue("rain_daily"); ok {
		replacements["{{last_rain_daily}}"] = fmt.Sprintf("%.2f", lastRainDaily)
	} else {
		replacements["{{last_rain_daily}}"] = "N/A"
	}
	if lastLightning, ok := alarm.GetTriggerValue("lightning_count"); ok {
		replacements["{{last_lightning_count}}"] = fmt.Sprintf("%d", int(lastLightning))
	} else if lastLightning, ok := alarm.GetPreviousValue("lightning_count"); ok {
		replacements["{{last_lightning_count}}"] = fmt.Sprintf("%d", int(lastLightning))
	} else {
		replacements["{{last_lightning_count}}"] = "N/A"
	}
	if lastLightningDist, ok := alarm.GetTriggerValue("lightning_distance"); ok {
		replacements["{{last_lightning_distance}}"] = fmt.Sprintf("%.1f", lastLightningDist)
	} else if lastLightningDist, ok := alarm.GetPreviousValue("lightning_distance"); ok {
		replacements["{{last_lightning_distance}}"] = fmt.Sprintf("%.1f", lastLightningDist)
	} else {
		replacements["{{last_lightning_distance}}"] = "N/A"
	}

	for placeholder, value := range replacements {
		result = strings.ReplaceAll(result, placeholder, value)
	}

	return result
}
