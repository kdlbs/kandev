---
id: "01-expose-saved-prompt-reads"
title: "Expose saved prompt reads"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-EXTERNAL-MCP-002
acceptance_criteria:
  - AC-INTEGRATIONS-EXTERNAL-MCP-002.1
  - AC-INTEGRATIONS-EXTERNAL-MCP-002.2
  - AC-INTEGRATIONS-EXTERNAL-MCP-002.3
  - AC-INTEGRATIONS-EXTERNAL-MCP-002.4
  - AC-INTEGRATIONS-EXTERNAL-MCP-002.5
  - AC-INTEGRATIONS-EXTERNAL-MCP-002.6
  - AC-INTEGRATIONS-EXTERNAL-MCP-002.7
  - AC-INTEGRATIONS-EXTERNAL-MCP-002.8
system_design:
  - ../../specs/integrations/system-design/external-mcp-shared-prompts.md
---

# Task 01: Expose saved prompt reads

## Summary

Add two read-only saved prompt tools to configuration and external MCP. Reuse
the current prompt repository and backend bridge without changing expansion.

## In scope

- Add prompt lookup by name to the prompt service.
- Add the list and get backend actions and handlers.
- Add the list and get MCP tools, schemas, annotations, and forwarding.
- Wire the prompt reader into MCP handlers.
- Update the configuration-agent context.
- Add focused service, handler, server, and context tests.

## Out of scope

- Prompt mutations over MCP.
- Prompt persistence or ownership changes.
- Workflow-step response changes.
- Public documentation updates.

## Acceptance

- Configuration and external MCP list both tools. Other MCP surfaces do not.
- List output omits content. Get output returns full content for an exact name.
- Invalid and unknown names return tool errors without content.

## Verification

```bash
go test ./internal/prompts/service ./internal/mcp/handlers ./internal/mcp/server ./internal/sysprompt ./pkg/websocket
```

Run this command from `apps/backend`.

## Files likely touched

- `apps/backend/internal/prompts/service/service.go`
- `apps/backend/internal/prompts/service/service_test.go`
- `apps/backend/pkg/websocket/actions.go`
- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/handlers/config_prompt_handlers.go`
- `apps/backend/internal/mcp/handlers/config_prompt_handlers_test.go`
- `apps/backend/internal/mcp/server/config_handlers.go`
- `apps/backend/internal/mcp/server/config_prompt_handlers_test.go`
- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/server_test.go`
- `apps/backend/internal/backendapp/helpers.go`
- `apps/backend/config/prompts/config-context.md`
- `apps/backend/internal/mcp/server/sysprompt_sync_test.go`

## Dependencies

None.

## Risks

- Reusing the full HTTP DTO for list output can expose content.
- A missing dependency can leave backend actions unregistered.

## Parallelism

`sequential`

## Inputs

- `REQ-INTEGRATIONS-EXTERNAL-MCP-002`
- `docs/specs/integrations/system-design/external-mcp-shared-prompts.md`
- Existing workflow export MCP handlers and tests.
- Existing prompt service, repository, and HTTP DTO.

## Results

Implemented the saved prompt read boundary.

- Added trimmed, case-sensitive name lookup with `ErrPromptNotFound` mapping in
  the prompt service.
- Added the two WebSocket actions and a narrow prompt-reader dependency for
  backend handlers.
- Added content-free list summaries and full name-based reads without internal
  prompt IDs.
- Registered both read-only tools only for configuration and external MCP
  surfaces, with the existing backend forwarding and request context.
- Added focused service, handler, server, and configuration-context coverage.
- `go test ./internal/prompts/service ./internal/mcp/handlers ./internal/mcp/server ./internal/sysprompt ./pkg/websocket` passed.
- `go test -race ./internal/prompts/service ./internal/mcp/handlers ./internal/mcp/server` passed.
