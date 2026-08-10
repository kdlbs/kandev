---
status: draft
created: 2026-08-08
owner: kandev
---

# Agent Runtime Availability

## Why

Kandev's standalone `agentctl` process is a core runtime dependency, but the
application backend can remain reachable after that child exits. In that state
the browser WebSocket still looks connected while agent actions, task updates,
and workspace tools stop progressing. Users need an authoritative explanation
and a supported recovery path without losing the last known task or workspace
data.

## What

- Kandev exposes one install-wide local agent-runtime availability snapshot:
  `available` or `unavailable`.
- The runtime becomes `available` only after the standalone `agentctl` process
  passes its health check and authentication handshake.
- An unexpected `agentctl` exit atomically changes the snapshot to
  `unavailable`, records the stable reason `agentctl_exited` and occurrence
  time, and publishes that snapshot to connected browsers.
- Intentional shutdown does not publish an outage.
- A newly loaded or reconnected browser receives the latest snapshot even when
  it missed the original exit event.
- While unavailable, every authenticated application route shows one
  persistent, non-dismissible alert explaining that the local agent runtime
  stopped, agent actions and workspace tools may not update, saved data remains
  intact, and Kandev must restart to recover.
- The alert is independent of the optional App status bar and remains visible
  when route data is empty or stale. It is not a transient toast.
- When the local restart capability is supported, the alert offers **Restart
  Kandev** and reuses the existing supervised restart progress flow. Otherwise
  it gives localized terminal or service-manager guidance. A failed restart
  request leaves the alert visible and exposes the failure.
- The browser retains last-known task, session, workspace, and message state
  while the runtime is unavailable. The availability event never clears those
  stores.
- The alert clears only when a fresh backend boot reports a healthy authenticated
  `agentctl`; browser WebSocket reconnection alone does not clear it.

## API surface

The boot payload and `/api/v1/app-state` initial state include:

```json
{
  "agentRuntime": {
    "status": "unavailable",
    "reason": "agentctl_exited",
    "occurred_at": "2026-08-08T14:22:52Z"
  }
}
```

`reason` and `occurred_at` are omitted while available. Unexpected exits publish
the same shape as the install-wide WebSocket action
`system.agent_runtime.status_changed`. The reason is a stable enum for UI
selection; raw process errors, stack traces, PIDs, and exit details remain in
diagnostic logs.

The existing `/api/v1/system/restart-capability`, `/api/v1/system/restart`, and
`boot_id` contracts remain the recovery API. This feature does not change the
general backend health endpoint.

## State machine

| Current state | Event | Next state | User result |
| --- | --- | --- | --- |
| startup | Health and auth handshake succeed | `available` | No alert |
| `available` | Child exits unexpectedly | `unavailable` | Persistent alert and recovery guidance |
| `unavailable` | Browser reconnects or reloads | `unavailable` | Snapshot is replayed; alert remains |
| `unavailable` | Supervisor starts a fresh healthy backend and child | `available` | New boot removes the alert |
| any | Intentional backend shutdown stops child | no outage transition | No stale failure is emitted |

The in-memory snapshot is monotonic within one backend boot: an unexpected exit
cannot return to `available` because this repair does not restart `agentctl` in
process.

## Failure modes

- If the runtime-status event cannot be delivered, boot hydration and replay on
  user subscription provide the current snapshot on reload or reconnect.
- If restart capability lookup fails, the alert stays visible and presents
  manual restart guidance instead of assuming the launch mode.
- If a supervised restart request fails, the existing backend remains in the
  unavailable state and the UI exposes the request error without dismissing the
  alert.
- If the browser WebSocket is also offline, the existing connectivity warning
  remains independent. Both conditions may be visible because they describe
  different failures.
- If `agentctl` fails before startup completes, normal backend startup failure
  behavior applies; no browser is served an inaccurate available snapshot.

## Persistence guarantees

Availability is process-local operational state, not database history. It is
initialized from the current launch, retained for the lifetime of that backend,
and replayed to clients. Kandev does not persist crash history or restore an
unavailable marker after a successful full restart. Existing task, workspace,
message, and session persistence is unchanged.

## Responsive and mobile contract

- Desktop and tablet render the alert in flow at the top of the route-content
  column, above route data and below no route-owned chrome.
- Phone uses the same in-flow alert rather than a toast, drawer, or second
  persistent bottom bar. Copy and actions stack vertically, stay within the
  viewport, and do not introduce document horizontal overflow.
- The restart action is at least 44 px on phone. The alert remains above the
  route's single scroll owner and does not cover bottom navigation or safe-area
  content.
- The alert uses `role="alert"`, a textual heading and explanation, and does not
  rely on color alone.

## Scenarios

- **GIVEN** a healthy running Kandev, **WHEN** `agentctl` exits unexpectedly,
  **THEN** every connected browser receives `unavailable` and shows one
  persistent alert while preserving last-known application data.
- **GIVEN** the runtime became unavailable before a browser connected, **WHEN**
  the browser loads or subscribes after WebSocket reconnect, **THEN** it receives
  the current unavailable snapshot and shows the same alert.
- **GIVEN** the App status bar feature is disabled, **WHEN** the runtime becomes
  unavailable, **THEN** the alert is still visible on every authenticated route.
- **GIVEN** supervised restart is supported, **WHEN** the user chooses **Restart
  Kandev**, **THEN** the existing restart flow waits for a changed `boot_id` and
  the fresh healthy boot clears the alert.
- **GIVEN** restart is unsupported, **WHEN** the alert renders, **THEN** it gives
  localized terminal or service-manager guidance and does not offer a broken
  restart action.
- **GIVEN** Kandev is intentionally shutting down, **WHEN** the launcher stops
  `agentctl`, **THEN** no unavailable event is emitted.
- **GIVEN** a phone viewport with sparse or stale route data, **WHEN** the runtime
  is unavailable, **THEN** the alert and any restart action fit without overlap,
  horizontal scrolling, or an inaccessible touch target.

## Out of scope

- Restarting `agentctl` inside the existing backend process or rebinding its
  launch token, PID, lifecycle manager, control clients, and active executions.
- Automatically restarting all of Kandev without an operator action.
- Changing `/health` readiness semantics, container/service restart policies,
  or browser WebSocket retry behavior.
- Persisted crash history, crash counters, raw diagnostics in the UI, or a new
  operational dashboard.
- Declaring remote executors unavailable or disabling every application action;
  the alert describes the local standalone runtime dependency.

## Decision record

[Publish immutable agent events and explicit runtime failure state](../../decisions/2026-08-08-agentctl-crash-containment.md)

## Implementation plan

[Backend failure containment](../../plans/backend-failure-containment/plan.md)
