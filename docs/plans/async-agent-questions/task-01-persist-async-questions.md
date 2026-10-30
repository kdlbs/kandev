---
id: "01-persist-async-questions"
title: "Persist async requests and delivery intents"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ASYNC-QUESTIONS-001
  - REQ-TASKS-ASYNC-QUESTIONS-002
  - REQ-TASKS-ASYNC-QUESTIONS-003
  - REQ-TASKS-ASYNC-QUESTIONS-004
acceptance_criteria:
  - AC-TASKS-ASYNC-QUESTIONS-001.3
  - AC-TASKS-ASYNC-QUESTIONS-002.1
  - AC-TASKS-ASYNC-QUESTIONS-002.2
  - AC-TASKS-ASYNC-QUESTIONS-002.3
  - AC-TASKS-ASYNC-QUESTIONS-002.4
  - AC-TASKS-ASYNC-QUESTIONS-003.3
  - AC-TASKS-ASYNC-QUESTIONS-004.3
system_design:
  - ../../specs/tasks/system-design/async-agent-questions.md
---

# Task 01: Persist async requests and delivery intents

## Summary

Create the task-owned async request store and neutral service contracts.
Prove atomic resolution, retry identity, cleanup, and durable delivery intents before exposing a new tool path.

## In scope

- The design's durable-state tables, schema ownership, indexes, and replayable migration behavior.
- Canonical request creation, strict replay fingerprints, one winning terminal outcome, and immutable delivery intents.
- Narrow repository/service ports under `internal/task/questions/`, without an orchestrator dependency.
- Stored task/session/incarnation binding, parent relationship serialization, and visibility-aware reads.
- Conditional cleanup contracts for terminal sessions, task deletion/archive, and invalidated hierarchy.
- Persistence conformance and environment-gated PostgreSQL race/replay coverage.

## Out of scope

- MCP registration, provider dispatch, explicit pause execution, and rendered UI.
- Blocking clarification schema or current-turn query changes.

## Acceptance

1. `TestAsyncQuestionsCreateReplay` fails before implementation, then proves replay identity and conflicting-content rejection.
2. Resolution races and failure injection prove one canonical outcome and its intents across database reopen, without external I/O inside transactions.
3. Fresh-schema, upgrade/replay, cleanup, and real PostgreSQL concurrency checks pass. Missing PostgreSQL evidence remains explicitly pending.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -trimpath -race ./internal/task/repository/sqlite -run '^TestAsyncQuestions' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/task/questions -run '^TestAsyncQuestionsStore' -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -trimpath -race ./internal/persistence/storeconformance -count=1)
```

For required real PostgreSQL evidence:

```bash
test -n "${KANDEV_TEST_POSTGRES_DSN:?Set a disposable PostgreSQL test DSN}"
(cd apps/backend && go test -trimpath -race ./internal/task/repository/sqlite -run '^TestAsyncQuestionsPostgres' -count=1 -v)
```

Run `bash scripts/lint-go-changed` with this work order's actual changed Go file paths after they exist.
New tests include `TestAsyncQuestionsResolutionWinner`, `TestAsyncQuestionsRestart`,
`TestAsyncQuestionsLifecycleCleanup`, and `TestAsyncQuestionsOutboxCrashWindow`.
The implementation adds equivalent PostgreSQL tests, including multiple real connections.

## Files likely touched

- **New:** `apps/backend/internal/task/models/async_question.go`
- **New:** `apps/backend/internal/task/questions/types.go`, `store.go`, and `store_test.go`
- **New:** `apps/backend/internal/task/repository/sqlite/async_questions.go`, `async_questions_test.go`, `async_questions_postgres_test.go`
- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/base_schema.go`, `base_migrations.go`
- `apps/backend/internal/persistence/requiredstores/catalog.go`
- `apps/backend/internal/persistence/storeconformance/adapters.go`, `scenarios.go`, and applicable upgrade fixtures

New paths are proposed implementation files. Existing paths are integration points.

## Dependencies

None. Recheck the source snapshot and any newly landed question changes before writing the RED tests.

## Risks

- Task SQL also serves PostgreSQL. SQLite-only assumptions can hide hierarchy and resolution races.
- Task models must not import queue or orchestrator types.
- A new table omitted from cleanup or conformance can survive reset or fail on upgrade.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/async-agent-questions.md), requirements `001` through `004`.
- [Design](../../specs/tasks/system-design/async-agent-questions.md), durable state, cleanup, and security boundaries.
- `apps/backend/AGENTS.md`, including SQL migration and PostgreSQL lock guidance.
- `.agents/skills/tdd/references/backend-tests.md`
- Existing queue admission receipt and clarification resolution race tests as patterns, not replacement contracts.

## Results

Completed and remediated after review. Added the task-owned async lifecycle
tables and `internal/task/questions/` ports, including retry identity, one
winning terminal outcome, immutable delivery intents, explicit wait gates,
per-session incarnation capture, current task/session/parent authority checks
serialized with the terminal transition, atomic withdrawal wake intents, and
session/task cleanup hooks.

Verification passed: targeted SQLite async tests, service tests, sqlguard, and
required-store catalog tests. PostgreSQL race/replay tests remain gated on
`KANDEV_TEST_POSTGRES_DSN`. Remediation regressions cover reparent, workspace
transfer, terminal session, replacement incarnation, atomic wake, and parent
delivery identity.
