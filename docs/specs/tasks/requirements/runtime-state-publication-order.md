---
status: active
system: tasks
created: 2026-07-30
owners:
  - kandev
---
# Runtime Task-State Publication Order Requirements

## Overview

This document is the migrated task-system source for the capability. The source detail below remains authoritative while the system is migrated into separate requirement and design records.

## Requirements

### REQ-TASKS-RUNTIME-STATE-PUBLICATION-ORDER-001: Runtime Task-State Publication Order

**Intent:** Preserve the observable task or workflow behavior recorded by the legacy specification.

#### Acceptance criteria

- **AC-TASKS-RUNTIME-STATE-PUBLICATION-ORDER-001.1:** When a consumer uses this capability, the system shall provide the observable behavior and exclusions documented below.

## Migrated source detail

## Why

Task surfaces combine the persisted task state with live session state. If a
session becomes `RUNNING` before its task is observably `IN_PROGRESS`, the
sidebar can briefly show a running spinner inside the `Review` state group even
though the task's workflow step and runtime are already active.

## What

- For a non-Office, unarchived task whose session transitions into `RUNNING`,
  Kandev reconciles the persisted task state to `IN_PROGRESS` before publishing
  that session's `session.state_changed` event.
- The same ordering applies when an answered `ask_user_question_kandev` call
  resumes the still-open agent turn directly, without launching a replacement
  prompt or waiting for a later runtime stream event.
- When reconciliation changes the task state, observers receive
  `task.state_changed` before `session.state_changed` announces `RUNNING`.
- The task and session publications share the task's existing per-task FIFO, so
  a session event cannot overtake an earlier task event when another
  task-scoped publication is already draining.
- The WebSocket gateway consumes both lifecycle notifications through one
  ordered NATS-style subscription, preserving that order when the event bus is
  remote; the in-memory event bus supports the same wildcard semantics.
- The task-state write remains guarded by the owning session's authoritative
  state. A concurrent clarification, cancellation, terminal transition, or
  archive prevents a stale runtime writer from promoting the task.
- Repeated stream activity for a session already in `RUNNING` does not cause
  repeated task-state writes or events.
- The executor's post-start task-state reconciliation remains an eventual
  recovery path when the earlier reconciliation fails or is skipped.
- Existing sidebar state grouping, workflow-step rendering, status icons, and
  desktop/mobile composition remain unchanged.

## Data model

This repair uses the existing persisted fields:

- `tasks.state`, including `REVIEW` and `IN_PROGRESS`
- `task_sessions.state`, including `WAITING_FOR_INPUT`, `STARTING`, and
  `RUNNING`

No schema, enum, or persistence-format changes are introduced.

## API surface

The existing WebSocket event names and payloads remain unchanged:

- `task.state_changed`
- `session.state_changed`

The ordering contract is defined by
[ADR-2026-07-30-runtime-task-state-before-running-event](../../../decisions/2026-07-30-runtime-task-state-before-running-event.md).

## State machine

- A workflow transition may persist the target workflow step and use `REVIEW`
  as a safe intermediate task state while no session owns active work.
- When a prompt or runtime transition successfully changes an owning session to
  `RUNNING`, the same lifecycle operation reconciles the task to
  `IN_PROGRESS` before exposing the new session state to clients.
- When a clarification answer wakes a `WAITING_FOR_INPUT` session inside the
  open MCP call, the clarification lifecycle performs the guarded task
  reconciliation and publishes its task event before publishing the
  `RUNNING` session event.
- When the turn settles and no sibling session is working, existing behavior
  returns the task to `REVIEW`.

## Failure modes

- If guarded task reconciliation loses to a newer session/task transition, it
  does not overwrite the winner.
- If task reconciliation returns an error after the session transition is
  persisted, Kandev logs the error and still reports the truthful session
  state. The existing executor-success reconciliation remains available to
  heal the task state.
- If the task publication queue is already draining, the task-state and
  session-state events append in lifecycle order without waiting reentrantly on
  the active publication.
- Office task status remains controlled by the Office lifecycle and is not
  promoted by this runtime reconciliation.

## Scenarios

- **GIVEN** a non-Office task in `REVIEW` with an owning session in
  `WAITING_FOR_INPUT`, **WHEN** the session accepts a new prompt and transitions
  to `RUNNING`, **THEN** `tasks.state` is `IN_PROGRESS` and
  `task.state_changed` is published before `session.state_changed(RUNNING)`.
- **GIVEN** a non-Office task in `REVIEW` whose owning session is
  `WAITING_FOR_INPUT` inside an open `ask_user_question_kandev` call, **WHEN**
  the user answers and the same agent turn resumes, **THEN** the persisted task
  becomes `IN_PROGRESS`, WebSocket observers receive `task.state_changed`
  before `session.state_changed(RUNNING)`, and the State-grouped sidebar moves
  the task out of `Review` without a page reload.
- **GIVEN** the sidebar is grouped by State, **WHEN** it observes a session
  transition to `RUNNING`, **THEN** the task is not rendered with a running
  spinner inside the `Review` group.
- **GIVEN** a remote event bus delivers the two lifecycle events, **WHEN** the
  gateway forwards them to WebSocket clients, **THEN** the task-state
  notification is delivered before the running-session notification.
- **GIVEN** a session and task already in `RUNNING`/`IN_PROGRESS`, **WHEN**
  additional tool or stream events arrive, **THEN** no redundant task-state
  write or state-change event is produced.
- **GIVEN** a clarification or cancellation changes the session before guarded
  task reconciliation commits, **WHEN** the stale runtime transition resumes,
  **THEN** the task is not promoted to `IN_PROGRESS`.
- **GIVEN** an archived or Office task, **WHEN** a session reports runtime
  activity, **THEN** this repair does not rewrite its task state.

## Out of scope

- Changing how sidebar State groups are derived or labeled.
- Inferring persisted task state in the frontend from session events.
- Changing workflow-step transitions, session-state enums, WebSocket payloads,
  Office task-state ownership, or task status icons.
- Adding new desktop or mobile layout, navigation, or touch behavior.