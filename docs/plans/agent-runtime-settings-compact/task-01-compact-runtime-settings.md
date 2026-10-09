---
id: "01-compact-runtime-settings"
title: "Compact runtime settings and navigation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-RUNTIME-NOTIFY-001
  - REQ-AGENTS-RUNTIME-NOTIFY-002
acceptance_criteria:
  - AC-AGENTS-RUNTIME-NOTIFY-001.1
  - AC-AGENTS-RUNTIME-NOTIFY-001.3
  - AC-AGENTS-RUNTIME-NOTIFY-001.4
  - AC-AGENTS-RUNTIME-NOTIFY-001.5
  - AC-AGENTS-RUNTIME-NOTIFY-001.6
  - AC-AGENTS-RUNTIME-NOTIFY-002.7
system_design:
  - ../../specs/agents/system-design/runtime-update-notifications.md
---

# Task 01: Compact Runtime Settings and Navigation

## Summary

Make runtime policies a compact section at the bottom of Agents settings with a closed ordinary-entry state. Preserve existing runtime notification entry points, policy drafts, management guidance and recovery on desktop and phone.

## In scope

- Move the section below installed-agent content; use the existing `SettingsGroup` disclosure and target registry with mounted children.
- Compact runtime rows, consolidate common help, preserve ownership/version/eligibility and outcome information, and apply intentional phone controls.
- TDD for disclosure and fragment behavior, focused desktop/phone E2E and public guide location instructions.

## Out of scope

Backend behavior, notification/indicator redesign, shared Settings redesign, changes to version dialogs/drawers or policy semantics, new dependencies. Commit, publication and merge are authorized by the user after validation.

## Acceptance

1. Ordinary entry shows installed agents first and a closed runtime section last; expanded rows follow UI-03/UI-04 with common help once and all existing status/actions available.
2. Existing indicator/group and notification/runtime fragments reveal, focus and scroll their destination on initial navigation, same-page navigation and delayed row arrival; inactive destinations open both disclosures. Closing/reopening retains dirty policy drafts and the shared save contribution.
3. Desktop and phone notification, consent/save/reload, unknown/native/fallback guidance and recovery flows pass. Phone hitboxes are at least 44px and sources wrap without horizontal page overflow; public guide describes the new location and disclosure.

## ASCII UI preview

