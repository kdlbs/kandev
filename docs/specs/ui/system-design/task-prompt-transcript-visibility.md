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
| `REQ-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001` | [Opening boundary](#opening-boundary), [Pagination boundary](#pagination-boundary), [Failure and compatibility](#failure-and-compatibility) |

## Components and responsibilities

- `useLazyLoadMessages` owns the visible pagination boundary for transcript consumers.
- The session message fetch owns one bounded initial request for the newest message window.
- `messages.metaBySession[sessionId].hasMore` keeps the raw backend `has_more` value.
- `messages.bySession[sessionId]` supplies the loaded messages and their prompt ordinals.
- The native transcript and Prompt History panel consume the shared hook result.
- Transcript navigation uses only the messages that the transcript already loaded.
- The hook also exposes an explicit raw-pagination loader for direct recovery consumers, such as session search.
- `requestOlderMessages` keeps request coordination and raw cursor metadata unchanged.

The Prompt History panel already uses prompt `#1` as its terminal boundary. The transcript uses the same boundary through the shared hook.

## Opening boundary

The session message fetch requests only the newest bounded window when a task opens. A tool-only window does not start an automatic backfill.

`TaskChatPanel` does not load older pages to find the last prompt. The last-prompt action becomes available after the prompt enters the loaded window.

If the newest window has no user message, the transcript shows the task-description fallback. Upward navigation then loads older pages through visible pagination.

## Pagination boundary

The frontend derives a visible `hasMore` value from two inputs:

1. The raw backend value is true.
2. No loaded user message has `prompt_index` equal to `1`.

If both conditions are true, transcript consumers can request another page. If prompt `#1` is loaded, the shared hook reports no visible older history.

The hook applies this rule to its reactive return value. It also applies the rule after each joined or completed request.

This immediate update stops one multi-page load operation at prompt `#1`. It prevents an extra request for pre-prompt internal rows.

The store keeps the raw backend value. Direct recovery code can use the hook's explicit raw-pagination loader when its contract requires the complete message stream. Session search uses this path because backend search can return a hit from before prompt `#1`. Transcript rendering and navigation continue to use visible pagination, so this recovery path does not restore the older-page control.

## Control flow

1. The initial message request stores one newest suffix and raw pagination metadata.
2. The shared hook reports visible older history while prompt `#1` is absent.
3. The user reaches the oldest loaded point.
4. An older-page request prepends messages through the existing coordinator.
5. The native transcript preserves a stable message-row position across the complete request.
6. If the page contains prompt `#1`, the shared hook changes its visible value to false.
7. The transcript removes its sentinel, loading state, and older-page control.
8. Explicit reverse search uses raw pagination when it must backfill a backend search hit.

## Failure and compatibility

If a request fails before prompt `#1` is known, the explicit older-page control remains available. A zero-result response keeps the existing retry behavior.

An initial tool-only window completes without an older-page request. The task-description fallback remains visible until upward pagination loads a stored user prompt.

Older payloads can omit `prompt_index`. In this case, the shared hook uses raw `has_more` exhaustion as the compatibility fallback.

## Responsive behavior

Desktop and mobile use the same native transcript and one vertical scroll owner. This change adds no mobile surface, control, or touch behavior.

The existing full-height mobile task layout remains the nearest mobile exemplar. The mobile pagination scenario proves the same visible start.

## Test boundaries

- A hook test covers raw `has_more: true` with loaded prompt `#1`.
- A hook test covers the compatibility path when the ordinal is absent.
- A hook test proves that a multi-page load stops when a response adds prompt `#1`.
- Hook and drain tests prove that explicit search backfill can continue through the raw boundary.
- Desktop and mobile browser tests seed hidden pre-prompt rows and prove that no older-page control remains.
- Desktop and mobile browser tests prove that task opening makes no request with an older-page cursor.
- A message-fetch test proves that a tool-only initial window completes without automatic backfill.
- Separate desktop and mobile browser tests preserve the prepend anchor while loading older pages.

## Related design

- [Prompt History Panel](prompt-history-panel.md) defines the `prompt_index` contract and its compatibility behavior.
