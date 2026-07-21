---
id: "03-mcp-stop-handler"
title: "MCP stop handler"
status: done
wave: 3
depends_on: ["02-coordinator-stop-operation"]
plan: "plan.md"
spec: "../../specs/tasks/parent-child-task-stop.md"
---

# Task 03: MCP stop handler

## Acceptance

- `mcp.stop_task` accepts only trusted direct-parent requests and rejects every
  broader relationship before stop side effects.
- Handler returns `stopped` or idempotent `not_running` with the target ID and
  maps validation, existence, authorization, and stop failures to typed errors.
- Production wiring supplies the structured orchestrator operation through a
  narrow `TaskStopper` dependency. Handler accepts neither reason nor force and
  never maps raw executor sentinels.
- Malformed JSON is `BAD_REQUEST`; required-ID, not-found, and forbidden cases
  produce their specified typed errors. Backend handler count becomes 25.
- Task-not-found sentinels and arbitrary task repository failures remain
  distinct. Production composition proves the orchestrator stopper is wired.
- Raw gateway clients cannot invoke any `mcp.*` action. A forged
  `sender_task_id` is rejected before dispatcher/handler side effects, while
  agent-stream MCP dispatch remains supported.

## Verification

```bash
cd apps/backend && go test ./internal/mcp/handlers ./internal/backendapp ./internal/gateway/websocket
```

## Files Likely Touched

- `apps/backend/pkg/websocket/actions.go`
- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/handlers/stop_task.go`
- `apps/backend/internal/mcp/handlers/stop_task_test.go`
- `apps/backend/internal/backendapp/helpers.go`
- `apps/backend/internal/backendapp/mcp_stop_composition_test.go`
- `apps/backend/internal/gateway/websocket/client.go`
- `apps/backend/internal/gateway/websocket/client_mcp_actions_test.go`

## Dependencies

- Task 02 defines the structured stop result consumed by this handler.

## Inputs

- Spec: `API surface`, `Permissions`, and `Failure modes`.
- Direct-parent precedent: `handleMessageTask` interrupt authorization.
- Production dependency wiring: `registerMCPAndDebugRoutes`.
- Authorization uses the same direct-parent predicate plus a same-workspace
  defense-in-depth check.
- Transport boundary:
  `ADR-2026-07-19-reject-mcp-actions-on-raw-websocket`.

## Output Contract

Update this task to `done`, update `plan.md`, and report action/response shapes,
authorization cases, files changed, tests run, blockers, and risks.

## Completion

- Added `mcp.stop_task`, a narrow orchestrator stopper dependency, explicit
  production wiring, and typed payload/lookup/authorization/error handling.
- Enforced direct-parent plus same-workspace authorization before mutation;
  reason, force, session, and sender spoof fields cannot reach lifecycle code.
- Raw `/ws` now rejects every `mcp.*` action with `FORBIDDEN` before shared
  dispatch, while direct trusted dispatcher tests preserve agent-stream use.
- Handler registration, relationship matrix, lookup failures, idempotent
  statuses, invalid dependency/status behavior, and forged raw transport are
  covered; gateway tests pass under `-race`.
- Production composition dispatches the action successfully through the real
  backend route graph and orchestrator stopper.
- Remaining trust assumption: mode-scoped MCP adapters and trusted in-process
  callers are the identity boundary described by the accepted ADR.
