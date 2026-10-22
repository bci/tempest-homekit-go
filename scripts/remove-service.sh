#!/bin/bash

# Tempest HomeKit Go Cross-Platform Service Removal Script
# Supports Linux (systemd), macOS (launchd), and Windows (Windows Service)
#
# Reverses scripts/install-service.sh: stops the service, removes its
# definition, deletes /opt/tempest-homekit-go and /var/log/tempest-homekit-go
# (Windows: the Program Files and ProgramData folders) and, on Linux, the
# "tempest" user. Deletions are recursive and not recoverable. Interactive:
# asks before deleting the configuration directory on Linux and macOS.
#
# Usage:
#   sudo ./scripts/remove-service.sh          # remove completely
#   sudo ./scripts/remove-service.sh stop     # stop only
#   ./scripts/remove-service.sh --help

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

#######################################
# Function to print colored output
# Print a message prefixed with a blue [INFO] tag.
# Globals:
#   BLUE, NC (read): ANSI color escape codes.
# Arguments:
#   $1 - Message text.
# Outputs:
#   Writes the tagged message to stdout (echo -e, so backslash escapes in $1
#   are interpreted).
# Returns:
#   0.
#######################################
print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

#######################################
# Print a message prefixed with a green [SUCCESS] tag.
# Globals:
#   GREEN, NC (read): ANSI color escape codes.
# Arguments:
#   $1 - Message text.
# Outputs:
#   Writes the tagged message to stdout (echo -e, so backslash escapes in $1
#   are interpreted).
# Returns:
#   0.
#######################################
print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

#######################################
# Print a message prefixed with a yellow [WARNING] tag.
# Globals:
#   YELLOW, NC (read): ANSI color escape codes.
# Arguments:
#   $1 - Message text.
# Outputs:
#   Writes the tagged message to stdout (echo -e, so backslash escapes in $1
#   are interpreted).
# Returns:
#   0.
#######################################
print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

#######################################
# Print a message prefixed with a red [ERROR] tag.
# Globals:
#   RED, NC (read): ANSI color escape codes.
# Arguments:
#   $1 - Message text.
# Outputs:
#   Writes the tagged message to stdout (echo -e, so backslash escapes in $1
#   are interpreted).
# Returns:
#   0.
#######################################
print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

#######################################
# Detect OS
# Identifies the host operating system family from `uname -s`.
# A Linux kernel is reported as "wsl" when /proc/version contains
# "Microsoft". The match is case-sensitive, so WSL2 kernels (which report
# lower-case "microsoft") are reported as "linux".
# Globals:
#   None
# Arguments:
#   None
# Outputs:
#   Writes one of: wsl, linux, macos, windows, unknown to stdout.
# Returns:
#   0.
#######################################
detect_os() {
    case "$(uname -s)" in
        Linux)
            if [[ -f /proc/version ]] && grep -q Microsoft /proc/version; then
                echo "wsl"
            else
                echo "linux"
            fi
            ;;
        Darwin)
            echo "macos"
            ;;
        CYGWIN*|MINGW32*|MSYS*|MINGW*)
            echo "windows"
            ;;
        *)
            echo "unknown"
            ;;
    esac
}

