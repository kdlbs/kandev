---
id: "02-permission-controls"
title: "Expose and prove Devin settings"
status: in-progress
wave: 2
depends_on:
  - "01-permission-intent"
plan: "plan.md"
requirements:
  - REQ-AGENTS-DEVIN-PERMISSIONS-001
acceptance_criteria:
  - AC-AGENTS-DEVIN-PERMISSIONS-001.1
  - AC-AGENTS-DEVIN-PERMISSIONS-001.2
  - AC-AGENTS-DEVIN-PERMISSIONS-001.3
  - AC-AGENTS-DEVIN-PERMISSIONS-001.4
  - AC-AGENTS-DEVIN-PERMISSIONS-001.6
  - AC-AGENTS-DEVIN-PERMISSIONS-001.7
  - AC-AGENTS-DEVIN-PERMISSIONS-001.8
  - AC-AGENTS-DEVIN-PERMISSIONS-001.9
system_design:
  - ../../specs/agents/system-design/devin-profile-permissions.md
---

# Task 02: Expose and prove Devin settings

## Summary

Expose Devin's sandbox override beside its existing profile permission controls.
Use the existing Start mode selector for Smart, Bypass, and conditional Autonomous behavior.
Add localized guidance, public docs, and desktop/phone acceptance.

## In scope

- Pure draft-default and sandbox-edit helpers.
- Provider-specific sandbox preference and contextual approval/terminal help.
- Unsupported Smart and sandbox-failure states without loss of unfinished edits.
- Localized copy in all seven languages and public installation/permission instructions.
- An isolated E2E Devin settings descriptor and deterministic capability scenarios.

## Out of scope

- A second mode selector, independent Smart/Bypass switches, routing-provider expansion, or live provider credentials.
- Automatic CLI or sandbox dependency installation.

## Acceptance

1. New drafts prefer Smart and show the matching sandbox override, while existing profiles retain their saved or inherited settings.
2. Desktop and phone users can choose Bypass or sandbox/Autonomous, save, and reload without contradictory active settings.
3. Unsupported and failed states retain edits and retry controls, with localized, keyboard-accessible and touch-accessible presentation.

## ASCII UI preview

