---
created: 2026-10-26
status: done
requirements:
  - REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-001
  - REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-002
  - REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-003
  - REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-004
system_design:
  - ../../specs/executors/system-design/session-control-plane-isolation.md
  - ../../specs/executors/system-design/session-control-plane-rollout.md
legacy_specs: []
---

# Implementation Plan: Per-session MCP token and Git broker relay

## Overview

Each agentctl instance authenticates its MCP routes with a per-instance token, and
same-host agents reach the Git credential broker through their own instance. The
change ships behind `features.sessionMcpToken`, off in `prod`.

The work lands as a port of a reviewed implementation: branch `feature/session-mcp-token-sandbox` on the `fork` remote (23 commits over `sandbox/nono-plus-main`). It is brought onto `main` as a squash of `git diff fork/sandbox/nono-plus-main fork/feature/session-mcp-token-sandbox`, with the sandbox-only parts dropped. `verifyRetainedSessionToken`, `remote_recovery_token.go`, `agentctl/session_mcp.go` and `internal/common/sessionmcp` do not exist on `main`; the port creates them. The policy object lives in `lifecycle` (design Rollout).

## Port rules

- Base is `origin/main`. Nothing here imports or references sandbox-provider code
  (`internal/agent/sandbox`, `internal/common/sandbox`, `internal/common/sandboxmcp`,
  `backendapp/sandbox_*`). A line that only makes sense with a sandbox provider is dropped
  and recorded below as re-applying when that work lands: the private sandbox MCP route
  exemption from the token, and the test that sets `execution.SandboxSelection`.
- Specs and plan documents of the stacked change are not carried over; this directory
  and `docs/specs/executors/{requirements,system-design}/session-control-plane-isolation.md`
  replace them.

## Review fixes applied while porting

1. `verifyRetainedSessionToken` (lifecycle `remote_recovery_token.go`) fails closed with
   `errRetainedTokenUnverified` when the key store is unavailable and the persisted record
   carries `mcp_token_nonce`; this is the unknown outcome of AC-001.11 (revoke, `RecoveryOutcomeUnknown`, never adopted), and `errRetainedTokenUnverified` is the error for it, not a fourth outcome. A record without a nonce is a mismatch whatever the store state. The "skip" log applies only when the policy object says the flag is off. Tests: record with nonce and no store; record without nonce and no store.
2. `revokeUnverified` retries with a timer in a `select` on `ctx.Done()`, not `time.Sleep`.
3. `agentctl/session_mcp.go` wraps the malformed-identity error with `%w: %w`.

## Rollout

Flag registered in `runtimeflags/registry.go` with the matching profile, config,
`runtimeflags/config.go` and frontend `features/types.ts` entries (REQ-004). Flag-off
parity tests and the compatibility matrix tests are part of the change.

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| 001.1 to 001.5, 001.12 | `agentctl/server/instance` guard tests; `agentctl/server/api` control and sibling-token tests. |
| 001.6 to 001.8, 001.14 | `agent/runtime/lifecycle` carrier tests per transport class; `agentctl/server/process` policy tests; ACP log redaction test. |
| 001.9, 001.10 | `internal/secrets` session key tests; lifecycle nonce and 409 retry tests. |
| 001.11, 001.13, 001.15 | Lifecycle verifier and rollback tests per executor type; review fix tests. |
| 002.1 to 002.12 | `agentctl/server/api` relay tests; `backendapp` and lifecycle broker tests. |
| 003.1, 003.2 | Config default unchanged; design records the decision. |
| 004.1 to 004.7 | Registry and profile contract tests; flag-off golden parity tests; compatibility matrix rows; public docs. |

## E2E tests

No new Playwright spec is required. The Feature Toggles settings row is a rendered surface, covered by the existing registry contract tests. The flag is off in every profile (AC-004.1), so existing e2e specs are unchanged. The mock agents forward the configured header (AC-004.6) so a spec that sets `KANDEV_FEATURES_SESSION_MCP_TOKEN=true` can launch them; Build adds such a spec only if it needs one.

## Work orders

- [task-01-session-mcp-token-and-broker-relay.md](task-01-session-mcp-token-and-broker-relay.md)
