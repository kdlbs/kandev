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

## Summary

Keep the next-step control right-aligned when it wraps below transcript utilities.
Use a browser geometry regression as the TDD red gate before the minimal layout
change, then prove the wrapped action still moves the task.

## In scope

`ChatStatusBarActions` layout and the plan's focused mobile/desktop geometry
scenarios. Keep component tests unless a meaningful behavioral adjustment is
needed. Record red/green evidence and compare rendered layout with UI-01.

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
