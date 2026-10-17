# AGENTS.md

Orientation for coding agents (and people) arriving at this repository. The rules for
Claude Code are in [CLAUDE.md](CLAUDE.md); the user-facing manual is [README.md](README.md).
Everything below was checked against the code on 2026-10-16.

## What this is

`tempest-homekit-go` is a single Go binary that reads weather observations from a
WeatherFlow Tempest station and:

- publishes them to Apple HomeKit as a bridge with sensor accessories (`github.com/brutella/hap`);
- serves a web dashboard (default port 8080) with charts, unit conversion and alarm status;
- evaluates user-defined alarms and delivers notifications (console, syslog, oslog, eventlog,
  email via SMTP or Microsoft 365 Graph, SMS via Twilio or AWS SNS, webhook, CSV/JSON file);
- optionally runs a standalone web alarm editor (`--alarms-edit`) or a terminal status
  console (`--status`).

Data can come from the WeatherFlow REST API (token + station name), the local UDP broadcast
of a Tempest hub (`--udp-stream`, works offline), a custom station URL, or a built-in
synthetic weather generator (`--use-generated-weather`, for UI testing).

## Layout

| Path | What lives there |
| --- | --- |
| `main.go` | Entry point: loads `.env`, parses config, dispatches the one-shot modes (`--test-*`, `--alarms-edit`, `--status`, `--version`, `--cleardb`) and otherwise starts the service. |
| `pkg/config` | Command-line flags, environment variables (`.env` via godotenv), validation, elevation parsing. |
| `pkg/service` | Service orchestration: data-source factory, polling loop, wiring of HomeKit, web and alarms. |
| `pkg/weather` | WeatherFlow API client, the `DataSource` interface and its API / UDP / generated implementations, TempestWX status scraping (chromedp). |
| `pkg/udp` | Listener for the Tempest hub's UDP broadcast (port 50222). |
| `pkg/generator` | Deterministic synthetic weather generator. |
| `pkg/homekit` | HAP bridge and accessories, custom characteristics. HAP pairing state is stored in `./db` at runtime. |
| `pkg/web` | Dashboard HTTP server and JSON API; `pkg/web/static` holds the browser JS/CSS/HTML served from disk. |
| `pkg/alarm` | Alarm types, condition evaluator, scheduling, manager (with file watching and reload), notifiers, and the `--test-*` delivery helpers. |
| `pkg/alarm/editor` | Standalone alarm-editor web server and its static assets. |
| `pkg/status` | Terminal (tview) status console and its themes. |
| `pkg/logger` | Level-filtered logging used everywhere. |
| `pkg/types` | Small shared types. |
| `pkg/testhelpers`, `pkg/web/testhelpers` | Headless-browser (chromedp) helpers used by browser-tagged tests. |
| `scripts/` | Build, cross-build, service install/remove, AWS SNS setup, GoDoc server, doc-maintenance helpers. |
| `tests/` | Manual integration shell scripts that drive a built binary (long sleeps; not run by CI). |
| `docs/`, `pkg/*/README.md` | Feature and package documentation. |
| `examples/` | Example alarm configuration files. |

Vendored third-party browser files (do not edit, do not document): `pkg/web/static/chart.umd.js`
(+ `.map`), `chartjs-adapter-date-fns.bundle.min.js`, `qrcode.min.js`, `date-fns.min.js`.
They are refreshed with `make vendor-static`.

## Commands

Build, vet, test and lint lines verified on macOS with Go 1.26.7 (go.mod requires go 1.24.0).

```sh
go build ./...                       # build every package
go build -o tempest-homekit-go       # build the binary in the repo root
go vet ./...
go test -tags=no_browser ./...       # what CI's main job runs; no Chrome needed
go test ./...                        # also runs headless-Chrome UI tests in pkg/web, pkg/testhelpers (~30 s)
golangci-lint run ./...              # no repo config; CI runs it with --issues-exit-code=0
./tempest-homekit-go --use-generated-weather --disable-homekit   # smoke run, dashboard on :8080 (from README; not run in the 2026-10-16 pass)
```

JavaScript unit tests use Jest with jsdom (`jest.web.config.json`, tests in
`pkg/web/static/__tests__/`). `package.json` is **not tracked** (it is in `.gitignore`), so a
fresh clone has no `npm test`; with a local `package.json` + `node_modules` the script is
`jest --runInBand --detectOpenHandles --config jest.web.config.json`.

Git hooks live in `.githooks/` (enable with `git config core.hooksPath .githooks`). The
pre-commit hook runs `gofmt -s -w` on staged Go files and re-stages them, and runs
`npm test` when JS files are staged.

## Conventions

- New data sources implement `weather.DataSource` and are created through
  `pkg/service/datasource_factory.go`.
- No inline CSS or JavaScript in HTML; HTML references external `.css`/`.js` files.
- Logging goes through `pkg/logger` (levels error/warn/info/debug), not bare `log`.
- Documentation standard (CLAUDE.md §11): GoDoc on every Go declaration, JSDoc in JS,
  Google Shell Style headers/function blocks in shell, PEP 257 in Python.
- Bookkeeping: `WORK_LOG.md` (the repo's work log — the CLAUDE.md profile calls it
  `WORKLOG.md`), `VERSIONS.md`, `features.yaml`.

## Guardrails

- Never commit `.env` files or credentials; `.env.example` is the only tracked env file.
  Alarm configs can hold SMTP/Twilio/AWS/Microsoft 365 credentials — keep real ones out of git.
- `scripts/install-service.sh`, `remove-service.sh`, `kill-all.sh` and `setup-aws-sns.sh` change
  the host (services, processes, AWS resources). Do not run them from an agent session.
- Running the binary without `--disable-homekit` advertises a HomeKit bridge on the local
  network via mDNS and writes pairing state to `./db`; `--cleardb` deletes that state.
- `.github/workflows/release.yml` publishes releases on tag push — do not push tags casually.

## Not done yet / known gaps

- `pkg/config.fetchWeatherFlowStationCoords` is a stub that always returns an error, so
  automatic elevation lookup falls back to a short hard-coded city table.
- `pkg/web/static/date-fns.min.js` contains an HTTP "Not found" body rather than the library.
- `package.json` is untracked, so the Jest suite is not reproducible from a clean clone and
  CI does not run it.
- CI pins Go `1.23.x` while go.mod requires 1.24.0 (setup-go relies on toolchain download).
- See `docs/code-review/` for open review findings.
