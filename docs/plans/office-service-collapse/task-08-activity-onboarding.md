---
id: "08-activity-onboarding"
title: "Collapse activity logging onto shared, and the onboarding strays"
status: pending
wave: 3
depends_on: ["01-duplication-detector"]
plan: "plan.md"
spec: none
parallel-safe: false
---

# Task 08: Activity Logging, Onboarding and Dashboard Strays

Cleans up the three duplicate groups that were **not in the original brief** —
they surfaced only once the detector normalized receiver identifiers.

## Scope

### Activity → `office/shared`

`service/activity.go` (146 LOC) duplicates `shared/activity.go`:

- `LogActivityWithRun` — 0.982, differs only by receiver
  (`*Service` vs `*ActivityLoggerImpl`) and one parameter rename
  (`workspaceID` → `wsID`). Cosmetic.
- `ListActivityFiltered` — byte-identical to **`dashboard/service.go:600`**
  (a third copy).

`office/shared` has 31 non-test importers and already provides the activity
interface most of the tree consumes. Delete `service/activity.go` and have the
facade hold a `shared.ActivityLogger`.

**This has one live consumer:** `scheduler/retry.go:85` calls
`ss.svc.LogActivityWithRun`. Repoint it at `shared` the same way task 05
repointed the agent readers — a narrow interface field on `SchedulerService`, not
a concrete type.

`ListActivityFiltered`'s dashboard copy stays; `office/dashboard` owns the
activity read routes.

### Onboarding strays

`service/service.go:833` `generateSlug` and `service/service.go:848`
`writeWorkspaceConfig` are byte-identical to `onboarding/service.go:506` and
`onboarding/service.go:479`. `office/onboarding` owns onboarding
(`office/routes.go:70`). Delete the facade copies.

`service/test_helpers.go:99` `GenerateSlugForTest` exists only to expose
`generateSlug` to tests and shows as unreachable in `deadcode ./...` — delete it
with the function it wraps.

### Explicitly out of scope

`dashboard/run_detail.go:83` `taskIDFromPayload` ↔
`scheduler/dispatch_routing.go:636` `extractRunTaskID` is a genuine identical
pair, but it involves **no `office/service` copy** — it is a dashboard↔scheduler
duplication outside this plan's remit. Recorded in
[`inventory.md`](inventory.md) for a future task; do not fix it here.

## Test migration

`service/activity_test.go` (72 LOC) moves to `shared/activity_test.go`, adapting
the receiver to `*ActivityLoggerImpl`. Check whether `office/shared` already has
activity tests and merge rather than overwrite.

## Acceptance

1. Detector Section A drops by **3** groups (`ListActivityFiltered`,
   `generateSlug`, `writeWorkspaceConfig`); Section B same-name pairs drop by 1
   (`LogActivityWithRun`).
2. `internal/office/scheduler` no longer calls `svc.LogActivityWithRun`.
3. `GenerateSlugForTest` is gone from `service/test_helpers.go`.
4. Activity rows written by the scheduler retry path are unchanged — same
   `action`, `actor_type`, `actor_id`, `run_id`.

## Verification

```bash
cd apps/backend && go test ./internal/office/shared/... ./internal/office/scheduler/... -count=1 -v
cd apps/backend && go test ./internal/office/... -count=1
make -C apps/backend test
make -C apps/backend lint
cd apps/backend && golangci-lint run ./... --new-from-rev=main --timeout=5m

grep -rn 'svc\.LogActivityWithRun' apps/backend/internal/office/scheduler/   # expect no hits
```

## Files likely touched

- deleted: `internal/office/service/activity.go`, `service/activity_test.go`
- `internal/office/service/service.go` (drop `generateSlug`,
  `writeWorkspaceConfig`; add a `shared.ActivityLogger` field)
- `internal/office/service/test_helpers.go` (drop `GenerateSlugForTest`)
- `internal/office/scheduler/retry.go` (activity repoint)
- `internal/office/shared/activity_test.go`
- `internal/office/services.go`, `internal/backendapp/main.go` (wiring)

## Dependencies

Task 01. Overlaps `scheduler/retry.go` with tasks 05 and 10 — sequence after 05.

## Parallelism

`sequential`.

## Rollback position

Single revert. The activity repoint and the `activity.go` deletion must be one
commit; the onboarding-stray deletions are independently revertible.

## Output contract

Summary, files changed, detector delta, the scheduler repoint grep, and evidence
that activity rows from the retry path are byte-identical before and after.

## Results

Pending.
