---
status: active
system: ui
created: 2026-08-03
owners:
  - kandev
---
# Backend-owned cancel-turn progress Requirements

## Overview

Cancelling an agent turn can outlive the chat component or browser page that started it. Users need the cancel control to keep showing the real operation state after task navigation, a page reload, or opening the session in another tab, rather than treating one browser component as the source of truth.

## Requirements

### REQ-UI-CANCEL-TURN-PROGRESS-001: Backend-owned cancel-turn progress

**Intent:** Cancelling an agent turn can outlive the chat component or browser page that started it. Users need the cancel control to keep showing the real operation state after task navigation, a page reload, or opening the session in another tab, rather than treating one browser component as the source of truth.

#### Acceptance criteria

- **AC-UI-CANCEL-TURN-PROGRESS-001.1:** Activating turn cancellation immediately disables that session's cancel control and shows its existing progress animation.
- **AC-UI-CANCEL-TURN-PROGRESS-001.2:** Once the backend accepts the request, the backend owns whether cancellation is pending. Every client renders the same session-scoped state from backend hydration and live updates.
- **AC-UI-CANCEL-TURN-PROGRESS-001.3:** Pending progress survives task and session navigation, a full page reload, replacement of the browser tab, and a second tab while the same backend cancellation operation remains active.
- **AC-UI-CANCEL-TURN-PROGRESS-001.4:** Cancellation progress is isolated by session; cancelling one session does not animate or disable another session's control.
- **AC-UI-CANCEL-TURN-PROGRESS-001.5:** Repeated activation may send a duplicate transport request during a race, but the backend runs at most one cancellation operation for that session and keeps progress pending until the owning operation settles.
- **AC-UI-CANCEL-TURN-PROGRESS-001.6:** Success, reconciliation of a missing or unresponsive execution, rejection, or timeout clears the backend progress state. A still-running session becomes retryable after a failed attempt.
- **AC-UI-CANCEL-TURN-PROGRESS-001.7:** Desktop and mobile chat composers expose the same behavior through their shared cancel control.
- **AC-UI-CANCEL-TURN-PROGRESS-001.8:** **GIVEN** an agent turn is running, **WHEN** the user activates the cancel control, **THEN** the control is immediately disabled, shows progress, and the backend runs one guarded cancellation.

## Migrated source detail

## Why

Cancelling an agent turn can outlive the chat component or browser page that started it. Users
need the cancel control to keep showing the real operation state after task navigation, a page
reload, or opening the session in another tab, rather than treating one browser component as the
source of truth.

Decision: [ADR-2026-08-03-backend-owned-cancellation-progress](../../../decisions/2026-08-03-backend-owned-cancellation-progress.md)

## What

- Activating turn cancellation immediately disables that session's cancel control and shows its
  existing progress animation.
- Once the backend accepts the request, the backend owns whether cancellation is pending. Every
  client renders the same session-scoped state from backend hydration and live updates.
- Pending progress survives task and session navigation, a full page reload, replacement of the
  browser tab, and a second tab while the same backend cancellation operation remains active.
- Cancellation progress is isolated by session; cancelling one session does not animate or disable
  another session's control.
- Repeated activation may send a duplicate transport request during a race, but the backend runs at
  most one cancellation operation for that session and keeps progress pending until the owning
  operation settles.
- Success, reconciliation of a missing or unresponsive execution, rejection, or timeout clears the
  backend progress state. A still-running session becomes retryable after a failed attempt.
- Desktop and mobile chat composers expose the same behavior through their shared cancel control.

## Data model

Cancellation progress is a backend runtime projection keyed by task-session ID:

- `cancellation_pending: boolean` is `true` while the orchestrator has an accepted cancellation
  operation for the session and `false` otherwise.
- `cancellation_revision: uint64` is a process-local per-session generation. It increments only on
  the first accepted reference (`0 → 1`) and the final release (`1 → 0`); overlapping references do
  not create extra generations. The backend reads the boolean and revision atomically.
- The field is orthogonal to `TaskSession.state`; cancellation does not introduce a `CANCELLING`
  lifecycle state and the session remains `RUNNING` until the existing lifecycle reconciliation
  changes it.
- The projection is not written to `task_sessions`, session metadata, browser storage, or another
  durable store. It is derived from the orchestrator's live cancellation registry and is serialized
  explicitly as both `true` and `false` so hydration can clear stale client state.
- The frontend may retain a session-keyed optimistic request flag between the click and the first
  backend signal. The effective control state is optimistic pending OR backend pending; only the
  backend field is authoritative after request acceptance and across page boundaries.

## API surface

- The existing `agent.cancel` WebSocket request remains
  `{ "session_id": string }`. Its success/error response continues to describe the completed
  cancellation attempt; request authorization and error codes do not change.
