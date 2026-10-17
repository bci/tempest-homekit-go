WORK LOG — Tempest HomeKit Go
================================

Overview
--------
This document captures a concise, developer-focused work log that explains what went into creating and iterating on the Tempest HomeKit Go application. It records high-level design decisions, notable code changes, testing and CI activities, contributor acknowledgements (human + AI), estimated time spent on major tasks, and lessons learned.

This file was generated and curated by the development team and with assistance from an automated coding assistant.

Contributors
------------
- Primary human developer: (You) — feature design, tests, JS/CSS tweaks, service wiring, release decisions.
- AI assistant (Copilot): generated code suggestions, test hardening, automated edits, documentation updates, CI workflow drafts and diagnostic tests.

Timeline & Estimated Effort
---------------------------
These are approximate times aggregated across the development sessions that produced the current repository state.
- Project scaffold and initial feature set (service + web UI + HomeKit): ~40-60 hours
- Popout/dedicated chart feature (design + implementation + tests): ~6-10 hours
- Headless (chromedp) test hardening and diagnostic tests: ~4-6 hours
- Unit tests and integration fixes: ~3-5 hours
- Release version bump, tag, and release workflow drafting: ~2-4 hours
- Documentation, README changes, acknowledgements, and housekeeping: ~2-3 hours
- Iterative debugging and CI troubleshooting: ~2-4 hours
- Alarm system development (editor, JSON validation, debug logging): ~8-10 hours
- Alarm change detection state persistence fix: ~2 hours
- Microsoft 365 OAuth2 email implementation: ~3-4 hours
- AWS SNS SMS notifications implementation (v1.8.0): ~4-6 hours
- UDP status integration for web console (v1.8.1): ~3-4 hours

Major Design Changes
--------------------
- Web UI architecture:
 - Centralized frontend logic placed in `pkg/web/static/script.js` and static popout template in `pkg/web/static/chart.html` to keep HTML minimal.
 - Charts use vendored Chart.js and date adapters for deterministic rendering.
- Deterministic Popout Configuration:
 - Chart popouts now receive a compact encoded `config` payload containing per-dataset metadata and `incomingUnits`. This ensures the popout graph closely matches small-card visuals (colors, dashes, fills).
- Headless Tests and Determinism:
 - Headless chromedp tests were hardened to prefer in-page hooks (e.g., `window.__dashboardReady`, `window.__lastStatusRaw`) and to inject vendored Chart.js and local scripts to avoid CDN flakiness.
- Unit and Label Handling:
 - Unit label helpers (`prettyUnitLabel`) were restored and harmonized between the dashboard and popout so both show consistent units.
- Release Automation:
 - Drafted a GitHub Actions `release.yml` to build cross-platform binaries and upload assets when an annotated tag is pushed. (Push of workflow file may require additional permissions.)
- Accessibility / UI polish:
 - Tempest Station link label was truncated to 15 characters for card layout with full URL in `title` and `aria-label` for hover and screen readers.
- Alarm System Architecture:
 - Implemented comprehensive alarm system with change-detection operators (`*field`, `>field`, `<field`)
 - Warning log level added (warn/warning aliases) between info and error
 - Alarm editor web UI with template variable system (18 variables including `alarm_description`)
 - JSON validation with line/column error reporting and helpful hints for missing @ prefix
 - Enhanced debug logging with pretty JSON output and detailed evaluation traces
 - **Critical Fix**: Changed `ProcessObservation()` to work with original alarms instead of copies, preserving `previousValue` state between calls. This fixed change-detection operators that were resetting state on every observation.
- Microsoft 365 Email Integration:
 - Implemented OAuth2 authentication for Microsoft 365/Exchange using Azure AD app credentials
 - Uses Microsoft Graph API for secure email delivery via `/users/{id}/sendMail` endpoint
 - Supports client credentials flow (server-to-server, no user interaction required)
 - Added dependencies: Azure SDK for Go, Microsoft Graph SDK for Go, golang.org/x/oauth2
 - Provider detection for "microsoft365", "o365", and "exchange" keywords
 - Automatic fallback to SMTP if OAuth2 credentials are missing
 - Full documentation with Azure AD setup instructions, permissions guide, and troubleshooting
 - Example alarm configuration with 6 realistic weather alert scenarios
