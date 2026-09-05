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
# Subagent context persistence System Design Part 4

## Purpose and boundaries

This design preserves the technical source detail for `REQ-AGENTS-SUBAGENT-CONTEXT-PERSISTENCE-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-SUBAGENT-CONTEXT-PERSISTENCE-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

### Backfill

**AC-21** *(amended by revision 1; sources named in revision 2)* — WHEN the
migration runs, THEN it SHALL insert one row with `source = 'backfill'` for every
existing `task_session_messages` row whose `metadata.normalized.kind` is
`subagent_task` **and whose `task_session_id`, `task_id` and
`metadata.tool_call_id` are all non-empty**.

*(revision 2)* **Every column's source is named below.** Revision 1 said each column
derives "from `metadata.normalized.subagent_task` and `metadata.tool_call_id`", which
is true of most columns and **false of two** — and in both cases the two readings a
builder could pick produce observably different databases:

| Target column | Source on `task_session_messages` |
|---|---|
| `task_session_id`, `task_id` | the row's own columns of the same name |
| `turn_id` | *(revision 2)* the row's own **`turn_id` column**, empty string normalized to NULL |
| `tool_call_id` | `metadata.tool_call_id` |
| `parent_tool_call_id` | `metadata.parent_tool_call_id` |
| `agent_status` | `metadata.normalized.subagent_task.status` |
| `tool_status` | *(revision 2)* the **top-level `metadata.status`** |
| `subagent_type`, `description`, `agent_id`, `child_session_id`, `model`, `is_async`, `total_tokens`, `tool_use_count`, `duration_ms` | the same-named fields under `metadata.normalized.subagent_task` |
| `observed_at` | the row's `created_at` column |
| `settled_at` | the row's `updated_at` column, per AC-23a |
| `agent_execution_id` | not derivable; the `'unknown'` sentinel per AC-31 |

**`turn_id` comes from the message's own column, not from the JSON.** There is no
`turn_id` under either JSON path, so a builder following revision 1 literally finds
nothing and writes NULL for every backfilled row — silently discarding the historical
fan-out-per-turn population, including the 42 three-way turns this document's *Why*
section uses as the entire justification for the feature. The column is populated
from the same turn id the orchestrator resolves when it writes the message, which is
the same value the live path records.

**`tool_status` comes from the top-level `metadata.status`, and `agent_status` from
`metadata.normalized.subagent_task.status`. They are different fields and must not be
conflated.** This is the distinction § *Two statuses* exists to defend, and on the
backfill path revision 1 left it to a coin flip: the only status the spec named lives
under `normalized.subagent_task`, so one builder maps that onto `tool_status` —
which AC-11 flatly forbids, and which would wrongly settle the ~57 `completed` rows —
while another finds no ACP tool status at all and leaves `tool_status` and
`settled_at` NULL for all 253 rows. The correct source is the top-level key, which
the message writer populates from the frame's ACP tool status: the identical value
`isTerminalToolStatus` classifies on the live path, in the same closed vocabulary
AC-11 enumerates. A builder SHALL NOT substitute `agent_status` for it under any
circumstances.

**AC-21b** *(added by revision 2)* — WHEN two or more qualifying messages share the
same `(task_session_id, metadata.tool_call_id)`, THEN the backfill SHALL derive the
row from the message with the greatest **`created_at`**, tiebroken by the greatest
**`id`**, and SHALL insert exactly one row for that key.

Nothing constrains `task_session_messages` to be unique on that pair — `tool_call_id`
lives inside a JSON document, so no database constraint can express it, and this
document's own *Sampled input shapes* notes that the observed uniqueness "is a
property of this sample, not a guarantee." AC-22's `ON CONFLICT DO NOTHING` keeps the
statement from failing but names no winner, and AC-23a mandates a single
`INSERT … SELECT` with no `ORDER BY`. Without a named tiebreak, which message supplies
`observed_at`, `settled_at` and every derived column is non-deterministic between two
correct implementations and between two replays of the same one. Newest-first is the
right direction because a later message carries the more complete tool-call state;
`id` is named as the second key only to make the order total, and both are real
columns on the source table.

**AC-21a** *(added by revision 1)* — WHEN a qualifying message is missing any of
those three identities, THEN the backfill SHALL skip it: no row, and the
statement SHALL NOT fail.

The filter is stated explicitly because it is not implied by the column types and
the two readings differ observably. `task_session_messages.task_id` is declared
`TEXT DEFAULT ''` while `task_session_subagents.task_id` is `NOT NULL`, and `''`
satisfies `NOT NULL` in both dialects — so an unfiltered `INSERT … SELECT` would
happily write a `task_id = ''` row rather than failing loudly. Such a row is
exactly what AC-2 refuses on the live path, and for the same reason: a
session-scoped row with no card cannot be joined to anything this feature exists
to answer. The backfill and the live path SHALL agree on which frames are
recordable. These skips are part of AC-28's named allowance on the shortfall
side; they do not increment `skipped_no_identity`, which is a live-path counter.

**AC-22** *(amended by Amendment 1)* — WHEN the backfill encounters a
`(task_session_id, agent_execution_id, tool_call_id)` that already has a row,
THEN it SHALL leave the existing row untouched (`ON CONFLICT DO NOTHING`), so a
live write always wins over a backfilled one and a replay is a no-op. Because
the backfill always writes the `'unknown'` sentinel (AC-31) and that sentinel is
shared with pre-amendment live rows, a backfill replay conflicts with its own
prior rows and a backfill never duplicates a subagent already captured live
without an execution id.

WHEN a subagent was captured live **with** a real execution id and the backfill
later derives a row for the same `(task_session_id, tool_call_id)`, THEN the
keys differ and both rows SHALL exist. This is a named, accepted consequence of
execution-scoping rather than an oversight: the two rows are distinguishable by
`source`, AC-25 already forbids comparing `backfill` and `live` rows
like-for-like, and the alternative — matching a backfilled row onto a live row
by a two-column prefix — would reintroduce exactly the cross-execution
clobber this amendment removes. The backfill is a one-time statement, guarded
*(revision 4)* by the presence of **both** backfill keys (AC-24d), so this overlap is
bounded to installations that ran live capture before their backfill.

**AC-23** — WHEN the backfill derives a value, THEN AC-7 through AC-9 and the
empty-string-to-NULL rule SHALL apply identically: an absent JSON field becomes
NULL, never `0` and never `''`.

**AC-23a** *(source clarified in revision 2)* — The backfill SHALL be a single
unbounded `INSERT … SELECT` with no `LIMIT` and no batching, and SHALL set
`settled_at` from the message's `updated_at` when the derived `tool_status` is
terminal, else NULL. **"The derived `tool_status`" is the value AC-21 names — the
top-level `metadata.status` — classified against AC-11's six terminal values.** It is
never `agent_status`. It performs
one full scan of `task_session_messages` at first boot after upgrade — a real,
accepted one-time cost on a large store (the reference store is 328 MB), taken
once because a partial or resumable backfill would produce exactly the
mid-series discontinuity AC-25 exists to prevent.

**AC-23b** — WHEN the backfill runs on PostgreSQL, THEN it SHALL use the
dialect-aware JSON helpers already present in `base_migrations.go`
(`jsonColumn`, `jsonText`), which normalise empty and `'null'` metadata
documents so a single malformed row cannot abort the statement.
