---
id: "01-atomic-admission"
title: "Atomic initial content selection"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-INITIAL-TASK-BRIEF-001
acceptance_criteria:
  - AC-TASKS-INITIAL-TASK-BRIEF-001.3
  - AC-TASKS-INITIAL-TASK-BRIEF-001.5
  - AC-TASKS-INITIAL-TASK-BRIEF-001.6
system_design:
  - ../../specs/tasks/system-design/initial-task-brief.md
---

# Task 01: Atomic initial content selection

## Summary

Extend the existing message transaction with an optional server-owned initial candidate.
Keep ordinary callers unchanged until Task 02 opts in.

## In scope

- Add typed internal admission options through service and repository contracts.
- Select content before prompt ordinal allocation under the existing write lock.
- Share selection with plan-comment and queued plan-comment transactions.
- Preserve replay identity, description snapshot validation, and rollback restoration.

## Out of scope

Do not change unrelated launch policy, historical messages, or public API fields.

## Acceptance

- `TestInitialTaskBriefAdmission` proves one winner, rollback retry, stale snapshot rejection, zero reservation, deletion, and restart behavior.
- `TestInitialTaskBriefAdmissionPostgres` proves the same boundary with separate connections.
- Candidate content, trusted-context identity, message metadata, and queue content commit together.

## Verification

Start with failing behavior assertions before implementation. Then run this
block from the repository root:

```bash
(cd apps/backend && go test -tags fts5 ./internal/task/repository/sqlite ./internal/task/service -run 'InitialTaskBrief|Prompt|MessageWithPlanComments' -count=1)
# For PostgreSQL parity, configure a disposable test database first.
(cd apps/backend && test -n "$KANDEV_TEST_POSTGRES_DSN" && go test -tags fts5 ./internal/task/repository/sqlite -run '^TestInitialTaskBriefAdmissionPostgres$' -count=1)
git diff --check
```

## Files likely touched

- `apps/backend/internal/task/service/service_messages.go`
- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/message.go`
- `apps/backend/internal/task/repository/sqlite/message_prompt_index.go`
- `apps/backend/internal/task/repository/sqlite/message_plan_comment.go`
- `apps/backend/internal/task/repository/sqlite/message_initial_task_brief_test.go (new)`
- `apps/backend/internal/task/repository/sqlite/message_initial_task_brief_postgres_test.go (new)`

## Dependencies

None.

## Risks

Do not use a separate history read or committed reservation followed by a later message insert.
A skipped PostgreSQL test is not parity evidence. Use existing disposable database fixtures.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/initial-task-brief.md).
- [System design](../../specs/tasks/system-design/initial-task-brief.md).
- [Package evidence and test matrix](plan.md).

## Results

- Added the server-owned initial candidate through service and repository admission contracts.
- Selected the candidate inside the existing prompt boundary transaction, with task-row snapshot validation and rollback restoration.
- Preserved idempotency and covered normal, plan-comment, and queued plan-comment writes.
- Added SQLite coverage for two competing direct submissions, fallback races, zero-valued fallback reservations, stale descriptions, rollback, deletion, restart, and plan/queue boundaries.
- Added a cross-connection PostgreSQL parity test. It skips when `KANDEV_TEST_POSTGRES_DSN` is unset.
- Validated the selected rendered prompt against `plancomments.MaxRenderedPromptBytes` before insertion, with rollback coverage for an oversized candidate.
- `go test -tags fts5 ./internal/task/repository/sqlite -run '^TestInitialTaskBrief' -count=1` passed.
- `go test -race -tags fts5 ./internal/task/repository/sqlite -run '^TestInitialTaskBrief' -count=1` passed.
- `go vet ./internal/task/repository/sqlite ./internal/task/service ./internal/task/handlers ./internal/orchestrator ./internal/backendapp` passed.
