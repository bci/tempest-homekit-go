# Docker Deployment Guide

This guide covers deploying Tempest HomeKit Go using Docker containers for both local development/testing and production deployment on QNAP NAS systems.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Quick Start](#quick-start)
- [Local Docker Desktop Deployment](#local-docker-desktop-deployment)
- [QNAP NAS Container Station Deployment](#qnap-nas-container-station-deployment)
- [Building and Pushing Images](#building-and-pushing-images)
- [Configuration](#configuration)
- [Environment Variables](#environment-variables)
- [Volumes and Persistence](#volumes-and-persistence)
- [Networking](#networking)
- [Troubleshooting](#troubleshooting)
- [Advanced Configuration](#advanced-configuration)

## Prerequisites

- **Docker Engine** (Docker Desktop for local, or Docker-compatible runtime for QNAP)
- **WeatherFlow API Token** (for production use with real weather data)
- **Station Name** (your WeatherFlow station name)
- **Basic Docker knowledge**

## Quick Start

### Test with Generated Weather (No API Token Required)

```bash
# Build the image
docker build -t tempest-homekit-go:latest .

# Run with generated weather
docker run -d -p 8080:8080 --name tempest-weather \
  --entrypoint ./tempest-homekit-go \
  tempest-homekit-go:latest \
  --use-generated-weather \
  --station "Test Station"

# Access at http://localhost:8080
```

### Production with Real Weather Data

```bash
# Run with WeatherFlow API
docker run -d -p 8080:8080 --name tempest-weather \
  -e TEMPEST_TOKEN="your-api-token" \
  -e TEMPEST_STATION_NAME="Your Station Name" \
  --entrypoint ./tempest-homekit-go \
  tempest-homekit-go:latest
```

## Local Docker Desktop Deployment

### Method 1: Docker Run (Simple)

```bash
# Create a directory for persistent data
mkdir -p ~/tempest-data

# Run the container
docker run -d \
  --name tempest-homekit-go \
  -p 8080:8080 \
  -p 51826:51826 \
  -e TEMPEST_TOKEN="your-api-token" \
  -e TEMPEST_STATION_NAME="Your Station Name" \
  -e LOG_LEVEL="info" \
  -v ~/tempest-data:/home/appuser/db \
  --restart unless-stopped \
  --entrypoint ./tempest-homekit-go \
  tempest-homekit-go:latest
```

### Method 2: Docker Compose (Recommended)

Create a `docker-compose.yml` file:

```yaml
version: '3.8'

services:
  tempest-homekit-go:
    image: tempest-homekit-go:latest
    build: .  # Remove this line if using pre-built image
    container_name: tempest-homekit-go
    ports:
      - "8080:8080"    # Web dashboard
      - "51826:51826"  # HomeKit bridge
    environment:
      # Required for WeatherFlow API
      - TEMPEST_TOKEN=your-api-token-here
      - TEMPEST_STATION_NAME=Your Station Name

      # Optional configuration
      - WEB_PORT=8080
      - LOG_LEVEL=info
      - SENSORS=temp,lux,humidity,uv
      - UNITS=imperial

      # Alarm configuration (if using)
      - ALARMS=@tempest-alarms.json
    volumes:
      # HomeKit database persistence
      - ./db:/home/appuser/db

      # Alarm configuration (optional)
      - ./tempest-alarms.json:/home/appuser/tempest-alarms.json:ro
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "--no-verbose", "--tries=1", "--spider", "http://localhost:8080/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 40s
```

Then run:

```bash
# Start the service
docker-compose up -d

# View logs
docker-compose logs -f

# Stop the service
docker-compose down
```

### Docker Desktop Specific Notes

- **Port conflicts**: If port 8080 is in use, change to another port (e.g., 8081:8080)
- **File sharing**: Enable file sharing in Docker Desktop settings for volume mounts
- **Resource limits**: Adjust CPU/memory limits in Docker Desktop if needed
- **Logs**: Use `docker logs tempest-homekit-go` to view application logs

## QNAP NAS Container Station Deployment

### Step 1: Prepare Your QNAP

1. **Install Container Station** (if not already installed):
   - Go to App Center → Container Station → Install

2. **Enable SSH** (optional, for advanced management):
   - Control Panel → Network & File Services → Telnet/SSH → Enable SSH

3. **Create shared folders** for persistent data:
   - Storage & Snapshots → Storage → Create new shared folder
   - Name: `tempest-data`
   - Purpose: Store HomeKit database and configuration

### Step 2: Pull or Build the Image

#### Option A: Pull from Docker Hub (Recommended)

```bash
# SSH into your QNAP
ssh admin@your-qnap-ip

# Pull the image
docker pull your-dockerhub-username/tempest-homekit-go:latest
```

#### Option B: Build on QNAP

```bash
# Transfer the source code to your QNAP
scp -r /path/to/tempest-homekit-go admin@your-qnap-ip:/share/tempest-data/

# SSH and build
ssh admin@your-qnap-ip
cd /share/tempest-data/tempest-homekit-go
docker build -t tempest-homekit-go:latest .
```

### Step 3: Create Container in Container Station

1. **Open Container Station** in QNAP web interface
2. **Click "Create" → "Create Container"**
3. **Container Settings**:
   - **Name**: `tempest-homekit-go`
   - **Image**: `your-dockerhub-username/tempest-homekit-go:latest` or `tempest-homekit-go:latest`

4. **Network Settings**:
   - **Network Mode**: Bridge (recommended) or Host
   - **Port Forwarding**:
     - Host Port: `8080` → Container Port: `8080` (Web Dashboard)
     - Host Port: `51826` → Container Port: `51826` (HomeKit)

5. **Environment Variables**:
   ```
   TEMPEST_TOKEN=your-api-token
   TEMPEST_STATION_NAME=Your Station Name
   WEB_PORT=8080
   LOG_LEVEL=info
   ```

6. **Volume Mounts**:
   - **Host Path**: `/share/tempest-data/db`
   - **Container Path**: `/home/appuser/db`
   - **Mount Type**: Read/Write

   For alarms (optional):
   - **Host Path**: `/share/tempest-data/tempest-alarms.json`
   - **Container Path**: `/home/appuser/tempest-alarms.json`
   - **Mount Type**: Read-only

7. **Advanced Settings**:
   - **Restart Policy**: Always
   - **CPU Limit**: 0.5 (50% of one core)
   - **Memory Limit**: 256MB
   - **Command**: `./tempest-homekit-go` (leave default)

8. **Create and Start** the container

### Step 4: Access and Configure

1. **Web Dashboard**: `http://your-qnap-ip:8080`
2. **HomeKit Setup**: Use the Home app on iOS to add the bridge
3. **Logs**: View in Container Station → Container → Logs

### QNAP-Specific Considerations

- **Storage**: Use QNAP shared folders for persistence
- **Backup**: Include `/share/tempest-data` in QNAP backup jobs
- **Updates**: Stop container, update image, restart
- **Resource Usage**: Monitor in Storage & Snapshots → Overview
- **Network**: Ensure ports 8080 and 51826 are open in firewall

## Building and Pushing Images

### Build the Image

```bash
# Clone the repository
git clone https://github.com/BEHRConsulting/tempest-homekit-go.git
cd tempest-homekit-go

# Build for local use
docker build -t tempest-homekit-go:latest .

# Build for specific platform (if needed)
docker build --platform linux/amd64 -t tempest-homekit-go:latest .
```

### Push to Docker Hub

```bash
# Login to Docker Hub
docker login

# Tag the image
docker tag tempest-homekit-go:latest your-username/tempest-homekit-go:latest

# Push to Docker Hub
docker push your-username/tempest-homekit-go:latest

# Tag with version (optional)
docker tag tempest-homekit-go:latest your-username/tempest-homekit-go:v1.11.0
docker push your-username/tempest-homekit-go:v1.11.0
```

### Automated Builds (GitHub Actions)

The repository includes GitHub Actions for automated building. To enable:

1. Fork the repository
2. Enable GitHub Actions
3. Configure Docker Hub credentials in repository secrets:
   - `DOCKERHUB_USERNAME`
   - `DOCKERHUB_TOKEN`

## Configuration

### Command Line Flags vs Environment Variables

All command-line flags can be set via environment variables. The mapping is:

| Flag | Environment Variable | Default |
|------|---------------------|---------|
| `--token` | `TEMPEST_TOKEN` | Required for API |
| `--station` | `TEMPEST_STATION_NAME` | Required for API |
| `--web-port` | `WEB_PORT` | `8080` |
| `--loglevel` | `LOG_LEVEL` | `error` |
| `--sensors` | `SENSORS` | `temp,lux,humidity,uv` |
| `--units` | `UNITS` | `imperial` |
| `--alarms` | `ALARMS` | None |

### Configuration Files

#### Environment File (.env)

Create a `.env` file for sensitive configuration:

```bash
# WeatherFlow API
TEMPEST_TOKEN=your-api-token
TEMPEST_STATION_NAME=Your Station Name

# Application Settings
WEB_PORT=8080
LOG_LEVEL=info
SENSORS=temp,lux,humidity,uv,pressure
UNITS=imperial

# Email Notifications (optional)
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USERNAME=your-email@gmail.com
SMTP_PASSWORD=your-app-password
SMTP_FROM_ADDRESS=your-email@gmail.com

# SMS Notifications (optional)
AWS_ACCESS_KEY_ID=your-aws-key
AWS_SECRET_ACCESS_KEY=your-aws-secret
AWS_REGION=us-east-1
AWS_SNS_TOPIC_ARN=arn:aws:sns:us-east-1:123456789012:tempest-alarms
```

Mount the .env file in Docker:

```yaml
volumes:
  - ./.env:/home/appuser/.env:ro
```

#### Alarm Configuration

Create `tempest-alarms.json`:

```json
{
  "alarms": [
    {
      "name": "High Temperature",
      "description": "Alert when temperature exceeds 85°F",
      "condition": "temperature > 85",
      "channels": ["console", "email"],
      "enabled": true,
      "cooldown": 1800
    }
  ]
}
```

## Environment Variables

### Core Configuration

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `TEMPEST_TOKEN` | WeatherFlow API access token | - | Yes (for API mode) |
| `TEMPEST_STATION_NAME` | Your station name from WeatherFlow | - | Yes (for API mode) |
| `WEB_PORT` | Web dashboard port | `8080` | No |
| `LOG_LEVEL` | Logging level (error/warn/info/debug) | `error` | No |
| `SENSORS` | Enabled sensors (comma-separated) | `temp,lux,humidity,uv` | No |
| `UNITS` | Unit system (imperial/metric/sae) | `imperial` | No |

### Data Source Options

| Variable | Description | Default |
|----------|-------------|---------|
| `STATION_URL` | Custom station URL (overrides API) | - |
| `UDP_STREAM` | Enable UDP broadcast listening | `false` |
| `DISABLE_INTERNET` | Offline mode (requires UDP or generated) | `false` |
| `USE_GENERATED_WEATHER` | Use simulated weather data | `false` |

### Notification Configuration

| Variable | Description | Default |
|----------|-------------|---------|
| `ALARMS` | Alarm configuration file path | - |
| `SMTP_HOST` | SMTP server hostname | - |
| `SMTP_PORT` | SMTP server port | `587` |
| `SMTP_USERNAME` | SMTP username | - |
| `SMTP_PASSWORD` | SMTP password | - |
| `SMTP_FROM_ADDRESS` | Email sender address | - |
| `AWS_ACCESS_KEY_ID` | AWS access key for SNS | - |
| `AWS_SECRET_ACCESS_KEY` | AWS secret key for SNS | - |
| `AWS_REGION` | AWS region | - |
| `AWS_SNS_TOPIC_ARN` | SNS topic ARN | - |

## Volumes and Persistence

### Required Volumes

#### HomeKit Database
- **Purpose**: Stores HomeKit pairing information and accessory configurations
- **Container Path**: `/home/appuser/db`
- **Host Path**: `./db` (local) or `/share/tempest-data/db` (QNAP)
- **Permissions**: Read/Write

#### Alarm Configuration (Optional)
- **Purpose**: Alarm rules and notification settings
- **Container Path**: `/home/appuser/tempest-alarms.json`
- **Host Path**: `./tempest-alarms.json`
- **Permissions**: Read-only

### Volume Mount Examples

Docker Compose:
```yaml
volumes:
  - ./db:/home/appuser/db
  - ./tempest-alarms.json:/home/appuser/tempest-alarms.json:ro
```

Docker Run:
```bash
-v /host/path/db:/home/appuser/db \
-v /host/path/tempest-alarms.json:/home/appuser/tempest-alarms.json:ro
```

QNAP Container Station:
- **Source**: `/share/tempest-data/db`
- **Destination**: `/home/appuser/db`
- **Type**: Read/Write

## Networking

### Port Configuration

| Port | Protocol | Purpose | Required |
|------|----------|---------|----------|
| `8080` | TCP | Web dashboard and API | Yes |
| `51826` | TCP | HomeKit bridge | Yes (for HomeKit) |

### Network Modes

#### Bridge Mode (Recommended)
- Container gets its own IP on Docker network
- Ports are forwarded from host
- Best for single-host deployments

#### Host Mode
- Container uses host network directly
- No port forwarding needed
- Better performance, but less isolation

### Firewall Considerations

Ensure these ports are open:
- **8080**: For web dashboard access
- **51826**: For HomeKit device discovery and pairing
- **80/443**: If reverse proxy is used

## Troubleshooting

### Common Issues

#### Container Won't Start
```bash
# Check container logs
docker logs tempest-homekit-go

# Check container status
docker ps -a
```

#### Web Dashboard Not Accessible
```bash
# Check if container is running
docker ps | grep tempest

# Check port mapping
docker port tempest-homekit-go

# Test health endpoint
curl http://localhost:8080/health
```

#### HomeKit Not Working
- Ensure port 51826 is forwarded
- Check firewall settings
- Verify HomeKit database persistence
- Reset HomeKit pairing if needed

#### Permission Issues
```bash
# Check volume permissions
ls -la /host/path/db

# Fix permissions
sudo chown -R 1000:1000 /host/path/db
```

### QNAP-Specific Issues

#### Container Station Not Starting
- Check available disk space
- Verify Docker image exists
- Check Container Station logs

#### Network Issues
- Ensure bridge network is available
- Check QNAP firewall settings
- Verify port forwarding in router

#### Storage Issues
- Check shared folder permissions
- Ensure sufficient space in storage pool
- Verify volume mount paths

### Logs and Debugging

#### View Application Logs
```bash
# Docker Compose
docker-compose logs -f tempest-homekit-go

# Docker Run
docker logs -f tempest-homekit-go

# QNAP Container Station
# Container Station → Container → Logs
```

#### Debug Mode
```bash
# Enable debug logging
-e LOG_LEVEL=debug

# View all environment variables
docker exec tempest-homekit-go env
```

#### Health Checks
```bash
# Manual health check
curl http://localhost:8080/health

# Check container health
docker ps --format "table {{.Names}}\t{{.Status}}"
```

## Advanced Configuration

### Reverse Proxy Setup

#### Nginx Example
```nginx
server {
    listen 80;
    server_name weather.yourdomain.com;

    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

#### Apache Example
```apache
<VirtualHost *:80>
    ServerName weather.yourdomain.com

    ProxyPass / http://localhost:8080/
    ProxyPassReverse / http://localhost:8080/

    <Location />
        Require all granted
    </Location>
</VirtualHost>
```

### SSL/TLS Configuration

```yaml
# docker-compose.yml with SSL
version: '3.8'
services:
  tempest-homekit-go:
    image: tempest-homekit-go:latest
    ports:
      - "443:8080"
    environment:
      - WEB_PORT=8080
      # ... other env vars
    volumes:
      - ./ssl:/home/appuser/ssl:ro
      - ./db:/home/appuser/db
```

### Monitoring and Alerts

#### Health Checks
The container includes built-in health checks. Monitor with:

```bash
# Docker health status
docker ps --format "table {{.Names}}\t{{.Status}}"

# Prometheus metrics (if configured)
curl http://localhost:8080/metrics
```

#### Log Aggregation
```yaml
# Send logs to external service
version: '3.8'
services:
  tempest-homekit-go:
    logging:
      driver: syslog
      options:
        syslog-address: "tcp://logstash:514"
```

### Backup and Recovery

#### Database Backup
```bash
# Stop container
docker stop tempest-homekit-go

# Backup database
cp -r ./db ./db-backup-$(date +%Y%m%d)

# Start container
docker start tempest-homekit-go
```

#### QNAP Backup
- Use QNAP's built-in backup tools
- Include `/share/tempest-data` in backup jobs
- Schedule regular backups of HomeKit database

### Performance Tuning

#### Resource Limits
```yaml
services:
  tempest-homekit-go:
    deploy:
      resources:
        limits:
          cpus: '0.50'
          memory: 256M
        reservations:
          cpus: '0.25'
          memory: 128M
```

#### JVM-like Tuning (if applicable)
- Monitor memory usage with `docker stats`
- Adjust logging level to reduce I/O
- Use appropriate sensor configurations

---

## Support

For issues specific to Docker deployment:
1. Check this documentation
2. Review container logs
3. Verify configuration and environment variables
4. Test with generated weather first
5. Check GitHub issues for similar problems

For application-specific issues, see the main README.md and project documentation.