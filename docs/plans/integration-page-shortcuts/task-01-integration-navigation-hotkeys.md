---
id: "01-integration-navigation-hotkeys"
title: "Implement integration navigation hotkeys"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-INTEGRATION-PAGE-SHORTCUTS-001
acceptance_criteria:
  - AC-UI-INTEGRATION-PAGE-SHORTCUTS-001.1
  - AC-UI-INTEGRATION-PAGE-SHORTCUTS-001.2
  - AC-UI-INTEGRATION-PAGE-SHORTCUTS-001.3
  - AC-UI-INTEGRATION-PAGE-SHORTCUTS-001.4
  - AC-UI-INTEGRATION-PAGE-SHORTCUTS-001.5
  - AC-UI-INTEGRATION-PAGE-SHORTCUTS-001.6
  - AC-UI-INTEGRATION-PAGE-SHORTCUTS-001.7
  - AC-UI-INTEGRATION-PAGE-SHORTCUTS-001.8
system_design:
  - ../../specs/ui/system-design/integration-page-shortcuts.md
---

# Task 01: Implement Integration Navigation Hotkeys

## Summary

Deliver the integration shortcut catalog, settings group, and global dispatcher
as one usable slice. Prove persistence, conflicts, and desktop/phone behavior
with the targeted tests in the plan.

## In scope

- The design's six built-in destinations, dynamic plugin integration links,
  stable identities, default-unbound resolution, and shared conflict entries.
- App-root navigation with core precedence and recording/editable/repeat guards.
- Existing settings draft/save integration with per-key rebasing and revision
  guards; reset-by-deletion; phone stacked controls and translated labels.
- Targeted unit/E2E tests and the keyboard settings how-to update.

## Out of scope

- Backend changes, provider connection changes, new dashboards, runtime flags,
  plugin SDK changes, plugin action editor relocation, broad review/test suites.

## Acceptance

- Catalog and dispatcher satisfy .1, .2, .4-.6; every active supported
  destination has one stable, configurable host navigation entry.
- Settings record/reset/Save satisfy .3 and .8, preserving other entries and
  activating only acknowledged bindings, including failed and concurrent saves.
- Desktop and phone rendered flows satisfy .7 and the UI-01/02/03 structure,
  with all targeted commands passing and results recorded.

## ASCII UI preview

