---
id: task-01-provider-primitive
title: Provider primitive on the agent profile
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-001
acceptance_criteria:
  - AC-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-001.1
  - AC-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-001.2
  - AC-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-001.3
  - AC-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-001.4
  - AC-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-001.5
system_design:
  - docs/specs/agents/system-design/openai-compatible-providers.md
---

# Task 01: Provider primitive on the agent profile

## Summary

Add the three provider fields to `AgentProfile` (JSON blob, no column
migration), a per-agent `OpenAICompatibleProvider()` capability method with a
non-nil spec for `CodexACP`, shared create/update/duplicate validation, and the
API projection (`provider_kind`, `provider_base_url`, `provider_api_key_secret_id`
id-only, read-only `provider_supported`).

## Scope

- `AgentProfile` struct fields + settings repo JSON encode/decode.
- `OpenAICompatibleProviderSpec` type + interface method in
  `internal/agent/agents`; `CodexACP` returns it, all others return nil.
- Validation helper: `openai_compatible` requires `provider_supported`, absolute
  http(s) `provider_base_url`, and a `Model` without `/`; other kinds clear the
  fields on write.
- DTO + `ToAPI` in `pkg/api/v1`.

## Exclusions

- No injection logic (task-02/03). No frontend (task-05).

## Implementation acceptance conditions

1. A profile round-trips the three fields through create, read, update, and
   duplicate; the API projection never contains the key value.
2. Saving `openai_compatible` with a missing/relative base URL, a slash model,
   or an unsupported agent is rejected with a specific error; saving `native`
   clears the other two fields.
3. `CodexACP.OpenAICompatibleProvider()` returns a spec with a non-reserved
   `ProviderID`; a sample of other agents return nil.

## Verification

1. `cd apps/backend && go test ./internal/agent/settings/... ./internal/agent/agents/... -run 'Provider|OpenAICompat' -race`
2. `cd apps/backend && go test ./pkg/api/v1/... -run Profile -race`
3. `make -C apps/backend lint`

## Likely files

- `apps/backend/internal/agent/settings/models/models.go`
- `apps/backend/internal/agent/settings/` (service validation, repo scan/write)
- `apps/backend/internal/agent/agents/agent.go`, `agents/codex_acp.go`
- `apps/backend/pkg/api/v1/` agent-profile DTO + `ToAPI`

## Risks

- `Model`-with-slash validation must not break native profiles that legitimately
  use slash model ids — gate the check on `provider_kind == openai_compatible`.
