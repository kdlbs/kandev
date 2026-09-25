---
id: "01-persist-and-sync-feedback"
title: "Persist and synchronize preview feedback"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-001
acceptance_criteria:
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-001.1
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-001.2
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-001.3
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-001.4
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-001.5
system_design:
  - ../../specs/tasks/system-design/web-preview-feedback.md
---

# Task 01: Persist and Synchronize Preview Feedback

## Summary

Create the task-owned revisioned collection, screenshot claim state, authorized
WebSocket CRUD, and frontend snapshot projection. This work makes pending data
durable and recoverable before any production capture or delivery UI depends on
it.

## In scope

- SQLite and PostgreSQL schema, replay-safe migrations, constraints, indexes,
  task cascades, and stable ordering.
- Shared models and `previewfeedback` validators for kinds, fields, collection
  limits, PNG limits, and optimistic versions.
- Service and repository CRUD, idempotent caller-generated create IDs, clear by
  collection revision, and screenshot task claims and deletion cleanup.
- Authorized `task.preview_feedback.*` WebSocket actions, stable conflict/error
  codes, full snapshots, and changed events.
- A task-keyed frontend API, state slice, reconnect reconciliation, and hooks.
- Tests for session deletion and primary changes remaining irrelevant to task
  ownership.

## Out of scope

- Composer context and message delivery.
- Browser or HTML-preview capture controls.
- Client-side screenshot rasterization.

## Acceptance

- Backend restart, reconnect, session changes, and a second authorized client
  recover the same task snapshot and screenshot descriptors.
- Exact-version update/delete and revision-guarded clear reject stale clients
  without changing the collection.
- Task deletion, feedback deletion, and failed creates leave no unowned durable
  screenshot claims; transient object-delete failure remains recoverable.

## TDD sequence

1. Add failing repository and service contract tests for schema replay,
   constraints, limits, optimistic versions, task cascades, and attachment
   ownership; run them and record the expected failure.
2. Implement the minimum models, migrations, repositories, and service until
   those tests pass under the race detector.
3. Add failing handler and frontend snapshot tests for authorization,
   idempotency, events, reconnect, and stale revisions; run them RED.
4. Implement the WebSocket and frontend state boundary, rerun GREEN, then
   refactor duplicate plan-comment-shaped synchronization helpers only where
   both call sites remain clearer.

## Verification

```bash
cd apps/backend
go test -race ./internal/task/previewfeedback ./internal/task/repository/sqlite ./internal/task/service ./internal/task/handlers
cd ../../apps
pnpm --filter @kandev/web test -- --run lib/api/domains/preview-feedback-api.test.ts lib/state/slices/preview-feedback lib/ws/handlers/preview-feedback.test.ts hooks/domains/comments/use-preview-feedback.test.ts
cd web
pnpm run typecheck
pnpm exec eslint lib/api/domains/preview-feedback-api.ts lib/state/slices/preview-feedback lib/ws/handlers/preview-feedback.ts hooks/domains/comments/use-preview-feedback.ts
```

## Files likely touched

- `apps/backend/internal/task/models/`
- `apps/backend/internal/task/previewfeedback/`
- `apps/backend/internal/task/repository/interfaces.go`
- `apps/backend/internal/task/repository/sqlite/base_schema.go`
- `apps/backend/internal/task/repository/sqlite/base_migrations.go`
- `apps/backend/internal/task/repository/sqlite/preview_feedback*.go`
- `apps/backend/internal/task/service/preview_feedback_service*.go`
- `apps/backend/internal/task/handlers/task_preview_feedback_handlers*.go`
- `apps/backend/pkg/websocket/actions.go`
- `apps/web/lib/api/domains/preview-feedback-api*.ts`
- `apps/web/lib/state/slices/preview-feedback/`
- `apps/web/lib/ws/handlers/preview-feedback*.ts`
- `apps/web/hooks/domains/comments/use-preview-feedback*.ts`

## Dependencies

None.

## Risks

- A claimed image can leak if row deletion and storage cleanup do not share the
  existing attachment lifecycle.
- Database checks and byte accounting can diverge between SQLite and PostgreSQL
  unless the shared validator remains authoritative.
- Snapshot replacement must reject stale revisions without hiding a legitimate
  empty collection.

## Parallelism

`sequential`

## Inputs

- Persistence, limits, synchronization, lifecycle, and security sections in the
  system design.
- Existing task plan comment and prompt attachment implementations as adjacent
  reference patterns.

## Results

Implemented task-owned SQLite/PostgreSQL-compatible storage, optimistic CRUD,
idempotent creates, screenshot claim validation and cleanup, authorized
WebSocket actions, authoritative change events, and a reconnecting task-keyed
frontend snapshot projection. Focused backend tests pass under the race detector;
the frontend API, state, WebSocket, and hook tests plus the web typecheck pass.
