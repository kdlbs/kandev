---
spec: docs/specs/tasks/requirements/workflow-task-step-transition-ledger.md
created: 2026-08-13
status: draft
---

# Implementation Plan: Task step transition ledger

## Overview

Two independently valuable slices, separated by a measurement gate the spec
mandates. Both need one shared prerequisite — a `telemetry_activations` registry
that publishes each contract's activation point — so that lands first. Slice 1
then adds the turn-start step stamp (one new key in an existing JSON column, no
schema change). After the gate, Slice 2 adds the `task_step_transitions` table,
wires the ledger write into the seven repository transactions that can mutate
`tasks.workflow_step_id`, gives real triggers and actors to the callers of those
transactions, and adds the three writer-health controls.

The order is dictated by dependency, not preference: the activation registry is
read by both writers; the ledger table must exist before the writer references
it; the writer must exist before callers can attribute to it; the pinning test
must run against a finished writer set.

**Prerequisite outside this feature.** The spec records that wiring
`session_step_history`'s dormant `CreateStepTransition` writer is separate work
that "lands first" because it touches the same code region
(`internal/orchestrator/event_handlers_workflow.go`,
`internal/orchestrator/workflow_store.go`). Confirm its state before starting
Task 05, which edits those files.

---

## Backend

### Area 1 — Telemetry contract activation registry (new package)

**`apps/backend/internal/telemetrycontract/`** — a small leaf package owning the
`telemetry_activations` table and the boot-time activation + health report. It is
deliberately generic: the spec frames this table as a registry for *any*
telemetry contract, and registers exactly two keys today.

`contract.go`:

```go
package telemetrycontract

// Contract describes one telemetry collection contract whose activation point
// and liveness must be readable from the database snapshot itself.
type Contract struct {
    Key         string // e.g. "turn.workflow_step_id_at_start"
    Version     int    // starts at 1; a bump APPENDS an activation row
    ExistsQuery string // SELECT returning 1 when the backing objects exist
    StatsQuery  string // SELECT (count, max_occurred_at) — max may be NULL
}

// Registry is the closed set of contracts this binary writes.
func Registry() []Contract
```

The two registrations:

| Key | Version | Backing objects | Stats source |
|---|---|---|---|
| `turn.workflow_step_id_at_start` | 1 | `task_session_turns` | rows whose `metadata` contains the stamp key; `started_at` as the recency column |
| `task_step_transitions` | 1 | `task_step_transitions` | `COUNT(*)`, `MAX(occurred_at)` |

`store.go`:

```go
// NewWithDB creates telemetry_activations idempotently and returns a Store.
func NewWithDB(writer, reader *sqlx.DB) (*Store, error)

// Activate writes one row per registered (key, version) that has none yet,
// stamped with this boot's UTC time. Never rewrites an existing row.
func (s *Store) Activate(ctx context.Context) error

// LogHealth emits one line per registered contract: object existence,
// activation time, row count, most recent occurred_at.
func (s *Store) LogHealth(ctx context.Context, log *logger.Logger)
```

DDL (dialect-split on the timestamp only; both dialects accept the rest):

```sql
CREATE TABLE IF NOT EXISTS telemetry_activations (
    contract_key     TEXT      NOT NULL,
    contract_version INTEGER   NOT NULL,
    activated_at     TIMESTAMP NOT NULL,
    PRIMARY KEY (contract_key, contract_version)
);
```

The composite primary key is load-bearing: a future version bump appends a
second activation row rather than overwriting the first. `Activate` uses
`INSERT ... ON CONFLICT DO NOTHING` (supported by both SQLite and pgx) so a
concurrent or repeated boot is a no-op.

**Wiring:** `apps/backend/internal/backendapp/storage.go`, immediately before the
existing `recordSchemaVersion(writer, ...)` call at line 101 — the comment there
already marks the point where every repository has finished `initSchema`, which
is exactly when a contract's backing objects are known to exist. `Activate` then
`LogHealth`. Neither is fatal: a failure logs and boot continues, matching
`recordSchemaVersion`'s contract.

### Area 2 — Turn-start step stamp (Slice 1)

**`apps/backend/internal/task/models/models.go`** — add beside
`TurnMetaKeyRuntimeConfigSnapshot` (line 232):

```go
// TurnMetaKeyWorkflowStepIDAtStart records the workflow step the turn's task
// was in when the turn started. Absent when the task held no step.
const TurnMetaKeyWorkflowStepIDAtStart = "workflow_step_id_at_start"
```

**`apps/backend/internal/task/service/service_turns.go`** — the whole of Slice 1's
write path is the two call sites of `runtimeConfigSnapshotMetadata(session)`:
`StartTurn` (line 44) and `createCompletedTurn` (line 79). Replace both with a
new method:

```go
// turnStartMetadata composes the turn's immutable start-of-turn metadata: the
// runtime config snapshot and the workflow step the task was in. The step stamp
// is independent of the runtime snapshot — a turn with nothing to snapshot still
// carries the stamp. A task read failure logs and omits the stamp; turn creation
// must never fail because telemetry could not be resolved.
func (s *Service) turnStartMetadata(ctx context.Context, session *models.TaskSession) map[string]interface{}
```

Behaviour, in order:

1. Start from `runtimeConfigSnapshotMetadata(session)`; if it returns `nil`,
   start from an empty map (do **not** return early — this is the spec scenario
   "no runtime config to snapshot, still stamped").
2. `s.tasks.GetTask(ctx, session.TaskID)`. On error: `s.logger.Debug`, record the
   absent-stamp metric, return the map as-is.
