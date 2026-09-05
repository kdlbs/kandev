# E2E UI State and Cleanup

Load this reference for session/WS lifecycle assertions, terminal and Dockview
helpers, or sidebar/context-menu interactions.

## Workflow and session invariants

For session-primary/profile behavior, prefer polling backend state with
`apiClient.listTaskSessions(taskId)` for invariants such as
`agent_profile_id`, `is_primary`, `state`, and session count, then add UI
assertions as secondary evidence. Agent output is not lifecycle readiness: if a
follow-up must use the synchronous prompt path rather than the queued path, or
a transcript is mutated after an auto-started boot turn, poll the exact session
state such as `WAITING_FOR_INPUT` before acting. The mock agent can persist text
before the lifecycle transition completes. Verify the affected full spec and
original CI shard with `--retries=0`.

## WebSocket capture

Reset capture arrays immediately before the stimulus, but finish all
subscription and lifecycle assertions before clearing them. Reselecting an
already-active session may emit no new subscribe frame; consume the original
frame or trigger a distinct state transition. Verify ordering in the owning
Playwright project with `--retries=0`.

Arm every WebSocket wait before the action that should produce it, including all
waits when one action emits multiple notifications. Establish startup state with
REST `expect.poll`, then correlate each post-action event by an active-to-settled
transition, revision, or timestamp; `watchWs` does not buffer frames, so never
attach a wait after a spinner or visibility assertion.

## Terminal and Dockview helpers

- Scope terminal/mobile helpers to the active `data-testid="terminal-panel"`;
  multiple terminal panels can be mounted at once.
- Scope Dockview preview polling to visible panels. Hidden or stale panels can
  remain mounted and produce false positives when helpers scan custom elements.
- If a helper uses `window.__dockviewApi__`, poll until the API is attached;
  silently skipping cleanup during page initialization leaks panels into later
  assertions.
- Dockview-capable specs must activate a visible public entry point (a tab or
  topbar/toolbar affordance) instead of assuming the default tab exists after
  another scenario. Scope assertions to the visible panel and run the full spec
  to catch order-dependent persisted-layout state.

For a restored hidden-surface regression, seed the persisted multi-panel state,
leave the target inactive, reload, and prove the target is mounted before
activating it through the shipped control. If the failure depends on a browser
observer missing the hidden-to-visible geometry transition, install a
pre-navigation wrapper that suppresses callbacks only for the target entries
and leaves unrelated observers intact. Assert both the correctly shaped
network request, including its cursor or direction, and the visible user
result. This distinguishes lifecycle recovery from an accidentally cooperative
observer without exposing a test-only production entry point.

## Sidebar and context-menu editing

For actions targeting a non-active item, create or select a separate navigation
task, assert its initial URL/task ID, act on the target row, then assert that the
URL/task ID is unchanged and persistence survives reload. Cover shared
desktop/mobile/tablet menus.
