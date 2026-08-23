---
status: draft
system: tasks
created: 2026-08-19
owners:
  - kandev
---


# Workflow quorum decision recording Requirements



## Overview



Office workflows can record participant decisions and move a task through guarded quorum transitions with diagnosable outcomes.



## Why

A `move_to_step` guarded by `wait_for_quorum` never fires. An Office task
driven through Work → Review with a reviewer attached runs the reviewer, gets a
substantive review back, and then sits at Review permanently. It is
indistinguishable, from every surface a human can see, from a hung card.

Verified 2026-08-19 against task `7611f8da-4fe3-4bb2-a5a2-ebe6f90130f2`
(workspace `95542bf3`) in `~/.kandev/data/kandev.db`:

```
sqlite> SELECT COUNT(*) FROM workflow_step_decisions;
0
```

Zero rows in the entire store, ever.

## What

Five behaviors, matching the five defects.

1. An agent holding `reviewer` or `approver` on the current step can record an
   approve or reject verdict with a reason, via an MCP tool, and it persists to
   `workflow_step_decisions`.
2. A rejection recorded through any surface — agent tool or human UI — is
   counted by `any_reject`. This needs both a vocabulary fix and a seat fix: the
   human decider has no participant row at all, so a rejection counts as a veto
   without requiring a quorum seat, while approvals still require one.
3. Recording a verdict through any surface immediately re-evaluates the guarded
   transitions for that task's current step.
4. A participant with `decision_required` is counted at the step where the
   decision is being taken, not only at the step where they were attached.
5. A guarded transition that does not fire says why, distinguishably, on a
   surface a human reads.

Ownership follows the engine. The engine already owns guard semantics,
threshold arithmetic, and the required-participant slate; this feature adds no
second implementation in the orchestrator. The MCP tool and the HTTP handler are
transports that call into the engine's decision API. AC-TASKS-QUORUM-REGRESSION-001.1 names that API and
the ports that make it reachable, because "call into the engine" is not today a
thing `office/dashboard` can do. There are two such ports and they are named
separately: AC-TASKS-QUORUM-REGRESSION-001.2 for the write side (record a decision, re-evaluate) and
AC-TASKS-QUORUM-REGRESSION-001.6 for the read side (evaluate every guard read-only, for the AC-TASKS-QUORUM-DIAGNOSTICS-001.4
diagnostic surface). Behavior 5 is unbuildable without the second one.

## Requirements



### REQ-TASKS-QUORUM-CORE-001: Workflow quorum decision recording



**Intent:** Office workflows can record participant decisions and move a task through guarded quorum transitions with diagnosable outcomes.



#### Acceptance criteria



- **AC-TASKS-QUORUM-CORE-001.1:** When a qualified agent records a verdict, the system shall persist it for the task's current workflow decision round.
- **AC-TASKS-QUORUM-CORE-001.2:** When a human or agent records a verdict, the system shall evaluate the guarded transition for the task's current step.
- **AC-TASKS-QUORUM-CORE-001.3:** When a quorum guard does not advance a task, the system shall expose a reason that distinguishes waiting from an unfulfillable or failed evaluation.
- **AC-TASKS-QUORUM-CORE-001.4:** When a task is evaluated, the system shall use the workflow engine as the single owner of quorum semantics.
- **AC-TASKS-QUORUM-CORE-001.5:** When the task is re-entered for another decision round, the system shall preserve the round boundary defined by the workflow configuration.



## Out of scope



Workspace approvals, new thresholds or guard variants, decision revocation, timeouts, delegation, Kanban completion, and the separate close-approval gate remain outside this capability.