#######################################
# Remove service on Linux (systemd)
# Stops, disables and deletes the systemd service and its files.
# Stops and disables tempest-homekit-go.service (errors ignored), deletes the
# unit file and reloads systemd, deletes the "tempest" user, and recursively
# deletes /opt/tempest-homekit-go and /var/log/tempest-homekit-go. Then asks
# on the terminal whether to keep /etc/tempest-homekit-go; any answer other
# than y/Y deletes it.
# Globals:
#   EUID (read). REPLY (set by read).
# Arguments:
#   None
# Outputs:
#   Writes progress and the prompt to stdout; reads one character from stdin.
# Returns:
#   0 on success; exits 1 if not run as root. If stdin is not a terminal,
#   `read` fails at EOF and `set -e` exits after the files are already gone
#   but before the configuration question is acted on.
#######################################
remove_linux() {
    print_status "Removing service on Linux (systemd)..."

    # Check if running as root
    if [ "$EUID" -ne 0 ]; then
        print_error "Please run as root (sudo ./scripts/remove-service.sh)"
        exit 1
    fi

    # Stop and disable service
    print_status "Stopping systemd service..."
    systemctl stop tempest-homekit-go.service 2>/dev/null || true
    systemctl disable tempest-homekit-go.service 2>/dev/null || true

    # Remove systemd service file
    print_status "Removing systemd service file..."
    rm -f "/etc/systemd/system/tempest-homekit-go.service"
    systemctl daemon-reload

    # Remove user
    print_status "Removing tempest user..."
    userdel tempest 2>/dev/null || true

    # Remove files and directories
    print_status "Removing installation files..."
    rm -rf "/opt/tempest-homekit-go"
    rm -rf "/var/log/tempest-homekit-go"

    # Ask about configuration
    read -p "Do you want to keep the configuration file? (y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        print_status "Removing configuration..."
        rm -rf "/etc/tempest-homekit-go"
    fi

    print_success "Service removed successfully!"
}

#######################################
# Remove service on macOS (launchd)
# Stops and unloads the launchd daemon and deletes its files.
# Stops and unloads com.tempest.homekit (errors ignored), deletes its plist,
# recursively deletes /opt/tempest-homekit-go and /var/log/tempest-homekit-go,
# then asks whether to keep /etc/tempest-homekit-go; any answer other than
# y/Y deletes it.
# Globals:
#   EUID (read). REPLY (set by read).
# Arguments:
#   None
# Outputs:
#   Writes progress and the prompt to stdout; reads one character from stdin.
# Returns:
#   0 on success; exits 1 if not run as root.
#######################################
remove_macos() {
    print_status "Removing service on macOS (launchd)..."

    # Check if running as root
    if [ "$EUID" -ne 0 ]; then
        print_error "Please run as root (sudo ./scripts/remove-service.sh)"
        exit 1
    fi

    # Stop and unload service
    print_status "Stopping launchd service..."
    launchctl stop com.tempest.homekit 2>/dev/null || true
    launchctl unload "/Library/LaunchDaemons/com.tempest.homekit.plist" 2>/dev/null || true

    # Remove launchd plist
    print_status "Removing launchd plist..."
    rm -f "/Library/LaunchDaemons/com.tempest.homekit.plist"

    # Remove files and directories
    print_status "Removing installation files..."
    rm -rf "/opt/tempest-homekit-go"
    rm -rf "/var/log/tempest-homekit-go"

    # Ask about configuration
    read -p "Do you want to keep the configuration file? (y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        print_status "Removing configuration..."
        rm -rf "/etc/tempest-homekit-go"
    fi

    print_success "Service removed successfully!"
}

#######################################
# Remove service on Windows
# Stops and removes the TempestHomeKit NSSM service and deletes its files.
# If nssm is not on PATH, prints the PowerShell commands to remove the
# service manually and continues. Both "/c/Program Files/Tempest HomeKit Go"
# and "/c/ProgramData/Tempest HomeKit Go" (which holds config.env and the
# logs) are deleted before the "keep a configuration backup?" question is
# asked, so the answer only changes the message printed; nothing is kept.
# Globals:
#   REPLY (set by read).
# Arguments:
#   None
# Outputs:
#   Writes progress and the prompt to stdout; reads one character from stdin.
# Returns:
#   0 on success; non-zero (via `set -e`) if a command fails.
#######################################
remove_windows() {
    print_status "Removing service on Windows..."

    # Stop service
    print_status "Stopping Windows service..."
    if command -v nssm &> /dev/null; then
        nssm stop TempestHomeKit 2>/dev/null || true
        nssm remove TempestHomeKit confirm 2>/dev/null || true
    else
        print_warning "NSSM not found. Manual service removal may be required."
        print_warning "Run these commands in an Administrator PowerShell:"
        echo "  Stop-Service TempestHomeKit"
        echo "  sc.exe delete TempestHomeKit"
    fi

    # Remove files and directories
    print_status "Removing installation files..."
    rm -rf "/c/Program Files/Tempest HomeKit Go"
    rm -rf "/c/ProgramData/Tempest HomeKit Go"

    # Ask about configuration backup
    read -p "Do you want to keep a configuration backup? (y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        print_status "Configuration removed."
    else
        print_status "Configuration backup preserved."
    fi

    print_success "Service removed successfully!"
}

