---
id: "07-mcp-connectivity"
title: "Permit only the current Kandev MCP route"
status: in_progress
wave: 7
depends_on:
  - "05-nono-provider-lifecycle"
plan: "plan.md"
requirements:
  - REQ-AGENTS-SANDBOX-005
acceptance_criteria:
  - AC-AGENTS-SANDBOX-005.1
  - AC-AGENTS-SANDBOX-005.2
  - AC-AGENTS-SANDBOX-005.3
  - AC-AGENTS-SANDBOX-005.4
system_design:
  - ../../specs/agents/system-design/sandbox-policies.md
---

# Task 07: Permit only the current Kandev MCP route

## Summary

Resolve dynamic runtime endpoint descriptors and expose a session-scoped MCP-only relay.

## In scope

Connect ACP and native MCP strategies through explicit policy, revoke stale endpoints, and test route/authentication/transport failures separately.

## Out of scope

No unrestricted backend port, allow-all egress, hard-coded paths, or approval-listener access.

## Acceptance

- A real sandboxed MCP tool round trip succeeds under an explicit logical-service grant.
- Settings APIs, other sessions, callback endpoints, unrelated loopback, redirects, DNS/direct-IP bypass, and revoked endpoints fail.
- Endpoint rotation preserves logical intent without broadening grants and reports unsupported hot changes.

## Verification

Commands run from the Kandev repository root unless a subshell selects the dedicated plugin checkout.
New test and runner paths below are implementation deliverables, not existing passing evidence.

```bash
(cd apps/backend && go test -trimpath -race ./internal/agent/sandbox/... ./internal/agent/mcpconfig ./internal/agent/runtime/lifecycle -run 'SandboxMCP|SandboxEndpoint|PassthroughMCP' -count=1)
python3 scripts/check-nono-real.py --mcp --record docs/plans/nono-sandbox-integration/acceptance.md
```

Use TDD for changed logic. Before the first pnpm command in this workspace, run `(cd apps && pnpm install --frozen-lockfile)`.
Run each listed suite after its files exist. Do not count a zero-test filter as a pass.

## Files likely touched

- `apps/backend/internal/agent/sandbox/endpoints.go (new)`
- `apps/backend/internal/agent/sandbox/endpoints_test.go (new)`
- `apps/backend/internal/agent/mcpconfig/passthrough.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_passthrough.go`
- `Dedicated Nono plugin: explicit route compiler`

## Dependencies

05-nono-provider-lifecycle

## Risks

Nono network rules must constrain the actual relay. Hostname-only permission does not prove path or session isolation.

## Parallelism

`sequential`

## Inputs

- [Evidence](investigation.md).
- [Design](../../specs/agents/system-design/sandbox-policies.md).

## Results

Implementation is authorized. Component checks are recorded below; full product acceptance is pending.


## Component evidence (2026-10-24)

The core relay accepts only its private route, bounds incoming bodies and concurrency, filters headers, rejects redirects, and cancels outstanding requests on close. Each HTTP request uses a fresh connection; this also avoids the pinned Nono forward proxy retaining an upstream connection after its first response.

The provider receives a logical `kandev.mcp` endpoint descriptor with the current loopback address and port. It receives no URL capability. An explicit logical grant is required. The core exact TCP relay enforces that endpoint. Ephemeral endpoint addresses do not become durable policy grants.

The real packaged Nono test now initializes MCP and calls `get_task_plan_kandev` through the core relay and the actual Kandev MCP server. The backend dispatcher is a scoped fixture. The same process cannot access the settings route, private profile/proc data, or approval listener, directly or through the proxy. This proves component connectivity, not a live task/agent journey.

The authenticated agentctl control route checks the retained process owner, attempt, generation, and policy digest. Configuration replaces the automatically injected MCP address with the captured relay, or removes it when no logical grant exists. Focused control-route and process tests pass. Backend task lifecycle wiring and the native terminal path remain unfinished.


### Native MCP artifacts

Generated native config now uses the explicitly granted child home. Core does not fall back to private host configuration when that grant is absent. Linux writes use directory-rooted operations, bounded regular-file reads, and atomic replacement. Focused regressions verify permitted writes, outside-path refusal, parent and leaf symlink refusal, malformed-merge refusal, and cleanup after directory replacement. Existing passthrough MCP tests and sandbox lifecycle tests pass under the race detector. These are transport checks, not actual Pi chat or terminal acceptance.


### Managed Pi configuration repair (2026-10-26)

Actual Pi ACP 0.0.34 discovery and sandboxed file work passed. The first same-session MCP call failed: the adapter stores ACP MCP settings without forwarding them, and Pi 1.3.0 skips project MCP configuration without project trust. This is an agent configuration defect, separate from Nono network enforcement.

The existing Pi strategy now emits the native `type` discriminator and writes sandbox MCP configuration into the captured isolated child home. Private `PI_CODING_AGENT_DIR` overrides are supported. Missing state, shared directory overrides, and unsupported native SSE fail closed. Ordinary launches retain project configuration. Core does not copy credentials or grant project trust. The focused regression failed before the repair. Protocol and native materialization checks, private path variants, and existing MCP artifact safety tests pass under the race detector. Actual managed-agent verification remains pending.