- AWS SNS SMS Integration (v1.8.0):
 - Implemented complete AWS SNS SMS notification delivery using AWS SDK v2
 - Support for both direct SMS to phone numbers and SNS topic broadcasting
 - Two-tier credential system: admin credentials for setup, runtime credentials for sending
 - Comprehensive setup automation script (`scripts/setup-aws-sns.sh`) with interactive configuration
 - Environment-first architecture: all provider credentials in `.env`, alarm rules in JSON
 - Complete unit test suite covering configuration, templating, and error handling
 - Enhanced `.gitignore` to protect all `.env` variants and backups
 - Full documentation with IAM setup, security best practices, and production considerations
- UDP Status Integration (v1.8.1):
 - Integrated UDP device_status and hub_status broadcasts into web console status display
 - Added StatusManager connectivity to UDPDataSource via service layer wiring
 - Periodic status updates (30-second interval) to catch status-only packets without observations
 - Automatic status formatting: battery voltage with Good/Fair/Low indicators, uptime as "Xd Xh Xm Xs", RSSI as signal quality with dBm
 - Web console shows `"dataSource": "udp"` when status populated from UDP broadcasts
 - Eliminates need for `--use-web-status` web scraping when using `--udp-stream`
 - Test fixes for interface changes: updated all mock UDP listeners to return maps instead of structs
 - Fixed Twilio test expectations from "not yet implemented" to "credentials missing"

Best & Worst Prompts (AI-assisted development)
----------------------------------------------
- Best prompts (helpful):
 - "Make popout charts deterministic and match small-card visuals exactly (per-dataset styles, units) and add headless tests verifying parity." — This prompt led to compact, testable config encoding and robust headless tests.
 - "Harden headless tests to avoid CDN timing flakiness by injecting vendored Chart.js and exposing in-page test hooks." — This improved reliability in CI.
 - "The alarm 'Lux Change' is not triggering after these observations..." with full log output — Provided concrete reproduction case that led to discovering the state persistence bug in `ProcessObservation()`.

- Worst/ambiguous prompts (costly):
 - Broad requests like "Add release automation" without specifying how to handle GitHub token permissions led to local-only workflow drafts and a failed push due to token scope restrictions. Lesson: explicitly mention token policy or request a PR instead of direct pushes.
 - Vague UI change requests without specifying exact truncation length or accessibility expectations required follow-up decisions.

Prompt history
--------------
This repository now includes `PROMPT_HISTORY.md` which tracks paraphrased prompts used to guide AI-assisted edits and feature work. Please keep that file current — add paraphrases and outcomes when prompts are used to change the codebase.

TODO: Maintain `PROMPT_HISTORY.md` with new prompts and outcomes (see top-level todo list).

Notable Files and Where Changes Happened
----------------------------------------
- `pkg/web/static/script.js` — Centralized frontend; chart creation, popout config encoding, dashboard updates, and station card rendering.
- `pkg/web/static/chart.html` — Dedicated popout page; unit detection and tick formatting.
- `pkg/web/ui_headless_test.go` and `pkg/web/popout_diagnostics_test.go` — Headless tests and diagnostic runner to capture console logs and in-page errors.
- `main.go` — Version bump and startup changes.
- `README.md`, `REQUIREMENTS.md`, `CODE_REVIEW.md` — Documentation updates and acknowledgements.
- `.github/workflows/release.yml` — Drafted release workflow (local). Needs PR or push with workflow scope.

