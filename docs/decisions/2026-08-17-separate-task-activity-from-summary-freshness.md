# ADR-2026-08-17-separate-task-activity-from-summary-freshness: Separate Task Activity From Summary Freshness

**Status:** accepted (amended 2026-08-29)
**Date:** 2026-08-17
**Area:** backend, frontend, protocol

## Context

Task sidebar rows currently use `TaskStatusSummary.updated_at` as their visible
time and saved-view sort value. That field records when any projected status
changed. A pull-request poll can therefore make many idle tasks look recently
active even when no user opened them and no agent ran.

Users need both concepts. Projection freshness is useful for status diagnostics,
while task activity is useful for finding recent work.

## Decision

Kandev will keep `TaskStatusSummary.updated_at` as transport and projection
freshness. It will add semantic field `last_activity_at` to the same bounded,
rebuildable summary.

`last_activity_at` is the latest durable time from these sources:

- task creation or a persisted task mutation
- a user-authored prompt, including a prompt that enters the queue
- a conversational agent turn start or completion

A turn with `metadata.lifecycle_only=true` is not conversational. The
synthetic turn named `agent_boot` on resume does not advance task activity.

The timestamp does not advance for task focus, session subscription, Git or
pull-request polling, queue bookkeeping, status-summary repair, or session
metadata maintenance. Live projection uses source timestamps and applies a
monotonic maximum. Rebuilds batch the same durable task, message, and turn
sources and preserve a newer stored value.

The saved-view wire key is `lastActivityAt`. The existing `updatedAt` key keeps
its behavior, so stored views remain compatible and users can choose either
meaning.

This decision extends
[ADR-2026-08-01-separate-task-summary-session-stream-traffic](2026-08-01-separate-task-summary-session-stream-traffic.md).
Task-list consumers continue to use the bounded task summary and do not load
background session streams.

## Consequences

Idle tasks stay stable when background provider status changes. Desktop and
mobile saved views can sort by meaningful work without opening task sessions.

The backend needs a batched activity query and more bounded projector inputs.
Both paths must exclude lifecycle-only turns. Conversational-turn milestones
prevent per-chunk agent output from producing task-list update traffic. A
running conversational turn becomes recent when it starts and advances again
when it completes.

Older summaries need a one-time semantic repair. During partial rollout, the
frontend falls back to the task update or creation time when
`last_activity_at` is absent.

## Alternatives Considered

- **Reuse summary `updated_at`.** Rejected because provider and Git status
  changes are not user or agent activity.
- **Use only `tasks.updated_at`.** Rejected because prompts and agent turns do
  not always mutate the task row.
- **Derive activity in each browser.** Rejected because background session
  history is not loaded and task switchers must not subscribe to it.
- **Write every activity to the task row.** Rejected because message and turn
  domains gain extra task writes and mix activity with task edits.
- **Count every persisted turn.** Rejected because lifecycle turns record
  runtime maintenance, not user or agent work.
