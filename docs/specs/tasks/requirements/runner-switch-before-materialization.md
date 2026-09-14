---
status: draft
system: tasks
created: 2026-09-07
owners:
  - kandev
---

# Runner Switch Before Materialization Requirements

## Overview

A task's runner is chosen once, when its first session is prepared, and frozen
on that session from then on. Because the workspace default executor is normally
the worktree executor, every task starts there. Moving a task today means editing
stored task metadata by hand, deleting the session, discarding the task
environment, and forcing a new session: two of those steps destroy durable state,
and none is reachable from the product.

This capability lets a user change the runner of a task that has not yet
materialized anything, without destroying state, because nothing physical exists
to destroy. It answers two independent questions and keeps them separate:

- **Is this task's runner still mutable?** A property of the task alone.
- **Can a particular target runner actually run this task?** A property of the
  target paired with the task's repository.

Conflating them is what makes the rule drift, so the first is projected on the
task and the second is enforced when a target is named.

## Terminology

- **Runner:** the executor profile a task will use, with the executor that
  profile belongs to.
- **Materialized:** the task owns at least one durable runtime artifact: a
  session, a task environment, a running-executor record, an attached workspace
  folder, or a shared workspace group membership.
- **Mutability gate:** the ordered conditions in AC-TASKS-RUNNER-SWITCH-001.3
  deciding whether the runner can change at all. Depends only on the task.
- **Compatibility gate:** the condition in AC-TASKS-RUNNER-SWITCH-002.7 deciding
  whether a named target runner can materialize this task's repository. Depends
  on the target.
- **Runner switch:** one application of the runner action to one task.

## Requirements

### REQ-TASKS-RUNNER-SWITCH-001: Runner mutability projection

**Intent:** A client learns whether a task's runner can be changed from one
server-derived verdict, so the rule lives in one place and cannot drift into a
second implementation in the client.

**User story:** As a user looking at a task I have not started, I want to see
whether I can still change its runner, so that I do not attempt a change the
system will refuse.

#### Acceptance criteria

- **AC-TASKS-RUNNER-SWITCH-001.1:** When the system projects a task to any
  client, the projection shall carry a boolean `runner_editable` and a string
  `runner_ineligible_reason`. Both fields shall always be present, including
  when `runner_editable` is `true` and when `runner_ineligible_reason` carries
  the eligible value. Neither field is omitted on any projection path.
  `runner_ineligible_reason` shall always hold a member of the closed vocabulary;
  the empty string is never a permitted value.
- **AC-TASKS-RUNNER-SWITCH-001.1a:** When a projection path emits a task without
  having run the mutability evaluation at all, it shall emit `runner_editable` as
  `false` and `runner_ineligible_reason` as `evaluation_unavailable`. This covers
  a path that never attempted an evaluation, where
  AC-TASKS-RUNNER-SWITCH-001.8 covers one that ran and failed. Such a path shall
  never emit `true` and never emit an empty reason. This is a backstop, not a
  licence: each of the four paths named in AC-TASKS-RUNNER-SWITCH-001.5 shall
  perform the evaluation, so this criterion governs only paths outside that
  set.
- **AC-TASKS-RUNNER-SWITCH-001.2:** When no condition in
  AC-TASKS-RUNNER-SWITCH-001.3 holds for a task, the system shall project
  `runner_editable` as `true` and `runner_ineligible_reason` as `eligible`.
- **AC-TASKS-RUNNER-SWITCH-001.3:** When at least one of the following
  conditions holds for a task, the system shall project `runner_editable` as
  `false` and shall project the reason code of the **first** condition that
  holds, evaluated in exactly this order. The order is part of the contract, so
  a task that trips several conditions always reports the same code:

  1. `task_archived`: the task is archived.
  2. `no_repository`: the task has no repository attachment.
  3. `multiple_repositories`: the task has more than one repository attachment.
  4. `session_exists`: the task has at least one session, in any state,
     including terminal states.
  5. `environment_exists`: the task has at least one task environment, in any
     status, including a status that indicates creation in progress or failure.
  6. `executor_running`: the task has at least one running-executor record, in
     any status.
  7. `workspace_folder_attached`: the task has at least one attached workspace
     folder.
  8. `workspace_path_set`: the task declares a non-empty host workspace path.
  9. `workspace_group_member`: the task holds a workspace group membership that
     has not been released.
  10. `workspace_binding_not_independent`: the task's workspace binding is not
      independent. A task with no parent is independent unless it declares a
      shared-group workspace mode. A task with a parent is independent only when
      it declares the new-workspace mode explicitly; an absent, inherited, or
      shared-group mode is not independent.
- **AC-TASKS-RUNNER-SWITCH-001.4:** When the same task is evaluated repeatedly,
  every evaluation completes, and no persisted state changed between them, the
  system shall project the same `runner_editable` and `runner_ineligible_reason`
  values every time. Determinism is a property of the evaluation, not of the
  infrastructure under it: an evaluation that could not complete reports
  `evaluation_unavailable` per AC-TASKS-RUNNER-SWITCH-001.8, and a later
  evaluation of unchanged state that does complete reports the real verdict
  instead. That difference is not a violation of this criterion, because one of
  the two evaluations did not happen.
- **AC-TASKS-RUNNER-SWITCH-001.5:** When one task is read individually, within a
  list, within a board snapshot, and within a task event payload, and the
  evaluation completes on each, the system shall project the same
  `runner_editable` and `runner_ineligible_reason` values on all four paths for
  the same persisted state. As in AC-TASKS-RUNNER-SWITCH-001.4, a path whose
  evaluation did not complete reports `evaluation_unavailable` under
  AC-TASKS-RUNNER-SWITCH-001.8a and is not counted as a disagreement; the
  criterion constrains the rule the four paths apply, not the availability of the
  reads beneath any one of them.
