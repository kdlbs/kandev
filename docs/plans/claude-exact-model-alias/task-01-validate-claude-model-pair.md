---
id: "01-validate-claude-model-pair"
title: "Validate strict Claude model settings and show the correction"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-001
acceptance_criteria:
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.1
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.10
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.11
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.12
system_design:
  - ../../specs/agents/system-design/no-silent-model-fallback-01.md
  - ../../specs/agents/system-design/no-silent-model-fallback-02.md
---

# Task 01. Validate strict Claude model settings and show the correction

## Summary

Reject the issue's conflicting pair in the normal profile save flow.
Keep the draft available so the user can make the selection explicit.

## Implementation

1. Add `TestClaudeExactModelConflictProfileSave` in the settings HTTP harness.
   Start with the issue's POST payload. Expect HTTP 400, a fixed error code,
   and zero profile writes or broadcasts. On the baseline the request returns
   200 and writes the profile.
2. Add one controller check for a different explicit `ANTHROPIC_MODEL`
   paired with a strict bare Claude family alias. Call it after
   `validateProfileEnvVarDTOs` in `CreateProfile` and after
   `validateCreateProfileRequest` for each nested profile before the agent
   insert. In `UpdateProfile`, capture the saved inputs before the patch.
   Check the merged inputs after the environment merge, outside the
   `req.EnvVars` block and before setting `UserModified` or writing.
   Use `agentNameForProfile` when the selection change can trigger the check.
3. Map the rejection to the same HTTP 400 response at the three endpoints.
   In their shared `agentSettingsRequest`, set the matching `ApiError` message
   with `t("agents:claudeExactModelAliasConflict", { envVar: "ANTHROPIC_MODEL" })`
   when the request fails. Rethrow the same error. The existing create and
   edit toasts show its message in the description.
4. Add the settings guidance to the existing exact-model section of
   `docs/public/agents-and-profiles.md` with the behavior change. Cover
   existing profiles by explaining how to choose the exact ID advertised by
   their executor. Preserve the distinction between settings validation and
   executor availability.

The delivery covers settings creation and edits. Duplicate and restore use
their existing copy semantics. Runtime model application, authentication,
environment resolution, and provider fallback stay with their existing owners.

## Acceptance

1. Profile POST, profile PATCH, and create-agent POST with a nested profile
   reject the conflicting pair before any write or profile event. Cover
   model-only, environment-only, and strictness-only patches. Check that the
   HTTP 400 body matches the fixed code and correction.
2. Full IDs, context variants such as `sonnet[1m]`, matching or omitted
   literal values, secret references, compatible profiles, and other
   agents keep their normal save behavior. Seed a conflicting profile.
   Its unrelated patch and identical full-form submission succeed.
   Selecting the full ID or removing the environment entry repairs it.
   Confirm that strictness and stored fallback fields survive the repair.
3. On desktop and phone, the failed save shows the localized correction.
   Both controls remain editable. Select the full ID, save again, and confirm
   the result on reload. On the phone, verify that the toast fits the viewport
   and that the page has no horizontal overflow.

## ASCII UI preview