Relevant excerpts of [UI-02 through UI-04 in the plan](plan.md#ascii-ui-preview), covering AC-AGENTS-RUNTIME-NOTIFY-001.5 and 001.6:

```text
UI-02: Ordinary visit, desktop and phone
Installed agents
  Agent cards and profiles
> Agent runtime updates

UI-03: Expanded desktop
v Agent runtime updates
  Shared explanation and help
  Claude  Selected: 0.62.0  Latest: 0.64.0
                    Automatic updates [off] [Manage versions]
  Runtime source | Kandev-managed
  [Retained outcome when present]
  > Other registrations (N)

UI-04: Expanded phone
v Agent runtime updates
  Shared explanation and help
  Claude
  Selected: 0.62.0   Latest: 0.64.0
  Runtime source | Kandev-managed
  Automatic updates                       [off]
  [Manage versions                            ]
  [Retained outcome when present]
  > Other registrations (N)
```

The page owns scrolling. Phone disclosures and controls retain 44px targets. Example versions/spacing are illustrative; bottom placement, compact grouping, initial collapse and automatic target reveal are required. Retain native/manual limitations and unknown values as specified in the complete preview.

## Verification

Run from the repository root; install workspace dependencies once before the first package command if absent. Add and run failing disclosure/navigation tests before changing production behavior, then use these commands for final checks:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/settings/agent-runtime-policies.test.tsx components/settings/use-runtime-auto-update-policy.test.ts components/settings/settings-group.test.tsx)
(cd apps/web && pnpm exec eslint --max-warnings 0 app/settings/agents/page.tsx components/settings/agent-runtime-policies.tsx components/settings/agent-runtime-policies.test.tsx components/settings/settings-group.tsx e2e/tests/settings/agent-runtime-notifications.spec.ts e2e/tests/settings/mobile-agent-runtime-notifications.spec.ts e2e/tests/settings/agent-runtime-notifications-helpers.ts e2e/tests/settings/agent-runtime-settings-helpers.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/agent-runtime-notifications.spec.ts tests/settings/agent-runtime-update.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-agent-runtime-notifications.spec.ts tests/settings/mobile-agent-runtime-update.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Do not overlap E2E runs or skip their fresh builds. Include any additional changed helper/test in the relevant lint/test invocation. Capture desktop/phone screenshots and compare the grouping, scroll owner and action order with the assigned previews. Once checks pass, mark this order done, update the plan to implemented, and confirm the extended requirements/design match the result.

## Files likely touched

- `apps/web/app/settings/agents/page.tsx`
- `apps/web/components/settings/agent-runtime-policies.tsx`
- `apps/web/components/settings/agent-runtime-policies.test.tsx`
- `apps/web/components/settings/settings-group.tsx` (owning disclosure chevron scope only)
- `apps/web/e2e/tests/settings/agent-runtime-notifications.spec.ts`
- `apps/web/e2e/tests/settings/mobile-agent-runtime-notifications.spec.ts`
- `apps/web/e2e/tests/settings/agent-runtime-notifications-helpers.ts`
- `apps/web/e2e/tests/settings/agent-runtime-settings-helpers.ts`
- `docs/public/agents-and-profiles.md`
- Locale catalogs only if an existing key cannot express necessary copy.
- This work order and its plan for actual results/status.

## Dependencies

None. Current runtime notification implementation and Settings target infrastructure are present in this checkout.

## Risks

See the plan's disclosure target, draft lifetime and ownership risks. Do not remove existing notification/persistence assertions to make the new collapsed layout pass.

## Parallelism

`sequential`

## Inputs

- [Runtime update notification requirements](../../specs/agents/requirements/runtime-update-notifications.md).
- [Compact settings presentation design](../../specs/agents/system-design/runtime-update-notifications.md#compact-settings-presentation).
- Existing `SettingsGroup`, `SettingsTargetProvider`, `revealSettingsTarget`, `settingsActionClassName` and runtime-policy test/helper patterns.
- `/tdd`, `/mobile-parity`, `/e2e`, `/docs-maintainer` and `apps/web/AGENTS.md`.

## Results

Implemented on 2026-10-02 in the user-started session. Initial policy disclosure test failed before the UI change (ordinary entry was fully expanded); after the change it passes. Rendered verification reproduced a chevron that remained unrotated (`rotate: none` instead of `90deg`) before correcting the shared disclosure group scope. Notification producers, indicator content and backend policy/update logic are unchanged.

Final verification (commands above run with the installed mise toolchain; E2E used `CAPTURE_PR_ASSETS=true` and `--host`):

- Workspace `pnpm install --frozen-lockfile`: passed.
- Targeted Vitest: 3 files, 17 tests passed, including existing SettingsGroup save/disclosure behavior.
- Changed-file ESLint including the new runtime-settings helper and shared SettingsGroup: passed. Typecheck: passed.
- `pnpm run i18n:check`: passed; existing catalog orphan notices are informational.
- Fresh production-build desktop E2E: 20 passed (59.3s), including notification navigation, ordinary closed entry, dirty draft retention, inactive fragment reveal and 390/767/768px controls.
- Fresh production-build phone E2E: 7 passed (25.2s), including touch expansion, at least 44px active targets, saved policy/reload, native fallback guidance, version drawer, rollback and retry.
- Public-doc validator tests: 62 passed; public docs validation: 47 pages passed. Updated the runtime notifications reference in `agents-and-profiles.md` with bottom placement and disclosure instructions.
- Documentation catalog, specification lint and diff checks: passed. PR-documentation coverage preflight resolves the work-order/requirement/acceptance/design chain.

Desktop and phone closed/expanded captures follow UI-02/UI-03/UI-04. The shared capture fixture replaces earlier manifest entries from the same spec when a later scenario captures; final PR assets are recaptured with one selected notification scenario per viewport and merged before publication. The final single-scenario capture commands reuse the just-built assets (`--no-build`) and disable retries. Implementation is complete; external delivery state is tracked in the task plan.
