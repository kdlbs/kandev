---
id: task-02-providerinject
title: providerinject package and Codex renderer
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

# Task 02: providerinject package and Codex renderer

## Summary

Create `internal/agent/providerinject/` with a pure `Build(spec, profile,
revealedKey) (Injection, error)` where `Injection = { CLIArgs []string; Env
map[string]string; ReservedKeys []string }`. Implement
`RenderCodexConfigOverrides` producing the `-c model_provider*` args and the
`OPENAI_API_KEY` env entry described in the system design. No I/O, no `*Manager`.

## Scope

- Package, `Injection` type, `Build`, and the Codex renderer.
- Verify (via an `acp-debug` capture against the pinned `codex-acp` bridge) that
  `-c key=value` overrides are forwarded to Codex; record the evidence in the
  work-order results. If not forwarded, implement the config-fragment fallback
  writing into a caller-provided isolated home path instead.
- Unit tests: arg shape, fixed non-reserved provider id, key redaction in any
  error, empty/relative base URL → error.

## Exclusions

- No call sites (task-03/04). No profile schema (task-01).

## Implementation acceptance conditions

1. `Build` with a valid Codex spec + `http://localhost:20128/v1` yields the
   exact documented `-c` args, `Env["OPENAI_API_KEY"]` set, and
   `ReservedKeys == ["OPENAI_API_KEY"]`.
2. `Build` with an empty or non-absolute base URL returns an error and no
   partial injection.
3. The provider id constant is asserted not to equal any Codex reserved
   built-in id.

## Verification

1. `cd apps/backend && go test ./internal/agent/providerinject/... -race`
2. `make -C apps/backend lint`

## Likely files

- `apps/backend/internal/agent/providerinject/*.go` (new)
- `apps/backend/internal/agent/agents/codex_acp.go` (spec constant, if refined)
- `apps/backend/internal/agent/agents/ACP_BRIDGE_VERSIONS.md` (evidence note)

## Risks

- Bridge `-c` forwarding is the key unknown; do the capture first.

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
