---
status: active
system: ui
created: 2026-07-31
owners:
  - kandev
---
# Context Window Reset Freshness Requirements

## Overview

The context-window indicator can continue showing the previous conversation's nearly-full usage after the user resets an agent session. That reading describes context which no longer exists and makes a fresh conversation look almost exhausted.

## Requirements

### REQ-UI-CONTEXT-WINDOW-RESET-FRESHNESS-001: Context Window Reset Freshness

**Intent:** The context-window indicator can continue showing the previous conversation's nearly-full usage after the user resets an agent session. That reading describes context which no longer exists and makes a fresh conversation look almost exhausted.

#### Acceptance criteria

- **AC-UI-CONTEXT-WINDOW-RESET-FRESHNESS-001.1:** A successful agent-context reset invalidates the session's last context-window reading.
- **AC-UI-CONTEXT-WINDOW-RESET-FRESHNESS-001.2:** The context-window indicator stays hidden after reset until the fresh agent conversation reports a new usable reading.
- **AC-UI-CONTEXT-WINDOW-RESET-FRESHNESS-001.3:** The invalidation is reflected in persisted session metadata and propagated to subscribed clients, so the stale reading does not return after refresh or in another open client.
- **AC-UI-CONTEXT-WINDOW-RESET-FRESHNESS-001.4:** A new context-window report after reset renders normally and becomes the new persisted reading.
- **AC-UI-CONTEXT-WINDOW-RESET-FRESHNESS-001.5:** A failed reset retains the prior reading because the agent conversation was not replaced.
- **AC-UI-CONTEXT-WINDOW-RESET-FRESHNESS-001.6:** The reset action remains available only while the session is idle; running sessions continue to hide the action because the backend does not accept context resets while busy.
- **AC-UI-CONTEXT-WINDOW-RESET-FRESHNESS-001.7:** **GIVEN** an idle session displays a nearly-full context-window ring, **WHEN** the user successfully resets agent context, **THEN** the ring disappears before the user starts the next turn.
- **AC-UI-CONTEXT-WINDOW-RESET-FRESHNESS-001.8:** **GIVEN** a successful context reset has hidden the ring, **WHEN** the fresh agent conversation has not reported context usage, **THEN** the ring remains absent across session-state updates and page refresh.

## Migrated source detail

## Why

The context-window indicator can continue showing the previous conversation's nearly-full usage after the user resets an agent session. That reading describes context which no longer exists and makes a fresh conversation look almost exhausted.

## What

- A successful agent-context reset invalidates the session's last context-window reading.
- The context-window indicator stays hidden after reset until the fresh agent conversation reports a new usable reading.
- The invalidation is reflected in persisted session metadata and propagated to subscribed clients, so the stale reading does not return after refresh or in another open client.
- A new context-window report after reset renders normally and becomes the new persisted reading.
- A failed reset retains the prior reading because the agent conversation was not replaced.
- The reset action remains available only while the session is idle; running sessions continue to hide the action because the backend does not accept context resets while busy.

## Persistence guarantees

- A successful reset clears `task_sessions.metadata.context_window` before the reset flow returns the session to its idle state.
- Live frontend context-window state is a cache of agent-reported or persisted session metadata. An explicit cleared value invalidates that cache; absence of a fresh report must not be interpreted as zero usage.
- If the provider reset succeeds but metadata clearing fails, Kandev logs the persistence failure, keeps the reset response successful, and publishes an explicit in-memory clear so the initiating client still hides stale usage.

## Failure modes

- If the agent reset fails, Kandev restores the idle session state and leaves the previous context-window reading intact.
- If clearing persisted metadata fails after the provider conversation has already reset, Kandev does not attempt to undo the reset. It logs the persistence failure, keeps the reset response successful, and the initiating client still hides its stale cached reading.

## Scenarios

- **GIVEN** an idle session displays a nearly-full context-window ring, **WHEN** the user successfully resets agent context, **THEN** the ring disappears before the user starts the next turn.
- **GIVEN** a successful context reset has hidden the ring, **WHEN** the fresh agent conversation has not reported context usage, **THEN** the ring remains absent across session-state updates and page refresh.
- **GIVEN** a successful context reset has hidden the ring, **WHEN** the fresh agent conversation reports context-window usage, **THEN** the ring reappears using only that new reading.
- **GIVEN** an idle session displays context-window usage, **WHEN** its context reset fails, **THEN** the previous ring remains visible.
- **GIVEN** the provider reset succeeds but clearing persisted metadata fails, **WHEN** the reset settles, **THEN** Kandev logs the persistence failure, the initiating client hides stale usage, and the provider reset is not reported as failed.
- **GIVEN** the user sends a new message after reset, **WHEN** the session becomes busy, **THEN** the reset action is hidden until the session is idle again.

## Out of scope

- Allowing context reset while an agent turn is running.
- Changing the ring's visual design, thresholds, tooltip, or toolbar placement.
- Estimating fresh-session usage before the agent reports it.
