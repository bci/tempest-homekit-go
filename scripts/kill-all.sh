#!/bin/bash

# kill-all.sh
# Kills all running tempest-homekit-go processes on the system
# Also frees up UDP port 50222 used by --stream-udp mode
#
# Usage:
#   ./scripts/kill-all.sh           # Kill with SIGTERM (graceful)
#   ./scripts/kill-all.sh -9        # Kill with SIGKILL (force)
#   ./scripts/kill-all.sh --force   # Kill with SIGKILL (force)
#
# Processes are found with `pgrep -f tempest-homekit-go`, which matches the
# string anywhere in a full command line - not only the app binary but any
# process whose arguments or path contain it (an editor or `tail -f` on a
# repo file, a test script run by absolute path, this script itself when
# invoked by a path that includes the repo directory). Their PIDs and command
# lines are printed before the signal is sent. After one second it checks
# again; if none remain it reports whether anything still holds port 50222
# (lsof checks TCP and UDP) and exits 0, otherwise it lists the survivors and
# exits 1. Only the first argument is examined; other arguments are ignored.

set -e

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Parse arguments
SIGNAL="TERM"
SIGNAL_FLAG="-15"

if [[ "$1" == "-9" ]] || [[ "$1" == "--force" ]]; then
    SIGNAL="KILL"
    SIGNAL_FLAG="-9"
    echo -e "${YELLOW}Force kill mode enabled (SIGKILL)${NC}"
fi

# Find all tempest-homekit-go processes
PIDS=$(pgrep -f "tempest-homekit-go" 2>/dev/null || true)

if [ -z "$PIDS" ]; then
    echo -e "${GREEN}✓${NC} No tempest-homekit-go processes found"
    exit 0
fi

# Count processes
PROC_COUNT=$(echo "$PIDS" | wc -l | tr -d ' ')

echo -e "${BLUE}Found ${PROC_COUNT} tempest-homekit-go process(es):${NC}"
echo ""

# Display process information before killing
ps -p $PIDS -o pid,ppid,command 2>/dev/null || true

echo ""
echo -e "${YELLOW}Sending SIG${SIGNAL} to process(es)...${NC}"

# Kill each process
KILLED=0
FAILED=0

for PID in $PIDS; do
    if kill $SIGNAL_FLAG $PID 2>/dev/null; then
        echo -e "${GREEN}✓${NC} Killed process $PID"
        KILLED=$((KILLED + 1))
    else
        echo -e "${RED}✗${NC} Failed to kill process $PID (may have already exited)"
        FAILED=$((FAILED + 1))
    fi
done

# Wait a moment for processes to terminate
sleep 1

# Verify all processes are gone
REMAINING=$(pgrep -f "tempest-homekit-go" 2>/dev/null || true)

echo ""
if [ -z "$REMAINING" ]; then
    echo -e "${GREEN}✓${NC} All processes terminated successfully"
    echo -e "  ${GREEN}Killed:${NC} $KILLED"
    if [ $FAILED -gt 0 ]; then
        echo -e "  ${YELLOW}Already gone:${NC} $FAILED"
    fi
    
    # Check if UDP port 50222 is still in use
    PORT_CHECK=$(lsof -i :50222 2>/dev/null || true)
    if [ -n "$PORT_CHECK" ]; then
        echo ""
        echo -e "${YELLOW}⚠${NC}  UDP port 50222 still in use by another process:"
        lsof -i :50222 2>/dev/null || true
        echo -e "${BLUE}Tip: Kill it with: kill <PID>${NC}"
    else
        echo -e "  ${GREEN}UDP port 50222:${NC} Free"
    fi
    
    exit 0
else
    REMAINING_COUNT=$(echo "$REMAINING" | wc -l | tr -d ' ')
    echo -e "${RED}✗${NC} Warning: $REMAINING_COUNT process(es) still running:"
    ps -p $REMAINING -o pid,command 2>/dev/null || true
    echo ""
    echo -e "${YELLOW}Tip: Use './scripts/kill-all.sh --force' to send SIGKILL${NC}"
    exit 1
fi
