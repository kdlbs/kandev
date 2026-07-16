---
id: "05-backend-output-projection"
title: "Backend shell output projection"
status: done
wave: 4
depends_on: ["04-e2e-and-verification"]
plan: "plan.md"
spec: "../../specs/ui/acp-shell-command-output.md"
---

# Task 05: Backend Shell Output Projection

## Acceptance

- REST message lists, WebSocket `message.list`, task boot state, and live message notifications carry shell output summary fields but never `stdout` or `stderr`; typed live metadata and persisted map metadata produce the same non-mutating projection.
- `GET /api/v1/task-sessions/:session_id/messages/:message_id/shell-output` returns the latest bounded snapshot, status, and update timestamp for a shell message, including a valid empty-running snapshot.
- Missing, cross-session, and non-shell message IDs return `404`, while all existing non-shell message metadata remains compatible.

## Verification

```bash
make -C apps/backend fmt
cd apps/backend && go test ./internal/task/models -run 'Test.*ShellOutput|Test.*MessageMetadata'
cd apps/backend && go test ./internal/task/handlers -run 'Test.*ShellOutput|Test.*ListMessages'
cd apps/backend && go test ./internal/task/service ./internal/backendapp -run 'Test.*Message.*Projection|Test.*Boot.*Shell'
make -C apps/backend lint
```

## Files likely touched

- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/models/message_shell_output.go` (new)
- `apps/backend/internal/task/models/message_shell_output_test.go` (new)
- `apps/backend/internal/task/dto/dto.go`
- `apps/backend/internal/task/handlers/message_handlers.go`
- `apps/backend/internal/task/handlers/message_handlers_test.go`
- `apps/backend/internal/task/service/service_events.go`
- `apps/backend/internal/task/service/service_events_test.go`
- `apps/backend/internal/backendapp/helpers_test.go`

## Dependencies

Task 04 provides the persisted normalized output contract and shipped baseline. No schema or ACP adapter change is permitted.

## Inputs

- Spec sections `Data model`, `API surface`, `Failure modes`, and the summary/on-demand scenarios.
- ADR-0042 projection boundary and storage decision.
- Existing paths: `models.Message.ToAPI`, `service.publishMessageEvent`, `handlers.messagesToAPI`, `handlers.wsListMessages`, and `bootStateBuilder.addTaskDetailActiveTaskState`.
- Existing repository behavior: live metadata may contain `*streams.NormalizedPayload`; rows read from SQLite contain `map[string]any`.

## Output contract

Report the projection/extraction API, response/error mapping, typed-versus-map coverage, payload paths proven body-free, files changed, exact tests run, blockers, and residual risks. Set this task to `in_progress` before code and `done` with the matching `plan.md` checkbox only after targeted tests and lint pass.
