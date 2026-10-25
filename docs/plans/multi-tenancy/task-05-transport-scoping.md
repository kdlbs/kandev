---
id: "05-transport-scoping"
title: "Transport scoping: WS gateway and MCP"
status: todo
wave: 2
depends_on: ["04-service-layer-org-scoping"]
plan: "plan.md"
spec: "../../specs/multi-tenancy/spec.md"
---

# Task 05: Transport Scoping — WS Gateway and MCP

## Acceptance

- `Hub.BroadcastToOrg(orgID, …)` exists and replaces `Hub.Broadcast` for every
  tenant-derived event. The remaining `//ws:global` call sites are an explicit
  allowlist (release notification, feature-toggle change, restart) pinned by a
  test that fails when a new global broadcast is added without an entry.
- The WS dispatch backstop (`internal/gateway/websocket/dispatch_scope.go`)
  resolves the org for any payload carrying `task_id`, `session_id`,
  `task_environment_id`, or a top-level `task.<verb>` `id`, and refuses a
  foreign-org reference before dispatch.
- WS subscriptions are org-checked at subscribe time, not only at broadcast
  time.
- `internal/mcp/scope` resolves stream → task → workspace → org and attaches an
  identity carrying both owner and org. An unresolvable org is denied; the path
  never returns an identity-free context.
- A client authenticated to org B receives no frame produced by org A, proven
  by a two-client gateway test rather than by inspecting broadcast call sites.

## Verification

- `go test -race ./internal/gateway/websocket/... ./internal/mcp/...` from `apps/backend`
- `go test ./internal/gateway/websocket/... -run 'TestGlobalBroadcastAllowlist|TestCrossOrgNoFrame'`

## Files Likely Touched

- `apps/backend/internal/gateway/websocket/{hub,dispatch_scope,access}.go`
- `apps/backend/internal/mcp/scope/scope.go`
- Every `hub.Broadcast` call site

## Inputs

- Spec: API surface (WS changes), Scenarios (cross-org no frame).
- Patterns: the existing dispatch-backstop rules and their `user_shell.stop` /
  `task.state` regression history in `apps/backend/AGENTS.md` — a new action
  name silently opts out of the backstop, so the org check must key off the
  same ref extraction rather than inventing a parallel one.

## Output Contract

Report the global-broadcast allowlist and its justification per entry, the
two-client cross-org test, RED/GREEN commands, and set this task plus its plan
checkbox to done.
