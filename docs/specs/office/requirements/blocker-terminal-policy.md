---
status: active
system: office
created: 2026-11-11
owners:
  - kandev
---

# Office Blocker Terminal Policy Requirements

## Overview

An Office task can be blocked by other tasks. Office uses blocker state for two
decisions: whether an assignment wake can start the dependent, and when the
dependent receives the `task_blockers_resolved` wake. Before this capability, those
decisions disagreed. Admission and the remaining-blocker check treated `COMPLETED`,
`FAILED` and `CANCELLED` all as resolved. The resolution wake fired only when a
blocker completed, or when it entered a step named Done or Cancelled. So whether a
dependent woke depended on which blocker finished last and on step names
([issue #4548](https://github.com/kdlbs/kandev/issues/4548)).

This capability defines one policy. Only a successfully completed blocker satisfies
an Office dependent, which matches the Kanban dependency policy
([task dependencies](../../tasks/system-design/task-dependencies.md)). A blocker that
fails or is cancelled keeps its dependents held. A held chain must not stall
silently, so Office wakes one responsible agent once for each such blocker
transition. It must not add a second alert for a failure that already alerts the
same agent.

[Assignment wake admission](scheduler.md) (`REQ-OFFICE-SCHEDULER-002`) owns the
assignment producers and their held or admitted outcomes. This document owns the
classification those producers apply.

## Terminology

- **Blocker:** a task named by a persisted dependency edge of an Office dependent.
  Valid same-workspace Kanban tasks can be blockers.
- **Satisfied blocker:** a blocker whose persisted task state is `COMPLETED`.
- **Failed blocker:** a blocker whose persisted task state is `FAILED` or
  `CANCELLED`.
- **Pending blocker:** a readable blocker that is neither satisfied nor failed.
- **Held dependent:** an unarchived, non-terminal Office dependent in the blocker's
  workspace that has at least one failed blocker.
- **Failure transition:** a committed task state change from a state other than
  `FAILED` or `CANCELLED` into `FAILED` or `CANCELLED`.
- **Responsible agent:** the runner of the failed blocker's parent task. When the
  blocker has no parent, or the parent has no runner, it is the workspace's CEO
  agent.

## Requirements

### REQ-OFFICE-BLOCKER-POLICY-001: Successful completion alone satisfies a blocker

**Intent:** A dependent proceeds only after all of its prerequisites have succeeded.
This result must not depend on the order in which blockers finish or on the path by
which they reach a terminal state.

**User story:** As an operator running an Office agent chain, I want a dependent to
start only after every prerequisite succeeds, so that an agent never builds on work
that failed or was abandoned.

#### Acceptance criteria

- **AC-OFFICE-BLOCKER-POLICY-001.1:** When any blocker of an Office dependent is a
  failed blocker, assignment admission shall admit zero `task_assigned` runs for
  that dependent. The same applies across all producers that
  `REQ-OFFICE-SCHEDULER-002` governs.
- **AC-OFFICE-BLOCKER-POLICY-001.2:** When a blocker completes while another blocker
  of the same dependent is a failed blocker, the system shall queue no
  `task_blockers_resolved` run for that dependent. The order in which the blockers
  reached their terminal states shall not change this outcome.
- **AC-OFFICE-BLOCKER-POLICY-001.3:** When the last unsatisfied blocker of a
  dependent completes and every other blocker is a satisfied blocker, the system
  shall queue one `task_blockers_resolved` run for that dependent. This also applies
  when an earlier failed blocker later reaches `COMPLETED`, for example after a
  retry.
- **AC-OFFICE-BLOCKER-POLICY-001.4:** When a blocker enters a workflow step named
  Done or Cancelled, the system shall evaluate resolution against persisted blocker
  states. It shall wake a dependent only when every blocker is a satisfied blocker.
  A step name alone shall not satisfy a blocker.
- **AC-OFFICE-BLOCKER-POLICY-001.5:** Archiving a blocker shall not change its
  classification. A blocker row that is missing or unreadable shall follow the
  unknown-read rule of `AC-OFFICE-SCHEDULER-002.8`, which preserves admission
  unless another blocker is positively known to be unsatisfied.
- **AC-OFFICE-BLOCKER-POLICY-001.6:** Kanban dependency status, Kanban escalation,
  and Office `task_children_completed` parent wakes shall keep their existing
  behavior.

### REQ-OFFICE-BLOCKER-POLICY-002: Escalate a blocker failure once

**Intent:** A failed blocker holds its dependents until someone retries the blocker,
replaces it, or removes the edge. One responsible agent must learn about this
exactly once, so the chain neither deadlocks silently nor raises duplicate alerts.

**User story:** As a coordinator agent, I want one wake that tells me a blocker
failed and which dependents it now holds, so that I can repair the chain without
polling or receiving duplicate alerts.

#### Acceptance criteria

- **AC-OFFICE-BLOCKER-POLICY-002.1:** When a blocker makes a failure transition and
  has at least one held dependent, the system shall queue one `task_blocker_failed`
  run for the responsible agent. The run payload shall name the failed blocker, its
  new state, and every held dependent.
- **AC-OFFICE-BLOCKER-POLICY-002.2:** When a blocker makes a failure transition and
  has no held dependent, the system shall queue no `task_blocker_failed` run.
- **AC-OFFICE-BLOCKER-POLICY-002.3:** A blocker reaching `FAILED` or `CANCELLED` by
  any committed path shall cause the same evaluation. This includes task service and
  dashboard state writes, Office status changes from the UI or an agent, agent
  runtime and launch-failure writes, and workflow step moves that change the task
  state. Tree cancel is excluded: it is a restorable operator hold, and dependents
  outside the cancelled tree stay held until it is restored or the edge is removed.
- **AC-OFFICE-BLOCKER-POLICY-002.4:** When the same failure transition is delivered
  more than once, the system shall persist at most one `task_blocker_failed` run for
  it. When the blocker later leaves the failed state and makes another failure
  transition, the system shall treat that transition as a new occurrence.
- **AC-OFFICE-BLOCKER-POLICY-002.5:** A change between `FAILED` and `CANCELLED` is
  not a failure transition and shall queue no `task_blocker_failed` run.
- **AC-OFFICE-BLOCKER-POLICY-002.6:** When the responsible agent already has a
  queued or claimed run, including an `agent_error` escalation for the same
  failure, or a run that itself cancelled the blocker, the escalation shall follow
  the existing claim-time merge (`AC-OFFICE-SCHEDULER-001.8`). It shall not start a
  separate run.
- **AC-OFFICE-BLOCKER-POLICY-002.7:** When no responsible agent exists, the system
  shall queue no run and shall log the unescalated failure at Warn with the blocker
  and dependent IDs. The Inbox Failed bucket shall keep listing a `FAILED` blocker
  as before.
- **AC-OFFICE-BLOCKER-POLICY-002.8:** The `task_blocker_failed` run shall follow
  existing queue policy: pause and replay, rate limits, self-trigger allowances, and
  causation. Its prompt shall state that the dependents remain held until the
  blocker is retried, replaced, or removed.

## Out of scope

- Changing Kanban dependency policy or Kanban blocked-reason presentation.
- Automatically retrying, cancelling, or rewiring a failed blocker or its
  dependents.
- New Inbox item types, UI, or user-facing copy.
- Changing how Office classifies children for `task_children_completed`.
- A periodic sweep for failure transitions whose publication failed. Publication of
  every committed state change is the delivery contract this capability relies on.
