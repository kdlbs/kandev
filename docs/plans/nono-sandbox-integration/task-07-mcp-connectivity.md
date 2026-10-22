---
id: "07-mcp-connectivity"
title: "Permit only the current Kandev MCP route"
status: pending
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

Pending. No production implementation is authorized during the design turn.
