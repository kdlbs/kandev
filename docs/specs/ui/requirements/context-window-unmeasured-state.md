---
status: active
system: ui
created: 2026-08-07
owners:
  - kandev
---
# Context Window Unmeasured State Requirements

## Overview

The ACP adapter can provide the context-window size before the first positive usage sample is available. While a turn is still in flight, the task chat can therefore show a context window with 0% and 0 of 1.0M tokens, even though the agent is already consuming context and the next completed usage update will replace those values. The numeric zero reads as a measured result rather than an unmeasured state.

## Requirements

### REQ-UI-CONTEXT-WINDOW-UNMEASURED-STATE-001: Context Window Unmeasured State

**Intent:** The ACP adapter can provide the context-window size before the first positive usage sample is available. While a turn is still in flight, the task chat can therefore show a context window with 0% and 0 of 1.0M tokens, even though the agent is already consuming context and the next completed usage update will replace those values. The numeric zero reads as a measured result rather than an unmeasured state.

#### Acceptance criteria

- **AC-UI-CONTEXT-WINDOW-UNMEASURED-STATE-001.1:** When a reliable context-window size is available and used is zero, keep the existing context ring available but render an explicit pending state:
- **AC-UI-CONTEXT-WINDOW-UNMEASURED-STATE-001.2:** the trigger has the translated accessible label "Context window: usage not measured";
- **AC-UI-CONTEXT-WINDOW-UNMEASURED-STATE-001.3:** the ring remains empty;
- **AC-UI-CONTEXT-WINDOW-UNMEASURED-STATE-001.4:** the tooltip shows "—%" and "— of <window size> tokens", never "0%" or "0 of ...";
- **AC-UI-CONTEXT-WINDOW-UNMEASURED-STATE-001.5:** the tooltip includes translated copy explaining that usage data appears after the first completed turn.
- **AC-UI-CONTEXT-WINDOW-UNMEASURED-STATE-001.6:** Preserve the existing source label, compaction row, context-window size formatting, and pinnable tooltip behavior while usage is pending.
- **AC-UI-CONTEXT-WINDOW-UNMEASURED-STATE-001.7:** When a positive usage sample arrives, return to the existing percentage and token-count presentation without changing its formatting or thresholds.
- **AC-UI-CONTEXT-WINDOW-UNMEASURED-STATE-001.8:** Continue hiding impossible reports where size <= 0 or used > size; an exactly full window remains valid.

## Migrated source detail

## Why

The ACP adapter can provide the context-window size before the first positive usage sample is
available. While a turn is still in flight, the task chat can therefore show a context window
with 0% and 0 of 1.0M tokens, even though the agent is already consuming context and the next
completed usage update will replace those values. The numeric zero reads as a measured result
rather than an unmeasured state.

## What

- When a reliable context-window size is available and used is zero, keep the existing context
  ring available but render an explicit pending state:
  - the trigger has the translated accessible label "Context window: usage not measured";
  - the ring remains empty;
  - the tooltip shows "—%" and "— of <window size> tokens", never "0%" or "0 of ...";
  - the tooltip includes translated copy explaining that usage data appears after the first
    completed turn.
- Preserve the existing source label, compaction row, context-window size formatting, and
  pinnable tooltip behavior while usage is pending.
- When a positive usage sample arrives, return to the existing percentage and token-count
  presentation without changing its formatting or thresholds.
- Continue hiding impossible reports where size <= 0 or used > size; an exactly full window
  remains valid.
- Route all new visible and accessible copy through the task translation namespace and keep the
  English and pseudo-locale catalogs synchronized.
- Keep the ACP, WebSocket, persistence, and session-runtime contracts unchanged. This is a
  presentation rule for the existing used === 0 state, not a new provider event or sample
  field.

## Scenarios

- **GIVEN** a session has a reliable ACP window size and no positive usage sample yet, **WHEN** a
  desktop user opens the context control, **THEN** the trigger and tooltip identify usage as not
  measured, show the em-dash values, retain the ACP source, and do not show a numeric zero for
  usage.
- **GIVEN** the same pending state on a phone viewport, **WHEN** the user taps the context ring,
  **THEN** the same pending content is reachable in the existing tap-pinned hover without a new
  drawer, route, or horizontal page overflow.
- **GIVEN** a pending context window, **WHEN** a positive ACP usage sample arrives, **THEN** the
  control updates to the current numeric percentage and token count.
- **GIVEN** a pending API-derived window, **WHEN** the user opens the control, **THEN** the
  pending presentation is identical except for the existing API source label.
- **GIVEN** a reliable non-zero sample, **WHEN** the control renders, **THEN** the current numeric
  percentage, progress bar, source, compaction count, and tooltip interactions remain unchanged.
- **GIVEN** an exact-full or impossible report, **WHEN** the control renders, **THEN** the existing
  full-window behavior or hidden behavior is preserved.

## Out of scope

- Changing ACP usage reporting, provider semantics, or the context-window size chosen by the
  adapter.
- Adding a persisted has_usage_sample field, changing WebSocket metadata, or changing session
  hydration.
- Changing the normal non-zero percentage/token formatting, color thresholds, source explanation,
  or compaction-count semantics.
- Adding polling, a spinner, a progress estimate, or a new context-window surface.
- Changing the existing tooltip's desktop hover, keyboard, touch-pinning, outside-dismissal, or
  Escape behavior.
