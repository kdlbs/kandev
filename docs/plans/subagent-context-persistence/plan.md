---
spec: docs/specs/agents/requirements/subagent-context-persistence.md
created: 2026-08-13
status: draft
---

# Implementation Plan: Subagent context persistence

> **Current implementation boundary (2026-08-15).** The execution-aware
> uniqueness key is the initial schema contract. This plan retains the original
> design notes below, but the supported implementation does not include a
> compatibility migration for the unshipped two-column table shape. See
> [ADR-2026-08-15-subagent-context-schema-boundary](../../decisions/2026-08-15-subagent-context-schema-boundary.md).

> The load-bearing differences from the original draft are:
>
> - **Uniqueness key** is `(task_session_id, agent_execution_id, tool_call_id)`,
>   not `(task_session_id, tool_call_id)`. `task_session_subagents` carries a
>   new `agent_execution_id TEXT NOT NULL DEFAULT 'unknown'` column (AC-31).
>   A live write with no execution identity available stores the `'unknown'`
>   sentinel and the service increments `unknown_execution`.
> - The final table is created by `initSubagentContextSchema`; `runMigrations()`
>   only performs the historical-message backfill and writes its two activation
>   keys. There is no execution-schema activation key or shape probe.
> - **The backfill and the activation-key writes are one transaction**, not
>   three independent `r.migrate.Apply(...)` calls: `tx.Beginx()` /
>   `defer tx.Rollback()` wraps the `INSERT ... SELECT`, the `capture_since`
>   write, and the `backfill_through` write, then `tx.Commit()`. A key is
>   therefore never written after a swallowed backfill failure — the whole
>   transaction rolls back together, so this is already the atomic-with-success
>   behavior a docs-only staleness report once flagged as missing.
> - **`RecordSubagentContextRequest` carries `ExecutionID string`**, propagated
>   from the orchestrator's frame handling through to the repository upsert's
>   conflict key.
>
> The "Schema (task 01)" through "Orchestrator wiring (task 04)" sections below
> are retained as the historical design record; the boundary above is
> authoritative for the current implementation.

## Overview

Add one new table, `task_session_subagents`, and a write path that fills it from
the two orchestrator frame handlers that already parse the subagent payload. The
order is forced by dependency: table DDL and the one-time backfill first, then
the repository upsert that encodes every merge rule, then the service-level
writer that owns normalization and the `expvar` counters, then the orchestrator
call sites, and finally the Postgres parity and health-query coverage. Nothing
user-facing changes, so there is no frontend and no E2E work.

The whole feature is additive telemetry. The governing constraint from the spec
is that it must be structurally impossible for this writer to break the product
path (AC-27), and that an unreported measurement must never be stored as `0`
(AC-7, and the regression named in the spec's *The one shape a builder must not
get wrong*).

---

## Backend

### Schema (task 01)

New table created in the schema-init DDL, in
`apps/backend/internal/task/repository/sqlite/base_schema.go`, as a new
`initSubagentContextSchema()` called from `initSessionSchema()` immediately after
`initMessageTurnSchema()`.

**On the spec's "DDL *and* migration" wording.** `apps/backend/AGENTS.md`
§ *Schema & migrations* is explicit that `initSchema()` runs every
`CREATE TABLE IF NOT EXISTS` step **before** `runMigrations()`, on every boot.
A *new table* is therefore fully created on existing databases by the schema-init
DDL alone, which satisfies AC-17. The "add it only via a migration" rule in that
document governs **new columns on existing tables**, which this feature has none
of. Do **not** duplicate the `CREATE TABLE` into `base_migrations.go` — the
migration entry for this feature is the backfill and the activation keys, and
nothing else.

Columns exactly as the spec's *Data model* table. The three measurement columns
carry **no `DEFAULT`**:

