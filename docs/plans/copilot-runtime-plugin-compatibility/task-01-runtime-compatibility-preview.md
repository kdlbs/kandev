---
id: "01-runtime-compatibility-preview"
title: "Expose Copilot runtime plugin impact"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-COPILOT-PLUGINS-005
  - REQ-AGENTS-COPILOT-PLUGINS-006
  - REQ-AGENTS-COPILOT-PLUGINS-007
acceptance_criteria:
  - AC-AGENTS-COPILOT-PLUGINS-005.4
  - AC-AGENTS-COPILOT-PLUGINS-005.8
  - AC-AGENTS-COPILOT-PLUGINS-005.9
  - AC-AGENTS-COPILOT-PLUGINS-005.10
  - AC-AGENTS-COPILOT-PLUGINS-006.5
  - AC-AGENTS-COPILOT-PLUGINS-006.6
  - AC-AGENTS-COPILOT-PLUGINS-006.7
  - AC-AGENTS-COPILOT-PLUGINS-007.4
system_design:
  - ../../specs/agents/system-design/copilot-profile-plugins-native-qualification.md
---

# Task 01: Expose Copilot runtime plugin impact

## Summary

Show the selected runtime's explicit-plugin compatibility before runtime
approval. Derive eligibility and qualified alternatives from the existing
provider boundary, with an accurate count or unknown profile impact.

## In scope

- Add `SupportedRuntimeVersions`, an independent ordered projection of the
  exact map for the current host platform. Keep current admission unchanged in
  this work order.
- Add the response-only preview object and matching browser API type from the
  design's runtime amendment. Use the resolved target for explicit/default choices.
- Count non-deleted concrete Copilot profiles with at least one selection,
  including disabled profiles. Mix selected, empty, disabled, deleted, and
  foreign-provider rows in the regression. Count a selected profile once.
- Add controller and handler contract coverage. Repository errors yield a null
  count; cancellation and other providers retain existing preview behavior.
- Show inline advisory feedback in the existing dialog/drawer. Test older
  responses, known zero, unknown impact, qualified targets, and delayed target responses.
- Add localized text in all shipped catalogs and generate Traditional
  Chinese/pseudo catalogs. Extend desktop and mobile runtime E2E fixtures.

## Out of scope

Version admission, native plugin commands, profile writes, extra confirmation,
automatic rollback, and native policy/authentication assertions.

## Acceptance

1. `TestCopilotRuntimeCompatibilityPreview` fails on the old preview contract,
   then passes its supported/unqualified, mixed-profile, unknown-count, and foreign-provider cases.
2. The UI shows the correct target and impact before the existing approval
   action. A delayed response cannot relabel another target; approval behavior stays intact.
3. Desktop and phone checks prove the advisory flow and existing target sizes.
   New copy passes localization gates without exposing profile or native data.

## ASCII UI preview

