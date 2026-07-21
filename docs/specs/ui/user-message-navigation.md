---
status: shipped
created: 2026-07-21
owner: cfl
---

# User Message Navigation

## Why

Long agent turns can put hundreds of tool calls between a user's prompts. Users need to move between their own messages without repeatedly scrolling or manually loading every intervening history page.

## What

- Each rendered prompt whose `author_type` is `user` provides up and down controls in its existing message action row.
- Navigation uses the user prompt that owns the activated action as its origin. It does not infer an origin from the viewport position.
- Up selects the immediately previous user prompt in chronological order. When no previous prompt is loaded but older history may exist, one action loads successive existing 20-row pages until it finds a user prompt, reaches the confirmed start of history, or cannot make progress.
- Down selects the immediately next user prompt from loaded newer history and never requests an older page.
- A destination is centered when the scroll range allows it; at the oldest or newest boundary it settles at the nearest reachable edge. It receives a brief, low-contrast outline that fades without changing its background or layout. Reduced-motion preferences avoid smooth scrolling and show the outline without animation.
- A control is disabled only at a known boundary. Up remains available while the server reports older history; down is disabled only when there is no newer user prompt.
- The existing **Load older messages** button remains available as a manual fallback.
- While a navigation request is pending, duplicate navigation is blocked and user-message navigation actions expose their busy state accessibly.

## Mobile Contract

- On fine-pointer desktop, navigation icons follow the existing message-action disclosure: they appear when the user prompt is hovered or contains keyboard focus.
- On mobile and coarse-pointer devices, user-message actions remain visible without hover and each navigation control has a touch target of at least 44px in both dimensions.
- Navigation controls participate in the message action row and do not overlay prompt content or require viewport-level clearance.
- The chat message viewport remains the single vertical scroll owner. Navigation actions scroll with the user prompt that owns them.
- Native and Virtuoso renderers share navigation state and stop selection; only their scroll-to-destination adapters differ.
- Controls use up/down icons with accessible names `Previous user message` and `Next user message`; keyboard focus reveals them on desktop and no required action depends on hover on touch devices.

## Failure Modes

- If loading older history fails, navigation stays at the current position, exits its busy state, and does not treat the failure as the start of history. Up and **Load older messages** remain retry paths.
- If a page is empty or leaves the cursor unchanged while `has_more` remains true, that navigation action stops without looping and does not mark the boundary known.
- If the active session changes or the chat unmounts during pagination, the pending action is discarded and never scrolls or highlights the replacement session.
- If a virtualized destination cannot be mounted or resolved, the chat remains at its current position and no unrelated row is highlighted.
- Rapid repeated activation while pagination or destination mounting is pending starts at most one navigation action.

## Scenarios

- **GIVEN** loaded user prompts above and below the viewport, **WHEN** the user activates up or down, **THEN** the adjacent prompt in that direction is centered when possible and briefly highlighted.
- **GIVEN** multiple user prompts are visible, **WHEN** navigation is activated from one prompt's action row, **THEN** that prompt is the origin regardless of viewport position.
- **GIVEN** hundreds of tool calls separate the current prompt from an older prompt outside loaded history, **WHEN** the user activates up once, **THEN** 20-row pages load until the older prompt is centered, reaches the nearest scroll boundary, or history is exhausted.
- **GIVEN** a newer user prompt is already loaded, **WHEN** the user activates down, **THEN** that prompt is centered or boundary-clamped without fetching another page.
- **GIVEN** a prompt is the oldest loaded prompt and older history is still reported, **WHEN** its actions render, **THEN** up remains enabled.
- **GIVEN** a prompt is at a confirmed history boundary, **WHEN** its actions render, **THEN** the corresponding directional control is disabled.
- **GIVEN** older-history loading fails or makes no progress, **WHEN** an up action completes, **THEN** the viewport stays in place and both retry paths remain available.
- **GIVEN** a fine-pointer desktop, **WHEN** a user prompt is hovered or its action row receives keyboard focus, **THEN** its navigation controls are visible and operable.
- **GIVEN** a coarse-pointer or mobile viewport, **WHEN** a user prompt is visible, **THEN** its navigation controls are directly visible with 44px touch targets, readable messages, and no document horizontal overflow.
- **GIVEN** either native or Virtuoso rendering, **WHEN** the same navigation action is performed, **THEN** stop selection, pagination, centering, highlighting, and boundary behavior are equivalent.
- **GIVEN** a user message row renders, **WHEN** its actions are available, **THEN** it shows up/down navigation icons alongside copy, raw, metadata, model, and timestamp actions rather than a floating viewport rail.
- **GIVEN** navigation reaches a destination, **WHEN** visual feedback is shown, **THEN** a subtle outline fades around the prompt without a yellow background fill or layout shift.
- **GIVEN** older history exists, **WHEN** navigation is unused or fails, **THEN** **Load older messages** remains available.

## Out of Scope

- Backend, database, or message-pagination API changes.
- Navigating agent, system, tool-call, hidden, or filtered-out messages.
- Keyboard shortcuts, a message minimap, deep links, or persisted navigation position.
- Replacing search, reverse prompt history, or the explicit **Load older messages** workflow.

## Implementation Plan

[User message navigation](../../plans/user-message-navigation/plan.md)