```sql
CREATE TABLE IF NOT EXISTS task_session_subagents (
    id                  TEXT PRIMARY KEY,
    task_session_id     TEXT NOT NULL,
    task_id             TEXT NOT NULL,
    turn_id             TEXT,
    tool_call_id        TEXT NOT NULL,
    parent_tool_call_id TEXT,
    subagent_type       TEXT,
    description         TEXT,
    agent_id            TEXT,
    child_session_id    TEXT,
    model               TEXT,
    agent_status        TEXT,
    tool_status         TEXT,
    is_async            INTEGER NOT NULL DEFAULT 0,
    total_tokens        INTEGER,
    tool_use_count      INTEGER,
    duration_ms         INTEGER,
    source              TEXT NOT NULL DEFAULT 'live',
    observed_at         TIMESTAMP NOT NULL,
    settled_at          TIMESTAMP,
    updated_at          TIMESTAMP NOT NULL,
    UNIQUE (task_session_id, tool_call_id),
    FOREIGN KEY (task_session_id) REFERENCES task_sessions(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_subagents_session_id ON task_session_subagents(task_session_id);
CREATE INDEX IF NOT EXISTS idx_subagents_task_id    ON task_session_subagents(task_id);
CREATE INDEX IF NOT EXISTS idx_subagents_turn_id    ON task_session_subagents(turn_id);
```

No `FOREIGN KEY` on `turn_id` — deliberate, per the spec: a turn deletion must
not delete the fan-out record.

### Migration: backfill and activation keys (task 01)

`apps/backend/internal/task/repository/sqlite/base_migrations.go`, a new
`migrateSubagentContextBackfill()` called from `runMigrations()` **after** the
existing statements. Three `r.migrate.Apply(...)` calls, all argument-free SQL
(`MigrateLogger.Apply` takes no bind args, and its swallow-plus-`WARN`
contract at `internal/db/migratelog.go:33` is what AC-20 is written against):

1. `task_session_subagents.backfill` — one unbounded
   `INSERT INTO task_session_subagents (...) SELECT ... FROM task_session_messages m
   WHERE <kind> = 'subagent_task' AND ... ON CONFLICT (task_session_id, tool_call_id) DO NOTHING`.
   No `LIMIT`, no batching (AC-23a).
2. `kandev_meta.subagent_context_capture_since` — write-once
   `INSERT ... ON CONFLICT(key) DO NOTHING` with an `fmt.Sprintf`-inlined
   `time.Now().UTC().Format(time.RFC3339)` literal.
3. `kandev_meta.subagent_context_backfill_through` — write-once, value from a
   subselect `COALESCE(MAX(m.created_at), '')` over the same predicate as (1).

`kandev_meta` is guaranteed to exist: `persistence.Provide` calls
`ensureMetaTable` (`internal/persistence/provider.go:72`) before
`repository.Provide` builds the task repo (`internal/backendapp/storage.go:41`).

Backfill derivations, all via dialect-aware helpers in the same file:

| Target | Source | Notes |
|---|---|---|
| `id` | `m.id` | The message's own UUID. Deterministic, so a replay maps to the same row and `DO NOTHING` is a true no-op. |
| `task_session_id`, `task_id` | `m.task_session_id`, `m.task_id` | Rows with `task_id = ''` are excluded by the `WHERE` (AC-2). |
| `turn_id` | `NULLIF(m.turn_id, '')` | |
| `tool_call_id` | `metadata.tool_call_id` | Excluded by `WHERE` when empty (AC-2). |
| `parent_tool_call_id` | `metadata.parent_tool_call_id` | Top-level metadata key, not inside `normalized`. |
| `agent_status` | `metadata.normalized.subagent_task.status` | |
| `tool_status` | `metadata.status` | The ACP status the message row already stores. |
| `observed_at` | `m.created_at` | |
| `settled_at` | `m.updated_at` when the derived `tool_status` is terminal, else NULL | Terminal set mirrors `isTerminalToolStatus`. |
| `source` | literal `'backfill'` | |

Three new helpers beside `jsonColumn` / `jsonText`
(`base_migrations.go:319-335`), each reusing `jsonColumn`'s empty/`'null'`
normalization so one malformed metadata document cannot abort the statement
(AC-23b):

- `jsonKey(postgres, column, key string) string` — a **top-level** text extract
  (`#>> '{key}'` / `json_extract(..., '$.key')`), needed for `tool_call_id`,
  `parent_tool_call_id`, `status`, and `normalized.kind`.
- `jsonInt(postgres, column, parent, key string) string` — `CAST` of the text
  extract to `BIGINT` (Postgres) / `INTEGER` (SQLite), wrapped so an empty
  extract and a **negative** value both yield NULL (AC-9, AC-23).
