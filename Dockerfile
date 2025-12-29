# Multi-stage Dockerfile for tempest-homekit-go
# Build stage
FROM golang:1.24-alpine AS builder

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o tempest-homekit-go .

# Runtime stage
FROM alpine:latest

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates

# Create a non-root user
RUN adduser -D -s /bin/sh appuser

# Set working directory
WORKDIR /home/appuser/

# Copy the binary from builder stage
COPY --from=builder /app/tempest-homekit-go .

# Copy static web files
COPY --from=builder /app/pkg/web/static ./pkg/web/static/

# Copy any necessary config files (optional)
# COPY --from=builder /app/.env.example .env

# Change ownership to non-root user
RUN chown -R appuser:appuser ./pkg/

# Switch to non-root user
USER appuser

# Expose ports (web dashboard and HomeKit)
EXPOSE 8080 51826

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run the application
CMD ["./tempest-homekit-go"]