3. If `task.WorkflowStepID != ""`, set the stamp key. Empty string sets nothing —
   never `""`, never `nil`, never `0`.
4. Return `nil` when the map ended up empty, preserving today's "no metadata"
   shape for turns that carry neither piece.

`runtimeConfigSnapshotMetadata` keeps its current signature and semantics; it
becomes an input to the composer rather than the whole answer.

Immutability needs no code: nothing rewrites `task_session_turns.metadata` after
creation. Task 02 adds a regression test that pins that.

### Area 3 — Ledger schema (Slice 2)

**`apps/backend/internal/task/repository/sqlite/base_schema.go`** — a new
`initStepTransitionsSchema` step appended to the `initSchema` step list, placed
after `initSessionSchema` (the FK targets `task_sessions`) and before
`runMigrations`. The `id` column is dialect-split exactly as
`session_step_history` does it (`internal/workflow/repository/sqlite.go:88-96`):

```go
idCol := "id INTEGER PRIMARY KEY AUTOINCREMENT"
if dialect.IsPostgres(r.db.DriverName()) {
    idCol = "id BIGSERIAL PRIMARY KEY"
}
```

```sql
CREATE TABLE IF NOT EXISTS task_step_transitions (
    <idCol>,
    task_id               TEXT      NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    session_id            TEXT      REFERENCES task_sessions(id) ON DELETE SET NULL,
    from_workflow_id      TEXT,
    from_workflow_step_id TEXT,
    to_workflow_id        TEXT,
    to_workflow_step_id   TEXT,
    trigger               TEXT      NOT NULL,
    actor_kind            TEXT      NOT NULL,
    actor_id              TEXT,
    contract_version      INTEGER   NOT NULL,
    occurred_at           TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_task_step_transitions_task
    ON task_step_transitions(task_id, occurred_at, id);
CREATE INDEX IF NOT EXISTS idx_task_step_transitions_occurred
    ON task_step_transitions(occurred_at);
```

No foreign key to `workflow_steps` or `workflows` — deliberate, per the spec: a
deleted step must not delete the historical fact that a card was in it.

This is a new table, not a new column, so `CREATE TABLE IF NOT EXISTS` in the
init block is correct and complete; the root guide's "columns only via
`runMigrations`" rule does not apply. Nothing is backfilled.

### Area 4 — Attribution contract (new package, Slice 2)

**`apps/backend/internal/steptelemetry/`** — a leaf package holding the closed
enums, the ambient attribution, the row type, and the metrics. Imported by the
sqlite repository, the task service, the orchestrator, and the MCP handlers; it
imports none of them, so no cycle is possible.

```go
package steptelemetry

const ContractVersion = 1
const ContractKey = "task_step_transitions"

type Trigger string
const (
    TriggerTaskCreated       Trigger = "task_created"
    TriggerManualMove        Trigger = "manual_move"
    TriggerTaskUpdate        Trigger = "task_update"
    TriggerMCPMove           Trigger = "mcp_move"
    TriggerMCPDeferredMove   Trigger = "mcp_deferred_move"
    TriggerEngineTransition  Trigger = "engine_transition"
    TriggerWIPPull           Trigger = "wip_pull"
    TriggerBulkMove          Trigger = "bulk_move"
    TriggerUnarchiveRestore  Trigger = "unarchive_restore"
    TriggerWorkflowAttached  Trigger = "workflow_attached"
    TriggerWorkflowDetached  Trigger = "workflow_detached"
    TriggerUnknown           Trigger = "unknown"
)

type ActorKind string
const (
    ActorHuman       ActorKind = "human"
    ActorAgent       ActorKind = "agent"
    ActorSystem      ActorKind = "system"
    ActorIntegration ActorKind = "integration"
    ActorUnknown     ActorKind = "unknown"
)

// Attribution is who and why. The zero value is the spec's recorded fact for a
// caller that declares nothing: unknown trigger, unknown actor, no IDs.
type Attribution struct {
    Trigger   Trigger
    ActorKind ActorKind
    ActorID   string
    SessionID string
}

func WithAttribution(ctx context.Context, a Attribution) context.Context
func FromContext(ctx context.Context) Attribution // normalizes zero -> unknown/unknown
```

**Why the attribution travels on the context and not in method signatures.** The
seven repository entry points are reached from dozens of call sites across the
service, orchestrator, MCP handlers, and integration watchers. The spec requires
that a caller who supplies nothing gets `unknown`/`unknown` recorded as a fact
rather than an error — that is ambient-default semantics, not a required
parameter. The repository already receives the acting identity this way
(`authn.IdentityFromContext`), which is the same value that becomes `actor_kind
= human`, so this reuses an established seam rather than inventing one. Adding
four parameters to seven signatures and threading them through every caller
would produce a much larger diff for a value most callers pass through untouched.

Also in the package, mirroring `internal/office/scheduler/metrics_vars.go`:

```go
var (
    stepTransitionsTotal = expvar.NewMap("telemetry_step_transitions_total")
    turnStampsTotal      = expvar.NewMap("telemetry_turn_stamps_total")
)

// RecordLedgerRow bumps the expvar counter keyed by trigger AND emits a
// telemetry.metric.step_transition_written log line.
func RecordLedgerRow(log *logger.Logger, trigger Trigger)

// RecordTurnStamp bumps the counter keyed by present/absent AND emits
// telemetry.metric.turn_stamped.
func RecordTurnStamp(log *logger.Logger, present bool)
```