- **AC-TASKS-RUNNER-SWITCH-001.6:** When a task is queued behind a work-in-
  progress limit and has not been admitted, the system shall evaluate its
  runner mutability exactly as it would for an admitted task; queuing alone
  shall not make the runner immutable.
- **AC-TASKS-RUNNER-SWITCH-001.7:** When a task has a parent, the system shall
  not project `runner_editable` as `false` for that reason alone; only condition
  10 of AC-TASKS-RUNNER-SWITCH-001.3 governs the parent relationship.
- **AC-TASKS-RUNNER-SWITCH-001.8:** When the system cannot complete the
  evaluation because a required read fails, it shall project `runner_editable`
  as `false` and `runner_ineligible_reason` as `evaluation_unavailable` rather
  than omitting the fields or projecting `true`.
- **AC-TASKS-RUNNER-SWITCH-001.8a:** The fail-closed substitution of
  AC-TASKS-RUNNER-SWITCH-001.8 shall apply per task, to exactly those tasks whose
  own evaluation could not be completed, however many tasks the failing read
  covered. When a projection evaluates several tasks together and one read fails,
  a task whose verdict does not depend on the failed read shall carry its real
  verdict, and only the tasks whose verdict does depend on it shall carry
  `evaluation_unavailable`. A batched read that covers every task in the
  projection and fails therefore degrades every task in it, and a read that
  covers some degrades only those; in both cases the rule is the same one. This
  does not conflict with AC-TASKS-RUNNER-SWITCH-001.5, which constrains paths
  reading the same persisted state: a transient read failure is not persisted
  state, so a task may carry `evaluation_unavailable` on one path and its real
  verdict on another at the same moment.
- **AC-TASKS-RUNNER-SWITCH-001.9:** When a client merges a task payload that
  does not carry `runner_editable`, the client shall treat the runner as not
  editable and shall not retain a previously cached `true`. Unlike the derived
  executor identity fields, which are gap-filled from cache, this verdict fails
  closed: a stale `true` offers an action the server will refuse, while a stale
  `false` only hides one a refresh restores.
- **AC-TASKS-RUNNER-SWITCH-001.9a:** When a payload's `runner_editable` is
  `false` but its `runner_ineligible_reason` is absent, empty, or outside the
  closed vocabulary, the client shall treat the reason as
  `evaluation_unavailable`. It shall not display an unrecognized code and shall
  not fall back to a cached reason.

- **AC-TASKS-RUNNER-SWITCH-001.10:** When a task is read while a runner switch
  for that task is in flight, the projection shall reflect either the state
  before the switch or the state after it, and the projected executor profile
  and the projected verdict in one payload shall describe the same state.
- **AC-TASKS-RUNNER-SWITCH-001.11:** When a task has never stored an explicit
  executor profile and would resolve its runner from the workspace default, the
  system shall evaluate its runner mutability normally and shall be able to
  project it as editable.
- **AC-TASKS-RUNNER-SWITCH-001.12:** The evaluation shall not read the task's
  workflow state, workflow step, or priority. A task in any state, including a
  terminal one, that has materialized nothing shall be projected as editable.

REQ-TASKS-RUNNER-SWITCH-002 (the runner switch action) lives in
[Runner switch before materialization action](runner-switch-before-materialization-action.md).
REQ-TASKS-RUNNER-SWITCH-003 (effect of a switch on the next launch) and
REQ-TASKS-RUNNER-SWITCH-004 (the runner editing surface) live in
[Runner switch before materialization effects](runner-switch-before-materialization-effects.md).
They were split out to keep each file within the requirement file-size limit;
the contract is unchanged and every acceptance criterion keeps its identifier.

## Out of scope

- Family tasks, excluded by condition 10 of AC-TASKS-RUNNER-SWITCH-001.3.
  Moving one requires deciding what happens to its siblings, a different
  contract.
- Multi-repository tasks, excluded by condition 3. A multi-repository move must
  decide per-repository materialization, which this capability does not model.
- Any change to the session-ensure behavior that prepares a session when a task
  with no session is opened. The typed conflict in
  AC-TASKS-RUNNER-SWITCH-002.13 is deliberately the backstop for that race,
  rather than a new gate on session preparation.
- Pre-filtering the runner picker by the compatibility gate. An incompatible
  target is rejected on save with `target_cannot_materialize_repository` rather
  than hidden. Hiding it needs a per-target verdict for every offered profile, a
  second projection contract; this capability defines only the per-task one.
- Narrowing the generic task-metadata replacement surfaces. They can still
  overwrite the stored executor profile wholesale, because they replace the whole
  metadata document. Constraining them would change the contract for every
  metadata key and belongs to whichever change takes that on.
- Changing the workspace default executor, or how a task's initial runner is
  chosen at creation; owned by
  [Task Create Executor Default](task-create-executor-default.md).
- Moving a runner after materialization, and any migration of an existing
  session, environment, or worktree between runners.
- Pinning the resolved default as an explicit stored choice from the edit dialog.
  A task that stores no executor profile is shown the runner it would resolve to,
  and selecting that same value is treated as no change
  (AC-TASKS-RUNNER-SWITCH-004.5b), because the dialog cannot tell a deliberate pin
  from a change reverted. Offering it would need a control that expresses "pin this
  default" as distinct from "select this profile", which is a surface decision this
  capability does not make. Anything taking it on needs to know that the switch
  action itself already supports the write — AC-TASKS-RUNNER-SWITCH-002.16 makes
  storing the default an ordinary change — so the missing piece is the control and
  its copy, not the contract underneath it.
- Bulk runner switching across several tasks.
