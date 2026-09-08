---
status: draft
system: tasks
requirements:
  - REQ-TASKS-TRANSITION-LEDGER-001
  - REQ-TASKS-TRANSITION-LEDGER-SCENARIOS-001
---


# Workflow task-step transition ledger System Design



## Purpose and boundaries



The task system owns the task-level transition ledger and its writer boundaries. Session history remains owned by the session lifecycle contract; downstream cost and analytics consumers read the ledger without changing its authority.



## Requirement mapping

| Requirement | Design source |
| --- | --- |
| `REQ-TASKS-TRANSITION-LEDGER-001` | Extracted from the legacy design sections below. |
| `REQ-TASKS-TRANSITION-LEDGER-SCENARIOS-001` | Extracted from the legacy design sections below. |


## Data model

### `task_step_transitions` (new)

```
task_step_transitions
  id                        int64      PK, monotonic per install
                                       (SQLite AUTOINCREMENT / Postgres BIGSERIAL)
  task_id                   string     FK -> tasks.id ON DELETE CASCADE
  session_id                string     FK -> task_sessions.id ON DELETE SET NULL,
                                       nullable — the initiating session, when one exists
  from_workflow_id          string     nullable — NULL only on the genesis row
  from_workflow_step_id     string     nullable — NULL on genesis, and when the task
                                       held no step (detached from a workflow)
  to_workflow_id            string     nullable — NULL when the task is detached
  to_workflow_step_id       string     nullable — NULL when the task is detached
  trigger                   enum       see Trigger below, NOT NULL
  actor_kind                enum       see Actor kind below, NOT NULL
  actor_id                  string     nullable — an opaque identifier, never a name
  contract_version          int        NOT NULL, starts at 1
  occurred_at               timestamp  NOT NULL, UTC
```

Indexes: `(task_id, occurred_at, id)` for interval reconstruction;
`(occurred_at)` for extract windowing.

`from_*` and `to_*` are never both NULL — a row with neither side would record
nothing. `from_workflow_step_id` equals `to_workflow_step_id` on no row; that
combination is the no-op case, which writes nothing.

**Empty string normalizes to NULL.** `tasks.workflow_step_id` uses `''` for "no
step" and `tasks.workflow_id` likewise. The ledger stores `NULL`, never `''`,
for the same condition, in both the `from_*` and `to_*` pairs. A consumer that
sees `''` in this table is looking at a bug.

**There is deliberately no foreign key to `workflow_steps` or `workflows`.**
Steps and workflows are deleted; the historical fact that a card was in a
now-deleted step must survive that deletion. Consumers MUST tolerate a step ID
with no matching row.

**Trigger** — why the step changed. Unrecognised or unattributable causes use
`unknown`; the enum is closed and new causes are added to it deliberately.

| Value | Meaning |
|---|---|
| `task_created` | The genesis row: the task was created into a step. |
| `manual_move` | A user moved the card (HTTP/WS move, drag, board action). |
| `task_update` | A task update changed `workflow_step_id` without going through the move path. |
| `mcp_move` | An agent moved the card via `move_task_kandev`, applied immediately. |
| `mcp_deferred_move` | An agent's `move_task_kandev` applied at turn end. |
| `engine_transition` | The workflow engine applied a step action (`on_enter`, `on_exit`, `on_turn_start`, `on_turn_complete`). |
| `wip_pull` | A WIP/queue reconciler pulled or promoted the card into a vacated step. |
| `bulk_move` | A bulk move of a selection or of a whole step. |
| `unarchive_restore` | A restore/rollback returned the card to a recorded step. |
| `workflow_attached` | The task was attached to a workflow. |
| `workflow_detached` | The task was removed from a workflow (`to_*` NULL). |
| `unknown` | The path did not declare a trigger. |

Each production path that mutates a task's workflow step maps to exactly one
trigger. The mapping is fixed so that no implementer has to choose:

