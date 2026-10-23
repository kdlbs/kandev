---
id: "01-shared-colors"
title: "Correct shared action colors"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-BUTTON-STATE-COLORS-001
acceptance_criteria:
  - AC-UI-BUTTON-STATE-COLORS-001.1
  - AC-UI-BUTTON-STATE-COLORS-001.2
  - AC-UI-BUTTON-STATE-COLORS-001.3
  - AC-UI-BUTTON-STATE-COLORS-001.4
  - AC-UI-BUTTON-STATE-COLORS-001.5
  - AC-UI-BUTTON-STATE-COLORS-001.6
system_design:
  - ../../specs/ui/system-design/button-state-colors.md
---

# Task 01: Correct Shared Action Colors

## Outcome

Implement the [design](../../specs/ui/system-design/button-state-colors.md) and
the explicit exceptions in the [inventory](inventory.md). Keep all existing
action predicates, native disabled semantics, labels, and geometry.

## UI preview

Use UI-01 and UI-02 in the [plan](plan.md#ui-preview). Unavailable primary
actions use a neutral surface and readable foreground. Available actions keep
their color and consistent label/icon/shortcut foreground. Phone controls stay
in their existing row. Granted permission and completed settings Save keep green status treatment;
Canvas sidebar setup uses neutral navigation styling.

## Files

- `apps/packages/ui/src/button.tsx`: default disabled variant only.
- `apps/web/components/task/markdown-file-toolbar.tsx`.
- `apps/web/components/editors/monaco/monaco-editor-toolbar.tsx`.
- `apps/web/components/editors/codemirror/codemirror-editor-toolbar.tsx`.
- `apps/web/components/task/chat/chat-input-toolbar-primitives.tsx`: spinner inheritance.
- `apps/web/components/settings/notification-permission-section.tsx`: granted-state exception.
- `apps/web/components/settings/settings-floating-save.tsx`: completed Saved-state exception.
- `apps/web/components/canvas/canvas-task-create-launcher.tsx`: sidebar variant.
- Existing Markdown desktop/mobile E2E suites and new
  `e2e/tests/settings/button-state-colors.spec.ts` plus
  `e2e/tests/settings/mobile-button-state-colors.spec.ts` for representative
  settings controls, notification status, and color variants.
- `e2e/helpers/button-state-colors.ts` and settings color helpers for rendered
  token, contrast, and request-barrier checks.
- `e2e/tests/pr/pr-detail-layout.spec.ts`: real pending Approve and Merge
  states, including the dark custom-color cascade.
- Component tests for changed notification/Canvas variant branches as needed;
  do not substitute class-string tests for rendered CSS checks.

## Regression sequence and acceptance

1. Add a rendered regression that fails on current disabled Save opacity/fill,
   mismatched shortcut foreground, and sidebar Canvas variant. Capture light
   and dark screenshots. Check a real disabled non-editor action to establish
   shared reach. Unit tests cover changed variant/status branches, not CSS math.
2. Apply the shared repair and the listed caller changes. Confirm each of the
   seven custom-colored consumers has its intended enabled/disabled treatment.
   Granted permission and completed Saved status are the listed disabled-color
   exceptions; pending/invalid actions remain neutral.
3. Verify clean, dirty, and pending Save states using existing fixture/event
   helpers. Assert exact source preservation, disabled activation prevention,
   contrast target, inherited foregrounds, and existing desktop/phone geometry.
   Use 767px/768px boundary cases. Run checks below; record actual results.

## Verification commands

Run from repository root with pinned pnpm. Existing Nix/browser wrappers may be
used as documented in the Markdown walkthrough work order. Build after CSS or
component changes. Browser runs are serial and use one worker.

```bash
corepack pnpm@9.15.9 --dir apps/web exec vitest run components/task/markdown-file-editor.test.tsx components/task/mobile/mobile-file-viewer-panel.test.tsx components/settings/notification-permission-section.test.tsx components/settings/settings-save-provider.test.tsx components/task/chat/chat-input-toolbar.test.tsx components/task/chat/chat-input-toolbar-composer.test.tsx components/canvas/canvas-task-create-launcher.test.tsx
corepack pnpm@9.15.9 --dir apps/web run typecheck
corepack pnpm@9.15.9 --dir apps/web run i18n:check
corepack pnpm@9.15.9 --dir apps/web e2e:run --host --shards 1 --project chromium tests/task/markdown-file-editing.spec.ts tests/settings/button-state-colors.spec.ts
corepack pnpm@9.15.9 --dir apps/web e2e:run --host --shards 1 --no-build --project mobile-chrome tests/task/mobile-markdown-file-editing.spec.ts tests/settings/mobile-button-state-colors.spec.ts
corepack pnpm@9.15.9 --dir apps/web e2e:run --host --shards 1 --no-build --project markdown-firefox tests/task/markdown-file-editing.spec.ts --grep 'button colors'
corepack pnpm@9.15.9 --dir apps/web e2e:run --host --shards 1 --no-build --project markdown-mobile-webkit tests/task/mobile-markdown-file-editing.spec.ts --grep 'button colors'
corepack pnpm@9.15.9 --dir apps/web e2e:run --host --shards 1 --no-build --project chromium tests/pr/pr-detail-layout.spec.ts --grep 'button colors'
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/button-state-colors
```

Name new Markdown color scenarios with `button colors` so the focused browser
commands select them. Add any new component-test path to the unit command before
completion. Run ESLint and Prettier on the final changed source/test inventory,
and the documentation-coverage preflight on this work order and its references.

## Dependencies and exclusions

Sequential. Preserve all uncommitted browser and walkthrough fixes. No engine,
package, persistence, palette, raw-button, or interaction redesign. No new copy.
Do not refresh the user's evaluation build until the implemented checks pass;
retain fixture files/database and never operate on the main instance.

## Results

Implementation and final runtime verification completed on 2026-10-23.

- RED: real Chromium Save failed on opacity 0.45. A second rendered PR test
  failed on the custom-green dark cascade before combined dark-disabled tokens.
- Production: eight small presentation changes; action predicates, geometry,
  existing non-default variants, and enabled custom colors are unchanged.
- Units: seven files, 108 passing tests. Typecheck and i18n checks passed.
- Static checks: changed production files pass ESLint and Prettier; E2E files
  have no lint errors and retain two existing nested-ternary warnings in the
  Markdown corpus scenario. Whitespace, catalog, spec lint, and documentation
  coverage passed.
- Browser evidence: desktop Markdown 15/15, desktop settings 8/8, GitHub
  custom-green actions 1/1, and phone Markdown 9/9 passed. Focused Firefox Save
  passed. Phone settings passed 8/8 and focused WebKit Save passed 1/1.
  These are 43 distinct browser cases across full suites and focused reruns.

The initial settings response hold allowed a server settings event to settle the
form before pending assertions; the fixture now holds request admission. The PR
merge fixture was corrected to its actual PUT method. Phone Canvas coverage
follows its native navigation link to settings; the neutral desktop sidebar
button is not rendered by the phone menu. These are test-fixture corrections,
not additional product changes.

Source-audit coverage is broader than rendered coverage. GitLab approval and
plan Send/Start were checked through their shared component and unchanged
consumer classes; no separate live-state claim is made for them. Native
Zen/macOS, OS IME, real phone keyboard, and assistive-technology checks remain
manual in the existing guide.

Final browser logs:

- Desktop Markdown: `/tmp/kandev-run.e2e.TvT67dnJ.log`.
- Desktop settings: eight passes in `/tmp/kandev-run.e2e.kB8ZSYHw.log`;
  corrected PR transport fixture: `/tmp/kandev-run.e2e.N1TYj7cS.log`.
- Phone Markdown: nine passes in `/tmp/kandev-run.e2e.3uGVGaIZ.log`;
  corrected native-phone settings suite: `/tmp/kandev-run.e2e.6aZMmk7w.log`.
- Firefox Save: `/tmp/kandev-run.e2e.0nOlGAUe.log`.
- WebKit phone Save: `/tmp/kandev-run.e2e.vcD46U66.log`.

The final application build is `/tmp/kandev-run.build.oqOTgCXm.log`. Only two
E2E-only files changed afterward to correct phone Canvas coverage; compiled
assets were unchanged. The final source manifest includes those corrected tests.

Live verification: the refreshed isolated instance passed eight clean/unsaved
Save checks in Firefox desktop and WebKit phone, both themes, and two real
unsupported-notification checks in WebKit. Clean contrast: 4.35:1 light,
4.05:1 dark. Dirty contrast: 5.58:1 light, 3.90:1 dark. Icons and desktop shortcut
inherit the button foreground. No file-save request occurred. All 66 fixture
hashes, the evaluation database, and main PID 1155/start time were preserved.
Evaluation PID: 1126364. Full evidence is
`/tmp/kandev-pr3113-evaluation-4e7mws9y/button-colors-runtime-smoke.json`.
The [plan](plan.md) records the exact asset/source fingerprints. All changes
remain uncommitted on the existing branch; `TESTING-PR3113.md` remains untracked.
