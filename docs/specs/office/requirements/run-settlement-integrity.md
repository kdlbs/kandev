---
status: draft
system: office
created: 2026-10-20
owners:
  - kandev
---

# Office Run Settlement Integrity Requirements

## Overview

An Office run is claimed while its agent works and must reach a terminal state
exactly once, for the work that actually happened. Three gaps broke that
contract on a production instance:

- A run launched on the per-launch session model never settled, because its
  session parked at `WAITING_FOR_INPUT` with the agent process alive. The run
  stayed claimed, held the agent's run ceiling, and stale-claim recovery
  requeued it into a fresh duplicate launch every 30 minutes.
- A child task with `metadata.workspace.mode = inherit_parent` could not launch
  when its parent had been run only by Office. Office sessions are never
  primary, and inheritance read only the parent's primary session. The launch
  retried silently on backoff with nothing visible.
- An agent failure made the run terminal and queued an `on_agent_error`
  replacement run while shared dynamic routing launched a successor in the same
  session. The replacement and the recovered original repeated the same work.

Office owns this contract because the run, its claim, its retry policy, and its
failure escalation are Office records. Workspace inheritance itself is a task
contract; this document owns only the Office-visible outcome of resolving it.

## Terminology

- **Launched session:** the task session recorded on the run row when the run
  launched its agent.
- **Per-launch session model:** the Kanban session model in which each launch
  creates a session that parks at `WAITING_FOR_INPUT` after a turn. It applies
  to tasks whose project/workflow identity does not make them Office tasks,
  such as tasks created in the Routine workflow.
- **Session verdict:** the session state the orchestrator records after an
  agent failure: a failure state, or a successor that continues the session.

## Requirements

### REQ-OFFICE-RUN-SETTLE-001: Parked launched-session settlement

**Intent:** A run whose agent finished its turn is finished, regardless of the
session model, so the agent's next run can be claimed and stale-claim recovery
never relaunches finished work.

#### Acceptance criteria

- **AC-OFFICE-RUN-SETTLE-001.1:** When a claimed task run's launched session
  uses the per-launch model and moves from `RUNNING` to `WAITING_FOR_INPUT`
  without an error message, the system shall finish the run with the processed
  outcome and release its task checkout.
- **AC-OFFICE-RUN-SETTLE-001.2:** When a session that is not the run's launched
  session parks, or a session parks with an error message, or a session moves
  to `WAITING_FOR_INPUT` from a state other than `RUNNING`, the system shall
  leave the run claimed.
- **AC-OFFICE-RUN-SETTLE-001.3:** When an Office task session parks at `IDLE`,
  the system shall leave settlement to the existing teardown completion path.
- **AC-OFFICE-RUN-SETTLE-001.4:** An Office task shall retain its run claim
  and checkout at the early park until that turn's runtime stop succeeds or
  its completion owner confirms a no-stop path. Cancelled turns, sessions
  without an execution, and superseded teardowns shall still settle through
  that completion confirmation. A failed stop shall not publish successful
  settlement. Duplicate confirmations shall not repeat completion effects.
- **AC-OFFICE-RUN-SETTLE-001.5:** When an Office IDLE teardown runs after its
  session was relaunched (the session is `STARTING`, is `RUNNING` again after
  the completion snapshot was not `RUNNING`, holds a different agent execution,
  or is running a turn other than the one that just completed), the system
  shall skip the IDLE flip and the agent stop.
- **AC-OFFICE-RUN-SETTLE-001.6:** A completing Office turn shall not settle
  a newer claim on the reused session. Its completion confirmation shall
  retain the original turn start; legacy completion timestamps older than
  the current claim shall be ignored.
- **AC-OFFICE-RUN-SETTLE-001.7:** Failed bootstrap cleanup shall retain the
  run's failure outcome. A later automatic failure projection shall preserve
  an already completed business task and its original result; existing manual
  task-state changes and session failure history shall remain available.

### REQ-OFFICE-RUN-SETTLE-002: Inherited workspace launch resolution

**Intent:** A child that inherits its parent's workspace can launch whenever the
parent has a valid workspace, and a launch that cannot inherit one is reported
instead of retried silently.

#### Acceptance criteria

- **AC-OFFICE-RUN-SETTLE-002.1:** When an `inherit_parent` task's parent has no
  primary session and its sessions all point at one environment that exists
  and whose owner is not archived, the system shall bind the child session to
  that environment.
- **AC-OFFICE-RUN-SETTLE-002.2:** When the parent's sessions carry no
  environment and the parent owns a task environment that exists, the system
  shall bind the child session to the parent-owned environment.
- **AC-OFFICE-RUN-SETTLE-002.3:** When no candidate environment exists or its
  owner is archived or unverifiable, the system shall refuse the launch as an
  unsafe workspace reuse.
- **AC-OFFICE-RUN-SETTLE-002.4:** When a run's launch is refused as an unsafe
  workspace reuse that can never succeed (archived parent, terminal parent
  without an environment, a dangling or foreign-owner environment, or
  conflicting session environments), the system shall fail the run without
  scheduling a retry and shall escalate it through the existing failure
  activity and coordinator escalation.
- **AC-OFFICE-RUN-SETTLE-002.5:** When the refusal is only that a live parent
  (not archived and not terminal) has not materialized any environment yet, the
  system shall keep the run on the existing retry backoff without escalating it,
  and on the first deferral shall record the refusal reason as a
  `workspace.inheritance_pending` run event and a `run.workspace_pending`
  activity entry.

### REQ-OFFICE-RUN-SETTLE-003: Agent-error session verdict

**Intent:** An operator can make Office wait for the failed session's verdict
before failing a task run, so a successor launched in the same session keeps
the work instead of competing with an `on_agent_error` replacement run.

#### Acceptance criteria

- **AC-OFFICE-RUN-SETTLE-003.1:** The runtime flag
  `features.officeAgentErrorSessionVerdict`
  (`KANDEV_FEATURES_OFFICE_AGENT_ERROR_SESSION_VERDICT`) shall default to off in
  every shipped profile and require a restart. When it is off, the system shall
  keep the existing immediate failure path.
- **AC-OFFICE-RUN-SETTLE-003.2:** When the flag is on and a task run's agent
  fails while its session is `CREATED`, `STARTING`, or `RUNNING`, the system
  shall keep the run claimed and shall not dispatch `on_agent_error`.
- **AC-OFFICE-RUN-SETTLE-003.3:** When a deferred failure's session then
  records `FAILED`, `CANCELLED`, or `WAITING_FOR_INPUT` with an error message,
  the system shall apply the existing terminal failure path and dispatch
  `on_agent_error` once.
- **AC-OFFICE-RUN-SETTLE-003.4:** When a deferred failure's session moves to
  `STARTING`, or parks without an error, the system shall discard the deferred
  failure and leave the run to the successor's completion.
- **AC-OFFICE-RUN-SETTLE-003.5:** When the flag is on and the session already
  records a failure verdict at the time of the agent failure, the system shall
  apply the failure at once.
- **AC-OFFICE-RUN-SETTLE-003.6:** When no state event resolves a deferred
  failure within the re-check delay, the system shall read the session again
  and apply the failure only when the session records a failure verdict.

## Out of scope

- Changing which tasks are classified as Office tasks, or the per-launch
  session model itself.
- Promoting Office sessions to primary. Office sessions do not use the primary
  mechanism by design (`tasks-01.md`).
- Changing dynamic routing's failover decision or the `on_agent_error` workflow
  action.
- Persisting deferred failures across a backend restart.