| Path | Trigger |
|---|---|
| Task creation admission placement | `task_created` |
| `Service.MoveTask` / `MoveTaskWithOptions` from an HTTP/WS board action | `manual_move` |
| `Service.MoveTask` reached from `move_task_kandev`, applied immediately | `mcp_move` |
| A `move_task_kandev` pending move applied at turn end | `mcp_deferred_move` |
| `Service.UpdateTask` where the request carries a new `workflow_step_id` | `task_update` |
| `executeStepTransition` and the engine's `ApplyTransition` | `engine_transition` |
| An explicit user cancellation completing a turn (`turnCompletionCauseUserCancellation`), reached through the same `on_turn_complete` code path as `engine_transition` | `user_cancellation` |
| Feeder pull and queued-task promotion on step vacate | `wip_pull` |
| `BulkMoveTasks` / `BulkMoveSelectedTasks` | `bulk_move` |
| `RestoreTaskMessageRollbackIfSessionState` | `unarchive_restore` |
| `AddTaskToWorkflow` | `workflow_attached` |
| `RemoveTaskFromWorkflow` | `workflow_detached` |
| Anything not in this table | `unknown` |

A move that reaches `MoveTask` through more than one of these is attributed to
the outermost caller: `mcp_move` beats `manual_move`, because the agent, not a
board click, is what caused it.

**Actor kind** — what kind of thing caused it.

| Value | Meaning |
|---|---|
| `human` | An authenticated identity, or the synthetic single-user identity when auth is disabled. `actor_id` is the user ID. |
| `agent` | An agent session acting through MCP, or a session whose turn the engine reacted to. `actor_id` is the session ID. |
| `system` | The workflow engine reacting to a non-session trigger, a queue reconciler, or a scheduler, with no initiating identity. `actor_id` is `NULL`. |
| `integration` | An external watcher or automation (GitHub, GitLab, Jira, Linear, Sentry, Azure DevOps, webhook automation). `actor_id` is the watch/automation ID. |
| `unknown` | Not determinable. `actor_id` is `NULL`. |

`actor_id` MUST be an identifier. Display names, titles, emails, and prompts
MUST NOT be written to this table.

