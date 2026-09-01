---
id: task-02-providerinject
title: acpprovider gateway-auth package
status: done
wave: 2
depends_on:
  - task-01-provider-primitive
plan: plan.md
requirements:
  - REQ-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-002
acceptance_criteria:
  - AC-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-002.1
  - AC-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-002.2
system_design:
  - docs/specs/agents/system-design/openai-compatible-providers.md
---

# Task 02: acpprovider gateway-auth package

## Summary

Create `internal/common/acpprovider/` with the pure gateway-auth primitive:
`BuildGatewayAuth(methodID, providerName, baseURL, apiKey) GatewayAuth` (returns
`{ MethodID, Meta }` where `Meta` is the ACP `_meta.gateway` payload) plus
`ValidateBaseURL` / `ValidateCredentialedBaseURL` shared by save-time and
launch-time validation. Tier-neutral: neither the backend nor agentctl imports
the other. No I/O, no `*Manager`.

## Scope

- Package, `GatewayAuth` type, `BuildGatewayAuth`, `ClientAuthMeta`, and the URL
  validators.
- Evidence capture (`acp-debug` against the pinned `codex-acp` bridge) confirming
  the bridge ignores CLI args and exposes the `gateway` auth method; record it in
  the work-order results / `ACP_BRIDGE_VERSIONS.md`.
- Unit tests: `Meta` shape with and without a key, provider-name omission,
  empty/relative/`ftp` base URL → error, credentialed cleartext non-loopback →
  error.

## Exclusions

- No call sites (task-03/04). No profile schema (task-01).

## Implementation acceptance conditions

1. `BuildGatewayAuth("gateway", "Kandev", "http://localhost:20128/v1", key)`
   yields `MethodID == "gateway"` and
   `Meta["gateway"] == {baseUrl, providerName, headers:{Authorization:"Bearer <key>"}}`;
   with an empty key `headers` is omitted.
2. `ValidateBaseURL` rejects empty / non-absolute / non-http(s) input;
   `ValidateCredentialedBaseURL` additionally rejects cleartext `http` to a
   non-loopback host.
3. `ProviderName` ("Kandev") is asserted not to equal any Codex reserved
   built-in provider id.

## Verification

1. `cd apps/backend && go test ./internal/common/acpprovider/... -race`
2. `make -C apps/backend lint`

## Likely files

- `apps/backend/internal/common/acpprovider/*.go` (new)
- `apps/backend/internal/agent/agents/openai_compatible_provider.go` (spec type)
- `apps/backend/internal/agent/agents/codex_acp.go` (spec constants)
- `apps/backend/internal/agent/agents/ACP_BRIDGE_VERSIONS.md` (evidence note)

## Risks

- Bridge behavior is the key unknown; the `acp-debug` capture settles both the
  CLI-arg question and the gateway `authenticate` shape before the type is locked.

## History

An earlier revision of this plan assumed codex-acp forwarded `-c model_provider*`
CLI overrides and shaped `Build` around `{ CLIArgs, Env, ReservedKeys }`. The
`acp-debug` capture (2026-08-31, `codex-acp` 1.7.0) showed the bridge ignores CLI
args and offers a first-class `gateway` auth method, so the package was built
around `BuildGatewayAuth` instead. task-03/04 wire the `authenticate` call and
capability advertisement; the `profile_env.go` credential fixes are unaffected.
