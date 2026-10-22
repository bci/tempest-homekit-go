#!/bin/bash

# Test script to verify "Lux Change" alarm triggers on second observation
#
# Starts the app on web port 8082 with --alarms @tempest-alarms.json, waits
# 130 s so two 60-second API polls arrive, stops it, then prints the
# observations, the "Lux Change" evaluation lines and whether the alarm
# fired. Needs network access (WeatherFlow API). Log: /tmp/lux-test-full.log.
#
# Manual smoke test, not part of `go test` or CI: it prints results for a
# person to read and exits 0 whether or not the checks pass. Run it from the
# repository root after `go build` (it executes ./tempest-homekit-go and reads
# ./tempest-alarms.json).
#
# Usage: ./tests/test-lux-change-alarm.sh
echo "=========================================="
echo "Testing Lux Change Alarm (*lux operator)"
echo "=========================================="
echo ""
echo "This test will run for 130 seconds (enough for 2 observations)"
echo "API polls every 60 seconds, so we need:"
echo "  1st observation: Establish baseline"
echo "  2nd observation (at 60s): Detect change and trigger alarm"
echo ""

# Start the application in background, saving all output
echo "Starting application..."
./tempest-homekit-go --loglevel info --alarms @tempest-alarms.json --web-port 8082 > /tmp/lux-test-full.log 2>&1 &
PID=$!

echo "Application started with PID: $PID"
echo "Waiting 130 seconds for 2 observations..."
echo ""

# Wait 130 seconds
sleep 130

# Kill the application
kill $PID 2>/dev/null
wait $PID 2>/dev/null

echo "=========================================="
echo "Test completed. Analyzing results..."
echo "=========================================="
echo ""

# Show all api data lines
echo "Observations received:"
grep "api data" /tmp/lux-test-full.log

echo ""
echo "Lux Change alarm evaluation:"
grep -A 2 "Evaluating alarm: 'Lux Change'" /tmp/lux-test-full.log

echo ""
# Check if alarm was triggered
if grep -q "🚨 Alarm triggered: Lux Change" /tmp/lux-test-full.log; then
    echo "✅ SUCCESS: Lux Change alarm triggered!"
    echo ""
    grep "🚨 Alarm triggered: Lux Change" /tmp/lux-test-full.log
    echo ""
    echo "Console notification:"
    grep -A 5 "🚨 ALARM: Lux Change" /tmp/lux-test-full.log
else
    echo "❌ FAILURE: Lux Change alarm did NOT trigger"
    echo ""
    echo "Last 30 lines of log:"
    tail -30 /tmp/lux-test-full.log
fi

echo ""
echo "Full output saved to: /tmp/lux-test-full.log"
