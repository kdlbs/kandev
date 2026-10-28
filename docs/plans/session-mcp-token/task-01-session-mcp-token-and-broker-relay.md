---
id: "01-session-mcp-token-and-broker-relay"
title: "Authenticate agent MCP per instance and relay the Git broker, behind a flag"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-001
  - REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-002
  - REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-003
  - REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-004
acceptance_criteria:
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.1
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.2
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.3
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.4
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.5
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.6
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.7
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.8
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.9
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.10
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.11
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.12
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.13
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.14
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.15
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.1
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.2
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.3
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.4
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.5
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.6
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.7
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.8
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.9
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.10
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.11
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.12
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-003.1
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-003.2
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-004.1
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-004.2
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-004.3
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-004.4
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-004.5
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-004.6
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-004.7
system_design:
  - ../../specs/executors/system-design/session-control-plane-isolation.md
  - ../../specs/executors/system-design/session-control-plane-rollout.md
---

# Task 01: Session MCP token and Git broker relay

## Summary

Port the reviewed implementation onto `main` as logical commits, add the runtime flag and
its flag-off parity tests, and apply the three review fixes in the plan.

## In scope

- Backend token derivation, nonce persistence, session key store, create-request fields,
  adoption verifier, revoke route, typed launch failures, per-agent carriers.
- agentctl MCP guard, `/identity` additions, broker relay, header redaction, managed-policy change.
- `features.sessionMcpToken` registration, profile entries, backend gating object, frontend key.
- Mock agents (`cmd/mock-agent`, ACP and TUI) forwarding the `Authorization` header (AC-004.6) and recorded-request tests for both.
- Public documentation in `docs/public/executors.md`: codes, causes, flag, upgrade order.

## Out of scope

- Everything under the design's Out of scope list.
- Any sandbox-provider route; it re-applies when that work lands.

## Verification

```
(cd apps/backend && go test ./internal/agentctl/server/api ./internal/agentctl/server/instance ./internal/agentctl/server/config ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl ./internal/common/sessionmcp ./internal/backendapp ./internal/runtimeflags ./internal/common/config ./internal/profiles ./internal/agent/mcpconfig ./internal/secrets ./internal/agentctl/server/process ./internal/orchestrator/executor ./internal/agentctl/server/adapter/... ./cmd/mock-agent -count=1)
(cd apps/web && pnpm run typecheck && pnpm exec vitest run lib/state/slices/features)
bash scripts/lint-go-changed <changed-go-files>
python3 scripts/lint-spec-files.py --all
python3 scripts/list-docs.py validate
```

Pre-existing failures are proven against `origin/main` by a control run of the same
packages and a `comm -13` of the `--- FAIL` names.

## Results

- **Ported.** The reviewed implementation was brought over as a squash onto `main`, minus the sandbox-only
  private MCP route exemption and its `execution.SandboxSelection` test (both re-apply when the sandbox
  provider lands). The runtime flag `features.sessionMcpToken` is registered in `profiles.yaml` (off in prod,
  dev and e2e), the config, `runtimeflags/registry.go` and the web feature types. One `SessionMCPPolicy` gates
  all backend behaviour; agentctl holds no flag.
- **Review fixes.** `verifyRetainedSessionToken` fails closed when the record carries a nonce and no key store
  exists; the revoke retry waits on a timer in a `select` instead of `time.Sleep`; the identity decode error
  wraps with `%w: %w`. Review round 1 also fixed token-less instances answering 503 (they stay unguarded until
  revoked), scoped the relay control-auth exemption to instances with an upstream broker URL, and made a
  reconnected Kubernetes pod count as retained.
- **Receipts.** Tests in agentctl api, instance and config, lifecycle, agent runtime agentctl, sessionmcp,
  backendapp, runtimeflags and the mock agent pass, with flag-off parity tests and `-race` on the new tests.
  Remaining failures in those packages fail identically on `origin/main` (macOS `/var` symlink and socket
  path limits). `scripts/lint-go-changed`, `lint-spec-files.py --all` and `list-docs.py validate` pass. One
  Playwright spec covers the Feature Toggles row.
- **Manual check.** A real `agentctl` binary with an enforcing and a token-less instance: no or wrong token on
  `/mcp` and `/message` of the enforcing instance answered 401 and the right token reached the handler; the
  token-less instance was reachable as today; after the revoke route both answered 503.
- **Known limitation.** Recovered Sprites and plugin-runtime instances expose no conditional-delete or revoke
  handle, so a retained instance there ends in a fail-closed unknown outcome. A control handle for those
  runtimes is follow-up work.
