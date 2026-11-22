---
id: "01-hide-unavailable-cards"
title: "Hide unavailable summary cards and prove history continuity"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-TURN-CHANGES-005
  - REQ-TASKS-TURN-CHANGES-006
acceptance_criteria:
  - AC-TASKS-TURN-CHANGES-005.1
  - AC-TASKS-TURN-CHANGES-005.6
  - AC-TASKS-TURN-CHANGES-005.7
  - AC-TASKS-TURN-CHANGES-005.9
  - AC-TASKS-TURN-CHANGES-005.12
  - AC-TASKS-TURN-CHANGES-005.13
  - AC-TASKS-TURN-CHANGES-006.2
  - AC-TASKS-TURN-CHANGES-006.7
system_design:
  - ../../specs/tasks/system-design/turn-changed-files.md
  - ../../specs/tasks/system-design/turn-change-history-integrity.md
---

# Task 01: Hide unavailable summary cards and prove history continuity

## Summary

Change the shared transcript eligibility rule and update its regression evidence.
Keep retained history selection and usable partial summaries available on desktop and phone.

## In scope

- Own `isTurnChangeCardVisible`, its focused unit tests, and history-selection regression cases.
- Replace desktop conflict and branch-switch expectations that require an unavailable card.
- Add phone conflict coverage to the existing mobile history-integrity suite.
- Preserve backend availability assertions and the mixed-result navigation test.
- Update the public changed-files how-to when the behavior is implemented.
- Record command results and complete the plan lifecycle after the checks pass.

## Out of scope

Backend files, capture state, history projection semantics, component styles, new copy, and unrelated refactors.
Do not remove `isUnavailableHistoryComparison` or unavailable viewer diagnostics.

## Acceptance

1. The shared predicate excludes unavailable and failed summaries, regardless of reason or retained counts, at all existing transcript anchors.
2. Desktop and phone prove the target card stays absent after settlement and reload while the reply remains visible.
3. Focused evidence preserves partial cards, pending/expired behavior, and explicit unavailable history selection. Public instructions match the result.

## ASCII UI preview

Reuse [UI-01 and UI-02 in the plan](plan.md#ascii-ui-preview).

```text
UI-01, desktop and phone:
[Final assistant reply]
[Next message]
```

No unavailable card, footer, action, or reserved spacing appears.
The transcript retains its existing scroll owner and message spacing.
Map: AC-TASKS-TURN-CHANGES-005.1, .7, .9, and .13.
UI-02 retains the current phone card and full-height `MobileDiffSheet` with its existing touch targets and safe areas.

## TDD

First add visibility regressions for history-unavailable and failed summaries, including retained file counts.
Run the focused unit command and record the behavioral RED before the production change.
Then change only transcript eligibility and rerun the unit checks.
Add history-selection coverage and update browser expectations. Run the managed browser checks against the fresh build.

## Verification

Run from the repository root. Install workspace dependencies once before the first package command in a fresh worktree.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/turn-changes/projection.test.ts lib/turn-changes/history-scope.test.ts components/task/historical-turn-diff-viewer-controls.test.tsx)
(cd apps/web && pnpm exec eslint --max-warnings 0 lib/turn-changes/projection.ts lib/turn-changes/projection.test.ts lib/turn-changes/history-scope.test.ts e2e/tests/git/turn-changes-history-conflict.spec.ts e2e/tests/git/turn-changes-title-rename.spec.ts e2e/tests/git/mobile-turn-changes-history-integrity.spec.ts)
(cd apps/web && pnpm exec playwright test --config e2e/playwright.config.ts --project=chromium e2e/tests/git/turn-changes-history-conflict.spec.ts e2e/tests/git/turn-changes-title-rename.spec.ts e2e/tests/git/turn-changes-history-mixed.spec.ts --list)
(cd apps/web && pnpm exec playwright test --config e2e/playwright.config.ts --project=mobile-chrome e2e/tests/git/mobile-turn-changes-history-integrity.spec.ts --list)
(cd apps/web && pnpm e2e:run --project chromium tests/git/turn-changes-history-conflict.spec.ts tests/git/turn-changes-title-rename.spec.ts tests/git/turn-changes-history-mixed.spec.ts)
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/git/mobile-turn-changes-history-integrity.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

