---
status: draft
system: office
created: 2026-05-04
owners:
  - cfl
---
# Office Agent Runtime — Error Handling Contract Requirements

## Overview

When an agent run fails mid-turn (invalid model, auth failure, malformed response, transient upstream), the failed wakeup is terminal — except a **classified-transient** failure on the legacy (non-routing) post-start path, which is retried a bounded number of times before it counts as a terminal failure (AC-OFFICE-RUNTIME-001.9). The workflow engine can still evaluate a configured `on_agent_error` action and queue a separate coordinator run for escalation.

## Requirements

### REQ-OFFICE-RUNTIME-001: Office Agent Runtime — Error Handling Contract

**Intent:** When an agent run fails mid-turn (invalid model, auth failure, malformed response, transient upstream), the failed agent run is terminal, except the bounded classified-transient retry carved out by AC-OFFICE-RUNTIME-001.9. A workflow can use `on_agent_error` to queue a separate coordinator run for escalation.

#### Acceptance criteria

- **AC-OFFICE-RUNTIME-001.1:** No automatic retry of the failed agent run, except the bounded classified-transient retry in AC-OFFICE-RUNTIME-001.9. Every other adapter error is terminal for the wakeup that produced it.
- **AC-OFFICE-RUNTIME-001.2:** The wakeup row is stamped with `status = failed` and `error_message`. The failure path does not queue another wakeup for the same failed agent. A configured `on_agent_error` action can queue a separate coordinator run.
- **AC-OFFICE-RUNTIME-001.3:** Re-runs of the failed agent happen only via explicit user action — **Resume session** in chat, **Mark fixed** on an inbox entry, or task reassignment to a different agent — except the automatic bounded retry in AC-OFFICE-RUNTIME-001.9. A coordinator run from `on_agent_error` is escalation, not a retry.
- **AC-OFFICE-RUNTIME-001.4:** **`agent_run_failed`** - one entry per failed (task, agent) wakeup while the agent is below threshold. Title: "<agent> failed on <task>". Action: **Mark fixed** -> dismiss + retry.
- **AC-OFFICE-RUNTIME-001.5:** **`agent_paused_after_failures`** - one entry per auto-paused agent. Title: "<agent> auto-paused after <N> failures (tasks A, B, C)". Action: **Mark fixed** -> unpause + retry the affected tasks.
- **AC-OFFICE-RUNTIME-001.6:** Changing `assignee_agent_instance_id` on a task fires the existing reactivity pipeline, which queues a fresh `task_assigned` wakeup for the new agent.
- **AC-OFFICE-RUNTIME-001.7:** The existing staleness check (`recovery-reliability` spec) cancels the prior wakeup for the (task, **old** agent) since the assignee has changed.
- **AC-OFFICE-RUNTIME-001.8:** Any per-task `agent_run_failed` inbox entry tied to the old (task, agent) auto-dismisses - the failure is no longer actionable on this task.
- **AC-OFFICE-RUNTIME-001.9:** A post-start failure on the legacy (`HandleAgentFailure`) path that classifies transient (the shared routing classifier's `ClassTransient && AutoRetryable && FallbackAllowed` predicate) is retried up to 2 times, at 5s then 10s, before it counts toward `consecutive_failures` or auto-pause — observed by `consecutive_failures` staying unchanged on the first classified-transient failure. This shares its attempt counter with the pre-launch retry tier (no separate column) and is abandoned under the same 24h staleness rule, so a run that already exhausted pre-launch retries gets no additional post-start retry. The retry is additionally refused — falling through to terminal accounting instead of being scheduled — when `run.requested_at` is already older than the 2h `staleRunThreshold` the claim path uses to cancel stale runs: scheduling it anyway would only have the claim path cancel it before relaunch, silently losing the failure instead of counting it. This retry re-drives the whole run from the top and may repeat any side effects the failed attempt already committed — no effect-safety gate applies to it, matching the routing tier's own post-start requeue. See [runtime-01.md § Legacy post-start transient retry](../system-design/runtime-01.md#legacy-post-start-transient-retry).

## System design

The migrated technical source is split into [part 1](../system-design/runtime-01.md), [part 2](../system-design/runtime-02.md).
