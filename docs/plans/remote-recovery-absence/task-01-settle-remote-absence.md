---
id: "01-settle-remote-absence"
title: "Settle definitive remote absence safely"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-001
acceptance_criteria:
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.1
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.2
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.3
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.5
system_design:
  - ../../specs/platform/system-design/durable-restart-reconciliation.md
---

# Task 01: Settle definitive remote absence safely

## Summary

Make synchronous startup and periodic remote attachment consume definitive
absence through the same guarded disposition. Repair the observed runtime row
before completing its pending entry, with all durable recovery state intact.

## In scope

- Read `apps/backend/AGENTS.md` and the plan's confirmed reproduction first.
- Add permanent RED tests in `remote_recovery_disposition_test.go`.
  `TestRemoteRecoveryConfirmedAbsenceSettlesGuard` must fail before the fix
  because the real retry path retains `ErrSessionRecoveryGuarded`.
- Cover both Remote Docker and Sprites. Include ordinary and deferred startup,
  plus repeated periodic retries through `pollRemoteStatuses`.
- Add the narrow disposition helper in `remote_recovery_disposition.go`.
  Retain the detailed outcome map in `attachRemoteRecovery` and consume
  startup outcomes in `Manager.Start` through the same helper.
- Accept absence only through `remoteRecoveryNoMatchProvesStopped`.
  Preserve unknown, omitted, error, legacy, and unsupported adapter outcomes.
- Pin the provider attempt's runtime row and nonzero timestamp. Re-read
  current owner and row. Use `RepairExecutorRunningDeadIfCurrent` directly.
  Storage or CAS failure retains the guard and returns retryable work.
- Fence tracked successors, execution rotation, accepted Stop, archive,
  cleanup, and shutdown. Respect pending-attempt ownership and waiters.
  Keep I/O outside `remoteRecoveryMu`.
- Remove only the matching pending entry after durable repair. A later pass
  or restart must converge after a crash or lost response around that commit.
- Use a real temporary repository for preservation and CAS tests. Include a
  mixed inventory with one absent owner and one live or unknown owner.

## Out of scope

Adapter proof expansion, database schemas, public APIs, UI changes, automatic
instruction continuation, and mutation of live developer data.
Absence settlement must not call Stop or start a runtime. It must not resolve
blocks, dispatch queued work, or publish availability or terminal events.

## Acceptance

1. Startup and a later remote-status pass finish a supported definitive-absence
   guard after the exact runtime-row repair. Another restart does not requeue
   that stopped row. Unknown and unsupported candidates remain protected.
2. Resume tokens, workspace and native identity, uncertain payloads, open
   recovery blocks, and queue claims survive. No prompt, Stop, availability,
   completion, or workflow side effect occurs.
3. Failed storage, changed owners, concurrent Stop/shutdown, and crash after
   repair preserve successors and converge on retry. Existing unknown-owner,
   live replay, and standalone deadline behavior remains correct.

## Verification

Run from the repository root. The focused RED command runs before production
edits. Run the other commands after the correction.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/runtime/lifecycle -run '^TestRemoteRecoveryConfirmedAbsenceSettlesGuard$' -count=1 -v)
(cd apps/backend && GOMEMLIMIT=2048MiB GOMAXPROCS=2 go test -p 1 -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle -count=1)
(cd apps/backend && GOMEMLIMIT=2048MiB GOMAXPROCS=2 go test -p 1 -trimpath -race -tags fts5 ./internal/task/repository/sqlite -run '^TestRepairExecutorRunningDead' -count=1)
bash scripts/lint-go-changed apps/backend/internal/agent/runtime/lifecycle/remote_recovery.go apps/backend/internal/agent/runtime/lifecycle/manager_lifecycle.go apps/backend/internal/agent/runtime/lifecycle/remote_recovery_plugin.go apps/backend/internal/agent/runtime/lifecycle/remote_recovery_disposition.go apps/backend/internal/agent/runtime/lifecycle/remote_recovery_disposition_test.go
git diff --check
```

Use the existing repository CAS contract without SQL changes. A dialect-sensitive
repository change expands scope. It requires the documented PostgreSQL
conformance checks before this task can become done.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/remote_recovery.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_lifecycle.go`
- `apps/backend/internal/agent/runtime/lifecycle/remote_recovery_disposition.go` (new)
- `apps/backend/internal/agent/runtime/lifecycle/remote_recovery_disposition_test.go` (new)

## Dependencies

None.

## Risks

The cleanup helper returns a best-effort boolean and cannot acknowledge this
durable transition. A session-only release can also affect a successor unless
the pending entry and observed row still match. Unknown evidence never expires
into proof of termination.

## Parallelism

`sequential`

## Inputs

- [Plan evidence and approach](plan.md#evidence-and-root-cause).
- [Definitive remote absence design](../../specs/platform/system-design/durable-restart-reconciliation.md#definitive-remote-absence).
- [Restart requirements](../../specs/platform/requirements/durable-restart-reconciliation.md), REQ-001.
- Existing `task08_remote_recovery_test.go`, `remote_recovery_replay_test.go`,
  `remote_recovery_pending_query_test.go`, and `manager_startup_recovery_guard_test.go`.
- `remote_recovery_stop.go` for existing no-match proof and Stop arbitration.
- `persistence.go` and `sqlite/executor.go` for the existing reader and CAS writer.
- ADR 0003 and ADR 0025 preserve runtime column ownership and resumable rows.

## Results

Done. The temporary diagnostic test failed for Remote Docker and Sprites during
planning and was removed. Permanent lifecycle coverage now exercises startup,
deferred and periodic recovery, exact CAS repair, owner rotation, shutdown and
Stop fences, and uncertain-state preservation.

- Focused Remote Docker/Sprites settlement regression: passed.
- Full lifecycle race suite: passed (`187.707s`).
- Executor-running CAS race regression: passed (`1.542s`).
- Changed Go lint: passed (`0 issues`).
- `make -C apps/backend build`: passed for all configured targets. The unsigned
  Darwin arm64 artifact reported the expected missing code-signing-tool warning.
