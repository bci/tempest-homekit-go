#!/bin/bash

# Tempest HomeKit Go Cross-Platform Build Script
# Builds for all supported platforms (Linux, macOS, Windows)
#
# Output files are named tempest-homekit-go-<linux|macos|windows>-<arch>[.exe]
# and written to dist/ at the project root (scripts/build.sh writes to build/
# instead, which is where install-service.sh looks for binaries).
#
# Usage: ./scripts/build-cross-platform.sh    (from any directory; no arguments)
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
#   Writes progress messages to stdout and the binary to dist/<name>.
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
        -o "dist/$output_name" \
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
# Builds binaries for all five supported OS/arch targets into dist/.
# Changes to the project root (the parent of this script's directory), so
# the script can be run from any directory, then builds linux/amd64,
# linux/arm64, darwin/amd64, darwin/arm64 and windows/amd64 in that order.
# Globals:
#   SCRIPT_DIR, PROJECT_DIR, VERSION, COMMIT, BUILD_TIME (set).
# Arguments:
#   $@ - Accepted but ignored.
# Outputs:
#   Writes progress and a listing of dist/ to stdout.
# Returns:
#   0 on success; exits non-zero (via `set -e`) at the first failed build.
#######################################
main() {
    echo "🌍 Building Tempest HomeKit Go for all platforms..."

    # Get the current directory
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

    # Change to project directory
    cd "$PROJECT_DIR"

    # Create dist directory
    mkdir -p dist

    # Get version info
    read VERSION COMMIT BUILD_TIME <<< $(get_version_info)

    print_status "Version: $VERSION"
    print_status "Commit: $COMMIT"
    print_status "Build Time: $BUILD_TIME"

    # Build for all platforms
    print_status "Building cross-platform binaries..."

    build_for_platform "linux" "amd64" "tempest-homekit-go-linux-amd64"
    build_for_platform "linux" "arm64" "tempest-homekit-go-linux-arm64"
    build_for_platform "darwin" "amd64" "tempest-homekit-go-macos-amd64"
    build_for_platform "darwin" "arm64" "tempest-homekit-go-macos-arm64"
    build_for_platform "windows" "amd64" "tempest-homekit-go-windows-amd64"

    # List built binaries
    print_success "Cross-platform build complete! 🎉"
    echo ""
    print_status "Built binaries in dist/:"
    ls -la dist/

    echo ""
    print_status "For platform-specific builds, use:"
    echo "  ./scripts/build.sh"
}

# Run main function
main "$@"