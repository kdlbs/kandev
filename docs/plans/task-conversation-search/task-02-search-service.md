---
id: "02-search-service"
title: "Background indexing and authorized search API"
status: done
wave: 2
depends_on: ['01-indexed-projection']
plan: "plan.md"
requirements:
  - REQ-TASKS-CONVERSATION-SEARCH-001
  - REQ-TASKS-CONVERSATION-SEARCH-002
  - REQ-TASKS-CONVERSATION-SEARCH-003
acceptance_criteria:
  - AC-TASKS-CONVERSATION-SEARCH-001.1
  - AC-TASKS-CONVERSATION-SEARCH-001.3
  - AC-TASKS-CONVERSATION-SEARCH-001.4
  - AC-TASKS-CONVERSATION-SEARCH-002.1
  - AC-TASKS-CONVERSATION-SEARCH-002.3
  - AC-TASKS-CONVERSATION-SEARCH-002.4
  - AC-TASKS-CONVERSATION-SEARCH-002.5
  - AC-TASKS-CONVERSATION-SEARCH-003.2
  - AC-TASKS-CONVERSATION-SEARCH-003.4
system_design:
  - ../../specs/tasks/system-design/conversation-search.md
---

# Task 02: Background indexing and authorized search API

## Summary

Run historical and live indexing under one lifecycle owner. Expose bounded conversation matches through an authorized workspace endpoint.

## In scope

Worker lifecycle, restart checkpoints, token compare-and-delete, live/backfill fairness, request admission and cancellation, DTOs, capability states, and backendapp startup/restore shutdown.

## Out of scope

Palette rendering and phone layout. Do not change the general workspace metadata endpoint or session-local Find.

## Acceptance

- Concurrent SQLite and PostgreSQL tests force both source/index commit orders, deletion, rollback, transfer, restart and historical/live overlap without lost work or stale results.
- HTTP tests deny unauthorized workspaces before searching, return bounded excerpts/results, and distinguish indexing, invalid queries, missing capability and limited execution.
- Worker shutdown joins before database closure, storage restore quiesces it, and cancelled database reads release their connection.

## Verification

Run from the repository root. PostgreSQL uses a disposable test database.
The commands below use the delivered test names.

```bash
(cd apps/backend && go test -tags fts5 -race ./internal/task/conversationsearch ./internal/task/handlers ./internal/task/service ./internal/backendapp -run 'ConversationSearch' -count=1)
(cd apps/backend && test -n "$KANDEV_TEST_POSTGRES_DSN" && go test -tags fts5 -race ./internal/task/repository/sqlite -run '^TestConversationSearch' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/task/repository/sqlite -run '^TestConversationSearch(Consistency|Cancellation)' -count=1)
git diff --check
```

## Files likely touched

- `apps/backend/internal/task/conversationsearch/ (new)`
- `apps/backend/internal/task/repository/sqlite/conversation_search*.go`
- `apps/backend/internal/task/service/conversation_search.go (new)`
- `apps/backend/internal/task/handlers/conversation_search.go (new)`
- `apps/backend/internal/task/handlers/task_http_handlers.go`
- `apps/backend/internal/task/dto/conversation_search.go (new)`
- `apps/backend/internal/backendapp/conversation_search_wiring.go (new)`
- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/backendapp/storage.go`
- `apps/backend/internal/task/repository/interface.go`

## Dependencies

Task 01, `01-indexed-projection`.

## Risks

Do not hold a source lock after taking a dirty-row lock. Metadata-only updates must not create index churn. Register cleanup in both ordinary shutdown and restore ownership paths.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/conversation-search.md).
- [System design](../../specs/tasks/system-design/conversation-search.md).
- [Plan](plan.md), scoped AGENTS.md, and the exact source/test patterns named there.

## Results

Implemented and validated. Authorization, admission limits, cancellation, backfill checkpoints, live/history fairness, concurrent PostgreSQL commit orders, and stop/join wiring pass. Restore ownership is covered by cleanup wiring; a full storage-restore rehearsal is still unmeasured. See [performance results](performance-results.md) for commands and evidence limits.
