# Docker Deployment Guide

This guide covers deploying Tempest HomeKit Go using Docker containers for both local development/testing and production deployment on QNAP NAS systems.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Quick Start](#quick-start)
- [Local Docker Desktop Deployment](#local-docker-desktop-deployment)
- [QNAP NAS Container Station Deployment](#qnap-nas-container-station-deployment)
- [AWS Container Service Deployment](#aws-container-service-deployment)
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

**Note:** The official Docker image (`behrconsulting/tempest-homekit-go`) supports multiple architectures (AMD64 and ARM64), making it compatible with both Intel/AMD and ARM-based systems including QNAP NAS devices.

## Quick Start

### Test with Generated Weather (No API Token Required)

```bash
# Build the image
docker build -t tempest-homekit-go:latest .

# Run with generated weather
docker run -d -p 8080:8080 -p 8081:8081 --name tempest-weather \
  --entrypoint ./tempest-homekit-go \
  tempest-homekit-go:latest \
  --use-generated-weather \
  --station "Test Station"

# Access at http://localhost:8080
```

### Production with Real Weather Data

```bash
# Run with WeatherFlow API
docker run -d -p 8080:8080 -p 8081:8081 --name tempest-weather \
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
  -p 8081:8081 \
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

# Pull the image (multi-architecture: supports both AMD64 and ARM64)
docker pull behrconsulting/tempest-homekit-go:latest
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
   - **Image**: `behrconsulting/tempest-homekit-go:latest`

4. **Network Settings**:
   - **Network Mode**: Bridge (recommended) or Host
   - **Port Forwarding**:
     - Host Port: `8080` → Container Port: `8080` (Web Dashboard)
     - Host Port: `51826` → Container Port: `51826` (HomeKit)
     - Host Port: `8081` → Container Port: `8081` (Alarm Editor)

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
- **Network**: Ensure ports 8080, 8081, and 51826 are open in firewall

## AWS Container Service Deployment

### Overview

AWS offers several container services for deploying Tempest HomeKit Go:
- **Amazon ECS (Elastic Container Service)**: Managed container orchestration
- **Amazon EKS (Elastic Kubernetes Service)**: Kubernetes-based orchestration
- **AWS Fargate**: Serverless container execution
- **AWS App Runner**: Fully managed container application service

This guide focuses on Amazon ECS with Fargate for serverless deployment.

### Prerequisites

- **AWS Account** with appropriate permissions
- **AWS CLI** configured (`aws configure`)
- **Docker image** pushed to Amazon ECR or Docker Hub
- **WeatherFlow API credentials**
- **AWS VPC** with public subnets (for internet access)

### Step 1: Prepare AWS Resources

#### Create ECR Repository (if using ECR)

```bash
# Create ECR repository
aws ecr create-repository \
  --repository-name tempest-homekit-go \
  --region us-east-1

# Get repository URI
REPO_URI=$(aws ecr describe-repositories \
  --repository-names tempest-homekit-go \
  --region us-east-1 \
  --query 'repositories[0].repositoryUri' \
  --output text)

# Login to ECR
aws ecr get-login-password --region us-east-1 | \
  docker login --username AWS --password-stdin $REPO_URI
```

#### Push Image to ECR

```bash
# Tag and push your image
docker tag tempest-homekit-go:latest $REPO_URI:latest
docker push $REPO_URI:latest
```

### Step 2: Create ECS Cluster

```bash
# Create ECS cluster
aws ecs create-cluster \
  --cluster-name tempest-cluster \
  --region us-east-1
```

### Step 3: Create Task Definition

Create a file `task-definition.json`:

```json
{
  "family": "tempest-homekit-go",
  "taskRoleArn": "arn:aws:iam::123456789012:role/ecsTaskExecutionRole",
  "executionRoleArn": "arn:aws:iam::123456789012:role/ecsTaskExecutionRole",
  "networkMode": "awsvpc",
  "requiresCompatibilities": ["FARGATE"],
  "cpu": "256",
  "memory": "512",
  "containerDefinitions": [
    {
      "name": "tempest-homekit-go",
      "image": "your-ecr-repo/tempest-homekit-go:latest",
      "essential": true,
      "portMappings": [
        {
          "containerPort": 8080,
          "hostPort": 8080,
          "protocol": "tcp"
        },
        {
          "containerPort": 51826,
          "hostPort": 51826,
          "protocol": "tcp"
        },
        {
          "containerPort": 8081,
          "hostPort": 8081,
          "protocol": "tcp"
        }
      ],
      "environment": [
        {
          "name": "TEMPEST_TOKEN",
          "value": "your-api-token"
        },
        {
          "name": "TEMPEST_STATION_NAME",
          "value": "Your Station Name"
        },
        {
          "name": "WEB_PORT",
          "value": "8080"
        },
        {
          "name": "LOG_LEVEL",
          "value": "info"
        }
      ],
      "logConfiguration": {
        "logDriver": "awslogs",
        "options": {
          "awslogs-group": "/ecs/tempest-homekit-go",
          "awslogs-region": "us-east-1",
          "awslogs-stream-prefix": "ecs"
        }
      },
      "healthCheck": {
        "command": [
          "CMD-SHELL",
          "wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1"
        ],
        "interval": 30,
        "timeout": 5,
        "retries": 3,
        "startPeriod": 60
      }
    }
  ]
}
```

Register the task definition:

```bash
aws ecs register-task-definition \
  --cli-input-json file://task-definition.json \
  --region us-east-1
```

### Step 4: Create Application Load Balancer (Optional)

For production deployments with load balancing:

```bash
# Create ALB
aws elbv2 create-load-balancer \
  --name tempest-alb \
  --subnets subnet-12345 subnet-67890 \
  --security-groups sg-12345 \
  --region us-east-1

# Create target group
aws elbv2 create-target-group \
  --name tempest-targets \
  --protocol HTTP \
  --port 8080 \
  --vpc-id vpc-12345 \
  --target-type ip \
  --region us-east-1

# Create listener
aws elbv2 create-listener \
  --load-balancer-arn $ALB_ARN \
  --protocol HTTP \
  --port 80 \
  --default-actions Type=forward,TargetGroupArn=$TARGET_GROUP_ARN \
  --region us-east-1
```

### Step 5: Create ECS Service

```bash
# Create service
aws ecs create-service \
  --cluster tempest-cluster \
  --service-name tempest-service \
  --task-definition tempest-homekit-go \
  --desired-count 1 \
  --launch-type FARGATE \
  --network-configuration "awsvpcConfiguration={subnets=[subnet-12345,subnet-67890],securityGroups=[sg-12345],assignPublicIp=ENABLED}" \
  --load-balancers "targetGroupArn=$TARGET_GROUP_ARN,containerName=tempest-homekit-go,containerPort=8080" \
  --region us-east-1
```

### Step 6: Configure Security Groups

Ensure your security group allows inbound traffic:

```bash
# Allow HTTP (web dashboard)
aws ec2 authorize-security-group-ingress \
  --group-id sg-12345 \
  --protocol tcp \
  --port 8080 \
  --cidr 0.0.0.0/0 \
  --region us-east-1

# Allow HomeKit
aws ec2 authorize-security-group-ingress \
  --group-id sg-12345 \
  --protocol tcp \
  --port 51826 \
  --cidr 0.0.0.0/0 \
  --region us-east-1
```

### Step 7: Access Your Application

1. **Get Load Balancer DNS**: 
   ```bash
   aws elbv2 describe-load-balancers --names tempest-alb --region us-east-1
   ```

2. **Web Dashboard**: `http://your-alb-dns-name`
3. **HomeKit Setup**: Use the Home app on iOS to add the bridge

### AWS-Specific Considerations

#### Cost Optimization
- **Fargate Spot**: Use Fargate Spot for development/testing (up to 70% savings)
- **Right-sizing**: Start with 0.25 vCPU / 512MB RAM, scale up if needed
- **Auto-scaling**: Configure based on CPU/memory usage

#### Persistence
- **HomeKit Database**: Use EFS for persistent storage
- **Environment Variables**: Store secrets in AWS Systems Manager Parameter Store or Secrets Manager
- **Backups**: Enable automated backups for EFS volumes

#### Networking
- **VPC Endpoints**: Use VPC endpoints for ECR to avoid NAT Gateway costs
- **Security Groups**: Restrict access to specific IP ranges for production
- **CloudFront**: Add CloudFront distribution for global access and caching

#### Monitoring
- **CloudWatch Logs**: Application logs are automatically sent to CloudWatch
- **CloudWatch Metrics**: Monitor ECS service metrics
- **X-Ray**: Enable AWS X-Ray for distributed tracing

#### Updates
```bash
# Update service with new image
aws ecs update-service \
  --cluster tempest-cluster \
  --service tempest-service \
  --task-definition tempest-homekit-go:new-version \
  --region us-east-1
```

### Alternative: AWS App Runner (Simplified)

For a fully managed experience:

```bash
# Create App Runner service
aws apprunner create-service \
  --service-name tempest-homekit-go \
  --source-configuration "ImageRepository={ImageIdentifier=your-ecr-repo/tempest-homekit-go:latest,ImageConfiguration={Port=8080}}" \
  --instance-configuration "Cpu=256,Memory=512" \
  --region us-east-1
```

App Runner automatically handles:
- Load balancing
- Auto-scaling
- Health checks
- SSL certificates

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
| `8081` | TCP | Alarm editor web UI | Optional (for alarm configuration) |
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

### AWS-Specific Issues

#### ECS Service Not Starting
```bash
# Check service status
aws ecs describe-services \
  --cluster tempest-cluster \
  --services tempest-service \
  --region us-east-1

# Check task status
aws ecs list-tasks --cluster tempest-cluster --region us-east-1
aws ecs describe-tasks --cluster tempest-cluster --tasks $TASK_ARN --region us-east-1
```

#### Task Definition Issues
- Verify ECR repository permissions
- Check execution role has necessary permissions
- Ensure subnet security groups allow outbound internet access
- Validate environment variables are properly set

#### Load Balancer Issues
```bash
# Check target health
aws elbv2 describe-target-health \
  --target-group-arn $TARGET_GROUP_ARN \
  --region us-east-1

# Check ALB access logs
aws elbv2 describe-load-balancers --names tempest-alb --region us-east-1
```

#### Networking Issues
- Verify VPC has internet gateway for public subnets
- Check security group rules allow inbound traffic on ports 8080, 8081, and 51826
- Ensure NAT Gateway exists for private subnet outbound access
- Validate route table configurations

#### CloudWatch Logs
```bash
# View application logs
aws logs tail /ecs/tempest-homekit-go \
  --follow \
  --region us-east-1

# Check for specific errors
aws logs filter-log-events \
  --log-group-name /ecs/tempest-homekit-go \
  --filter-pattern ERROR \
  --region us-east-1
```

#### Fargate Resource Issues
- **CPU/Memory**: Increase task size if containers are being killed
- **ENI Limits**: Check ENI availability in subnets
- **Service Limits**: Verify account limits for ECS services

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

#### AWS Monitoring
For ECS deployments:

```bash
# View CloudWatch metrics
aws cloudwatch get-metric-statistics \
  --namespace AWS/ECS \
  --metric-name CPUUtilization \
  --dimensions Name=ClusterName,Value=tempest-cluster Name=ServiceName,Value=tempest-service \
  --start-time $(date -u -d '1 hour ago' +%Y-%m-%dT%H:%M:%S) \
  --end-time $(date -u +%Y-%m-%dT%H:%M:%S) \
  --period 300 \
  --statistics Average \
  --region us-east-1

# Set up CloudWatch alarms
aws cloudwatch put-metric-alarm \
  --alarm-name "TempestServiceUnhealthy" \
  --alarm-description "Alert when ECS service becomes unhealthy" \
  --metric-name UnhealthyTaskCount \
  --namespace AWS/ECS \
  --statistic Average \
  --period 300 \
  --threshold 1 \
  --comparison-operator GreaterThanThreshold \
  --dimensions Name=ClusterName,Value=tempest-cluster Name=ServiceName,Value=tempest-service \
  --region us-east-1
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

#### AWS Backup
For ECS deployments with persistent storage:

```bash
# Backup EFS volume (if using EFS)
aws backup create-backup-vault --backup-vault-name tempest-backups --region us-east-1

# Create backup plan
aws backup create-backup-plan --backup-plan file://backup-plan.json --region us-east-1

# Where backup-plan.json contains:
{
  "BackupPlan": {
    "BackupPlanName": "tempest-homekit-backup",
    "Rules": [
      {
        "RuleName": "DailyBackups",
        "TargetBackupVaultName": "tempest-backups",
        "ScheduleExpression": "cron(0 5 ? * * *)",
        "Lifecycle": {
          "DeleteAfterDays": 30
        }
      }
    ]
  }
}
```

For RDS (if using database):
```bash
# Create DB snapshot
aws rds create-db-snapshot \
  --db-instance-identifier your-db-instance \
  --db-snapshot-identifier tempest-snapshot-$(date +%Y%m%d) \
  --region us-east-1
```

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

For AWS ECS deployments:
1. Check ECS service and task status in AWS Console
2. Review CloudWatch logs for application errors
3. Verify security group and network configurations
4. Check ECR repository permissions and image availability
5. Validate task definition and environment variables

For application-specific issues, see the main README.md and project documentation.