Use [UI-DEVIN-01/02/03 in the plan](plan.md#ascii-ui-preview).
The existing page owns scrolling and Save.

```text
Desktop:  Start model [model v]  Start mode [Smart v]  [Refresh]
          Extra settings: Devin sandbox [Disabled v]
          Auto-approve all permissions [off]

Phone:    Start model [model                    v]
          Start mode  [Smart                    v]
          Extra settings
          Devin sandbox
          [Disabled                             v]
          Auto-approve all permissions [off]

Enabled:  Start mode [Autonomous]  Sandbox [Enabled v]
Failure:  Preserve the requested values and show retry/edit actions.
```

AC-AGENTS-DEVIN-PERMISSIONS-001.1/001.2/001.7/001.8 map to the normal views.
Criteria 001.3/001.4/001.9 map to the failure view.
Spacing illustrates hierarchy. Shared primitives own physical sizing.

## TDD and implementation details

Add `lib/devin-profile-settings.test.ts` before the helper and component.
Cover new Smart defaults, explicit existing values, atomic sandbox changes, inherited/custom values, and unrelated entry preservation.
Add `devin-sandbox-preference.test.tsx` for selection, labels, dirty state, keyboard use, and failure retention.
Extend new-draft creation tests to cover the backend-equivalent default table.

Wire the preference into `ProfileFormFields` for `devin-acp` only.
Reuse the existing Start mode control, provider mode metadata, and draft discovery hook.
Mark sandbox edits stale through the existing full launch snapshot.
Treat enabled sandbox as an Autonomous request, rather than leave a conflicting Smart/Bypass choice active.
Show inherited values accurately on legacy profiles.
Keep Kandev auto-approval independent and explain its effect.
Clearly scope ACP Start mode when CLI passthrough is enabled.

Follow the phone profile-discovery exemplar.
Stack the sandbox field on phones and keep one existing settings scroll owner.
Use shared Select controls and at least 44px phone/coarse-pointer targets.
Preserve compact desktop density and accessible error announcements.

For E2E, add an opt-in Devin mock descriptor following the existing `KANDEV_E2E_MOCK_COPILOT` pattern.
Gate it under E2E mode, keep it outside default profile selection, and restore the worker environment/data after each test.
Do not add `devin-acp` to `RoutableProviderIDs` for this fixture.
Expose Smart-present, Smart-absent, Autonomous, and sandbox-error contexts through the deterministic fake peer.
Save and reload through real APIs.
Pair mocked capabilities with Task 01's real child-environment and mode-confirmation tests.

Add desktop/phone cases for defaults, Bypass, enabled sandbox, legacy inherited values, missing Smart, failed sandbox, and independent auto-approval.
Assert target geometry and document overflow on phone.
Check keyboard selection on desktop and touch selection on phone.

Add copy to English, Portuguese, Simplified Chinese, Japanese, and Korean catalogs.
Generate the Traditional Chinese pair with `pnpm run i18n:zh-hant`.
Update `docs/public/agents-and-profiles.md` with the official curl installer and the qualified permission behavior.
The installer implementation already matches the official endpoint.

## Verification

If dependencies are absent, install them once from `apps/` before these commands.

```bash
(cd apps && pnpm install --frozen-lockfile)
```

Run from the repository root. Keep managed E2E invocations sequential.

```bash
(cd apps/web && pnpm exec vitest run lib/devin-profile-settings.test.ts components/settings/devin-sandbox-preference.test.tsx 'app/settings/agents/[agentId]/agent-create-catalogue.test.tsx' 'app/settings/agents/[agentId]/agent-create-target-catalogue.test.tsx')
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint lib/devin-profile-settings.ts lib/devin-profile-settings.test.ts components/settings/devin-sandbox-preference.tsx components/settings/devin-sandbox-preference.test.tsx components/settings/profile-form-fields.tsx components/settings/profile-capabilities-row.tsx 'app/settings/agents/[agentId]/page.tsx' e2e/helpers/devin-profile-permissions.ts e2e/tests/settings/devin-profile-permissions.spec.ts e2e/tests/settings/mobile-devin-profile-permissions.spec.ts)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/devin-profile-permissions.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-devin-profile-permissions.spec.ts)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/registry -run 'Test.*(Mock|Devin)' -count=1)
bash scripts/lint-go-changed apps/backend/internal/agent/registry/provider.go apps/backend/internal/agent/registry/provider_test.go
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Add any further changed frontend source/test paths to ESLint and Vitest commands.
Keep the Go lint list aligned with actual registry fixture changes.
Record actual desktop and phone discovery/pass counts.

## Files likely touched

- New `apps/web/lib/devin-profile-settings.ts` and `devin-profile-settings.test.ts`
- New `apps/web/components/settings/devin-sandbox-preference.tsx` and `devin-sandbox-preference.test.tsx`
- `apps/web/components/settings/profile-form-fields.tsx`
- `apps/web/components/settings/profile-capabilities-row.tsx` and relevant mode-selector helpers
- `apps/web/app/settings/agents/[agentId]/page.tsx` and its new-draft tests
- New `apps/web/e2e/tests/settings/devin-profile-permissions.spec.ts`
- New `apps/web/e2e/tests/settings/mobile-devin-profile-permissions.spec.ts`
- New `apps/web/e2e/helpers/devin-profile-permissions.ts`
- `apps/backend/internal/agent/registry/provider.go` and mock registry tests
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko}/agents.json`
- `docs/public/agents-and-profiles.md`

## Dependencies

[Task 01](task-01-permission-intent.md) must pass first.

## Risks

- Fresh capability snapshots can omit Smart even with the same CLI version.
- Rendering an inherited override as Disabled misrepresents effective configuration.
- Mock capabilities prove UI behavior, not actual Devin permission or sandbox enforcement.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/devin-profile-permissions.md)
- [System design](../../specs/agents/system-design/devin-profile-permissions.md)
- Existing Copilot curated settings, profile capability controls, and phone profile-discovery tests.

## Results

Implemented 2026-11-03; browser acceptance remains open.

- `lib/devin-profile-settings.ts` mirrors the backend defaults and sandbox edits. `lib/devin-profile-settings.test.ts` covers new Smart defaults, explicit values, inherited/custom values, atomic enabled/disabled/inherit changes, unrelated entry preservation, and conflict detection.
- `components/settings/devin-sandbox-preference.tsx` is the three-choice control wired into `ProfileFormFields` for `devin-acp`. It shows the inherited value accurately, explains the Autonomous coupling, announces an unavailable Smart mode, and uses a 44px touch target.
- New drafts created from the agent setup page receive Smart plus explicit `DEVIN_SANDBOX=false`; existing drafts and saved profiles are untouched.
- Localization: eight new keys in English, Portuguese, Simplified Chinese, Japanese, and Korean; the Traditional Chinese pair was generated with `pnpm run i18n:zh-hant`. `pnpm run i18n:check` passed.
- Browser coverage: `KANDEV_E2E_MOCK_DEVIN` registers a disposable Devin identity through the deterministic mock ACP peer. `e2e/helpers/devin-profile-permissions.ts` and the desktop/phone `devin-profile-permissions` specs cover the new-profile default, saved sandbox/Autonomous coupling, and phone touch geometry.
- Managed runs: `pnpm e2e:run --host --shards 1 --project chromium tests/settings/devin-profile-permissions.spec.ts` passed (2 passed, 10.1s, including the explicit Smart plus `DEVIN_SANDBOX=false` save). `--project mobile-chrome tests/settings/mobile-devin-profile-permissions.spec.ts` passed (1 passed, 8.0s, 44px control and no horizontal overflow).
- Remaining gap: the mock peer does not yet expose Smart-present, Smart-absent, or sandbox-initialization-failure scenarios, so those capability branches rely on unit/controller tests rather than browser proof. This work order stays open for that reason.