- Full and summary task-session DTOs add the non-optional wire fields
  `cancellation_pending: boolean` and `cancellation_revision: uint64`. The frontend's in-memory
  `TaskSession` type may keep both properties optional for partial event/test rows, but backend DTO,
  boot, REST, and subscription boundaries always serialize explicit values.
- Task-detail boot state and task-session REST responses include the field on every session row.
- The initial session-subscription `session.state_changed` snapshot includes
  `cancellation_pending` and `cancellation_revision`, including explicit `false` and `0` values.
- Live transitions use the semantic WebSocket notification `session.cancellation_changed` with
  payload `{ "session_id": string, "cancellation_pending": boolean, "cancellation_revision": number }`.
  It is delivered only to clients subscribed to that session; task-summary and task-switcher traffic
  do not carry it. Clients reject values whose revision is lower than the session's current revision.

## State machine

| State | Trigger | Result |
|---|---|---|
| Idle | An authorized `agent.cancel` request is accepted | Backend publishes pending and starts or joins the guarded cancellation operation. |
| Pending | The initiating page disconnects or reloads | No transition; the backend operation continues. |
| Pending | A duplicate request arrives | The existing operation remains the sole lifecycle cancellation; progress stays pending. |
| Pending | Cancellation succeeds or the backend reconciles a missing/unresponsive execution | Backend publishes idle after lifecycle, session, message, and turn reconciliation settle. |
| Pending | Cancellation fails or times out | Backend publishes idle and the existing request error behavior remains visible to the initiating client. |

## Permissions

Invoking cancellation continues to use the existing session authorization check. Boot state, REST
session reads, session subscriptions, and live cancellation notifications retain their existing
workspace/session visibility rules.

## Failure modes

- If the WebSocket is unavailable before the backend accepts the request, the optimistic browser
  flag clears on request failure and no backend pending state is created.
- If the initiating page disconnects after acceptance, cancellation continues independently of the
  request connection. A replacement page hydrates the backend state.
- If a live cancellation notification is missed, the next boot payload, REST session read, or
  session-subscription snapshot repairs the client.
- If cancellation fails or reaches its lifecycle timeout, the backend clears pending in all exit
  paths. A session still reported as running exposes the cancel control for retry.
- If publishing a live transition fails, cancellation itself still proceeds; an authoritative
  snapshot repairs the display on the next read or subscription.

## Persistence guarantees

The state survives React remounts, SPA navigation, full page reloads, browser-tab replacement, and
other clients for as long as the same backend process owns the active cancellation operation. The
accepted operation also ignores cancellation of the initiating WebSocket request.

The runtime projection intentionally does not survive a Kandev backend restart. There is no safe
continuation token for an in-flight process cancellation, so startup exposes no stale pending flag
and existing session/execution recovery determines the durable coarse state. If the session remains
`RUNNING`, the user can issue another idempotent cancellation request.

## Scenarios

- **GIVEN** an agent turn is running, **WHEN** the user activates the cancel control, **THEN** the
  control is immediately disabled, shows progress, and the backend runs one guarded cancellation.
- **GIVEN** the backend reports cancellation pending for task A, **WHEN** the user opens task B and
  returns to task A, **THEN** task A's cancel control still shows progress and remains disabled.
- **GIVEN** the backend reports cancellation pending, **WHEN** the user reloads or replaces the
  page, **THEN** the first hydrated task-session state shows the disabled animated control without
  waiting for a new transition.
- **GIVEN** one tab has an accepted cancellation pending, **WHEN** a second authorized tab opens the
  same session, **THEN** the second tab shows the same pending control state.
- **GIVEN** one session has a pending cancellation, **WHEN** the user views another running session,
  **THEN** the other session's cancel control remains independent and enabled.
- **GIVEN** a cancellation is pending, **WHEN** another cancel request arrives for that session,
  **THEN** the backend does not invoke a second lifecycle cancellation and does not clear progress
  until the owning operation settles.
- **GIVEN** cancellation rejects or times out while the session remains running, **WHEN** pending is
  cleared, **THEN** the control becomes retryable.
- **GIVEN** the compact mobile chat composer is active, **WHEN** cancellation is accepted and the
  page reloads, **THEN** its shared cancel control hydrates the same backend-owned progress state.
- **GIVEN** cancellation was pending before a backend restart, **WHEN** Kandev starts again, **THEN**
  no stale pending flag is exposed and the recovered session state determines whether cancel can be
  retried.

## Out of scope

- Persisting or replaying an in-flight cancellation across a Kandev backend restart.
- Adding `CANCELLING` to the durable session lifecycle or changing prompt-admission semantics.
- Changing the lifecycle cancellation timeout, escalation policy, or terminal task/session result.
- Adding task-list badges, new cancel copy, notifications, navigation, layout, or touch gestures.
