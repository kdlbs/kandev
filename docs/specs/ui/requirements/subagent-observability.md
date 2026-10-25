---
status: active
system: ui
created: 2026-08-03
owners:
  - nova28
---
# Subagent Observability Requirements

## Overview

An agent can dispatch subagents — a review wave, a research fan-out, a parallel migration — and each one is a substantial unit of work: minutes of wall time, tens of thousands of tokens, its own tool transcript. Kandev already ingests all of that. It normalizes the ACP subagent frames, stores per-subagent identity and metrics, nests each subagent's child tool calls under it, and derives a live `active_subagent_count` per session and per task.

## Requirements

### REQ-UI-SUBAGENT-OBSERVABILITY-001: Subagent Observability

**Intent:** An agent can dispatch subagents — a review wave, a research fan-out, a parallel migration — and each one is a substantial unit of work: minutes of wall time, tens of thousands of tokens, its own tool transcript. Kandev already ingests all of that. It normalizes the ACP subagent frames, stores per-subagent identity and metrics, nests each subagent's child tool calls under it, and derives a live `active_subagent_count` per session and per task.

#### Acceptance criteria

- **AC-UI-SUBAGENT-OBSERVABILITY-001.1:** The Claude subagent result parser reads two additional fields off `_meta.claudeCode.toolResponse`, the same map it already reads six fields from:
- **AC-UI-SUBAGENT-OBSERVABILITY-001.2:** `content` — an array of ACP content blocks. Text blocks are concatenated in order (newline-separated) into the subagent payload's `result_text`. Non-text blocks (image, audio) are skipped. Absent, empty, or all-non-text `content` leaves `result_text` empty.
- **AC-UI-SUBAGENT-OBSERVABILITY-001.3:** `resolvedModel` — a string, into the payload's `model`.
- **AC-UI-SUBAGENT-OBSERVABILITY-001.4:** `result_text` is a general subagent-payload field, not a Claude-specific one: its existing Auggie producer is unchanged, and a value already set by an earlier frame is not overwritten by a later empty one.
- **AC-UI-SUBAGENT-OBSERVABILITY-001.5:** The captured text is stored verbatim on the message payload. Truncation is a rendering concern, not a persistence one.
- **AC-UI-SUBAGENT-OBSERVABILITY-001.6:** A subagent is never collapsed into a turn group. When a turn group would contain subagent messages, those messages render at the group's own level, in chronological position, and the group collapses only the remaining tool calls. A group left with no non-subagent tool calls renders no collapsed row.
- **AC-UI-SUBAGENT-OBSERVABILITY-001.7:** The collapsed face of a subagent card shows, when the data is present: the subagent type, a description, the child tool-call count, duration, token count, model, and the **first line of `result_text`** as a single-line result summary.
- **AC-UI-SUBAGENT-OBSERVABILITY-001.8:** The result summary is the first non-empty line of `result_text`, truncated with an ellipsis to fit one line. The full `result_text` is shown in the expanded body, above any nested child messages.

## Migrated source detail

## Why

An agent can dispatch subagents — a review wave, a research fan-out, a parallel
migration — and each one is a substantial unit of work: minutes of wall time,
tens of thousands of tokens, its own tool transcript. Kandev already ingests all
of that. It normalizes the ACP subagent frames, stores per-subagent identity and
metrics, nests each subagent's child tool calls under it, and derives a live
`active_subagent_count` per session and per task.

None of it is reachable in practice.

A real session (task `93ff1109`, two review waves, four subagents) renders its
reviewers only after the user scrolls back ~50 messages and expands a row
labelled `4 tool calls, 2 subagents` — the subagent cards are collapsed inside a
turn group that treats a 243-second reviewer as equivalent to
`cat > /tmp/prompt.txt`. The kanban board shows nothing at all:
`active_subagent_count` reaches the frontend store and no component reads it,
because rendering it was an explicit non-goal of
[background-work-liveness](../../platform/requirements/background-work-liveness.md) and
[ADR 0049](../../../decisions/0049-fine-grained-foreground-idle-busy-signal.md)
deliberately preserved subagent identities "for a future subagent list without
committing the current UI to one". This spec is that follow-up.

