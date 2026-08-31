# ADR-2026-08-31-workflow-profile-session-switch-policy: Make workflow profile-session switching explicit

**Status:** accepted
**Date:** 2026-08-31
**Area:** workflow

## Context

Fixed-profile workflow steps currently complete the session being switched away
from. A later step that returns to the same agent profile cannot reuse that
conversation because terminal sessions are deliberately excluded from profile
reuse. This exclusion prevents a resumed agent from treating an earlier
completion as undone. It also prevents authors from handing work back to the
same agent conversation.

Workflow authors need three distinct outcomes: retain the safe current behavior,
keep and reuse profile conversations, or keep old conversations answerable while
starting a fresh conversation on every profile re-entry.

## Decision

Each workflow stores one portable `profile_session_policy` enum:

- `complete` preserves the current behavior. The source session becomes
  `COMPLETED`, and a later profile re-entry reuses the newest independently
  eligible nonterminal session when one exists; otherwise, it creates a new
  session.
- `park_reuse` stops the source runtime but leaves its session nonterminal and
  answerable. A later profile re-entry reuses the newest eligible nonterminal
  session for that profile.
- `park_new` stops the source runtime but leaves its session nonterminal and
  answerable. Every profile re-entry creates a new session.

An absent or unknown value normalizes to `complete`. The policy applies only
when a workflow transition changes the effective agent profile. It does not
split one same-profile run merely because the task enters another step.

Parked workflow sessions use the existing `WAITING_FOR_INPUT` state and retain
their provider resume token, messages, task environment, and executor profile.
Kandev does not add a parallel lifecycle state. Before stopping an execution for
a parked switch, the orchestrator records an execution-stamped workflow-switch
stop intent. Completion and stopped handlers atomically mark the matching
intent consumed and suppress only the matching old execution event. The
consumed tombstone remains durable across delayed delivery and backend
restart. A later execution with a different identity remains eligible for
ordinary turn-complete processing.

The setting is workflow-scoped rather than step-scoped. It is exported,
imported, and synchronized with the workflow definition.

## Consequences

- Existing workflows keep their current safety and session-count behavior.
- Workflow authors can preserve several answerable conversations without
  keeping several agent processes running.
- `park_reuse` deliberately preserves provider conversation context, including
  the agent's earlier completion. Authors select it only when that continuity is
  desired.
- `park_new` can accumulate nonterminal historical sessions. They remain
  user-manageable through existing session controls and task cleanup.
- The orchestrator must distinguish a deliberate profile-switch stop from an
  ordinary agent completion by execution identity. Session state alone is not a
  sufficient signal.
- Workflow persistence, portable export/import, sync, editor drafts, and public
  documentation gain the same enum.

## Alternatives Considered

- Always park and reuse sessions: rejected because it would revive stale
  conversations for every existing workflow. It could also reintroduce the
  workflow-cycle incident guarded by the current tests.
- A boolean retention flag plus a separate reuse flag: rejected because it
  permits invalid or confusing combinations and requires precedence rules.
- A per-step reuse option: rejected because the requested ownership is the
  workflow and mixed policies make session-count behavior difficult to predict.
- Revive `COMPLETED` sessions in place: rejected because terminal rows are
  historical endpoints and can carry stale completion intent.
- Add a `PARKED` task-session state: rejected because
  `WAITING_FOR_INPUT` already represents a stopped, resumable, answerable
  session and is supported by prompt and UI paths.