UI-01 from the [combined preview](plan.md#ui-01-runtime-compatibility-before-approval),
for 005.9/005.10 and 006.5/006.6/006.7:

```text
Desktop dialog                 Phone bottom drawer
Target 9.9.9                   Target 9.9.9
[Version picker]               [Version picker]
Plugin loading unavailable     Plugin loading unavailable
2 saved profiles affected      2 saved profiles affected
Qualified: <server list>        Qualified: <wrapping list>
             [Cancel] [Update]        [Cancel] [Update]
```

Use explicit unknown impact for null count. Known zero still shows the target's
limitation. Preserve the existing body scroller, fixed footer, safe areas,
28px desktop actions, and at least 44px phone actions. Copy is illustrative.

## Verification

From the repository root, after one workspace bootstrap if dependencies are absent:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/copilotplugins ./internal/agent/settings/controller ./internal/agent/settings/handlers -count=1)
(cd apps/web && pnpm exec vitest run components/settings/copilot-plugin-selection.test.ts components/settings/copilot-plugin-profile-selection.test.tsx components/settings/agent-runtime-update-copilot-compatibility.test.tsx components/settings/use-agent-update-dialog-state.test.ts hooks/domains/settings/use-copilot-plugin-discovery.test.tsx components/settings/agent-runtime-update-control.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings components/settings/agent-runtime-update-control.tsx components/settings/copilot-runtime-compatibility.tsx components/settings/agent-runtime-update-copilot-compatibility.test.tsx lib/api/domains/agent-update-api.ts)
(cd apps/web && pnpm run i18n:zh-hant && pnpm run i18n:pseudo && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/settings/agent-runtime-update-copilot.spec.ts tests/settings/copilot-profile-plugins.spec.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/settings/mobile-agent-runtime-update-copilot.spec.ts tests/settings/mobile-copilot-profile-plugins.spec.ts)
bash scripts/lint-go-changed apps/backend/internal/agent/copilotplugins/context.go apps/backend/internal/agent/copilotplugins/runtime_compatibility_test.go apps/backend/internal/agent/settings/dto/dto.go apps/backend/internal/agent/settings/controller/agent_update.go apps/backend/internal/agent/settings/controller/runtime_copilot_compatibility.go apps/backend/internal/agent/settings/controller/runtime_copilot_compatibility_test.go apps/backend/internal/agent/settings/handlers/runtime_copilot_compatibility_test.go
git diff --check
```

Do not repeat installation if the workspace is already bootstrapped. Run the
new causal regression alone for Red, then the listed suites for final Green.
Extend the exact lint list for any additional changed Go source or test file.
Managed E2E commands run sequentially and rebuild the assets they test.

## Files likely touched

- `apps/backend/internal/agent/copilotplugins/context.go` and new `runtime_compatibility_test.go`.
- `apps/backend/internal/agent/settings/dto/dto.go`.
- `apps/backend/internal/agent/settings/controller/agent_update.go` and new `runtime_copilot_compatibility.go` / `runtime_copilot_compatibility_test.go`.
- New `apps/backend/internal/agent/settings/handlers/runtime_copilot_compatibility_test.go`.
- `apps/web/lib/api/domains/agent-update-api.ts`.
- `apps/web/components/settings/agent-runtime-update-control.tsx`, `copilot-runtime-compatibility.tsx`, and their tests.
- `apps/web/components/settings/use-agent-update-dialog-state.test.ts` and its hook only if target fencing needs a correction.
- `apps/web/hooks/domains/settings/use-agent-runtime-updates.test.tsx` and its API hook if response handling needs a correction.
- `apps/web/e2e/tests/settings/agent-runtime-update-helpers.ts`, `agent-runtime-update-copilot.spec.ts`, and `mobile-agent-runtime-update-copilot.spec.ts`.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko,pseudo}/agents.json`.

New test paths are proposed outputs; all other paths exist at the planning baseline.

## Dependencies

None. Read the native-authority runtime amendment and both existing ADRs linked
from the plan. Do not treat a preview as launch admission.

## Risks

Profile count is an advisory snapshot. Missing reads must never become zero.
Both UI work orders share locale catalogs and must execute sequentially.

## Parallelism

`sequential`

## Inputs

- Requirements 005, 006, and 007 in `copilot-profile-plugins.md`.
- Native-authority design: Runtime compatibility amendment.
- `Controller.previewAgentUpdate`, settings `Repository`, existing runtime
  update control/tests, and `AgentRuntimeUpdateSurface`.

## Results

Implemented the shared `SupportedRuntimeVersions` projection, the response-only
`copilot_plugin_compatibility` preview object, the non-deleted/nonempty profile
count, and the inline desktop/phone advisory. Preview remains read-only and adds
no approval gate. Foreign providers omit the object.

Evidence:

- `TestCopilotRuntimeCompatibilityPreview` covers supported/unqualified targets,
  disabled selected profiles, empty/deleted/foreign profiles, unknown impact,
  cancellation, and foreign-provider omission.
- Handler contract tests cover the Copilot object and foreign-provider omission.
- `agent-runtime-update-copilot-compatibility.test.tsx` covers unqualified,
  unknown, zero, qualified, older-server fallback, and approval behavior. The
  dialog-state fencing test verifies a delayed response cannot relabel another
  target.
- Chromium `agent-runtime-update-copilot.spec.ts` passed 2 tests; mobile-chrome
  `mobile-agent-runtime-update-copilot.spec.ts` passed 1 test.
- Typecheck, i18n check/ratchet, and changed-file Oxlint passed.