Testing & CI
------------
- Unit tests: `go test ./...` run locally and pass in the developer environment.
- Headless tests: chromedp-based UI tests were added and hardened to avoid CDN/time-related flakiness. One diagnostic test captures `window.__popoutError` and console logs.
- CI notes: Adding workflow changes requires a token with `workflow` permission or merging via a PR. The release workflow was drafted locally and the tag `v1.4.1` was created and pushed.

Accessibility & UX
------------------
- Popout and dashboard charts now share consistent unit labeling.
- Station link truncation added with `title` and `aria-label` so the full URL is accessible to screen readers.

Lessons Learned
---------------
- Vendor critical frontend dependencies (Chart.js/date adapter) for deterministic headless testing.
- Expose small test-only hooks in-page (`window.__dashboardReady`, `window.__lastStatusRaw`) to make headless tests less brittle.
- Keep UI text truncation consistent and add accessible attributes rather than relying on CSS-only truncation.
- When automating pushes to GitHub workflows, ensure the push token has `workflow` permissions or create a PR for manual merge.

Time Tracking (approximate)
---------------------------
- See "Timeline & Estimated Effort" above for task-level estimates. Keep these as a living estimate; they can be refined by tracking commits and timestamps if desired.

Risks & Open Items
------------------
- Release workflow push: requires a PAT with `workflow` scope or a PR merge by a maintainer with sufficient permissions.
- CI environment must have headless Chromium available for chromedp tests; vendoring Chart.js reduces but does not eliminate flakiness.
- More conservative assertions could be added to headless diagnostic tests to turn diagnostics into strict parity checks.

Next recommended actions
------------------------
1. Commit and push the `WORK_LOG.md` and the recent UI changes. If you want the release workflow applied, either push with a token that has `workflow` scope or open a PR so it can be merged by a maintainer.
2. Optionally, add a `CHANGELOG.md` with the `v1.4.1` notes and link to the release workflow.
3. Consider adding more strict headless assertions to ensure exact visual parity (colors, dataset counts) if you need CI-based enforcement.

Appendix: Example commit/PR message
----------------------------------
Title: "UI: truncate Tempest Station link + add WORK_LOG.md; tests: headless fixes"

Body:
- Truncate station link label to 15 chars; full URL in title/aria-label
- Add `WORK_LOG.md` capturing project history and AI-assisted work
- Headless test hardening and popout parity tests
- Drafted `release.yml`; needs PR or PAT with workflow scope to enable

---

If you'd like, I can now:
- Commit & push these changes and open a PR, or
- Expand the `WORK_LOG.md` with more granular timestamps (per-commit breakdown) and link to the exact commit SHAs and diffs.

Tell me how you'd like to proceed (push/PR, or more detail in the work log).

## 2025-12-29 — Release v1.11.0 and Documentation updates

Summary:
- Created and pushed annotated tag `v1.11.0` and published a GitHub release (release body drafted from `CHANGELOG.md`).
- Added a user-facing `CHANGELOG.md` entry for `v1.11.0` and a short `RELEASE_NOTES.md` for operators.
- Updated `VERSIONS.md` with concise release notes covering: Advanced Rules Engine, CSV/JSON alarm delivery, and Per-Alarm Scheduling.
- Updated `README.md`: moved the following features from "Future" to "Completed": Advanced Rules Engine, Additional Alarm Delivery Methods (CSV/JSON), and Per-Alarm Scheduling System.
- Consolidated environment variable documentation: removed duplicated inline env list and pointed flags to the authoritative "Available Environment Variables" table; moved that table closer to the flags for discoverability.
- Removed inline `Env:` annotations from command-line flag descriptions (flags now reference the environment table exclusively).
- Added a release badge to the `README.md` top header linking to tag `v1.11.0` and created `RELEASE_NOTES.md` for operator guidance.

