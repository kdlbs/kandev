---
status: active
system: tasks
created: 2026-08-19
updated: 2026-09-14
owners:
  - cfl12
---

# Task Launch Failure Recovery Requirements

## Overview

Task launch can fail because a pull request is already terminal, a base branch
is stale or missing, or a generic preparation error occurs. Users need a
durable, safe error summary and targeted recovery actions on desktop and
mobile.

## Requirements

### REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001: Task Launch Failure Recovery

**Intent:** Make handled launch failures observable and recoverable without
launching work against the wrong pull request or repository branch.

#### Acceptance criteria

- **AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.1:** When workflow auto-start
  finds relevant GitHub pull requests and every relevant pull request is
  terminal, it shall leave the task
  in its current step, avoid starting a session, and show a durable
  `pr_already_closed` reason after reload; manual launch shall bypass this
  gate.
- **AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.2:** When a handled launch error
  is projected, the task surface shall show a safe category, bounded detail,
  and only the recovery actions valid for the affected task repository.
- **AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.3:** When a live remote default
  or user-selected branch resolves, the system shall persist it to the exact
  task-repository row before relaunching.
- **AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.4:** When a recovery request
  names a foreign session or repository row, or an error stamp that is no
  longer current, the system shall reject it without mutation.
- **AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.5:** When a user selects
  `mark_review_done`, the system shall offer and accept it only for a valid
  terminal workflow step with all relevant pull requests in terminal states.
- **AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.6:** When a launch lookup fails,
  no relevant pull request exists, or a relevant pull request is open or
  unknown, the system shall preserve the normal launch path.
- **AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.7:** When recovery is used on
  desktop or mobile, the same error projection, authorization, and outcome
  shall apply without horizontal page overflow.
- **AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.8:** When recovery fails, the
  source error record shall remain visible and update with the new typed cause,
  bounded details, and valid actions; a successful recovery shall clear it only
  after the required relaunch or task move succeeds.
- **AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.9:** When the initial prompt fails
  before the agent produces an event, the system shall settle the active turn
  and session with a durable error without waiting for stall detection. The
  error shall exclude file paths and provider secrets.
- **AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.10:** When an initial-prompt error
  belongs to an old execution or prompt, the system shall not fail a replacement
  execution or successor turn.

### Bootstrap failure amendment

- **AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.11:** When asynchronous agent startup fails during initial launch or resume, the system shall persist a safe launch failure for the current execution before projecting the failed state. The same cause shall survive reload with its recovery identity.
- **AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.12:** A delayed startup failure shall not replace a newer execution's error or settled state. Recovery details shall not expose credentials, local paths, or an empty repository identifier.

These criteria are implemented in the
[contribution resume recovery package](../../../plans/contribution-resume-recovery/plan.md),
with execution fencing, transaction-serialized ownership, and recovery-surface
regressions recorded in its verification results.

### REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-002: Error ownership and retained session history

**Intent:** Keep session failures in their conversation and shared failures visible across the affected task.

This September 14 amendment is accepted product intent with implementation pending.
It changes presentation and retention, not recovery permissions or provider resume behavior.

#### Acceptance criteria

- **AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-002.1:** Each failure shall identify session or task scope. A shared resource failure shall name its affected resource when known. An initiating session shall not make a shared failure session-scoped.
- **AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-002.2:** A session failure shall create one durable chronological entry in its owning session. Repeated delivery of the same failure shall not create another entry.
- **AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-002.3:** Successful recovery, later messages, reload, and reconnect shall preserve the original session error. Recovery shall retire only matching active error state.
- **AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-002.4:** An active shared error shall appear once below the task header and above tab content. It shall remain visible on session, Plan, PR, Files, and plugin views. Task previews and tasks without sessions shall expose the same error.
- **AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-002.5:** A newer session error or session recovery shall not replace or clear an unresolved shared error. An unrelated session shall not display another session's error as its own.
- **AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-002.6:** One failure shall have one recovery control surface per task view. A shared error shall not repeat as an actionable banner inside each session. Separate failures can remain visible together.
- **AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-002.7:** Phone users shall see the shared error outside tab content and reach its details and actions from a bottom drawer. Targets shall measure at least 44 pixels. Desktop shall use a compact shared strip and details dialog.
- **AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-002.8:** Older records shall remain readable without invented failures or guessed resource scope. Stale recovery requests shall remain unable to mutate a successor error.

Implementation belongs to the [error scope package](../../../plans/error-scope-and-history/plan.md).
Session action presentation remains owned by the
[agent recovery requirement](../../agents/requirements/session-recovery-failures.md).

## Out of scope

- A background poller for all repository defaults.
- A PR gate for manual launches or providers other than GitHub.
- Changes to ACP resume semantics or bulk repository-default repair.

## System design

- [Task Launch Failure Recovery System Design](../system-design/task-launch-failure-recovery.md)
