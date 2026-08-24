---
id: "02-close-missing-payload"
title: "Close missing Go-cache payloads"
status: done
wave: 2
depends_on: ["01-defer-retained-rotation"]
plan: "plan.md"
spec: "../../specs/system-page/requirements/storage-maintenance.md"
---

# Task 02: Close Missing Go-cache Payloads

## Intent

Make permanent deletion idempotent when a Go-cache quarantine payload is already absent. Preserve
the live original path and keep restore fail-closed.

## Acceptance

- Eligible and forced permanent deletion transition a valid missing-payload entry to `deleted`.
- The deletion path does not inspect recursively, remove, rename, or replace a populated original
  cache, and bulk purge reports zero deleted bytes for the absent payload.
- Restore of the same state returns `storage.ErrConflict` and leaves the entry active.

## Files Likely Touched

- `apps/backend/internal/backendapp/storage_maintenance.go`
- `apps/backend/internal/backendapp/storage_maintenance_test.go`
- `docs/plans/go-cache-quarantine-lifecycle/plan.md`
- `docs/plans/go-cache-quarantine-lifecycle/task-02-close-missing-payload.md`

## Dependencies

- Task 01: retained rotation behavior and result classification.

## Parallelism

`sequential`.

## Inputs

- Spec sections: **Go build cache**, **Failure modes**, and **Scenarios**.
- Plan section: **Missing-payload permanent deletion**.
- Existing tests:
  `TestQuarantineControllerDoesNotTreatPopulatedReplacementAsRestoredPayload` and
  `TestQuarantineControllerDoesNotMarkMissingPayloadPlaceholderRestored`.

## TDD Sequence

1. Split the existing combined restore/delete assertion and add eligible, forced, and bulk
   missing-payload deletion cases.
2. Run `go test ./internal/backendapp -run 'TestQuarantineController.*MissingGoCachePayload'` and
   record the expected conflict failures.
3. Refactor the delete path without changing restore behavior.
4. Run the backendapp package and record the passing result.

## Verification

```bash
cd apps/backend && go test ./internal/backendapp
```

## Output Contract

Report the files changed, the red and green test commands, the exact outcomes, filesystem and
database assertions, remaining risks, and the synchronized task and plan status.

## Results

- Red: `cd apps/backend && go test ./internal/backendapp -run
  'TestQuarantineController.*MissingGoCachePayload'` failed for the eligible, forced, and bulk
  missing-payload cases because deletion returned `storage.ErrConflict`.
- Green: the same focused command passed (4 tests), and `cd apps/backend && go test
  ./internal/backendapp` passed (316 tests).
- Permanent deletion now closes a validated missing Go-cache payload without reading or changing the
  original cache path. Restore continues through the existing fail-closed missing-payload guard.
  Purge uses the deletion-time payload outcome and reports zero deleted bytes when the durable entry
  was already missing on disk, including a payload disappearing before the deletion check.
