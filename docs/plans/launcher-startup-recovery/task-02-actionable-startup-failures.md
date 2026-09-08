---
id: "02-actionable-startup-failures"
title: "Present actionable startup failures"
status: done
wave: 2
depends_on:
  - "01-bind-aware-readiness"
plan: "plan.md"
requirements:
  - REQ-LAUNCHER-STARTUP-002
acceptance_criteria:
  - AC-LAUNCHER-STARTUP-002.1
  - AC-LAUNCHER-STARTUP-002.2
  - AC-LAUNCHER-STARTUP-002.3
  - AC-LAUNCHER-STARTUP-002.4
  - AC-LAUNCHER-STARTUP-002.5
  - AC-LAUNCHER-STARTUP-002.6
system_design:
  - ../../specs/launcher/system-design/startup-recovery.md
---

# Task 02: Present Actionable Startup Failures

## Summary

Return typed readiness evidence and print one concise failure summary. State
whether the backend exited or the launcher stopped a live process.

## In scope

- Add typed health observations and failure classes.
- Retain the last safe result for each health target.
- Print effective binds, configuration provenance, log path, and the relevant
  next action.
- Keep one bounded backend-output dump and one supervisor shutdown.

## Out of scope

- Parsing backend or ACP log messages.
- Changing backend log retention or diagnostic bundles.
- Adding metrics or persisted failure records.

## Acceptance

- Output distinguishes early exit, connection timeout, non-success HTTP, and
  a foreign process.
- A live backend stopped after timeout is not described as crashed.
- Failure output contains no health token or sensitive configuration value.

## Verification

```bash
(cd apps/backend && go test ./internal/launcher -run 'Test(WaitForHealth|StartupFailure|RunManagedApp|RunDev)' -count=1 && go test ./internal/launcher -count=1)
```

## Files likely touched

- `apps/backend/internal/launcher/health.go`
- `apps/backend/internal/launcher/health_test.go`
- `apps/backend/internal/launcher/start.go`
- `apps/backend/internal/launcher/start_test.go`
- `apps/backend/internal/launcher/dev.go`
- `apps/backend/internal/launcher/dev_test.go`
- `apps/backend/internal/launcher/bootstrap.go`

## Dependencies

Task 01 supplies the ordered health targets and access URL.

## Risks

- Error details must stay bounded and must not expose tokens.
- Output must remain useful in terminal, service, and desktop capture paths.

## Parallelism

`sequential`

## Inputs

- `REQ-LAUNCHER-STARTUP-002` and the failure-presentation design.
- Existing process-output capture and supervisor shutdown behavior.

## Results

Added typed per-target health observations and readiness failure classes for
early exits, unreachable backends, unhealthy HTTP responses, and different
processes. Startup output now includes effective binds, target outcomes,
configuration provenance, backend log path, process state, a class-specific
next step, and the public troubleshooting guide. Health tokens, response
bodies, and sensitive configuration values are not included. Backend output is
still bounded and dumped once before the single supervisor shutdown.

Verification passed:

- `go test ./internal/launcher -run 'Test(WaitForHealth|StartupFailure|RunManagedApp|RunDev)' -count=1`
- `go test ./internal/launcher -count=1`
