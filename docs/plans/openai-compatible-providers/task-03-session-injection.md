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

Wire `Manager.resolveProviderGatewayAuth` into the lifecycle session-launch
path: reveal the provider key, validate the base URL, adapt a loopback host to
the launching runtime, and build the `acpprovider.GatewayAuth` carried in
`CreateInstanceRequest.ProviderGatewayAuth`. The agentctl ACP adapter advertises
`clientCapabilities.auth._meta.gateway=true` and issues `authenticate(gateway)`
after `initialize`. Abort launch with a sanitized `PROVIDER_MISCONFIGURED` error
on reveal failure, bad base URL, or an unsupported agent. Also fix the two
`profile_env.go` behaviors.

## Scope

- Lifecycle: `resolveProviderGatewayAuth` call + `ProviderGatewayAuth` /
  `KeyEnvVar` / revealed-key threading into `CreateInstanceRequest`
  (`internal/agent/runtime/lifecycle`).
- agentctl ACP adapter: `ClientAuthMeta` advertisement in `initialize` and the
  `authenticate` gateway call in the session-init path
  (`internal/agentctl/server/adapter/transport/acp`), carried through the
  instance config.
- `mergeEnvFillMissing(dst, src, reserved []string)`: `reserved` keys overwrite
  `dst`; all others keep fill-missing. Only provider injection passes a
  non-empty `reserved` (the spec's `KeyEnvVar`).
- `resolveAgentProfileEnvVars`: single non-cancel reveal failure → warn + skip
  that entry + return partial map (not `ErrProfileSecretUnavailable` for the
  whole set). Cancellation errors still propagate.
- `PROVIDER_MISCONFIGURED` sanitized error type/string.

## Exclusions

- Probe/inference path (task-04). Frontend (task-05).

## Implementation acceptance conditions

1. A session on an `openai_compatible` Codex profile launches, the adapter sends
   `authenticate(gateway)` with the revealed key in `_meta.gateway.headers`, and
   the key is also exported to env; the injected key wins over an inherited
   `OPENAI_API_KEY`; a `native` Codex profile in the same process is unchanged.
2. Missing secret, empty/relative base URL, or an agent with no provider spec
   fails the launch with `PROVIDER_MISCONFIGURED` (no vendor fallback, no key in
   the message).
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

- Auth ordering: `authenticate(gateway)` must run after the `initialize`
  round-trip and before the first `session/new`; a failure aborts rather than
  falling back to the vendor endpoint. Assert the frame order in a test.
- Do not broaden the `mergeEnvFillMissing` signature change into the indexed-key
  (`gitconfigenv`) path.

## History

An earlier revision assumed codex-acp forwarded `-c model_provider*` CLI
overrides, so this task appended `Injection.CLIArgs` to the agent argv. The
`acp-debug` capture (2026-08-31, `codex-acp` 1.7.0) showed the bridge ignores CLI
args and offers a first-class `gateway` auth method, so the task now wires
`authenticate(gateway)` + the capability advertisement into the agentctl ACP
adapter and carries the `GatewayAuth` through the instance config. The two
`profile_env.go` credential fixes are unchanged.
