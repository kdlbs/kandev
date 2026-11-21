---
id: "01-runtime-semantics"
title: "Separate thinking intent from permission mode"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-SESSION-SETTING-SEMANTICS-001
acceptance_criteria:
  - AC-AGENTS-SESSION-SETTING-SEMANTICS-001.1
  - AC-AGENTS-SESSION-SETTING-SEMANTICS-001.2
  - AC-AGENTS-SESSION-SETTING-SEMANTICS-001.3
  - AC-AGENTS-SESSION-SETTING-SEMANTICS-001.4
  - AC-AGENTS-SESSION-SETTING-SEMANTICS-001.5
  - AC-AGENTS-SESSION-SETTING-SEMANTICS-001.6
  - AC-AGENTS-SESSION-SETTING-SEMANTICS-001.7
  - AC-AGENTS-SESSION-SETTING-SEMANTICS-001.8
system_design:
  - ../../specs/agents/system-design/session-setting-semantics.md
---

# Task 01: Runtime semantics and compatibility

## Summary

Normalize Pi thinking aliases and apply one winning thinking value after the
selected model settles. Preserve real permission checks and existing saved intent.

## In scope

- Shared semantic classification, probe/runtime projection and post-model catalogs.
- Canonical config selection, legacy compatibility, source precedence, and strict
  thinking-specific startup evidence through shared API/types.
- Profile PATCH clear/canonicalization, duplicate/read compatibility, ordinary
  resume/reset and provider-restored exception. Keep unrelated fields unchanged.
- Deterministic regression tests at real transport, lifecycle, and API boundaries.

## Out of scope

Rendered controls, upstream extension fixes, SQL migration, and new reset policy.

## Acceptance

1. Pi medium on off-only Argon fails as thinking-unavailable/mismatch with no
   prompt. Valid/default startup retains exact model and sends one prompt.
2. Model changes replace thinking choices; stale updates cannot restore them.
   Saved aliases resolve once with source precedence and explicit clears persist.
3. Existing permission refusals, unconfirmed requests, cancellation and session
   isolation remain unchanged. Errors retain bounded and correlated evidence.

## Verification

Run from repository root, using red-green-refactor. New test files below are
planned outputs and must exist before their commands are treated as evidence.

```bash
(cd apps/backend && go test -trimpath -race ./internal/agentctl/acpcompat ./internal/agentctl/server/adapter/transport/acp -run 'Test(SessionSettingSemantics|PiThinking|SetMode)' -count=1)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle ./internal/agent/settings/profileconfig ./internal/agent/settings/controller ./internal/agent/settings/handlers ./internal/task/models -run 'Test.*(PiThinking|Bootstrap|Mode|ConfigOption)' -count=1)
```

Run `bash scripts/lint-go-changed` with every added/changed Go file as explicit
arguments after implementation. Record the exact expanded command and result.

## Files likely touched

