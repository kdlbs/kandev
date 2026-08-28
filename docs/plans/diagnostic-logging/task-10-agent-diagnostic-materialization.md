---
id: "10-agent-diagnostic-materialization"
title: "Agent diagnostic materialization"
status: completed
wave: 5
depends_on:
  - "07-diagnostic-bundle-backend"
  - "08-browser-logs-and-bundle-ui"
plan: "plan.md"
spec: "../../specs/platform/requirements/diagnostic-logging.md"
---

# Task 10: Agent diagnostic materialization

## Acceptance

- Task-mode MCP registers `get_diagnostic_bundle_kandev` with one required
  source enum (`backend`, `frontend`, or `all`) and no task, session, user,
  path, URL, or credential arguments.
- The MCP server supplies its immutable task/session IDs. Existing MCP identity
  scoping resolves the task owner before the handler creates the bundle, and
  task/session-pair authorization happens before bundle or runtime access.
- The handler waits for ready/partial within the five-minute job lifetime and
  streams the ZIP through agentctl to a server-selected owner-only
  `.kandev/diagnostics/<bundle-id>.zip` path in that execution workspace.
- Transfer is streaming and capped at 256 MiB; it does not marshal the ZIP into
  JSON/base64 or buffer it wholly in backend, MCP, WebSocket, or agentctl
  memory. Remote and local executors use the same bounded contract.
- The MCP result contains the executor-local path plus manifest completeness,
  source, byte-range, and loss summary. Backend-only never requests frontend
  capture.
- Materialized artifacts are removed on session/task workspace cleanup or
  after 24 hours, whichever comes first, and never appear in git status.
- `scripts/kandev-logs` supports host-side jobs. With auth enabled it reads a
  PAT only from `KANDEV_API_TOKEN`; it never accepts or prints raw credentials.

## Verification

```bash
cd apps/backend
go test ./internal/mcp/server ./internal/mcp/handlers \
  ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl \
  ./internal/agentctl/server/api
```

```bash
cd apps/backend
go test -race ./internal/mcp/handlers ./internal/agent/runtime/lifecycle
```

```bash
scripts/kandev-logs --help
```

## Files likely touched

- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/server_test.go`
- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/handlers/diagnostic_bundle.go`
- `apps/backend/internal/mcp/handlers/diagnostic_bundle_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_execution.go`
- `apps/backend/internal/agent/runtime/lifecycle/diagnostic_materializer.go`
- `apps/backend/internal/agent/runtime/lifecycle/diagnostic_materializer_test.go`
- `apps/backend/internal/agent/runtime/agentctl/client_diagnostics.go`
- `apps/backend/internal/agentctl/server/api/workspace_diagnostics.go`
- `apps/backend/internal/agentctl/server/api/workspace_diagnostics_test.go`
- `apps/backend/internal/system/logbundle/service.go`
- `scripts/kandev-logs`
- focused shell tests for `scripts/kandev-logs`

## Dependencies

- Task 07 supplies identity-owned source-selectable jobs and archives.
- Task 08 supplies frontend capture for frontend/all requests.

## Parallelism

Sequential after Tasks 07 and 08. This crosses MCP, runtime, agentctl, and the
bundle service and must settle before legacy debug export is removed.

## Inputs

- Spec: Agent diagnostics, Permissions, performance limits, and agent scenarios.
- Plan: Agent diagnostic materialization.
- Existing patterns: task-mode MCP registration, in-session identity scoping,
  task/session-pair authorization, lifecycle execution lookup, and agentctl
  workspace file services.

## Risks

- Never accept an agent-selected output path or infer identity from tool input.
- The task/session pair must be authorized before resolving its execution; a
  caller cannot combine its own session with another task.
- Streaming must propagate cancellation and close both source/destination
  handles without buffering a 256 MiB archive.
- A delayed materialization from an old execution cannot write into a successor
  execution; bind transfer to immutable execution identity and validate it
  again before the destination is committed.
- Cleanup removes only the server-owned diagnostics directory and never a
  workspace root, repository, or caller-selected path.

## Output contract

Report tool schema, identity and task/session ownership, local/remote streaming
path, archive/cleanup bounds, files changed, exact tests/results, blockers or
risks, and update this task plus `plan.md` status in the same conversation.
