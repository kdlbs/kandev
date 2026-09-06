---
id: "10-per-org-agent-credentials"
title: "Per-org agent credential home"
status: todo
wave: 4
depends_on: ["09-per-org-filesystem-roots"]
plan: "plan.md"
spec: "../../specs/multi-tenancy/spec.md"
---

# Task 10: Per-Org Agent Credential Home

## Acceptance

- Every ACP subprocess and every agentctl child process launched for an org
  receives `HOME`, `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, and `XDG_CACHE_HOME`
  pointed at that org's credential home.
- The redirection is applied at the runtime-environment seam that covers all
  entry points — agent sessions, host-utility probes, sessionless prompts, and
  utility agents — not per call site. Coverage is proven at the producer
  boundary, not only at one consumer.
- `gh auth`, `claude login`, and provider API-key logins performed inside org A
  are invisible to org B. A test asserts an org-B agent reports no credential
  when only org A has authenticated.
- Org-scoped secrets and integration credentials are injected from the org's
  secret store; instance templates contribute env-var names only.
- With the flag off, the environment of every subprocess is byte-identical to
  today.

## Verification

- `go test ./internal/agent/runtime/... ./internal/agentctl/server/process/...`
- `go test ./internal/... -run 'TestOrgCredentialHome|TestCrossOrgAgentCredential'`

## Files Likely Touched

- `apps/backend/internal/agent/runtime/lifecycle/{process_runner,profile_resolver}.go`
- `apps/backend/internal/agentctl/server/process/`
- `apps/backend/internal/agent/credentials/`
- `apps/backend/internal/secrets/`, `internal/scriptengine/`

## Inputs

- Spec: What (agent credentials), Scenarios (`gh auth status` in the wrong org).
- Patterns: the runtime-environment invariant in `apps/backend/AGENTS.md` —
  `Agent.Runtime().Env` applies to every ACP subprocess entry point, and new
  overrides must be routed through host-utility probes and sessionless prompts
  into agentctl child processes before sanitization.

## Output Contract

Enumerate the subprocess entry points covered and how the producer-boundary
assertion proves none was missed. Report the flag-off byte-identity evidence,
RED/GREEN commands, and set this task plus its plan checkbox to done.
