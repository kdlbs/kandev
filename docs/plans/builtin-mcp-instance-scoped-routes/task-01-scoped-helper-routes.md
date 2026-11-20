---
id: "01-scoped-helper-routes"
title: "Add compatible scoped helper routes"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-MCP-INSTANCE-001
  - REQ-AGENTS-MCP-INSTANCE-002
  - REQ-AGENTS-MCP-INSTANCE-003
  - REQ-AGENTS-MCP-INSTANCE-004
acceptance_criteria:
  - AC-AGENTS-MCP-INSTANCE-001.1
  - AC-AGENTS-MCP-INSTANCE-001.2
  - AC-AGENTS-MCP-INSTANCE-001.3
  - AC-AGENTS-MCP-INSTANCE-001.4
  - AC-AGENTS-MCP-INSTANCE-001.5
  - AC-AGENTS-MCP-INSTANCE-002.1
  - AC-AGENTS-MCP-INSTANCE-002.3
  - AC-AGENTS-MCP-INSTANCE-002.4
  - AC-AGENTS-MCP-INSTANCE-003.2
  - AC-AGENTS-MCP-INSTANCE-003.3
  - AC-AGENTS-MCP-INSTANCE-003.4
  - AC-AGENTS-MCP-INSTANCE-004.4
  - AC-AGENTS-MCP-INSTANCE-004.5
system_design:
  - ../../specs/agents/system-design/builtin-mcp-instance-scoped-routes.md
---

# Task 01: Add compatible scoped helper routes

## Summary

Make an explicit scoped create work through all helper-owned native MCP paths.
Omitted opt-in retains the existing bare service, so current backend callers
continue to work while lifecycle migration proceeds.

## In scope

- Add `scoped_mcp_routes` request/config/create-response/`InstanceInfo` fields
  on the helper and runtime client sides, including `Instance.Info()` serialization.
- Add the runtime client's effective-mode reader. Test missing/false/true fields
  and malformed responses. Retained adoption uses only `InstanceInfo`.
- Create the neutral `internal/common/mcpurl` builder and path/ID validation.
  Preserve host, wildcard, IPv6, zone, and legacy empty-host behavior.
- Validate scoped IDs before listener allocation and direct config injection.
  Keep empty-ID direct construction only for legacy mode.
- Group all core/package routes. Reject invalid native targets before auth with
  a small 404 body and a per-server bounded logger.
- Apply the prefix to initial and rebound HTTP/SSE transports and advertised
  message endpoints. Keep the external server's route contract intact.
- Migrate helper-owned ACP injection, native auto-approval/managed policy,
  passthrough snapshot, and project-file target/read-back URLs together.
- Add behavioral tests listed under this order in the plan. Run TDD here,
  including actual transport initialization, actual approval, and no-effect assertions.

## Out of scope

Backend default activation, lifecycle client reconstruction, provider artifact
writers, browser tests, and publication. Do not add a flag or credential.
Do not change any package grant or question/title/completion policy.

## Acceptance

1. Explicit scoped instances pass core/package HTTP and SSE handshakes. Bare
   and foreign native targets return 404 before dispatch, independent of auth.
2. Helper-generated endpoints and actual approval decisions match the same mode.
   Negotiation handles old/new peer shapes. Legacy, package, and control cases pass.
3. Port reuse by another task rejects the old prefix. Invalid scoped IDs leave
   no listener/lease. SSE clients can POST to the advertised scoped URL.

## Verification

Run from the repository root. Run the relevant new cases first for RED,
then run this block for GREEN. The first package is new in this order.

```bash
(cd apps/backend && go test -trimpath ./internal/common/mcpurl ./internal/agentctl/server/config ./internal/agentctl/server/instance ./internal/agentctl/server/api ./internal/agentctl/server/process ./internal/mcp/server ./internal/agent/runtime/agentctl -count=1)
git ls-files --modified --others --exclude-standard -z -- 'apps/backend/**/*.go' | xargs -0 -r bash scripts/lint-go-changed
git diff --check
```

The lint command includes modified and new files before staging. Do not stage
these changes without user authorization. Record exact test counts/results,
including the initial failing behavior, in Results.

## Files likely touched

- New `apps/backend/internal/common/mcpurl/{builtin.go,builtin_test.go}`.
- `apps/backend/cmd/agentctl/main.go`.
- `apps/backend/internal/agentctl/server/config/{config.go,mcp_url.go,builtin_mcp.go}`.
- `apps/backend/internal/agentctl/server/instance/{instance.go,manager.go}` and the
  existing override builder in that package.
- `apps/backend/internal/agentctl/server/api/{server.go,builtin_mcp.go,agent.go}`
  and a new `builtin_mcp_instance.go` for the route guard.
- `apps/backend/internal/mcp/server/{server.go,builtin_packages.go,builtin_preparation.go}`.
- `apps/backend/internal/agentctl/server/process/{managed_tool_policy.go,manager_permission_policy.go,passthrough.go,project_mcp_files.go}`.
- New `builtin_mcp_instance_test.go` siblings in config, instance, api, process,
  and MCP server. Existing package/permission/host tests affected by signatures.
- `apps/backend/internal/agent/runtime/agentctl/{control.go,client.go}` and
  new `builtin_mcp_instance_test.go` for negotiation/decoding.

Keep tests below the Go file-size limit. The common package owns the single
native URL implementation. Existing wrappers can only delegate to it.

## Dependencies

None. This order does not enable scoped mode for backend creates.

## Risks

- A Gin group alone leaves SSE's advertised message endpoint unscoped.
- Auth can turn an intended 404 into a 401 unless the rejection guard runs first.
- Package rebinding can rebuild transports without the prefix.
- A URL-only approval assertion can miss a broken allow-option selection.
- Live listener/SSE tests need bounded contexts and owned cleanup.

## Parallelism

`sequential`

## Inputs

- Design: Per-instance negotiation, Shared URL contract, Instance server and
  transports, Delivery and exact-URL policy, Authority and observability.
- Existing `builtin_mcp_session_test.go`, `builtin_packages_test.go`,
  `builtin_mcp_permission_test.go`, `manager_permission_policy_test.go`, and
  `managed_tool_policy_test.go`.
- Scoped backend/agentctl/API `AGENTS.md` and `/tdd` backend testing guidance.

## Results

Completed. Added a shared URL/path builder, negotiated helper route state,
validated scoped IDs before port allocation, and applied the effective mode to
helper routes, core/package transports, ACP injection, approval checks,
passthrough snapshots, and project-file targets. Existing external and legacy
routes remain unchanged.

RED evidence: the new common URL tests failed against a temporary empty builder
with URL and invalid-identity assertions. GREEN evidence: the focused scoped
route and delivery regression cases passed, including SSE advertisement and
POST, package transport rebinding, no-dispatch rejection, exact approval, and
old-peer response decoding.

Verification passed on 2026-11-16:

- `(cd apps/backend && go test -trimpath ./internal/common/mcpurl ./internal/agentctl/server/config ./internal/agentctl/server/instance ./internal/agentctl/server/api ./internal/agentctl/server/process ./internal/mcp/server ./internal/agent/runtime/agentctl -count=1)`
- `git ls-files --modified --others --exclude-standard -z -- 'apps/backend/**/*.go' | xargs -0 -r bash scripts/lint-go-changed` (0 issues)
- `git diff --check`
