---
status: draft
created: 2026-08-03
owner: kandev
---

# Cancel-turn progress across task navigation

## Why

Cancelling an agent turn can take long enough for a user to inspect another task. Today the
cancel control loses its progress animation when the user returns, making an outstanding request
look retryable and leaving the user unsure whether Kandev is still stopping the agent.

## What

- Starting a turn cancellation puts that session's cancel control into a disabled progress state
  until the cancellation request settles.
- The progress state survives task or session navigation that remounts the chat composer within
  the same browser tab.
- Cancellation progress is isolated by session; cancelling one session does not animate or disable
  another session's control.
- Repeated activation while the request is pending sends only one cancellation request.
- Success, rejection, timeout, or an unavailable WebSocket clears the progress state so a still
  running session can be retried.
- Desktop and mobile chat composers expose the same progress behavior through their shared cancel
  control.

## Persistence guarantees

Cancellation progress is transient browser-tab state keyed by session ID. It survives React
component and task-route remounts under the existing application-level state provider. It does not
survive a full page reload, browser restart, or navigation that replaces the application shell;
after those boundaries, authoritative session state determines whether the cancel control remains
available.

## Failure modes

- If the WebSocket client is unavailable, the UI does not retain a stuck progress state.
- If `agent.cancel` rejects or reaches its client timeout, the progress state clears and the control
  becomes retryable when the session is still running.
- If the session stops through another lifecycle event while cancellation is pending, existing
  session-state rendering may remove the cancel control; settling the original request still clears
  its transient session entry.

## Scenarios

- **GIVEN** an agent turn is running, **WHEN** the user activates the cancel control, **THEN** the
  control is disabled, shows progress, and sends exactly one `agent.cancel` request until that
  request settles.
- **GIVEN** a cancellation request is still pending, **WHEN** the user opens another task and returns
  to the original task in the same tab, **THEN** the original session's cancel control still shows
  progress and remains disabled.
- **GIVEN** one session has a pending cancellation, **WHEN** the user views another running session,
  **THEN** the other session's cancel control does not show cancellation progress.
- **GIVEN** a pending cancellation rejects or times out while the session remains running, **WHEN**
  the user returns to that session, **THEN** its cancel control is enabled for another attempt.
- **GIVEN** the compact mobile chat composer is active, **WHEN** the same cancel-and-navigate flow
  occurs, **THEN** its shared cancel control reflects the same session-scoped progress state.

## Out of scope

- Persisting cancellation progress across a full page reload or browser restart.
- Changing the `agent.cancel` WebSocket contract, backend cancellation semantics, or timeout.
- Adding new cancel copy, notifications, task navigation, layout, or touch interactions.
