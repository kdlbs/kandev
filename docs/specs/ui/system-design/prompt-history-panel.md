---
status: draft
system: ui
requirements:
  - REQ-UI-PROMPT-HISTORY-PANEL-001
created: 2026-08-13
owners:
  - clem
---
# Prompt History Panel System Design

## Purpose and boundaries

This design preserves the technical source detail for `REQ-UI-PROMPT-HISTORY-PANEL-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-PROMPT-HISTORY-PANEL-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Why

Reviewing what was asked of an agent requires scrolling the transcript; past prompts are easy to lose among long agent replies, and there is no way to see at a glance how long the agent worked on each prompt. A compact per-task history of prompts with send time and agent-work duration fixes both.

## What

- A new optional panel, **Prompt history**, is available from the task workbench "+" menu (the `AddPanelMenuItems` dropdown) on the desktop workbench and the Office workbench, next to the existing Todos entry. On a phone, the same panel is available from the `Panels` bottom-navigation picker.
- Passthrough sessions: the "+" menu does not offer the panel (the same `!state.isPassthrough` guard the Plan and Todos rows use), because passthrough sessions render a toolbar instead of a transcript. Because the panel is a reusable, persisted layout panel, a tab can still be present after a layout restore or a session/task switch into a passthrough session; in that case the panel content renders a passthrough empty state with NO navigation arrows (the transcript the arrow would jump to does not exist), instead of a dead control or a hidden-but-broken list.
- The panel lists the prompts of the task's active session, newest first. A prompt is a message with `author_type === "user"`. Prompt history owns an independent user-message projection and pagination cursor, so it can load prompts without loading every transcript row. The panel always reflects the task's active session: when the active session changes (session tab click, session dropdown, or automatic handoff), it discards the previous projection and loads the new session. A task with multiple agent transcripts therefore shows exactly one session's prompts at a time — it never merges sessions — and switching the active session swaps the whole list. A task with no active session, or an active session whose prompt entries remain empty after loading and pagination are exhausted, shows the empty state; during initial loading it shows the neutral loading state.
- Each row shows, on one line:
  - a clickable arrow button on the left;
  - a small `#N` number label at the start of the prompt bubble, in front of the prompt text, where `N` is the prompt's 1-based ordinal among ALL user messages of its session ordered by `created_at` ascending with ties broken by message `id` ascending (`#1` is the very first prompt of the whole session, and the newest prompt shows the highest number). The number is small type (`text-[10px]`-scale), distinct from the prompt text, and remains visible when the row is expanded. A prompt whose message carries no ordinal (older payloads, transient WS frames) renders without a number. Prompt numbers appear only in this panel, never in the transcript;
  - the prompt text truncated to a single line with an ellipsis (CSS truncation, so the visible character count adapts to panel width);
  - the time the prompt was sent, in compact relative form (`5m ago`, `3h ago`) with the absolute time in a `title` attribute;
  - the duration of agent work that followed the prompt (see Duration rules);
  - an expand/collapse chevron button on the right, visible when the collapsed text overflows the single line (horizontal overflow: `scrollWidth > clientWidth` of the collapsed text element) OR when the row is currently expanded — visibility is `hasCollapsedOverflow || expanded`, so the only collapse control never disappears the moment it is used (the expanded text wraps and would no longer report horizontal overflow).
- Pagination follows the transcript's auto-load pattern, not a button: when the user scrolls the panel down and the last rendered prompt is not the session's first prompt (the session has older prompts the panel has not loaded yet), the panel automatically loads the next older page. Each auto-load accumulates at least 10 new user prompts (message pages contain agent replies too, so the panel keeps fetching pages until the threshold, pagination exhaustion, a zero-result page, or a page cap). The sentinel and its loading indicator are present only while message pagination reports `has_more` AND no loaded entry has `promptNumber === 1`; if payloads omit ordinals, they fall back to `has_more` exhaustion. The loading indicator renders the `loadingOlderMessages` copy and its placement depends on whether the panel content scrolls, measured from the prompt rows alone (never from the indicator itself):

- When the prompt rows overflow the panel (the panel actually scrolls), the indicator is a floating message at the bottom of the panel: absolutely positioned within the panel root and out of the content flow, so its appearance and removal cannot change the scrollable content height, the sentinel's geometry, or the row positions.
- When the prompt rows fit (the panel does not scroll), the indicator renders in the content flow directly under the last message.

