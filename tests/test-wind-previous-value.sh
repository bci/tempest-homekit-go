#!/bin/bash

# Test script to verify last_wind_speed shows correct previous value
#
# Starts the app at debug level on web port 8085, waits 130 s for two live
# observations, stops it, and compares the previous/current wind speed from
# the "Change detected in wind_speed:" debug line with the "Last Wind Speed:"
# and "Wind speed:" values in the "Wind Change" notification, numerically
# via awk. Needs network access (WeatherFlow API). Log: /tmp/wind-test.log.
#
# Manual smoke test, not part of `go test` or CI: it prints results for a
# person to read and exits 0 whether or not the checks pass. Run it from the
# repository root after `go build` (it executes ./tempest-homekit-go and reads
# ./tempest-alarms.json).
#
# Usage: ./tests/test-wind-previous-value.sh
echo "=========================================="
echo "Testing Previous Value Fix"
echo "=========================================="
echo ""
echo "Testing Wind Change alarm to verify last_wind_speed"
echo "This test will run for 130 seconds to get 2 observations"
echo ""

# Start the application
echo "Starting application..."
./tempest-homekit-go --loglevel debug --alarms @tempest-alarms.json --web-port 8085 > /tmp/wind-test.log 2>&1 &
PID=$!

echo "Application started with PID: $PID"
echo "Waiting 130 seconds for 2 observations..."
sleep 130

# Kill the application
kill $PID 2>/dev/null
wait $PID 2>/dev/null

echo ""
echo "=========================================="
echo "Results:" 
echo "=========================================="
echo ""

# Check if wind alarm was triggered
if grep -q "🚨 Alarm triggered: Wind Change" /tmp/wind-test.log; then
    echo "✅ Wind Change alarm triggered"
    echo ""
    
    # Show the change detection debug line
    echo "Debug log shows:"
    grep "Change detected in wind_speed:" /tmp/wind-test.log | tail -1
    echo ""
    
    # Show the notification
    echo "Notification shows:"
    echo "---"
    grep -A 6 "🚨 ALARM: Wind Change" /tmp/wind-test.log | head -7
    echo "---"
    echo ""
    
    # Extract values for comparison
    DEBUG_LINE=$(grep "Change detected in wind_speed:" /tmp/wind-test.log | tail -1)
    PREV_VAL=$(echo "$DEBUG_LINE" | sed -n 's/.*: \([0-9.]*\) -> .*/\1/p')
    CURR_VAL=$(echo "$DEBUG_LINE" | sed -n 's/.* -> \([0-9.]*\)/\1/p')
    
    NOTIF_PREV=$(grep -A 6 "🚨 ALARM: Wind Change" /tmp/wind-test.log | grep "Last Wind Speed:" | sed 's/.*: //')
    NOTIF_CURR=$(grep -A 6 "🚨 ALARM: Wind Change" /tmp/wind-test.log | grep "Wind speed:" | sed 's/.*: //')
    
    echo "Comparison:"
    echo "  Debug log previous: $PREV_VAL"
    echo "  Notification previous: $NOTIF_PREV"
    echo "  Debug log current: $CURR_VAL"
    echo "  Notification current: $NOTIF_CURR"
    echo ""
    
    # Use awk for numeric comparison (handles 0.10 == 0.1)
    if awk "BEGIN {exit !($PREV_VAL == $NOTIF_PREV)}"; then
        echo "✅ Previous value matches numerically! ($PREV_VAL == $NOTIF_PREV)"
    else
        echo "❌ Previous value mismatch!"
        echo "   Expected: $PREV_VAL"
        echo "   Got: $NOTIF_PREV"
    fi
    
    # Also verify current value
    if awk "BEGIN {exit !($CURR_VAL == $NOTIF_CURR)}"; then
        echo "✅ Current value matches numerically! ($CURR_VAL == $NOTIF_CURR)"
    else
        echo "⚠️  Current value mismatch!"
        echo "   Expected: $CURR_VAL"
        echo "   Got: $NOTIF_CURR"
    fi
else
    echo "⚠️  Wind Change alarm did not trigger"
    echo ""
    echo "Wind speed observations:"
    grep "api data.*Wind:" /tmp/wind-test.log
fi

echo ""
echo "Full log saved to: /tmp/wind-test.log"
