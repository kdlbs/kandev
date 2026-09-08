---
id: "01-attachment-storage-api"
title: "Attachment storage API"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/prompt-attachments.md"
---

# Task 01: Attachment storage API

## Acceptance

1. Authenticated multipart upload streams one raw file into private backend
   storage, accepts exactly 100 MiB, rejects 100 MiB + 1 with `413`, and leaves
   no committed file/usable row after a failed or interrupted upload.
2. The durable registry enforces owner/workspace/task/session scope, ten-file
   and 100 MiB aggregate claims, 24-hour staged expiry, descriptor-only message
   metadata, and atomic no-partial-claim behavior.
3. Authorized content download and staged deletion work without exposing a host
   path; attachment inventory/cleanup follows the typed storage-maintenance and
   fail-closed ownership boundary.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/task/service ./internal/task/repository/sqlite ./internal/task/handlers ./internal/backendapp
```

## Files likely touched

- `apps/backend/internal/task/models/attachment.go` (new)
- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/attachment.go` (new)
- `apps/backend/internal/task/repository/sqlite/base_schema.go`
- `apps/backend/internal/task/repository/sqlite/base_migrations.go`
- `apps/backend/internal/task/service/attachment_service.go` (new)
- `apps/backend/internal/task/handlers/attachment_handlers.go` (new)
- `apps/backend/internal/task/handlers/task_http_handlers.go`
- `apps/backend/internal/task/handlers/message_handlers.go`
- `apps/backend/internal/backendapp/helpers.go`
- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/backendapp/storage_maintenance.go`
- Focused `*_test.go` files beside the changed packages

## Dependencies

None.

## Parallelism

Sequential. This task establishes the shared schema, repository, storage root,
and service/API contract consumed by every later task.

## Inputs

- Spec: Data model, API surface, State machine, Permissions, Failure modes,
  Persistence guarantees
- Plan: Backend / Attachment registry and private storage; HTTP and submission contracts
- ADR 0045 and ADR-2026-08-04-file-backed-prompt-attachments

## Output contract

Report the schema/API shape, files changed, exact tests and outcomes, temporary
file cleanup evidence, authorization/side-effect boundaries, blockers, risks,
and synchronized task/plan status.

## Results

Implemented the SQLite attachment registry, authenticated multipart upload/content/delete handlers, private file-backed storage, exact 100 MiB boundary checks, ten-file/100 MiB aggregate claim validation, ownership checks, staged expiry cleanup, and atomic temporary-file promotion. Focused verification passed with `go test -tags fts5 ./internal/task/repository/sqlite ./internal/task/handlers ./internal/task/service ./internal/backendapp`.
