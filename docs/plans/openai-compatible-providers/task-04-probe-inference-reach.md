---
id: task-04-probe-inference-reach
title: Probe and inference/utility provider reach
status: done
wave: 4
depends_on:
  - task-03-session-injection
plan: plan.md
requirements:
  - REQ-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-003
acceptance_criteria:
  - AC-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-003.1
  - AC-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-003.2
  - AC-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-003.3
system_design:
  - docs/specs/agents/system-design/openai-compatible-providers.md
---

# Task 04: Probe and inference/utility provider reach

## Summary

Carry provider injection through `InferenceConfigDTO` (`ProviderArgs []string`,
reuse `Env`), populate it from the resolved profile in the backend utility
caller, and apply it in `acp_executor.go` for both the inference and probe
subprocesses. Surface a sanitized upstream failure (e.g. provider `401`) instead
of "peer disconnected".

## Scope

- `InferenceConfigDTO.ProviderArgs` + backend population in
  `lifecycle/utility.go` (run `providerinject.Build` when the profile is
  `openai_compatible`).
- `acp_executor.go`: append `ProviderArgs` next to `CLIFlags`; merge provider
  env in `sanitizeEnvForAgent` with reserved-key precedence.
- Agent-models probe path: same builder via the profile context it receives.
- Error path: on `session/new` / `prompt` failure include a redacted stderr tail
  (key + tmp paths stripped) so an upstream status is legible.

## Exclusions

- No new provider fields (task-01). No live-session path (task-03).

## Implementation acceptance conditions

1. A profile-scoped utility prompt on an `openai_compatible` Codex profile
   reaches a stub OpenAI server with the injected bearer key.
2. The sessionless model probe for that profile context applies the same
   injection.
3. A stubbed provider `401` produces an error whose message contains a
   sanitized upstream indication, not only "peer disconnected", and no key.

## Verification

1. `cd apps/backend && go test ./internal/agentctl/server/utility/... -run 'Provider|Inference|Probe' -race`
2. `cd apps/backend && go test ./internal/agent/runtime/lifecycle/... -run 'Inference|Provider' -race`
3. `make -C apps/backend test`
4. `make -C apps/backend lint`

## Likely files

- `apps/backend/internal/agentctl/server/utility/types.go`
- `apps/backend/internal/agentctl/server/utility/acp_executor.go`
- `apps/backend/internal/agent/runtime/lifecycle/utility.go`
- `apps/backend/internal/agent/runtime/lifecycle/` agent-models probe caller
- sibling `*_test.go`

## Risks

- Redaction of the stderr tail must be conservative; prefer an allowlist of
  recognizable HTTP status lines over free-text passthrough.

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
