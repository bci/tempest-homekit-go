#!/bin/bash
# myrun.sh
# Simple script re-build start 2 background processes:
# 1. tempest-homekit-go: console with [additional flags...]
# 2. tempest-homekit-go: editor
# Usage: ./scripts/myrun.sh [additional flags...]
# Examples: ./scripts/myrun.sh --disable-homekit --web-port 8086
#
# Developer convenience launcher. Run from the repository root: it calls
# ./scripts/kill-all.sh to stop every running tempest-homekit-go process, runs
# `go build` in the current directory, then starts three instances in the
# background and returns immediately:
#   - the console with `--alarms @tempest-alarms.json` plus your flags,
#   - the alarm editor with `--alarms-edit @tempest-alarms.json` plus your flags,
#   - a webhook listener on port 8082 (`--webhook-listener-port 8082`), which
#     does not receive your extra flags.
# The background processes keep running after the script exits; stop them with
# ./scripts/kill-all.sh. `set -e` stops the script if kill-all or the build
# fails.
set -e

./scripts/kill-all.sh # Ensure no other instances are running

# Build and run
go build

./tempest-homekit-go --alarms @tempest-alarms.json "$@" &
./tempest-homekit-go --alarms-edit @tempest-alarms.json "$@" å&
./tempest-homekit-go --webhook-listener-port 8082 &
