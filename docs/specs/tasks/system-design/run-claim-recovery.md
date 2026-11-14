---
status: draft
system: tasks
requirements:
  - REQ-TASKS-RUN-CLAIM-RECOVERY-001
created: 2026-11-13
updated: 2026-11-14
owners:
  - kandev
---

# Run claim recovery design

## Boundary

Keep the repair in the existing recovery path. The task system owns claim
storage. Office and the runtime already own session and execution information.
Do not introduce a generic inspector model or another maintenance loop.

## Current failure

`SchedulerIntegration.recoverStaleClaimedRuns` calls `RecoverStale` with a
30-minute cutoff. The repository changes every older claimed row to queued.
It does not check whether the session still runs.

The next tick attempts another launch. Routing can interpret its rejection as
provider failure and park the row. `resolveLifecycleRun` and `FinishRun` only
accept claimed rows, so the original completion cannot finish the parked row.

## Small repair

Replace the unconditional bulk recovery call with this local sequence:

1. Read the aged claimed runs.
2. Check the associated session through existing session/runtime information.
3. Preserve active or uncertain claims.
4. Conditionally requeue abandoned claims that never launched.

A small callback or local interface in Office service options is sufficient
for the runtime check. Reuse the existing backend dependency wiring.
Do not add evidence types to `runs/models` or enumerate unrelated workspace executions.

For a task-bound run, use its recorded session. When that ID is missing,
use the existing task/agent session lookup. An unresolved launched session
must not become evidence that no execution exists.
The existing runtime lookup and error-preserving liveness probe provide the check.
Protect startup and a ready execution whose completion is still being processed.
An unavailable lookup defers recovery for that row.

For a taskless run, use `ListRunSessions` and its existing attempt ownership.
A `preparing` or `running` attempt protects the run from the generic sweep.
Leave abandoned taskless-attempt settlement to its existing lifecycle/restart path.

A preserved claim keeps its original `claimed_at` value. Its normal completion
continues to use `FinishRun`; that method needs no broader status guard.
No second launch means this defect produces no duplicate-launch provider error.
General launch-conflict classification remains separate work.

## Storage and race guard

Keep candidate reads and a small per-run conditional update in the existing
runs repository. Retain the current age cutoff and recovery-priority rule.
Match the run ID, claimed status, original claim instant, and session in the write.
Include the observed route sequence where routing can change ownership.
A changed row is a no-op. Do not hold a SQL transaction during a runtime probe.

The single scheduler already serializes its own claim and launch passes.
Normal executor admission remains the final check against a competing launch.
A launched run without active execution stays with lifecycle/restart recovery;
the stale-claim sweep does not become a general execution-replay mechanism.

## Existing stranded rows

Do not build an automatic repair scanner or infer ownership from error text.
The narrow repair prevents new rows from entering this failure sequence.
Previously parked rows require targeted cleanup once their agents stop.
Cleanup is outside implementation scope and does not authorize a live database write.

## Requirement mapping and tests

| Criteria | Evidence |
| --- | --- |
| 001.1 and 001.4 | Age a live run by 35 minutes. Tick twice, then complete it. Assert one launch and normal finish. |
| 001.2 | Failed or uncertain session check leaves the claim unchanged. |
| 001.3 | An abandoned unlaunched claim still returns to queued. |
| 001.6 | Completion, cancellation, or a newer claim wins over the stale observation. |
| 001.7 | Taskless preparing/running attempt guard and dialect-compatible conditional writes. |

Criterion prefixes are `AC-TASKS-RUN-CLAIM-RECOVERY-`.
Use the existing backend service harness and a controlled runtime.
Add a production-wiring assertion so tests cannot pass with an unwired guard.
No rendered UI changes or browser test is required.

## Related documents

- [Requirements](../requirements/run-claim-recovery.md).
- [Plan](../../../plans/office-stale-claim-recovery/plan.md).
- [Queue ownership](../../../decisions/2026-08-01-global-run-scheduler-ownership.md).
- [Run contracts](../../../decisions/2026-09-26-run-contract-ownership.md).
