---
status: draft
created: 2026-08-12
updated: 2026-08-12
owner: nova28
---

# Subagent context persistence

## Why

When an agent fans out to subagents (Claude's `Task` tool, OpenCode's `task`,
Cursor's `Task`, Auggie's `sub-agent-<type>:`), Kandev already recognises the
fan-out and already receives the child's identity and usage on the completion
frame. It renders them, counts them in memory, and then keeps no queryable
record of them.

The consequence is a specific, measured distortion. In the reference store
(`~/.kandev/data/kandev.db`, snapshotted 2026-08-12), **sessions per card is
exactly 1.0 for every step**, while 253 subagent invocations exist across 38
sessions and 127 turns — including **42 turns that fanned out to exactly three
subagents**. A step that spawned three parallel reviewers is, in the store,
indistinguishable from one long conversation. Every per-context cost figure is
therefore wrong in the same direction, and the Ops Cost analysis of the Review
step's cache cost had to *infer* a three-way fan-out by reading the step's
prompt, because nothing in the store recorded it.

## What this feature is, and is not

**It is persistence and a parent link.** The data already arrives on a frame
Kandev parses (`internal/agentctl/server/adapter/transport/acp/subagent.go`) and
is already carried in a typed payload
(`streams.SubagentTaskPayload`). This spec adds a durable, queryable row per
subagent invocation, linked to the session and turn that spawned it. It adds no
new instrumentation, no new agent-side capability, and no new wire fields.

**It is not a session.** A subagent context is deliberately *not* a
`task_sessions` row. See *The design question*, below — that decision is part of
the contract, not an implementation detail.

**It is not a UI feature.** No frontend surface changes. The existing subagent
card already renders from message metadata and continues to do so, unchanged.

**It is not cost attribution.** Token counts are stored exactly as the agent
reported them, with provenance. No pricing, no dollars, no rollup into
`task_sessions.cost_subcents`.

### One correction to the framing

The originating analysis states the data is "never persisted". That is
imprecise, and the imprecision matters to the design. The normalized payload
*is* written today — as opaque JSON at
`task_session_messages.metadata.normalized.subagent_task` (253 such rows exist
in the reference store). What does not exist is a **relational fact**: no
stable row identity, no first-class parent link, no column a query can group by,
and no independent record — the JSON lives inside a row that is cascade-deleted
with its session.

This matters twice over:

1. It makes a **backfill possible**, which this spec takes (see *Backfill*).
2. It makes the message table an **independent expected-count source** for
   writer health (see *Writer health*).

---

## The design question: a row in `task_sessions`, or its own table?

**Answer: its own table, `task_session_subagents`.** The parent link is a
foreign key from that table to `task_sessions`, not a `parent_session_id` column
added to `task_sessions`.

The reasoning, from the code as it stands:

**A subagent has none of the things a `task_sessions` row asserts.** It has no
workspace and no `workspace_path`; no executor, `executor_profile_id`, or
`task_environment_id`; no worktree or `base_branch`; no `agent_profile_id`; no
`executors_running` row; no ACP session Kandev can `session/load` or resume; no
`state` machine (`CREATED` → `RUNNING` → …); no launch, stop, or recover path.
A row in `task_sessions` implies all of those, and every one would be a lie.

**`task_sessions` is read by paths that would be actively harmed.** Session
rows feed lifecycle recovery (`RecoverInstances`), the orchestrator, session-tab
listings, WebSocket subscription scoping, the auth scoper
(`AuthorizeSessionAccess` resolves session → task → workspace → owner), the boot
payload, and cost rollups. Inserting non-launchable rows there forces a new
"is this real" predicate into every one of them, and any path that forgets it
becomes a defect.

**It would silently break the very series this feature exists to fix.** The
downstream extract's `dim_session` is `SELECT … FROM task_sessions`. Adding
subagent rows to that table would change what "sessions per card" *means*
mid-series, with no schema versioning to signal it — which is precisely the
discontinuity the cross-cutting rule for this work forbids. A separate table is
purely additive: the existing series stays comparable, and the new fact is
opt-in for any consumer that wants it.

