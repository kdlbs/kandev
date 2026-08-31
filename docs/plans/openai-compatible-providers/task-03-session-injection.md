---
id: task-03-session-injection
title: Live session injection and credential-delivery fixes
status: done
wave: 3
depends_on:
  - task-02-providerinject
plan: plan.md
requirements:
  - REQ-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-002
  - REQ-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-004
acceptance_criteria:
  - AC-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-002.3
  - AC-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-002.4
  - AC-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-002.5
  - AC-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-004.1
  - AC-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-004.2
  - AC-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-004.3
system_design:
  - docs/specs/agents/system-design/openai-compatible-providers.md
---

# Task 03: Live session injection and credential-delivery fixes

## Summary

Wire `providerinject.Build` into the lifecycle session-launch path: reveal the
provider key, append `Injection.CLIArgs` to the agent argv after MCP `-c` args
and before `CLIFlags`, and merge `Injection.Env` with reserved-key precedence.
Abort launch with a sanitized `PROVIDER_MISCONFIGURED` error on reveal failure
or bad base URL. Also fix the two `profile_env.go` behaviors.

## Scope

- Lifecycle: provider reveal + `Build` call + argv/env merge at the ACP session
  launch site (`internal/agent/runtime/lifecycle`, near where MCP `-c` args and
  `CLIFlags` are assembled and `CreateInstanceRequest` is built).
- `mergeEnvFillMissing(dst, src, reserved []string)`: `reserved` keys overwrite
  `dst`; all others keep fill-missing. Only provider injection passes a
  non-empty `reserved`.
- `resolveAgentProfileEnvVars`: single non-cancel reveal failure → warn + skip
  that entry + return partial map (not `ErrProfileSecretUnavailable` for the
  whole set). Cancellation errors still propagate.
- `PROVIDER_MISCONFIGURED` sanitized error type/string.

## Exclusions

- Probe/inference path (task-04). Frontend (task-05).

## Implementation acceptance conditions

1. A session on an `openai_compatible` Codex profile launches with the provider
   `-c` args in argv and the revealed key in env; the injected key wins over an
   inherited `OPENAI_API_KEY`; a `native` Codex profile in the same process is
   unchanged.
2. Missing secret or empty/relative base URL fails the launch with
   `PROVIDER_MISCONFIGURED` (no vendor fallback, no key in the message).
3. With two secret env entries where one reveal fails, the other is still
   delivered and a warn log names the failing key.

## Verification

1. `cd apps/backend && go test ./internal/agent/runtime/lifecycle/... -run 'ProfileEnv|Provider|MergeEnv|SecretUnavailable' -race`
2. `cd apps/backend && go test ./internal/agent/runtime/lifecycle/... -race`
3. `make -C apps/backend lint`
4. Changed-file lint: `cd apps/backend && golangci-lint run ./... --new-from-rev=<base-sha> --timeout=5m`

## Likely files

- `apps/backend/internal/agent/runtime/lifecycle/profile_env.go`
- `apps/backend/internal/agent/runtime/lifecycle/session.go` / `command.go` / the
  `CreateInstanceRequest` assembly site
- `apps/backend/internal/agent/runtime/lifecycle/*_test.go`

## Risks

- Argv ordering: provider `-c` must not land after a `--` that Codex treats as
  end-of-flags. Assert final argv in a test.
- Do not broaden the `mergeEnvFillMissing` signature change into the indexed-key
  (`gitconfigenv`) path.

## REDESIGN NOTE (2026-08-31)

Verified against `@agentclientprotocol/codex-acp` 1.7.0: the bridge ignores CLI
args, so `-c` overrides do not reach codex. codex-acp instead exposes a
first-class ACP **gateway provider**: advertise
`clientCapabilities.auth._meta.gateway=true` in `initialize`, then send
`authenticate({methodId:"gateway", _meta:{gateway:{baseUrl, headers:{Authorization:"Bearer <key>"}, providerName}}})`.
See the "Design update" block in the system design. This work order is re-scoped:
`providerinject.Build` returns ACP gateway `authenticate` params (not CLI args);
task-03 wires the `authenticate` call + capability into the agentctl ACP adapter
session-init path and carries base URL + revealed key through the instance
config; task-04 does the same for the utility/probe ACP executor. The two
`profile_env.go` credential fixes in task-03 still stand as written. The
already-merged task-02 pure package keeps its shape (`Injection` → gateway
params) and tests are updated accordingly.
