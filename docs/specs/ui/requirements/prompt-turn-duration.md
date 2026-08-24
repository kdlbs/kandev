---
status: draft
system: ui
created: 2026-08-20
owners:
  - clem
---
# Prompt Turn Duration on Message Hover Requirements

## Overview

Reviewing a transcript means hovering a prompt to copy it or see its metadata; the time the agent spent on that prompt is only visible in the Prompt history panel, which requires opening it. Showing the completed turn's duration directly in the per-message hover row puts the same information where the reviewer already is.

## Requirements

### REQ-UI-PROMPT-TURN-DURATION-001: Prompt Turn Duration on Message Hover

**Intent:** Reviewing a transcript means hovering a prompt to copy it or see its metadata; the time the agent spent on that prompt is only visible in the Prompt history panel, which requires opening it. Showing the completed turn's duration directly in the per-message hover row puts the same information where the reviewer already is.

#### Acceptance criteria

- **AC-UI-PROMPT-TURN-DURATION-001.1:** The existing action row of a **user prompt** message (the `MessageActions` row: copy, raw toggle, navigation, metadata, timestamp) additionally shows the duration of the turn that the prompt produced, when that turn has completed.
- **AC-UI-PROMPT-TURN-DURATION-001.2:** The duration renders in the same style as the Prompt history panel: an hourglass icon (`IconHourglass`, `h-3 w-3`, `aria-hidden`) followed by the compact duration text produced by `formatPromptDuration` (`0s`, `5s`, `5m 23s`, `1h 2m 3s`), localized with the existing `task:durationUnitSeconds` / `task:durationUnitMinutes` / `task:durationUnitHours` keys. No new copy, no translation changes.
- **AC-UI-PROMPT-TURN-DURATION-001.3:** The duration is the wall-clock time from the prompt's `created_at` until its turn's `completed_at`, rounded down to whole seconds and clamped at zero. The timestamp parsing, flooring, and clamping arithmetic — and the compact `formatPromptDuration` display — are the same as the Prompt history panel's (`docs/specs/ui/requirements/prompt-history-panel.md`, "Duration rules"). The end bound is the turn's completion only: the panel's additional "earlier of turn completion and next prompt" bound does NOT apply here, because this row shows the duration of the turn that resulted from this prompt and only after that turn has completed. The duration appears only when both timestamps are present and parseable.
- **AC-UI-PROMPT-TURN-DURATION-001.4:** The duration is hidden while the turn is still running and never appears for agent messages (their rows are not prompts and would otherwise show a meaningless partial duration). Absent duration renders nothing — no placeholder, no layout stub beyond the row's normal empty state.
- **AC-UI-PROMPT-TURN-DURATION-001.5:** The duration is part of the existing action row. On fine-pointer layouts at or above `sm`, it follows the row's hover/focus reveal. On coarse-pointer layouts, it stays visible so touch users can reach it. It adds no new interaction surface.
- **AC-UI-PROMPT-TURN-DURATION-001.6:** **GIVEN** a user prompt message whose `turn_id` resolves to a turn with a parseable `completed_at`, **WHEN** the message's hover row renders, **THEN** the row shows the hourglass icon followed by the duration from the prompt's `created_at` to the turn's `completed_at`, floored to whole seconds.
- **AC-UI-PROMPT-TURN-DURATION-001.7:** **GIVEN** a user prompt message whose turn has no `completed_at` (still running or never completed), **WHEN** the row renders, **THEN** no duration is shown.
- **AC-UI-PROMPT-TURN-DURATION-001.8:** **GIVEN** an agent message whose turn is completed, **WHEN** its row renders, **THEN** no duration is shown.

## Migrated source detail

## Why

Reviewing a transcript means hovering a prompt to copy it or see its metadata; the time the agent spent on that prompt is only visible in the Prompt history panel, which requires opening it. Showing the completed turn's duration directly in the per-message hover row puts the same information where the reviewer already is.

## What