The name follows the existing sibling convention: `task_session_messages`,
`task_session_turns`, `task_session_commits`, `task_session_subagents`.

---

## Sampled input shapes

Every shape below was read from the live store on 2026-08-12, not assumed.

### Population by terminal status (n = 253)

| status | n | agent_id | child_session_id | model | total_tokens | tool_use_count | duration_ms |
|---|---|---|---|---|---|---|---|
| `async_launched` | 190 | 190 | 0 | 190 | **0** | **0** | **0** |
| `completed` | 57 | 57 | 0 | 49 | 57 | 57 | 57 |
| `started` | 5 | 0 | 5 | 0 | 0 | 0 | 0 |
| *(absent)* | 1 | 0 | 0 | 0 | 0 | 0 | 0 |

Three facts drive the whole contract:

1. **75% of subagent invocations report no usage at all.** `async_launched` is
   Claude's `run_in_background` dispatch; the dispatch is terminal for the
   `Task` tool, the subagent runs out-of-band and writes to an `outputFile`
   Kandev never reads. These rows will never gain tokens, duration, or a tool
   count. Storing `0` for them would fabricate 190 measurements.
2. **`child_session_id` is empty for every Claude row** and is populated only by
   OpenCode (5 rows, all non-terminal). It cannot be the identity.
3. **`tool_call_id` is present on all 253 rows and unique across all 253.**
   `agent_id` is unique among the 247 rows that carry one, but is absent for
   three of the four supported agents. `tool_call_id` is the identity.

### Other measured facts

- **Fan-out distribution per turn:** 1→65 turns, 2→12, **3→42**, 4→5, 5→2, 8→1.
- **Nesting:** exactly 1 of 253 rows carries a `parent_tool_call_id` — a
  subagent that spawned a subagent. Rare, real, and must be specified.
- `turn_id` and `task_session_id` are non-empty on all 253 rows.
- `subagent_type` is missing on 3 rows.
- No two subagent rows within a turn share a `created_at` (microsecond
  precision), so ties are not observed — but they are not prevented either.
- Observed window: 2026-08-03 15:32:20Z to 2026-08-12 13:24:58Z.

### The payload as parsed

`streams.SubagentTaskPayload` carries `Description`, `Prompt`, `SubagentType`,
`Status`, `AgentID`, `Model`, `ChildSessionID`, `DurationMs`, `TotalTokens`,
`ToolUseCount *int`, `ResultText`, `IsAsync`, `OutputFile`,
`CanReadOutputFile`. `ToolUseCount` is already a pointer specifically so a
reported `0` is distinguishable from "not reported" — that distinction is
load-bearing here and must survive into the column.

---

## Data model

A new table, created on fresh databases in the schema-init DDL **and**
introduced on existing databases by an idempotent migration in
`runMigrations()`, per the repository's schema rules.