- `jsonBoolToInt(postgres, column, parent, key string) string` — `1` / `0`.
  Postgres `#>>` yields `'true'`; SQLite `json_extract` yields `1`. The helper
  must accept both spellings, which is the whole reason it exists.

`total_tokens` and `duration_ms` are `omitempty int64` on the payload, so a
reported `0` never reaches the JSON and absent-means-NULL falls out for free.
`tool_use_count` is `*int` with `omitempty`, so it is present in the JSON exactly
when the agent reported it — including a genuine `0` (AC-8).

### Model and repository upsert (task 02)

`apps/backend/internal/task/models/subagent_context.go`:

```go
type SubagentContext struct {
    ID, TaskSessionID, TaskID, ToolCallID string
    TurnID, ParentToolCallID, SubagentType, Description *string
    AgentID, ChildSessionID, Model, AgentStatus, ToolStatus *string
    IsAsync       bool
    TotalTokens   *int64
    ToolUseCount  *int64
    DurationMs    *int64
    Source        string
    ObservedAt    time.Time
    SettledAt     *time.Time
    UpdatedAt     time.Time
}
```

Every nullable column is a pointer. This is not style: `ToolUseCount` must
distinguish a reported `0` from "not reported", and a non-pointer `int64` cannot.

`apps/backend/internal/task/repository/sqlite/subagent_context.go`:

- `UpsertSubagentContext(ctx context.Context, sc *models.SubagentContext) error`
  — a **single** `INSERT ... ON CONFLICT (task_session_id, tool_call_id) DO UPDATE SET ...`.
  No read-then-write anywhere (AC-14). Every merge rule lives in the conflict
  clause:

  | Column | Conflict expression | AC |
  |---|---|---|
  | `turn_id` | `COALESCE(task_session_subagents.turn_id, excluded.turn_id)` | AC-2a — the *original* turn wins; a later turn never re-attributes |
  | text fields | `COALESCE(excluded.<c>, task_session_subagents.<c>)` | AC-4, AC-5 — fill-forward, never blank |
  | `tool_status` | `CASE WHEN task_session_subagents.settled_at IS NOT NULL THEN task_session_subagents.tool_status ELSE COALESCE(excluded.tool_status, ...) END` | AC-12 — frozen at the terminal value |
  | `is_async` | `CASE WHEN excluded.is_async = 1 THEN 1 ELSE task_session_subagents.is_async END` | sticky, never reset to 0 |
  | metrics | `COALESCE(excluded.<c>, task_session_subagents.<c>)` | AC-4, AC-7 |
  | `settled_at` | `COALESCE(task_session_subagents.settled_at, excluded.settled_at)` | AC-11 — write-once |
  | `updated_at` | `excluded.updated_at` | |
  | `source`, `observed_at`, `id` | **absent from the SET list** | write-once: a backfilled row is never relabelled `live` (AC-22), and first observation wins (AC-1a) |

  Both dialects accept `excluded.` on the right and unqualified names on the
  left; the self-qualified `task_session_subagents.<c>` reads are valid on both.
  Statement built once as a package-level `const` and passed through
  `r.db.Rebind`, matching `review.go:27` and `document.go:291`.

- `ListSubagentContextsBySession(ctx, sessionID string) ([]*models.SubagentContext, error)`
  and `ListSubagentContextsByTurn(ctx, turnID string)` — both
  `ORDER BY observed_at ASC, tool_call_id ASC` (AC-13). These are the read seam
  AC-13 is written against and the seam the tests assert ordering through. They
  are repository-level only: no service method, no DTO, no route. The spec's
  *Out of scope* forbids a read API, not a repository accessor.

### Service writer and counters (task 03)

`apps/backend/internal/task/service/subagent_context.go`, one entry point:

```go
type RecordSubagentContextRequest struct {
    TaskSessionID, TaskID, TurnID string
    ToolCallID, ParentToolCallID  string
    ToolStatus                    string
    Payload                       *streams.SubagentTaskPayload
    ObservedAt                    time.Time
}

func (s *Service) RecordSubagentContext(ctx context.Context, req RecordSubagentContextRequest)
```

**It returns nothing.** That is the mechanism by which AC-27 holds: a writer
with no error return cannot fail the enclosing message write, turn, or stream,
and no future caller can accidentally propagate one. It owns:

- Identity gate — empty `TaskSessionID`, `TaskID`, or `ToolCallID` ⇒ no write,
  `skipped_no_identity`++, return (AC-2).