#######################################
# Stop service without removing (for all platforms)
# Stops the installed service without uninstalling it.
# Uses systemctl (Linux, WSL), launchctl stop (macOS) or nssm stop (Windows);
# stop errors are ignored, so "Service stopped." is printed even when the
# service was not installed or not running.
# Globals:
#   EUID (read). OS (set).
# Arguments:
#   None
# Outputs:
#   Writes status messages to stdout.
# Returns:
#   0; exits 1 if root is required but missing, or on an unsupported OS.
#######################################
stop_service() {
    print_status "Stopping Tempest HomeKit Go service..."

    OS=$(detect_os)

    case "$OS" in
        linux)
            if [ "$EUID" -ne 0 ]; then
                print_error "Please run as root (sudo ./scripts/remove-service.sh stop)"
                exit 1
            fi
            systemctl stop tempest-homekit-go.service 2>/dev/null || true
            print_success "Service stopped."
            ;;
        macos)
            if [ "$EUID" -ne 0 ]; then
                print_error "Please run as root (sudo ./scripts/remove-service.sh stop)"
                exit 1
            fi
            launchctl stop com.tempest.homekit 2>/dev/null || true
            print_success "Service stopped."
            ;;
        windows)
            if command -v nssm &> /dev/null; then
                nssm stop TempestHomeKit 2>/dev/null || true
                print_success "Service stopped."
            else
                print_warning "NSSM not found. Manual stop may be required."
                print_warning "Run in Administrator PowerShell: Stop-Service TempestHomeKit"
            fi
            ;;
        wsl)
            if [ "$EUID" -ne 0 ]; then
                print_error "Please run as root (sudo ./scripts/remove-service.sh stop)"
                exit 1
            fi
            systemctl stop tempest-homekit-go.service 2>/dev/null || true
            print_success "Service stopped."
            ;;
        *)
            print_error "Unsupported OS: $OS"
            exit 1
            ;;
    esac
}

#######################################
# Show usage
# Prints command-line usage.
# Globals:
#   None
# Arguments:
#   None
# Outputs:
#   Writes the usage text, using $0 as the script name, to stdout.
# Returns:
#   0.
#######################################
usage() {
    echo "Usage: $0 [stop]"
    echo ""
    echo "Commands:"
    echo "  (no args)  Remove and uninstall the service completely"
    echo "  stop       Stop the service without removing it"
    echo ""
    echo "Examples:"
    echo "  $0         # Remove service completely"
    echo "  $0 stop    # Stop service only"
}

#######################################
# Main removal function
# Parses the single optional command and runs the removal or stop.
# "stop" stops the service and exits; -h/--help prints usage and exits; any
# other value (or none) performs a full removal for the detected OS, with WSL
# treated as Linux.
# Globals:
#   OS (set).
# Arguments:
#   $1 - Optional: "stop", "-h" or "--help".
# Outputs:
#   Writes progress to stdout.
# Returns:
#   0 on success; exits 1 on an unsupported OS or when a remover exits.
#######################################
main() {
    # Check for stop command
    if [ "$1" = "stop" ]; then
        stop_service
        exit 0
    fi

    # Check for help
    if [ "$1" = "-h" ] || [ "$1" = "--help" ]; then
        usage
        exit 0
    fi

    echo "🗑️  Removing Tempest HomeKit Go Service..."

    # Detect OS
    OS=$(detect_os)
    print_status "Detected OS: $OS"

    case "$OS" in
        linux)
            remove_linux
            ;;
        macos)
            remove_macos
            ;;
        windows)
            remove_windows
            ;;
        wsl)
            print_warning "Running in WSL. Removing Linux service..."
            remove_linux
            ;;
        *)
            print_error "Unsupported OS: $OS"
            exit 1
            ;;
    esac

    print_success "Removal complete! 🎉"
}

# Run main function
main "$@"