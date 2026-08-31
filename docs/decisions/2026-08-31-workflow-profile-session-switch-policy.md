# ADR-2026-08-31-workflow-profile-session-switch-policy: Make workflow step profile-session switching explicit

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
starting a fresh conversation on profile re-entry.

The intended outcome can differ by destination step. For example, returning to a
planning step can continue its earlier conversation, while entering a review
step can require a fresh conversation. Agent selection and session behavior are
therefore one step-entry routing decision.

## Decision

Each workflow step stores one portable `profile_session_policy` enum beside
its optional `agent_profile_id`:

- `complete` preserves the current behavior. The source session becomes
  `COMPLETED`, and destination entry reuses the newest independently eligible
  nonterminal matching session when one exists; otherwise, it creates a new
  session.
- `park_reuse` stops the source runtime but leaves its session nonterminal and
  answerable. Destination entry reuses the newest eligible nonterminal session
  for that profile.
- `park_new` stops the source runtime but leaves its session nonterminal and
  answerable. Destination entry creates a new session.

An absent or unknown value normalizes to `complete`. The destination step's
policy applies only when entry changes the effective agent profile. It does not
split one same-profile run merely because the task enters another step.

The workflow engine continues to resolve transitions and destination steps as it
does today. The orchestrator's step-entry session router reads the policy from
the already resolved destination step. No workflow action, event, or transition
type is added.

Parked workflow sessions use the existing `WAITING_FOR_INPUT` state and retain
their provider resume token, messages, task environment, and executor profile.
Kandev does not add a parallel lifecycle state. Before stopping an execution for
a parked switch, the orchestrator records an execution-stamped workflow-switch
stop intent. Completion and stopped handlers atomically mark the matching intent
consumed and suppress only the matching old execution event. The consumed
tombstone remains durable across delayed delivery and backend restart. A later
execution with a different identity remains eligible for ordinary
turn-complete processing. The park claim and state transition hold the
session's lifecycle guard; runtime teardown starts only after that guard is
released so a synchronous terminal callback can consume the intent without
deadlocking.

The workflow editor uses one combined step agent selector. Its primary view
selects or searches agent profiles, and a nested configuration view selects the
session behavior with explanatory copy. Desktop uses a popover. Phone viewports
use an inset bottom drawer with the same draft and save logic.

## Consequences

- Existing steps keep their current safety and session-count behavior because
  omitted values normalize to `complete`.
- One workflow can express different conversation boundaries for planning,
  implementation, review, and repeated uses of one profile.
- The policy persists, exports, imports, and synchronizes with each step instead
  of with the workflow.
- The workflow engine state machine does not change. Step CRUD, portable
  contracts, sync, orchestrator step-entry routing, and editor draft state do.
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

## Alternatives Considered

- A workflow-wide policy: rejected because it is too coarse for workflows whose
  stages need different conversation boundaries.
- A separate policy control in Workflow details: rejected because it separates
  session-entry behavior from the step and agent profile that consume it.
- A standalone select beside the step agent select: rejected because authors
  must discover and reconcile two controls for one routing decision. A combined
  selector makes the relationship visible while preserving one save flow.
- A transition-edge policy: rejected because session behavior is an invariant of
  entering the destination step, regardless of which predecessor reached it.
- Always park and reuse sessions: rejected because it would revive stale
  conversations for every existing step. It could also reintroduce the
  workflow-cycle incident guarded by current tests.
- A boolean retention flag plus a separate reuse flag: rejected because it
  permits invalid or confusing combinations and requires precedence rules.
- Revive `COMPLETED` sessions in place: rejected because terminal rows are
  historical endpoints and can carry stale completion intent.
- Add a `PARKED` task-session state: rejected because
  `WAITING_FOR_INPUT` already represents a stopped, resumable, answerable
  session and is supported by prompt and UI paths.