Files changed and actions:
- `CHANGELOG.md` — added user-facing v1.11.0 entry. (commit: "chore(changelog): add v1.11.0 user-facing changelog entry")
- `RELEASE_NOTES.md` — created operator-focused release notes. (commit: "docs: add RELEASE_NOTES.md and release badge for v1.11.0")
- `README.md` — moved features to Completed, consolidated env var docs, added release badge. (commits: "docs: mark Advanced Rules Engine as completed in README", "docs: mark Additional Alarm Delivery Methods and Per-Alarm Scheduling as completed in README", "docs(readme): consolidate environment variables documentation and point to available variables table")
- `VERSIONS.md` — appended short release notes for v1.11.0. (commit: "docs: add alarm-related release notes to v1.11.0 in VERSIONS.md")
- Git tag: `v1.11.0` (annotated) created and pushed.
- GitHub release: created `v1.11.0` (release body: `CHANGELOG.md`).

Notes and follow-ups:
- Confirmed `HISTORY_*` environment variables are present in the "Available Environment Variables" table (`HISTORY_POINTS`, `HISTORY_REDUCE`, `HISTORY_REDUCE_METHOD`, `HISTORY_BIN_MINUTES`, `HISTORY_KEEP_RECENT_HOURS`) and the `--history*` flags remain documented in the flags list. If you want, I can move the environment table immediately above the flags list (instead of the current nearby placement) — it's already been moved closer for discoverability.
- If you prefer to keep `Env:` inline annotations for a few high-traffic flags, tell me which ones and I can re-add them consistently (but the canonical source will remain the environment table).

Estimate: ~30-60 minutes of editorial and release tasks (docs, tagging, release creation).

---
## 2026-10-16 — Documentation pass (branch `overnight-docs`)

Why: CLAUDE.md §11 requires complete in-language documentation (GoDoc, JSDoc, Google Shell
Style headers, PEP 257) on every declaration, and the repo had large gaps — most unexported
Go functions, all test functions, nearly all of `pkg/web/static/script.js` and the shell
scripts were undocumented, and several packages carried more than one `// Package` comment.
The prose docs had also drifted from the code (Go version, coverage numbers, architecture
tree, a "cross-platform" heading that ran the single-platform script).

What changed (comments and docs only — no code behaviour):
- GoDoc on every Go file (source and tests): one package comment per package, every func,
  method, type, const and var. Fields of column-aligned structs and one-line fake methods in
  tests are described in the type's doc comment, because a comment line between aligned
  fields makes gofmt re-pad the code lines.
- JSDoc on all first-party JS (dashboard `script.js`, alarm editor `script.js`, helpers,
  Jest tests and configs, tools/tests scripts). Skipped as third-party: `chart.umd.js(.map)`,
  `chartjs-adapter-date-fns.bundle.min.js`, `qrcode.min.js`, `qrcode-browser.js` (a copy of
  node-qrcode's `lib/browser.js`), and `date-fns.min.js` (an HTTP "Not found" body).
- Google Shell Style headers and Globals/Arguments/Outputs/Returns blocks on all shell
  scripts and the pre-commit hook; PEP 257 docstrings on the Python helpers. Style-guide
  code changes ([[ ]], local, main) were deliberately not made.
- Doc comments that were factually wrong were corrected (e.g. `logger.Info`, `Schedule`
  JSON keys, `maskString`, `NewWeatherSystemModern`, `GetConfig`'s "copy").
- README: Go 1.24+, measured coverage (44.2%, was claimed 60.3%), test/lint/Jest/hook
  commands, accurate architecture tree, build-script headings. New AGENTS.md and
  features.yaml. Two broken links in docs/development fixed.

How it was verified: every tracked .go file at tag `pre-docs` and now was parsed without
comments and printed canonically — identical; `//go:` directives identical; JS token streams,
Python ASTs (minus docstrings) and shell non-comment lines identical. `go build`, `go vet`,
`go test ./...` (incl. headless Chrome) and Jest (7 suites / 14 tests) pass before and after;
golangci-lint reports the same 5 pre-existing staticcheck findings before and after.

Bugs noticed during the pass were not fixed here; they are in `docs/code-review/2026-10-16.md`
on the `code-review` branch.