The first managed E2E command builds the frontend and backend. Run the phone command afterward without intervening production changes.
Record discovery counts, browser results, and rendered desktop/phone comparison with UI-01.
Run the coverage preflight from the repository root:

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const order = 'docs/plans/hide-unavailable-turn-changes/task-01-hide-unavailable-cards.md';
const paths = [
  order,
  'docs/plans/hide-unavailable-turn-changes/plan.md',
  'docs/specs/tasks/requirements/turn-changed-files-presentation.md',
  'docs/specs/tasks/system-design/turn-changed-files.md',
  'docs/specs/tasks/system-design/turn-change-history-integrity.md',
];
const fileContents = Object.fromEntries(paths.map(file => [file, fs.readFileSync(file, 'utf8')]));
const result = validateCoverage({
  changedFiles: ['apps/web/lib/turn-changes/projection.ts', order],
  fileContents,
});
console.log(JSON.stringify({ status: result.status, errors: result.errors }));
if (!result.ok) process.exitCode = 1;
NODE
```

The preflight must accept this work order and both system-design references.

## Files likely touched

- `apps/web/lib/turn-changes/projection.ts`
- `apps/web/lib/turn-changes/projection.test.ts`
- `apps/web/lib/turn-changes/history-scope.test.ts`
- `apps/web/e2e/tests/git/turn-changes-history-conflict.spec.ts`
- `apps/web/e2e/tests/git/turn-changes-title-rename.spec.ts`
- `apps/web/e2e/tests/git/mobile-turn-changes-history-integrity.spec.ts`
- `docs/public/tasks-and-workflows.md`
- This work order, its plan, and the linked specification lifecycle fields.

## Dependencies

None. Existing conflict and branch-switch scenarios provide the required backend fixtures.

## Risks

Keep aggregate summary eligibility separate from repository status and content availability.
Keep unavailable history in selectors and viewer controls.
Use the exact change-set identity for negative card assertions and wait for settled backend state.

## Parallelism

`sequential`

## Inputs

- [Presentation requirements](../../specs/tasks/requirements/turn-changed-files-presentation.md), REQ-TASKS-TURN-CHANGES-005 and -006.
- [Core design](../../specs/tasks/system-design/turn-changed-files.md#transcript-projection-and-presentation).
- [History integrity design](../../specs/tasks/system-design/turn-change-history-integrity.md#projection-and-presentation).
- Existing `message-list-native.tsx` predicate call sites, conflict fixtures, and mixed-result tests.

## Results

Implemented. `isTurnChangeCardVisible` now rejects summary availability `unavailable` and `failed` before admitting pending or nonzero counts, so the existing predicate call sites in `message-list-native.tsx` filter reply anchors, fallback rows, and summary admission before `TurnChangedFilesCard` mounts. `isUnavailableHistoryComparison` and `projectTurnChangeSummary` remain for historical viewer diagnostics and explicit/latest history selection. Repository-level availability and usable partial summaries are unchanged.

Tests added or updated:

- `projection.test.ts`: unavailable and failed summaries stay hidden with zero or retained files and any reason; pending, ready, expired, and usable partial summaries retain eligibility; the unavailable history projection still identifies viewer diagnostics.
- `history-scope.test.ts`: latest and explicit unavailable historical selections remain available while the chat card is hidden.
- `turn-changes-history-conflict.spec.ts` (Chromium): waits for the terminal unavailable WS summary, then requires no card for the exact `data-change-set-id` after settlement and reload while the assistant reply remains visible.
- `turn-changes-title-rename.spec.ts` (Chromium): the branch-switch case keeps its API unavailability assertions and now requires an absent card. The shared absence helper waits a negative-assertion dwell before asserting.
- `mobile-turn-changes-history-integrity.spec.ts` (mobile-chrome): adds the conflict scenario, requiring no card before and after reload, the reply visible, and no horizontal document overflow.
- `turn-changes-history-mixed.spec.ts` is unchanged and still proves the usable partial card.
- `docs/public/tasks-and-workflows.md` now explains that unavailable or failed summaries have no card and that partial summaries with usable changes remain.

Verification executed from the repository root, with package commands run from `apps/web` as shown:

- `pnpm exec vitest run lib/turn-changes/projection.test.ts lib/turn-changes/history-scope.test.ts components/task/historical-turn-diff-viewer-controls.test.tsx`: 35 passed.
- Turn-change-related Vitest sweep across 12 files: 86 passed.
- ESLint on all changed TypeScript files: zero warnings.
- `tsc --noEmit` and `rich-output:check`: passed with `NODE_OPTIONS=--max-old-space-size=8192`; the default heap OOMs in this container.
- Playwright discovery: 5 Chromium tests in 3 files; 3 mobile-chrome tests in 1 file.
- `pnpm e2e:run --project chromium ...`: 5 passed.
- `pnpm e2e:run --no-build --project mobile-chrome ...`: 3 passed.
- Captured desktop and phone UI-01 screenshots with `CAPTURE_PR_ASSETS=1`: the earlier healthy card remains, the reply is visible, and no unavailable card, footer, action, or reserved spacing follows it.
- `node --test scripts/validate-public-docs.test.mjs`: 63 passed. `node scripts/validate-public-docs.mjs`: 54 pages. `python3 scripts/list-docs.py validate`: 425 decisions and 1885 specifications. `python3 scripts/lint-spec-files.test.py`: 36 passed. `python3 scripts/lint-spec-files.py --all`, the coverage preflight, and `git diff --check`: passed.

### Review remediation (2026-11-02)

Addressed the current-head review findings. The superseded packages now describe this visibility package as completed. The public how-to separates usable partial cards from hidden unavailable or failed summaries and keeps expiry guidance in its own paragraph. The core design drops the failure-row anchor wording. The phone conflict scenario resets the shared seed checkout before seeding. The linked presentation requirement and core system design advance to `active` and `current`. Re-ran the phone conflict scenario (1 passed), ESLint on the changed spec, and the public-doc/specification validations.
