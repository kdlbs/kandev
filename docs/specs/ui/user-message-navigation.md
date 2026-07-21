---
status: shipped
created: 2026-07-21
owner: cfl
---

# User Message Navigation

## Why

Long agent turns can put hundreds of tool calls between a user's prompts. Users need to move between their own messages without repeatedly scrolling or manually loading every intervening history page.

## What

- Task-session chat provides a viewport-level vertical rail with up and down controls for moving between rendered prompts whose `author_type` is `user`.
- The rail replaces the left/right navigation controls currently repeated beneath individual user messages.
- Navigation uses the prompt nearest the viewport's vertical center as its origin. If a prompt spans the center, that prompt is the origin; after navigation, the destination remains the origin until the user scrolls elsewhere.
- Up selects the immediately previous user prompt in chronological order. When no previous prompt is loaded but older history may exist, one action loads successive existing 20-row pages until it finds a user prompt, reaches the confirmed start of history, or cannot make progress.
- Down selects the immediately next user prompt from loaded newer history and never requests an older page.
- A destination is centered when the scroll range allows it; at the oldest or newest boundary it settles at the nearest reachable edge. It receives a brief visual highlight. Reduced-motion preferences avoid smooth scrolling and animated emphasis.
- A control is disabled only at a known boundary. Up remains available while the server reports older history; down is disabled only when there is no newer user prompt.
- The existing **Load older messages** button remains available as a manual fallback.
- While a navigation request is pending, duplicate navigation is blocked and the rail exposes its busy state accessibly.

## Mobile Contract

- On fine-pointer desktop, the rail overlays the right edge of the chat viewport and reveals when the chat is hovered or contains focus. Keyboard focus also reveals the controls.
- On mobile and coarse-pointer devices, the rail stays visible whenever a session has a rendered user prompt and each control has a touch target of at least 44px in both dimensions.
- The mobile chat content keeps enough right-side clearance for the visible rail, including the safe-area inset, so controls do not cover message text or actions and do not create horizontal overflow.
- The chat message viewport remains the single vertical scroll owner. The rail is viewport-level UI and does not scroll with message rows.
- Native and Virtuoso renderers share navigation state and stop selection; only their scroll-to-destination adapters differ.
- Controls use up/down icons with accessible names `Previous user message` and `Next user message`; no required action depends on hover.

## Failure Modes

- If loading older history fails, navigation stays at the current position, exits its busy state, and does not treat the failure as the start of history. Up and **Load older messages** remain retry paths.
- If a page is empty or leaves the cursor unchanged while `has_more` remains true, that navigation action stops without looping and does not mark the boundary known.
- If the active session changes or the chat unmounts during pagination, the pending action is discarded and never scrolls or highlights the replacement session.
- If a virtualized destination cannot be mounted or resolved, the chat remains at its current position and no unrelated row is highlighted.
- Rapid repeated activation while pagination or destination mounting is pending starts at most one navigation action.

## Scenarios

- **GIVEN** loaded user prompts above and below the viewport, **WHEN** the user activates up or down, **THEN** the adjacent prompt in that direction is centered when possible and briefly highlighted.
- **GIVEN** the viewport is between user prompts after manual scrolling, **WHEN** navigation is activated, **THEN** it uses the user prompt nearest the viewport center as the origin.
- **GIVEN** hundreds of tool calls separate the current prompt from an older prompt outside loaded history, **WHEN** the user activates up once, **THEN** 20-row pages load until the older prompt is centered, reaches the nearest scroll boundary, or history is exhausted.
- **GIVEN** a newer user prompt is already loaded, **WHEN** the user activates down, **THEN** that prompt is centered or boundary-clamped without fetching another page.
- **GIVEN** the current prompt is the oldest loaded prompt and older history is still reported, **WHEN** the rail renders, **THEN** up remains enabled.
- **GIVEN** the current prompt is at a confirmed history boundary, **WHEN** the rail renders, **THEN** the corresponding directional control is disabled.
- **GIVEN** older-history loading fails or makes no progress, **WHEN** an up action completes, **THEN** the viewport stays in place and both retry paths remain available.
- **GIVEN** a fine-pointer desktop, **WHEN** the chat is hovered or the rail receives keyboard focus, **THEN** the vertical rail is visible and its controls are operable.
- **GIVEN** a coarse-pointer or mobile viewport, **WHEN** chat is open, **THEN** the rail is visible with 44px controls, safe-area clearance, readable messages, and no document horizontal overflow.
- **GIVEN** either native or Virtuoso rendering, **WHEN** the same navigation action is performed, **THEN** stop selection, pagination, centering, highlighting, and boundary behavior are equivalent.
- **GIVEN** the navigation rail is available, **WHEN** a user message row renders, **THEN** it does not show the old per-message left/right navigation buttons.
- **GIVEN** older history exists, **WHEN** navigation is unused or fails, **THEN** **Load older messages** remains available.

## Out of Scope

- Backend, database, or message-pagination API changes.
- Navigating agent, system, tool-call, hidden, or filtered-out messages.
- Keyboard shortcuts, a message minimap, deep links, or persisted navigation position.
- Replacing search, reverse prompt history, or the explicit **Load older messages** workflow.

## Implementation Plan

[User message navigation](../../plans/user-message-navigation/plan.md)
