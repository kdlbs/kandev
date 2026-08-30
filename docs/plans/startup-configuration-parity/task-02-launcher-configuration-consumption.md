---
id: "02-launcher-configuration-consumption"
title: "Launcher configuration consumption"
status: done
wave: 2
depends_on: ["01-configuration-catalog-and-source-resolution"]
plan: "plan.md"
spec: "../../specs/platform/requirements/startup-configuration-parity.md"
---

# Task 02: Launcher configuration consumption

Make every Go launcher mode resolve its bootstrap settings from the shared
configuration contract before it starts the backend.

## Acceptance

- `dev`, `start`, `run`, and `service` use the same source resolver.
- Launch flags override environment values. Environment values override YAML.
- The launcher honors YAML for `homeDir`, `server.host`, `server.port`,
  `database.path`, `logging.level`, `launcher.webPort`,
  `launcher.healthTimeoutMs`, and `launcher.noBrowser`.
- Compatibility environment variables keep their current priority, including
  `KANDEV_BACKEND_PORT` over `KANDEV_PORT`.
- The launcher does not replace a YAML `database.path` with its default path.
- The child backend reads the exact file selected by the launcher even when its
  working directory differs.
- Internal path handoff remains private and does not appear in the public
  operator catalog.
- Launches without a configuration file keep current defaults.
- Invalid bootstrap configuration stops before child startup and reports the
  selected file.

## Files likely touched

- `apps/backend/internal/launcher/constants.go`
- `apps/backend/internal/launcher/ports.go`
- `apps/backend/internal/launcher/health.go`
- `apps/backend/internal/launcher/env.go`
- `apps/backend/internal/launcher/start.go`
- `apps/backend/internal/launcher/dev.go`
- `apps/backend/internal/launcher/service.go`
- `apps/backend/internal/launcher/*_test.go`
- `apps/backend/internal/common/config/source.go`

## Dependencies

Task 01 defines the catalog, source resolver, and typed launcher section.

## TDD sequence

1. Add launcher tests for each precedence layer, YAML database path, home file
   discovery, selected-path handoff, no-browser, health timeout, and port
   compatibility. Run them RED.
2. Add a bootstrap configuration projection and replace direct stable
   environment reads in the launcher.
3. Preserve only required internal child wiring in the child environment.
4. Run focused tests GREEN. Run the complete launcher suite and build the Go
   launcher binary.

## Verification

```bash
cd apps/backend && go test ./internal/launcher -run '^Test.*(Config|Port|Health|Browser|DatabasePath|Home)' -count=1
cd apps/backend && go test ./internal/launcher -count=1
cd apps/backend && go build ./cmd/kandev
```

## Risks

- Service installation and direct launch can have different working
  directories. Tests must cover the selected-path handoff.
- The internal backend port handoff is not operator configuration. Removing it
  would break child startup even after public port resolution is typed.
- `homeDir` controls data paths and configuration discovery. Bootstrap code
  must resolve it only once.

## Output contract

Record RED and GREEN results, precedence evidence for every launcher field,
child handoff behavior, files changed, and remaining risks in `## Results`.

## Results

- RED: `go test ./internal/launcher -run "^Test(Launcher|DevBootstrap)" -count=1`
  failed to compile before the bootstrap projection and private handoff existed.
- GREEN: `go test ./internal/launcher -run '^Test.*(Config|Port|Health|Browser|DatabasePath|Home)' -count=1`
  passed, covering YAML values, environment-over-YAML, flag-over-environment,
  dev web-port selection, health timeout, browser suppression, database path,
  invalid-file reporting, and selected-file handoff.
- GREEN: `go test ./internal/launcher -count=1` passed.
- GREEN: `go build ./cmd/kandev` passed.
- The launcher now loads the shared config before port, data-directory, and
  child startup decisions. Start, run, dev, and service installation preserve
  the selected file through private `KANDEV_INTERNAL_CONFIG_FILE` wiring.
- YAML database and logging values remain typed in the child process; the
  launcher does not copy those YAML values into public environment overrides.
- Service units preserve YAML home/logging values and emit the private file
  handoff. Existing no-file defaults and `KANDEV_BACKEND_PORT` over
  `KANDEV_PORT` behavior remain covered by the complete launcher suite.

Review remediation:

- Service bootstrap discovery now resolves flag, explicit environment, and
  service-mode default homes before selecting `config.yaml`. Systemd and
  launchd tests cover flag-over-YAML and environment-over-YAML service metadata.
- The generated service unit emits `KANDEV_HOME_DIR` unless YAML is the actual
  winning source, so a stronger flag or environment value cannot be shadowed
  by the pinned file.
- Dev backend and Vite readiness waits now use the same typed YAML health
  timeout. The regression captures both timeout arguments.
- GREEN: `go test ./internal/launcher -count=1` passed with 230 tests;
  `go build ./cmd/kandev` passed; changed-file `golangci-lint` passed.
