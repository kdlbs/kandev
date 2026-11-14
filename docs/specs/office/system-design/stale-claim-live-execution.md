---
status: draft
system: office
requirements:
  - REQ-OFFICE-STALE-CLAIM-001
---

# Office Stale-Claim Recovery for Live Executions System Design

## Purpose and boundaries

Office owns `SchedulerIntegration.recoverStaleClaimedRuns` and the runs
repository it writes. The orchestrator owns task sessions and agent runtime
liveness. Office reaches the orchestrator only through its task starter
adapter, as it already does for launches and recovery blocks.

Out of scope: a finished turn left at `WAITING_FOR_INPUT` whose run never
settles. That session is not live, so this design leaves its run to the
existing sweep.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-STALE-CLAIM-001` | [Sweep order](#sweep-order), [Liveness probe](#liveness-probe) |

## Sweep order

Each tick computes one cutoff and runs two steps:

1. If the task starter implements `TaskRunExecutionProbe`, list the stale
   task-bound claims (`ListStaleClaimedTaskRuns`). For each live one, call
   `RenewStaleClaim`, which sets `claimed_at` to the sweep time only while the
   row is still `claimed` and older than the cutoff.
2. Call the existing `RecoverStale` with the same cutoff. Renewed rows are no
   longer older than the cutoff, so only the remaining stale claims are
   requeued.

A probe error leaves the run to step 2, which keeps the earlier behavior. The
renewed run settles through the normal turn-completion path. If its execution
ends without settling the run, a later sweep finds it not live and requeues it.

## Liveness probe

`orchestrator.Service.OfficeTaskRunExecutionLive` loads the run's recorded
session, or the (task, agent) session when none is recorded. It requires the
session to belong to the run's task and agent. `STARTING` is live because a
launch is in progress. `RUNNING` is live while `probeAgentRunning` reports the
agent runtime as running. Every other state is not live.
