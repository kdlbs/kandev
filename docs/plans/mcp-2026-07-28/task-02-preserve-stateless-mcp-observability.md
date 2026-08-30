---
id: "02-preserve-stateless-mcp-observability"
title: "Preserve stateless MCP observability"
status: done
wave: 2
depends_on: ["01-serve-dual-era-mcp"]
plan: "plan.md"
spec: "../../specs/platform/requirements/mcp-session-observability.md"
---

# Task 02: Preserve Stateless MCP Observability

## Inputs

- `AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.4` and `.9`.
- `docs/specs/agents/system-design/mcp-protocol-compatibility.md` section
  `Attachment observability`.
- `docs/specs/platform/system-design/mcp-session-observability.md` sections
  `Evidence model`, `Session, execution, and attempt ownership`, and
  `Release-safe attachment report`.
- The amendment in
  `docs/decisions/2026-08-30-dual-era-mcp-protocol.md`.

## Change

1. Write failing evidence and hook tests for modern discovery, direct modern
   `tools/list`, modern tool calls, and legacy initialize and close behavior.
2. Add `protocol_accepted` to the attachment evidence kinds. Keep
   `initialize_observed` readable for legacy and stored history.
3. Record modern protocol acceptance against the current attachment attempt.
4. Keep modern `tools/list` catalog and tool-call evidence on that attempt.
5. Do not create an MCP connection ID for modern requests. Do not record
   `connection_closed` when a stateless HTTP request ends.
6. Preserve the current legacy connection ID, initialize, list, call, and
   close evidence.
7. Confirm that stored reports with only `initialize_observed` still rehydrate
   and derive the same status.

## Acceptance

- Modern discovery or another accepted modern request records
  `protocol_accepted` for the current attachment attempt.
- A direct modern `tools/list` makes the Kandev attachment Active and stores
  the safe tool catalog without a discovery event.
- Modern evidence contains no fabricated connection ID or per-request close
  event.
- Legacy initialize and connection close keep their existing evidence and
  status behavior.
- Old attachment histories that contain `initialize_observed` remain valid.

## Verification

```bash
cd apps/backend
go test -race ./internal/agentctl/types/streams ./internal/mcp/server ./internal/agent/runtime/lifecycle
git diff --check -- internal/agentctl/types/streams internal/mcp/server internal/agent/runtime/lifecycle
```

## Files likely touched

- `apps/backend/internal/agentctl/types/streams/mcp_attachment.go`
- `apps/backend/internal/agentctl/types/streams/mcp_attachment_test.go`
- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/server_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/mcp_attachment_snapshot_test.go`

## Dependencies

Task 01. Its SDK hooks define the available request and session context.

## Parallelism

Parallel-safe with Task 03. This task owns evidence types and MCP hook tests.

## Output contract

Report the modern and legacy evidence transitions, compatibility behavior for
stored history, files changed, commands run, failures, and risks. Update this
work order and `plan.md` with the result.

## Result

- Added `protocol_accepted` evidence and mapped it to the existing Connected status.
- Modern discovery, tools/list, and tools/call evidence uses the current attachment attempt and an empty connection ID.
- Modern requests do not create connection-close evidence.
- Legacy initialize evidence remains connection-owned. The v1 transport registers a legacy initialize session after handling the request, so initialization falls back to the current attempt until registration records the connection.
- Modern request hooks snapshot the attachment attempt before dispatch, so rollover during an in-flight request cannot move its evidence to a successor attempt.
- Legacy fallback is limited to initialize evidence, and `ConnectedAt` is retained from the first protocol acceptance.
- Legacy DELETE cleanup still emits connection-close evidence, and existing initialize history remains valid.
- Verification passed: focused rollover, fallback-scope, and write-once tests, followed by `go test -race ./internal/agentctl/types/streams ./internal/mcp/server ./internal/agent/runtime/lifecycle` and the scoped diff check.