| Column | Type | Null? | Meaning |
|---|---|---|---|
| `id` | TEXT PK | no | Kandev-generated UUID. Not an ordering key. |
| `task_session_id` | TEXT | no | Parent session. FK → `task_sessions(id)` ON DELETE CASCADE. |
| `task_id` | TEXT | no | Denormalised parent task, mirroring `task_session_turns`. |
| `turn_id` | TEXT | yes | Turn that spawned it. NULL when no turn was active. |
| `tool_call_id` | TEXT | no | Agent-supplied invocation identity. |
| `parent_tool_call_id` | TEXT | yes | Set when a subagent spawned this subagent. NULL for top-level. |
| `subagent_type` | TEXT | yes | e.g. `security-reviewer`. NULL when not reported. |
| `description` | TEXT | yes | Short label from `rawInput.description`. |
| `agent_id` | TEXT | yes | Provider-supplied child agent id. NULL for providers that omit it. |
| `child_session_id` | TEXT | yes | OpenCode child ACP session. NULL elsewhere. |
| `model` | TEXT | yes | Resolved child model, verbatim (e.g. `claude-opus-5[1m]`). |
| `agent_status` | TEXT | yes | The **agent-reported subagent** status, verbatim (`completed`, `async_launched`, `started`, …). NULL when never reported. |
| `tool_status` | TEXT | yes | The **ACP tool-call** status of the launching `Task` call (`pending`, `in_progress`, `completed`, `failed`, `cancelled`). NULL when never reported. |
| `is_async` | INTEGER | no | 1 for a detached dispatch, else 0. Default 0. |
| `total_tokens` | INTEGER | **yes** | As reported. **NULL when not reported.** |
| `tool_use_count` | INTEGER | **yes** | As reported. **NULL when not reported; 0 only when the agent reported 0.** |
| `duration_ms` | INTEGER | **yes** | As reported. **NULL when not reported.** |
| `source` | TEXT | no | `live` or `backfill`. Provenance. Default `live`. |
| `observed_at` | TIMESTAMP | no | When Kandev first observed the launch frame. |
| `settled_at` | TIMESTAMP | yes | When a terminal status was first observed. NULL while outstanding. |
| `updated_at` | TIMESTAMP | no | Last write to this row. |

Constraints and indexes:

- `UNIQUE(task_session_id, tool_call_id)` — the upsert key. Uniqueness is
  **session-scoped by design**; global uniqueness of an agent-supplied
  `tool_call_id` is not assumed, even though it holds for all 253 observed rows.
- FK `task_session_id` → `task_sessions(id)` ON DELETE CASCADE.
- **`turn_id` carries no foreign key.** `task_session_messages.turn_id`
  cascades from `task_session_turns`, which would mean deleting a turn silently
  deletes the fan-out record for that turn. The measurement must outlive the
  turn row, so `turn_id` is a plain nullable column.
- `INDEX(task_session_id)`, `INDEX(task_id)`, `INDEX(turn_id)`.

Deliberately **not** stored: `prompt`, `result_text`, `output_file`. The prompt
and result are free text already retained on the message row; duplicating them
here doubles the store's largest text payload and adds a second copy of
prompt content to any extract. This table is structure, not content.

### Two statuses, deliberately separate

`agent_status` and `tool_status` are different measurements from different
layers and are **not** merged into one column:

- `agent_status` is the child's own report, read from
  `_meta.claudeCode.toolResponse.status` (or the provider's equivalent). Its
  vocabulary is provider-defined and open — `async_launched` is a value only
  Claude produces, and it means *dispatched, will never report usage*.
- `tool_status` is the ACP status of the launching `Task` tool call, which
  Kandev already classifies via `isTerminalToolStatus`. Its vocabulary is
  closed and provider-neutral.

Terminality (AC-11) keys off `tool_status`, because that is the signal the
orchestrator already treats as authoritative everywhere else. `agent_status` is
stored verbatim and is never used to decide whether the row has settled.

### Column-level normalization rules

These apply to every write path, live and backfill:

- **An empty string is stored as NULL**, never as `''`, for every nullable TEXT
  column. `SubagentTaskPayload` uses `""` for "absent"; the column uses NULL, so
  that `COUNT(model)` counts models rather than blanks.
- `is_async` defaults to `0` and is set to `1` only on a positive report; it is
  never set back to `0` by a later frame.
- `source` defaults to `'live'`.
- `observed_at`, `settled_at`, `updated_at` are UTC wall-clock instants from the
  backend process (`time.Now().UTC()`), at the same precision as the sibling
  session tables.
- `task_id` is required (NOT NULL). WHEN a frame carries no task id, the row is
  not written and AC-2's `skipped_no_identity` path applies — a session-scoped
  row with no card cannot be joined to anything the feature exists to answer.
- Reported metrics are stored verbatim within `int64`; there is no upper clamp.
  The largest observed `total_tokens` is 275,717.

---

## Acceptance criteria

EARS-style. Each is observable from the database or from a logged/exported
counter without reading source.

### Capture

**AC-1** — WHEN the orchestrator observes a tool-call frame whose normalized
kind is `subagent_task`, and a non-empty session id, task id and `tool_call_id`
are all present, THEN a `task_session_subagents` row SHALL exist for
`(task_session_id, tool_call_id)` with `source = 'live'` and `observed_at` set
to Kandev's observation time.

**AC-1a** — `observed_at` SHALL be the instant Kandev first observed a frame it
could *recognise* as a subagent, not the instant the subagent began work. For
Claude and OpenCode the initial `tool_call` carries no `rawInput`, so
recognition occurs on a later `tool_call_update`; the column is therefore a
Kandev observation time and SHALL NOT be published as a subagent start time.

**AC-2** — WHEN a subagent frame carries an empty `tool_call_id`, an empty
session id, or an empty task id, THEN the system SHALL NOT write a row, SHALL
increment the `skipped_no_identity` writer counter, and SHALL NOT fail the
enclosing message write or turn.

**AC-2a** — WHEN a `tool_call_id` that already has a row in a session is
observed again under a *different* `turn_id`, THEN the existing row SHALL be
updated per AC-3 and its original `turn_id` SHALL be preserved. A second row
SHALL NOT be created, and the fan-out SHALL NOT be re-attributed to the later
turn.

**AC-3** — WHEN a later frame for an already-recorded `(task_session_id,
tool_call_id)` arrives, THEN the system SHALL update that same row and SHALL NOT
create a second row.

**AC-4** — WHEN a frame reports a field that the stored row already holds
non-NULL, and the frame's value for that field is absent or empty, THEN the
stored value SHALL be preserved unchanged. (This mirrors `applySubagentResult`,
which never blanks a value already learned from an earlier frame.)

**AC-5** — WHEN a frame reports a non-empty value for a field, THEN that value
SHALL replace the stored value, EXCEPT as constrained by AC-9.

**AC-6** — WHEN a frame reports a subagent whose `parent_tool_call_id` is
non-empty, THEN the row SHALL be written with that `parent_tool_call_id`, and
SHALL NOT be suppressed for being nested.

### Nulls, zeroes, and absence

**AC-7** — WHEN an agent does not report `total_tokens`, `tool_use_count`, or
`duration_ms`, THEN the corresponding column SHALL be NULL. The system SHALL NOT
write `0` for an unreported value. (This is not hypothetical: 190 of 253
observed rows report none of the three.)

**AC-8** — WHEN an agent reports `tool_use_count` as exactly `0` (the payload's
`ToolUseCountKnown` is true and the count is 0), THEN the column SHALL be `0`,
distinguishable by query from an unreported count.

**AC-9** — WHEN a reported numeric value for `total_tokens`, `tool_use_count`,
or `duration_ms` is negative, THEN the system SHALL store NULL for that field,
SHALL increment the `anomalous_value` writer counter, and SHALL leave every
other field of the frame unaffected. A negative count is not a measurement.

**AC-10** — WHEN a subagent is never observed reaching a terminal `tool_status`,
THEN `agent_status` and `tool_status` SHALL hold their last observed values (or
NULL if none was ever reported) and `settled_at` SHALL be NULL. The row SHALL
still exist. (5 observed rows are in exactly this state.)

**AC-10a** — WHEN a frame reports an `agent_status` value Kandev does not
recognise, THEN it SHALL be stored verbatim and SHALL NOT affect `settled_at`.
`agent_status` has an open, provider-defined vocabulary.

### Terminality and ordering

**AC-11** — WHEN a frame's **ACP `tool_status`** is first classified terminal by
`isTerminalToolStatus` (`completed`, `failed`, `cancelled`), THEN `settled_at`
SHALL be set to that observation time and SHALL NOT be modified by any later
frame. `agent_status` SHALL NOT be used to decide terminality — in particular,
`async_launched` is a Claude *agent* status meaning "dispatched, no usage will
follow", and its own tool call is terminal in the ACP sense, so those rows
settle via `tool_status` like any other.

**AC-12** — WHEN a non-terminal frame arrives for a row whose `settled_at` is
already set, THEN `settled_at` SHALL remain set, `tool_status` SHALL remain the
terminal value, and the frame MAY still fill columns that are currently NULL.
(Out-of-order and duplicate frame delivery is an observed reality on this
stream.)

**AC-13** — WHEN subagent contexts are read for a session or a turn, THEN the
order SHALL be `observed_at ASC, tool_call_id ASC`. `tool_call_id` is the named
tiebreak: it is unique within the session by construction (the UNIQUE
constraint), stable across replays, and total. `id` is a generated UUID and
SHALL NOT be used as a tiebreak.

### Concurrency

**AC-14** — WHEN two frames for the same `(task_session_id, tool_call_id)` are
processed concurrently, THEN the write SHALL be a single atomic upsert
statement, and the resulting row SHALL satisfy AC-4, AC-5, AC-11 and AC-12
regardless of which frame commits first. The implementation SHALL NOT use a
read-then-write sequence for field merging; the fill-forward and
terminality rules are expressed inside the upsert's conflict clause.

**AC-15** — WHEN two frames for *different* `tool_call_id`s in the same turn are
processed concurrently (the observed 3-, 4-, 5- and 8-way fan-outs), THEN both
rows SHALL be written, and neither SHALL block the other.

**AC-16** — WHEN the parent session row is deleted, THEN its
`task_session_subagents` rows SHALL be removed by the foreign key cascade,
consistent with `task_session_messages` and `task_session_turns`.

### Migration

**AC-17** — WHEN `runMigrations()` runs against a database that predates this
feature, THEN the table, its indexes and its constraints SHALL be created, and
existing data SHALL be unaffected.

**AC-18** — WHEN `runMigrations()` is invoked twice in succession against the
same database, THEN the second invocation SHALL succeed and SHALL leave the
schema and the row set unchanged. (Mirrors
`task_external_id_migration_test.go`: seed a pre-migration row, call
`runMigrations` twice, assert idempotence.)

**AC-19** — WHEN the migration runs against PostgreSQL, THEN it SHALL apply and
replay with the same result as SQLite, using `internal/db` error classification
rather than local error-string matching, per ADR 0027.

**AC-20** — WHEN a statement in this migration fails, THEN — because the
repository's migration runner swallows errors
(`internal/db/migratelog.go:33`) — the failure SHALL be observable as a `WARN`
log carrying the migration's name, and the writer's health signals (AC-24,
AC-25) SHALL make the resulting absence of rows detectable rather than silent.

### Backfill

**AC-21** — WHEN the migration runs, THEN it SHALL insert one row with
`source = 'backfill'` for every existing `task_session_messages` row whose
`metadata.normalized.kind` is `subagent_task`, deriving each column from
`metadata.normalized.subagent_task` and `metadata.tool_call_id`, and setting
`observed_at` from the message's `created_at`.

**AC-22** — WHEN the backfill encounters a `(task_session_id, tool_call_id)`
that already has a row, THEN it SHALL leave the existing row untouched
(`ON CONFLICT DO NOTHING`), so a live write always wins over a backfilled one
and a replay is a no-op.

**AC-23** — WHEN the backfill derives a value, THEN AC-7 through AC-9 and the
empty-string-to-NULL rule SHALL apply identically: an absent JSON field becomes
NULL, never `0` and never `''`.

**AC-23a** — The backfill SHALL be a single unbounded `INSERT … SELECT` with no
`LIMIT` and no batching, and SHALL set `settled_at` from the message's
`updated_at` when the derived `tool_status` is terminal, else NULL. It performs
one full scan of `task_session_messages` at first boot after upgrade — a real,
accepted one-time cost on a large store (the reference store is 328 MB), taken
once because a partial or resumable backfill would produce exactly the
mid-series discontinuity AC-25 exists to prevent.

**AC-23b** — WHEN the backfill runs on PostgreSQL, THEN it SHALL use the
dialect-aware JSON helpers already present in `base_migrations.go`
(`jsonColumn`, `jsonText`), which normalise empty and `'null'` metadata
documents so a single malformed row cannot abort the statement.

### Activation point and provenance

**AC-24** — WHEN the migration completes successfully for the first time on an
installation, THEN `kandev_meta` SHALL hold two keys, written once and never
overwritten:

- `subagent_context_capture_since` — RFC3339 UTC instant from which `live`
  capture is authoritative.
- `subagent_context_backfill_through` — the RFC3339 UTC `created_at` of the
  newest message the backfill read, or the empty string when the backfill
  inserted no rows.

**AC-25** — The published contract for consumers SHALL be: a session whose
activity predates `subagent_context_backfill_through` has **unknown** fan-out,
not zero fan-out; rows with `source = 'backfill'` are limited to what survived
in message metadata and SHALL NOT be compared like-for-like against `live`
rows without disclosing the provenance split. Absence of a row is never
evidence of absence of a subagent before `subagent_context_capture_since`.

### Writer health

**AC-26** — The system SHALL expose counters, under the existing `expvar`
convention used by `routing_*` and `subproc_*`, for: `attempted`, `persisted`,
`skipped_no_identity`, `anomalous_value`, and `failed`.

**AC-27** — WHEN a persist attempt fails, THEN the system SHALL log at `WARN`
with the session id and tool-call id, SHALL increment the `failed` counter, and
SHALL NOT fail the enclosing message write, the turn, or the agent stream.
Telemetry never breaks the product path.

**AC-28** — The expected-versus-observed health check SHALL be answerable from
the database alone, with no counter access, as:

```sql
SELECT COUNT(*) FROM task_session_messages
 WHERE json_extract(metadata,'$.normalized.kind') = 'subagent_task';
-- versus
SELECT COUNT(*) FROM task_session_subagents;
```

These two counts SHALL agree for all activity after
`subagent_context_capture_since`, up to rows skipped under AC-2. The message
count is a valid independent expectation **because the message write is
load-bearing for the UI**: a broken message write is noticed by a human within
one turn, whereas a broken context write is noticed by nobody. That asymmetry
is what makes the comparison a real check rather than two readings of the same
failure.

**AC-29** — WHEN the writer has stopped while message writes continue, THEN the
AC-28 comparison SHALL diverge, and the divergence SHALL be attributable in time
via `MAX(observed_at)` on `task_session_subagents`. No separate heartbeat row is
written, precisely so that the health signal cannot itself be the thing that
silently stops.

---

## Failure modes

| Condition | Behaviour |
|---|---|
| Frame with no `tool_call_id` or no session id | No row; `skipped_no_identity`++; product path unaffected (AC-2). |
| Upsert returns an error | `WARN` + `failed`++; product path unaffected (AC-27). |
| Negative reported metric | That field NULL; `anomalous_value`++; other fields written (AC-9). |
| Frames arrive out of order | Terminality is monotonic; NULLs may still fill (AC-11, AC-12). |
| Duplicate frames | Upsert; exactly one row (AC-3). |
| Parent session deleted mid-turn | FK cascade removes rows; no orphan (AC-16). |
| Migration statement fails | Swallowed by the runner; surfaced as `WARN` + detectable via AC-28 (AC-20). |
| Backfill re-runs | `ON CONFLICT DO NOTHING`; no duplicates, no clobber (AC-22). |
| Agent reports an `agent_status` Kandev does not recognise | Stored verbatim; never affects `settled_at` (AC-10a). |
| Frame carries no task id | No row; `skipped_no_identity`++ (AC-2). |
| Same `tool_call_id` observed under a later turn | Existing row updated; original `turn_id` preserved (AC-2a). |
| A turn row is deleted | Subagent rows survive; `turn_id` has no FK, by design. |

## Persistence guarantees

- A row, once written, is durable until its parent session is deleted.
- `settled_at` is write-once (AC-11).
- `source` is write-once: a backfilled row is never relabelled `live`, and a
  live row is never overwritten by the backfill (AC-22).
- The two `kandev_meta` activation keys are write-once (AC-24).
- No column in this table participates in any existing rollup
  (`task_sessions.cost_subcents`, `tokens_in`, `tokens_out` are untouched).

## Permissions

The table is session-scoped and inherits the session's scope. Any read surface
added later MUST authorize via the parent session's task
(`authorizeTaskID(session.TaskID)`), per the backend's session-keyed entry-point
rule. This spec adds no read surface, so it adds no new authorization path.

---

## Out of scope

Named exclusions. Each is a contract, not an omission.

- **No `parent_session_id` column on `task_sessions`.** Rejected above.
- **No change to `dim_session` or to "sessions per card".** That series keeps its
  current meaning; the new table is a separate, additive fact.
- **No UI surface, no DTO field, no WebSocket event.** The subagent card
  continues to render from message metadata. No E2E coverage is implied,
  because no user-visible surface changes.
- **No REST/MCP read API.** Consumers read the table directly via the extract.
  If a product surface is wanted later, it is a separate spec with its own
  authorization design.
- **No merging of OpenCode's child ACP session.** `child_session_id` is stored;
  resolving it into the parent transcript is a separate feature.
- **No reading of Claude's async `outputFile`.** The 190 `async_launched` rows
  stay usage-less by design; inventing their usage is out of scope, and
  approximating it is forbidden by AC-7.
- **No cost or dollar attribution.** Tokens are as-reported, unpriced,
  unaggregated into any existing cost column.
- **No `prompt` or `result_text` column.** Structure, not content.
- **No recursive subagent-tree API.** `parent_tool_call_id` is recorded (AC-6);
  walking it is a consumer concern.
- **No retention or pruning policy beyond the FK cascade.** The table grows with
  message history and is bounded by the same session lifetime.
- **No retroactive repair of rows the backfill could not see.** Sessions already
  deleted are gone; AC-25 requires disclosing that, not fixing it.

## Verification

- Repository tests alongside `task_external_id_migration_test.go`: fresh-DB
  create, pre-migration seeded row, `runMigrations` twice, backfill idempotence,
  and the NULL-not-zero assertions for an `async_launched`-shaped payload.
- The same fresh-plus-replay matrix under `KANDEV_TEST_POSTGRES_DSN` (AC-19).
- Orchestrator tests for the frame paths: first-frame insert, later-frame merge,
  out-of-order terminal, nested `parent_tool_call_id`, empty-identity skip
  (each of the three missing ids), same-`tool_call_id`-later-turn, negative
  metric, reported-zero tool count, empty-string-to-NULL, and concurrent
  same-key upsert.
- A query-level check of AC-28 against a seeded store.

### The one shape a builder must not get wrong

A regression test SHALL assert, on an `async_launched`-shaped payload, that
`total_tokens`, `tool_use_count` and `duration_ms` are all `IS NULL` and none is
`0`. This is 75% of real traffic; a `DEFAULT 0` on any of those three columns
would make every future token-per-subagent average wrong by roughly a factor of
four, silently and permanently, and would do so *after* the activation point,
where AC-25 offers consumers no protection.

## Related

- ADR [0027 — replayable schema migrations](../../decisions/0027-replayable-schema-migrations.md)
- `apps/backend/AGENTS.md` § *Schema & migrations (SQLite repository)*
- `apps/backend/internal/agentctl/AGENTS.md` § *Subagent tool-call nesting: what each agent emits*