- `nilIfEmpty(string) *string` applied to every nullable TEXT field, so `""`
  becomes NULL and `COUNT(model)` counts models (spec § *Column-level
  normalization rules*).
- Metric normalization: `DurationMs`/`TotalTokens` map to `*int64` and are left
  nil when zero-or-absent; `ToolUseCount` maps to `*int64` **only** when the
  payload pointer is non-nil, preserving a reported `0` (AC-7, AC-8). Any
  negative value ⇒ that field nil **and** `anomalous_value`++, other fields
  unaffected (AC-9).
- Terminality: `settled_at = &req.ObservedAt` only when
  `orchestrator`-equivalent terminal classification of `req.ToolStatus` is true.
  `agent_status` is stored verbatim and never consulted (AC-10a, AC-11). The
  classifier is unexported in `internal/orchestrator`, so the service gets its
  own `isTerminalToolStatus` over the identical set
  (`complete`, `completed`, `success`, `error`, `failed`, `cancelled`) with a
  comment naming `event_handlers_streaming.go:762` as the peer, plus a test that
  pins the two lists together.
- Failure handling: repository error ⇒ `WARN` with session id and tool call id,
  `failed`++, return (AC-27).
- `attempted`++ on entry past the nil-payload check; `persisted`++ on success.

`apps/backend/internal/task/service/subagent_context_metrics.go` — one
`expvar.NewMap("subagent_context_total")` with the five keys AC-26 names
(`attempted`, `persisted`, `skipped_no_identity`, `anomalous_value`, `failed`),
following `internal/office/scheduler/metrics_vars.go` (package-level `NewMap`,
counters only, no gauges).

### Orchestrator wiring (task 04)

`apps/backend/internal/orchestrator/service.go` — a narrow interface beside
`MessageCreator`:

```go
type SubagentContextRecorder interface {
    RecordSubagentContext(ctx context.Context, req taskservice.RecordSubagentContextRequest)
}
```

held as an optional `subagentContexts` field on `Service`, nil-checked at both
call sites exactly as `messageCreator` is.

Two call sites in `event_handlers_streaming.go`, each already holding every
input:

- `handleToolCallEvent` (~line 312) — after the `CreateToolCallMessage` block,
  using `s.getActiveTurnID(payload.SessionID)`.
- `persistToolUpdateMessage` (~line 565) — after `UpdateToolCallMessage`, using
  the `turnID` that function already resolved (`peekActiveTurnID` for terminal
  frames, `getActiveTurnID` otherwise).

Both guarded by `payload.Data.Normalized.Kind() == streams.ToolKindSubagentTask`
and a non-nil `Normalized.SubagentTask()`. A new
`recordSubagentContextFromFrame(ctx, payload, turnID)` helper holds the guard and
the request build so neither handler grows past the package's `funlen` limit of
80 lines / 50 statements.

`payload.SessionID` is the Kandev task session id: `messageCreatorAdapter`
passes the same value straight into `CreateMessageRequest.TaskSessionID`
(`internal/backendapp/adapters.go:879`), despite the `agentSessionID` parameter
name.

`apps/backend/internal/backendapp/adapters.go` — a `subagentContextAdapter` over
`*taskservice.Service`, wired where `messageCreatorAdapter` is wired.

---

## Frontend

None. The spec's *What this feature is, and is not* rules out any frontend
surface: the subagent card keeps rendering from message metadata, unchanged. No
DTO field, no WebSocket event, no store slice.

---

## Tests

### Migration and backfill — `subagent_context_migration_test.go` (task 01)

Patterned on `task_external_id_migration_test.go`.

- **Fresh DB creates the table, indexes, and UNIQUE constraint.** File:
  `apps/backend/internal/task/repository/sqlite/subagent_context_migration_test.go`.
  How: `NewWithDB` over a temp SQLite file, then assert
  `idx_subagents_session_id`, `idx_subagents_task_id`, `idx_subagents_turn_id`
  exist in `sqlite_master` and that a duplicate
  `(task_session_id, tool_call_id)` insert is rejected. (AC-17)
- **Replay is a no-op.** How: `repo.runMigrations()` twice; assert no error and
  an unchanged row count. (AC-18)
