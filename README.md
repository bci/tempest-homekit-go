# Tempest HomeKit Go

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go)](https://golang.org) [![Test Coverage](https://img.shields.io/badge/coverage-44.2%25-yellow?style=flat)](./coverage.out) [![Build](https://github.com/BEHRConsulting/tempest-homekit-go/actions/workflows/ci.yml/badge.svg)](https://github.com/BEHRConsulting/tempest-homekit-go/actions) [![Release v1.11.0](https://img.shields.io/github/v/tag/BEHRConsulting/tempest-homekit-go?label=release&style=flat)](https://github.com/BEHRConsulting/tempest-homekit-go/releases/tag/v1.11.0) [![License](https://img.shields.io/badge/license-MIT-blue.svg?style=flat)](LICENSE)

Tempest HomeKit Go is a Go service that reads WeatherFlow Tempest station data and exposes sensors to Apple HomeKit while providing a modern web dashboard. It supports UDP stream mode, historical preloading, alarm notifications, and optional device status scraping.

Table of Contents
- [Quick Start](#quick-start)
- [Features](#features)
- [Status Console](#status-console)
- [Configuration](#configuration)
- [Roadmap](#roadmap)
- [Contributing](#contributing)

<!-- Version history moved to VERSIONS.md -->

<!-- Brief research note: detailed methodology moved to docs for readability -->
This project was developed with assistance from AI tools and iterative, research-driven workflows. For details on the development methodology and research notes, see the `docs/` directory.
## Important Sensor Notes

Warning: **HomeKit Sensor Compliance**: Due to HomeKit's limited native sensor types, the **Pressure** and **UV Index** sensors use the standard HomeKit **Light Sensor** service for compliance. In the Home app, these will appear as "Light Sensor" with units showing as "lux" - **please ignore the "lux" unit** for these sensors as they represent atmospheric pressure (mb) and UV index values respectively. This is a HomeKit limitation, not an application issue.

 **Web Console Only Mode**: This application can be run with HomeKit services completely disabled by using the `--disable-homekit` flag. In this mode, only the web dashboard will be available, providing a lightweight weather monitoring solution without HomeKit integration.

## Contributors

- **Kent** - Principal Investigator, Vibe Programming methodology implementation

## Public release notes

This repository is prepared for public GitHub release as a Vibe Programming research project. Key discovery keywords included in this repository are: `vibe`, `macOS`, `HomeKit`, `tempest`, `weather`, `TempestWX`, and `WeatherFlow` to improve discoverability.

- Project status: Work in progress (stable) — feature-complete for the core functionality described in this README, actively maintained and under continued test/coverage improvements.
- Authors: Kent and contributors listed above. This project was developed using Vibe Programming techniques with AI-assisted development tools.

If you use or contribute to this project, please follow the contributing guidelines and include references to the Vibe Programming methodology in PR descriptions when changes are research-related.

## Acknowledgments

We acknowledge the human contributors and AI assistants who supported this project:

- Human contributors: Kent
- AI assistants: Claude Sonnet 3.5, GitHub Copilot (Grok Code Fast 1 preview), GPT-5 mini

### Vibe Programming Methodology Validation

This project represents a controlled experiment in AI-assisted software development, demonstrating the practical application of conversational programming techniques in production software development.

For a concise history of versions and notable changes, see `VERSIONS.md`.

## Features

![Tempest HomeKit Web Console](Tempest-HomeKit-WebConsole.png)

*Modern web dashboard with real-time weather data, interactive charts, and alarm monitoring*

- **Real-time Weather Monitoring**: Continuously polls WeatherFlow Tempest station data every 60 seconds
- **HomeKit Integration**: Individual HomeKit accessories for each weather sensor
- **Multiple Sensor Support**: Temperature, Humidity, Wind Speed, Wind Direction, Rain Accumulation, UV Index, Pressure, and Ambient Light
- **Modern Web Dashboard**: Interactive web interface with real-time updates, unit conversions, and professional styling
 - **External JavaScript Architecture**: Clean separation of concerns with all JavaScript externalized to `script.js`
 - **Interactive Chart Pop-out System**: Advanced data visualization with expandable chart windows
 - **80% Screen Coverage**: Pop-out windows automatically sized to 80% of screen dimensions
 - **Resizable & Draggable**: Native browser window controls for optimal user experience
 - **Complete Historical Data**: Each pop-out displays full 1000+ point datasets with proper legends
 - **Professional Styling**: Gradient backgrounds with clean chart containers and interactive controls
 - **Multi-chart Support**: Temperature, humidity, wind, rain, pressure, light, and UV charts
 - **Pressure Analysis System**: Advanced pressure forecasting with trend analysis and weather predictions
 - **Interactive Info Icons**: Clickable info icons (Info) with detailed tooltips for pressure calculations and sensor explanations
 - **Consistent Positioning**: All tooltips positioned with top-left corner aligned with bottom-right of info icons
 - **Rain Info Icon Fix**: Resolved JavaScript issue where unit updates removed the rain info icon
 - **Proper Event Handling**: Enhanced event propagation control to prevent unit toggle interference
 - **UV Index Display**: Complete UV exposure categories using NCBI reference data with EPA color coding
 - **Interactive Tooltips**: Information tooltips for all sensors with standardized positioning
 - **Accessories Status**: Real-time display of enabled/disabled sensor status in HomeKit bridge card
- **Cross-platform Support**: Runs on macOS, Linux, and Windows with automated service installation
- **TempestWX Device Status Scraping** (Optional):
 - **Headless Browser Integration**: Uses Chrome/Chromium to scrape detailed device status from TempestWX
 - **15-Minute Periodic Updates**: Background scraping with automatic caching
 - **Comprehensive Device Data**: Battery voltage, uptime, signal strength, firmware versions, serial numbers
 - **Multiple Fallback Layers**: Headless browser → HTTP scraping → API fallback for reliability
 - **Data Source Transparency**: Clear indication of data source (web-scraped, http-scraped, api, fallback)
 - **Enable with `--use-web-status` flag**: Optional enhancement for users who want detailed device monitoring
- **UDP Stream Feature** (Offline Mode):
 - **Local Network Monitoring**: Listen for UDP broadcasts from Tempest hub on port 50222
 - **Offline Operation**: Enables weather monitoring during internet outages without API access
 - **Real-time Updates**: Process observation messages as they're broadcast every minute
 - **No API Token Required**: Works entirely on local network without WeatherFlow cloud services
 - **Multiple Message Types**: Supports obs_st (Tempest), obs_air, obs_sky, rapid_wind, device_status, hub_status
 - **Enable with `--udp-stream` flag**: Monitor Tempest station locally without internet connectivity
 - **Full Offline Mode with `--disable-internet`**: Disables all internet access for complete offline operation
- **Flexible Configuration**: Command-line flags and environment variables for easy deployment
- **Enhanced Debug Logging**: Multi-level logging with emoji indicators, calculated values, API calls/responses, and comprehensive DOM debugging

## Roadmap

Planned enhancements and strategic priorities for upcoming releases. Items are grouped by priority and include brief implementation notes and suggested CLI/environment configuration where relevant.

### ✅ Completed Features

**High Priority - Completed:**
- **Alarms and Notifications** ✓ - Fully implemented rule-based alerting system with multiple channels (Email, SMS, Webhook, Console, Syslog, OSLog, EventLog). Supports templated messages, cooldown periods, and interactive web-based alarm editor.
  - CLI/Env: `--alarms @alarms.json` or `--alarms '{...json...}'`. File-watcher for auto-reload.
  - Channels: SMTP/Microsoft 365 OAuth2 email, Twilio/AWS SNS SMS, HTTP webhooks, console logging, syslog, macOS OSLog, Windows EventLog
  - Features: Change detection operators (`*field`, `>field`, `<field>`), template expansion, cooldown management, web console status card

- **Notification Integrations** ✓ - Complete multi-provider support with secure credential management
  - Email: SMTP with TLS + Microsoft 365 OAuth2 (Mail.Send permission)
  - SMS: Twilio (trial/production) + AWS SNS (production-grade)
  - Webhook: HTTP POST with JSON payloads and template expansion
  
  - **Advanced Rules Engine** ✓ - Enhanced alarm capabilities
    - Description: Boolean logic, time windows, rate limiting, complex condition combinations
    - Features: `AND`/`OR` operators, time-based triggers, notification throttling
    - Notes: Extends current condition syntax beyond simple threshold comparisons
  - Testing: `--test-email`, `--test-sms`, `--test-webhook` flags for validation

- **Alarm Editor** ✓ - Modern web-based alarm configuration interface
  - Features: Search/filter, create/edit/delete alarms, visual status, live validation, auto-save
  - Access: `--alarms-edit @alarms.json` starts editor at http://localhost:8081
  - Independent operation with file watching for live reload

### 🔄 In Progress / Medium Priority

**Multi-station Monitoring** - Allow monitoring multiple Tempest stations from single instance
- Description: Each station gets own data source, history buffer, and HomeKit grouping
- CLI/Env: `--stations config.json` or multiple `--station-url` entries with station tagging
- Notes: Requires per-station goroutines, scoped caches, and aggregated UI views

**Container & Serverless Deployment** - Production deployment options
- Docker: Lightweight image with environment variables and docker-compose examples
- AWS Lambda: Serverless handler for data ingestion (HomeKit not supported in serverless)
- Notes: Include example configurations and CI build steps for image publishing

### 🔮 Future / Long-term Goals
 
**Multi-tenant UI** - Role-based access controls for managed deployments
- Description: User isolation, per-tenant station configuration, access controls
- Features: Authentication, tenant-specific dashboards, admin controls
- Notes: Requires database layer for user management and session handling

 

**Database Alarm Delivery** - MariaDB/MySQL integration
- Description: Store alarm events as JSON records in local or remote MariaDB/MySQL databases
- Features: Secure protocol support with fallback to plain text, UUID primary keys, timestamp indexing, configurable connection parameters
- Notes: Requires database credentials in .env file, supports both local and remote database servers

**Contributing / Implementation Notes:**
- **Current Focus**: Multi-station monitoring and container deployment
- **Test Coverage**: Maintain 60%+ coverage with new features
- **Backward Compatibility**: Use feature flags for major changes
- **Documentation**: Update CLI flags and env vars in REQUIREMENTS.md as features are implemented

## Alarms and Notifications

The alarm system enables rule-based weather alerting with multiple notification channels. Configure alarms to trigger when weather conditions meet specific criteria (temperature thresholds, lightning proximity, rain events, etc.).

**Supported Notification Channels:**
- **Console**: Log messages to stdout (always visible regardless of log level)
- **Syslog**: Local or remote syslog server
- **OSLog**: macOS unified logging system (os_log API via CGO)
- **Email**: SMTP or Microsoft 365 OAuth2
- **SMS**: **AWS SNS** (fully implemented) | Twilio (coming soon)
- **Webhook**: HTTP POST with JSON payload and template expansion
- **CSV File**: Log events to CSV files with configurable retention
- **JSON File**: Log events to JSON files with validation and configurable retention
- **EventLog**: System event log (Windows) or syslog (Unix)

**Features:**
- Flexible condition syntax: `temperature > 85`, `humidity > 80 && temperature > 35`, `lux > 10000 && lux < 50000`
- **Change detection operators**: `*field` (any change), `>field` (increase), `<field` (decrease)
 - Example: `*lightning_count` triggers on any lightning strike
 - Example: `>rain_rate` triggers when rain intensifies
 - Example: `<lightning_distance` triggers when lightning gets closer
- **Flexible scheduling**: Restrict alarms to specific times, days, or sunrise/sunset
 - Daily time ranges (e.g., 9 AM to 5 PM)
 - Weekly schedules (e.g., Monday-Friday only)
 - Sunrise/sunset based (e.g., only during daylight hours)
 - See [Alarm Scheduling Documentation](docs/ALARM_SCHEDULING.md)
- Template-based messages with runtime value interpolation (`{{temperature}}`, `{{timestamp}}`, etc.)
- Cooldown periods to prevent notification storms
- Cross-platform file watching for live configuration reloads
- Per-alarm tags for easy filtering and organization
- **Web console alarm status card**: View alarm status, last triggered times, and configuration directly in the dashboard

- **Additional Alarm Delivery Methods** ✓ - CSV and JSON file logging
  - Description: Log alarm events to local CSV or JSON files with configurable retention policies
  - Features: FIFO queue management, configurable file paths and max days retention, fallback to temp files on write failures
  - Notes: Extends notification channels for audit trails and data export

- **Per-Alarm Scheduling System** ✓ - Time-based alarm activation
  - Description: Schedule alarms to be active only during specific time windows
  - Features: Daily/hourly scheduling, sunrise/sunset triggers, days-of-week restrictions, start/end time ranges
  - Notes: Reduces false positives during maintenance windows or off-hours

**Quick Start:**
```bash
# Run with alarm configuration
./tempest-homekit-go --token "your-token" --station "Your Station Name" --alarms @alarms.json

# Test email configuration before deploying
./tempest-homekit-go --email-test --station "Your Station Name" --alarms @alarms.json

# Edit alarm configuration (standalone editor mode)
./tempest-homekit-go --alarms-edit @alarms.json --alarms-edit-port 8081
```

**Example Alarm Configuration Files:**
- `examples/alarms.example.json` - Complete alarm examples (works with any provider)
- `examples/alarms-ms365.example.json` - Same alarms, shows MS365 setup instructions
- `examples/alarms-aws.example.json` - Same alarms, shows AWS SNS setup instructions

**Important:** All email/SMS credentials are configured in `.env` file only - NOT in alarm JSON files! The alarm JSON files contain only alarm rules. Configure your provider credentials in `.env` (see `.env.example` for details), then use any of the alarm example files above.

### Testing Email Configuration

Before deploying alarms in production, test your email configuration:

```bash
./tempest-homekit-go --test-email user@example.com --alarms @alarms.json
```

The email test will:
1. Validate email provider configuration (Microsoft 365 OAuth2 or SMTP)
2. Check all required credentials from `.env` file
3. Send a test email to the specified address with:
 - Application name and version
 - Timestamp and command line options
 - Email configuration details
 - Current weather data from your station
5. Provide troubleshooting guidance if delivery fails

### Testing SMS Configuration

Before deploying SMS alarms in production, test your SMS configuration:

```bash
./tempest-homekit-go --test-sms +15555551234 --alarms @alarms.json
```

The SMS test will:
1. Validate SMS provider configuration (Twilio or AWS SNS)
2. Check all required credentials from `.env` file
3. Send a test SMS to the specified phone number with:
 - Application name and version
 - Timestamp
 - SMS provider information
5. Provide troubleshooting guidance if delivery fails

**Microsoft 365 Setup:**
For complete Microsoft 365 OAuth2 setup instructions, see the detailed comments in `.env.example`. You'll need:
- Azure AD app registration
- Client ID, Client Secret, and Tenant ID
- Mail.Send API permission with admin consent
- From address (`MS365_FROM_ADDRESS`)

**SMTP Setup:**
For generic SMTP providers (Gmail, SendGrid, Mailgun, etc.), configure:
- `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD`
- `SMTP_FROM_ADDRESS`, `SMTP_USE_TLS=true`

See `.env.example` for provider-specific examples with standard ports and TLS settings.

**AWS SNS SMS Setup:**
For AWS SNS configuration (recommended for production SMS), see detailed setup instructions in `.env.example`. Quick setup:
1. Create IAM user with `sns:Publish` permission (principle of least privilege)
2. Configure credentials in `.env`: `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_REGION`
3. Run the automated setup script: `./scripts/setup-aws-sns.sh`
4. The script will:
 - Verify AWS CLI credentials (uses your admin credentials from `~/.aws/`)
 - Configure production SMS settings (type, spending limits)
 - Optionally create SNS topics with subscriptions
 - Update `.env` with Topic ARN automatically
 - Send test SMS for verification

**Important**: The AWS credentials in `.env` are for the **application runtime user** (limited permissions). The setup script uses your **admin AWS CLI credentials** from `~/.aws/credentials` or `aws configure`.

**Twilio SMS Setup:**
For Twilio SMS configuration (great for development and moderate volume), see detailed setup instructions in `.env.example`. Quick setup:
1. Sign up for Twilio: https://www.twilio.com/try-twilio (get $15 trial credit)
2. Get credentials from Twilio Console: https://console.twilio.com/
 - Account SID (starts with "AC")
 - Auth Token (click "Show" to reveal)
3. Purchase a phone number with SMS capability from the Twilio Console
4. Configure in `.env`: `TWILIO_ACCOUNT_SID`, `TWILIO_AUTH_TOKEN`, `TWILIO_FROM_NUMBER` (E.164 format: +1XXXXXXXXXX)
5. Test configuration: `./tempest-homekit-go --test-sms +15555551234 --alarms @alarms.json`

**Note**: Trial accounts can only send to verified phone numbers. To verify a number:
1. Go to Twilio Console → Phone Numbers → Verified Caller IDs
2. Click "+" to add a new number
3. Enter the number and verify via SMS or call

**Pricing**: ~$0.0079 per message (US), ~$1/month for phone number. Upgrade to paid account for unrestricted sending.

## Status Console

The status console provides a real-time terminal-based UI for monitoring your Tempest station without opening a web browser. It displays live weather data, logs, station status, alarm information, and HomeKit status in a responsive multi-panel interface.

### Features

- **Real-time Updates**: Auto-refresh every 5 seconds (configurable)
- **Multi-Panel Layout**: 7 separate windows showing:
  - **Console Logs**: Scrolling log output with color-coded messages
  - **Tempest Sensors**: Current sensor readings (temperature, humidity, wind, pressure, UV, light, rain)
  - **Station Status**: Device and hub information (battery, uptime, signal strength, firmware)
  - **Alarm Status**: Triggered and cooling down alarms with timestamps
  - **HomeKit Status**: Active/disabled status with published sensors
  - **System Info**: Application metadata and runtime information
  - **Footer**: Running time, refresh countdown, current theme, keyboard shortcuts
- **12 Color Themes**: 6 dark and 6 light themes optimized for terminal readability
- **Smart Log Colorization**: Automatic color coding for ERROR, WARN, INFO, DEBUG messages
- **Responsive Layout**: Adapts to terminal size changes automatically
- **Keyboard Controls**: Interactive controls for refresh, quit, and theme cycling
- **Optional Timeout**: Auto-exit after specified duration

### Quick Start

```bash
# Start status console with default settings (5 second refresh)
./tempest-homekit-go --status --token "your-token" --station "Your Station"

# Custom refresh interval (10 seconds)
./tempest-homekit-go --status --status-refresh 10 --token "your-token" --station "Your Station"

# With timeout (exit after 5 minutes)
./tempest-homekit-go --status --status-timeout 300 --token "your-token" --station "Your Station"

# With specific theme
./tempest-homekit-go --status --status-theme dark-ocean --token "your-token" --station "Your Station"

# List all available themes
./tempest-homekit-go --status-theme-list
```

### Configuration Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--status` | `false` | Enable status console mode |
| `--status-refresh` | `5` | Refresh interval in seconds |
| `--status-timeout` | `0` | Auto-exit timeout in seconds (0=never) |
| `--status-theme` | `dark-ocean` | Color theme name |
| `--status-theme-list` | - | List all themes and exit |

### Environment Variables

```bash
# In .env file
STATUS=true
STATUS_REFRESH=10
STATUS_TIMEOUT=300
STATUS_THEME=dark-forest
```

### Available Themes

**Dark Themes** (optimized for dark terminal backgrounds):
- `dark-ocean` (default) - Deep blue with cyan accents
- `dark-forest` - Forest green with emerald highlights
- `dark-sunset` - Warm amber and orange tones
- `dark-twilight` - Purple and lavender palette
- `dark-matrix` - Classic green terminal style
- `dark-cyberpunk` - Neon pink and cyan accents

**Light Themes** (optimized for light terminal backgrounds):
- `light-sky` - Sky blue with navy accents
- `light-garden` - Olive green with earth tones
- `light-autumn` - Rust orange and brown palette
- `light-lavender` - Soft purple and pink tones
- `light-monochrome` - Clean black and gray
- `light-ocean` - Teal and aqua accents

### Keyboard Controls

| Key | Action |
|-----|--------|
| `q` or `Q` | Quit the status console |
| `r` or `R` | Refresh immediately (reset countdown) |
| `t` or `T` | Cycle to next theme |
| `ESC` | Quit the status console |
| `Ctrl-C` | Quit the status console |

### Display Panels

#### Console Logs
- Captures application log output in real-time
- Color-coded by log level (ERROR=red, WARN=yellow, INFO=green, DEBUG=cyan)
- Scrolls automatically to show latest messages
- Strips ANSI escape sequences for clean display

#### Tempest Sensors
- Current temperature (°F or °C)
- Relative humidity (%)
- Wind speed and direction
- Atmospheric pressure (mb or inHg)
- UV index
- Light level (lux)
- Rain accumulation (daily and rate)
- Lightning information

#### Station Status
- Device battery voltage and status
- Device and hub uptime
- Signal strength (RSSI)
- Firmware versions
- Serial numbers
- Data source indicator (API, web-scraped, UDP)

#### Alarm Status
- **Triggered Alarms**: Active alarms with trigger timestamps
- **Cooling Down**: Alarms in cooldown with remaining time
- Alarm count summary (enabled/total)
- Configuration file path and last reload time

#### HomeKit Status
- Service status (Active/Disabled)
- Published sensors list when active
- PIN and accessory information
- Bridge status

#### System Info
- Application name and version
- Station name
- Current units (imperial/metric/sae)
- Log level

#### Footer
- Running time (hh:mm:ss format)
- Refresh countdown (hh:mm:ss format)
- Current theme name
- Keyboard shortcuts reminder

### Technical Details

**Implementation:**
- Built with `tview` (Go terminal UI framework)
- Non-blocking UI updates using `app.Draw()`
- Goroutines for auto-refresh and timer updates
- Context-based cancellation for clean shutdown
- Synchronized state management with `sync.Mutex`
- HTTP API polling with 500ms timeout
- Log capture via `io.Pipe` redirection

**API Endpoints Used:**
- `/api/weather` - Current weather data
- `/api/status` - Station and HomeKit status
- `/api/alarm-status` - Alarm information

**Performance:**
- Minimal CPU usage (< 1% on modern systems)
- Low memory footprint (< 5MB additional)
- Responsive to terminal resize events
- Graceful handling of API timeouts

### Examples

```bash
# Monitor station with custom theme and refresh
./tempest-homekit-go --status --status-theme dark-cyberpunk --status-refresh 3 \
  --token "your-token" --station "Your Station"

# Quick check with 2-minute timeout
./tempest-homekit-go --status --status-timeout 120 \
  --token "your-token" --station "Your Station"

# Light theme for bright terminals
./tempest-homekit-go --status --status-theme light-sky \
  --token "your-token" --station "Your Station"

# Combine with alarm monitoring
./tempest-homekit-go --status --alarms @alarms.json \
  --token "your-token" --station "Your Station"
```

### Troubleshooting

**Console not updating:**
- Check API token and station name are correct
- Verify network connectivity
- Increase `--status-refresh` if API is rate-limited

**Colors look wrong:**
- Try a different theme with `t` key
- Use `--status-theme-list` to preview all themes
- Light themes for light backgrounds, dark themes for dark backgrounds

**Terminal too small:**
- Resize terminal to at least 80x24 characters
- Status console adapts to available space automatically

**Logs not showing:**
- Status console captures logs from the main application
- Adjust `--loglevel` flag for more/less verbosity
- Console logs show regardless of log level setting

### Testing Webhook Configuration

Before deploying webhook alarms in production, test your webhook configuration:

```bash
./tempest-homekit-go --test-webhook https://webhook.site/your-test-url --alarms @alarms.json
```

The webhook test will:
1. Validate webhook URL format and accessibility
2. Send a test HTTP POST request with JSON payload
3. Include application metadata, timestamp, and current weather data
4. Display response status and troubleshooting guidance if delivery fails

**Webhook Payload Example:**
```json
{
 "test": true,
 "timestamp": "2025-01-20T15:30:45Z",
 "application": "tempest-homekit-go",
 "version": "1.0.0",
 "station": "Chino Hills",
 "weather": {
 "temperature": 72.5,
 "humidity": 65,
 "wind_speed": 5.2,
 "wind_direction": "SW",
 "pressure": 29.92,
 "uv_index": 3,
 "lightning_distance": 25,
 "rain_rate": 0.0
 }
}
```

### File-Based Delivery Methods (CSV and JSON)

The alarm system supports logging alarm events to local CSV and JSON files with configurable retention policies. This is useful for audit trails, data export, and integration with other monitoring systems.

**CSV File Delivery:**
- Logs alarm events as CSV records with timestamp, alarm information, and sensor data
- Configurable file path and maximum retention days (0 = unlimited)
- Default path: `/tmp/tempest-alarms.csv` (macOS/Linux) or `%TEMP%\tempest-alarms.csv` (Windows)
- FIFO queue management - automatically rotates files when max days is reached
- Fallback to temporary files if the configured path cannot be opened (handles permission issues or locked files)

**JSON File Delivery:**
- Logs alarm events as structured JSON objects with timestamp, message, alarm info, and sensor data
- Configurable file path and maximum retention days (0 = unlimited)
- Default path: `/tmp/tempest-alarms.json` (macOS/Linux) or `%TEMP%\tempest-alarms.json` (Windows)
- Built-in JSON validation in the alarm editor before saving
- Fallback to temporary files if the configured path cannot be opened

**Important Notes for Temporary Directories:**
- **macOS**: Files in `/tmp` are automatically deleted at system boot and may be cleaned up by the system when needed
- **Windows**: Files in `%TEMP%` are periodically cleaned up by the system and disk cleanup utilities
- **Production Use**: For persistent logging, configure custom file paths outside of temporary directories
- **Fallback Behavior**: If the configured file cannot be opened (permissions, locked by another process), the system will log a warning and create a temporary file to ensure alarm delivery continues

**Example CSV Output:**
```
2025-01-20 15:30:45,High Temperature,High temperature detected: 85F,Temperature exceeds threshold,temperature,85.0,humidity,65.0,pressure,1013.25,wind_speed,5.2,lux,50000,uv,6,rain_daily,2.5
```

**Example JSON Output:**
```json
{
  "timestamp": "2025-01-20T15:30:45Z",
  "message": "ALARM: High Temperature triggered",
  "alarm": {
    "name": "High Temperature",
    "description": "High temperature detected: 85F",
    "condition": "temperature > 85",
    "enabled": true,
    "triggered_count": 1
  },
  "sensors": {
    "temperature": 85.0,
    "humidity": 65.0,
    "pressure": 1013.25,
    "wind_speed": 5.2,
    "lux": 50000,
    "uv": 6,
    "rain_daily": 2.5
  }
}
```

### Using the Alarm Editor

The alarm editor provides a modern web interface for managing alarm configurations:

1. **Start the editor:**
 ```bash
 ./tempest-homekit-go --alarms-edit @alarms.json
 ```

2. **Open your browser to** `http://localhost:8081` (or custom port with `--alarms-edit-port`)

3. **Editor features:**
 - **Search & filter**: Find alarms by name or tag
 - **Create alarms**: Click "New Alarm" button to add alarms
 - **Edit alarms**: Click "Edit" on any alarm card
 - **Delete alarms**: Click "Delete" on any alarm card
 - **Visual status**: Green dot = enabled, red dot = disabled
 - **Live validation**: Conditions are validated before saving
 - **Auto-save**: Changes saved immediately to JSON file

4. **Alarm form fields:**
 - **Name**: Unique identifier (required)
 - **Description**: Optional description
 - **Condition**: Expression like `temperature > 85` or `humidity > 80 && temperature > 35` (required)
 - **Tags**: Comma-separated tags for organization
 - **Cooldown**: Seconds before alarm can fire again (default: 1800)
 - **Enabled**: Toggle alarm on/off

The editor operates independently from the main service and saves changes directly to your alarm configuration file. If the main service is running with `--alarms`, it will automatically detect and reload the configuration when changes are saved.

### Web Console Alarm Status

When running the main service with alarms enabled, the web dashboard (`http://localhost:8080`) automatically displays an **Alarm Status** card showing:

- **System Status**: Active/Not Configured indicator
- **Configuration File**: Name of the alarm configuration file being monitored
- **Last Read**: Timestamp when the configuration was last loaded (updates only on file changes)
- **Alarm Counts**: Number of enabled alarms vs total alarms
- **Active Alarms List**: Details for each enabled alarm:
 - Alarm name and condition
 - Last triggered timestamp (or "Never")
 - Delivery channels (console, syslog, oslog, email, SMS, webhook, eventlog)

The alarm status refreshes automatically every 10 seconds, providing real-time visibility into your alarm system without needing to open the alarm editor or check log files.

**Example:**
```bash
# Start service with alarms - web console will show alarm status card
./tempest-homekit-go --token "your-token" --station "Your Station Name" --alarms @tempest-alarms.json

# Open http://localhost:8080 to view dashboard with alarm status
```

## Quick Start

Note: When using the WeatherFlow API token (`--token` or the `TEMPEST_TOKEN` env var), you must also specify the station name with `--station "Your Station Name"` or set `TEMPEST_STATION_NAME`. Examples in this README have been updated to include `--station` where `--token` is used.

### Prerequisites
- Go 1.24 or later (go.mod: `go 1.24.0`)
- WeatherFlow Tempest station with API access
- Apple device with HomeKit support
- Google Chrome (optional, for detailed device status via `--use-web-status`)

### Build and Run
```bash
git clone https://github.com/BEHRConsulting/tempest-homekit-go.git
cd tempest-homekit-go
go build
./tempest-homekit-go --token "your-api-token" --station "Your Station Name"
```

### Test with Generated Weather
```bash
# Traditional approach
./tempest-homekit-go --use-generated-weather

# New flexible station URL approach
./tempest-homekit-go --station-url http://localhost:8080/api/generate-weather

# Using environment variable (equivalent to above)
STATION_URL=http://localhost:8080/api/generate-weather ./tempest-homekit-go

# With historical data preloading (preloads up to HISTORY_POINTS observations)
./tempest-homekit-go --use-generated-weather --history-read # preloads up to HISTORY_POINTS observations
```

### Build Script (Current Platform)
```bash
./scripts/build.sh
```
For binaries for every platform use `./scripts/build-cross-platform.sh` (see [Installation](#installation)).

### Install as System Service
```bash
sudo ./scripts/install-service.sh --token "your-api-token" --station "Your Station Name"
```

## Installation

### Option 1: Build from Source
```bash
git clone https://github.com/BEHRConsulting/tempest-homekit-go.git
cd tempest-homekit-go
go mod tidy
go build -o tempest-homekit-go
```

### Option 2: Platform-Specific Build (Current Platform Only)
```bash
./scripts/build.sh
```
This builds only for your current platform (macOS binaries on macOS, Linux on Linux, etc.).

### Option 3: Cross-Platform Build (All Platforms)
```bash
./scripts/build-cross-platform.sh
```
This builds optimized binaries for Linux, macOS, and Windows from any platform.

### Option 4: Install as Service
For production deployment, install as a system service:
```bash
# Linux (systemd)
sudo ./scripts/install-service.sh --token "your-api-token" --station "Your Station Name"

# macOS (launchd)
sudo ./scripts/install-service.sh --token "your-api-token" --station "Your Station Name"

# Windows (NSSM)
./scripts/install-service.sh --token "your-api-token" --station "Your Station Name"
```

### Dependencies
- `github.com/brutella/hap` - Modern HomeKit Accessory Protocol implementation (v0.0.32)
- `github.com/chromedp/chromedp` - Headless browser automation for TempestWX status scraping
- Custom weather services with unique UUIDs to prevent temperature conversion issues

## Usage

### Basic Usage
If you are using the WeatherFlow Tempest API (default behavior), provide your API token with `--token` or the `TEMPEST_TOKEN` environment variable. If you instead use a custom station URL via `--station-url` or enable generated weather with `--use-generated-weather`, a WeatherFlow API token is not required.

```bash
# WeatherFlow API (requires token)
./tempest-homekit-go --token "your-weatherflow-token" --station "Your Station Name"

# Custom station URL (no WeatherFlow token required)
./tempest-homekit-go --station-url http://localhost:8080/api/generate-weather

# Generated weather (no WeatherFlow token required)
./tempest-homekit-go --use-generated-weather
```

### Configuration Options

#### Command-Line Flags (alphabetical order)
- `--alarms`: Alarm configuration: @filename.json or inline JSON string (default: none). Env: ALARMS
- `--alarms-edit`: Run alarm editor for specified config file: @filename.json (default: none)
- `--alarms-edit-port`: Port for alarm editor web UI (default: 8081). Env: ALARMS_EDIT_PORT
- `--cleardb`: Clear HomeKit database and reset device pairing
- `--disable-alarms`: Disable alarm initialization and processing (useful for testing or reducing resource usage)
- `--disable-homekit`: Disable HomeKit services and run web console only
- `--elevation`: Station elevation in meters (default: auto-detect, valid range: -430m to 8848m)
- `--env`: Custom environment file to load (default: ".env"). Env: ENV_FILE
    - Overrides the default `.env` file location
    - Useful for multiple configurations or deployment environments
    - Example: `./tempest-homekit-go --env /etc/tempest/production.env`
- `--loglevel`: Logging level - debug, info, warn/warning, error (default: "error")
- `--logfilter`: Filter log messages to only show those containing this string (case-insensitive) - useful for targeted debugging
- `--pin`: HomeKit pairing PIN (default: "00102003") 
- `--sensors`: Sensors to enable - 'all', 'min' (temp,lux,humidity), or comma-delimited list with aliases supported:
    - **Temperature**: `temp` or `temperature`
    - **Light**: `lux` or `light`
    - **UV**: `uv` or `uvi`
    - **Other sensors**: `humidity`, `wind`, `rain`, `pressure`, `lightning`
    - (default: "temp,lux,humidity")
- `--station`: Tempest station name (default: "Chino Hills")
- `--station-url`: Custom station URL for weather data (e.g., `http://localhost:8080/api/generate-weather`). Overrides Tempest API
- `--history <points>`: Number of data points to store in history (default: 1000, min: 10). Env: `HISTORY_POINTS`
- `--history-read`: Preload historical observations from Tempest API up to `HISTORY_POINTS` (bool). Env: `READ_HISTORY`
- `--history-reduce <factor>`: Reduce historical data by averaging N points into 1 (default: 1 = no reduction). Env: `HISTORY_REDUCE`
- `--history-reduce-method <method>`: Method to reduce historical data: `timebin` (default), `factor`, `lttb`. Env: `HISTORY_REDUCE_METHOD`
- `--history-bin-size <minutes>`: Bin size in minutes for timebin reduction (default: 10). Env: `HISTORY_BIN_MINUTES`
- `--history-keep-recent-hours <hours>`: Keep recent N hours of data at full resolution when reducing history (default: 24). Env: `HISTORY_KEEP_RECENT_HOURS`
- `--chart-history <hours>`: Number of hours of data to show in charts (default: 24, 0=all). Env: `CHART_HISTORY_HOURS`
- `--generate-path <path>`: Path for generated weather endpoint (default: `/api/generate-weather`). Env: `GENERATE_WEATHER_PATH`
- `--status`: Enable terminal-based status console with real-time monitoring
- `--status-refresh`: Status console refresh interval in seconds (default: 5)
- `--status-timeout`: Status console auto-exit timeout in seconds, 0=never (default: 0)
- `--status-theme`: Status console color theme name (default: "dark-ocean")
- `--status-theme-list`: List all available status console themes and exit
-- `--token`: WeatherFlow API access token (required when using the WeatherFlow API as the data source)
- `--units`: Units system - imperial, metric, or sae (default: "imperial")
- `--units-pressure`: Pressure units - inHg or mb (default: "inHg")
- `--udp-stream`: Enable UDP broadcast listener for local station monitoring (port 50222)
- `--disable-internet`: **Offline Mode** - Disables all internet connectivity for complete offline operation
    - **Requires**: `--udp-stream` or `--use-generated-weather` (must have a local data source)
    - **Incompatible with**: `--use-web-status`, `--history-read` (both require internet access)
    - **Use Case**: Internet outages, air-gapped systems, privacy-focused deployments, testing without network
    - **Limitations**: No forecast data, no historical preloading, no web scraping
- `--disable-webconsole`: **HomeKit Only Mode** - Disables the web dashboard server
    - **Incompatible with**: `--disable-homekit` (cannot disable both HomeKit and web console)
    - **Use Case**: Minimal resource usage, HomeKit-only deployments, reduced attack surface
- `--use-generated-weather`: Use simulated weather data for testing (automatically sets station-url)
- `--use-web-status`: Enable headless browser scraping of TempestWX status page every 15 minutes (requires Chrome, incompatible with `--disable-internet`)
- `--version`: Show version information and exit
- `--webhook-listener`: Start webhook listener server on port 8082 (or custom port) to receive and inspect webhook requests
- `--web-port`: Web dashboard port (default: "8080")

#### Environment Variables
Environment variables are documented in the "Available Environment Variables" table below. Refer to that table for defaults and descriptions, e.g. `HISTORY_POINTS`, `STATUS_REFRESH`, `TEMPEST_TOKEN`, and others.

### Example with Full Configuration
```bash
./tempest-homekit-go \
 --token "your-api-token" \
 --station "Your Station Name" \
 --pin "12345678" \
 --web-port 8080 \
 --loglevel info \
 --sensors "temp,humidity,lux,uv,pressure" \
 --elevation 150 \
 --use-web-status
```

### Sensor Configuration Examples
```bash
# Using sensor aliases (recommended for readability)
./tempest-homekit-go --token "your-token" --station "Your Station Name" --sensors "temperature,light,uvi"

# Traditional sensor names (also supported)
./tempest-homekit-go --token "your-token" --station "Your Station Name" --sensors "temp,lux,uv"

# Mixed aliases and traditional names
./tempest-homekit-go --token "your-token" --station "Your Station Name" --sensors "temperature,humidity,light,wind"

# All available sensors
./tempest-homekit-go --token "your-token" --station "Your Station Name" --sensors "all"

# Minimal sensor set
./tempest-homekit-go --token "your-token" --station "Your Station Name" --sensors "min"
```

### Offline Mode Examples
```bash
# Valid: Full offline mode with UDP stream (real station)
./tempest-homekit-go --token "your-token" --station "Your Station Name" --udp-stream --disable-internet

# Valid: Full offline mode with generated weather (testing/simulation)
./tempest-homekit-go --disable-internet --use-generated-weather

# Valid: Offline with custom sensors
./tempest-homekit-go --token "your-token" --station "Your Station Name" --udp-stream --disable-internet --sensors "temp,humidity,pressure"

# Invalid: Missing data source
./tempest-homekit-go --token "your-token" --station "Your Station Name" --disable-internet
# ERROR: --disable-internet mode requires --udp-stream or --use-generated-weather (need a local data source)

# Invalid: Can't use web scraping in offline mode
./tempest-homekit-go --token "your-token" --station "Your Station Name" --udp-stream --disable-internet --use-web-status
# ERROR: --use-web-status cannot be used with --disable-internet (requires internet access)

# Invalid: Can't preload history in offline mode
./tempest-homekit-go --token "your-token" --station "Your Station Name" --udp-stream --disable-internet --history-read
# ERROR: --history-read cannot be used with --disable-internet (requires WeatherFlow API access)
```

### HomeKit Only Mode Examples
```bash
# Valid: HomeKit only, no web console
./tempest-homekit-go --token "your-token" --station "Your Station Name" --disable-webconsole

# Valid: Offline HomeKit with UDP stream, no web console
./tempest-homekit-go --token "your-token" --station "Your Station Name" --udp-stream --disable-internet --disable-webconsole

# Invalid: Can't disable both HomeKit and web console
./tempest-homekit-go --token "your-token" --station "Your Station Name" --disable-homekit --disable-webconsole
# ERROR: cannot disable both HomeKit (--disable-homekit) and web console (--disable-webconsole) - at least one service must be enabled
```

### Validation Examples
```bash
# Invalid elevation (too high) - shows helpful error message
./tempest-homekit-go --token "your-token" --station "Your Station Name" --elevation 10000
# Error: elevation must be between -430m and 8848m (Earth's surface range)

# Invalid sensor name - shows available options
./tempest-homekit-go --token "your-token" --station "Your Station Name" --sensors "invalid-sensor"
# Error: invalid sensor 'invalid-sensor'. Available: temp/temperature, lux/light, uv/uvi, humidity, wind, rain, pressure, lightning

# Missing required token - shows usage
./tempest-homekit-go --sensors "temp"
# Error: WeatherFlow API token is required. Use --token flag or TEMPEST_TOKEN environment variable
```

### Web Console Only (No HomeKit)
```bash
# Run web dashboard only without HomeKit services
./tempest-homekit-go \
 --token "your-api-token" \
 --station "Your Station Name" \
 --disable-homekit \
 --web-port 8080 \
 --loglevel info
```

### TempestWX Device Status Scraping

Enable detailed device status monitoring with the `--use-web-status` flag:

```bash
# Basic usage with device status scraping
./tempest-homekit-go --token "your-token" --station "Your Station Name" --use-web-status

# With full configuration
./tempest-homekit-go --token "your-token" --station "Your Station Name" --use-web-status --loglevel debug
```

**Requirements:**
- Google Chrome or Chromium installed
- Internet access to https://tempestwx.com

**What it provides:**
- **Battery Status**: Real battery voltage (e.g., "2.69V") and condition (Good/Fair/Poor)
- **Device Uptime**: How long your Tempest device has been running
- **Hub Uptime**: How long your Tempest hub has been running - **Signal Strength**: Wi-Fi signal strength for hub, device signal strength
- **Firmware Versions**: Current firmware for both hub and device
- **Serial Numbers**: Hardware serial numbers for troubleshooting
- **Last Activity**: Timestamps of last status updates and observations

**Status API Response with Web Scraping:**
```json
{
 "stationStatus": {
 "batteryVoltage": "2.69V",
 "batteryStatus": "Good",
 "deviceUptime": "128d 6h 19m 29s",
 "hubUptime": "63d 15h 55m 1s",
 "hubWiFiSignal": "Strong (-42)",
 "deviceSignal": "Good (-65)",
 "hubSerialNumber": "HB-00168934",
 "deviceSerialNumber": "ST-00163375",
 "hubFirmware": "v329",
 "deviceFirmware": "v179",
 "dataSource": "web-scraped",
 "lastScraped": "2025-09-18T03:15:30Z",
 "scrapingEnabled": true
 }
}
```

**Without `--use-web-status` (default):**
Basic status with API-only data:
```json
{
 "stationStatus": {
 "batteryVoltage": "--",
 "dataSource": "api",
 "scrapingEnabled": false
 }
}
```

**How it works:**
1. **Headless Browser**: Launches Chrome to load the TempestWX status page
2. **JavaScript Execution**: Waits for JavaScript to populate the device status data
3. **Data Extraction**: Parses the loaded content to extract device information
4. **15-Minute Updates**: Automatically refreshes data every 15 minutes
5. **Graceful Fallbacks**: Falls back to HTTP scraping, then API-only if issues occur

### UDP Stream (Offline Mode)

The UDP stream feature enables local monitoring of your Tempest station without requiring internet connectivity. This is particularly useful during internet outages when you still need weather data for HomeKit automations.

**Use Case: Internet Outage Resilience**

When your internet connection goes down, the WeatherFlow API becomes unavailable. With UDP streaming enabled, your Tempest hub broadcasts weather observations on your local network, allowing continuous monitoring without cloud access.

#### Operation Modes

**1. Hybrid Mode (UDP + Internet)**
```bash
# UDP for real-time observations, API for forecast/history (recommended for most users)
./tempest-homekit-go --udp-stream --token "your-token" --station "Your Station Name"

# Add historical data preloading from API
./tempest-homekit-go --udp-stream --history-read --token "your-token" --station "Your Station Name" # preloads up to HISTORY_POINTS observations

# Enable UDP status updates in web console (battery, RSSI, uptime, firmware)
./tempest-homekit-go --udp-stream --token "your-token" --station "Your Station Name"
```
- Real-time UDP observations every 60 seconds
- Forecast data from WeatherFlow API
- Historical data preloading available
- Device/hub status from UDP broadcasts (battery, signal strength, uptime, firmware)
- Info: Status updates checked every 30 seconds from UDP data

**2. Full Offline Mode (UDP Only)**
```bash
# Complete offline operation - no internet access at all
./tempest-homekit-go --udp-stream --disable-internet

# Offline mode with custom sensors and debug logging
./tempest-homekit-go --udp-stream --disable-internet --sensors "temp,humidity,lux,wind" --loglevel debug
```
- Real-time UDP observations only
- Device/hub status from UDP broadcasts (battery, signal, uptime, firmware)
- No forecast data
 - No historical data preloading (`--history-read` not allowed)
- No web scraping (`--use-web-status` not allowed - but UDP status still works)
- Zero internet dependency - works during complete outages
- Info: API token (`--token`) still required but not used for network calls

**3. HomeKit Only Mode (No Web Console)**
```bash
# HomeKit accessories only, disable web dashboard
./tempest-homekit-go --token "your-token" --station "Your Station Name" --disable-webconsole

# HomeKit only with offline mode
./tempest-homekit-go --token "your-token" --station "Your Station Name" --udp-stream --disable-internet --disable-webconsole
```
- HomeKit accessories enabled
- Web dashboard disabled (port not opened)
- Reduced resource usage
- Minimal attack surface

#### Configuration Validation

The `--disable-internet` flag enforces strict validation to prevent conflicting configurations:

| Configuration | Result | Reason |
|--------------|--------|--------|
| `--disable-internet` alone | **ERROR** | Requires local data source |
| `--disable-internet --udp-stream` | **Valid** | Pure offline mode with real station |
| `--disable-internet --use-generated-weather` | **Valid** | Pure offline mode with simulated data |
| `--disable-internet --use-web-status` | **ERROR** | Web scraping requires internet access |
| `--disable-internet --history-read` | **ERROR** | Historical data requires WeatherFlow API |
| `--disable-internet --udp-stream --history-read` | **ERROR** | History requires API calls |
| `--disable-homekit --disable-webconsole` | **ERROR** | At least one service must be enabled |

**Error Messages:**
```
ERROR: --disable-internet mode requires --udp-stream or --use-generated-weather (need a local data source)
ERROR: --use-web-status cannot be used with --disable-internet (requires internet access)
ERROR: --history-read cannot be used with --disable-internet (requires WeatherFlow API access)
```

#### Network Requirements

**Hardware:**
- Tempest hub and monitoring device must be on the same local network
- UDP port 50222 must be accessible (no firewall blocking)
- Hub broadcasts observations every 60 seconds

**What it provides:**
- **Real-time Observations**: Temperature, humidity, wind, pressure, UV, rain, lightning data
- **Device Status**: Battery voltage, RSSI, sensor status
- **Hub Status**: Firmware version, uptime, reset flags
- **No Internet Required**: Complete offline operation with `--disable-internet` flag

**Network Topology:**
- Both devices on same subnet (hub broadcasts to 255.255.255.255)
- No special router configuration needed for standard LAN setups

**UDP Status Integration:**
When `--udp-stream` is enabled, device and hub status is automatically populated from UDP broadcasts:
- **Device Status**: Battery voltage (with Good/Fair/Low indicators), uptime, RSSI signal strength, sensor status, serial number
- **Hub Status**: Firmware version, uptime, WiFi RSSI, reset flags, serial number
- **Update Frequency**: Status checked every 30 seconds from UDP data
- **Web Console**: Status API shows `"dataSource": "udp"` when populated from UDP broadcasts
- **No Web Scraping Needed**: UDP provides real-time status without `--use-web-status` flag

**Status API Response with UDP Stream:**
```json
{
 "udpStatus": {
 "enabled": true,
 "receivingData": true,
 "packetCount": 147,
 "stationIP": "192.168.1.50",
 "serialNumber": "ST-00163375",
 "lastPacketTime": "2025-01-20T15:30:45Z"
 }
}
```

**Limitations:**
- No forecast data in full offline mode (`--disable-internet`)
- Historical data limited to observations received since startup
- Requires hub on local network (won't work remotely)

## Testing

The application includes comprehensive testing flags for validating configurations and troubleshooting issues before deployment.

### Testing Flags

#### API and Data Source Testing

**Test WeatherFlow API Endpoints** (`--test-api`)
```bash
./tempest-homekit-go --test-api
```
Tests all WeatherFlow API endpoints:
- Station discovery and details
- Current observations
- Historical data retrieval
- Performance metrics

**Test Local Web Server API Endpoints** (`--test-api-local`)
```bash
# Test with default port 8084 (avoids conflicts with running service)
./tempest-homekit-go --test-api-local --use-generated-weather

# Test with custom port
./tempest-homekit-go --test-api-local --web-port 9090 --use-generated-weather

# Test with debug output
./tempest-homekit-go --test-api-local --use-generated-weather --loglevel debug
```
Tests all local web server API endpoints:
- **Standalone Test Mode**: Runs in isolation on port 8084 by default (avoids conflicts with port 8080)
- **No HomeKit**: HomeKit services automatically disabled for testing
- **No Alarms**: Alarm system automatically disabled for testing
- **Clean Output**: Service logs suppressed unless `--loglevel debug` is specified
- **Endpoints Tested**: /api/weather, /api/status, /api/alarm-status, /api/history, /api/units, /api/generate-weather
- **Custom Port**: Override default port with `--web-port` flag
- **Use Cases**: Validate API responses, test web integrations, debug endpoint issues without affecting running service

**Test UDP Broadcast Listener** (`--test-udp [seconds]`)
```bash
# Listen for 120 seconds (default)
./tempest-homekit-go --test-udp

# Listen for custom duration
./tempest-homekit-go --test-udp 30

# With debug logging for detailed packet info
./tempest-homekit-go --test-udp 60 --loglevel debug
```
Tests UDP broadcast reception:
- Listens on port 50222 for Tempest station broadcasts
- **Pretty-prints packets in real-time** as they arrive (obs_st, obs_air, obs_sky, rapid_wind, evt_precip, evt_strike, device_status, hub_status)
- Displays periodic statistics every 5 seconds (total packets, new packets, station info)
- Shows latest observation data when complete
- With `--loglevel debug`, shows detailed packet information
- Helps diagnose network/firewall issues

#### Notification Delivery Testing

**Test Email Delivery** (`--test-email <email>`)
```bash
./tempest-homekit-go --test-email user@example.com --alarms @alarms.json
```
Tests email notification delivery:
- Auto-detects provider (Microsoft 365 OAuth2 or SMTP)
- Validates credentials from environment variables
- Sends test email with weather data
- Uses real delivery path (factory pattern)

**Test Webhook Delivery** (`--test-webhook <url>`)
```bash
./tempest-homekit-go --test-webhook https://webhook.site/your-test-url --alarms @alarms.json
```
Tests webhook notification delivery:
- Validates webhook URL and configuration
- Sends test HTTP POST request with JSON payload
- Includes weather data and alarm information
- Uses real delivery path (factory pattern)

**Test Console Notifications** (`--test-console`)
```bash
./tempest-homekit-go --test-console --alarms @alarms.json
```
Tests console/stdout notification delivery.

**Test Historical Coverage** (`--test-history`)
```bash
./tempest-homekit-go --test-history --token "your-token" --station "Your Station"
```
Fetches as much historical observation data as the WeatherFlow API will return and prints a short report:
- Lists the starting timestamp for each 500-point block (newest-first)
- Shows total points fetched and the time range covered
- Useful to validate historical coverage and detect gaps in the API data

Note: `--test-history` requires a valid `--token` and `--station` name and will exit after printing the report.

**Test Syslog Notifications** (`--test-syslog`)
```bash
./tempest-homekit-go --test-syslog --alarms @alarms.json
```
Tests syslog notification delivery (local or remote).

**Test OSLog Notifications** (`--test-oslog`) - macOS only
```bash
./tempest-homekit-go --test-oslog --alarms @alarms.json
```
Tests macOS unified logging system integration.

**Test Event Log Notifications** (`--test-eventlog`) - Windows only
```bash
./tempest-homekit-go --test-eventlog --alarms @alarms.json
```
Tests Windows Event Log integration.

**Webhook Listener Server** (`--webhook-listener [port]`)
```bash
# Start webhook listener on default port 8082
./tempest-homekit-go --webhook-listener

# Start webhook listener on custom port
./tempest-homekit-go --webhook-listener 9000

# Start webhook listener with debug logging
./tempest-homekit-go --webhook-listener --loglevel debug
```
Starts an HTTP server to receive and inspect webhook requests:
- **Default port**: 8082 (configurable with `--webhook-listener <port>`)
- **Endpoints**:
 - `POST /webhook`: Receives webhook payloads and pretty-prints JSON to console
 - `GET /health`: Health check endpoint returning server status
 - `GET /`: Usage instructions and endpoint documentation
- **Features**:
 - Pretty-printed JSON output for incoming webhook payloads
 - Request metadata logging (method, URL, headers, timestamp)
 - Automatic JSON detection and formatting
 - Graceful shutdown with SIGINT/SIGTERM handling
 - Real-time console output for webhook inspection and debugging
- **Use Cases**: Testing webhook integrations, debugging webhook payloads, monitoring webhook delivery

#### Service Testing

**Test HomeKit Bridge** (`--test-homekit`)
```bash
./tempest-homekit-go --test-homekit
```
Tests HomeKit bridge configuration:
- Displays sensor configuration
- Shows pairing instructions
- Validates PIN and station settings
- Dry-run mode (doesn't start actual bridge)

**Test Web Status Scraping** (`--test-web-status`)
```bash
./tempest-homekit-go --test-web-status
```
Tests web status scraping capability:
- Validates Chrome/Chromium availability
- Provides setup guidance
- Placeholder for future headless browser implementation

**Test Specific Alarm** (`--test-alarm <name>`)
```bash
./tempest-homekit-go --test-alarm "high-temperature" --alarms @alarms.json --station "Test"
```
Tests a specific alarm trigger:
- Validates alarm exists and is enabled
- Sends test observation to trigger alarm
- Tests entire notification delivery pipeline
- Shows notification results for all channels

### Testing Best Practices

1. **Test in order**: Start with `--test-api` to validate connectivity
2. **Test notifications**: Use test flags before deploying alarm configurations
3. **Use factory pattern**: All notification tests use the real delivery path
4. **Check credentials**: Test flags validate environment variables are correct
5. **Validate network**: Use `--test-udp` to diagnose UDP broadcast issues

## HomeKit Setup

1. Start the application with your WeatherFlow API token and specify your station (use `--station "Your Station Name"` or set `TEMPEST_STATION_NAME`)
2. On your iOS device, open the Home app
3. Tap the "+" icon to add an accessory
4. Select "Don't have a code or can't scan?"
5. Choose the "Tempest Bridge"
6. Enter the PIN (default: 00102003)

The following sensors will appear as separate HomeKit accessories:
- **Temperature Sensor**: Air temperature in Celsius (uses standard HomeKit temperature characteristic)
- **Humidity Sensor**: Relative humidity as percentage (uses standard HomeKit humidity characteristic) - **Light Sensor**: Ambient light level in lux (uses built-in HomeKit Light Sensor service)
- **Pressure Sensor**: Atmospheric pressure in mb (uses Light Sensor service for compliance - ignore "lux" unit label)
- **UV Index Sensor**: UV index value (uses Light Sensor service for compliance - ignore "lux" unit label)
- **Custom Wind Speed Sensor**: Wind speed in miles per hour (custom service prevents unit conversion)
- **Custom Wind Gust Sensor**: Wind gust speed in miles per hour (custom service)
- **Custom Wind Direction Sensor**: Wind direction in cardinal format with degrees (custom service)
- **Custom Rain Sensor**: Rain accumulation in inches (custom service)
- **Custom Lightning Count Sensor**: Lightning strike count (custom service)
- **Custom Lightning Distance Sensor**: Lightning strike distance (custom service)
- **Custom Precipitation Type Sensor**: Precipitation type indicator (custom service)

**Important**: The **Pressure** and **UV Index** sensors use HomeKit's standard Light Sensor service for maximum compatibility. In the Home app, they will appear as "Light Sensor" with "lux" units, but display the correct pressure (mb) and UV index values. Please ignore the "lux" unit label for these sensors - this is a HomeKit platform limitation, not an application issue.

Warning: **HomeKit Compliance Warning**: As of Home.app v10.0, all sensors labeled as "(custom service)" above will return an "Out of Compliance" error when attempting to add the accessory to the Home app. Only the standard HomeKit services (Temperature, Humidity, Light, Pressure, UV Index) will successfully pair. This is due to Apple's stricter compliance enforcement in recent Home app versions.

## Web Dashboard

Access the modern web dashboard at `http://localhost:8080` (or your configured port).

### Dashboard Features
- **External JavaScript Architecture**: Clean separation with all dashboard JavaScript (about 6,000 lines) kept in the external `script.js` file
- **Real-time Updates**: Weather data refreshes every 10 seconds with comprehensive error handling
- **Pressure Analysis System**: Advanced atmospheric pressure monitoring with:
 - **Trend Analysis**: Rising, Falling, or Stable pressure trends
 - **Weather Forecasting**: Predictions based on pressure patterns (Fair, Cloudy, Stormy)
 - **Interactive Info Icon**: Click the Info icon for detailed pressure calculation explanations
- **Interactive Unit Conversion**: Click any sensor card to toggle units:
 - Temperature: **Temperature**: Celsius (°C) ↔ Fahrenheit (°F)
 - ️ **Wind Speed**: Miles per hour (mph) ↔ Kilometers per hour (kph)
 - ️ **Rain**: Inches (in) ↔ Millimeters (mm)
 -  **Pressure**: Millibars (mb) ↔ Inches of Mercury (inHg)
- **UV Index Monitor**:  Complete UV exposure assessment with NCBI reference categories:
 - **Minimal (0-2)**: Low risk exposure with EPA green color coding
 - **Low (3-4)**: Moderate risk with yellow coding  - **Moderate (5-6)**: High risk with orange coding
 - **High (7-9)**: Very high risk with red coding
 - **Very High (10+)**: Extreme risk with violet coding
- **Enhanced Information System**: Info: Detailed sensor tooltips with proper event propagation handling
- **Accessories Status**: Real-time HomeKit sensor status showing enabled/disabled state with priority sorting
- **Wind Direction Display**: Shows cardinal direction + degrees (e.g., "WSW (241°)")
- **Unit Persistence**: Preferences saved in browser localStorage
 - **Alarm Tag Persistence**: The web dashboard persistently stores the selected alarm tag in browser localStorage under the key `alarm-selected-tag`. If a `?tag=` URL parameter is present it takes precedence over the saved value; clearing the selection removes the stored key. This is a client-side preference only and is not persisted server-side.
 - **Tempest Station Tooltip**: The Tempest Station card shows an informational tooltip about device and hub details only when those details are available. Detailed device/hub info is populated either from the local UDP stream (`--udp-stream`) or from the optional headless web scraping mode (`--use-web-status`). Without one of those enabled the dashboard will show a brief tooltip indicating the data source limitation.
- **Modern Design**: Responsive interface with weather-themed styling and cache-busting script loading
- **All Sensors**: Complete weather data display with comprehensive DOM debugging
- **HomeKit Status**: Bridge status, accessory count, and pairing PIN
- **Connection Status**: Real-time Tempest station connection status
- **Mobile Friendly**: Works perfectly on all devices with enhanced event listener management

### API Endpoints
- `GET /`: Main dashboard HTML with external JavaScript
- `GET /pkg/web/static/script.js`: External JavaScript file with cache-busting timestamps
- `GET /api/weather`: JSON weather data with pressure analysis
- `GET /api/status`: Service and HomeKit status with optional TempestWX device status
- `POST /webhook`: Receives webhook payloads and displays formatted alarm data in console (webhook listener mode)
- `GET /health`: Health check endpoint returning server status (webhook listener mode)
- `GET /`: Usage instructions and endpoint documentation (webhook listener mode)

## Architecture

```
tempest-homekit-go/
├── main.go                 # Entry point: env/flags, one-shot test modes, service start
├── go.mod / go.sum         # Module definition and checksums
├── Makefile                # vendor-static: refresh vendored Chart.js / adapter / QR code JS
├── scripts/                # build, cross-build, service install/remove, AWS SNS setup, GoDoc server
├── tests/                  # manual integration shell scripts (drive a built binary)
├── examples/               # example alarm configurations
├── docs/                   # feature and development documentation
└── pkg/
    ├── alarm/              # alarm types, evaluator, scheduling, manager, notifiers
    │   └── editor/         # standalone web alarm editor (server + static assets)
    ├── config/             # flags, environment variables, validation
    ├── generator/          # synthetic weather generator
    ├── homekit/            # HAP bridge, accessories, custom characteristics
    ├── logger/             # level-filtered logging
    ├── service/            # orchestration and data-source factory
    ├── status/             # terminal status console (tview) and themes
    ├── testhelpers/        # headless-browser test helpers
    ├── types/              # shared types
    ├── udp/                # Tempest hub UDP broadcast listener
    ├── weather/            # WeatherFlow API client, DataSource implementations, status scraping
    └── web/                # dashboard HTTP server and JSON API
        └── static/         # dashboard JS/CSS/HTML (script.js) and vendored Chart.js / QR code libraries
```

See [AGENTS.md](AGENTS.md) for a per-directory description and the verified build/test commands.

## API Integration

### WeatherFlow Tempest API
- **Stations Endpoint**: `GET /swd/rest/stations?token={token}`
- **Observations Endpoint**: `GET /swd/rest/observations/station/{station_id}?token={token}`

### Supported Weather Metrics
- **Air Temperature**: In Fahrenheit/Celsius
- **Relative Humidity**: As percentage
- **Wind Speed**: Average wind speed in mph/kph
- **Wind Direction**: Degrees with cardinal conversion
- **Rain Accumulation**: Total precipitation in inches/mm
- **Air Pressure**: Atmospheric pressure in mb/inHg
- **UV Index**: UV exposure level (0-15)
- **Ambient Light**: Illuminance in lux

## Logging

### Log Levels
- **error**: Only errors and critical messages
- **info**: Basic operational messages + sensor data summary
- **debug**: Detailed sensor data + complete API JSON responses

### Example Log Output (Info Level)
```
2025-09-21 10:30:00 Starting service with config: WebPort=8080, LogLevel=info
2025-09-21 10:30:00 Starting Tempest HomeKit service...
2025-09-21 10:30:00 Found station: Chino Hills (ID: 178915)
2025-09-21 10:30:00 INFO: HomeKit server started successfully with PIN: 00102003
2025-09-21 10:30:00 INFO: Starting web dashboard on port 8080
2025-09-21 10:30:00 Starting web server on port 8080
2025-09-21 10:30:00 INFO: Successfully read weather data from Tempest API - Station: Chino Hills
2025-09-21 10:30:00 INFO: Sensor data - Temp: 22.7°C, Humidity: 77%, Wind: 0.3 mph (238°), Rain: 0.000 in, Light: 1 lux
```

### Example Log Output (Debug Level)
```
2025-09-21 10:30:00 service.go:25: Starting Tempest HomeKit service...
2025-09-21 10:30:00 service.go:29: DEBUG: Fetching stations from WeatherFlow API
2025-09-21 10:30:00 modern_setup.go:39: DEBUG: Creating new weather system with hap library
2025-09-21 10:30:00 modern_setup.go:89: DEBUG: Created temperature sensor accessory
2025-09-21 10:30:00 modern_setup.go:169: DEBUG: Created UV Index sensor accessory using light sensor service with UV range
2025-09-21 10:30:00 service.go:284: DEBUG: HomeKit - UV Index: 0
2025-09-21 10:30:00 service.go:304: DEBUG: Updating UV Index: 0.000
```

### Example Log Output (Error Level - Default)
```
2025-09-21 10:30:00 Starting service with config: WebPort=8080, LogLevel=error
2025-09-21 10:30:00 Starting Tempest HomeKit service...
2025-09-21 10:30:00 Found station: Chino Hills (ID: 178915)
2025-09-21 10:30:00 Starting web server on port 8080
```

### Service Management

### Linux (systemd)
```bash
# Install
sudo ./scripts/install-service.sh --token "your-token" --station "Your Station Name"

# Check status
sudo systemctl status tempest-homekit-go

# View logs
sudo journalctl -u tempest-homekit-go -f

# Remove
sudo ./scripts/remove-service.sh
```

### macOS (launchd)
```bash
# Install
sudo ./scripts/install-service.sh --token "your-token" --station "Your Station Name"

# Check status
sudo launchctl list | grep tempest

# View logs
log show --predicate 'process == "tempest-homekit-go"' --last 1h

# Remove
sudo ./scripts/remove-service.sh
```

### Windows (NSSM)
```bash
# Install
./scripts/install-service.sh --token "your-token" --station "Your Station Name"

# Check status
sc query tempest-homekit-go

# View logs (via Event Viewer)
# Remove
./scripts/remove-service.sh
```

## Configuration

### Environment Variables (.env File)

The application supports configuration via environment variables, which can be stored in a `.env` file for convenience. This is particularly useful for persistent configuration without specifying command-line flags every time.

#### Quick Setup

1. **Copy the example file:**
 ```bash
 cp .env.example .env
 ```

2. **Edit `.env` with your values:**
 ```bash
 nano .env # or use your preferred editor
 ```

3. **Run without flags:**
 ```bash
 ./tempest-homekit-go # Will automatically load .env settings
 ```

#### Available Environment Variables

**Core Configuration:**

| Variable | Default | Description |
|----------|---------|-------------|
| `TEMPEST_TOKEN` | *(see below)* | WeatherFlow API token |
| `TEMPEST_STATION_NAME` | *(required)* | Your station name from WeatherFlow |
| `HOMEKIT_PIN` | `00102003` | HomeKit pairing PIN |
| `SENSORS` | `temp,lux,humidity,uv` | Enabled sensors (comma-delimited) |
| `WEB_PORT` | `8080` | Web console port |
| `UNITS` | `imperial` | Unit system (imperial/metric/sae) |
| `UNITS_PRESSURE` | `inHg` | Pressure units (inHg/mb/hpa) |
| `HISTORY_POINTS` | `1000` | Data points to store (min 10) |
| `CHART_HISTORY_HOURS` | `24` | Hours to display in charts (0=all) |
| `LOG_LEVEL` | `error` | Logging level (error/warn/warning/info/debug) |
| `LOG_FILTER` | *(empty)* | Filter log messages |
| `ENV_FILE` | `.env` | Custom environment file to load |
| `HISTORY_REDUCE` | `1` | Reduce historical points when loading (1 = no reduction) |
| `HISTORY_REDUCE_METHOD` | `timebin` | Reduction method: timebin, factor, lttb |
| `HISTORY_BIN_MINUTES` | `10` | Timebin size in minutes for timebin reduction |
| `HISTORY_KEEP_RECENT_HOURS` | `24` | Keep recent N hours at full resolution when reducing |

**Data Source Options:**

| Variable | Default | Description |
|----------|---------|-------------|
| `READ_HISTORY` | `false` | Preload historical data from API (true/false) |
| `STATION_URL` | *(empty)* | Custom station URL (overrides Tempest API) |
| `UDP_STREAM` | `false` | Enable UDP mode for offline operation (true/false) |
| `DISABLE_INTERNET` | `false` | Disable all internet access (true/false) |
| `GENERATE_WEATHER_PATH` | `/api/generate-weather` | Path for generated weather endpoint |

**Alarm & Notification (Email):**

| Variable | Default | Description |
|----------|---------|-------------|
| `ALARMS` | *(empty)* | Alarm configuration: @filename.json or inline JSON |
| `ALARMS_EDIT` | *(empty)* | Run alarm editor for specified config file |
| `ALARMS_EDIT_PORT` | `8081` | Port for alarm editor web UI |
| `TAG_LIST` | *(empty)* | Predefined tags for alarm editor dropdown (JSON array) |
| `CONTACT_LIST` | *(empty)* | Contact list for alarm notifications (JSON array) |
| `SMTP_HOST` | *(empty)* | SMTP server hostname |
| `SMTP_PORT` | `587` | SMTP server port |
| `SMTP_USERNAME` | *(empty)* | SMTP authentication username |
| `SMTP_PASSWORD` | *(empty)* | SMTP authentication password |
| `SMTP_FROM_ADDRESS` | *(empty)* | Email sender address |
| `SMTP_FROM_NAME` | *(empty)* | Email sender name |
| `SMTP_USE_TLS` | `true` | Use TLS for SMTP connection (true/false) |
| `MS365_CLIENT_ID` | *(empty)* | Microsoft 365 OAuth2 client ID |
| `MS365_CLIENT_SECRET` | *(empty)* | Microsoft 365 OAuth2 client secret |
| `MS365_TENANT_ID` | *(empty)* | Microsoft 365 tenant ID |
| `MS365_FROM_ADDRESS` | *(empty)* | Microsoft 365 sender address |

**Alarm & Notification (SMS):**

| Variable | Default | Description |
|----------|---------|-------------|
| `TWILIO_ACCOUNT_SID` | *(empty)* | Twilio account SID |
| `TWILIO_AUTH_TOKEN` | *(empty)* | Twilio authentication token |
| `TWILIO_FROM_NUMBER` | *(empty)* | Twilio sender phone number (E.164 format) |
| `AWS_ACCESS_KEY_ID` | *(empty)* | AWS access key for SNS |
| `AWS_SECRET_ACCESS_KEY` | *(empty)* | AWS secret key for SNS |
| `AWS_REGION` | *(empty)* | AWS region for SNS |
| `AWS_SNS_TOPIC_ARN` | *(empty)* | AWS SNS topic ARN |

**Alarm & Notification (Syslog):**

| Variable | Default | Description |
|----------|---------|-------------|
| `SYSLOG_ADDRESS` | *(empty)* | Syslog server address |
| `SYSLOG_NETWORK` | *(empty)* | Syslog network protocol (tcp/udp) |
| `SYSLOG_PRIORITY` | `warning` | Syslog priority level |
| `SYSLOG_TAG` | `tempest-weather` | Syslog message tag |

**Note:** Command-line flags always override environment variables.

**Overriding .env Boolean Values**: To disable a boolean flag that's set to `true` in your `.env` file, explicitly pass `--flag=false` on the command line. For example, if your `.env` contains `USE_HISTORY=true`, you can disable it with:
```bash
./tempest-homekit-go --use-history=false
```

#### Example .env Configurations

**API Mode (Cloud Data):**
```bash
# Get your token from: https://tempestwx.com/settings/tokens
TEMPEST_TOKEN=your-actual-token-here
TEMPEST_STATION_NAME=My Station Name
SENSORS=temp,humidity,pressure,wind
LOG_LEVEL=info
```

Warning: **Security Note**: Never commit your `.env` file with real credentials! See [SECURITY.md](SECURITY.md) for details.

**UDP Mode (Local Offline):**
```bash
UDP_STREAM=true
DISABLE_INTERNET=true
HISTORY_POINTS=500
CHART_HISTORY_HOURS=12
LOG_LEVEL=debug
```

**Minimal Memory:**
```bash
HISTORY_POINTS=100
CHART_HISTORY_HOURS=6
SENSORS=temp,humidity
```

### WeatherFlow API Token
1. Visit [tempestwx.com](https://tempestwx.com)
2. Go to Settings → Data Authorizations
3. Create a new personal access token
4. Use with `--token` flag or `TEMPEST_TOKEN` environment variable

### Station Discovery
The application automatically finds your station by name. Ensure your station name in WeatherFlow matches the `--station` parameter.

## Troubleshooting

### HomeKit Re-pairing (Database Reset)

When you make changes to HomeKit accessories (such as modifying sensor types, names, or configurations), you may need to reset the HomeKit database and re-pair the bridge with your Home app. This ensures the changes take effect properly.

#### Using the Built-in --cleardb Command (Recommended)

The easiest way to reset HomeKit pairing is using the built-in `--cleardb` command:

```bash
# Stop the current service if running
pkill -f tempest-homekit-go

# Clear the database and reset pairing
./tempest-homekit-go --cleardb

# Restart the service normally
./tempest-homekit-go --token "your-api-token" --station "Your Station Name"
```

#### Manual Database Reset

If you prefer to do it manually:

1. **Stop the Application**
 ```bash
 # If running as a service
 sudo systemctl stop tempest-homekit-go # Linux
 sudo launchctl stop tempest-homekit-go # macOS
 sc stop tempest-homekit-go # Windows
  # Or kill the process directly
 pkill -f tempest-homekit-go
 ```

2. **Delete the HomeKit Database**
 ```bash
 # Navigate to the application directory
 cd /path/to/tempest-homekit-go
  # Remove the database directory (this contains all pairing information)
 rm -rf ./db/
  # Verify the directory is empty
 ls -la ./db/
 ```

3. **Restart the Application**
 ```bash
 # Start the application again
 ./tempest-homekit-go --token "your-api-token" --station "Your Station Name"
  # Or restart the service
 sudo systemctl start tempest-homekit-go # Linux
 sudo launchctl start tempest-homekit-go # macOS
 sc start tempest-homekit-go # Windows
 ```

4. **Re-pair in Home App**
 - Open the Home app on your iOS device
 - The "Tempest HomeKit Bridge" should appear as a new, unpaired accessory
 - Tap the "+" icon to add an accessory
 - Select "Don't have a code or can't scan?"
 - Choose the "Tempest HomeKit Bridge"
 - Enter the PIN (default: `00102003`)

5. **Verify the Changes**
 - Check that all accessories appear correctly
 - Confirm sensor types and names are as expected
 - Test that sensors are no longer grouped incorrectly

#### Important Notes:
- **Data Loss**: This will remove all HomeKit pairing information and automation rules
- **Re-setup Required**: You'll need to re-add any scenes, automations, or accessory groupings
- **Safe Operation**: The weather data collection continues normally; only HomeKit pairing is affected
- **Backup First**: Consider noting any important automation rules before resetting

#### Alternative: Clear Specific Database Files
If you want to be more selective, you can remove specific database files instead of the entire directory:
```bash
# Remove only pairing information (keeps other HomeKit data)
rm -f ./db/pairings.json

# Remove accessory cache (forces rediscovery)
rm -f ./db/accessories.json
```

### Common Issues
- **"Station not found"**: Verify station name matches exactly (case-sensitive)
- **"API request failed"**: Check internet connection and API token validity
- **HomeKit pairing fails**: Ensure PIN is correct and no other devices are pairing
- **Web dashboard not loading**: Check if port 8080 is available
- **Sensors showing wrong values/types**: Reset HomeKit database and re-pair (see above)

### Debug Mode
Enable detailed logging for troubleshooting:
```bash
./tempest-homekit-go --loglevel debug --token "your-token" --station "Your Station Name"
```

Filter logs to show only specific messages (case-insensitive):
```bash
# Show only UDP-related messages
./tempest-homekit-go --loglevel debug --logfilter "udp" --udp-stream

# Show only forecast-related messages
./tempest-homekit-go --loglevel info --logfilter "forecast" --token "your-token" --station "Your Station Name"

# Show only observation parsing messages
./tempest-homekit-go --loglevel debug --logfilter "parsed" --udp-stream
```

### Service Issues
```bash
# Check service status
./scripts/install-service.sh --status

# Restart service
./scripts/remove-service.sh
./scripts/install-service.sh --token "your-token" --station "Your Station Name"
```

## Development

### GoDoc Server
Browse the complete Go documentation and API references locally:
```bash
# Start GoDoc server on port 6060 (opens browser automatically)
./scripts/start-godoc.sh

# Start on custom port without opening browser
./scripts/start-godoc.sh --port 8080 --no-browser

# View help
./scripts/start-godoc.sh --help
```

Then visit `http://localhost:6060` to browse:
- Package documentation for all modules (`pkg/config`, `pkg/weather`, etc.)
- Function and type definitions with examples
- Cross-referenced source code
- Standard library documentation

### Running Tests
```bash
# Run all tests (includes headless-Chrome UI tests in pkg/web and pkg/testhelpers, ~30 s)
go test ./...

# Run all tests without the browser-dependent ones (what CI's main job runs)
go test -tags=no_browser ./...

# Static checks (no repo-level golangci config; CI runs golangci-lint with --issues-exit-code=0)
go vet ./...
golangci-lint run ./...

# Run tests with coverage
go test -cover ./...

# Run verbose tests
go test -v ./...

# Run specific package tests
go test ./pkg/config/...
go test ./pkg/weather/...
go test ./pkg/web/...
go test ./pkg/service/...
```

JavaScript unit tests (dashboard helpers) run under Jest with jsdom, using `jest.web.config.json`
and the tests in `pkg/web/static/__tests__/`. `package.json` is listed in `.gitignore` and is not
tracked, so a fresh clone needs a local `package.json` with `jest`, `jest-environment-jsdom` and
`jsdom` before this works:

```bash
npx jest --runInBand --config jest.web.config.json
```

Git hooks are in `.githooks/` (`git config core.hooksPath .githooks`). The pre-commit hook runs
`gofmt -s -w` on staged Go files and `npm test` when JavaScript files are staged.

# Test Coverage Overview
- Measured 2026-10-16 with `go test -coverprofile=coverage.out ./...` (browser tests included); see the table below.

**Current Aggregate Coverage**: 44.2%

Note: The aggregate coverage uses a single `coverage.out` collected with `go test -coverprofile=coverage.out ./...` and is the authoritative project-wide percentage. The current project goal is to raise overall coverage to >= 70% by adding targeted tests (priority: `pkg/service`, then `pkg/weather`).

### Package Coverage Breakdown

| Package | Coverage (2026-10-16) |
|---------|----------|
| main | 0.0% |
| pkg/alarm | 56.7% |
| pkg/alarm/editor | 35.1% |
| pkg/config | 86.2% |
| pkg/generator | 56.3% |
| pkg/homekit | 76.3% |
| pkg/logger | 82.4% |
| pkg/service | 39.5% |
| pkg/status | 14.6% |
| pkg/testhelpers | 43.0% |
| pkg/udp | 63.5% |
| pkg/weather | 38.3% |
| pkg/web | 54.4% |
| **Overall** | **44.2%** |

### Testing Architecture
The project includes unit tests and integration-style tests that use small, isolated test doubles and local httptest servers where appropriate. Test patterns used across the repo:
- Table-driven tests for parsing and validation logic
- Fake implementations for interface dependencies (e.g., `weather.DataSource`, UDP listeners)
- `httptest` servers for HTTP client/server interactions
- Package-level injection points (for example, a `DataSourceFactory` variable in `pkg/service`) to make orchestration testable without starting long-lived goroutines

When running coverage locally, use `go test -coverprofile=coverage.out ./...` and then `go tool cover -func=coverage.out` to inspect the aggregate and per-file percentages.

### Building for Development
```bash
go build -o tempest-homekit-go
```

### Code Quality
- Comprehensive error handling and recovery
- Unit test coverage for all packages with table-driven tests
- Modular design for maintainability
- Follows Go best practices and conventions
- HTTP testing with `httptest.ResponseRecorder`
- Mock data creation for realistic test scenarios

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Make your changes with tests
4. Commit your changes (`git commit -m 'Add amazing feature'`)
5. Push to the branch (`git push origin feature/amazing-feature`)
6. Open a Pull Request

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Acknowledgments

- **WeatherFlow** for the Tempest weather station and API
- **Apple** for the HomeKit platform
- **hc library** for HomeKit Go implementation
- **Community** for feedback and contributions

## References

This project was developed using various technologies, libraries, and tools. Below is a comprehensive list of key components and resources that contributed to the development:

### Core Technologies
- **Go Programming Language** (v1.24.2+) - Primary programming language
- **HomeKit Accessory Protocol** - Apple's smart home communication protocol
- **WeatherFlow Tempest API** - Weather data source and API integration

### Go Libraries and Dependencies
- **`github.com/brutella/hap`** - HomeKit Accessory Protocol implementation for Go
- **Standard Library Packages**:
 - `net/http` - Web server implementation
 - `encoding/json` - JSON data handling
 - `sync` - Concurrent programming primitives
 - `time` - Time and date operations
 - `log` - Logging functionality
 - `os` - Operating system interface
 - `flag` - Command-line flag parsing

### Web Technologies (Embedded Dashboard)
- **HTML5** - Dashboard structure and markup
- **CSS3** - Responsive styling and animations
- **JavaScript (ES6+)** - Interactive functionality and real-time updates
- **Chart.js** (v4.4.0) - Interactive charts and data visualization
- **date-fns** (v2.30.0) - Date and time manipulation in JavaScript
- **Chart.js Date-Fns Adapter** (v2.0.1) - Time-based chart integration

### Development Tools and AI Assistance
- **GitHub Copilot** - AI-powered code suggestions and development assistance
- **Visual Studio Code** - Primary development environment
- **Go Modules** - Dependency management
- **Git** - Version control system

### Platform-Specific Tools
- **systemd** (Linux) - Service management
- **launchd** (macOS) - Service management
- **NSSM** (Windows) - Non-Sucking Service Manager for Windows services

### Build and Deployment
- **Cross-compilation** - Go's built-in cross-platform compilation
- **Shell scripting** - Bash scripts for automated builds and deployment
- **Platform detection** - Runtime OS and architecture detection

### External Resources and Documentation
- **WeatherFlow API Documentation** - Weather data integration reference
- **Apple HomeKit Developer Documentation** - HomeKit protocol implementation guide
- **Go Documentation** - Standard library and language reference
- **MDN Web Docs** - JavaScript, HTML, and CSS reference

### Development Practices
- **Test-Driven Development** - Unit testing approach
- **Modular Architecture** - Clean code organization
- **Error Handling** - Comprehensive error management
- **Logging** - Multi-level logging system
- **Configuration Management** - Flexible configuration via flags and environment variables

## Additional Documentation

### Alarm System Documentation
- **[ALARM_SCHEDULING.md](docs/ALARM_SCHEDULING.md)** - Complete scheduling system: time ranges, weekly schedules, sunrise/sunset
- **[ALARM_LOGGING.md](pkg/alarm/docs/ALARM_LOGGING.md)** - Alarm logging behavior (always visible regardless of log level)
- **[ALARM_COOLDOWN_STATUS.md](pkg/alarm/docs/ALARM_COOLDOWN_STATUS.md)** - Real-time cooldown status display in web console
- **[OSLOG_NOTIFIER.md](docs/development/OSLOG_NOTIFIER.md)** - macOS unified logging integration for alarms
- **[docs/webhook-delivery.md](docs/webhook-delivery.md)** - Complete webhook delivery method documentation with Go server example
- **[CHANGE_DETECTION_OPERATORS.md](docs/development/CHANGE_DETECTION_OPERATORS.md)** - Complete technical reference for change detection operators (*field, >field, <field)
- **[CHANGE_DETECTION_QUICKREF.md](docs/development/CHANGE_DETECTION_QUICKREF.md)** - Quick reference guide with examples
- **[CHANGE_DETECTION_VISUAL.md](docs/development/CHANGE_DETECTION_VISUAL.md)** - Visual diagrams and state transition timelines
- **[CHANGE_DETECTION_SUMMARY.md](docs/development/CHANGE_DETECTION_SUMMARY.md)** - Implementation summary and architecture
- **[ALARM_EDITOR_MESSAGES.md](pkg/alarm/docs/ALARM_EDITOR_MESSAGES.md)** - Message configuration with variable templates
- **[ALARM_EDITOR_CHANNEL_FIX.md](pkg/alarm/docs/ALARM_EDITOR_CHANNEL_FIX.md)** - Documentation of alarm channel save fix
- **[WEB_ALARM_STATUS_CARD.md](docs/development/WEB_ALARM_STATUS_CARD.md)** - Web console alarm status card implementation
- **[examples/alarms-with-change-detection.json](examples/alarms-with-change-detection.json)** - Ready-to-use alarm configurations

### Package Documentation
Each package includes detailed README files:
- **[pkg/alarm/README.md](pkg/alarm/README.md)** - Alarm package documentation
- **[pkg/alarm/editor/README.md](pkg/alarm/editor/README.md)** - Alarm editor documentation
- **[pkg/config/README.md](pkg/config/README.md)** - Configuration package documentation
- **[pkg/weather/README.md](pkg/weather/README.md)** - Weather data source documentation
- **[pkg/web/README.md](pkg/web/README.md)** - Web dashboard documentation
- **[pkg/service/README.md](pkg/service/README.md)** - Service orchestration documentation

---

**Status**: **COMPLETE** - All planned features implemented and tested
- Weather monitoring with 11 HomeKit sensors (Temperature + 10 custom weather sensors)
- Complete HomeKit integration with compliance optimization
- Modern web dashboard with real-time updates and interactive features
- UV Index monitoring with NCBI reference data and EPA color coding
- Information tooltips system with standardized positioning
- HomeKit accessories status monitoring with enabled/disabled indicators
- Interactive unit conversions with localStorage persistence
- Cross-platform build and deployment with automated service management
- Professional styling and enhanced user experience
- Comprehensive logging and error handling
- Database management with --cleardb command
- Production-ready with graceful error recovery
- Weather monitoring with 6 metrics (Temperature, Humidity, Wind Speed, Wind Direction, Rain, Light)
- Complete HomeKit integration with individual sensors
- Modern web dashboard with real-time updates
- Interactive unit conversions with persistence
- Cross-platform build and deployment
- Service management for all platforms
- Comprehensive logging and error handling
- Database management with --cleardb command
- Production-ready with graceful error handling