---
status: draft
system: office
created: 2026-05-04
owners:
  - cfl
---
# Office Agent Runtime — Error Handling Contract Requirements

## Overview

When an agent run fails mid-turn (invalid model, auth failure, malformed response, transient upstream), the failed wakeup is terminal. The workflow engine can still evaluate a configured `on_agent_error` action and queue a separate coordinator run for escalation.

## Requirements

### REQ-OFFICE-RUNTIME-001: Office Agent Runtime — Error Handling Contract

**Intent:** When an agent run fails mid-turn (invalid model, auth failure, malformed response, transient upstream), the failed agent run is terminal. A workflow can use `on_agent_error` to queue a separate coordinator run for escalation.

#### Acceptance criteria

- **AC-OFFICE-RUNTIME-001.1:** No automatic retry of the failed agent run. Every adapter error is terminal for the wakeup that produced it.
- **AC-OFFICE-RUNTIME-001.2:** The wakeup row is stamped with `status = failed` and `error_message`. The failure path does not queue another wakeup for the same failed agent. A configured `on_agent_error` action can queue a separate coordinator run.
- **AC-OFFICE-RUNTIME-001.3:** Re-runs of the failed agent happen only via explicit user action: **Resume session** in chat, **Mark fixed** on an inbox entry, or task reassignment to a different agent. A coordinator run from `on_agent_error` is escalation, not a retry.
- **AC-OFFICE-RUNTIME-001.4:** **`agent_run_failed`** - one entry per failed (task, agent) wakeup while the agent is below threshold. Title: "<agent> failed on <task>". Action: **Mark fixed** -> dismiss + retry.
- **AC-OFFICE-RUNTIME-001.5:** **`agent_paused_after_failures`** - one entry per auto-paused agent. Title: "<agent> auto-paused after <N> failures (tasks A, B, C)". Action: **Mark fixed** -> unpause + retry the affected tasks.
- **AC-OFFICE-RUNTIME-001.6:** Changing `assignee_agent_instance_id` on a task fires the existing reactivity pipeline, which queues a fresh `task_assigned` wakeup for the new agent.
- **AC-OFFICE-RUNTIME-001.7:** The existing staleness check (`recovery-reliability` spec) cancels the prior wakeup for the (task, **old** agent) since the assignee has changed.
- **AC-OFFICE-RUNTIME-001.8:** Any per-task `agent_run_failed` inbox entry tied to the old (task, agent) auto-dismisses - the failure is no longer actionable on this task.

## System design

The migrated technical source is split into [part 1](../system-design/runtime-01.md), [part 2](../system-design/runtime-02.md).