Log event names are the first field of the line so a log-aggregation rule matches
on the name without scanning free text — the `routing.metric.*` precedent.

### Area 5 — Ledger writer inside the mutating transactions (Slice 2)

**`apps/backend/internal/task/repository/sqlite/step_transitions.go`** (new):

```go
// stepTransitionTx is satisfied by both *sql.Tx and *sqlx.Tx, the two
// transaction types the mutation paths in this package use.
type stepTransitionTx interface {
    ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
    QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// readTaskStepInTx reads the task's current (workflow_id, workflow_step_id)
// inside the write transaction, taking a row lock on Postgres. Reading outside
// the transaction would break the chain invariant under concurrent moves.
func (r *Repository) readTaskStepInTx(ctx context.Context, tx stepTransitionTx, taskID string) (workflowID, stepID string, found bool, err error)

// recordStepTransition writes exactly one ledger row when the step actually
// changed. It is a no-op when fromStepID == toStepID (position-only reorder,
// re-issued move to the current step) and when both sides are empty.
func (r *Repository) recordStepTransition(ctx context.Context, tx stepTransitionTx, in stepTransitionInput) error
```

`readTaskStepInTx` issues `SELECT workflow_id, workflow_step_id FROM tasks WHERE
id = ?` with `FOR UPDATE` appended on Postgres (`dialect.IsPostgres`). On SQLite
the writer pool already serializes writers, so the plain read inside the write
transaction is sufficient.

`recordStepTransition` normalizes `""` to `NULL` on all four `from_*`/`to_*`
columns, reads `steptelemetry.FromContext(ctx)` for trigger/actor/session,
stamps `occurred_at = time.Now().UTC()` and `contract_version =
steptelemetry.ContractVersion`, and returns the INSERT error unchanged.

