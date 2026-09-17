---
created: 2026-09-17
status: done
requirements:
  - REQ-TASKS-JOURNAL-BACKFILL-001
system_design:
  - ../../specs/tasks/system-design/conversation-journal-backfill-performance.md
---

# Conversation Journal Backfill Keyset Plan

## Outcome

Replace the OR-chain keyset comparison in `backfillConversationJournal`'s turn
and message batch queries with a row-value (tuple) comparison, so batch
selection stays an index seek against the existing per-session indexes
regardless of how much of the corpus has already been backfilled.

## Scope

- `apps/backend/internal/task/repository/sqlite/conversation_journal.go`: the
  two batch-selection queries (turns, messages).
- Regression coverage for the timestamp-tie case in
  `apps/backend/internal/task/repository/sqlite/conversation_journal_test.go`.

## Out of scope

See `REQ-TASKS-JOURNAL-BACKFILL-001`'s "Out of scope" section: journal schema,
retention, and trigger-path changes; the plugin session-event mirror backfill;
and widening the two per-session indexes to cover `id`.

## Work packages

- [task-01-row-value-keyset.md](task-01-row-value-keyset.md): implement the
  row-value keyset and its regression test.
