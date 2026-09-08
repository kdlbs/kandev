---
status: draft
system: tasks
created: 2026-08-12
owners:
  - nova28
---


# Workflow task-step transition ledger scenarios Requirements



## Overview



The ledger contract is observable through turn starts, committed transitions, rollback behavior, and downstream attribution scenarios.



## Scenarios

### Slice 1 — turn-start step stamp

- **GIVEN** a session whose task sits in workflow step `S`, **WHEN** a turn
  starts, **THEN** the turn's metadata contains `workflow_step_id_at_start` = `S`.
- **GIVEN** a session whose task has no workflow step, **WHEN** a turn starts,
  **THEN** the turn's metadata contains no `workflow_step_id_at_start` key — not
  an empty string, not `null`, not `0`.
- **GIVEN** a session with no runtime config to snapshot, **WHEN** a turn starts
  while its task sits in step `S`, **THEN** the metadata contains
  `workflow_step_id_at_start` = `S` and no `runtime_config_snapshot`.
- **GIVEN** a turn stamped with step `S`, **WHEN** the task moves to step `T`
  before the turn completes, **THEN** the turn's stamp is still `S`.
- **GIVEN** a turn created before activation, **WHEN** it is read after
  activation, **THEN** it carries no stamp and is not backfilled.
- **GIVEN** the task row cannot be read, **WHEN** a turn starts, **THEN** the
  turn is created without a stamp and turn creation does not fail.
- **GIVEN** a synthetic completed turn created for a lifecycle message, **WHEN**
  it is written while its task sits in step `S`, **THEN** it carries the same
  stamp as a normal turn.

### Slice 2 — ledger writes

- **GIVEN** a task in step `A`, **WHEN** a user moves it to step `B`, **THEN**
  exactly one ledger row exists with `from_workflow_step_id` = `A`,
  `to_workflow_step_id` = `B`, trigger `manual_move`, actor kind `human`, and
  `actor_id` equal to the acting user's ID.
- **GIVEN** a task in step `A` with no session at all, **WHEN** a bulk move
  sends it to step `B`, **THEN** one row is written with `session_id` NULL and
  trigger `bulk_move`.
- **GIVEN** a task in step `A` with two live sessions, **WHEN** a WIP pull
  promotes it into step `B`, **THEN** one row is written with `session_id` NULL,
  trigger `wip_pull`, and actor kind `system`.
- **GIVEN** an agent session on a task in step `A`, **WHEN** the agent calls
  `move_task_kandev` and it applies immediately, **THEN** one row is written
  with trigger `mcp_move`, actor kind `agent`, and `actor_id` equal to that
  session's ID.
- **GIVEN** an agent's deferred `move_task_kandev`, **WHEN** it applies at turn
  end, **THEN** one row is written with trigger `mcp_deferred_move`.
- **GIVEN** a workflow step whose `on_turn_complete` transitions to step `B`,
  **WHEN** the turn completes, **THEN** one row is written with trigger
  `engine_transition`.
- **GIVEN** a task in step `A`, **WHEN** a move to step `A` is issued, **THEN**
  no row is written.
- **GIVEN** a task in step `A`, **WHEN** only its position within `A` changes,
  **THEN** no row is written.
- **GIVEN** a WIP-limited target step at capacity, **WHEN** a move into it is
  rejected, **THEN** no row is written and the task's step is unchanged.
- **GIVEN** the transaction that changes the step is rolled back for any reason,
  **WHEN** it completes, **THEN** no row exists for it.
- **GIVEN** a task created into step `A`, **WHEN** creation commits, **THEN** one
  row exists with `from_workflow_id` and `from_workflow_step_id` NULL and
  trigger `task_created`.
- **GIVEN** a task created targeting a full step `B` with feeder step `A`,
  **WHEN** creation commits and places it in `A`, **THEN** the genesis row's
  `to_workflow_step_id` is `A`, the step it was actually placed in.
- **GIVEN** a task attached to a workflow, **WHEN** it is later removed from
  that workflow, **THEN** a row is written with `to_workflow_id` and
  `to_workflow_step_id` NULL and trigger `workflow_detached`.
- **GIVEN** a task whose step changed via a task update rather than the move
  path, **WHEN** the update commits, **THEN** one row is written with trigger
  `task_update`.
- **GIVEN** a caller that declares no trigger or actor, **WHEN** its step change
  commits, **THEN** the row records trigger `unknown`, actor kind `unknown`, and
  `actor_id` NULL.
- **GIVEN** a task created with no workflow, **WHEN** creation commits, **THEN**
  no row is written; **WHEN** it is later attached to a workflow, **THEN** its
  first row has trigger `workflow_attached` and `from_*` NULL.
- **GIVEN** an ephemeral (quick-chat) task in step `A`, **WHEN** it moves to
  step `B`, **THEN** a row is written exactly as for any other task.
- **GIVEN** a task in step `A`, **WHEN** it is archived, unarchived in place,
  cascade-archived, or deleted without its step changing, **THEN** no row is
  written for that action.
- **GIVEN** an engine action that switches a task to a step in a **different**
  workflow, **WHEN** the transition commits, **THEN** one row is written whose
  `from_workflow_id` and `to_workflow_id` differ, with trigger
  `engine_transition`.
