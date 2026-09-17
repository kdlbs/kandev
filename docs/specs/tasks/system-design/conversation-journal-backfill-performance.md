---
status: current
system: tasks
requirements:
  - REQ-TASKS-JOURNAL-BACKFILL-001
---

# Conversation Journal Backfill Performance System Design

## Purpose and boundaries

`Repository.backfillConversationJournal`
(`apps/backend/internal/task/repository/sqlite/conversation_journal.go`) is a
startup step that pages through `task_session_turns` and
`task_session_messages` rows with no matching `conversation_turn_versions` /
`conversation_message_versions` row and journals them, one bounded batch per
transaction. This design owns the batch-selection query only. It does not own
the journal schema, the trigger-based write path for newly created turns and
messages (both predate this design), or the plugin session-event mirror
backfill, which pages a different table on its own schedule.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-JOURNAL-BACKFILL-001` | [Batch selection](#batch-selection) |

## Batch selection

Both loops (turns, then messages) share the same shape: select up to 100 rows
with no journal counterpart, ordered by `(task_session_id, started_at\|created_at, id)`,
journal each row in one transaction, then use the last row of the batch as the
keyset cursor for the next `SELECT`.

The cursor comparison is a row-value (tuple) comparison against all three
ordering columns together:

```sql
AND (t.task_session_id, t.started_at, t.id) > (?, ?, ?)
ORDER BY t.task_session_id, t.started_at, t.id LIMIT 100
```

An equivalent-looking OR-chain (`task_session_id > ? OR (task_session_id = ?
AND (started_at > ? OR (started_at = ? AND id > ?)))`) satisfies the same
ordering but is not sargable against a composite `(session_id, timestamp)`
index in SQLite: the planner falls back to `MULTI-INDEX OR` plus a temporary
B-tree sort of every remaining row, so batch cost grows with the size of the
remaining corpus instead of staying roughly constant. The row-value form lets
the planner seek `idx_turns_session_started(task_session_id, started_at)` /
`idx_messages_session_created(task_session_id, created_at)` directly to the
next batch (AC-TASKS-JOURNAL-BACKFILL-001.1). Because the comparison's third
term is the row's own `id`, a batch boundary that falls inside a group of rows
sharing one timestamp within a session still orders and pages correctly,
without skipping or repeating a row (AC-TASKS-JOURNAL-BACKFILL-001.2).

The two existing indexes are two columns wide; the third (`id`) term is
resolved by an in-batch scan bounded by the 100-row batch size. This is not a
correctness concern and is out of scope for this design (see the parent
requirement's exclusions).

## Persistence

Each batch commits in its own transaction before the next `SELECT`, so an
interrupted backfill resumes from the last committed cursor on the next boot
rather than restarting the whole corpus.

## Observability

The backfill emits no per-batch telemetry; a stalled backfill is currently
visible only as an extended, silent startup.