**UI-01 · Claude profile save error** after Save in Settings > Agents &
Profiles. See the [complete preview](plan.md#ascii-ui-preview).

```text
Start model          sonnet
Fallback settings    Exact model required
Environment Variables
  ANTHROPIC_MODEL    claude-sonnet-5-5
[Save changes]

Failed to save profile
Choose a full model ID in Start model, or remove
ANTHROPIC_MODEL from Environment Variables.
```

On create, the existing agent setup error title may read "Failed to save
agent." The correction stays the same. On the phone, use the existing
single-column profile editor and shared toast. Retain the desktop keyboard
and phone touch paths through the model picker and Save control.

## Verification

Cover selection logic with `TestClaudeExactModelAliasConflict` in the
controller. Use table rows for the four aliases, `sonnet[1m]`, a full ID,
agent and strictness, and the literal value cases. Include a trimmed value,
a whitespace-only value, a different key case, and a secret reference.
`TestClaudeExactModelAliasConflictTransitions` covers a new or changed pair,
an identical legacy pair, and a repaired pair. Use HTTP tests for request
merging, error responses, and writes.

The existing `TestDuplicateProfile_CopiesFullConfiguration` checks that
duplication copies the model, strictness, fallback, and environment entries.
Run it as the copy regression control.

Add an action test in `agents.test.ts` that asserts the translated
`ApiError.message` and preserves the status, code, and response body.

Use the existing fake repository and settings router for server tests. Use
`KANDEV_MOCK_PROVIDERS=claude-acp` with the managed isolated backend in the
browser cases. Seed a compatible Claude profile with the pair and a dormant
fallback. If needed, serve both sample model IDs through the model-discovery
fixture, then use real profile write requests. Enable strictness, expect the
error toast, select the full ID, save again, and reload. Use the existing
mobile layout assertion and remove the disposable profile at cleanup.

Run from the repository root. On a fresh worktree, install dependencies once
with `(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/backend && go test -trimpath ./internal/agent/settings/handlers ./internal/agent/settings/controller -run '^(TestClaudeExactModelConflictProfileSave|TestClaudeExactModelAliasConflict|TestClaudeExactModelAliasConflictTransitions|TestCreateProfileEndpoint|TestUpdateProfileEndpoint|TestUpdateProfileEndpoint_WiresProviderFields|TestCreateAgentEndpointRejectsInvalidRequests|TestValidateRequireExactModelPolicy|TestDuplicateProfile_CopiesFullConfiguration)$' -count=1)
(cd apps/backend && go test -trimpath ./internal/agent/runtime/lifecycle -run '^(TestApplyStartModelPolicyExecutorAuthority|TestStrictStartModelFailure.*|TestRequiredEmptyStartModelReportsSelectionMissing|TestStartModelCancellationAndDeadlineKeepTheirMeaning|TestStartModelTransportFailureIsNotReportedAsModelApplicationFailure|TestStartModelProviderRejectionStaysModelApplicationFailure|TestStartModelConsumersPreserveProviderReplyCause|TestStartModelStartupSuccessDispatchesPrompt|TestInitializeAndPromptWithLayers_UnadvertisedModelFailsBeforeInference)$' -count=1)
bash scripts/lint-go-changed apps/backend/internal/agent/settings/controller/{agent_crud,profile_crud,claude_exact_model_policy,claude_exact_model_policy_test}.go apps/backend/internal/agent/settings/handlers/{handlers,profile_handlers,claude_exact_model_validation_test}.go
(cd apps/web && pnpm run i18n:zh-hant && pnpm run i18n:pseudo)
(cd apps/web && pnpm exec vitest run app/actions/agents.test.ts components/settings/agent-profile-save-catalogue.test.tsx)
(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings app/actions/agents.ts app/actions/agents.test.ts)
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/settings/claude-exact-model-alias.spec.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/settings/mobile-claude-exact-model-alias.spec.ts
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files

- `apps/backend/internal/agent/settings/controller/{agent_crud,profile_crud,claude_exact_model_policy,claude_exact_model_policy_test}.go`
- `apps/backend/internal/agent/settings/handlers/{handlers,profile_handlers,claude_exact_model_validation_test}.go`
- `apps/web/app/actions/agents.ts` and `apps/web/app/actions/agents.test.ts`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko,pseudo}/agents.json`
- `apps/web/e2e/tests/settings/{claude-exact-model-alias,mobile-claude-exact-model-alias}.spec.ts`
- `docs/public/agents-and-profiles.md`

## Dependencies and risk

No preceding work order. Apply the guard only to the normalized selection
inputs. Comparing full environment lists would block unrelated legacy edits.
The catalog used at launch can still differ from what the editor shows.

## Results

The settings controller now rejects newly created or changed strict Claude ACP
bare-alias pairs with a different literal `ANTHROPIC_MODEL`. Existing matching
legacy pairs still allow unrelated edits and can be repaired. All three write
endpoints return the same fixed HTTP 400 error, and the shared web action shows
the localized correction while retaining the editable draft.

Verification passed: the settings controller and HTTP regression suite, the
runtime lifecycle regression suite, changed-Go lint, and both selected web
Vitest files (13 tests). Oxlint, i18n checks and ratchet, desktop Chromium E2E,
mobile Chrome E2E, public-doc tests (63 tests), public-doc validation (55
pages), spec validation (459 decisions and 2,070 specifications), full spec
lint, and `git diff --check` all passed. The E2E runs also built the backend
and Vite assets.
