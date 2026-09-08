---
status: building
created: 2026-08-19
owner: kandev
---

# Administrative Turn Settlement

## Why

An agent can prove it completed a workflow step even when its provider never sends the terminal lifecycle event. Kandev must release that exact finished turn without interrupting a real successor or rolling a task backward.

Decision: [ADR-2026-08-19-durable-administrative-turn-settlement](../../../decisions/2026-08-19-durable-administrative-turn-settlement.md).

## What

- An accepted signal-gated completion identifies the exact active task/session/turn/step and is settled once by provider completion or conservative reconciliation.
- Later foreground or tool activity, clarification, user work, a generation mismatch, or a successor turn prevents automatic settlement of that identity.
- A settled prior completion never re-evaluates an old workflow step. If the task moved, Kandev marks the intent superseded and ensures only the current transition's on-entry delivery.
- Repeated on-entry processing for one committed task-step transition creates at most one real successor prompt.
- `settle_stale_session_kandev` settles one demonstrably stale exact turn while preserving the task, worktree, history, queues, and sibling sessions.

## Data model

`session_completion_intents` has one row per `(session_id, turn_id, workflow_step_id)` with captured execution/generation, completion summary, state (`pending`, `settling`, `settled`, `reopened`, `superseded`, `rejected`), and activity/settlement timestamps. `session_control_events` records authorized stale-settlement attempts and evidence without prompt content.

## API surface

Task MCP exposes `settle_stale_session_kandev` with an exact target session and turn identity. It returns a settled, already-settled, or `active_turn`/`not_stale` result. `stop_task_kandev` remains the broad direct-child, all-live-session halt operation.

## Permissions

The caller and target must be in one workspace. A caller must be a different session on the target task, its direct parent, or the server-recorded supervisor of the target session. Relation alone is insufficient: Kandev requires terminal evidence and absence of active ownership evidence.

## Scenarios

- **GIVEN** a completion signal and no provider terminal event, **WHEN** the quiet grace passes with no conflicting activity, **THEN** only the captured turn becomes terminal and configured completion runs once.
- **GIVEN** the task moved before reconciliation, **WHEN** the old intent settles, **THEN** it is superseded and only the current transition is delivered once.
- **GIVEN** a same-profile Work-to-Review handoff, **WHEN** the former turn settles, **THEN** Review receives one real successor turn.
- **GIVEN** an authorized coordinator targets an active or ambiguous turn, **WHEN** it requests stale settlement, **THEN** Kandev performs no mutation and returns `active_turn` or `not_stale`.

## Out of scope

- Reviving terminal sessions, resetting context, or broadening parent-child stop authority.