Excerpt of [the full plan preview](plan.md#ascii-ui-preview), AC .1-.3, .6-.8.

UI-01: Desktop, Settings > Preferences > Keyboard Shortcuts.

```text
Integrations
Open GitHub     [Ctrl+Alt+G] [Reset]
Open Jira       [Unbound]
               [Reset] [Save changes]
```

UI-02: Phone, Settings index > Keyboard Shortcuts.

```text
Integrations
Open GitHub
[Ctrl+Alt+G] [Reset]
Open Jira
[Unbound]
  [Reset] [Save changes]
```

UI-03: Conflict and save failure, shared behavior.

```text
Open GitHub [!] Same shortcut as: Open Jira
[Ctrl+Alt+G] [Reset]
Save failed. Draft remains editable.
[Reset] [Save changes]
```

The settings page owns scrolling; floating Save retains safe-area and final-row
clearance. Phone controls wrap below labels and have 44px hit targets. Copy and
spacing are illustrative; localize all host text.

## Verification

Run from repository root. Install dependencies once if the worktree is fresh.
Write behavioral tests first and record RED/GREEN evidence using `/tdd` and
`/e2e`. New test paths below are part of this work order.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/keyboard/integration-shortcuts.test.ts lib/keyboard/shortcut-overrides.test.ts lib/keyboard/shortcut-conflicts.test.ts lib/keyboard/plugin-shortcuts.test.ts hooks/use-integration-shortcuts.test.ts hooks/use-app-shortcuts.test.ts hooks/use-plugin-shortcuts.test.ts components/global-commands.test.tsx components/settings/general-settings.test.tsx components/settings/keyboard-shortcuts-card.test.tsx components/settings/plugins/plugin-shortcuts-card.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint --max-warnings 0 lib/keyboard hooks/use-integration-shortcuts.ts hooks/use-integration-shortcut-entries.ts hooks/use-plugin-shortcuts.ts components/global-commands.tsx components/settings/general-settings.tsx components/settings/keyboard-shortcuts-card.tsx components/settings/plugins/plugin-shortcuts-card.tsx components/settings/use-shortcut-draft.ts)
(cd apps/web && pnpm run i18n:zh-hant && pnpm run i18n:pseudo && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium e2e/tests/settings/keyboard-shortcuts.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/settings/mobile-integration-shortcuts.spec.ts e2e/tests/integrations/mobile-integrations-nav.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Managed E2E commands rebuild production assets and fixture packages. Run them
sequentially; do not overlap suites or override the runner's worker budget.
If helper extraction changes another test suite, add its exact test path to
this block before reporting completion. Record focused desktop and phone
screenshots/geometry evidence during the named E2E runs.

## Files likely touched

- `apps/web/lib/keyboard/integration-shortcuts.ts` and `.test.ts` (new).
- `apps/web/lib/keyboard/core-shortcuts.ts` (shared reserved identities).
- `apps/web/hooks/use-integration-shortcut-entries.ts` (shared reactive catalog).
- `apps/web/lib/keyboard/shortcut-overrides.ts` and `.test.ts`.
- `apps/web/lib/keyboard/plugin-shortcuts.ts` and `.test.ts`.
- `apps/web/lib/keyboard/shortcut-conflicts.test.ts`.
- `apps/web/hooks/use-integration-shortcuts.ts` and `.test.ts` (new).
- `apps/web/hooks/use-plugin-shortcuts.ts` and `.test.ts` (shared reserved logic).
- `apps/web/components/global-commands.tsx`.
- `apps/web/components/settings/general-settings.tsx` and `.test.tsx`.
- `apps/web/components/settings/keyboard-shortcuts-card.tsx` and `.test.tsx`.
- `apps/web/components/settings/plugins/plugin-shortcuts-card.tsx` and `.test.tsx`.
- `apps/web/components/settings/use-shortcut-draft.ts` (shared draft hook moved from the plugin editor).
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko,pseudo}/settings.json`.
- `apps/web/e2e/tests/settings/keyboard-shortcuts.spec.ts`.
- `apps/web/e2e/tests/settings/mobile-integration-shortcuts.spec.ts` (new).
- `apps/web/e2e/fixtures/plugins/prompt-history-plugin/bundle.js` and its generated
  `apps/backend/cmd/plugin-fixture/fixture-package/ui/bundle.js` for the integration
  nav fixture; keep host and SDK contracts intact.
- `docs/public/sessions-and-review.md`; `docs/screenshots.md` only if captions need updating.

## Dependencies

None. Inspect existing plugin fixture registrations before adding fixture data.
No native workers are authorized; implement in the primary session.

## Risks

- Capture ordering during recording; distinguish app navigation from plugin actions.
- Empty-key sentinels fail backend validation; delete default-unbound overrides.
- Save rebasing and stale responses must retain unrelated entries and pending edits.
- Narrow phone controls and floating Save must not clip the final row.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/integration-page-shortcuts.md), all criteria.
- [System design](../../specs/ui/system-design/integration-page-shortcuts.md), all sections.
- `apps/web/AGENTS.md`; `/tdd`, `/e2e`, `/mobile-parity`, and `/docs-maintainer`.
- Existing shortcut, plugin-editor save, mobile settings-index, and plugin upload tests.
- ADR 0041 and Settings Manual Save references linked by the design.

## Results

Completed on 2026-10-06 in the primary session after the user's implementation
request. All eight acceptance criteria are covered by the focused catalog,
settings, dispatcher, packaged-plugin, desktop and phone tests.

- Behavioral RED preceded catalog/dispatch and editor implementation. The final
  targeted Vitest run passed 114 tests across 11 files; the added workspace-switch
  case passed in the 18-test dispatcher suite (115 distinct tests).
- Shared `useShortcutDraft` keeps per-key rebasing, failed-save drafts, pending
  edits and stale-response guards for both editors. A separate RED regression
  confirmed final-binding reset with an omitted empty response map; the fix and
  plugin/editor regressions passed (29 tests).
- Typecheck and the listed production ESLint passed. Prettier, Traditional Chinese
  conversion, pseudo generation, `i18n:check` and `i18n:ratchet` passed.
- `mise exec -- scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --project chromium e2e/tests/settings/keyboard-shortcuts.spec.ts`:
  9 passed; log `/tmp/kandev-run.e2e.y52B3hTd.log`.
- The added resize scenario ran with the same production assets and only test
  changes: `mise exec -- scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --project chromium e2e/tests/settings/keyboard-shortcuts.spec.ts --grep 'keeps a draft through phone and desktop boundaries'`:
  1 passed; log `/tmp/kandev-run.e2e.JlDwjHkv.log`.
- `mise exec -- scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --project mobile-chrome e2e/tests/settings/mobile-integration-shortcuts.spec.ts e2e/tests/integrations/mobile-integrations-nav.spec.ts`:
  2 passed; log `/tmp/kandev-run.e2e.Aa5Vti2s.log`.
- Both full managed E2E runs rebuilt backend, web and fixture package. Runs were
  sequential with one worker. Phone rotation and 767/768/1280px desktop boundary
  checks preserve draft state, measure touch dimensions and desktop density,
  assert no horizontal overflow and prove bottom-row Save clearance. Screenshots
  were inspected and retained locally at `/tmp/kandev-integration-shortcuts-phone.png`
  and `/tmp/kandev-integration-shortcuts-desktop.png`.
- Public-doc unit tests passed (62); public-doc validation passed (47 pages).
  `validateCoverage` passed for the actual changed-file set. Specification catalog
  validation (357 decisions, 1410 specs), specification lint and `git diff --check`
  passed.

The shell uses the installed toolchain via `mise exec --`; the verification block
above assumes Node/pnpm are already on PATH. The UI matches the reviewed previews.
No backend contract, plugin SDK or provider connection behavior changed. Work is
complete; the user subsequently requested a PR, authorizing commit and publication.

PR preparation fixed test-only hook lint findings by splitting dispatcher test
groups and naming a repeated shortcut identity. The affected dispatcher and
settings suites were rerun, and the commit retains all normal hooks.


## PR review remediation

The user requested remediation of PR #4274. Seven findings receive code or
coverage changes; the optional runtime display-name suggestion is documented:
settings supply installed display names, while dispatch consumes stable IDs and
hrefs without an additional plugin fetch.

- AC .2: repeated plugin integration identities retain the first eligible
  registration and matching label/path.
- AC .5/.6: legacy Tab and Shift+Tab bindings preserve focus; integration
  recording ends without capturing them. Ctrl/Cmd+Shift+P remains reserved for
  the command panel in both integration and plugin dispatch.
- AC .8: recorder descriptions expose the current localized binding and a
  polite, atomic live status announces changes.
- Unbound values are rejected by validation, with direct validator tests and
  distinct unbound/malformed dispatcher cases. Save tests arm the causal
  shortcut PATCH before clicking and use the default UI assertion timeout.

Behavioral RED reproduced the missing keyboard guards, duplicate identities
and recorder accessibility before the production fixes. Focused Vitest passes 132 tests across 12 files;
typecheck, targeted ESLint, formatting and localization checks pass. Public-doc
validation (62 tests and 47 pages), delivery-package coverage, specification
catalog/lint and whitespace checks pass. Managed desktop E2E passes all 11 tests
(log `/tmp/kandev-run.e2e.rj5NVUrf.log`); mobile E2E passes both tests
(log `/tmp/kandev-run.e2e.aPm9QrAk.log`). The desktop run rebuilt production
assets; the sequential mobile run reused those unchanged assets with
`--no-build`. Existing rotation, touch sizing, Save clearance and responsive
boundary assertions remain covered. Exact-head remote CI/review and current-base
merge-result verification are externally pending until the remediation push.


### Merge verification follow-up (2026-10-07)

The mobile browser test initially failed to navigate after pressing the saved
shortcut immediately after a document navigation to Home. It now waits for
the real Open navigation menu control and focuses that non-editable control
before pressing the shortcut. This establishes page readiness and preserves
the global shortcut's editable-target suppression. All saved-setting, reset,
touch-target and viewport-containment assertions remain unchanged.

Three first attempts passed with
`E2E_PORT_OFFSET=0 pnpm --dir apps/web e2e:run --host --no-build --shards 1
--project mobile-chrome tests/settings/mobile-integration-shortcuts.spec.ts --
--retries=0 --repeat-each=3` from the root. Fresh hosted verification remains
pending.
