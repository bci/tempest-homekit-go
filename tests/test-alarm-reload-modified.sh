#!/bin/bash

# Test script to verify alarm config reload shows detailed logging
#
# Starts the app on web port 8083 with --alarms @tempest-alarms.json, edits
# the "Lux Change" description in tempest-alarms.json with sed to trigger the
# file watcher, prints the last 20 log lines, then restores the file from
# tempest-alarms.json.backup and stops the app. If interrupted before the
# restore, tempest-alarms.json is left modified and the .backup/.tmp files
# remain. Log: /tmp/reload-test2.log. Runs about 8 s.
#
# Manual smoke test, not part of `go test` or CI: it prints results for a
# person to read and exits 0 whether or not the checks pass. Run it from the
# repository root after `go build` (it executes ./tempest-homekit-go and reads
# ./tempest-alarms.json).
#
# Usage: ./tests/test-alarm-reload-modified.sh
echo "=========================================="
echo "Testing Alarm Config Reload Logging"
echo "=========================================="
echo ""

# Backup the original file
cp tempest-alarms.json tempest-alarms.json.backup

# Start the application in background
echo "Starting application with alarm file watching..."
./tempest-homekit-go --loglevel info --alarms @tempest-alarms.json --web-port 8083 > /tmp/reload-test2.log 2>&1 &
PID=$!
echo "Application started with PID: $PID"

# Wait for initialization
echo "Waiting 5 seconds for initialization..."
sleep 5

echo ""
echo "Initial load output:"
grep -E "(Alarm manager initialized|Loaded alarm:|alarms are enabled)" /tmp/reload-test2.log | head -6
echo ""

# Make a small change to the file (add a space to description)
echo "Modifying tempest-alarms.json (changing a description)..."
sed -i.tmp 's/"This alarm should alert on LUX change"/"This alarm should alert on LUX change "/' tempest-alarms.json

# Wait for reload to happen
echo "Waiting 3 seconds for file watcher to detect change..."
sleep 3

echo ""
echo "=========================================="
echo "Reload Output (last 20 lines):"
echo "=========================================="
tail -20 /tmp/reload-test2.log

# Restore original file
echo ""
echo "Restoring original file..."
mv tempest-alarms.json.backup tempest-alarms.json
rm -f tempest-alarms.json.tmp

# Clean up
echo "Stopping application..."
kill $PID 2>/dev/null
wait $PID 2>/dev/null

echo ""
echo "✅ Test complete. Full log saved to: /tmp/reload-test2.log"
echo ""
echo "Expected output after reload:"
echo "  ✓ INFO: Alarm config file changed, reloading: tempest-alarms.json"
echo "  ✓ INFO: Alarm manager initialized with 3 alarms"
echo "  ✓ INFO: Loaded alarm: Hot outside"
echo "  ✓ INFO: Loaded alarm: Lightning Nearby"
echo "  ✓ INFO: Loaded alarm: Lux Change"
echo "  ✓ INFO: 3 of 3 alarms are enabled"
echo "  ✓ INFO: Alarm config reloaded successfully"
