---
id: "01-defer-retained-rotation"
title: "Defer retained Go-cache rotation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/system-page/requirements/storage-maintenance.md"
---

# Task 01: Defer Retained Go-cache Rotation

## Intent

Make a retained Go-cache generation a successful cleanup deferral instead of a maintenance error.
Keep all failed and ambiguous intent states fail-closed.

## Acceptance

- An active `quarantined` entry for the same Go-cache path returns a typed active-intent
  classification that remains compatible with `storage.ErrConflict`.
- Cleanup returns `skipped: true`, `reason: "active_quarantine"`, unchanged before/after bytes, and
  zero reclaimed bytes without changing either cache path or the active entry.
- Failed-intent retry and ambiguous filesystem behavior remain unchanged.

## Files Likely Touched

- `apps/backend/internal/system/storage/quarantine_retry.go`
- `apps/backend/internal/system/storage/store_test.go`
- `apps/backend/internal/system/storage/gocache/provider.go`
- `apps/backend/internal/system/storage/gocache/provider_test.go`
- `docs/plans/go-cache-quarantine-lifecycle/plan.md`
- `docs/plans/go-cache-quarantine-lifecycle/task-01-defer-retained-rotation.md`

## Dependencies

None.

## Parallelism

`sequential`.

## Inputs

- Spec sections: **Go build cache**, **Failure modes**, **Persistence guarantees**, and **Scenarios**.
- Plan section: **Typed cleanup deferral**.
- Existing patterns: `ReleaseFailedQuarantineIntent` and temporary-artifact `CleanupResult` skip
  fields.

## TDD Sequence

1. Add the above-threshold retained-generation regression test.
2. Run `go test ./internal/system/storage/gocache -run
   TestCleanupDefersWhenGoCacheQuarantineIsActive` and record the expected `ErrConflict` failure.
3. Add the typed classification and minimal cleanup-result conversion.
4. Run the targeted packages and record the passing result.

## Verification

```bash
cd apps/backend && go test ./internal/system/storage ./internal/system/storage/gocache
```

## Output Contract

Report the files changed, the red and green test commands, the exact outcomes, remaining risks, and
the synchronized task and plan status.

## Results

- Red: `cd apps/backend && go test ./internal/system/storage/gocache -run
  TestCleanupDefersWhenGoCacheQuarantineIsActive` failed because cleanup returned the existing
  `storage.ErrConflict` for the active entry.
- Green: `cd apps/backend && go test ./internal/system/storage ./internal/system/storage/gocache`
  passed (123 tests).
- Added `storage.ActiveQuarantineIntentError`, which preserves `errors.Is(err, storage.ErrConflict)`
  and the existing path context. Go-cache cleanup now reports a successful `active_quarantine`
  skip with unchanged byte counts and no new intent. Failed and ambiguous intent states remain
  errors.
