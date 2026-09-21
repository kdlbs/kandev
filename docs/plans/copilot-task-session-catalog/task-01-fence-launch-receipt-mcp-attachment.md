---
id: "01-fence-launch-receipt-mcp-attachment"
title: "Fence launch receipt MCP attachment binding"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-MCP-SESSION-OBSERVABILITY-001
acceptance_criteria:
  - AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.4
system_design:
  - "../../specs/platform/system-design/mcp-session-observability.md"
---

# Task 01: Fence launch receipt MCP attachment binding

## Acceptance

- Lifecycle stamps accepted MCP attachment attempts and evidence with the
  startup generation leased to the callback, without mutating caller-owned
  values.
- The orchestrator binds catalog evidence to the current launch receipt only
  when session, execution, and startup generation match.
- A delayed attachment from an earlier startup generation of the same
  execution remains bounded diagnostic history and cannot overwrite the
  replacement receipt's catalog attachment ID.
- Current fresh and resumed attachments remain bindable when their immutable
  identity matches the receipt.

## Production paths

- `apps/backend/internal/agentctl/types/streams/mcp_attachment.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_events.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming.go`

## Verification

- Focused lifecycle and orchestrator Go tests, including the delayed
  same-execution attachment fixture.
- The same focused packages under `go test -race`.
- The repository PR documentation coverage evaluator with this work order,
  plan, requirement, and system-design links loaded from the Git snapshot.
