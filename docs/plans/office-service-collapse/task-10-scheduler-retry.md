---
id: "10-scheduler-retry"
title: "Collapse the scheduler retry fork (blocked on db3adcf4)"
status: blocked
wave: 4
depends_on: ["09-scheduler-run-claim"]
blocked_on: "sibling task db3adcf4 (run-cancel writer consolidation)"
plan: "plan.md"
spec: none
parallel-safe: false
---

# Task 10: Scheduler Retry Fork

> **BLOCKED.** Do not start until sibling task `db3adcf4` — consolidating the
> run-cancel writers — has landed on `main`. This task's central drift (D4) is
> exactly the `cancelRetry` branch that `db3adcf4` is rewriting. Starting early
> guarantees a conflict in the one part of the tree the plan was told to leave
> alone.

## Scope

Four pairs between `service/retry.go` and `scheduler/retry.go`:

| Pair | Similarity | Nature |
| --- | --- | --- |
| `retryDelayWithJitter` | **identical** | pure function, no drift |
| `escalateFailure` | 0.982 | cosmetic — receiver + `ss.svc.` indirection |
| `queueCEOAgentError` | 0.960 | cosmetic — plus `service.AgentListFilter` qualification |
| `scheduleRetry` | 0.745 | **real drift (D4)** |

As in task 09, the facade is the owner: `office/scheduler` imports
`office/service`, not the reverse.

## D4 — the real drift

`service/retry.go:46` opens with a staleness guard that `scheduler/retry.go:50`
does not have at all:

```go
if stale, reason := isRetryStale(run); stale {
    return s.cancelRetry(ctx, run, reason)
}
```

and logs `zap.String("source", "backoff")`, which `scheduler` also omits.

**The facade is correct.** `office/scheduler` currently reschedules runs that the
facade would cancel as stale. Collapsing onto the facade's copy fixes this — but
that fix routes through `cancelRetry`, which is why the task is blocked.

## Required first step, after `db3adcf4` lands

1. Re-run the detector and re-diff all four pairs against the **post-`db3adcf4`**
   tree. `scheduleRetry`'s similarity and `cancelRetry`'s shape will both have
   moved; the D4 description above is written against `fb1d8fdcd` and must be
   re-verified, not assumed.
2. Establish where `db3adcf4` left ownership of the cancel write. If it
   centralized cancellation somewhere new, `cancelRetry` should call that rather
   than this task re-establishing a second writer.
3. Only then collapse. **Scope is provisional until step 1 is done.**

## Constraint

`office/repository/sqlite.Repository` embeds `*runssqlite.Repository`
(`office/repository/sqlite/base.go:60`). This task is the only one in the plan
that goes near run writes. It must not move ownership of them — the retry
scheduling write (`repo.ScheduleRetry`) and the cancel write stay exactly where
`db3adcf4` leaves them.

## Test migration

Facade `retry_test.go` cases are the surviving suite. Port any scheduler-side
retry case not already covered. Add a test pinning D4: a stale run reaching
`scheduleRetry` is cancelled with its reason, not rescheduled — asserted through
`office/scheduler`'s entry point, so it proves the fork is actually gone.

## Acceptance

1. Detector Section A drops by **1** group (`retryDelayWithJitter`); Section B
   same-name pairs drop by 3.
2. A stale run routed through `office/scheduler` is cancelled, not rescheduled.
3. `zap` field `source=backoff` is emitted on the scheduler retry path.
4. `git diff` shows no change to run-write ownership or to
   `office/repository/sqlite/base.go`.

## Verification

```bash
cd apps/backend && go test ./internal/office/scheduler/... ./internal/office/service/... -run 'Retry|Escalat|Cancel' -count=1 -v
cd apps/backend && go test ./internal/office/... ./internal/runs/... -count=1
make -C apps/backend test
make -C apps/backend lint
cd apps/backend && golangci-lint run ./... --new-from-rev=main --timeout=5m

git diff --stat main -- apps/backend/internal/office/repository/   # expect empty
```

## Files likely touched

- `internal/office/scheduler/retry.go` (delete the fork; delegate)
- `internal/office/service/retry.go`, `internal/office/service/retry_cancel.go`
- retry test files on both sides

## Dependencies

Task 09, and **`db3adcf4` merged to `main`**.

## Parallelism

`sequential`.

## Rollback position

Single revert to task 09's state, which leaves the retry fork intact but the
run-claim fork collapsed. Because this task rebases onto `db3adcf4`, record the
`db3adcf4` merge SHA in `## Results` so a later revert knows its base.

## Output contract

Summary, files changed, detector delta, the **re-verified** D4 diff against the
post-`db3adcf4` tree, the `db3adcf4` merge SHA, the D4 regression test with its
pre-fix failure output, and the empty repository-diff check.

## Results

Pending.
