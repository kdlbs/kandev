---
id: "02-agentctl-credentialed-entries"
title: "Credentialed built-in entries in agentctl"
status: done
wave: 2
depends_on:
  - "01-agentctl-credential-enforcement"
plan: "plan.md"
requirements:
  - REQ-AGENTS-BUILTIN-MCP-CALLER-AUTH-001
  - REQ-AGENTS-BUILTIN-MCP-CALLER-AUTH-002
  - REQ-AGENTS-BUILTIN-MCP-CALLER-AUTH-003
acceptance_criteria:
  - AC-AGENTS-BUILTIN-MCP-CALLER-AUTH-001.3
  - AC-AGENTS-BUILTIN-MCP-CALLER-AUTH-002.1
  - AC-AGENTS-BUILTIN-MCP-CALLER-AUTH-002.2
  - AC-AGENTS-BUILTIN-MCP-CALLER-AUTH-002.3
  - AC-AGENTS-BUILTIN-MCP-CALLER-AUTH-003.4
system_design:
  - ../../specs/agents/system-design/builtin-mcp-caller-auth.md
---

# Task 02: Credentialed built-in entries in agentctl

## Summary

Every built-in server entry that agentctl builds carries
`Authorization: Bearer <instance credential>`. ACP and Codex app-server deliver
it on new, load, resume, and reset. Permission and managed-tool policies keep
recognizing the built-in entry by its exact header.

## In scope

- `config.BuiltinMCPServers` and `injectKandevMcpServerAtHost` take the
  credential. `injectKandevMcpServers`, `sessionMCPServers`,
  `builtinSessionMCPServers`, and `PrepareBuiltinProfile` copy headers.
- Exact-header recognition in `injectedKandevMCPConfigured`,
  `canonicalManagedMCPServers`, `adapterMCPServers`,
  `builtinPermissionAllowed`, and `builtinEndpointOnly`.
- The mock agent's ACP SSE client forwards session `headers`.
- Header redaction wherever agentctl logs MCP server entries.

## Out of scope

- Backend-side passthrough, Cursor, and project-file composition (Task 03).
- The mock-agent TUI client (Task 03).

## Acceptance

- ACP session setup and Codex app-server config contain the header for every
  built-in entry. This holds for new, load, and resume.
- Automatic approval and managed-conversation checks pass with the credentialed
  entry. They fail for the same URL with a different or extra header.
- The mock agent completes an `ask_user_question_kandev` round trip against an
  enforcing instance in a Go integration test.

## Verification

```bash
(cd apps/backend && go test -tags fts5 ./internal/agentctl/server/config/... ./internal/agentctl/server/api/... ./internal/agentctl/server/process/... ./internal/agentctl/server/adapter/transport/acp/... ./internal/agentctl/server/adapter/transport/codexappserver/... ./cmd/mock-agent/... -count=1)
```

## Files likely touched

- `apps/backend/internal/agentctl/server/config/builtin_mcp.go`
- `apps/backend/internal/agentctl/server/config/config.go`
- `apps/backend/internal/agentctl/server/api/agent.go`
- `apps/backend/internal/agentctl/server/api/builtin_mcp.go`
- `apps/backend/internal/agentctl/server/process/builtin_mcp_preparation.go`
- `apps/backend/internal/agentctl/server/process/managed_tool_policy.go`
- `apps/backend/internal/agentctl/server/process/manager_permission_policy.go`
- `apps/backend/internal/agentctl/server/process/builtin_mcp_permission.go`
- `apps/backend/internal/agentctl/server/process/passthrough.go`
- `apps/backend/internal/agentctl/server/process/project_mcp_files.go`
- `apps/backend/internal/agentctl/types/passthrough.go`
- `apps/backend/internal/agentctl/types/project_mcp_files.go`
- `apps/backend/cmd/mock-agent/mcp_client.go`, `main.go`
- Matching `_test.go` files listed in the plan's Tests table

## Dependencies

Task 01

## Risks

- One missed builder silently drops the header and disables automatic approval.
  Add a test that enumerates every builder output.
- `process/passthrough.go` and `project_mcp_files.go` hard-code `127.0.0.1`.
  Keep the URL unchanged. Only add the credential field.

## Parallelism

`sequential`

## Inputs

- System design: Built-in entry construction, Delivery paths, Policy recognition, Security.
- Inventory of builders and checks in the plan's Technical approach.

## Results

Done 2026-11-12. Targeted verification passed; see the plan's Verification results.
