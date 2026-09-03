---
id: "02-consume-comments-during-prompt-admission"
title: "Consume comments during prompt admission"
status: completed
wave: 2
depends_on: ["01-persist-shared-plan-comments"]
plan: "plan.md"
requirements:
  - REQ-TASKS-PLAN-COMMENTS-002
  - REQ-TASKS-PLAN-COMMENTS-003
acceptance_criteria:
  - AC-TASKS-PLAN-COMMENTS-002.2
  - AC-TASKS-PLAN-COMMENTS-002.3
  - AC-TASKS-PLAN-COMMENTS-002.4
  - AC-TASKS-PLAN-COMMENTS-002.5
  - AC-TASKS-PLAN-COMMENTS-002.6
  - AC-TASKS-PLAN-COMMENTS-002.7
  - AC-TASKS-PLAN-COMMENTS-003.1
  - AC-TASKS-PLAN-COMMENTS-003.2
  - AC-TASKS-PLAN-COMMENTS-003.3
  - AC-TASKS-PLAN-COMMENTS-003.4
  - AC-TASKS-PLAN-COMMENTS-003.6
system_design:
  - ../../specs/tasks/system-design/plan-comments.md
---

# Task 02: Consume comments during prompt admission

## Summary

Teach direct-message and queued-prompt admission to accept versioned plan
comment references, format backend-owned content, and consume it atomically.
Add the primary-session guard and durable queue identity required by Run.

## In scope

- Shared canonical plan-comment Markdown formatting from persisted rows.
- Optional `plan_comment_refs` on `message.add` and `message.queue.add`, plus
  empty-base-content validation.
- Conditional comment deletion and revision allocation inside direct-message
  and queue insert transactions.
- `require_primary_session`, stable conflict codes, direct replay, and
  caller-owned queue replay.
- Distinct non-auto-merged queue admission for busy Run.

## Out of scope

- Frontend target selection and UI error presentation.
- CRUD endpoints and comment snapshots owned by Task 01.
- Other comment-source formatting.

## Acceptance

- Persisted message/queue content uses only server-loaded comment text; stale,
  missing, duplicated, or cross-task references roll back the entire attempt.
- Exactly one concurrent attempt can accept a referenced comment, while direct
  and queued idempotent replays return their original durable row.
- Primary-guarded requests cannot land on a stale or non-primary session, and
  queue capacity or any admission failure leaves every comment pending.

## Verification

```bash
cd apps/backend && go test ./internal/task/handlers ./internal/task/repository/sqlite ./internal/orchestrator/handlers ./internal/orchestrator/messagequeue
```

## Files likely touched

- `apps/backend/internal/task/handlers/message_handlers.go`
- `apps/backend/internal/task/handlers/message_handlers_test.go`
- `apps/backend/internal/task/repository/sqlite/message.go`
- `apps/backend/internal/task/repository/sqlite/message_prompt_index.go`
- `apps/backend/internal/orchestrator/handlers/queue_handlers.go`
- `apps/backend/internal/orchestrator/handlers/queue_handlers_test.go`
- `apps/backend/internal/orchestrator/messagequeue/repository.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_sqlite.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_memory.go`
- `apps/backend/internal/orchestrator/messagequeue/service.go`
- `apps/backend/internal/orchestrator/messagequeue/types.go`
- `apps/backend/internal/task/plancomments/format.go`
- `apps/backend/internal/task/repository/plancommenttx/consume.go`

## Dependencies

Task 01.

## Risks

- The direct-message path runs workflow turn-start hooks before persistence;
  reference and primary revalidation must occur after any resolved session
  switch and before commit.
- Queue auto-merge and idempotent replay can conflict if a comment-bearing
  entry loses its original ID.
- Both repositories must take task and session locks in the established order
  to avoid a cross-path deadlock.

## Parallelism

`sequential`

## Inputs

- Requirements: `REQ-TASKS-PLAN-COMMENTS-002` and
  `REQ-TASKS-PLAN-COMMENTS-003`.
- System design: delivery references, composer delivery, Run routing, atomic
  acceptance, and failure recovery.
- Existing `createUserMessageWithBoundary`, message idempotency, queue
  admission, queue capacity, and auto-merge tests.

## Results

- Added one canonical server formatter plus a shared transactional resolver and
  conditional-consumption helper for direct and queued prompt admission.
- Extended direct messages with versioned references, replay fingerprints,
  primary guards, committed snapshot events, and stale turn-start protection.
- Added caller-owned queued admissions that bypass auto-merge, preserve
  attachments on rollback, and map revision, primary, capacity, and replay
  conflicts to stable WebSocket errors.
- Focused task handler/repository and orchestrator handler/queue suites pass.
