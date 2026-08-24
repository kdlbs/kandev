---
status: current
system: ui
requirements:
  - REQ-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001
created: 2026-08-24
owners:
  - kandev
---

# Task Transcript History Visibility System Design

## Purpose and boundaries

This design defines the visible start of a task transcript. The first user prompt is the visible start, even when internal rows exist before it.

The design changes no backend query, message order, persistence rule, or API field. It uses the existing `prompt_index` message field.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001` | [Pagination boundary](#pagination-boundary), [Failure and compatibility](#failure-and-compatibility) |

## Components and responsibilities

- `useLazyLoadMessages` owns the visible pagination boundary for transcript consumers.
- `messages.metaBySession[sessionId].hasMore` keeps the raw backend `has_more` value.
- `messages.bySession[sessionId]` supplies the loaded messages and their prompt ordinals.
- The native transcript, Prompt History panel, and transcript navigation consume the shared hook result.
- `requestOlderMessages` keeps request coordination and raw cursor metadata unchanged.

The Prompt History panel already uses prompt `#1` as its terminal boundary. The transcript uses the same boundary through the shared hook.

## Pagination boundary

The frontend derives a visible `hasMore` value from two inputs:

1. The raw backend value is true.
2. No loaded user message has `prompt_index` equal to `1`.

If both conditions are true, transcript consumers can request another page. If prompt `#1` is loaded, the shared hook reports no visible older history.

The hook applies this rule to its reactive return value. It also applies the rule after each joined or completed request.

This immediate update stops one multi-page load operation at prompt `#1`. It prevents an extra request for pre-prompt internal rows.

The store keeps the raw backend value. Direct recovery code can still inspect the complete message stream when its contract requires raw pagination.

## Control flow

1. The initial message request stores a newest suffix and raw pagination metadata.
2. The shared hook reports visible older history while prompt `#1` is absent.
3. An older-page request prepends messages through the existing coordinator.
4. If the page contains prompt `#1`, the shared hook changes its visible value to false.
5. The transcript removes its sentinel, loading state, and older-page control.
6. Transcript navigation treats prompt `#1` as the start and does not drain pre-prompt rows.

## Failure and compatibility

If a request fails before prompt `#1` is known, the explicit older-page control remains available. A zero-result response keeps the existing retry behavior.

Older payloads can omit `prompt_index`. In this case, the shared hook uses raw `has_more` exhaustion as the compatibility fallback.

## Responsive behavior

Desktop and mobile use the same native transcript and one vertical scroll owner. This change adds no mobile surface, control, or touch behavior.

The existing full-height mobile task layout remains the nearest mobile exemplar. The mobile pagination scenario proves the same visible start.

## Test boundaries

- A hook test covers raw `has_more: true` with loaded prompt `#1`.
- A hook test covers the compatibility path when the ordinal is absent.
- A hook test proves that a multi-page load stops when a response adds prompt `#1`.
- Desktop and mobile browser tests seed hidden pre-prompt rows and prove that no older-page control remains.

## Related design

- [Prompt History Panel](prompt-history-panel.md) defines the `prompt_index` contract and its compatibility behavior.
