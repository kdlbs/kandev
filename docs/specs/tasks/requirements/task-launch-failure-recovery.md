---
status: draft
system: tasks
created: 2026-08-19
updated: 2026-09-01
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

## Out of scope

- A background poller for all repository defaults.
- A PR gate for manual launches or providers other than GitHub.
- Changes to ACP resume semantics or bulk repository-default repair.

## System design

- [Task Launch Failure Recovery System Design](../system-design/task-launch-failure-recovery.md)