**The missing-table case needs no detection code and must not get any.** If the
`CREATE TABLE` was silently swallowed by the migration runner, the INSERT fails
with `no such table` / `relation does not exist`, that error returns, and the
enclosing transaction rolls back — which is precisely the spec's required
behaviour ("the transaction fails rather than committing a step change with no
row"). The rule for the implementer is therefore a negative one: never log-and-
continue on `recordStepTransition`'s error, and never wrap it in a table-exists
probe that would let the step change commit alone.

The seven chokepoints, each already holding a transaction except where noted:

| Function | File:line | Change |
|---|---|---|
| `insertTaskTx` (task creation) | `task.go:319` | Genesis row after the INSERT when `task.WorkflowStepID != ""`, `from_*` NULL. Reads the task struct, which by this point already carries the *actual* admission placement (`applyAdmissionPlacement`, `task.go:235`), so the feeder-placement scenario is satisfied for free. A task with no workflow writes nothing. |
| `UpdateTask` | `task.go:469` | `readTaskStepInTx` before the UPDATE; `recordStepTransition` after the `RowsAffected` check. |
| `UpdateTaskIfWorkflowStepHasCapacity` | `task.go:778` | Same. A WIP rejection returns before the UPDATE, so no row — as required. |
| `PromoteQueuedTaskIfWorkflowStepHasCapacity` | `task.go:830` | Same, but the UPDATE is conditional: record only when `rows > 0`. |
| `RestoreTaskMessageRollbackIfSessionState` | `task.go:2043` | Same conditional shape — the UPDATE is guarded by an `EXISTS` on session state; record only when `rows > 0`. |
| `AddTaskToWorkflow` | `workflow.go:17` | **Currently has no transaction.** Wrap in `r.db.BeginTx`, read-then-update-then-record. |
| `RemoveTaskFromWorkflow` | `workflow.go:25` | Same. Its UPDATE is guarded by `AND workflow_id = ?`; record only when `rows > 0`. `to_*` normalize to NULL from the `''` the statement writes. |

Archive, unarchive-in-place, cascade archive, and delete are untouched: none of
them write `workflow_step_id`, so none writes a row, which is the specified
behaviour rather than an omission.

### Area 6 — Caller attribution (Slice 2)

Each caller wraps its context once, as close to the entry point as possible.
Actor resolution is uniform: `authn.IdentityFromContext(ctx)` present →
`ActorHuman` with `Identity.UserID` (including the synthetic single-user identity
when auth is disabled); an agent session → `ActorAgent` with the session ID in
**both** `ActorID` and `SessionID`; no initiating identity → `ActorSystem` with
empty `ActorID`.

| Site | File | Attribution |
|---|---|---|
| `MoveTaskWithOptions` | `service/service_workflow.go:428` | `manual_move` + identity actor — **only if the context carries no trigger already**, so an `mcp_move` set by the MCP handler wins over the inner board-move default (the spec's outermost-caller rule). |
| `BulkMoveSelectedTasks`, `BulkMoveTasks` | `service/service_workflow.go:947,1049` | `bulk_move`, `SessionID` empty |
| `Service.UpdateTask` (request carries a new step) | `service/service_tasks.go:1362` | `task_update` |
| `promoteNextQueuedTask` / `pullNextTaskOnVacate` | `service/service_workflow.go:558,612` | `wip_pull`, `ActorSystem`, `SessionID` empty — even when the task has live sessions, since no one session is the initiator |
| `applyMoveTaskImmediate` | `mcp/handlers/config_task_handlers.go:176` | `mcp_move`, `ActorAgent`, actor+session = `session.ID` |
| `applyPendingMove` | `orchestrator/event_handlers_workflow.go:1243` | `mcp_deferred_move`, `ActorAgent` |
| `executeStepTransition` | `orchestrator/event_handlers_workflow.go:296` | `engine_transition`; `ActorAgent` when reached from a session turn, `ActorSystem` otherwise |
| `workflowStore.ApplyTransition` | `orchestrator/workflow_store.go:154` | Same, using its `sessionID` argument to decide agent vs system |
| Watcher-driven moves | `orchestrator/watcher_dispatch.go` + `source_jira.go` / `source_linear.go` | `ActorIntegration`, `ActorID` = watch ID |

Task creation's genesis row needs no wrapping: the writer hard-codes
`TriggerTaskCreated`, and the actor comes from the identity already on the
context.

Anything not wrapped records `unknown`/`unknown`, which is a recorded fact.

---

## Frontend

**None.** The spec's *API surface* section is explicit: no HTTP route, no
WebSocket event, no MCP tool, no frontend surface. Consumers read the database.
No store slice, hook, component, or API client changes.

---

## Tests

### Slice 1 — turn-start step stamp

| What | File | How |
|---|---|---|
| Task in step `S` → turn metadata carries `workflow_step_id_at_start` = `S` | `internal/task/service/service_turns_step_stamp_test.go` (new file — `service_turns_test.go` is near the 800-line revive limit) | Service test with a stub task repo; assert the persisted turn's metadata |
| Task with no step → key **absent**, not `""`/`null`/`0` | same | Assert key membership with the two-value map read, not a value comparison |
| No runtime config to snapshot → stamp present, `runtime_config_snapshot` absent | same | Session with empty profile snapshot and no runtime metadata |
| Task moves to `T` mid-turn → stamp still `S` | same | Move the task after `StartTurn`, re-read the turn |
| Task read fails → turn still created, no stamp | same | Task repo stub returning an error; assert `StartTurn` returns a turn and `err == nil` |
| Synthetic completed turn carries the same stamp | same | Exercise `createCompletedTurn` through its lifecycle-message caller |
| Pre-activation turn is not backfilled | `internal/task/repository/sqlite/step_stamp_migration_test.go` (new) | Seed a turn with metadata lacking the key, boot, assert unchanged |

### Slice 2 — ledger writes

| What | File | How |
|---|---|---|
| `A`→`B` manual move writes one row with correct `from`/`to`/trigger/actor | `internal/task/repository/sqlite/step_transitions_test.go` (new) | Table-driven over the trigger matrix against a real SQLite DB |
| No-op move to the current step writes no row | same | Assert row count unchanged |
| Position-only reorder writes no row | same | Same step, new position |
| WIP-rejected move writes no row and leaves the step unchanged | same | `UpdateTaskIfWorkflowStepHasCapacity` at capacity |
| Rolled-back transaction leaves no row | same | Force a failure after the UPDATE inside the tx |
| Genesis row on create: `from_*` NULL, trigger `task_created` | same | `CreateTask` into a step |
| Genesis row records the *feeder* step when WIP diverted the placement | same | `CreateTaskWithWorkflowStepAdmission` with a full target and a feeder |
| Task created with no workflow writes no row; later attach writes the first row with `workflow_attached` | same | Create bare, then `AddTaskToWorkflow` |
| Detach writes `to_*` NULL with `workflow_detached` | same | `RemoveTaskFromWorkflow` |
| Empty-string step normalizes to NULL on both sides | same | Seed `workflow_step_id = ''`, move in |
| No trigger/actor declared → `unknown`/`unknown`/NULL | same | Call the repo with a bare context |
| Ephemeral (quick-chat) task records exactly as any other | same | `IsEphemeral: true` |
| Archive / unarchive-in-place / cascade / delete write no row | `internal/task/repository/sqlite/step_transitions_lifecycle_test.go` (new) | Row count unchanged across each action |
| Row survives deletion of the workflow step it names | same | Delete the step, assert the row and its step ID |
| Missing ledger table fails the transaction rather than committing the step change alone | same | `DROP TABLE task_step_transitions`, attempt a move, assert error **and** unchanged `tasks.workflow_step_id` |
| Ledger row deleted with its task; survives session deletion with `session_id` NULL | same | Exercise both cascades |

### Ordering, chain, and concurrency

| What | File | How |
|---|---|---|
| `A`→`B`→`A`→`B` yields four rows with an intact chain | `internal/task/repository/sqlite/step_transitions_chain_test.go` (new) | Read ordered by `(occurred_at, id)`, assert each `from` equals the previous `to` |
| Rows sharing an `occurred_at` are totally ordered by `id` | same | Freeze the clock across two moves |
| Two concurrent moves → two rows, chain intact, last `to` equals the task's step | same | Two goroutines, `errgroup`, real DB |
| Last row's `to_workflow_step_id` equals `tasks.workflow_step_id` | same | Invariant assertion helper reused by every case above |
| Retried engine trigger with the same `OperationID` writes no second row | `internal/orchestrator/workflow_step_ledger_test.go` (new) | Replay the trigger through the engine's applied-operations store |

### Actor and privacy

| What | File | How |
|---|---|---|
| Auth disabled → `human` + synthetic identity's user ID | `internal/task/repository/sqlite/step_transitions_actor_test.go` (new) | Synthetic identity on the context |
| Jira/Linear watcher move → `integration` + watch ID | same | Attribution built by the watcher source |
| No display name, email, title, or prompt text in any column | same | Scan every text column of every row produced by the full trigger matrix against a fixture whose title/description/prompt are unique sentinels |

### Migration and activation

| What | File | How |
|---|---|---|
| Pre-feature DB gains both tables; existing tasks have no rows; existing turns have no stamp | `internal/task/repository/sqlite/step_transitions_migration_test.go` (new) | Mirrors `task_external_id_migration_test.go`: seed pre-migration rows, run migrations, assert |
| Migration runner replays twice more with no change | same | Call `NewWithDB` three times total |
| Postgres behaves identically | same, env-gated on `KANDEV_TEST_POSTGRES_DSN` | Same fresh-then-replay sequence |
| First boot writes one activation row per key at version 1 with that boot's UTC time | `internal/telemetrycontract/store_test.go` (new) | `Activate` on a fresh DB |
| Subsequent boot leaves the activation row unchanged | same | `Activate` twice, compare `activated_at` |
| A version bump appends a second row rather than overwriting | same | Register version 2, assert two rows |

### Writer health

| What | File | How |
|---|---|---|
| A new production statement mutating `tasks.workflow_step_id` fails the build until registered | `internal/task/repository/sqlite/step_transition_writers_pin_test.go` (new) | `go/parser` scans `apps/backend/internal` recursively; resolve package constants and `fmt.Sprintf` column setters, qualify identities by package and receiver, and assert set equality against the registered names |
| A ledger row bumps the expvar counter for its trigger and logs `telemetry.metric.step_transition_written` | `internal/steptelemetry/metrics_test.go` (new) | Read the expvar map; assert with `zaptest`'s observed logs |
| A stamped/unstamped turn bumps the turn counter and logs `telemetry.metric.turn_stamped` | same | Both branches |
| Boot emits one health line per registered contract with existence, activation, count, recency | `internal/telemetrycontract/health_test.go` (new) | Observed logs against a seeded DB |

---

## E2E Tests

**None.** The spec has zero user-visible changes: no UI, no API, no WebSocket
event. There is no observable outcome for Playwright to assert. Coverage is
entirely Go-side, including the real-database integration tests above.

---

## Verification Results

All six tasks (01–06) implemented and committed together in
`15beb3a003d214b8110e48f0a1b254b2c02daf9d` on `feature/tel-task-grain-step-bhh`
(37 files, +2,478/−4). One commit rather than one per task, given the
interlocking nature of the seven chokepoints and their callers — see the
per-task decision notes below for what would otherwise have been split.

**Slice 1 gate.** No production deployment exists to measure two weeks of
post-activation turns against. Per the spec's own default ("If the
measurement is inconclusive... Slice 2 proceeds as specified"), this counts
as inconclusive and Slice 2 proceeded as specified in the same session.

**Commands run (backend, `apps/backend`), all green:**
- `go build ./...`
- `go test ./internal/telemetrycontract/... ./internal/steptelemetry/... ./internal/task/repository/sqlite/... ./internal/task/service/... ./internal/task/... ./internal/orchestrator/... ./internal/mcp/... ./internal/office/engine_adapters/... ./internal/backendapp/...`
- `go test -race` on the same set (chain/concurrency tests specifically require `-race`)
- `go test -run 'Postgres|Activation|StepTransition' ...` — env-gated legs all
  **skipped** (`KANDEV_TEST_POSTGRES_DSN` unset in this environment); the
  Postgres-parity test files exist and compile (`store_postgres_test.go`,
  `step_transitions_postgres_test.go`, `step_transitions_writer_postgres_test.go`)
  and must be run in CI's postgres-boot job.
- `golangci-lint run ./...` (backend) — 0 issues
- `golangci-lint run ./... --new-from-rev=db4fc039ac637766d6ec30e996918c538303021d` — 0 issues
- Pinning-test bite check: temporarily added an unregistered
  `UPDATE tasks SET workflow_step_id = ?` statement to `task.go`, confirmed
  `TestStepTransitionWritersArePinned` failed naming it, removed the
  statement, confirmed `git diff --check` was clean and the test passed again.

**Root gauntlet, all green:**
- `make fmt` (backend + web; caught and fixed one stray smart-quote
  corruption in a comment during a fmt pass — reverted to plain `""`)
- `make typecheck` — required generating `apps/web/lib/generated/{changelog,release-notes}.json`
  via `node scripts/generate-release-notes.mjs && node scripts/generate-changelog.mjs`
  first (a `predev`/`prebuild` step this checkout had never run); clean after.
- `make lint` (backend + web + harness + architecture) — 0 issues
- `make lint-format` — clean
- `cd apps/web && pnpm run i18n:ratchet` — clean (no UI source touched)
- `make test` (test-backend + test-web + test-cli + test-scripts) — 5 failures,
  all proven pre-existing/environmental by reproducing identically against
  merge-base `db4fc039a` in scratch worktrees, then removed:
  - `internal/repoclone` `TestEnsureWorkspaceClonedWithBasicAuthKeepsCredentialScopedToGitChild/context_cancellation` — flaky at merge-base too (3 reruns, mixed pass/fail both there and on this branch)
  - `internal/system/storage/workspaces` (3 tests) — reproduces identically at merge-base; macOS temp-dir "not a directory" environment issue
  - `apps/web` `lib/http-git-server.test.ts` (3 tests) — no Docker daemon in this sandbox; reproduces identically at merge-base
  - `apps/web` `storage-maintenance-settings.test.tsx` (3 tests) — 5s timeout under this host's heavy concurrent-session CPU load; reproduces at merge-base
  - `scripts/pr-state` "threads count" — file is byte-identical to merge-base (`git diff` empty), so trivially pre-existing (bash-version array-expansion issue)

**Deliberate scope decisions made during Build (beyond the ones recorded
during Spec), for review:**
1. **Genesis-row and three other chokepoints hardcode their trigger at the
   writer, not via caller wiring.** `insertTaskTx` (task_created),
   `RestoreTaskMessageRollbackIfSessionState` (unarchive_restore),
   `AddTaskToWorkflow` (workflow_attached) — via its sole caller,
   `WorkflowSwitcherAdapter.SwitchTaskWorkflow` — and the two
   `pullNextTaskOnVacate` implementations (service and orchestrator; wip_pull)
   each have exactly one semantic meaning regardless of caller, so Task 05's
   plan (which only enumerated caller-side wiring) was extended to cover
   these four gaps against the spec's own Path→Trigger table, which lists all
   eleven mappings as fixed and caller-independent for these paths.
2. **`RemoveTaskFromWorkflow` has zero production callers** on this branch
   (confirmed by repo-wide grep) — same situation the spec notes for
   `session_step_history`'s `CreateStepTransition`. It is wired identically
   to `AddTaskToWorkflow` (workflow_detached) so it is correct the moment a
   caller appears; there was nothing to wire attribution *to*.
3. **Watcher-driven Jira/Linear activity in this codebase only creates
   tasks, never moves an existing one** (no `MoveTask` call site found under
   `internal/jira`, `internal/linear`, or the watcher dispatch/source files).
   The spec's "watcher moves a card → actor integration" scenario is
   satisfied by wiring the *genesis* row's actor to `ActorIntegration` +
   `WatchID` for watcher-originated creates instead, via a small extension
   to `genesisAttribution` that prefers a caller-set actor over the
   identity-derived default.
4. **The ledger-row expvar/log counter fires at INSERT time inside the
   transaction, not after commit.** A later statement in the same
   caller-owned transaction failing after the ledger INSERT (rare — e.g. the
   runner sync in `UpdateTask`) rolls the row back but the counter still
   incremented. Documented in `step_transitions.go`; treated as acceptable
   because the counter is a liveness signal ("is the writer alive"), not a
   row-for-row audit trail — the ledger table itself is that audit trail.
5. **`e1cbdf22` (wire `session_step_history`) had not landed** on this
   branch at Build time (`CreateStepTransition` still had zero production
   callers, reconfirmed before editing `event_handlers_workflow.go` /
   `workflow_store.go`), so this card proceeded as the sole writer into
   those two files.

---

## Testing Step — QA Pass And Scenario Audit

Committed as `4cbcc8f47` ("test: close ledger coverage gaps for cross-workflow
and cascade-archive"), 4 files, +212.

**Proof-level determination.** `git diff --name-only origin/main...HEAD` lists
41 changed files, all under `apps/backend/`, `docs/plans/`, `docs/specs/` —
zero `apps/web/` files. Per the mechanical rule (E2E required only for
non-exempt `apps/web/` changes), no E2E is required; backend-only proof.

**Attack pass.** Traced the full watcher → genesis-row-actor context
attribution pipeline line by line (`watcher_dispatch.go` →
`backendapp/orchestrator.go` → `service_tasks.go`; no `context.Background()`
reset points found). Verified the SQLite-writer-serialization claim against
`internal/db/sqlite.go`'s `SetMaxOpenConns(1)`. Reconfirmed
`RemoveTaskFromWorkflow` still has zero production callers. Scanned the full
diff for debug/TODO/panic red flags (none found). Identified one dead-code
no-op branch in `recordStepTransition` (the empty-string/empty-string check
is subsumed by the equal-values check above it) — harmless, left as-is rather
than churning a working file for a no-op cleanup.

**Test-honesty audit** against all ~47 GIVEN/WHEN/THEN scenarios in
`docs/specs/tasks/requirements/workflow-task-step-transition-ledger.md` § Scenarios, by
category:

| Category | Scenarios | Result |
|---|---|---|
| Slice 1 — turn-start step stamp | 7 | fully covered |
| Slice 2 — ledger writes | 19 | 2 gaps found + closed |
| Ordering, chain, and concurrency | 7 | 2 gaps found + closed |
| Actor and privacy | 3 | fully covered |
| Migration and activation | 6 | fully covered |
| Writer health | 3 | fully covered |

Migration/activation and writer-health scenarios are each satisfied by more
than one existing test working together (e.g. the "ledger table absent"
scenario is proven jointly by
`TestMissingLedgerTableFailsTransactionRatherThanCommittingStepChangeAlone`
and `TestLogHealthEmitsOneLinePerRegisteredContract`) — no new tests were
needed there.

**4 coverage gaps found and closed, zero production defects found:**

1. **Cross-workflow engine transition had no ledger-row-level test.** The
   only existing test,
   `TestWorkflowStore_ApplyTransitionSyncsWorkflowIDAcrossWorkflows`, asserts
   on `task.WorkflowID`/`task.WorkflowStepID` but never queries
   `task_step_transitions`, so the spec's "one row is written whose
   `from_workflow_id` and `to_workflow_id` differ" claim was unverified.
   Closed by `TestApplyTransitionCrossWorkflowRecordsDifferingWorkflowIDs`
   (`internal/orchestrator/workflow_step_ledger_test.go`), which mirrors that
   test's `wf2`/`step-wf2` setup and additionally reads the last ledger row.
2. **Cascade-archive (`ArchiveTaskIfActive`) had no direct behavioral test**
   asserting it writes no ledger row — structurally guaranteed by the
   pinning test's exclusion (its SQL never touches `workflow_step_id`), but
   untested at the ledger level. Closed by `TestArchiveTaskIfActiveWritesNoRow`
   (`internal/task/repository/sqlite/step_transitions_lifecycle_test.go`).
3. **Backwards host-clock correction was untested.** No test proved the
   chain invariant (`from == previous row's to`) holds when rows are read in
   `id` order after `occurred_at` runs backwards, or that the ledger is not
   "repaired"/reordered by timestamp. Closed by
   `TestChainSurvivesBackwardsClockCorrectionWhenOrderedByID`
   (`internal/task/repository/sqlite/step_transitions_chain_test.go`), which
   forces a row's `occurred_at` earlier than the genesis row's and confirms
   both the chain and the raw (unrepaired) value survive under `id` ordering.
4. **Turn-stamp vs. ledger-row disagreement was untested.** The spec's
   explicit cross-slice scenario ("they disagree, and neither is treated as
   an error") had no test reading both the turn's stamp and the ledger's
   last row after a mid-turn move. Closed by
   `TestTurnStampAndLedgerRowCanDisagreeWithoutError`
   (`internal/task/service/service_turns_step_stamp_test.go`).

All 4 new tests pass fresh (non-cached); no existing test was weakened,
skipped, or had its assertions loosened.

**Commands run (backend, `apps/backend`), fresh receipts:**
- `go clean -testcache && go build ./...` — clean.
- `go test ./internal/orchestrator/... ./internal/task/repository/sqlite/... ./internal/task/service/... ./internal/steptelemetry/... ./internal/telemetrycontract/... ./internal/mcp/handlers/...` — all `ok`, fresh.
- `go test -race ./internal/orchestrator/... ./internal/task/repository/sqlite/... ./internal/task/service/...` — one `internal/orchestrator` goroutine-leak failure in `workflow_e2e_test.go`'s `runWorkflowTestCase` (a file this card never touches); re-ran `go test -race ./internal/orchestrator/...` alone immediately after and it passed clean — confirmed timing flake, not a regression.
- `make -C apps/backend lint` — `0 issues.`
- `make -C apps/backend test lint` — `test` target failed, which blocks `make`'s sequential `lint` prerequisite (same behavior seen during Build), so `lint` was re-run standalone (above, clean). `test` failures, both pre-existing:
  - `internal/repoclone` (1 test) — re-ran `go test ./internal/repoclone/... -v` standalone and it passed clean; same known timing flake documented in the Build step's Verification Results above.
  - `internal/system/storage/workspaces` (same 3 tests, same `open dependency workspace root: not a directory` error text as the Build step) — already proven pre-existing against merge-base `db4fc039a` via scratch worktree during Build; not re-proven again since it is the identical failure mode with receipts already on file above.

**E2E:** none, confirmed twice — the spec states zero user-visible surfaces,
and this step's mechanical file-diff check independently confirms zero
`apps/web/` changes on the branch.

---

## Review Round 1 / Round 2 — test-rigor follow-up on the writer-health pinning gate

A later Kandev task (`fix-mcp-deferred-mov_428h3jqo`) opened a follow-up card
strengthening `TestStepTransitionWritersArePinned` and its supporting
detector (`findWorkflowStepIDMutators` in
`step_transition_writers_pin_test.go`) against seven mutation-proven false
negatives, then ran two adversarial review rounds against the fix (a
Kandev-internal review leg plus an independent cross-vendor `codex` leg each
round). This entry is the durable receipt Review Round 1 required and Review
Round 2 confirmed was still missing from this file after Build round 2's
commit message (`84a466d44`) claimed — inaccurately — that it had already
been recorded here. Recording it now closes that gap.

**What Round 1 found and Round 2 confirmed fixed:**
1. The production scan was rooted at this package's own directory (`"."`),
   not the whole backend source tree, so recursion into subdirectories bought
   the gate nothing against a writer added in a sibling package. Fixed by
   `findBackendSourceRoot`, which walks up to `apps/backend/internal`.
2. The Postgres concurrency test relied on a `sync.WaitGroup` barrier, which
   guarantees simultaneous start but not actual lock contention — reintroducing
   the historical bug it exists to catch produced only 7/20 failures (~35%).
   Fixed by `TestPostgresReadTaskStepInTxBlocksOnConcurrentRowLock`, a
   channel-gated deterministic lock-hold test mirroring
   `turn_step_stamp_postgres_test.go`'s existing pattern. The test now polls
   `pg_stat_activity` to confirm that the competing backend is waiting on the
   row lock, and it uses the repository clock seam to prove the transition
   timestamp is sampled after lock release; the same reintroduced bug fails
   5/5.
3. (Round 2, new) `funcIdentity()` keyed registered writers as
   `"ReceiverType.FuncName"` with no package qualification. Since this
   codebase declares a type literally named `Repository` in 11 different
   backend packages, a same-named method in any of them collided with an
   already-registered entry and the gate missed it. Fixed by qualifying the
   identity with the writer's package directory relative to the scan root
   (`"task/repository/sqlite/Repository.updateTaskTx"`), plus a permanent
   regression fixture (`TestFindWorkflowStepIDMutatorsQualifiesByPackage`)
   proving two packages with colliding receiver-type-and-method names are now
   reported as two distinct writers, not collapsed into one.

**Both-ways (fail-then-pass) receipt for the evasive-shapes fixture test,**
run against the actual pre-fix detector, not reconstructed from memory:

```
$ git show 9fe867f9f:apps/backend/internal/task/repository/sqlite/step_transition_writers_pin_test.go \
  > /tmp/pin_old_detector_source.go
# extracted the pre-fix functionMutatesWorkflowStepID/literalMutatesWorkflowStepID
# (BasicLit-only — no Sprintf handling, no const resolution, bare-name keys —
# i.e. exactly origin/main's shape before this card's fixes) into a standalone
# `go run` program and ran it against the current three fixture files.
$ go run /tmp/pin_old_detector_repro/main.go
found["constHoistedStepMutator"]        = false   # (a) unreachable: literal is const-hoisted, old detector only walked BasicLit
found["sprintfInterpolatedStepMutator"] = false   # (b) unreachable: old detector had no Sprintf handling at all
found["updateTaskTx"]                   = true    # (c) RepoA and RepoB collapse into one bare-name key — a duplicate masked as a single hit, not a miss
```

3 of the 5 fixture shapes were genuinely undetectable (or miscounted) under
the pre-fix detector, confirming the fixtures encode real false negatives,
not hypothetical ones. Post-fix, all 5 pass —
`TestFindWorkflowStepIDMutatorsCatchesEvasiveShapes`, run fresh:

```
$ go test ./internal/task/repository/sqlite/ -run TestFindWorkflowStepIDMutatorsCatchesEvasiveShapes -v -count=1
--- PASS: TestFindWorkflowStepIDMutatorsCatchesEvasiveShapes (0.00s)
```

**Package-qualification receipt (Round 2, Finding A), live decoy in a real
backend package, not a synthetic fixture:**

```
$ cat > internal/office/repository/sqlite/zz_review_collision_tmp.go   # package sqlite, a DIFFERENT
package sqlite                                                        # backend package than the real
                                                                       # writer's internal/task/repository/sqlite
func (r *Repository) updateTaskTx() string {
	return "UPDATE tasks SET workflow_step_id = ? WHERE id = ?"
}

$ go build ./internal/office/...                                                    # clean
$ go test ./internal/task/repository/sqlite/ -run TestStepTransitionWritersArePinned -v -count=1
--- FAIL: TestStepTransitionWritersArePinned
    function(s) [office/repository/sqlite/Repository.updateTaskTx] contain a
    statement that mutates tasks.workflow_step_id but are not registered...
$ rm internal/office/repository/sqlite/zz_review_collision_tmp.go
$ git status --porcelain internal/office/ internal/task/                            # empty — clean revert
```

Before the Round 2 fix this exact decoy passed silently (the collision with
the already-registered `"Repository.updateTaskTx"` entry masked it); after
the fix it is reported under its own package-qualified identity and fails
the gate as intended.

**Process note, disclosed rather than omitted:** while drafting this card's
own Kandev task plan during Round 2, the reviewer wrote a sentence claiming
the evasive-shapes both-ways receipt above had already been re-verified
before that verification had actually been run — the same class of
unsubstantiated claim this section exists to prevent. It was caught before
being reported anywhere external, and the real verification (shown above)
was performed immediately after. Recorded here so the fix's provenance is
honest rather than convenient.

---

## Implementation Waves And Parallel Candidates

Sequential by default. The two `parallel-safe` labels below are the only tasks
whose files are genuinely disjoint from a concurrent sibling; every other task
edits `internal/task/repository/sqlite/` or a file an earlier task created.

```
Wave 1 — shared foundation:
- [ ] [task-01-telemetry-activation-registry](task-01-telemetry-activation-registry.md)

Wave 2 — Slice 1 (ships and is measured alone):
- [ ] [task-02-turn-start-step-stamp](task-02-turn-start-step-stamp.md)

=== GATE: measure Slice 1 before building Slice 2 (see below) ===

Wave 3 — Slice 2 persistence:
- [ ] [task-03-ledger-schema-migration](task-03-ledger-schema-migration.md)

Wave 4 — Slice 2 writer:
- [ ] [task-04-ledger-writer-chokepoints](task-04-ledger-writer-chokepoints.md)

Wave 5 — Slice 2 attribution:
- [ ] [task-05-caller-attribution-wiring](task-05-caller-attribution-wiring.md)

Wave 6 — Slice 2 health controls:
- [ ] [task-06-writer-health-controls](task-06-writer-health-controls.md)
```

**The gate is a real stop, not a formality.** Per the spec, Slice 1 ships alone
and is measured on a store with at least two weeks of post-activation turns:
report the share of post-activation turns carrying the stamp, and the change in
the 47.0 / 30.5 / 22.5 attribution split when `turn_start` is admitted as a
basis. If the measurement is inconclusive — too few turns, or a change inside
noise — the recorded default is that **Slice 2 proceeds as specified**. The gate
exists to let evidence shrink Slice 2, not to let missing evidence stall it.

Task 04 is the only task in the plan that changes committed behaviour on every
step change in the product. It is deliberately scoped to leave the ledger fully
working with `unknown` triggers, so that Task 05 — which touches the orchestrator
and MCP handlers — can be reviewed as attribution quality rather than as
correctness of the write path.

## Open Questions

- **`session_step_history` wiring order.** The spec records that wiring that
  table's dormant writer is separate work that lands first, because it edits
  `internal/orchestrator/event_handlers_workflow.go` and
  `internal/orchestrator/workflow_store.go` — the same two files Task 05 edits.
  Confirm whether that work has landed before starting Task 05; if it has not,
  Task 05 is the second writer into those files and should expect conflicts.