For actor kind `agent`, `actor_id` and `session_id` hold the same session ID.
The duplication is intentional: `actor_id` then has one uniform meaning ("the
identifier of whoever caused this") across every actor kind, and consumers do
not branch on kind to find the actor.

An `engine_transition` is `agent` when the trigger came from a session's turn
(`on_turn_start`, `on_turn_complete`, `on_exit`/`on_enter` reached through one)
and `system` when it came from a non-session trigger such as a
children-completed rollup or a scheduled evaluation.

A `user_cancellation` is never `agent`, even though it reaches the engine
through the same `on_turn_complete` code path an `engine_transition` does: the
turn didn't run to completion on its own, a caller forced it closed. Actor
kind for `user_cancellation` follows the same identity-on-context rule as
`manual_move` — `human` with the cancelling request's user ID when an
authenticated identity is present, `system` with no identifier when it is not
(for example, an automated caller that cancels on a session's behalf with no
request-scoped identity). This is a deliberate carve-out from the
`engine_transition` rule above, not a relaxation of it: the trigger value
itself (`user_cancellation` vs. `engine_transition`) is what distinguishes a
forced completion from a natural one, so a consumer never has to inspect
`actor_kind` to tell them apart.

### `telemetry_activations` (new)

A one-row-per-contract registry that makes each telemetry contract's activation
point readable from the snapshot itself.

```
telemetry_activations
  contract_key      string     PK part 1 — e.g. "turn.workflow_step_id_at_start",
                               "task_step_transitions"
  contract_version  int        PK part 2
  activated_at      timestamp  NOT NULL, UTC — first boot on which this version of
                               the contract became active on this install
```

The primary key is `(contract_key, contract_version)`, not `contract_key` alone,
so a future version bump **appends** a row rather than overwriting the first
activation. That is the whole point: a series that changed meaning mid-stream
must show two activation points, not one.

Written once per `(key, version)`, on the boot that first makes it active; never
rewritten. It lives in the same
schema as the data it describes so that a database reset clears both together —
an activation marker that outlives its data would be worse than none.

This spec registers exactly two keys: `turn.workflow_step_id_at_start`
(Slice 1) and `task_step_transitions` (Slice 2), each at version `1`.

### `task_session_turns.metadata` (existing column, new key)

`metadata` is a JSON object; today its only key is `runtime_config_snapshot`
(1,722 of 1,722 turns in the reference store). Slice 1 adds a sibling key:

```json
{
  "runtime_config_snapshot": { "model": "opus[1m]", "mode": "auto" },
  "workflow_step_id_at_start": "e6f925e0-de99-4a29-88d7-1843d033e141"
}
```

- The key is present only when the task held a workflow step when the turn
  started. It is otherwise **absent** — not `""`, not `null`, not `0`.
- Adding the key MUST NOT depend on `runtime_config_snapshot` being present. A
  turn whose runtime snapshot is empty still carries the stamp.
- The value is a workflow step ID. It is not resolved to a step name; step names
  are mutable and the ID is the join key.

## API surface

**None.** This feature adds no HTTP route, no WebSocket event, no MCP tool, and
no frontend surface. Its only consumers read the database.

Existing surfaces are unchanged, including `GET /api/v1/sessions/:id/workflow/history`.

The Go-side contract is the invariant in *What*, Slice 2: the repository
transaction that commits a change to `tasks.workflow_step_id` also commits the
ledger row. Callers supply trigger and actor; a caller that supplies neither
gets `unknown` / `unknown`, which is a recorded fact and not an error.

## Determinism, ordering, and concurrency

**Ordering.** Ledger rows for one task are ordered by `occurred_at` ASC, tiebroken
by `id` ASC. `id` is the authoritative sequence — two transitions can share a
timestamp, and `occurred_at` alone is not a total order. There is no third
tiebreak because `id` is unique.

**Clock.** `occurred_at` is the host wall clock at transaction time, in UTC. It
can move backwards across an NTP correction. When `occurred_at` and `id`
disagree about the order of two rows **for the same task**, `id` is correct;
`occurred_at` is a measurement, `id` is the record. Consumers computing
durations from a backwards-stepping pair get a non-positive interval and MUST
treat it as unknown rather than clamping it to zero.

**`id` is not a global watermark.** Postgres `BIGSERIAL` allocates from a
sequence before commit, so ids can commit out of order across concurrent
transactions. An incremental extract MUST window on `occurred_at` with an
overlap wide enough to cover its longest transaction, never on `MAX(id)` seen
last time. Within a single task the chain invariant below makes gaps detectable;
across tasks it does not.

**Chain invariant.** For any single task, reading its rows in that order, each
row's `from_workflow_step_id` equals the previous row's `to_workflow_step_id`,
and the last row's `to_workflow_step_id` equals the task's current
`tasks.workflow_step_id`. This holds under concurrent moves; it therefore
requires that the read of the old step and the write of the new one be
serialized against other writers of the same task row, not read outside the
write transaction.

**Concurrency.** Two callers moving the same task to different steps produce two
rows with an intact chain, in the order the transactions committed. This spec
does not change which move wins — last committed writer still wins, as today.
Neither move is rejected merely because the other raced it.

**No-op writes.** A committed update whose `workflow_step_id` is unchanged
writes no row, whatever else it changed. This is the whole of the feature's
idempotency: a client retrying a move that already applied produces a second
committed update, a no-op step change, and therefore no second row.

**Engine retries.** Engine transitions are already deduplicated by the engine's
`OperationID` applied-operations store, so a retried trigger does not re-apply
the transition and writes no second row. No additional idempotency key is added,
and there is no unique constraint on the ledger — a card legitimately moving
A→B→A→B produces four rows.

**Genesis rows.** Task creation writes one row with `from_*` NULL and trigger
`task_created`, including when WIP capacity places the task in a feeder step
rather than the requested target. Without it, the first interval of every task
created after activation would have no start boundary in the ledger.

A task created with **no workflow at all** writes no genesis row — there is no
step to record, and a row with both sides NULL is forbidden. Its first row is
the `workflow_attached` row written when it later joins a workflow.

**Which tasks are recorded.** All of them, including ephemeral (quick-chat) and
config tasks, and including tasks whose origin excludes them from WIP counts.
Filtering by task kind is the consumer's job; excluding a class of task here
would put invisible holes in the chain invariant, and the invariant is what
makes the ledger self-checking.

**Archiving is not a transition.** Archive, unarchive-in-place, cascade archive,
and delete do not change `tasks.workflow_step_id` and therefore write no row.
Only `unarchive_restore`, which does set a step, writes one.

**Ledger vs. `tasks.updated_at`.** `occurred_at` is the transaction's timestamp,
so a row's `occurred_at` and its task's `updated_at` agree at write time. The
task's `updated_at` moves on later unrelated writes; the ledger row never does.

## Permissions

The ledger is written by the same transactions that already perform authorized
step mutations; it grants no new capability and performs no authorization of its
own. If a caller was not permitted to move the task, no move and therefore no
row occurs.

There is no read surface, so there is no read authorization. Anyone with
database access already has it.

## Failure modes

| Condition | Behaviour |
|---|---|
| Ledger insert fails inside the transaction | The whole transaction rolls back: the step change does not commit either. Telemetry never silently diverges from the state it describes. |
| Migration fails at boot | Kandev's migration runner swallows migration errors (`db.MigrateLogger.Apply` logs and continues). A failed `CREATE TABLE` is therefore invisible at boot. The writer MUST detect the missing table at first write and fail the transaction rather than skip the row — a step change that cannot be recorded is preferable to a ledger that is quietly partial. The boot health line (below) reports the table's absence. |
| Trigger or actor cannot be determined | Written as `unknown`, which is a recorded fact. It is never guessed. |
| Task has several live sessions and no single initiator | `session_id` is `NULL`. |
| Task has no workflow step (detached) | `to_*` NULL with trigger `workflow_detached`; the stamp key is absent from new turns. |
| Task is deleted | Rows cascade away with the task. Kandev prefers archive over delete, and archived tasks retain their rows; a deleted task's step history is gone by design and is not recoverable. |
| Session is deleted | `session_id` becomes `NULL`; the row survives, because the transition was a task-level fact. |
| Slice 1: the task row cannot be read when a turn starts | The turn is still created; the stamp key is absent. Turn creation MUST NOT fail because telemetry could not be resolved. |

## Persistence guarantees

- Ledger rows and turn stamps survive restart, upgrade, and backup/restore. They
  are ordinary rows in the primary database.
- Nothing about this feature is held in memory across a restart. There is no
  queue, no buffer, and no deferred flush; a row exists exactly when its
  transaction committed.
- `telemetry_activations` rows survive restart and upgrade, and are cleared by a
  database reset together with the data they describe.
- No retention policy, no TTL, no pruning. The ledger is bounded by the number
  of step changes (72 tasks and 1,722 turns on the reference store); it needs
  none.
- Postgres parity is required per [ADR 0027](../../../decisions/0027-replayable-schema-migrations.md).

## Migration and activation

Kandev has no migration framework: schema is Go string DDL applied idempotently
at boot, and the runner swallows errors. Therefore:

- New columns are added nullable and backfilled by a separate idempotent
  statement, following the `task_session_messages.updated_at` pattern
  (`ALTER TABLE … ADD COLUMN` then `UPDATE … WHERE … IS NULL`). Anything
  referencing a new column — index, backfill, partial predicate — runs after the
  `ADD COLUMN`, never in the `CREATE TABLE` init block.
- Nothing is backfilled with a substituted value. Pre-activation state is
  absent, and absent is the correct reading.
- The migration is replay-safe: running it twice on the same database is a
  no-op, and dialect-specific replay classification uses `internal/db`
  (`IsDuplicateColumnError` / `IsAlreadyExistsError`), never local string
  matching.
- Coverage mirrors `task_external_id_migration_test.go`: seed a pre-migration
  row, run migrations, assert the pre-existing row reads NULL and the new
  objects exist, then call the migration runner **twice more** and assert
  idempotence — on SQLite and, env-gated, on Postgres.
- On the boot that first activates each contract, a `telemetry_activations` row
  is written with that boot's UTC time and contract version `1`. It is written
  once and never rewritten.

## Writer health

A telemetry writer that silently stops is worse than one that was never built,
because its output still looks like data. Three controls, all required:

1. **A pinning test over the mutation set.** The production statements that may
   change `tasks.workflow_step_id` are enumerated in a test. A new statement
   that is not registered fails the build. This is the control that matters:
   the realistic failure is not the writer breaking, it is a new code path
   bypassing it. The set as of this spec is
   `UpdateTask`, `UpdateTaskIfWorkflowStepHasCapacity`,
   `PromoteQueuedTaskIfWorkflowStepHasCapacity`,
   `RestoreTaskMessageRollbackIfSessionState`, `AddTaskToWorkflow`,
   `RemoveTaskFromWorkflow`, and task creation's admission placement.
2. **Runtime counters**, following the `routing.metric.*` precedent: an expvar
   counter of rows written keyed by trigger, and of turns created keyed by
   whether the stamp was present. Mirrored as structured logs under a
   `telemetry.metric.*` namespace so a log-aggregation rule can match the event
   name without scanning free text.
3. **A boot health line** reporting, for each registered contract key: whether
   its objects exist, its activation timestamp, the current row count, and the
   most recent `occurred_at`. An install whose ledger stopped growing is visible
   in one line.

## Relationship to `session_step_history`

`session_step_history` exists, has a `CreateStepTransition` writer with **zero
production callers** on `main` (verified at `db4fc039a`), and is read by
`GET /api/v1/sessions/:id/workflow/history`, which therefore returns
`{"history":[]}` on every install. Wiring that writer is separate work and lands
first; it and this ledger touch the same code region and cannot proceed in
parallel.

This spec does not delete, deprecate, migrate, or dual-write that table. The two
answer different questions — "what did this session do" versus "what happened to
this card" — and a card can change step with no session or with several, which
is why the second question needs its own grain.

## Downstream consumer contract

Normative for the analysis extract, not for this repository — recorded here
because the producer's obligations only make sense against it.

Every published per-step figure MUST carry an `attribution_basis` per row and
coverage counts by basis. Basis is resolved in this precedence:

| Basis | Source | Precedence |
|---|---|---|
| `task_history` | A `task_step_transitions` interval containing the event | 1 (strongest) |
| `turn_start` | `task_session_turns.metadata.workflow_step_id_at_start` | 2 |
| `message_reconstruction` | `task_session_messages.metadata.workflow_step_id` on workflow-automation messages (the mechanism in use today) | 3 |
| `unknown` | No source resolves the event | 4 |

**A partly-populated ledger MUST NOT silently replace message-timeline
reconstruction.** Both remain live; the ledger raises precedence for events it
covers and changes nothing for events it does not. A figure published without
its coverage-by-basis breakdown is a figure whose meaning changed when the
ledger activated, and that is exactly the discontinuity this contract exists to
prevent.

Kandev's obligations that make this possible: publish the activation point
(`telemetry_activations`), version each row (`contract_version`), and leave
pre-activation state absent rather than defaulted.

## Honest limits

- Attribution improves **forward only**. History is not repaired.
- A cost event whose window crosses a step boundary is still not divided between
  the two steps. The 30.5% "dominant of two steps" bucket shrinks as boundaries
  sharpen; it does not go to zero, and this feature does not attempt it.
- Turn-level stamping records the step at turn **start**. A turn that spans a
  step change is attributed to where it began, which is a choice, not a
  measurement.
- Step names are not captured. A renamed step is the same step; a deleted step
  leaves rows pointing at an ID with no row to join, and the consumer must
  tolerate that.

## Gate between slices

Slice 1 ships alone and is measured before Slice 2 is built. The measurement
that opens the gate: on a store with at least two weeks of post-activation
turns, report the share of post-activation turns carrying the stamp, and the
resulting change in the 47.0 / 30.5 / 22.5 attribution split when `turn_start`
is admitted as a basis. If Slice 1 alone moves the unattributable bucket enough,
Slice 2's scope is a decision made against that number rather than against this
spec's assumption.

If the measurement is inconclusive — too few post-activation turns, or a change
inside noise — the default is that **Slice 2 proceeds as specified**. The gate
exists to let evidence shrink Slice 2, not to let missing evidence stall it.

The two slices can disagree: a turn's stamp reads the task's step at turn start
and can be superseded by a move that commits moments later, which the ledger
records and the stamp does not. That is expected, not an inconsistency, and is
why `task_history` outranks `turn_start` in the basis precedence. Neither writer
corrects the other.