The panel root is a positioned outer wrapper whose scrollable content lives in a distinct inner scroller: the floating message anchors to the outer wrapper (the panel viewport) and is a sibling of the scroller, so it never scrolls with the content (an absolutely positioned child of a scroll container moves with its content). In both modes the sentinel stays the final in-flow child of the content wrapper (the indicator is either a sibling after the wrapper or an out-of-flow overlay, never an in-flow element before the sentinel), so the sentinel's geometry is stable while loading. While the user is pinned at the panel's bottom, a completed load scrolls the scroller back to the new bottom so the sentinel stays in view and the next older page loads without requiring a scroll-away/scroll-back; scrolling away from the bottom cancels the stick. The indicator stays mounted for a short grace window after each page settles so consecutive auto-loads (the sentinel re-arms while still intersecting) render one continuous indicator instead of a per-page flash; it disappears once a settle is not followed by another load within the window or pagination ends. A rejected or zero-result request disarms the sentinel until it observes the user scroll away and back; when short content prevents that false transition, a wheel/touch gesture on the panel root retries through the same user-gesture path. Loading stops automatically once the first prompt of the session is reached. Prompt history and transcript pagination have separate requests and metadata; each consumer serializes its own cursor, while WebSocket events fan out to both caches. The panel never renders a load-more button.
- The expand button behaves like the anchored last-prompt bar: expanded rows show the full prompt untruncated (wrapping text) inside the row, capped at 40 % of the panel's height, with in-box scrolling. The cap is measured from the panel's own root element (the component's container, via a ref), not from any transcript selector; it re-measures on panel resize and falls back to 40 % of the viewport height when no container can be measured. The chevron toggles `aria-expanded` and shows the collapse state.
- Clicking a row's arrow button navigates to that prompt in the transcript panel, the same way the scroll-to-last-prompt button navigates to the last prompt: the session's chat panel is activated (opened if absent, focused if present) and the transcript scrolls so the prompt message's top is aligned to the top of the transcript viewport, or to the nearest reachable position when the scroll range is shorter.
- A session with no user prompts shows a neutral loading state while its initial message request is in flight, and shows the definitive empty state only once `entries` is empty, `has_more` is false, and loading has ended; while older pages remain (`has_more`), the panel remains paginatable instead of declaring the session empty. When entries are empty but an older-page request is in flight (`isLoadingMore`), older-page loading takes precedence: the panel shows the loading message in the content flow (an empty panel has nothing to scroll, so it never floats), not the neutral initial loading text or the empty state.

### Duration rules

For each prompt message `M`, the agent-work duration is the wall-clock time from `M.created_at` until the earlier of:

1. the completion of the turn that `M` belongs to (`turn_id` → `Turn.completed_at`), or
2. the send time of the next prompt message (`created_at` of the next user message in the session).

Duration is displayed only when such an end time exists, rounded down to whole seconds, clamped at zero. The last prompt (newest user message) shows a duration only when its turn is already completed; while the agent is still working on it, no duration is shown. A prompt whose `turn_id` is absent shows a duration only when a later prompt bounds it.

Robustness rules: a prompt whose `created_at` is not parseable as a timestamp is excluded from the list (its ordering position is undefined). The "next prompt" bound is defined over the filtered prompt list only: after exclusion and sorting, the next prompt is the immediately following entry of the same session — so an unparseable user message can never become a next-prompt candidate, and a valid prompt followed by an invalid user message is bounded by the next *valid* prompt after it. Unparseable `Turn.completed_at` is treated as absent rather than producing `NaN`. Ordering is deterministic: prompts sort by `created_at` ascending, ties broken by `message id` ascending (entries are then displayed newest first). All associations are session-scoped: a turn only bounds a prompt when both share the same `session_id` (matched by `turn_id`), and only the next prompt of the same session bounds the duration.

Prompt history derives entries from its own user-message projection. The projection is ordered by the backend's normalized microsecond timestamp and message ID, and older pages are prepended. The "next prompt" bound is evaluated over the prompts currently in this projection. The transcript cache remains a separate mixed-author projection and can contain rows that prompt history has not loaded yet.

## State ownership and lifecycle

Zustand owns `messagePrompts.bySession` and its per-session pagination metadata. `useSessionPrompts` owns the initial request lifecycle, while the older-page hook owns its cursor request. A per-session generation changes when a session is removed; a response can merge only when its generation still matches. WebSocket create, update, and delete handlers reconcile both the transcript and prompt projections. Timestamp comparison uses the full RFC3339Nano value, with the same microsecond ordering as the backend.

## Prompt navigation

Selecting a row creates a session-scoped jump target and activates Chat. Chat first searches the loaded transcript. If the target is absent, it requests `GET /messages?around=<message-id>`, merges the target and newer rows, and retries after the row renders. Desktop and phone clear the target only after a successful scroll or a confirmed missing target. A request cannot consume a newer target or a target for another session.

## API surface

The message JSON contract (`v1.Message`, served by the paginated `GET /api/v1/task-sessions/:id/messages` requests used by the web store (`limit`, `before`, `after`, or `sort`), the task-detail boot payload, and the `session.message.added` / `session.message.updated` WebSocket events) gains one optional field. The legacy unpaginated `GET /api/v1/task-sessions/:id/messages` response remains unchanged and may omit `prompt_index`.

`prompt_index` — `number | undefined`, present only on messages with `author_type === "user"` in payloads produced by a server implementing this contract. It is the 1-based ordinal of the message among all user messages of its own session, ordered by `created_at` ascending with ties broken by `id` ascending — the same ordering the panel uses to display entries. It is stable across pagination: a prompt keeps its `prompt_index` no matter which pages are loaded, and newly arriving prompts (WebSocket or refetch) carry their own index. Payloads produced by older servers may omit the field even for historical user messages; if the client has no previously known ordinal for that message, the panel renders the row without a number. Once a valid ordinal is known, client reconciliation retains it across later payloads that omit the optional field.

Live user-message creation assigns the ordering timestamp and `prompt_index` under one per-session write boundary before publishing the WebSocket event. Concurrent user creates therefore cannot publish duplicate or stale live ordinals; HTTP/refetch remains the authoritative repair path for older or externally seeded rows.

The field is persisted per user message at creation: `prompt_seq` is allocated from a per-session monotonic sequence inside the same write boundary that assigns the ordering timestamp, so ordinals are stable across pagination, message deletion, and clock corrections. Existing databases gain the column through an idempotent migration that backfills historical rows with their previously derived ordinal and seeds each session's counter.

## Out of scope

- The phone surface has no workbench "+" menu, so it exposes Prompt history from the grouped `Panels` bottom-navigation picker. The panel uses the same rows and navigation callback as desktop. Selecting an arrow returns to Chat; if needed, Chat loads an around window before it scrolls to the chosen prompt. The mobile flow is covered by `apps/web/e2e/tests/task/mobile-prompt-history-panel.spec.ts`.
- Prompt numbers in the transcript: the `#N` label appears only in the prompt-history panel, never next to prompts in the chat message list.
- A load-more button in the panel: older prompts load automatically on scroll; no button is offered.
- The transcript's own loading-older indicator (top-of-list, in-flow): this change applies to the prompt-history panel only.
- Editing, re-sending, or reordering prompts.
- Queued-but-not-yet-sent messages (they live in the message queue, not the transcript).
- Per-prompt model, token, or cost statistics.

## Scenarios

- **GIVEN** a task whose active session has user prompts, **WHEN** the user opens Prompt history from the "+" menu, **THEN** the panel lists the prompts newest first, each as one truncated line with its send time, its agent-work duration, and arrow/expand controls.
- **GIVEN** a prompt longer than one panel line, **WHEN** the panel renders it collapsed, **THEN** the text is truncated with an ellipsis and the expand chevron is visible; **WHEN** the user clicks the chevron, **THEN** the full prompt appears in a scrollable area whose height is capped at 40 % of the panel height, and the chevron flips to the collapse state; clicking again collapses.
- **GIVEN** a prompt whose turn completed, **WHEN** the panel renders it, **THEN** the row shows the duration from prompt send until turn completion.
- **GIVEN** a prompt that was followed by a later prompt before its turn completed, **WHEN** the panel renders it, **THEN** the row shows the duration from prompt send until the later prompt's send time.
- **GIVEN** the last prompt whose turn is still running, **WHEN** the panel renders it, **THEN** the row shows no duration.
- **GIVEN** two prompts that share one turn, **WHEN** the panel renders them, **THEN** the earlier prompt's duration runs to the later prompt's send time (the turn-completion bound only applies where it is earlier).
- **GIVEN** a prompt with an unparseable `created_at`, **WHEN** the panel renders, **THEN** the prompt is absent from the list.
- **GIVEN** a valid prompt followed by an unparseable user message followed by another valid prompt, **WHEN** the panel renders, **THEN** the invalid message is absent and the first prompt's duration runs to the later valid prompt's send time.
- **GIVEN** a prompt and a turn with identical timestamps, **WHEN** the panel renders, **THEN** ordering is deterministic (ties broken by message id) and the duration is clamped to at least 0 seconds.
- **GIVEN** a prompt row in the panel, **WHEN** the user clicks its arrow button, **THEN** the session chat panel becomes active and the transcript scrolls to the prompt's top position, or the nearest reachable position when the scroll range is shorter.
- **GIVEN** a session with no user prompts in the currently loaded page but with `has_more` older messages, **WHEN** the panel opens, **THEN** it keeps the panel in a loading/paginatable state and automatically discovers older prompts; it shows the definitive empty state only after pagination is exhausted.
- **GIVEN** the active session changes before its first message request completes, **WHEN** the new session has no entries and metadata still says `has_more=false` while `isLoading=true`, **THEN** the panel shows the neutral loading state rather than the definitive empty state.
- **GIVEN** an older-page request rejects or returns zero rows while `has_more` remains true, **WHEN** the user remains at the bottom, **THEN** no immediate retry occurs; **WHEN** the user scrolls away and back, or performs a wheel/touch gesture while the sentinel cannot exit because the content is shorter than the root, **THEN** the next eligible request retries the retained cursor.
- **GIVEN** the panel is paginating older pages with an older-page request in flight, **WHEN** the prompt rows overflow the panel, **THEN** the loading message renders as a floating overlay at the bottom of the panel and the panel's content layout (scroll height, sentinel geometry, row positions) is unchanged by its presence; **WHEN** the prompt rows fit the panel, **THEN** the loading message renders in the content flow directly under the last message; **WHEN** the request settles and no further load starts within the grace window, **THEN** the message disappears.
- **GIVEN** the panel is auto-loading consecutive older pages, **WHEN** one page settles and the next starts within the grace window, **THEN** the loading message stays mounted throughout (no per-page flash).
- **GIVEN** a session with no loaded entries while an older-page request is in flight, **WHEN** the panel renders, **THEN** it shows the loading message in the content flow (not the neutral initial loading text or the empty state) and remains paginatable.
- **GIVEN** the transcript and Prompt history are both visible for one session and both older-message sentinels intersect in the same turn, **WHEN** pagination begins, **THEN** each consumer sends at most one request for its own cursor and both projections reconcile later WebSocket changes.
- **GIVEN** a task with multiple agent transcripts and the panel open, **WHEN** the active session switches (session tab, session dropdown, or automatic handoff), **THEN** the panel re-renders the newly active session's prompts newest first and no prompt from the previous session remains; while the new session's turns are still hydrating, its rows show no durations.
- **GIVEN** the panel open with a long prompt list, **WHEN** the user resizes the panel narrower, **THEN** each row's truncated text re-truncates to the new width (no horizontal overflow).
- **GIVEN** a session with `N` user messages, **WHEN** the panel renders, **THEN** the newest prompt row shows `#N`, the oldest shows `#1`, and every row's number matches its position in the session-wide user-message order (ties broken by message id).
- **GIVEN** a user prompt sent by another task's agent (has `sender_task_id` metadata), **WHEN** the panel renders it, **THEN** the row shows its `#N` label (it counts in the session's prompt numbering) with the robot icon.
- **GIVEN** only part of a long session's messages loaded (older pages not yet fetched), **WHEN** the panel renders, **THEN** each loaded row still shows its absolute session-wide number (e.g. the oldest loaded prompt does not restart at `#1`), and the numbers do not change as more pages load.
- **GIVEN** a prompt whose message payload lacks `prompt_index` and has no previously known ordinal, **WHEN** the panel renders it, **THEN** the row shows the prompt without a number label.

## Open questions

None.
