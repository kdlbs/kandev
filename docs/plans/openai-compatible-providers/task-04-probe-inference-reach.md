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

Carry `acpprovider.GatewayAuth` through `InferenceConfigDTO`
(`ProviderGatewayAuth *acpprovider.GatewayAuth`, reuse `Env`), populate it from
the resolved profile in the backend utility caller, and apply the same
`authenticate(gateway)` in `acp_executor.go` for both the inference and probe
subprocesses. Surface a sanitized upstream failure (e.g. provider `401`) instead
of "peer disconnected".

## Scope

- `InferenceConfigDTO.ProviderGatewayAuth` + backend population in
  `lifecycle/utility.go` (call `Manager.resolveProviderGatewayAuth` when the
  resolved profile is `openai_compatible`).
- `acp_executor.go`: advertise the gateway client capability and issue
  `authenticate(gateway)` after `initialize` in both `executeACPSession` and
  `probeACPSessionWithContext`; merge the provider key env in
  `sanitizeEnvForAgent` with reserved-key precedence.
- Agent-models probe path: the probe executor accepts `ProviderGatewayAuth` and
  applies the same `authenticate`. Provider profiles use free-text model entry,
  so no per-profile probe caller is wired (AC-003.1, revised).
- Error path: on `session/new` / `prompt` failure include a redacted stderr tail
  (key + tmp paths stripped) so an upstream status is legible.

## Exclusions

- No new provider fields (task-01). No live-session path (task-03).

## Implementation acceptance conditions

1. A profile-scoped utility prompt on an `openai_compatible` Codex profile
   reaches a stub OpenAI server with the bearer key from
   `authenticate(gateway)`.
2. The sessionless probe executor accepts `ProviderGatewayAuth` and issues the
   gateway `authenticate` when supplied (AC-003.1, revised: provider profiles
   use free-text model entry, so no per-profile probe caller is wired).
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
- `apps/backend/internal/agentctl/server/utility/acp_provider_gateway_test.go`
- sibling `*_test.go`

## Risks

- Redaction of the stderr tail must be conservative; prefer an allowlist of
  recognizable HTTP status lines over free-text passthrough.

## History

An earlier revision carried provider config as `InferenceConfigDTO.ProviderArgs
[]string` (CLI `-c` flags applied next to `CLIFlags`). The `acp-debug` capture
(2026-08-31, `codex-acp` 1.7.0) showed the bridge ignores CLI args and offers a
first-class `gateway` auth method, so the DTO now carries
`ProviderGatewayAuth *acpprovider.GatewayAuth` and the executor issues the same
`authenticate(gateway)` used by the live-session path. `providerinject` and
`ProviderArgs` never shipped.
