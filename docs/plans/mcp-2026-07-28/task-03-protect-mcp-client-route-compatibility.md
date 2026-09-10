---
id: "03-protect-mcp-client-route-compatibility"
title: "Protect MCP client and route compatibility"
status: done
wave: 2
depends_on: ["01-serve-dual-era-mcp"]
plan: "plan.md"
spec: "../../specs/agents/requirements/mcp-protocol-compatibility.md"
---

# Task 03: Protect MCP Client and Route Compatibility

## Inputs

- `AC-AGENTS-MCP-PROTOCOL-001.5` through `.7`.
- `docs/specs/agents/system-design/mcp-protocol-compatibility.md` sections
  `Routes and profiles`, `Application and security boundaries`, and
  `Compatibility boundaries`.
- Existing route and client code in agentctl, backendapp, lifecycle, Jira, and
  the mock agent.

## Change

1. Write route tests that prove agentctl still exposes `/mcp`, `/sse`, and
   `/message`, and the external backend still exposes its three existing MCP
   routes.
2. Extend external endpoint integration tests with modern stateless requests.
   Prove that every request resolves bearer-token identity and tool
   permissions without protocol-session state.
3. Keep agent and passthrough injection pointed at the existing HTTP `/mcp`
   URL with SSE fallback. Add regression assertions where current tests do not
   cover that ordering.
4. Keep the Jira client on its explicit `2025-06-18` request. Adapt it only as
   required by the SDK compile-time API.
5. Keep the mock agent on legacy SSE. Adapt it only as required by the SDK
   compile-time API.
6. Prove that configured third-party server definitions remain unchanged when
   Kandev passes them to an agent.

## Acceptance

- Existing HTTP and SSE route paths do not change.
- An authenticated external modern client can discover, list, and call tools.
  Each stateless request enforces the same caller permissions.
- Agentctl and passthrough configurations still prefer `/mcp` and retain SSE
  compatibility where the client supports it.
- The Jira client still requests `2025-06-18`.
- The mock agent still uses SSE.
- Kandev does not add protocol metadata to a third-party server definition.

## Verification

```bash
cd apps/backend
go test -race ./internal/agentctl/server/api ./internal/backendapp ./internal/agent/runtime/lifecycle ./internal/jira ./cmd/mock-agent
git diff --check -- internal/agentctl/server/api internal/backendapp internal/agent/runtime/lifecycle internal/jira cmd/mock-agent
```

## Files likely touched

- `apps/backend/internal/agentctl/server/api/agent_test.go`
- `apps/backend/internal/backendapp/helpers_test.go`
- External MCP route or integration tests under
  `apps/backend/internal/backendapp/`.
- `apps/backend/internal/agent/runtime/lifecycle/manager_passthrough_test.go`
- `apps/backend/internal/jira/mcp_client.go`
- `apps/backend/internal/jira/mcp_client_test.go`
- `apps/backend/cmd/mock-agent/mcp_client.go`
- `apps/backend/cmd/mock-agent/mock_agent_test.go`

## Dependencies

Task 01. These tests run against its dual-era server and SDK API.

## Parallelism

Parallel-safe with Task 02. This task does not own evidence types or MCP server
hook tests.

## Output contract

Report the routes, auth cases, client pins, injection cases, files changed,
commands run, failures, and compatibility risks. Update this work order and
`plan.md` with the result.

## Result

- Added route assertions for agentctl `/mcp`, `/sse`, and `/message`, and external `/mcp`, `/mcp/sse`, and `/mcp/message`.
- Added an authenticated external modern-client test for discovery, tools/list, and tools/call. It sends a PAT on every request and checks the same identity at the route and dispatcher.
- Preserved existing HTTP-first `/mcp` injection, SSE fallback, and third-party MCP definitions. Existing injection and passthrough tests cover these paths.
- Kept Jira at `2025-06-18` and pinned the mock agent's SSE initialize request to legacy `2024-11-05` after the SDK latest version changed to modern.
- Verification passed: `go test -race ./internal/agentctl/server/api ./internal/backendapp ./internal/agent/runtime/lifecycle ./internal/jira ./cmd/mock-agent` with 3,632 tests, plus the scoped diff check.
