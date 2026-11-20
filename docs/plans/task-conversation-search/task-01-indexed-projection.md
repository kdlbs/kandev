---
id: "01-indexed-projection"
title: "Indexed conversation projection"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-CONVERSATION-SEARCH-001
  - REQ-TASKS-CONVERSATION-SEARCH-002
acceptance_criteria:
  - AC-TASKS-CONVERSATION-SEARCH-001.1
  - AC-TASKS-CONVERSATION-SEARCH-001.2
  - AC-TASKS-CONVERSATION-SEARCH-002.1
  - AC-TASKS-CONVERSATION-SEARCH-002.2
  - AC-TASKS-CONVERSATION-SEARCH-002.5
system_design:
  - ../../specs/tasks/system-design/conversation-search.md
---

# Task 01: Indexed conversation projection

## Summary

Create the rebuildable projection and native text indexes with transactional dirty tracking. Prove literal matching and canonical deletion before adding a background consumer.

## In scope

SQLite FTS5 and PostgreSQL pg_trgm capability migration, projection tables, dirty tokens, canonical triggers, bounded candidate query, plain excerpts, and idempotent fresh/replay upgrades.

## Out of scope

Background scheduling, API routes, frontend behavior, and public documentation.

## Acceptance

- Real SQLite and PostgreSQL tests prove quoted literal substrings, punctuation, accents, Unicode, excluded message types, and live ownership joins.
- Update/delete/rollback/cascade and delete-reinsert tests prove no stale text survives. Fresh and replay migrations work, including capability absence.
- Selective query plans use the native index. Candidate retrieval and excerpts obey the design bounds without exact counts.

## Verification

Run from the repository root. PostgreSQL uses a disposable test database.
The commands below use the delivered test names.

```bash
(cd apps/backend && go test -tags fts5 ./internal/task/repository/sqlite -run '^TestConversationSearch' -count=1)
(cd apps/backend && go test ./internal/task/repository/sqlite -run '^TestConversationSearchUnavailable' -count=1)
(cd apps/backend && test -n "$KANDEV_TEST_POSTGRES_DSN" && go test -tags fts5 -race ./internal/task/repository/sqlite -run '^TestConversationSearchPostgres' -count=1)
git diff --check
```

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/conversation_search*.go (new)`
- `apps/backend/internal/task/repository/sqlite/base_migrations.go`
- `apps/backend/internal/task/repository/sqlite/base_schema.go`
- `apps/backend/internal/task/repository/sqlite/message.go (canonical SQL paths to audit)`
- `apps/backend/internal/task/models/conversation_search.go (new)`

## Dependencies

None.

## Risks

External-content FTS synchronization and cascade triggers need behavior tests, not DDL-string checks. PostgreSQL extension provisioning may be denied. Do not install source foreign keys that invert the worker lock order.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/conversation-search.md).
- [System design](../../specs/tasks/system-design/conversation-search.md).
- [Plan](plan.md), scoped AGENTS.md, and the exact source/test patterns named there.

## Results

Implemented and validated on SQLite and a real PostgreSQL server. Matching, eligibility, stale-token rejection, edits, rollback, ownership transfer, deletion, and replay tests pass. Missing-FTS startup also passes. See [performance results](performance-results.md) for commands and evidence limits.
