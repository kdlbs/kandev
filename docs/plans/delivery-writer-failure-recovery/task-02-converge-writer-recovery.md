---
id: "02-converge-writer-recovery"
title: "Bound recovery and converge cancelled sessions"
status: done
wave: 2
depends_on:
  - "01-preserve-storage-evidence"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.4
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.5
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.6
system_design:
  - ../../specs/platform/system-design/durable-delivery-failure.md
---

# Bound recovery and converge cancelled sessions

## Summary

The health monitor starts the same reconciliation again when its cooldown
expires. The existing reconciliation cycle already bounds terminal and
transport checks. The recovery handler clears a matching settled notice but
skips the existing repair for a stale running session row.

## Scope

- In `probeDeliveryHealth`, use the existing revision and owner checks.
  Start the existing cycle once per matching failure revision. Continue
  applying pressure on valid observations. State-only peers use the cooldown.
- In the already-cancelled branch, pass the exact settled binding to
  `convergeSettledDeliverySessionState`. This reuses the idle runtime,
  terminal, and successor checks.
- Take idle-runtime evidence before trying the session guard. If the
  synchronous subscriber finds the guard busy, leave the work with the
  existing settlement or recovery path.

## Acceptance

1. Repeated polls of the same writer failure start one cycle and emit one
   warning. Changed, owner-matched evidence can trigger reevaluation. Explicit
   Retry connection, normal late terminal replay, and Stop retain their paths.
2. A real projected cancellation and a current idle owner allow only the
   stale active row to converge to `WAITING_FOR_INPUT`. The terminal outcome,
   existing effects, and independent recovery blocks remain authoritative.
3. A cancelled label without terminal proof, a busy guard, a live successor, or
   a replaced execution leaves current work intact. Unresolved submissions
   retain the existing recovery view and admission block.

## Verification

Use the existing fake-time fixtures and the real SQLite settlement fixture.
The baseline failed `TestIssue5513UnchangedReportRestartsRecovery` and
`TestIssue5513WriterRecoveryLeavesSettledCancelRunning`. Extend the existing
guards and replay tests where they already cover the assertion.

| Scenario | Target proof |
| --- | --- |
| Stable revision, later healthy or changed revision | `TestDeliveryHealthPersistentFailureRunsOneCycle` and the existing healthy and real-terminal monitor cases |
| Proven settled cancellation | `TestDeliverySessionStateConvergesFromWriterRecovery` with the real repository fixture |
| Changed owner, active successor, or busy guard | Exercise the same recovery-event entry point with the existing successor fixture and a held guard |
| Explicit retry, restart, and ordered terminal projection | Existing targeted reconciliation and session-state tests exercise the same outbox and recovery authority |

```bash
cd apps/backend
go test -trimpath -race ./internal/agent/runtime/lifecycle ./internal/orchestrator -run 'Test(DeliveryHealth|DeliveryReconcile|DeliverySessionState|DeliveryWriterFailure|AgentctlDisconnect)' -count=1
go test -trimpath ./internal/task/repository/sqlite -run 'Test(SQLiteDeliverySessionState|DeliverySessionStatePostgres)' -count=1
```

Run the existing Postgres cases with an isolated `KANDEV_TEST_POSTGRES_DSN`
when available. Record results for each lifecycle scenario. The desktop and
phone tests in work order 03 cover visible recovery and the later prompt.

## Likely files

- `apps/backend/internal/agent/runtime/lifecycle/delivery_health_monitor.go` and `delivery_health_monitor_test.go`
- `apps/backend/internal/orchestrator/agent_delivery_recovery_state.go` and `agent_delivery_recovery_state_test.go`
- `apps/backend/internal/orchestrator/agent_delivery_session_state.go` and `agent_delivery_session_state_test.go`

## Dependencies

[01 Preserve storage evidence](task-01-preserve-storage-evidence.md).

## Risks

A runtime can remain live while its writer refuses work. Retain the writer
health and historical prompt outcome as separate evidence. Use current
ownership checks at the mutation boundary because projection and cancellation
can race with the recovery event.

## Parallelism

Sequential.

## Results

RED: `TestDeliveryHealthPersistentFailureRunsOneCycle` observed eight cycles
for eight unchanged failure reports. `TestDeliverySessionStateConvergesFromWriterRecovery`
left the real settled cancellation at `RUNNING`.

GREEN: Each positive failure revision now starts one automatic cycle while
valid pressure observations continue. State-only reports retain cooldown
behavior. The recovery subscriber loads the current settled terminal, requires
an exact cancelled binding, and reuses session convergence. The synchronous
path tries the session guard once. If it is busy, one lifecycle-bounded
follow-up carries the exact terminal, waits outside the subscriber, and repeats
the existing owner and successor checks before correcting the session row.

Lifecycle evidence:

| Scenario | Acceptance | Proof | Result |
| --- | --- | --- | --- |
| Stable and changed failure reports | AC-006.4, AC-006.6 | `TestDeliveryHealthPersistentFailureRunsOneCycle`, `TestDeliveryHealthRecoveryDoesNotResend` | One cycle per revision; updated revision reevaluates; prompt identity stays unchanged. |
| Stale owner and sibling isolation | AC-005.2, AC-006.4 | `TestDeliveryHealthStaleOwnerAndSiblingIsolation` | Stale failures do not reconcile another owner or session. |
| Exact settled cancellation and no terminal proof | AC-006.5 | `TestDeliverySessionStateConvergesFromWriterRecovery`, `TestDeliveryRecoveryCancelledLabelWithoutTerminalProofKeepsSessionRunning` | Exact proof repairs the stale row; a cancelled label alone does not. |
| Busy guard, replaced runtime, and live successor | AC-006.5 | `TestDeliverySessionStateWriterRecoveryLeavesBusyGuardToSettlement`, `TestDeliverySessionStateRequiresIdleCurrentRuntime`, `TestDeliverySessionStateLeavesLiveSuccessorRunning` | The subscriber returns without waiting; one follow-up repairs the stale row after release only while exact owner and no-successor checks still pass. Replaced ownership leaves state intact. |
| Retry, Stop, and terminal replay paths | AC-006.2, AC-006.4 | Existing delivery reconciliation cases in the verification command | Existing recovery controls remain covered and pass. |

Verification passed:

```bash
cd apps/backend
go test -trimpath -race ./internal/agent/runtime/lifecycle ./internal/orchestrator -run 'Test(DeliveryHealth|DeliveryReconcile|DeliverySessionState|DeliveryWriterFailure|AgentctlDisconnect)' -count=1
go test -trimpath ./internal/task/repository/sqlite -run 'Test(SQLiteDeliverySessionState|DeliverySessionStatePostgres)' -count=1
```

`KANDEV_TEST_POSTGRES_DSN` was not configured, so PostgreSQL cases were not run.

Review regression verification also passed:

```bash
cd apps/backend
go test -trimpath ./internal/orchestrator -run 'TestDeliverySessionState(WriterRecoveryLeavesBusyGuardToSettlement|ConvergesFromWriterRecovery|LeavesLiveSuccessorRunning)$' -count=1
go test -trimpath -race ./internal/orchestrator -run 'TestDeliverySessionState(WriterRecoveryLeavesBusyGuardToSettlement|ConvergesFromWriterRecovery|LeavesLiveSuccessorRunning)$' -count=1
go test -trimpath ./internal/agent/runtime/lifecycle -run '^TestDeliveryHealthRetriesUnstartedFailureAfterCancelledStop$' -count=1
```
