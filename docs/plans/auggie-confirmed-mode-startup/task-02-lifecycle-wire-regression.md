---
id: "02-lifecycle-wire-regression"
title: "Prove lifecycle prompt admission for confirmed legacy modes"
status: done
wave: 2
depends_on:
  - "01-confirm-existing-legacy-mode"
plan: "plan.md"
requirements:
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-002
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-007
acceptance_criteria:
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.7
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-007.3
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-007.4
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-007.8
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-007.9
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-007.10
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-007.11
system_design:
  - ../../specs/agents/system-design/agent-permission-control-integrity.md
---

# Task 02: Prove lifecycle prompt admission for confirmed legacy modes

## Summary

Add deterministic boundary evidence that a reported matching legacy mode permits
the first prompt through normal startup and ordinary resume. Use the real ACP
adapter's result so this test reproduces the original failure before Task 01.

## In scope

- Add `TestLegacyConfirmedModeLifecycle` with fresh-start, native-load, and
  context-reset cases plus different-mode and missing-report negative cases.
  Use a temporary workspace and ACP pipe
  agent reporting Auggie's legacy-only shape, with `{}` and no notification for
  any mode mutation. The fixture advertises `default` and `ask`.
- Connect a real `acp.Adapter` to that fixture and initialize it normally. Reuse
  the lifecycle WS test-server/client infrastructure, routing session new/load,
  mode and prompt actions to the adapter rather than synthesizing mode success.
- Drain adapter updates, close pipes/connections, and join fixture goroutines.
  Avoid real inference, provider auth, global files, or mock-agent feature flags.
- Assert provider prompt count, no mode mutation for matching reports, selected
  mode confirmation before dispatch, and unchanged profile/override inputs.
  For `default -> ask`, assert unconfirmed failure and no provider prompt.
- Retain existing strict startup, ordinary-resume, context-reset, and explicit
  provider-restored recovery tests. No lifecycle gate weakening is expected.

## Out of scope

New production lifecycle policy, real model requests, UI/error-copy changes,
model fallback changes, and broader Office/provider policy changes.

## Acceptance

- Fresh `session/new` and ordinary `session/load` with matching `default` each
  admit exactly one first provider prompt and send zero mode mutation RPCs.
- A matching report from the replacement context-reset session admits its next
  prompt with zero redundant mode RPCs. A different silent selected mode fails
  before prompt dispatch; resumed and reset sessions missing their own report
  cannot borrow previous-session state.
- Existing winning-mode, strict Auggie mode/model, reset, and explicit recovery
  behavior passes its focused checks; fixture cleanup is race/leak clean.

## Verification

From repository root. Prove red by running the new wire fixture against Task
01's pre-fix adapter behavior in a temporary reversible local change, then
restore Task 01 and run the commands below. Do not change the live instance.

```bash
(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/lifecycle -run '^TestLegacyConfirmedModeLifecycle$' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/lifecycle -run '^(TestLegacyConfirmedModeLifecycle|TestInitializeAndPromptWithLayers_AuggieTaskRejectsUnappliedMode|TestInitializeAndPromptWithLayers_AppliesOnlyWinningModeBeforePrompt|TestInitializeAndPromptWithLayers_ReappliesModeAfterResumeBeforePrompt|TestInitializeAndPromptWithLayers_ProviderRestoredOmitsSavedStartupSettings|TestAuggieTaskStartRequiresSelectedModel)$' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/lifecycle -run '^(TestApplySessionModeAfterResetDoesNotCacheUnconfirmedRequest|TestManager_ResetAgentContext_FailsClosedOnSessionModeRestore|TestManager_ResetAgentContext_ReappliesSessionMode)$' -count=1)
(cd apps/backend && go test -tags fts5 -race ./internal/agent/runtime/lifecycle -run '^TestLegacyConfirmedModeLifecycle$' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Task 01's full adapter test command already covers its reset-transition boundary.

## Files likely touched

- New `apps/backend/internal/agent/runtime/lifecycle/session_mode_legacy_integration_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/session_test.go` (test helpers only if reuse requires it)
- `docs/plans/auggie-confirmed-mode-startup/plan.md` and both work orders (status/results)

## Dependencies

Task 01 supplies the guarded adapter result. Preserve its files and tests.

## Risks

A server returning a handcrafted confirmed result would hide the bug. Keep the
wire fixture connected to the production adapter, assert RPC counts, and prove
the initial prompt reaches the provider fixture. Join update-drain goroutines
so lifecycle package leak checks remain meaningful.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/permission-control-integrity.md), sections 002 and 007.
- [Design](../../specs/agents/system-design/agent-permission-control-integrity.md#already-satisfied-legacy-modes).
- [Strict Auggie startup](../agent-resume-mode-fallback/task-01-strict-auggie-startup.md).
- `newMockAgentServer`, `createTestClient`, `connectAgentStream`, and prompt
  callbacks in `session_test.go`; `permission_mode_e2e_test.go` adapter connection
  and update-drain pattern (reuse the pattern, not its Claude credential setup).
- Root, backend, and agentctl `AGENTS.md`; `/tdd`.

## Results

Implemented `TestLegacyConfirmedModeLifecycle` as a real lifecycle-to-adapter
fixture. Fresh start, native load, and reset each use the adapter's session
report. Matching reports admit one provider prompt with no provider mode
mutation; a silent `default -> ask` change fails before prompt dispatch. Load
and reset cases prove that missing replacement-session mode state cannot reuse
the prior session's cached mode. The test also checks the final selected mode
and request ordering, drains adapter updates, closes the client and ACP pipes,
and joins the fixture's disconnect/update/ACP goroutines.

The lifecycle regression was red with the guarded shortcut disabled: fresh,
load, and reset matching-mode cases all failed because the silent provider
response was unconfirmed. Restoring the guard made all cases pass.

Validation passed:

```text
go test -tags fts5 ./internal/agent/runtime/lifecycle -run '^TestLegacyConfirmedModeLifecycle$' -count=1
go test -tags fts5 ./internal/agent/runtime/lifecycle -run '^(TestLegacyConfirmedModeLifecycle|TestInitializeAndPromptWithLayers_AuggieTaskRejectsUnappliedMode|TestInitializeAndPromptWithLayers_AppliesOnlyWinningModeBeforePrompt|TestInitializeAndPromptWithLayers_ReappliesModeAfterResumeBeforePrompt|TestInitializeAndPromptWithLayers_ProviderRestoredOmitsSavedStartupSettings|TestAuggieTaskStartRequiresSelectedModel)$' -count=1
go test -tags fts5 ./internal/agent/runtime/lifecycle -run '^(TestApplySessionModeAfterResetDoesNotCacheUnconfirmedRequest|TestManager_ResetAgentContext_FailsClosedOnSessionModeRestore|TestManager_ResetAgentContext_ReappliesSessionMode)$' -count=1
go test -tags fts5 -race ./internal/agent/runtime/lifecycle -run '^TestLegacyConfirmedModeLifecycle$' -count=1
make -C apps/backend build
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```
