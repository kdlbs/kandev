---
id: "01-agentctl-credential-enforcement"
title: "agentctl credential and enforcement"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-BUILTIN-MCP-CALLER-AUTH-001
  - REQ-AGENTS-BUILTIN-MCP-CALLER-AUTH-003
acceptance_criteria:
  - AC-AGENTS-BUILTIN-MCP-CALLER-AUTH-001.1
  - AC-AGENTS-BUILTIN-MCP-CALLER-AUTH-001.2
  - AC-AGENTS-BUILTIN-MCP-CALLER-AUTH-001.4
  - AC-AGENTS-BUILTIN-MCP-CALLER-AUTH-001.5
  - AC-AGENTS-BUILTIN-MCP-CALLER-AUTH-001.6
  - AC-AGENTS-BUILTIN-MCP-CALLER-AUTH-003.2
  - AC-AGENTS-BUILTIN-MCP-CALLER-AUTH-003.3
  - AC-AGENTS-BUILTIN-MCP-CALLER-AUTH-003.5
system_design:
  - ../../specs/agents/system-design/builtin-mcp-caller-auth.md
---

# Task 01: agentctl credential and enforcement

## Summary

Each agentctl instance issues a random MCP caller credential when it is created
and returns it through authenticated control responses. When the create request
asks for enforcement, the built-in MCP routes reject requests without that
credential.

## In scope

- `CreateRequest.BuiltinMCPAuthEnforced` and the control-authenticated instance
  read `GET /api/v1/mcp/builtin-credential`. Create, get, and list responses
  never carry the credential.
- Credential generation in `instance.Manager` and storage on
  `config.InstanceConfig`.
- The `builtinMCPCallerAuth` middleware on the core and package `/mcp`, `/sse`,
  and `/message` routes, with `/health` open.
- Rate-limited rejection logs without the credential value.

## Out of scope

- Adding headers to built-in entries (Task 02).
- Backend toggle and delivery (Task 03).

## Acceptance

- With enforcement on, requests to every built-in MCP route get 401 when they
  have no credential, the control credential, or another instance's
  credential. This includes a previous instance on the same port. With the own
  credential, `initialize`, `tools/list`, and an SSE session work.
- With enforcement off, behavior equals today's, and the credential is still
  issued.
- The credential is absent from list responses and captured logs.

## Verification

```bash
(cd apps/backend && go test -tags fts5 ./internal/agentctl/server/api/... ./internal/agentctl/server/instance/... -run 'BuiltinMCPCallerAuth|MCPCredential|InstanceAuth' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/agentctl/server/api/... ./internal/agentctl/server/instance/... -count=1)
```

## Files likely touched

- `apps/backend/internal/agentctl/server/instance/instance.go`
- `apps/backend/internal/agentctl/server/instance/manager.go`
- `apps/backend/internal/agentctl/server/config/config.go`
- `apps/backend/internal/agentctl/server/api/builtin_mcp.go`
- `apps/backend/internal/agentctl/server/api/server.go`
- `apps/backend/internal/agentctl/server/api/control_server.go`
- New `apps/backend/internal/agentctl/server/api/builtin_mcp_auth.go` and `builtin_mcp_auth_test.go`
- New `apps/backend/internal/agentctl/server/instance/manager_mcp_credential_test.go`

## Dependencies

None

## Risks

- Route matching must cover every package suffix in
  `mcpprofile.BuiltinPackageCatalog()`. Derive it from the same source as
  `builtinMCPExemptPaths`, not a second list.
- The SSE stream is long-lived. Authenticate on connect only, and do not tie it
  to control-credential rotation.

## Parallelism

`sequential`

## Inputs

- System design: Credential lifecycle, Endpoint enforcement, Observability.
- Existing patterns: `instanceAuth` in `api/auth.go`, `tokenEqual`.

## Results

Done 2026-11-12. Targeted verification passed; see the plan's Verification results.
