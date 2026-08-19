---
status: draft
created: 2026-08-13
owner: clem
---

# Prompt History Panel

## Why

Reviewing what was asked of an agent requires scrolling the transcript; past prompts are easy to lose among long agent replies, and there is no way to see at a glance how long the agent worked on each prompt. A compact per-task history of prompts with send time and agent-work duration fixes both.

## What

- A new optional panel, **Prompt history**, is available from the task workbench "+" menu (the `AddPanelMenuItems` dropdown) on the desktop workbench and the Office workbench, next to the existing Todos entry. On a phone, the same panel is available from the `Panels` bottom-navigation picker.
- Passthrough sessions: the "+" menu does not offer the panel (the same `!state.isPassthrough` guard the Plan and Todos rows use), because passthrough sessions render a toolbar instead of a transcript. Because the panel is a reusable, persisted layout panel, a tab can still be present after a layout restore or a session/task switch into a passthrough session; in that case the panel content renders a passthrough empty state with NO navigation arrows (the transcript the arrow would jump to does not exist), instead of a dead control or a hidden-but-broken list.
- The panel lists the prompts of the task's active session, newest first. A prompt is a message with `author_type === "user"` in the session's loaded transcript messages (same definition the transcript's scroll-to-last-prompt affordance uses). The panel always reflects the task's active session: when the active session changes (session tab click, session dropdown, or automatic handoff), the panel re-derives its list from the newly active session's transcript. A task with multiple agent transcripts therefore shows exactly one session's prompts at a time — it never merges sessions — and switching the active session swaps the whole list. A task with no active session, or an active session with no user prompts, shows the empty state.
- Each row shows, on one line:
  - a clickable arrow button on the left;
  - the prompt text truncated to a single line with an ellipsis (CSS truncation, so the visible character count adapts to panel width);
  - the time the prompt was sent, in compact relative form (`5m ago`, `3h ago`) with the absolute time in a `title` attribute;
  - the duration of agent work that followed the prompt (see Duration rules);
  - an expand/collapse chevron button on the right, visible when the collapsed text overflows the single line (horizontal overflow: `scrollWidth > clientWidth` of the collapsed text element) OR when the row is currently expanded — visibility is `hasCollapsedOverflow || expanded`, so the only collapse control never disappears the moment it is used (the expanded text wraps and would no longer report horizontal overflow).
- The expand button behaves like the anchored last-prompt bar: expanded rows show the full prompt untruncated (wrapping text) inside the row, capped at 40 % of the panel's height, with in-box scrolling. The cap is measured from the panel's own root element (the component's container, via a ref), not from any transcript selector; it re-measures on panel resize and falls back to 40 % of the viewport height when no container can be measured. The chevron toggles `aria-expanded` and shows the collapse state.
- Clicking a row's arrow button navigates to that prompt in the transcript panel, the same way the scroll-to-last-prompt button navigates to the last prompt: the session's chat panel is activated (opened if absent, focused if present) and the transcript scrolls so the prompt message's top is aligned to the top of the transcript viewport, or to the nearest reachable position when the scroll range is shorter.
- A session with no user prompts shows an empty state.

### Duration rules

For each prompt message `M`, the agent-work duration is the wall-clock time from `M.created_at` until the earlier of:

1. the completion of the turn that `M` belongs to (`turn_id` → `Turn.completed_at`), or
2. the send time of the next prompt message (`created_at` of the next user message in the session).

Duration is displayed only when such an end time exists, rounded down to whole seconds, clamped at zero. The last prompt (newest user message) shows a duration only when its turn is already completed; while the agent is still working on it, no duration is shown. A prompt whose `turn_id` is absent shows a duration only when a later prompt bounds it.

Robustness rules: a prompt whose `created_at` is not parseable as a timestamp is excluded from the list (its ordering position is undefined). The "next prompt" bound is defined over the filtered prompt list only: after exclusion and sorting, the next prompt is the immediately following entry of the same session — so an unparseable user message can never become a next-prompt candidate, and a valid prompt followed by an invalid user message is bounded by the next *valid* prompt after it. Unparseable `Turn.completed_at` is treated as absent rather than producing `NaN`. Ordering is deterministic: prompts sort by `created_at` ascending, ties broken by `message id` ascending (entries are then displayed newest first). All associations are session-scoped: a turn only bounds a prompt when both share the same `session_id` (matched by `turn_id`), and only the next prompt of the same session bounds the duration.

## Out of scope

- The phone surface has no workbench "+" menu, so it exposes Prompt history from the grouped `Panels` bottom-navigation picker. The panel uses the same rows and navigation callback as desktop. Selecting an arrow returns to Chat and scrolls to the chosen prompt. The mobile flow is covered by `apps/web/e2e/tests/task/mobile-prompt-history-panel.spec.ts`.
- Loading older prompts beyond the messages the transcript has already loaded (pagination follows the transcript's existing load-more behavior).
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
- **GIVEN** a session with no user prompts, **WHEN** the panel renders, **THEN** an empty state is shown instead of rows.
- **GIVEN** a task with multiple agent transcripts and the panel open, **WHEN** the active session switches (session tab, session dropdown, or automatic handoff), **THEN** the panel re-renders the newly active session's prompts newest first and no prompt from the previous session remains; while the new session's turns are still hydrating, its rows show no durations.
- **GIVEN** the panel open with a long prompt list, **WHEN** the user resizes the panel narrower, **THEN** each row's truncated text re-truncates to the new width (no horizontal overflow).

## Open questions

None.
