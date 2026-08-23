---
spec: docs/specs/system-page/requirements/storage-maintenance.md
created: 2026-08-12
status: done
---

# Implementation Plan: Go-cache Quarantine Lifecycle Repair

## Overview

An adopted Go cache can exceed its threshold while an older generation remains in quarantine.
The current provider treats that normal retention state as a conflict and fails each maintenance run.
It also cannot close an entry when its recorded payload is already absent.

The repair first makes the retained-generation condition a typed, successful deferral. It then makes
permanent deletion idempotent for a missing Go-cache payload without changing the live cache. The
final task updates the operator guide and records focused validation results.

Confirmed root cause: `gocache.Provider.rotate` calls `ReleaseFailedQuarantineIntent` before each
rotation. That helper returns `ErrConflict` for every active `quarantined` entry. The provider
propagates the error even though retention intentionally keeps the entry active. The live incident
also has a missing `quarantine_path`, but `deleteGoCacheWithRetention` rejects that state before it
can make the durable entry terminal.

---

## Backend

### Typed cleanup deferral

- Add a typed active-intent classification in
  `apps/backend/internal/system/storage/quarantine_retry.go`. Preserve `errors.Is(err,
  storage.ErrConflict)` and the current human-readable path context for callers that still treat the
  condition as an error.
- Extend `gocache.CleanupResult` in
  `apps/backend/internal/system/storage/gocache/provider.go` with `skipped` and `reason` fields.
- In `gocache.Provider.cleanup`, convert only the typed active-intent condition into a successful
  result with `bytes_after == bytes_before`, zero reclaimed bytes, `skipped: true`, and
  `reason: "active_quarantine"`.
- Keep failed or ambiguous quarantine intents on the existing fail-closed path. Do not weaken the
  active-original unique index or allow more than one retained generation per cache path.

### Missing-payload permanent deletion

- Refactor Go-cache deletion in
  `apps/backend/internal/backendapp/storage_maintenance.go` to distinguish an absent quarantine
  payload from an unsafe or unreadable path.
- After the existing confirmation, retention, entry-state, ownership, and containment checks pass,
  transition an absent payload to `deleted`. Do not inspect, remove, rename, or replace the original
  cache path in this branch.
- Keep `restoreGoCache` on the existing ambiguous-state guard so that restore still fails closed when
  the payload is absent.
- In `workspaceQuarantineController.Purge`, measure payload presence before deletion for result
  accounting. Add zero to `deleted_bytes` when a successfully closed entry had no payload. Continue
  to count the row in `deleted`.
- Do not add a schema migration or a new quarantine state. The existing `deleted` state accurately
  records that no quarantined payload remains.

---

## Frontend

No frontend code change is required. `CleanupResult.Skipped` and `CleanupResult.Reason` extend the
`MaintenanceRun.Result` payload under `go_cache.result`; the existing run-result rendering accepts
those fields. Route shapes, UI actions, and confirmation flows remain unchanged, and the existing
**Delete**, **Clear eligible**, and **Force clear all** actions use the repaired backend paths.

---

## Public Documentation

- Update `docs/public/operations.md` in its how-to-oriented Storage maintenance section.
- Explain that one restorable Go-cache generation can defer later rotations without failing the run.
- Explain that permanent deletion can close a missing Go-cache payload without changing the live
  replacement cache. Restore remains unavailable for missing payloads.

---

## Tests

- **What:** an above-threshold replacement cache with an active retained generation produces a
  successful deferral and preserves both paths and the durable entry.
  **File:** `apps/backend/internal/system/storage/gocache/provider_test.go`.
  **How:** use the real provider with temporary cache and trash paths plus a recording quarantine
  store. First prove that the regression test fails because cleanup returns `ErrConflict`. Then make
  it pass with the exact result and filesystem assertions.
- **What:** the typed active-intent error remains compatible with `ErrConflict`, while failed-intent
  retry and ambiguous-state behavior remain unchanged.
  **Files:** `apps/backend/internal/system/storage/store_test.go` and
  `apps/backend/internal/system/storage/gocache/provider_test.go`.
  **How:** extend the existing table-driven retry tests and assert both classifications.
- **What:** eligible and forced permanent deletion close a missing Go-cache payload, preserve a
  populated original cache, and report zero deleted bytes.
  **File:** `apps/backend/internal/backendapp/storage_maintenance_test.go`.
  **How:** use the production controller with a real temporary SQLite store and real filesystem
  paths. Cover individual deletion and bulk purge.
- **What:** restore of the same missing-payload state still fails closed.
  **File:** `apps/backend/internal/backendapp/storage_maintenance_test.go`.
  **How:** retain and narrow the existing missing-payload restore regression test.
- **Integration boundary:** the backendapp tests exercise the production quarantine controller,
  real `storage.Store`, SQLite state transitions, and filesystem effects. The existing
  `MaintenanceRun.Result` envelope carries the result-field extension without route or UI action
  changes, so a handler or frontend test is not required.

---

## E2E Tests

No E2E test is required. The repair changes backend lifecycle semantics without changing the UI,
route shapes, confirmation flows, or user interaction sequence.

---

## Verification Results

- Task 01: `cd apps/backend && go test ./internal/system/storage ./internal/system/storage/gocache`
  passed (123 tests).
- Task 02: `cd apps/backend && go test ./internal/backendapp` passed (316 tests).
- Task 03: `node --test scripts/validate-public-docs.test.mjs && node scripts/validate-public-docs.mjs
  && git diff --check` passed (60 validation tests and 41 published pages).

---

## Implementation Waves And Parallel Candidates

The default execution order is sequential in the primary conversation. These waves do not
authorize subagents.

Wave 1:

- [x] [Task 01: Defer retained Go-cache rotation](task-01-defer-retained-rotation.md)

Wave 2:

- [x] [Task 02: Close missing Go-cache payloads](task-02-close-missing-payload.md)

Wave 3:

- [x] [Task 03: Update operator documentation](task-03-update-operator-docs.md)

Task 02 follows Task 01 so the primary conversation can integrate and record one lifecycle change
at a time. Task 03 documents both backend outcomes. No task is marked parallel-safe.

## Risks

- A broad conversion of `ErrConflict` into a skip can hide failed or ambiguous rename states. The
  implementation must match only the new typed active-intent classification.
- Missing-payload deletion must never use the original cache as a fallback deletion target.
- Bulk result accounting must not claim that historical `size_bytes` were removed when the payload
  was already absent.
- Restore must remain fail-closed because Kandev cannot prove that a populated original path is the
  quarantined generation.

## Out of Scope

- Multiple simultaneous restorable generations for one Go-cache path.
- A new database state or migration for externally missing payloads.
- A new repair button, API route, or automatic deletion before the configured retention deadline.
- Changes to workspace or temporary-artifact restore semantics.
