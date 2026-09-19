---
id: "01-capture-tool-catalog"
title: "Capture the Kandev tool catalog"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/mcp-session-observability.md"
---

# Task 01: Capture the Kandev Tool Catalog

## Acceptance

- The current Kandev attachment stores a sorted, bounded list of tool names and
  descriptions from the actual `tools/list` response.
- The report exposes the full count and a truncation marker. Historical
  attempts keep the count but not catalog entries.
- Persistence, boot hydration, and WebSocket serialization keep the optional
  catalog fields. No schema, argument, result, credential, or endpoint data is
  added.

## Verification

```bash
cd apps/backend && go test ./internal/agentctl/types/streams ./internal/mcp/server ./internal/agent/runtime/lifecycle ./internal/orchestrator
```

Write failing tests for the catalog bounds, history removal, MCP hook payload,
and JSON rehydration before the implementation.

## Files likely touched

- `apps/backend/internal/agentctl/types/streams/mcp_attachment.go`
- `apps/backend/internal/agentctl/types/streams/mcp_attachment_test.go`
- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/server_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/mcp_attachment_snapshot_test.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming_test.go`

## Dependencies

None.

## Parallelism

Sequential. Task 02 consumes this wire contract.

## Inputs

- Spec sections `Kandev tool catalog`, `Release-safe attachment report`, and
  `Failure modes`.
- ADR `2026-08-16-session-mcp-tool-catalog`.
- Existing `AddAfterListTools` and `MCPAttachmentHistory.Apply` behavior.

## Output contract

Report the normalized contract, catalog bounds, files changed, tests run,
blockers, and risks. Update this task and the plan status in the same session.

## Results

Implemented the safe current-attempt catalog contract. Kandev publishes only
tool names and descriptions from `tools/list`; entries are normalized to 128
non-empty names, descriptions are bounded to 1,024 UTF-8 bytes, and the full
tool count drives the truncation marker. Superseded attempts retain counts but
drop catalog entries.

Changed the stream contract, MCP hook observer, lifecycle JSON rehydration
fixture, catalog tests, and observer tests. The catalog fields remain
optional, so schema version 1 and existing generic persistence and WebSocket
serialization continue to work.

Verification:

```text
Go test: 4029 passed in 4 packages
```
