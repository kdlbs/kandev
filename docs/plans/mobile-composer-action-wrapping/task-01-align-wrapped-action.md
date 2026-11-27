---
id: "01-align-wrapped-action"
title: "Align wrapped workflow action"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-COMPOSER-ACTION-WRAP-001
acceptance_criteria:
  - AC-UI-COMPOSER-ACTION-WRAP-001.1
  - AC-UI-COMPOSER-ACTION-WRAP-001.2
  - AC-UI-COMPOSER-ACTION-WRAP-001.3
  - AC-UI-COMPOSER-ACTION-WRAP-001.4
  - AC-UI-COMPOSER-ACTION-WRAP-001.5
system_design:
  - ../../specs/ui/system-design/composer-action-wrapping.md
---

# Task 01: Align Wrapped Workflow Action

## Historical placement revision

This completed work order records the original below-utilities composition.
The 2026-10-14 user correction changes phone placement; its implementation and
UI-02 preview are owned by [Task 02](task-02-place-action-above-utilities.md).
Results and UI-01 below are evidence of the earlier revision, not acceptance
of the new upper workflow row.

## Summary

Keep the next-step control right-aligned when it wraps below transcript utilities.
Use a browser geometry regression as the TDD red gate before the minimal layout
change, then prove the wrapped action still moves the task.

## In scope

`ChatStatusBarActions` layout and the plan's focused mobile/desktop geometry
scenarios. Keep component tests unless a meaningful behavioral adjustment is
needed. Record red/green evidence and compare rendered layout with UI-01.
The workflow action must also meet the shared 44px phone target contract with
either pointer, retaining its compact 24px height on fine-pointer desktop.

## Out of scope

Workflow behavior, icon reorganization, passthrough layout, input controls,
backend code, and localization changes.

## Acceptance

1. A genuine wrapped phone action aligns to the toolbar's content right edge
   within 1px and meets the containment and touch checks in the linked criteria.
2. Tapping it performs the existing destination move; current visibility and
   options behavior continue to pass their focused tests.
3. Fine-pointer phone widths and wide desktop pass the plan's layout cases.

## ASCII UI preview