- The existing action row of a **user prompt** message (the `MessageActions` row: copy, raw toggle, navigation, metadata, timestamp) additionally shows the duration of the turn that the prompt produced, when that turn has completed.
- The duration renders in the same style as the Prompt history panel: an hourglass icon (`IconHourglass`, `h-3 w-3`, `aria-hidden`) followed by the compact duration text produced by `formatPromptDuration` (`0s`, `5s`, `5m 23s`, `1h 2m 3s`), localized with the existing `task:durationUnitSeconds` / `task:durationUnitMinutes` / `task:durationUnitHours` keys. No new copy, no translation changes.
- The duration is the wall-clock time from the prompt's `created_at` until its turn's `completed_at`, rounded down to whole seconds and clamped at zero. The timestamp parsing, flooring, and clamping arithmetic — and the compact `formatPromptDuration` display — are the same as the Prompt history panel's (`docs/specs/ui/requirements/prompt-history-panel.md`, "Duration rules"). The end bound is the turn's completion only: the panel's additional "earlier of turn completion and next prompt" bound does NOT apply here, because this row shows the duration of the turn that resulted from this prompt and only after that turn has completed. The duration appears only when both timestamps are present and parseable.
- The duration is hidden while the turn is still running and never appears for agent messages (their rows are not prompts and would otherwise show a meaningless partial duration). Absent duration renders nothing — no placeholder, no layout stub beyond the row's normal empty state.
- The duration is part of the existing action row. On fine-pointer layouts at or above `sm`, it follows the row's hover/focus reveal. On coarse-pointer layouts, it stays visible so touch users can reach it. It adds no new interaction surface.

## Scenarios

- **GIVEN** a user prompt message whose `turn_id` resolves to a turn with a parseable `completed_at`, **WHEN** the message's hover row renders, **THEN** the row shows the hourglass icon followed by the duration from the prompt's `created_at` to the turn's `completed_at`, floored to whole seconds.
- **GIVEN** a user prompt message whose turn has no `completed_at` (still running or never completed), **WHEN** the row renders, **THEN** no duration is shown.
- **GIVEN** an agent message whose turn is completed, **WHEN** its row renders, **THEN** no duration is shown.
- **GIVEN** a user prompt message with an unparseable `created_at` or a turn with an unparseable `completed_at`, **WHEN** the row renders, **THEN** no duration is shown (an unparseable timestamp never yields a `NaN` duration).
- **GIVEN** a completed turn that completed before the prompt was sent (clock skew), **WHEN** the row renders, **THEN** the duration is clamped to `0s`.
- **GIVEN** a completed turn with sub-second elapsed time, **WHEN** the row renders, **THEN** the duration is `0s` (floored, never rounded up).
- **GIVEN** a completed turn lasting 5.9 seconds, **WHEN** the row renders, **THEN** the duration reads `5s`.
- **GIVEN** a user prompt message with no `turn_id`, **WHEN** the row renders, **THEN** no duration is shown (the duration is bound to the prompt's own completed turn only).
- **GIVEN** a completed-turn user prompt in the transcript on a fine-pointer viewport at or above the `sm` breakpoint, **WHEN** the row is not hovered/focused, **THEN** the duration is hidden with the rest of the row; **WHEN** the row is hovered or keyboard-focused, **THEN** the duration is visible.
- **GIVEN** a completed-turn user prompt in the transcript on a coarse-pointer viewport, **WHEN** the row renders, **THEN** the duration is visible at every supported width so it does not depend on hover.
- **GIVEN** two user prompts whose messages share one completed turn, **WHEN** each message's row renders, **THEN** each row shows its own duration from its own `created_at` to the turn's `completed_at` (the two durations differ); this intentionally differs from the Prompt history panel, whose earlier-of-turn-completion-and-next-prompt bound would show the earlier prompt a shorter duration.

## Out of scope

- Changes to the Prompt history panel, its duration rules, or its pagination.
- Durations on agent-message rows, queued messages, or any message that is not a user prompt.
- A live "elapsed so far" indicator while a turn is running.
- New i18n keys or translation updates (the three `task:durationUnit*` keys are reused).
- Changes to the row's fine-pointer reveal behavior: below `sm` the row remains always visible, and at `sm+` it remains hidden until hover/focus. Coarse-pointer layouts keep the row visible at every width.
