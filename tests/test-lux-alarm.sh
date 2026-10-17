#!/bin/bash

# Test script to verify "Lux Change" alarm triggers correctly
# This simulates the lux changes the user observed
#
# Runs the app in the foreground for 30 s (`timeout 30s`) with
# --alarms @tempest-alarms.json on the default web port, tees all output to
# /tmp/lux-test-output.txt, then reports whether "Alarm triggered: Lux Change"
# appeared. Live weather data is used; nothing is simulated by the script.
# `timeout` is GNU coreutils: absent on stock macOS, where the app is never
# started and the test reports a failure.
#
# Manual smoke test, not part of `go test` or CI: it prints results for a
# person to read and exits 0 whether or not the checks pass. Run it from the
# repository root after `go build` (it executes ./tempest-homekit-go and reads
# ./tempest-alarms.json).
#
# Usage: ./tests/test-lux-alarm.sh

echo "=========================================="
echo "Testing Lux Change Alarm with *lux operator"
echo "=========================================="
echo ""
echo "This test will run for about 30 seconds to simulate lux changes."
echo "Expected behavior: Alarm should trigger after the first lux value is established."
echo ""
echo "Starting application with --loglevel info..."
echo ""

# Run with timeout to auto-terminate after 30 seconds
timeout 30s ./tempest-homekit-go --loglevel info --alarms @tempest-alarms.json 2>&1 | tee /tmp/lux-test-output.txt

echo ""
echo "=========================================="
echo "Test completed. Checking results..."
echo "=========================================="
echo ""

# Check if alarm was triggered
if grep -q "🚨 Alarm triggered: Lux Change" /tmp/lux-test-output.txt; then
    echo "✅ SUCCESS: Lux Change alarm triggered as expected!"
    echo ""
    echo "Alarm trigger details:"
    grep "🚨 Alarm triggered: Lux Change" /tmp/lux-test-output.txt
    echo ""
    echo "Console notifications:"
    grep -A 4 "🚨 ALARM: Lux Change" /tmp/lux-test-output.txt | head -5
else
    echo "❌ FAILURE: Lux Change alarm did not trigger"
    echo ""
    echo "Last 20 lines of output:"
    tail -20 /tmp/lux-test-output.txt
fi

echo ""
echo "Full output saved to: /tmp/lux-test-output.txt"
