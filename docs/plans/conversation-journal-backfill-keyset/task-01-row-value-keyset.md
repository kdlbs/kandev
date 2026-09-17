---
id: task-01-row-value-keyset
title: Row-value keyset for conversation journal backfill
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-TASKS-JOURNAL-BACKFILL-001
acceptance_criteria:
  - AC-TASKS-JOURNAL-BACKFILL-001.1
  - AC-TASKS-JOURNAL-BACKFILL-001.2
system_design:
  - docs/specs/tasks/system-design/conversation-journal-backfill-performance.md
---

# Task 01: Row-value keyset for conversation journal backfill

See [plan.md](plan.md).

## Outcome

`backfillConversationJournal`'s turn and message batch queries select the next
batch with a row-value (tuple) keyset comparison instead of an OR-chain, so
SQLite seeks the existing per-session index for every batch rather than
sorting the whole remaining table.

## In scope

- The two batch-selection `SELECT` queries in
  `apps/backend/internal/task/repository/sqlite/conversation_journal.go`
  (turns and messages), per
  [Batch selection](../../specs/tasks/system-design/conversation-journal-backfill-performance.md#batch-selection).
- A regression test proving a same-session, same-timestamp batch boundary
  still journals every row exactly once
  (`AC-TASKS-JOURNAL-BACKFILL-001.2`).

## Exclusions

Journal schema/retention/trigger changes, the plugin session-event mirror
backfill, and widening `idx_turns_session_started` /
`idx_messages_session_created` to cover `id` are out of scope; see the parent
requirement's "Out of scope" section.

## Acceptance conditions

1. Both batch queries compare `(task_session_id, started_at|created_at, id) >
   (?, ?, ?)` as a single row-value expression (`AC-TASKS-JOURNAL-BACKFILL-001.1`).
2. `TestConversationJournalBackfillPagesAcrossSessionsAndTurnTimestampTies`
   seeds more than one batch's worth of turns per session sharing a single
   `started_at`, runs the backfill, and asserts every source turn is
   journaled exactly once (`AC-TASKS-JOURNAL-BACKFILL-001.2`). Confirmed to
   fail against the prior OR-chain comparison and pass against the row-value
   comparison.

## Verification

Run from `apps/backend`:

```sh
go test ./internal/task/repository/sqlite/... -run TestConversationJournalBackfill -v
go vet ./internal/task/repository/sqlite/...
golangci-lint run ./internal/task/repository/sqlite/... --new-from-rev=85c93d3fde6cf99d1c20bedb2da0af8842c21cac --timeout=5m
```

## Results

Implemented and merged into the fix branch. All three verification commands
pass; the new test was confirmed to fail against the pre-fix OR-chain query
and pass against the row-value keyset.
