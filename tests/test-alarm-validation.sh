#!/bin/bash
# Test script for alarm JSON validation with timeout
#
# Runs the app seven times for at most 2 s each (`timeout 2s`) with a
# missing @ prefix, malformed JSON, wrong structure, missing condition,
# missing channels, and finally the valid @tempest-alarms.json, printing the
# first matching error (or "Loaded alarm") lines for each. `timeout` is GNU
# coreutils: absent on stock macOS, where every case prints nothing.
#
# Manual smoke test, not part of `go test` or CI: it prints results for a
# person to read and exits 0 whether or not the checks pass. Run it from the
# repository root after `go build` (it executes ./tempest-homekit-go and reads
# ./tempest-alarms.json).
#
# Usage: ./tests/test-alarm-validation.sh

echo "Testing Alarm JSON Validation"
echo "=============================="
echo ""

# Test 1: Missing @ prefix (filename without @)
echo "Test 1: Missing @ prefix (should show hint)"
timeout 2s ./tempest-homekit-go --loglevel error --alarms tempest-alarms.json 2>&1 | grep -A3 "ERROR.*alarm" | head -5
echo ""

# Test 2: Invalid JSON syntax (missing closing brace)
echo "Test 2: Invalid JSON - missing closing brace"
timeout 2s ./tempest-homekit-go --loglevel error --alarms '{"alarms":[{"name":"test"' 2>&1 | grep -A3 "ERROR.*alarm" | head -5
echo ""

# Test 3: Invalid JSON with typo (missing comma)
echo "Test 3: Invalid JSON - missing comma"
timeout 2s ./tempest-homekit-go --loglevel error --alarms '{"alarms":[{"name":"test" "enabled":true}]}' 2>&1 | grep -A3 "ERROR.*alarm" | head -5
echo ""

# Test 4: Valid JSON but wrong structure
echo "Test 4: Valid JSON but wrong structure (missing alarms array)"
timeout 2s ./tempest-homekit-go --loglevel error --alarms '{"wrong":"field"}' 2>&1 | grep -A3 "ERROR.*alarm" | head -5
echo ""

# Test 5: Valid JSON with correct structure but invalid alarm (missing condition)
echo "Test 5: Valid structure but missing required field (condition)"
timeout 2s ./tempest-homekit-go --loglevel error --alarms '{"alarms":[{"name":"test","enabled":true,"channels":[]}]} ' 2>&1 | grep -A3 "ERROR.*alarm" | head -5
echo ""

# Test 6: Valid JSON with correct structure but invalid alarm (missing channels)
echo "Test 6: Valid structure but missing required field (channels)"
timeout 2s ./tempest-homekit-go --loglevel error --alarms '{"alarms":[{"name":"test","enabled":true,"condition":"temp>85"}]}' 2>&1 | grep -A3 "ERROR.*alarm" | head -5
echo ""

# Test 7: Correct usage with @ prefix
echo "Test 7: Correct usage with @ prefix (should succeed)"
timeout 2s ./tempest-homekit-go --loglevel info --alarms @tempest-alarms.json 2>&1 | grep -E "(Loaded alarm|alarms are enabled)" | head -5
echo ""

echo "=============================="
echo "All tests completed"
