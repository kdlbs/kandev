---
status: active
system: tasks
created: 2026-08-19
updated: 2026-10-12
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

This September 14 amendment is implemented in the [error scope package](../../../plans/error-scope-and-history/plan.md).
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

### REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-003: Retained Quick Chat setup failures

Draft amendment, 2026-10-02. Delivery: [Setup recovery UX](../../../plans/setup-recovery-ux/plan.md).

- **AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-003.1:** Once a Quick Chat session exists, a synchronous or asynchronous setup failure shall retain its task, session, tab, preparation details, and safe error. Reload and reconnect shall restore the failed conversation without placing it on the task board.
- **AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-003.2:** Navigating tabs, opening another chat, or dismissing the Quick Chat surface during creation shall not delete a created conversation. A late creation result shall reconcile its identity without stealing the user's newer selection or reopening a dismissed surface. Explicit deletion and existing expiration policy remain available.
- **AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-003.3:** A failed chat shall expose the existing valid recovery actions and allow explicit deletion. Opening or restoring it shall not automatically retry away the error. Retrying shall reuse its task/conversation and preserve prior failure history.
- **AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-003.4:** A failure before a session exists shall retain the setup form, selected agent, repositories, and an inline safe error with a retry action. A toast alone shall not be the error surface. No nonexistent conversation shall be advertised as restorable.
- **AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-003.5:** Desktop and phone shall expose the same failure and recovery outcomes. Phone users shall retain the full-height Quick Chat surface, scroll to error details, and reach actions with at least 44px touch targets without horizontal page overflow.

### REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004: Abandoned workspace materialization recovery

Draft amendment, 2026-10-12. Delivery: [Environment failure recovery](../../../plans/upstream-environment-failure-recovery/plan.md).

**Intent:** A task whose first workspace materialization was abandoned or failed
before it created anything physical can start again, while a workspace that
exists or is still being prepared stays protected.

**User story:** As a user whose first launch failed before its workspace was
created, I want the next start of the task to prepare the workspace again, so
that the task does not stay unlaunchable until I find a manual reset.

Terms used below:

- *Materialization owner*: the session elected to turn the task's `creating`
  environment into a physical workspace.
- *Launch budget*: the configured preparation timeout plus the agent launch
  timeout plus the allowance that the orphaned-session reconciliation uses.
- *Live owner*: an owner session in `STARTING` or `RUNNING` with a durable
  runtime record.
- *Physical evidence*: a workspace path, container, sandbox, or non-deleted
  repository inventory row on the environment.

- **AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.1:** When a start or launch whose
  session owns a `creating` environment returns an error before the environment
  is ready, the system shall release that claim by storing the `failed` state.
  A session ceiling deferral is not an error for this purpose and shall keep
  the claim.
- **AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.2:** When a session binding finds
  a `creating` environment whose owner is missing or terminal, or whose claim is
  older than the launch budget without a live owner, the system shall elect the
  requesting session as the new owner instead of returning `workspace_preparing`
  or `workspace_reuse_unsafe`.
- **AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.3:** When the owner of a `creating`
  environment is live, or its claim is younger than the launch budget, a session
  binding shall keep returning the recoverable `workspace_preparing` error and
  shall not change the environment.
- **AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.4:** When a session binding, or the
  launch of a session already bound to the environment, finds a `failed`
  environment of the same task or shared workspace group with no owner and no
  physical evidence, the system shall elect that session as the new owner and run
  full workspace preparation.
- **AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.5:** When a `failed` environment has
  physical evidence, or belongs to a parent task through `inherit_parent`, a
  start shall keep failing with `workspace_reuse_unsafe` and shall not change the
  environment. The existing environment reset action remains its recovery.
- **AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.6:** When several starts re-elect the
  same environment at the same time, exactly one shall become the owner. A launch
  that loses the race shall wait for the new owner like any `creating` sibling or
  attach to its published workspace, and no second physical workspace shall be
  created.
- **AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.7:** When a former owner finishes or
  fails after its claim was re-elected, its readiness publication and its failure
  write shall be refused without changing the environment.
- **AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.8:** Re-election shall happen only
  inside a session binding, start, or launch request. No background process,
  page load, or failed `session.ensure` request shall retry a failed environment
  without a new request.
- **AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.9:** A task that is being deleted or
  an environment held by a worktree recovery claim shall not be re-elected; the
  binding shall fail as it does today.
- **AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.10:** When a start is refused
  because a `failed` environment of the task has physical evidence, the error
  shall keep the `workspace_reuse_unsafe` reason and its message and retry
  guidance shall name the task environment reset action, instead of asking the
  caller to retry after the initial launch or to restore the workspace.

## Out of scope

- A background poller for all repository defaults.
- A PR gate for manual launches or providers other than GitHub.
- Changes to ACP resume semantics or bulk repository-default repair.
- A periodic sweep for abandoned `creating` environments. Abandonment is
  decided when a start needs the environment.
- Automatic replacement of a `failed` environment that has physical evidence,
  for example a worktree created before the launch failed. It needs a way to
  retain the old environment while a new one becomes canonical, which is a
  separate schema decision.

## System design

- [Task Launch Failure Recovery System Design](../system-design/task-launch-failure-recovery.md)
