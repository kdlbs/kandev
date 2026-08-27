---
id: "01-persist-compaction-count"
title: "Persist inferred compaction counts"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/context-compaction-count.md"
---

# Task 01: Persist inferred compaction counts

## Intent

Turn consecutive persisted context-usage samples into an idempotent, restart-safe lifetime count on each task session and publish that count with live context updates.

## Acceptance

- One atomic repository update stores the incoming context window and increments `context_compaction_count` only when its valid `used` value is lower than the prior persisted baseline; duplicate, first, equal, and increasing samples do not increment.
- A new backend service instance continues from persisted metadata, while explicit context resets and model changes retain the count and clear only the comparison baseline.
- Successful `session.state_changed` metadata patches contain the window and resulting count together; failed or generation-stale writes publish neither.

## TDD sequence

1. Add SQLite and PostgreSQL repository behavior cases and run them to confirm no atomic context-window/count method exists.
2. Add orchestrator persistence, restart, reset, duplicate, and event-patch cases and run them to confirm the count is absent.
3. Implement the typed metadata keys, repository contract, dialect-specific atomic update, and guarded orchestrator propagation; rerun the exact checks and refactor without widening scope.

## Files likely touched

- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/repository/sqlite/session.go`
- `apps/backend/internal/task/repository/sqlite/session_test.go`
- `apps/backend/internal/task/repository/sqlite/postgres_schema_test.go`
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/context_window.go`
- `apps/backend/internal/orchestrator/event_handlers_git.go`
- `apps/backend/internal/orchestrator/event_handlers_git_test.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming.go`
- `apps/backend/internal/orchestrator/executor/executor_interaction.go`
- `apps/backend/internal/orchestrator/executor/executor_office.go`
- `apps/backend/internal/backendapp/helpers.go`
- `apps/backend/internal/backendapp/helpers_test.go`
- `apps/backend/internal/task/service/service_turns.go`

## Dependencies

None.

## Parallelism

`sequential` — repository and orchestrator changes share the session metadata contract and must land as one coherent backend slice.

## Verification

- `cd apps/backend && go test -run 'Test(UpdateSessionContextWindow|PostgresUpdateSessionContextWindow|ContextWindowCompaction)' ./internal/task/repository/sqlite ./internal/orchestrator`

## Inputs

- Spec `What`, `Data model`, `Failure modes`, `Persistence guarantees`, and the first six scenarios.
- Existing guarded write boundary in `apps/backend/internal/orchestrator/context_window.go`.
- Existing context event persistence and broadcast in `apps/backend/internal/orchestrator/event_handlers_git.go`.
- Dialect rules in `apps/backend/AGENTS.md` and replay-safe metadata patterns in `apps/backend/internal/task/repository/sqlite/session.go`.

## Output contract

Report the result, actual files changed, exact tests and counts, blockers, risks, and synchronized task/plan status in this conversation.

## Results

- RED: `cd apps/backend && go test -run TestContextWindowDecreasePersistsCompactionCount ./internal/orchestrator` — failed as expected because the pre-change persistence path had no `context_compaction_count`.
- GREEN: `cd apps/backend && go test -run TestUpdateSessionContextWindowSQLiteCountsStrictUsageDrops ./internal/task/repository/sqlite` — 1 passed.
- GREEN: `cd apps/backend && go test -run 'Test(ContextWindowDecreasePersistsCompactionCount|ContextWindowCompactionCountSurvivesResetAndServiceRestart|ContextWindowCompactionCountIsPublishedWithWindowUpdate|ContextWindowResetDropsStale)' ./internal/orchestrator` — 5 passed.
- GREEN: `cd apps/backend && go test -run 'Test(UpdateSessionContextWindowSQLiteCountsStrictUsageDrops|PostgresUpdateSessionContextWindowCountsStrictUsageDrops)' ./internal/task/repository/sqlite` — SQLite behavior passed; PostgreSQL case was environment-gated and skipped because no test DSN was configured.
- Final targeted package check: `cd apps/backend && go test ./internal/task/repository/sqlite ./internal/orchestrator ./internal/backendapp ./internal/task/service` — passed.
- Full backend check: `cd apps/backend && go test ./...` — passed.
- Review fixup: context-window event persistence is synchronous to preserve arrival order, and model-change clears route through the guarded reset callback. Full `go test -race ./...` was not rerun locally; the PR race job had already exposed an unrelated `internal/integration` failure on the previous head.
- `git diff --check` — passed. No external services or durable data outside the test databases were changed.