The sharpest gap is that the card records what a subagent *did* but never what
it *returned*. For a review wave that is the whole point: an orchestrator's claim
to have run N reviewers cannot be checked against anything, because the verdicts
exist only in the orchestrator's own prose. A wire capture (`acpdbg`,
claude-agent-acp 0.49.0) confirms the returned text is already on the frame
kandev parses — `_meta.claudeCode.toolResponse.content` sits beside the
`totalTokens` and `totalDurationMs` the adapter already reads, and is stepped
over.

## What

### Capture

- The Claude subagent result parser reads two additional fields off
  `_meta.claudeCode.toolResponse`, the same map it already reads six fields from:
  - `content` — an array of ACP content blocks. Text blocks are concatenated in
    order (newline-separated) into the subagent payload's `result_text`. Non-text
    blocks (image, audio) are skipped. Absent, empty, or all-non-text `content`
    leaves `result_text` empty.
  - `resolvedModel` — a string, into the payload's `model`.
- `result_text` is a general subagent-payload field, not a Claude-specific one:
  its existing Auggie producer is unchanged, and a value already set by an
  earlier frame is not overwritten by a later empty one.
- The captured text is stored verbatim on the message payload. Truncation is a
  rendering concern, not a persistence one.

### Transcript

- A subagent is never collapsed into a turn group. When a turn group would
  contain subagent messages, those messages render at the group's own level, in
  chronological position, and the group collapses only the remaining tool calls.
  A group left with no non-subagent tool calls renders no collapsed row.
- The collapsed face of a subagent card shows, when the data is present: the
  subagent type, a description, the child tool-call count, duration, token count,
  model, and the **first line of `result_text`** as a single-line result summary.
- The result summary is the first non-empty line of `result_text`, truncated with
  an ellipsis to fit one line. The full `result_text` is shown in the expanded
  body, above any nested child messages.
- A subagent with no captured `result_text` shows no result summary line and no
  placeholder. Absence is silent — a subagent that returned nothing and an agent
  that does not report results must not be made to look like a failure.
- The description is not rendered when it merely restates the subagent type:
  the existing exact-match suppression is extended to also suppress a
  description whose leading words are the subagent type (case- and
  separator-insensitive, so `test-supervisor` suppresses the prefix of
  `Test-supervisor review of new invariant tests`, leaving
  `review of new invariant tests`).
- The `tools` metadata chip is suppressed when the reported `tool_use_count`
  equals the rendered child-message count, since the header already states it.
  A divergence between the two keeps both.

### Header status

- While a card is effectively active (`isSubagentEffectivelyActive`), its
  header always shows `task:working` and the existing `GridSpinner`, even when
  the card has expandable children. The previous
  `isActive && !hasExpandableContent` gate on the Working label is gone: a live
  wave with nested tool calls must not look like a settled type + description
  row.
- After a failed or cancelled finish, the header shows a red `IconX` with
  `aria-label` `task:failed` or `task:cancelled`. Successful completion shows a
  muted check (`aria-label` `task:statusCompleted`); the spinner and Working
  copy disappear, and the existing identity and metrics chips appear. No Done
  chip is added.
- UUID, model, duration, and token chips stay identity and metrics. They still
  render only when the card is not active. No Running / Failed / Done chip is
  added to the metadata row.

### Turn-group label

- A collapsed turn group's label states the counts it actually describes. The
  leading numeral is the number of collapsed tool calls, and pluralization
  agrees with it.
- Because subagents are no longer collapsed into groups, the label never
  mentions subagents.

### Board

- A kanban card shows a subagent indicator whenever the task's
  `active_subagent_count` is greater than zero: a small chip carrying the count,
  next to the existing status affordances.
- The chip is live — it appears, updates, and disappears from the same
  `active_subagent_count` already delivered by `task.updated` and the boot
  payload, with no new API surface.
- At zero, or when the field is absent, no chip renders.

## Scenarios

- **GIVEN** a Claude subagent completes with `toolResponse.content` carrying text
  blocks, **WHEN** the result is parsed, **THEN** the payload's `result_text` is
  the concatenated text and its `model` is `resolvedModel`.
- **GIVEN** a `toolResponse` whose `content` is absent, empty, or contains only
  non-text blocks, **WHEN** the result is parsed, **THEN** `result_text` is empty
  and no placeholder text is substituted.
- **GIVEN** a payload whose `result_text` was already set by an earlier frame,
  **WHEN** a later frame reports empty content, **THEN** the existing value is
  retained.