- **GIVEN** a task whose step is stored as the empty string, **WHEN** it moves
  into step `B`, **THEN** the row's `from_workflow_step_id` is NULL, not `""`.
- **GIVEN** a row referencing a workflow step that is later deleted, **WHEN**
  the step is deleted, **THEN** the row survives unchanged with its step ID
  intact.

### Ordering, chain, and concurrency

- **GIVEN** a task moved `A`→`B`→`A`→`B`, **WHEN** its rows are read ordered by
  `(occurred_at, id)`, **THEN** four rows appear and each row's
  `from_workflow_step_id` equals the previous row's `to_workflow_step_id`.
- **GIVEN** two rows for one task sharing an identical `occurred_at`, **WHEN**
  they are ordered by `(occurred_at, id)`, **THEN** the order is total and
  matches the order the transitions committed.
- **GIVEN** two callers concurrently moving the same task, one to `B` and one to
  `C`, **WHEN** both transactions commit, **THEN** exactly two rows exist, their
  chain is intact, and the later row's `to_workflow_step_id` equals the task's
  final `workflow_step_id`.
- **GIVEN** an engine trigger that is retried with the same `OperationID`,
  **WHEN** the retry is handled, **THEN** no second row is written.
- **GIVEN** any task, **WHEN** its last row is read, **THEN** its
  `to_workflow_step_id` equals `tasks.workflow_step_id`.
- **GIVEN** two rows for one task whose `occurred_at` values run backwards
  because the host clock was corrected, **WHEN** they are ordered by `id`,
  **THEN** the chain invariant still holds and the ledger is not repaired or
  reordered by timestamp.
- **GIVEN** a turn stamped with step `S` and a ledger row moving the task to `T`
  a moment after that turn started, **WHEN** both are read, **THEN** they
  disagree, and neither is treated as an error.

### Actor and privacy

- **GIVEN** authentication is disabled, **WHEN** the single synthetic user moves
  a card, **THEN** actor kind is `human` and `actor_id` is that identity's user
  ID.
- **GIVEN** a Jira or Linear watcher that moves a card, **WHEN** the move
  commits, **THEN** actor kind is `integration` and `actor_id` is the watch ID.
- **GIVEN** any row, **WHEN** it is inspected, **THEN** `actor_id` holds an
  identifier and no display name, email, title, or prompt text appears anywhere
  in the row.

### Migration and activation

- **GIVEN** a database created before this feature, **WHEN** the backend boots,
  **THEN** the ledger table and `telemetry_activations` exist, pre-existing
  tasks have no ledger rows, and pre-existing turns carry no stamp.
- **GIVEN** a database already migrated, **WHEN** the migration runner is run
  twice more, **THEN** it succeeds both times and changes nothing.
- **GIVEN** a Postgres deployment, **WHEN** the same fresh-then-replay sequence
  runs, **THEN** it behaves identically to SQLite.
- **GIVEN** a first boot on which a contract activates, **WHEN** boot completes,
  **THEN** `telemetry_activations` holds one row for that key with
  `contract_version` = 1 and that boot's UTC time.
- **GIVEN** a subsequent boot, **WHEN** it completes, **THEN** the existing
  activation row is unchanged.
- **GIVEN** an install where the ledger table is absent because its migration
  silently failed, **WHEN** a step change is attempted, **THEN** the transaction
  fails rather than committing a step change with no row, and the boot health
  line reports the table as absent.

### Writer health

- **GIVEN** a new production statement that mutates `tasks.workflow_step_id`,
  **WHEN** the test suite runs, **THEN** the pinning test fails until that
  statement is registered.
- **GIVEN** a ledger row is written, **WHEN** metrics are read, **THEN** the
  expvar counter for that trigger has incremented and a `telemetry.metric.*`
  log line names the event.
- **GIVEN** any boot, **WHEN** startup completes, **THEN** one health line per
  registered contract key reports object existence, activation time, row count,
  and most recent `occurred_at`.

## Requirements



### REQ-TASKS-TRANSITION-LEDGER-SCENARIOS-001: Workflow task-step transition ledger scenarios



**Intent:** The ledger contract is observable through turn starts, committed transitions, rollback behavior, and downstream attribution scenarios.



#### Acceptance criteria



- **AC-TASKS-TRANSITION-LEDGER-SCENARIOS-001.1:** When a ledger scenario occurs, the system shall preserve the committed, ordered, and bounded evidence described by the scenario.



## Out of scope

- **Backfilling history.** No pre-activation transition is reconstructed, from
  the message timeline or otherwise.
- **Dividing a cost event across a mid-turn step change.** Explicitly not
  attempted; the "dominant of two steps" bucket remains.
- **`workflow_step_decisions`.** A different mechanism with a live writer. Not
  merged, not extended, not read here.
- **Wiring, changing, or removing `session_step_history`** and its endpoint.
  Separate work, lands first.
- **Any read API.** No HTTP route, no WS event, no MCP tool, no UI. Consumers
  read the database.
- **The downstream extract and dashboard.** The consumer contract above is
  recorded for the analysis side; implementing it is not part of this feature.
- **Step-name capture or step-rename history.** IDs only.
- **Retention, pruning, or archival of ledger rows.**
- **A `parent_session_id` / subagent grain.** Separate card.
- **Per-turn cost columns.** Separate card.