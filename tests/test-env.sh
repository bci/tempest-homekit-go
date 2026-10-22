#!/bin/bash
# Test that .env is loaded
#
# Exports HISTORY_POINTS=12345 in this shell and runs the app with generated
# weather, HomeKit disabled and --history-read, printing the first log line
# matching "generating.*historical" so a person can see whether the value was
# honoured. Because the variable is exported directly, this exercises
# environment-variable handling rather than reading it from a .env file. The
# app keeps running until it exits on its own (no timeout is applied).
#
# Usage: ./tests/test-env.sh    (from the repository root, after `go build`)
export HISTORY_POINTS=12345
./tempest-homekit-go --use-generated-weather --disable-homekit --history-read 2>&1 | grep -i "generating.*historical" | head -1
