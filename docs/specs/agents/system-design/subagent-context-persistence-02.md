---
status: draft
system: agents
requirements:
  - REQ-AGENTS-SUBAGENT-CONTEXT-PERSISTENCE-001
created: 2026-08-12
updated: 2026-08-14
owners:
  - nova28
---
# Subagent context persistence System Design Part 2

## Purpose and boundaries

This design preserves the technical source detail for `REQ-AGENTS-SUBAGENT-CONTEXT-PERSISTENCE-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-SUBAGENT-CONTEXT-PERSISTENCE-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

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
| `agent_execution_id` | TEXT | no | *(added by Amendment 1)* The lifecycle execution that emitted the frame (`AgentStreamEventPayload.ExecutionID`). Part of the upsert key. Never NULL: the reserved sentinel `'unknown'` is stored when no execution identity is available (AC-31). |
| `tool_call_id` | TEXT | no | Agent-supplied invocation identity. Unique only **within one execution** — see *Constraints and indexes*. |
| `parent_tool_call_id` | TEXT | yes | Set when a subagent spawned this subagent. NULL for top-level. |
| `subagent_type` | TEXT | yes | e.g. `security-reviewer`. NULL when not reported. |
| `description` | TEXT | yes | Short label from `rawInput.description`. |
| `agent_id` | TEXT | yes | Provider-supplied child agent id. NULL for providers that omit it. |
| `child_session_id` | TEXT | yes | OpenCode child ACP session. NULL elsewhere. |
| `model` | TEXT | yes | Resolved child model, verbatim (e.g. `claude-opus-5[1m]`). |
| `agent_status` | TEXT | yes | The **agent-reported subagent** status, verbatim (`completed`, `async_launched`, `started`, …). NULL when never reported. |
| `tool_status` | TEXT | yes | The **ACP tool-call** status of the launching `Task` call, stored verbatim. Non-terminal values include `pending` and `in_progress`; the six terminal values are enumerated in AC-11. NULL when never reported. |
| `is_async` | INTEGER | no | 1 for a detached dispatch, else 0. Default 0. |
| `total_tokens` | INTEGER | **yes** | As reported. **NULL when not reported.** |
| `tool_use_count` | INTEGER | **yes** | As reported. **NULL when not reported; 0 only when the agent reported 0.** |
| `duration_ms` | INTEGER | **yes** | As reported. **NULL when not reported.** |
| `source` | TEXT | no | `live` or `backfill`. Provenance. Default `live`. |
| `observed_at` | TIMESTAMP | no | When Kandev first observed the launch frame. |
| `settled_at` | TIMESTAMP | yes | When a terminal status was first observed. NULL while outstanding. |
| `updated_at` | TIMESTAMP | no | Last write to this row. |

Constraints and indexes *(amended by Amendment 1)*:

- `UNIQUE(task_session_id, agent_execution_id, tool_call_id)` — the upsert key.
  Uniqueness is **execution-scoped**, because that is the widest scope in which
  a `tool_call_id` is actually unique. The original two-column key assumed
  session scope; *Execution identity, as sampled* shows why that assumption is
  false, and the *Amendment log* records the review that caught it.

  The column order is the key's own contract, not incidental: `task_session_id`
  leads so the constraint's backing index is usable as a leftmost-prefix scan for
  session-scoped reads (AC-13), rather than being usable only for full-key
  lookups. *(revision 1)* An earlier draft went further and said this meant the
  key needed no second index — which contradicted the index list two bullets
  below, where `INDEX(task_session_id)` is still declared. The claim is narrowed
  to what is true: **the key order means no NEW index is required for AC-13's
  reads.** The pre-existing single-column index is retained, in both the
  fresh-database DDL and the AC-33 rebuild, so that a migrated database and a
  fresh one have byte-identical index sets (AC-33 clause 4) — dropping a
  now-redundant index is a separate, behaviour-neutral change with its own
  measurement, and silently doing it inside a correctness migration would make
  the AC-33 replay test encode an arbitrary choice.

  Two rows that differ **only** in `agent_execution_id` are the intended
  outcome, not a duplicate. They record two genuinely different invocations
  that happened to reuse an ID, and collapsing them is precisely the data loss
  this key prevents.
- FK `task_session_id` → `task_sessions(id)` ON DELETE CASCADE.
- **`agent_execution_id` carries no foreign key**, for the same reason
  `turn_id` does not. Execution rows live in `executors_running` and are
  removed when the executor stops (`agent_execution_id` was itself dropped from
  `task_sessions` by an earlier migration, `base_migrations.go:857`), so an FK
  would delete the measurement when the thing it measured finished — the exact
  failure the `turn_id` decision already rejected. It is a plain value column
  that happens to be part of the key.
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
- *(added by Amendment 1)* `agent_execution_id` is **NOT NULL and never
  empty**. It is the reserved literal `'unknown'` whenever no execution
  identity is available, and the frame's `ExecutionID` otherwise. It is
  write-once: once a row exists, no later frame changes its
  `agent_execution_id`, because doing so would move the row to a different
  identity. `'unknown'` cannot collide with a real value, which is always a
  UUID.

  **There is exactly one sentinel, deliberately shared** by all three sources
  of "no execution identity" — the backfill, rows written before this amendment,
  and a live frame whose `ExecutionID` was empty. A second sentinel would look
  tidier and would be a bug: the backfill's `ON CONFLICT DO NOTHING` (AC-22)
  only suppresses a duplicate when the backfilled row lands on the *same* key
  as the row already there, so a backfill sentinel distinct from the
  pre-amendment live sentinel would insert a second row for a subagent already
  recorded.

  *(revision 1)* **What sharing the sentinel does and does not preserve.** An
  earlier draft claimed `observed_at` against `subagent_context_capture_since`
  separates pre-amendment rows from a live frame that genuinely arrived without
  an execution id. **That claim was false and is withdrawn.**
  `subagent_context_capture_since` is write-once (AC-24) and was written by the
  build that shipped *before* this amendment, so it marks the start of capture,
  not the amendment boundary; after AC-33 re-sentinels every legacy row, a
  pre-amendment row and a post-amendment empty-`ExecutionID` row are
  indistinguishable — both `source = 'live'`, both `observed_at` after
  `capture_since`, both `'unknown'`.

  What actually separates them is the third activation key
  `subagent_context_execution_since` (AC-24), written by the AC-33 migration. A
  `'unknown'` row whose `observed_at` precedes it is a legacy row that never had
  an execution dimension; a `'unknown'` row whose `observed_at` follows it is a
  live frame that genuinely arrived without an execution id, and that case also
  increments the `unknown_execution` counter (AC-31). `source` continues to
  separate backfilled from live. Without that third key the distinction is not
  recoverable from the database at all, and AC-31's whole purpose — making a
  regression that silently stops propagating execution identity *observable* —
  would hold only for the lifetime of one process's expvar counters.
- `observed_at`, `settled_at`, `updated_at` are UTC wall-clock instants from the
  backend process (`time.Now().UTC()`), at the same precision as the sibling
  session tables.
- `task_id` is required (NOT NULL). WHEN a frame carries no task id, the row is
  not written and AC-2's `skipped_no_identity` path applies — a session-scoped
  row with no card cannot be joined to anything the feature exists to answer.
- Reported metrics are stored verbatim within `int64`; there is no upper clamp.
  The largest observed `total_tokens` is 275,717.

---
