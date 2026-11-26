---
id: "01-confirm-contention-health-failures"
title: "Confirm contention-only health failures"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007
acceptance_criteria:
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.3
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.4
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.7
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.9
system_design:
  - ../../specs/platform/system-design/postgres-domain-store-parity.md
---

# Task 01: Confirm contention-only health failures

## Summary

A single periodic SQLite probe that fails only by deadline no longer marks healthy persistence unhealthy.
One confirmation probe three seconds later decides the recorded state.

## In scope

- Split `Health.check` in `internal/persistence/requiredstores/health.go` into probe and record steps.
- Confirmation logic in the periodic loop, with one warning for an unconfirmed deadline.
- Unit tests with deterministic probe outcomes and an injectable confirmation delay.

## Out of scope

Startup `Check`, PostgreSQL probes, probe interval and timeout, maintenance deferral, diagnostics API shape,
and writer-holder attribution.

## Acceptance

1. RED: a healthy tracker whose periodic probe fails once with `deadline_exceeded` and then succeeds remains healthy.
   On current code the tracker becomes unhealthy. When the confirmation also fails, the tracker becomes unhealthy
   with the confirmation's diagnostic.
2. Missing tables, driver errors, mixed failure classes, an already unhealthy tracker, and startup `Check`
   record on the first probe exactly as before. A deferred confirmation records nothing.
3. Loop cancellation during the confirmation wait returns promptly without recording. The confirmation never
   triggers a second confirmation, and the ticker cadence is unchanged.

## Verification

Run from the repository root.

```bash
(cd apps/backend && go test -trimpath -race ./internal/persistence/requiredstores/... -count=1)
(cd apps/backend && go test -trimpath ./internal/backendapp/... -run 'Persistence|Health' -count=1)
bash scripts/lint-go-changed apps/backend/internal/persistence/requiredstores/health.go
git diff --check
```

## Files likely touched

- `apps/backend/internal/persistence/requiredstores/health.go`.
- A new `apps/backend/internal/persistence/requiredstores/health_confirmation_test.go`.

Keep the confirmation delay a package constant with a test-only override, like `SetInterval`.

## Dependencies

None.

## Risks

- Recording inside `check` today also logs transitions; keep exactly one transition log per recorded result.
- Holding the maintenance lease across the three-second wait would block maintenance; release it between probes.

## Parallelism

`sequential`

## Inputs

- [SQLite contention confirmation](../../specs/platform/system-design/postgres-domain-store-parity.md#sqlite-contention-confirmation).
- [ADR-2026-11-25-confirm-contention-health-failures](../../decisions/2026-11-25-confirm-contention-health-failures.md).
- `apps/backend/AGENTS.md` conventions and TDD guidance.

## Results

- `go test -trimpath -race ./internal/persistence/requiredstores/... -count=1` passed.
- `go test -trimpath ./internal/backendapp/... -run 'Persistence|Health' -count=1` passed.
- `bash scripts/lint-go-changed apps/backend/internal/persistence/requiredstores/health.go apps/backend/internal/persistence/requiredstores/health_test.go apps/backend/internal/persistence/requiredstores/health_confirmation_test.go` reported 0 issues.
- `make build-kandev` passed with the updated backend.
- `git diff --check` passed.