- **GIVEN** a turn containing two subagents and four tool calls, **WHEN** the
  transcript renders, **THEN** both subagent cards appear at top level and the
  collapsed group reads `4 tool calls`.
- **GIVEN** a turn containing one tool call, **WHEN** the transcript renders,
  **THEN** the collapsed group reads `1 tool call`.
- **GIVEN** a turn whose only tool activity is subagents, **WHEN** the transcript
  renders, **THEN** the subagent cards appear and no collapsed group row renders.
- **GIVEN** a completed subagent with `result_text` spanning several lines,
  **WHEN** its card is collapsed, **THEN** a single-line summary shows the first
  non-empty line, and **WHEN** expanded, **THEN** the full text is shown above
  any child messages.
- **GIVEN** a subagent with no `result_text`, **WHEN** its card renders, **THEN**
  no result summary line appears.
- **GIVEN** a subagent of type `test-supervisor` described as
  `Test-supervisor review of new invariant tests`, **WHEN** its header renders,
  **THEN** the description shows without the redundant leading type.
- **GIVEN** a subagent reporting `tool_use_count` of 10 with 10 rendered child
  messages, **WHEN** its metadata row renders, **THEN** the `tools` chip is
  omitted; **GIVEN** a reported count that differs from the child count, **THEN**
  both are shown.
- **GIVEN** a task whose `active_subagent_count` is two, **WHEN** its kanban card
  renders, **THEN** a chip shows the count; **WHEN** the count drops to zero,
  **THEN** the chip disappears without a reload.
- **GIVEN** an active subagent with nested child tool calls, **WHEN** its card
  renders collapsed or expanded, **THEN** the header shows `Working...` and a
  spinner, and the UUID/model chips are absent.
- **GIVEN** a successfully completed subagent, **WHEN** its card renders,
  **THEN** the header has no spinner, no Working copy, and a muted check
  labelled Completed, and the existing identity and metrics chips are visible.
- **GIVEN** a failed or cancelled subagent, **WHEN** its card renders, **THEN**
  the header shows a red error mark labelled Failed or Cancelled, with no
  spinner and no success check.

## Out of scope

- Modelling a subagent as a kandev task or task session. Subagents stay inside
  the parent session; this spec surfaces them, it does not promote them.
- `toolResponse.toolStats` (per-kind tool breakdown and line counts). Available
  on the same map and worth a later pass; not captured here.
- Parsing a verdict taxonomy (APPROVE / REQUEST_CHANGES) out of `result_text`.
  The text is surfaced verbatim; kandev does not interpret it.
- Result capture for non-Claude subagent dialects beyond the existing Auggie
  path. OpenCode, Cursor, and Amp continue to report only what they report.
- A dedicated subagent list, drill-down panel, or per-subagent filtering on the
  board. The board gets a count chip only.
- A Running / Failed / Done chip on the subagent metadata row. Status stays a
  header glyph (Working copy while live, then a muted check or a red X).
- Promoting identity chips (UUID, model) into the active header. They remain
  settled-card metadata.
- Any change to `active_subagent_count` derivation, liveness accounting, or the
  busy-signal contract, all owned by
  [background-work-liveness](../../platform/requirements/background-work-liveness.md).

## Notes

- Wire evidence: `_meta.claudeCode.toolResponse` on a Claude Task completion
  carries `agentId`, `agentType`, `status`, `content`, `prompt`, `resolvedModel`,
  `toolStats`, `totalDurationMs`, `totalTokens`, `totalToolUseCount`, `usage`.
  The adapter reads six of these in `claudeSubagentResponse`
  (`apps/backend/internal/agentctl/server/adapter/transport/acp/subagent.go`).
- The same text also arrives on the terminal frame's `content[]` and
  `rawOutput`, but that path is unreachable: `rawOutput` is a JSON array there
  and `extractSubagentResult` type-asserts it to a map. The `toolResponse` path
  is the one this spec uses; the array case is left alone.
- Frontend seams: `filterVisibleMessages` / turn grouping in
  `apps/web/hooks/use-processed-messages.ts`, the card in
  `apps/web/components/task/chat/messages/tool-subagent-message.tsx`, chips in
  `subagent-meta.ts`, the group label in `turn-group-message.tsx`, and the board
  chip in `apps/web/components/kanban-card-content.tsx` reading the
  `activeSubagentCount` already mapped in `apps/web/lib/kanban/map-task.ts`.
