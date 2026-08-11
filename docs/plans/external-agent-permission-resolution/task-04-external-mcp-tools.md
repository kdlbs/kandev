---
id: "04-external-mcp-tools"
title: "External MCP tools"
status: completed
wave: 4
depends_on: ["03-authorized-permission-service"]
plan: "plan.md"
spec: "../../specs/agents/external-permission-resolution.md"
---

# Task 04: External MCP tools

## Acceptance

- External discovery exposes exactly `list_pending_agent_permissions_kandev` and
  `resolve_agent_permission_kandev` with the typed schemas in the spec; other MCP surfaces do not.
- Handlers call only the authorized permission service, preserve foreign/not-found privacy, return
  stable descriptive failures, and cannot accept cancellation, arbitrary commands/options, option
  metadata, or hidden server context as tool arguments.
- Unit and authenticated integration tests cover happy list/resolve, wrong user/task/session,
  unknown option, stale/replaced/expired, duplicate/replay, no pending request, audit/delivery
  failure, and redaction canaries with exactly one provider response.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/mcp/handlers ./internal/mcp/server ./internal/integration
```

## Files likely touched

- `apps/backend/pkg/websocket/actions.go`
- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/handlers/agent_permissions.go`
- `apps/backend/internal/mcp/handlers/agent_permissions_test.go`
- `apps/backend/internal/mcp/handlers/mcp_identity_scope_test.go`
- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/handlers.go`
- `apps/backend/internal/mcp/server/agent_permissions_test.go`
- `apps/backend/internal/backendapp/helpers.go`
- `apps/backend/internal/integration/agent_permission_mcp_test.go`

## Dependencies

Task 03.

## Parallelism

Parallel-safe with task 05 after task 03. This task owns backend MCP/registration/integration files;
task 05 owns web and documentation files. User authorization is still required for parallel agents.

## Inputs

- Spec: complete MCP API and failure-code contract.
- Existing patterns: `list_task_sessions_kandev`, external profile registration, MCP identity scope
  tests, and runtime schema validation.

## Risks

- Tool discovery is not authorization; tests must invoke stale/disallowed calls directly.
- MCP error wrapping must preserve stable codes rather than flattening every domain failure to an
  internal error.

## Output contract

Report tool schemas/surface membership, integration security cases, exact commands/results, files
changed, blockers/risks, then update task/plan status.

## Results

Completed 2026-08-11.

- Registered `list_pending_agent_permissions_kandev` and
  `resolve_agent_permission_kandev` exclusively on the external MCP surface. The list schema is
  `task_id` plus optional `session_id`; the resolution schema requires exactly task, session,
  request, pending, and option IDs.
- Added a narrow MCP handler interface over the authorized orchestrator service, stable domain
  error codes, safe generic handling for unexpected errors, and backend wiring. No cancellation,
  command, option metadata, or caller-authored provider input is accepted.
- External-MCP resolution attribution now requires a process-local transport attestation installed
  only by the authenticated external dispatcher bridge. Direct regular WebSocket and in-session
  dispatcher traffic cannot forge the `external_mcp` audit source.
- Added handler/schema/surface/forwarding tests and an authenticated integration test covering
  foreign PAT denial, happy list/resolve, durable PAT audit attribution without token identity,
  replay rejection, transport-source forgery rejection, sensitive command/CWD/action-detail
  redaction, and exactly one provider response. Task 01 and Task 03 tests cover projection redaction
  plus unknown option, stale/replaced, missing, concurrent, audit, and delivery failures at their
  owning boundaries.
- Verification passed:
  `cd apps/backend && go test -tags fts5 ./internal/mcp/handlers ./internal/mcp/server ./internal/integration`
  (`ok`, with the pinned Go 1.26.0 binary and temporary Go caches; loopback permission was required
  for repository `httptest` suites).
