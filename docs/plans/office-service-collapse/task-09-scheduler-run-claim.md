---
id: "09-scheduler-run-claim"
title: "Collapse the scheduler run-claim and guard fork"
status: pending
wave: 4
depends_on: ["05-agents-domain"]
plan: "plan.md"
spec: none
parallel-safe: false
---

# Task 09: Scheduler Run-Claim / Guard Fork

The direction reverses here. For the six CRUD domains the sub-package is the
owner; for the scheduler **the facade is the owner**, because
`office/scheduler` already imports `office/service`:

- `scheduler/run.go:154` — `svc *service.Service`
- `scheduler/run.go:54` — `type LaunchContext = service.LaunchContext`
- `scheduler/executor_resolver.go:11` — `type ExecutorConfig = service.ExecutorConfig`
- `scheduler/retry.go:110` — `service.AgentListFilter`

Making the facade delegate to `scheduler` would invert an existing edge and
create an import cycle. So `scheduler`'s copies collapse **onto** the facade's.

## Scope

Delete the duplicated run-claim and guard logic from `office/scheduler`, leaving
`office/service` as the single implementation:

| Pair | Similarity | Facade home | Scheduler copy |
| --- | --- | --- | --- |
| `ClaimNextRun` | **identical** | `service/scheduler_runs.go:18` | `scheduler/run_processing.go:15` |
| `ProcessRunGuard` | 0.979 | `service/scheduler_runs.go:100` | `scheduler/run_processing.go:42` |
| `guardAgentStatus` | 0.978 | `service/run.go:170` | `scheduler/run.go:315` |

All three differ only by receiver (`s` vs `ss`) and one hop of collaborator
indirection (`s.GetAgentFromConfig` vs `ss.svc.GetAgentFromConfig`) — no
behavioral drift. Re-confirm with the detector before deleting.

**Note the interaction with task 05:** that task repoints
`ss.svc.GetAgentFromConfig` onto `agents.AgentService`. Whichever lands second
must reconcile — if 05 has landed, the scheduler copies already call `agents`,
and collapsing them onto the facade's copies means the facade's `ProcessRunGuard`
/ `guardAgentStatus` should also take an agent reader rather than calling its own
`GetAgentFromConfig` (which task 05 deleted). Sequence 05 first, as declared.

## Explicitly out of scope

`retryDelayWithJitter` (identical), `escalateFailure` (0.982),
`queueCEOAgentError` (0.960) and `scheduleRetry` (0.745) are **task 10** and are
blocked on `db3adcf4`. Do not touch `service/retry.go`,
`service/retry_cancel.go`, or `scheduler/retry.go`'s retry logic here.

## Test migration

`office/scheduler` has 3 test files / 848 LOC; the facade has run and scheduler
tests. Since the facade is the owner here, **scheduler-side tests for these three
functions move to `office/service`**, not the other way round. Diff first: any
`ClaimNextRun` / `ProcessRunGuard` / `guardAgentStatus` case present only in
`scheduler`'s suite must be ported into the facade's, and the task records which.

Leave every scheduler test covering tick-loop, dispatch, and routing where it is
— those are `office/scheduler`'s own behavior, not fork.

## Acceptance

1. Detector Section A drops by **1** group (`ClaimNextRun`); Section B same-name
   pairs drop by 2.
2. `office/scheduler` has one implementation of run claiming and agent-status
   guarding, delegating to `office/service`.
3. Run claiming is unchanged under concurrency — the existing claim/lease tests
   pass unmodified.
4. No change to `office/repository/sqlite` or any run-write path.

## Verification

```bash
cd apps/backend && go test ./internal/office/scheduler/... ./internal/office/service/... -count=1 -v
cd apps/backend && go test ./internal/office/scheduler/... -run 'Claim|Guard' -count=20   # concurrency shake-out
cd apps/backend && go test ./internal/office/... -count=1
make -C apps/backend test
make -C apps/backend lint
cd apps/backend && golangci-lint run ./... --new-from-rev=main --timeout=5m
```

## Files likely touched

- `internal/office/scheduler/run_processing.go` (delete `ClaimNextRun`,
  `ProcessRunGuard`; delegate)
- `internal/office/scheduler/run.go` (delete `guardAgentStatus`; delegate)
- `internal/office/service/scheduler_runs.go`, `internal/office/service/run.go`
  (accept an agent reader if task 05 has landed)
- scheduler test files for the three functions

## Dependencies

Task 05 (the agent-reader repoint). Task 01 for the count check.

## Parallelism

`sequential`.

## Rollback position

Single revert. This task deliberately does not touch the cancel path, so it is
independent of `db3adcf4` and can be reverted without coordinating with it.

## Output contract

Summary, files changed, detector delta, the ported-test list, the `-count=20`
concurrency result, and confirmation that `service/retry*.go` and
`scheduler/retry.go` were not modified.

## Results

Pending.
