---
id: "08-message-reference-metadata"
title: "Durable message reference metadata"
status: completed
wave: 2
depends_on: ["01-core-and-task-search"]
plan: "plan.md"
spec: "../../specs/ui/entity-reference-composer.md"
---

# Task 08: Durable Message Reference Metadata

## Acceptance

- A neutral `internal/entityrefs` leaf structurally normalizes/canonicalizes arrays; `message.add` and queue add/update resolve the persisted conversation workspace and dispatch provider authorization through the mention registry before persistence.
- Queue persistence and both normal/workflow queue-drain paths preserve normalized reference arrays without reauthorizing previously accepted references.
- Queue update replaces references so deleted links cannot leave stale metadata; direct/queued arrays dedupe by canonical `ref`.
- Client projection and agent prompt context expose sanitized typed references without breaking legacy task mentions.

## Verification

```bash
cd apps/backend && go test ./internal/task/handlers/... ./internal/task/models/... ./internal/orchestrator/handlers/... ./internal/orchestrator/messagequeue/...
```

## Files likely touched

- `apps/backend/internal/orchestrator/message_meta.go`
- `apps/backend/internal/entityrefs/reference.go`
- `apps/backend/internal/entityrefs/reference_test.go`
- `apps/backend/internal/task/handlers/message_handlers.go`
- `apps/backend/internal/task/handlers/message_handlers_test.go`
- `apps/backend/internal/orchestrator/handlers/queue_handlers.go`
- `apps/backend/internal/orchestrator/handlers/queue_handlers_test.go`
- `apps/backend/internal/orchestrator/messagequeue/service.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_sqlite.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_sqlite_test.go`
- `apps/backend/internal/task/models/message_shell_output.go`
- `apps/backend/internal/task/models/message_shell_output_test.go`

## Dependencies

Task 01 normalized API type.

## Inputs

Spec data/API/persistence/failure sections; `UserMessageMeta`, existing message metadata projection, and queue `metadata_json` patterns.

## Output contract

Report validation/dedup/update semantics, files changed, exact tests, blockers, risks, and mark task/plan done.
