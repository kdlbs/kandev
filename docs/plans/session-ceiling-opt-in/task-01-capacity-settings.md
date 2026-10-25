---
id: "01-capacity-settings"
title: "Persist and apply opt-in session capacity"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-SESSION-CEILING-001
  - REQ-AGENTS-SESSION-CEILING-002
acceptance_criteria:
  - AC-AGENTS-SESSION-CEILING-001.1
  - AC-AGENTS-SESSION-CEILING-001.2
  - AC-AGENTS-SESSION-CEILING-001.3
  - AC-AGENTS-SESSION-CEILING-001.4
  - AC-AGENTS-SESSION-CEILING-001.5
  - AC-AGENTS-SESSION-CEILING-001.6
  - AC-AGENTS-SESSION-CEILING-001.7
  - AC-AGENTS-SESSION-CEILING-001.8
  - AC-AGENTS-SESSION-CEILING-001.9
  - AC-AGENTS-SESSION-CEILING-002.1
  - AC-AGENTS-SESSION-CEILING-002.2
  - AC-AGENTS-SESSION-CEILING-002.3
  - AC-AGENTS-SESSION-CEILING-002.4
  - AC-AGENTS-SESSION-CEILING-002.5
system_design:
  - ../../specs/agents/system-design/session-concurrency-ceiling.md
---

# Task 01: Persist and apply opt-in session capacity

## Summary

Deliver the disabled default and a working install-level settings API. Apply
changes to the existing controller without restart or loss of deferred launches.

## In scope

- Add the typed `sessioncapacity` resolver/store/service/handlers over the
  existing install settings store. Follow the paired API, defaults, lock,
  validation, persistence failure, and partial-update contracts.
- Resolve capacity before automatic startup workers and inject it through
  `ServiceConfig`. Wire API and target to the same runtime instance.
- Remove the CPU-derived fallback and direct constructor environment parsing.
  Keep explicit environment override support in the composition resolver.
- Add the mutex-protected setter and disabled admission path. Retain current
  reservation identity and signal the existing sweeper after capacity expands.
- Update config inventory wording and affected unit/integration fixtures.

## Out of scope

- Rendered Settings UI, translations, public docs, and live-instance changes.
- New queue semantics, new storage tables, or a runtime feature flag.

## Acceptance

1. With no explicit choice, fresh and upgraded installs admit without a ceiling,
   including a failed population read, and emit no new override warning.
2. GET/PATCH, authorization, environment precedence, stored-state restoration,
   validation, concurrent partial saves, and save failure pass focused tests.
3. Live enable/decrease preserves admitted work; disable/increase retries the
   original eligible queued launch once. Race tests retain reservation ownership.

## Verification

Run commands from the repository root. Use TDD to demonstrate the default and
disabled-population failures before changing production code. Add settings API
and runtime transition tests before their implementation.

```bash
(cd apps/backend && go test ./internal/system/sessioncapacity ./internal/system/settings ./internal/system ./internal/backendapp ./internal/common/config -count=1)
(cd apps/backend && go test ./internal/orchestrator ./internal/task/service -count=1)
(cd apps/backend && go test -race ./internal/orchestrator ./internal/system/sessioncapacity -count=1)
git diff --check
```

The new package/test files are created by this work order. Run the complete
listed package tests after updating environment-dependent fixtures. Tests for
the removed CPU default must be replaced with meaningful disabled-default
assertions, not deleted without replacement.

## Files likely touched

- New `apps/backend/internal/system/sessioncapacity/{types,resolver,store,service,handler}.go` and tests.
- `apps/backend/internal/system/system.go`, `system_routes_test.go`.
- `apps/backend/internal/backendapp/{orchestrator,main}.go` and new `session_capacity_settings_test.go`.
- `apps/backend/internal/orchestrator/{service,session_ceiling,session_ceiling_config,ceiling_sweep}.go`.
- `apps/backend/internal/orchestrator/{session_ceiling_config_test,session_ceiling_wiring_test}.go` and new `session_ceiling_settings_test.go`.
- `apps/backend/internal/common/config/catalog.go` and its inventory tests.
- Scoped `AGENTS.md` only if an existing architecture statement needs correction.

## Dependencies

None. Existing storage and queue-setting patterns are available on this branch.

## Risks

Persist-before-apply must not return success while the target keeps an old value.
Do not hold the controller lock while signaling the sweeper or add a reverse
lock dependency from admission into settings storage. Keep retry eligibility
checks and reservation identity across setting changes.

## Parallelism

`sequential`

## Inputs

- Paired requirement and design, especially Settings contract, admission, and persistence.
- `apps/backend/AGENTS.md`.
- `internal/system/queuesettings` and `internal/system/settings`.
- Existing ceiling config, wiring, replay, manual override, and race tests.

## Results

Implemented the install-wide opt-in capacity resolver, persistence, API, startup
wiring, live controller setter, and disabled admission path.

- Backend package tests passed for `internal/backendapp`, `internal/system`,
  `internal/system/sessioncapacity`, `internal/orchestrator`,
  `internal/task/service`, `internal/workflow/handlers`, and
  `internal/mcp/handlers`.
- `go test -race ./internal/orchestrator ./internal/system/sessioncapacity
  -count=1` passed, including reservation ownership and live capacity changes.
- `make lint`, `make build`, and `go run ./cmd/settings-catalog --check` passed.