- **Backfill inserts one row per subagent message.** How: seed a session, a
  turn, and message rows whose `metadata.normalized.kind = 'subagent_task'`,
  then `runMigrations()`; assert one row each with `source = 'backfill'` and
  `observed_at = m.created_at`. (AC-21)
- **`async_launched` shape stores NULL, not `0`** — the spec's named
  must-not-get-wrong regression. How: seed a message whose
  `subagent_task` object has `status: "async_launched"` and no
  `total_tokens` / `tool_use_count` / `duration_ms`; assert all three columns
  `IS NULL` and that none is `0`. (AC-7)
- **A reported `tool_use_count` of `0` survives as `0`.** (AC-8)
- **A negative reported metric backfills as NULL.** (AC-9, AC-23)
- **Empty JSON strings become NULL.** How: a payload with `model: ""`; assert
  `model IS NULL`, not `''`.
- **Malformed metadata does not abort the statement.** How: seed one message row
  with `metadata = ''` and another with `metadata = 'null'` alongside a valid
  subagent row; assert the valid row still lands. (AC-23b)
- **A live row wins over the backfill.** How: pre-insert a `source = 'live'` row
  for a `(session, tool_call_id)` that also exists as a message, run
  `runMigrations()`, assert `source` is still `'live'` and there is exactly one
  row. (AC-22)
- **Rows with no `task_id` or no `tool_call_id` are not backfilled.** (AC-2)
- **Activation keys are written once.** How: assert both `kandev_meta` keys are
  present and RFC3339-parseable (`backfill_through` may be `''`), capture their
  values, `runMigrations()` again, assert both unchanged. (AC-24)
- **Fresh DB with no subagent messages sets `backfill_through` to `''`.** (AC-24)

### Repository upsert — `subagent_context_test.go` (task 02)

File: `apps/backend/internal/task/repository/sqlite/subagent_context_test.go`.

- **First upsert inserts.** (AC-1)
- **Second upsert for the same key updates in place, one row.** (AC-3)
- **Fill-forward: a frame with empty fields does not blank learned values.**
  Table-driven over every nullable column. (AC-4)
- **A non-empty value replaces the stored value.** (AC-5)
- **`settled_at` is write-once**: a second terminal frame with a later timestamp
  leaves the first value. (AC-11)
- **A non-terminal frame after settling leaves `settled_at` and `tool_status`
  alone but still fills NULL columns.** (AC-12)
- **Never-terminal row exists with `settled_at IS NULL`.** (AC-10)
- **`turn_id` from the first frame survives a later frame carrying a different
  turn.** (AC-2a)
- **`is_async` is sticky** across a later frame reporting false.
- **`source` and `observed_at` are not overwritten** by a later upsert.
- **Nested `parent_tool_call_id` is stored and the row is not suppressed.**
  (AC-6)
- **Concurrent upserts of the same key** — N goroutines, `errgroup`; assert
  exactly one row and that the invariants above hold regardless of commit order.
  (AC-14)
- **Concurrent upserts of different keys in one turn** — the observed 3-, 4-,
  5- and 8-way fan-outs; assert every row lands. (AC-15)
- **Deleting the parent session cascades the rows away.** (AC-16) Also add
  `task_session_subagents` to `assertNoWorkspaceCascadeDependents` in
  `workspace_test.go:317`, so the workspace cascade test covers the new table.
- **Read ordering is `observed_at ASC, tool_call_id ASC`** — seed rows sharing an
  `observed_at` and assert the `tool_call_id` tiebreak, per AC-13's explicit
  rejection of `id` as a tiebreak.

### Service writer — `subagent_context_test.go` (task 03)

File: `apps/backend/internal/task/service/subagent_context_test.go`.

- **Table-driven identity gate**: each of empty session id, empty task id, empty
  tool call id ⇒ no repository call, `skipped_no_identity` incremented by one.
  (AC-2)
- **Table-driven normalization**: `""` ⇒ NULL for every nullable TEXT column;
  absent metric ⇒ nil; reported `0` `ToolUseCount` ⇒ `*int64(0)`; negative
  ⇒ nil plus `anomalous_value`++ with the other fields of the same frame intact.
  (AC-7, AC-8, AC-9)
- **Terminal `tool_status` sets `settled_at`; `agent_status = "async_launched"`
  alone does not.** (AC-10a, AC-11)
