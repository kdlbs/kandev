---
id: "01-startup-version"
title: "Startup banner version identity"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-GO-DEV-LAUNCHER-001
acceptance_criteria:
  - AC-PLATFORM-GO-DEV-LAUNCHER-001.9
system_design:
  - ../../specs/platform/system-design/go-dev-launcher.md
---

# Task 01: Startup banner version identity

## Summary

Add the native build version to the shared `dev`, `start`, and `run` startup
banner. Preserve the existing startup fields and document the new line in the
public CLI reference.

## In scope

- Carry `launcher.BuildInfo.Version` into each shared startup path.
- Render the `version:` line before the existing URL lines.
- Use `dev` for an unstamped or empty build value.
- Add focused launcher coverage and update `docs/public/cli.md`.

## Out of scope

- Changing service-unit version metadata.
- Changing `KANDEV_VERSION` release-header behavior.
- Changing backend health or system-info payloads.

## Acceptance

- Every `dev`, `start`, and `run` startup banner includes exactly one version
  line whose value matches the binary's `--version` value.
- Empty build metadata renders `dev`, and all existing URL, MCP, database, and
  log-level lines remain present and unchanged.
- The public CLI reference describes the additive version line and its source.

## Verification

```bash
cd apps/backend && go test ./internal/launcher -run 'TestLogStartupPrintsVersion|TestRun.*BuildVersion' -count=1
cd apps/backend && go test ./internal/launcher -count=1
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `apps/backend/internal/launcher/launcher.go`
- `apps/backend/internal/launcher/start.go`
- `apps/backend/internal/launcher/run.go`
- `apps/backend/internal/launcher/dev.go`
- `apps/backend/internal/launcher/network_test.go`
- `docs/public/cli.md`

## Dependencies

None.

## Risks

- Direct launcher tests construct mode configuration values, so the build
  version must be threaded without changing unrelated startup defaults.

## Parallelism

`sequential`

## Inputs

- `docs/specs/platform/requirements/go-dev-launcher.md`, AC `.9`.
- `docs/specs/platform/system-design/go-dev-launcher.md`.
- Existing `launcher.BuildInfo`, `logStartup`, and network banner tests.

## Results

Implemented the version line for `dev`, `start`, and `run`. The value is
normalized to `dev` when build metadata is empty and is shared with the
launcher `--version` path. Updated the public CLI reference.

Verification:

- `go test ./internal/launcher -run 'TestLogStartupPrintsVersion|TestRun.*BuildVersion' -count=1` passed.
- `go test ./internal/launcher -count=1` ran 259 tests; two existing service
  configuration tests were blocked by the host's `/root/.kandev/config.yaml`.
- `node --test scripts/validate-public-docs.test.mjs` passed.
- `node scripts/validate-public-docs.mjs` passed.
