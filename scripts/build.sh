#!/bin/bash

# Tempest HomeKit Go Build Script
# Builds for the current platform only
#
# "Current platform" means the host OS family: both the amd64 and arm64
# binaries are built for it (Windows: amd64 only), named
# tempest-homekit-go-<linux|macos|windows>-<arch>[.exe], into build/ at the
# project root. Use scripts/build-cross-platform.sh for every OS at once.
#
# Usage: ./scripts/build.sh    (from any directory; takes no arguments)
# Requires: go; git is optional (used only for version/commit metadata).

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
# Get version info
# Collects version metadata for the -ldflags -X injection.
# VERSION is the most recent git tag (falls back to "v1.0.0"), COMMIT the
# short HEAD hash (falls back to "unknown"), BUILD_TIME the current UTC time
# in ISO 8601. It is called inside $(...), so the globals it sets live only in
# that subshell; main re-reads them from the printed line with `read`.
# Globals:
#   VERSION, COMMIT, BUILD_TIME (set, in the calling subshell only).
# Arguments:
#   None
# Outputs:
#   Writes "VERSION COMMIT BUILD_TIME" as one space-separated line to stdout.
# Returns:
#   0.
#######################################
get_version_info() {
    VERSION=$(git describe --tags --abbrev=0 2>/dev/null || echo "v1.0.0")
    COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
    BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)
    echo "$VERSION" "$COMMIT" "$BUILD_TIME"
}

#######################################
# Build for specific platform
# Cross-compiles the module in the current directory for one target.
# Sets GOOS/GOARCH for a single `go build` and passes -X main.version,
# -X main.commit and -X main.buildTime. The main package declares none of
# those variables, so the linker ignores them and the values are not
# embedded in the binary.
# Globals:
#   VERSION, COMMIT, BUILD_TIME (read): values for the -X flags.
# Arguments:
#   $1 - Target GOOS (linux, darwin, windows).
#   $2 - Target GOARCH (amd64, arm64).
#   $3 - Output file base name; ".exe" is appended when $1 is windows.
# Outputs:
#   Writes progress messages to stdout and the binary to build/<name>.
# Returns:
#   0 on success. Under `set -e` a failing `go build` exits the whole script
#   before the $? check, so the "Failed to build" branch and `return 1` are
#   never reached.
#######################################
build_for_platform() {
    local os=$1
    local arch=$2
    local output_name=$3

    print_status "Building for $os/$arch..."

    local ldflags="-X main.version=$VERSION -X main.commit=$COMMIT -X main.buildTime=$BUILD_TIME"

    if [[ "$os" == "windows" ]]; then
        output_name="${output_name}.exe"
    fi

    GOOS=$os GOARCH=$arch go build \
        -ldflags "$ldflags" \
        -o "build/$output_name" \
        .

    if [[ $? -eq 0 ]]; then
        print_success "Built $output_name"
    else
        print_error "Failed to build for $os/$arch"
        return 1
    fi
}

#######################################
# Main build function
# Builds the release binaries for the host OS family into build/.
# Changes to the project root (the parent of this script's directory), so
# the script can be run from any directory. Linux and WSL hosts get
# linux/amd64 and linux/arm64; macOS hosts get darwin/amd64 and
# darwin/arm64; Windows (Cygwin/MinGW/MSYS) hosts get windows/amd64.
# Globals:
#   SCRIPT_DIR, PROJECT_DIR, VERSION, COMMIT, BUILD_TIME, CURRENT_OS (set).
# Arguments:
#   $@ - Accepted but ignored.
# Outputs:
#   Writes progress, a listing of build/ and an install hint to stdout.
# Returns:
#   0 on success; exits 1 on an unsupported OS, and exits non-zero (via
#   `set -e`) if any build fails.
#######################################
main() {
    echo "🌤️ Building Tempest HomeKit Go for current platform..."

    # Get the current directory
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

    # Change to project directory
    cd "$PROJECT_DIR"

    # Create build directory
    mkdir -p build

    # Get version info
    read VERSION COMMIT BUILD_TIME <<< $(get_version_info)

    print_status "Version: $VERSION"
    print_status "Commit: $COMMIT"
    print_status "Build Time: $BUILD_TIME"

    # Detect current OS
    CURRENT_OS=$(detect_os)
    print_status "Detected OS: $CURRENT_OS"

    # Build targets based on current OS
    case "$CURRENT_OS" in
        linux)
            build_for_platform "linux" "amd64" "tempest-homekit-go-linux-amd64"
            build_for_platform "linux" "arm64" "tempest-homekit-go-linux-arm64"
            ;;
        macos)
            build_for_platform "darwin" "amd64" "tempest-homekit-go-macos-amd64"
            build_for_platform "darwin" "arm64" "tempest-homekit-go-macos-arm64"
            ;;
        windows)
            build_for_platform "windows" "amd64" "tempest-homekit-go-windows-amd64"
            ;;
        wsl)
            build_for_platform "linux" "amd64" "tempest-homekit-go-linux-amd64"
            build_for_platform "linux" "arm64" "tempest-homekit-go-linux-arm64"
            ;;
        *)
            print_error "Unsupported OS: $CURRENT_OS"
            exit 1
            ;;
    esac

    # List built binaries
    print_success "Build complete! 🎉"
    echo ""
    print_status "Built binaries:"
    ls -la build/

    echo ""
    print_status "To install as a service, run:"
    echo "  sudo ./scripts/install-service.sh"
}

# Run main function
main "$@"