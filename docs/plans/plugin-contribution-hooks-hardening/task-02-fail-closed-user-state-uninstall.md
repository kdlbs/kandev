---
id: "02-fail-closed-user-state-uninstall"
title: "Fail-closed user-state uninstall"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/plugins/spec.md"
---

# Task 02: Fail-closed user-state uninstall

## Acceptance

- A `plugin_user_state` purge error aborts uninstall before package, record, registry,
  or deliverer-success removal and returns a retryable error.
- The plugin remains stopped but installed after failure; an idempotent retry can purge
  every user's rows and complete uninstall.
- Successful uninstall and nil-store test construction retain their current behavior.

## Verification

```bash
cd apps/backend && go test ./internal/plugins -run 'TestServiceUninstall(DeletesPluginUserStateForEveryUser|FailsClosedWhenUserStateCleanupFails|RetriesAfterUserStateCleanupRecovers)$'
```

The implementation must first add the injected cleanup-failure regression and confirm
that current best-effort cleanup incorrectly returns success.

## Files likely touched

- `apps/backend/internal/plugins/service.go`
- `apps/backend/internal/plugins/service_test.go`
- `apps/backend/internal/plugins/user_state_handlers.go` only if the narrow store seam
  changes the internal accessor type

## Dependencies

None.

## Parallelism

`parallel-safe` with Tasks 01, 03, and 07; backend ownership is disjoint.

## Inputs

- Spec: uninstall API, per-user storage failure mode, persistence guarantee, and retry
  scenario.
- Plan: **Fail-closed per-user state purge**.
- ADR-2026-08-01-per-user-plugin-storage and
  ADR-2026-08-04-plugin-contribution-lifecycle-authority.
- Existing fail-closed secret-cleanup path in `Service.Uninstall`.

## Risks

Filesystem, vault, SQLite, and YAML record cleanup cannot share one transaction. Keep
the retry invariant explicit and do not claim broader atomicity.

## Output contract

Report cleanup ordering, files changed, red-test evidence, exact Go test results,
remaining partial-failure semantics, and synchronize task/plan status/results.

## Results

- Red phase: the injected `DeleteAllForPlugin` failure demonstrated that the former
  best-effort cleanup could return success after package/record deletion.
- `Service.Uninstall` now purges per-user state with the request context before
  package, record, and registry deletion. A purge error preserves the stopped,
  installed record through the existing retryable reconciliation path; nil stores
  remain a no-op for narrowly constructed tests.
- `rtk go test ./internal/plugins -run 'TestServiceUninstall(DeletesPluginUserStateForEveryUser|FailsClosedWhenUserStateCleanupFails|RetriesAfterUserStateCleanupRecovers)$'`
  — 2 tests passed.
- `rtk go test ./internal/plugins` — 281 tests passed.
- Partial-failure semantics remain intentionally non-transactional across vault,
  filesystem, SQLite, and YAML; the installed record is retained whenever the
  per-user purge itself fails so the operator can retry.