- `apps/backend/internal/agentctl/acpcompat/session_setting_semantics.go` and tests (new).
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session.go`,
  `adapter_session_mode.go`, `adapter_updates.go`, `adapter_mode_state.go`, and new Pi tests.
- `apps/backend/internal/agentctl/server/utility/` selected-model probe projection.
- `apps/backend/internal/agentctl/types/streams/`, agent settings DTOs, and `apps/packages/types/src/`.
- `apps/backend/internal/agent/runtime/lifecycle/session.go`, `bootstrap_failure.go`, and new Pi tests.
- `apps/backend/internal/agent/settings/profileconfig/`, `controller/`, `handlers/` and their save/duplicate tests.
- `apps/backend/internal/task/models/launch_errors.go` and cause normalization tests.

## Dependencies

None. Task 02 consumes the resulting optional metadata and safe cause codes.

## Risks

Do not add a broad mode bypass. Do not assume missing metadata means no override.
Do not infer thinking from a model name or use global probe results as live state.

## Parallelism

`sequential`

## Inputs

[Requirements](../../specs/agents/requirements/session-setting-semantics.md),
[design](../../specs/agents/system-design/session-setting-semantics.md), and plan evidence.
Use existing `adapter_mode_set_test.go`, `adapter_mode_satisfied_test.go`,
`session_mode_source_test.go`, and `session_test.go` as test-harness precedents.

## Results

Completed on 2026-11-19.

- Reused trusted agent identity and existing configuration category/ID instead of adding redundant transport fields. See the updated system design.
- Added strict Pi thinking normalization, source precedence, current-model capability validation, profile PATCH canonicalization/clear, reset compatibility, and provider-restored bypass.
- Added bounded thinking causes and persisted-error decoding. A regression caught and fixed loss of setting values/source through `LoadLastAgentError`.
- Red tests reproduced stale mode lists, ambiguous aliases, empty-snapshot publication, clamping, permission-labelled thinking failures, duplicate application, provider-restored replay, reset precedence, profile clearing, and persisted evidence loss. These tests pass after the fix.
- Focused lifecycle/controller/profile/model tests passed with `-race -tags fts5`. The full ACP adapter suite passed with `-race` (35.254 s).
- Shared classification, selected-model probe projection, mock Pi registration, and mock capability tests passed.
- Changed-Go lint passed except a repeated string in the new persistence test. The test constant was extracted, and the affected models package then passed with zero issues.
- No SQL migration, dependency, global Pi setting, permission bypass, or exact-model relaxation was introduced.


PR review added creation-time normalization and coverage for a model change acknowledged before the catalog event. Both focused race test runs passed:

```bash
cd apps/backend
go test -trimpath -race -tags fts5 ./internal/agent/settings/controller ./internal/agent/runtime/lifecycle ./internal/agentctl/acpcompat -run 'Test(PiThinking|SessionSettingSemantics)' -count=1
go test -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle -run 'TestPiThinking|TestInitializeAndPromptWithLayers' -count=1
```

Further review added stable-key lookup when the advertised Pi option ID differs, cleared old thinking choices when a model change returns no catalog, and bound paired mode and model events to one settings generation. The full ACP adapter race suite and the focused backend race tests passed.

### Changed-Go lint invocation

```bash
bash scripts/lint-go-changed \
  apps/backend/cmd/mock-agent/main.go \
  apps/backend/cmd/mock-agent/pi_thinking.go \
  apps/backend/cmd/mock-agent/pi_thinking_test.go \
  apps/backend/internal/agent/agents/mock.go \
  apps/backend/internal/agent/registry/pi_thinking_test.go \
  apps/backend/internal/agent/registry/provider.go \
  apps/backend/internal/agent/runtime/lifecycle/bootstrap_failure.go \
  apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go \
  apps/backend/internal/agent/runtime/lifecycle/pi_thinking_reset_test.go \
  apps/backend/internal/agent/runtime/lifecycle/pi_thinking_settings_test.go \
  apps/backend/internal/agent/runtime/lifecycle/session.go \
  apps/backend/internal/agent/runtime/lifecycle/session_thinking.go \
  apps/backend/internal/agent/settings/controller/pi_thinking.go \
  apps/backend/internal/agent/settings/controller/pi_thinking_test.go \
  apps/backend/internal/agent/settings/controller/profile_crud.go \
  apps/backend/internal/agent/settings/profileconfig/pi_thinking.go \
  apps/backend/internal/agent/settings/profileconfig/pi_thinking_test.go \
  apps/backend/internal/agentctl/acpcompat/session_setting_semantics.go \
  apps/backend/internal/agentctl/acpcompat/session_setting_semantics_test.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session_mode.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session_semantics.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_updates.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/pi_thinking_settings_test.go \
  apps/backend/internal/agentctl/server/utility/acp_executor.go \
  apps/backend/internal/agentctl/server/utility/pi_thinking_test.go \
  apps/backend/internal/task/models/launch_errors.go \
  apps/backend/internal/task/models/models.go \
  apps/backend/internal/task/models/thinking_errors.go \
  apps/backend/internal/task/models/thinking_errors_test.go
```

The full run reported only the test string duplication described above. After that test-only repair, the models package passed the same check with zero issues. All other listed packages had zero issues.