- **An unrecognised `agent_status` is stored verbatim and does not settle.**
  (AC-10a)
- **A repository error logs `WARN`, increments `failed`, and the call still
  returns normally.** How: a stub repo returning an error plus
  `zaptest`/observed-logs. (AC-27)
- **`attempted` and `persisted` move as specified** on the happy path. (AC-26)
- **The service's terminal-status set equals the orchestrator's** — pin both
  lists so a future edit to one breaks the test. (AC-11)

### Orchestrator integration — `event_handlers_streaming_subagent_test.go` (task 04)

File:
`apps/backend/internal/orchestrator/event_handlers_streaming_subagent_test.go`.

- **`handleToolCallEvent` with a `subagent_task` payload records a context** with
  the active turn id. (AC-1)
- **`handleToolUpdateEvent` for the same tool call records again** so the merge
  path runs; assert one recorded request per frame with the right turn id.
  (AC-3)
- **A terminal update carries the terminal `tool_status` through.** (AC-11)
- **A non-`subagent_task` normalized kind records nothing.**
- **A nil `subagentContexts` field is safe** — no panic, message write still
  happens. (AC-27)
- **A nested frame (`ParentToolCallID` set) is recorded, not dropped.** (AC-6)
- **Integration through the real service and repository**, using
  `newServiceBackedMessageCreator(repo)` as `event_handlers_streaming_test.go:906`
  does, so one test exercises handler → service → repo end to end.

### Postgres parity and the health query (task 05)

- **Fresh Postgres schema plus migration replay**, including the backfill and
  both activation keys. File:
  `apps/backend/internal/task/repository/sqlite/subagent_context_postgres_test.go`,
  gated on `KANDEV_TEST_POSTGRES_DSN` via `testutil.OpenIsolatedPostgres`.
  (AC-19)
- **The upsert's conflict clause behaves identically on Postgres** — fill-forward,
  `settled_at` write-once, `is_async` sticky, `turn_id` preserved. This is the
  ADR-0027 requirement that schema replay alone is insufficient for a
  dialect-sensitive method. (AC-19)
- **The backfill's JSON helpers work on `jsonb`**, including the `''`/`'null'`
  metadata rows and the boolean spelling difference. (AC-23b)
- **AC-28 health query** on a seeded store: assert the
  `task_session_messages` `subagent_task` count equals
  `COUNT(*) FROM task_session_subagents` after live writes, and that seeding a
  message the writer never saw makes the two diverge. File:
  `apps/backend/internal/task/repository/sqlite/subagent_context_health_test.go`.
  (AC-28, AC-29)

---

## E2E Tests

None. The spec's *Out of scope* states no UI surface changes, therefore "no E2E
coverage is implied". Adding one would test nothing this feature changes.

---

## Verification Results

Pending. On completion, synchronize this section with each task's `## Results`:
record exact commands and outcomes/counts, generated artifact paths, and
cleanup/teardown evidence.

---

## Implementation Waves And Parallel Candidates

Every task shares the `internal/task` subtree and each depends on the previous
one's schema or seam, so the chain is sequential. Nothing here is
parallel-safe: tasks 01 and 02 both edit the same package and task 01 owns
shared schema DDL.

```
Wave 1:
- [ ] [task-01-schema-and-backfill](task-01-schema-and-backfill.md)

Wave 2:
- [ ] [task-02-repository-upsert](task-02-repository-upsert.md)

Wave 3:
- [ ] [task-03-service-writer](task-03-service-writer.md)

Wave 4:
- [ ] [task-04-orchestrator-wiring](task-04-orchestrator-wiring.md)

Wave 5:
- [ ] [task-05-postgres-and-health](task-05-postgres-and-health.md)
```

## Open Questions

None. Two spec ambiguities were resolved against repository convention rather
than left open, and both are recorded above so a builder does not re-litigate
them:

1. **"Schema-init DDL *and* an idempotent migration"** for a new table would be
   a duplicated `CREATE TABLE`. `apps/backend/AGENTS.md` settles it: schema init
   runs before `runMigrations()` on every boot, so the DDL alone satisfies
   AC-17. See *Schema (task 01)*.
2. **AC-13 mandates a read ordering** while *Out of scope* forbids a read API.
   Resolved as two repository-level list methods and no service, DTO, or route.
   See *Model and repository upsert (task 02)*.