UI-01 excerpt from [the plan](plan.md#ascii-ui-preview), covering
AC-UI-COMPOSER-ACTION-WRAP-001.1 through .5:

```text
Phone, crowded:
| [transcript icons...................] |
|                         [Open PR ->] |
| [Composer...........................] |

Desktop, sufficient width:
| [status]       [transcript icons] [Open PR ->] |
```

Toolbar stays above the composer outside transcript scrolling. Right alignment
and order are structural; labels and spaces are illustrative. A hidden action
has no reserved line, and the moving state retains its current disabled control.

## Verification

Run from repository root. The managed E2E runner rebuilds production assets.
Run its two project commands sequentially.

```bash
(cd apps/web && pnpm exec vitest run components/task/chat/chat-status-bar.test.tsx components/task/chat/chat-input-area.test.tsx components/task/workflow-move-proceed-button.test.tsx)
(cd apps/web && pnpm exec eslint components/task/chat/chat-status-bar.tsx e2e/tests/workflow/mobile-workflow-step-move-overrides.spec.ts e2e/tests/workflow/workflow-step-move-overrides.spec.ts e2e/tests/workflow/composer-action-layout-helpers.ts e2e/tests/workflow/workflow-step-move-overrides-helpers.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/workflow/mobile-workflow-step-move-overrides.spec.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/workflow/workflow-step-move-overrides.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Install workspace dependencies first if executing in a fresh worktree.

## Files likely touched

- `apps/web/components/task/chat/chat-status-bar.tsx`
- `apps/web/e2e/tests/workflow/mobile-workflow-step-move-overrides.spec.ts`
- `apps/web/e2e/tests/workflow/workflow-step-move-overrides.spec.ts`
- `apps/web/e2e/tests/workflow/workflow-step-move-overrides-helpers.ts` for disposable fixture inputs.
- `apps/web/e2e/tests/workflow/composer-action-layout-helpers.ts` for rendered geometry checks.

## Dependencies

None. Existing workflow fixtures and passthrough layout are reference inputs.

## Risks

Test setup must reveal actual utility controls and prove line separation. Restore
modified user settings. Geometry should measure the toolbar content edge, with
padding accounted for, rather than treating visibility as alignment evidence.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/composer-action-wrapping.md)
- [Design](../../specs/ui/system-design/composer-action-wrapping.md)
- Existing mobile direct-move test and passthrough status action group.

## Results

Completed on 2026-10-09. `ChatStatusBarActions` now uses bounded end justification.
UI-01 matches the rendered phone screenshot. Actual wrapping is proven at 360px
and 393px before right-edge measurement; the longer fitting label retains a 44px
hit target and the tap commits the destination step. Fine-pointer phone alignment,
767/768px alignment, and wide desktop single-line order also passed.

Commands ran from repository root with isolated directory changes as shown in
Verification. E2E commands added `--host` and `--retries=0`; the local shell needed
`PATH=/usr/local/go/bin:$PATH`. Both final full-suite commands rebuilt production
assets and ran sequentially with one worker.

| Check | Result |
| --- | --- |
| `pnpm install --frozen-lockfile` from `apps/` | Passed; fresh workspace dependencies installed |
| Mobile regression before production edit: managed mobile runner with `--grep 'wrapped mobile workflow' --retries=0` | RED: 138.171875px right-edge gap, expected at most 1px |
| Vitest command in Verification | 3 files, 60 tests passed after production change |
| ESLint command in Verification | Passed for production file, both specs, and both helpers |
| Prettier check for the same five changed TS/TSX files | Passed |
| `pnpm e2e:run --host --project mobile-chrome tests/workflow/mobile-workflow-step-move-overrides.spec.ts -- --retries=0` | 4 passed in 29.3s |
| `pnpm e2e:run --host --project chromium tests/workflow/workflow-step-move-overrides.spec.ts -- --retries=0` | 6 passed in 44.6s |
| Playwright discovery with chromium, mobile-chrome, and containers projects | Zero discovery errors |
| `python3 scripts/list-docs.py validate` | Passed: 368 decisions, 1495 specifications |
| `python3 scripts/lint-spec-files.py --all` | Passed |
| `.github/scripts/pr-docs.cjs` `validateCoverage` on the delivery package and changed paths | Covered; zero errors |
| `git diff --check` | Passed |

The 768px workbench can have a narrow chat pane; its test uses a shorter fitting
label and allows wrapping while requiring right alignment. Phone wrap cases use
the longer label. No workflow logic or persisted production state changed.

Review follow-up: documented the wrapped-line invariant and browser-helper
contracts in response to CodeRabbit's docstring-coverage warning. This changes
comments only; the rendered layout and test behavior retain the results above.
ESLint and Prettier passed for the three affected TypeScript files, and spec
validation and diff checks passed. Remote CI and reviewer verification remain
pending for the follow-up commit.

Further review follow-up: CodeRabbit identified the fine-pointer phone action's
24px height. The extended 360px browser case failed with an expected minimum of
44px and an actual height of 24px before the production edit. This caller now
applies a 44px minimum height and width below 768px. The geometry check asserts
44px targets for fine-pointer phones, 24px desktop height, and center-point
reachability. The fine-pointer test also clicks the wrapped action at 360px and
verifies its move request and committed destination.

Post-fix validation passed: the same three Vitest suites passed 60 tests;
`pnpm e2e:run --host --project chromium tests/workflow/workflow-step-move-overrides.spec.ts -- --retries=0`
passed all 6 tests in 45.9s, and the mobile-chrome command with `--no-build`
passed all 4 tests in 28.8s. Mobile reused the production assets rebuilt by the
desktop command. ESLint and Prettier passed for the affected production file,
geometry helper, and desktop spec; catalog validation, spec lint, and diff checks
passed. The system design and plan now explicitly cover the fine-pointer phone
target. Fresh desktop, mouse-phone, and touch-phone screenshots are captured for
PR publication. Remote CI and review completion remain pending for the new head.
The touch and mouse regression cases also describe their behavioral contracts
in addition to their acceptance-criterion tags. ESLint and Prettier passed after
these comment-only additions; the production implementation and browser results
above are unchanged.


### Current-main reconciliation and CI remediation

Resumed on 2026-10-14. The previous current-head CI run completed with two
flaky tests: the agents fallback-summary case assumed the first provider owned
a profile, and session-tab Rename lost its input to menu focus restoration.
Neither failure is waived by the retry passing.

Preserved the landed mobile action bar's flattened phone wrappers, trailing
workflow button, and contained label wrapping, adding this package's 44px
minimum width. Geometry now measures the union of visible utility controls;
the old wrapper-box assertion failed on the merged phone layout. The desktop
regression captures fresh mouse-phone and desktop screenshots.

The fallback-summary fixture now selects the owner and profile by the seeded
profile ID. The shared session menu hands Rename off after close-autofocus,
following the terminal menu's existing pattern. Its real-menu component
regression failed before the fix with one premature blur commit, then passed
with no commit and a focused editor; ordinary dismissal retains focus return.
The tab reconciliation design and its work order record that existing-contract
repair. Public docs need no change: existing Rename and workflow actions retain
the same entry points, semantics, and terminology.

Post-remediation commands (from repository root):

- `pnpm --dir apps/web exec vitest run components/task/session-tab-menu.test.tsx components/task/preview-session-tabs.test.tsx`: 45 passed.
- The three Vitest suites in Verification: 60 passed.
- `PATH=/usr/local/go/bin:$PATH scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --project chromium e2e/tests/workflow/workflow-step-move-overrides.spec.ts e2e/tests/settings/agents-browse-page.spec.ts e2e/tests/session/session-tab-rename.spec.ts -- --retries=0`: 9 passed (1.2m).
- Focused ESLint for the changed component, menu, tests, and geometry helper: passed.
- Harness validation: 19 harness tests, all 204 files, and 36 spec-linter tests passed.
- Catalog validation: 381 decisions and 1594 specifications; full spec lint passed.

- `PATH=/usr/local/go/bin:$PATH scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --no-build --project mobile-chrome e2e/tests/workflow/mobile-workflow-step-move-overrides.spec.ts e2e/tests/chat/mobile-chat-status-bar.spec.ts -- --retries=0`: 8 passed (1.1m), including the landed phone/coarse-tablet action bar scenarios.
- Prettier formatted the geometry helper and screenshot call; remaining changed TypeScript files were already formatted.
- The required staged Go format check identified four incoming base files; normalized indentation with `gofmt`, with an empty whitespace-insensitive diff against main. No Go logic changed.

Fresh screenshot publication and remote CI/review completion remain pending
for the new head. Strict whitespace checks on the incoming main
delta identify pre-existing CRLF audit CSVs; the PR delta against main is clean.
