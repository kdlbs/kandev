---
id: "03-build-parent-question-lifecycle"
title: "Build the parent question lifecycle"
status: done
wave: 3
depends_on:
  - "01-persist-task-contract"
  - "02-derive-runtime-contract"
plan: "plan.md"
spec: "../../specs/tasks/autopilot-mode.md"
---

# Task 03: Build the Parent Question Lifecycle

## Acceptance

- `ask_parent_question_kandev` validates and durably records one structured pending
  request, routes it to the resolved direct parent, returns immediately, and never
  creates an operator clarification.
- A correlated direct-parent reply resolves and resumes the recorded child exactly
  once; duplicate, unauthorized, mismatched, superseded, and stale replies have no
  delivery side effect.
- Pending questions gate turn completion/queue draining, project to the existing
  sidebar clarification state, survive restart, and clear on every specified exit.

## Verification

```bash
cd apps/backend && go test ./internal/mcp/server ./internal/mcp/handlers ./internal/task/repository/sqlite ./internal/task/statussummary ./internal/task/handlers ./internal/orchestrator/... ./internal/clarification/...
```

## Files likely touched

- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/handlers.go`
- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/repository/sqlite/message.go`
- `apps/backend/internal/task/statussummary/model.go`
- `apps/backend/internal/task/statussummary/projector.go`
- `apps/backend/internal/task/statussummary/rebuild.go`
- `apps/backend/internal/task/handlers/message_handlers.go`
- `apps/backend/internal/orchestrator/event_handlers_clarification.go`
- `apps/backend/internal/orchestrator/clarification_guard.go`
- `apps/backend/internal/clarification/types.go`
- `apps/backend/internal/clarification/store.go`
- `apps/backend/internal/clarification/canceller.go`
- `apps/backend/internal/mcp/handlers/message_task_test.go`
- `apps/backend/internal/task/statussummary/projector_test.go`
- `apps/backend/internal/task/statussummary/rebuild_test.go`
- `apps/backend/internal/orchestrator/event_handlers_clarification_test.go`

## Dependencies

- Task 01 supplies task identity and persistence.
- Task 02 supplies the discoverable tool and autopilot prompt contract.

## Parallelism

Backend lifecycle work is serialized after Task 02 because it implements the tool
whose inventory Task 02 defines. Documentation may proceed in parallel once the
public payload names are fixed.

## Inputs

- Spec sections `Parent question protocol`, `State model`, `Persistence and restart`, and `Failure behavior`.
- Existing `message_task_kandev` queue/interrupt service, clarification pending
  actions, status-summary rebuild, prompt-generation guard, and turn-completion flow.

## Output contract

Report the hidden message type and metadata schema, question/reply API payloads,
transaction and serialization boundaries, state transitions, restart behavior,
idempotency key, exact error codes, tests run, and results for answer/supersede/
terminal races.

## Results

Done. Added durable parent-question messages, direct-parent authorization,
`reply_to_question_id` correlation, idempotent answer handling, child waiting and
resume behavior, task pending-action projection, and the parent-question MCP profile
registration. Handler/server/orchestrator tests cover the lifecycle and event order.
