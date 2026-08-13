---
status: draft
created: 2026-08-12
owner: nova28
---

# Task step transition ledger

## Why

Kandev records *when a card was worked on* but not *which workflow step it was
in at the time*. Cost, token, and duration events can be joined to a task and a
session, but attributing them to a step requires reconstructing step boundaries
from the message timeline. On the reference store that reconstruction leaves
**47.0%** of spend cleanly inside one step, **30.5%** merely "dominant of two
steps in the window", and **22.5%** unattributable — so "what does Review cost"
cannot be answered without a caveat larger than the answer.

The one table that looks like it should answer this, `session_step_history`, is
keyed by **session** while Kandev tracks step state on the **task**
(`base_migrations.go` removed `task_sessions.workflow_step_id` for exactly that
reason). A card can change step with no active session, or with several, so
attributing a task-level transition to one session manufactures ownership that
does not exist.

## What this feature is, and is not

**It is a forward-looking record of step boundaries.** From its activation point
onward, every durable change to a task's workflow step is recorded with its
timestamp, its cause, and its actor, so downstream analysis can bound a step
interval from data rather than infer it from chat.

**It cannot repair the past.** No historical transition is reconstructed or
backfilled. Rows before the activation point do not exist and MUST NOT be
synthesised.

**It does not make per-step cost exact.** Cost events bill windows that cross
step boundaries; better boundaries improve attribution but do not decide how one
window's cost divides between two steps. The "dominant of two steps" bucket
shrinks; it does not disappear.

**It is not `workflow_step_decisions`.** That table has a live writer on the
Office approval path; its zero rows mean an unused feature, not a missing one.
The two mechanisms MUST NOT be merged.

**It is not a replacement for `session_step_history`.** This spec neither wires,
removes, nor changes that table or the `GET /api/v1/sessions/:id/workflow/history`
endpoint that reads it. See *Relationship to `session_step_history`*.

## What

The feature ships as two independently valuable slices. **Slice 1 SHALL ship and
be measured before Slice 2 is built** (see *Gate between slices*).

### Slice 1 — turn-start step stamp

- A turn created for a session records the workflow step its task was in at the
  moment the turn started.
- The stamp is present on every turn created after activation whose task has a
  workflow step, and **absent** — never empty, never `0` — when the task has
  none.
- The stamp is immutable once written. A step change during the turn does not
  rewrite it.
- No column is added to `task_session_turns`.

### Slice 2 — task-level transition ledger

- Every committed change to a task's workflow step produces exactly one ledger
  row, written in the same database transaction as the change.
- A row that is not committed with its transition does not exist: a rolled-back
  or WIP-rejected move leaves no row.
- A move that does not change the step (position-only reorder, re-issued move to
  the current step) produces **no** row.
- Each row names the task, the step it left and the step it entered, when, why
  (trigger), what kind of actor caused it, and that actor's **identifier** —
  never a display name.
- The initiating session is recorded when one exists and is genuinely the
  initiator; it is `NULL` otherwise. A transition with no session, or with
  several candidate sessions and no single initiator, MUST record `NULL` rather
  than pick one.
- Each row carries the collection-contract version it was written under.

### Cross-cutting

- Legacy rows and unstamped turns get `NULL` / absent — **never `0`, never
  `""`**.
- Both slices publish a machine-readable activation point, because the
  downstream extract is a point-in-time snapshot with no schema versioning: a
  column whose meaning changes mid-series is silently discontinuous.
- Both writers are observable at runtime, and the set of code paths that may
  mutate a task's workflow step is pinned by a test (see *Writer health*).

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
