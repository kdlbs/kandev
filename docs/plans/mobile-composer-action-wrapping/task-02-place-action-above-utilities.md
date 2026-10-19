---
id: "02-place-action-above-utilities"
title: "Place workflow action above utilities"
status: done
wave: 2
depends_on:
  - "01-align-wrapped-action"
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

# Task 02: Place Workflow Action Above Utilities

## Summary

Give the phone next-step action a right-aligned row above the floating transcript
utility icons. The prior implementation right-aligns it below those icons because
utility children precede it in the flattened flex flow. Update the existing
rendered oracle first; its revised line-order assertion must fail on that layout.

## In scope

The responsive action/utility composition in `ChatStatusBarActions` and
`ChatStatusBarRightControls`, its existing phone/mouse geometry regressions,
and meaningful order/state coverage. Compare rendered results with UI-02.
After checks, refresh PR screenshots and continue the already authorized
commit/push/PR workflow, preserving unrelated live PR body sections.

## Out of scope

Passthrough, status/notice semantics, icon order or overflow menus, composer input
controls, workflow/backend logic, new copy, and persistence. Do not merge the PR.

## Acceptance

1. On phones with touch or fine pointers, the sole workflow action is entirely
   above every visible utility and aligned within 1px of the tray's content right
   edge. Dense/long and sparse/short cases meet the size/containment criteria.
2. Hidden and moving states allocate the specified rows; keyboard order matches
   visual order, resize preserves component identity, and tap/click performs the
   existing move with the same preview/options and eligibility behavior.
3. Desktop inline/trailing-wrap behavior, compact sizing, and existing terminal
   notice/coarse-tablet scenarios pass. Fresh PR screenshots show UI-02.

## ASCII UI preview

UI-02 excerpt from [the plan](plan.md#ascii-ui-preview), covering
AC-UI-COMPOSER-ACTION-WRAP-001.1 through .5:

```text
Phone before:
| [transcript utility icons...........] |
|                         [Open PR ->] |
| [Composer...........................] |

Phone after:
|                         [Open PR ->] |
| [transcript utility icons...........] |
| [Composer...........................] |

Hidden:  utility row -> composer (no empty workflow row)
Moving:  disabled upper action -> utility row -> composer

Desktop, sufficient width:
| [status]       [transcript icons] [Open PR ->] |
| [Composer..................................] |
```

The upper phone action row is structural, even when one line could fit. Keep
existing notices/status information before the action group and all rows outside
transcript scrolling. Labels and spacing are illustrative; retain shared sizes,
safe-area clearance, and complete-control wrapping.

## Verification

From repository root, run the phone line-order regression before production
changes and record its expected failure. The managed runner builds the assets.
Use bounded Go build resources and run browser projects sequentially.

```bash
PATH=/usr/local/go/bin:$PATH GOMEMLIMIT=3GiB GOMAXPROCS=2 GOFLAGS=-p=2 scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --project mobile-chrome e2e/tests/workflow/mobile-workflow-step-move-overrides.spec.ts -- --grep 'wrapped mobile workflow' --retries=0
```

After implementation, run:

```bash
pnpm --dir apps/web exec vitest run components/task/chat/chat-status-bar.test.tsx components/task/chat/chat-input-area.test.tsx components/task/workflow-move-proceed-button.test.tsx
pnpm --dir apps/web exec eslint components/task/chat/chat-status-bar.tsx components/task/chat/chat-status-bar.test.tsx e2e/tests/workflow/mobile-workflow-step-move-overrides.spec.ts e2e/tests/workflow/workflow-step-move-overrides.spec.ts e2e/tests/workflow/composer-action-layout-helpers.ts
pnpm --dir apps/web exec prettier --check components/task/chat/chat-status-bar.tsx components/task/chat/chat-status-bar.test.tsx e2e/tests/workflow/mobile-workflow-step-move-overrides.spec.ts e2e/tests/workflow/workflow-step-move-overrides.spec.ts e2e/tests/workflow/composer-action-layout-helpers.ts
PATH=/usr/local/go/bin:$PATH GOMEMLIMIT=3GiB GOMAXPROCS=2 GOFLAGS=-p=2 scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --project chromium e2e/tests/workflow/workflow-step-move-overrides.spec.ts -- --retries=0
PATH=/usr/local/go/bin:$PATH GOMEMLIMIT=3GiB GOMAXPROCS=2 GOFLAGS=-p=2 scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --no-build --project mobile-chrome e2e/tests/workflow/mobile-workflow-step-move-overrides.spec.ts e2e/tests/chat/mobile-chat-status-bar.spec.ts -- --retries=0
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Keep the original regression name through the red gate so its grep selects the
test. The mobile final command reuses the immediately preceding desktop build.
Install dependencies from `apps/` in a fresh worktree. Copy captures before the
next suite overwrites test results. Review all three viewports; publish assets
through `/pr` and rerun required CI/review handling after pushing.

## Files likely touched

- `apps/web/components/task/chat/chat-status-bar.tsx`
- `apps/web/components/task/chat/chat-status-bar.test.tsx`
- `apps/web/e2e/tests/workflow/composer-action-layout-helpers.ts`
- `apps/web/e2e/tests/workflow/mobile-workflow-step-move-overrides.spec.ts`
- `apps/web/e2e/tests/workflow/workflow-step-move-overrides.spec.ts`
- This package's requirement, design, plan, and work-order results.

## Dependencies

Completed Task 01 and existing disposable workflow/history fixtures. Reuse
`useResponsiveBreakpoint`; do not add another viewport/pointer policy.

## Risks

- CSS-only visual reordering can disagree with keyboard traversal. Reorder the
  keyed children using the canonical phone condition and verify focus order.
- Replacing keyed instances or mounting two trees can reset disclosure state or
  duplicate actions. Retain the same parent and child identities across resize.
- A narrow utility cluster can restore overflow corrected on current main. Use
  a full-width wrapping row and retain 320px and coarse-tablet coverage.
- Sparse controls can conceal accidental same-line behavior. Test that case as
  well as dense controls; measure every visible utility.
- The extra phone row reduces transcript space when controls formerly fit.
  This is the requested hierarchy; add no spacer when the action is hidden.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/composer-action-wrapping.md)
- [Design](../../specs/ui/system-design/composer-action-wrapping.md)
- [Existing mobile action-bar design](../../specs/ui/system-design/mobile-task-action-bar.md)

## Results

Implemented on 2026-10-14 following the user's explicit implementation request.
Phone action/control ordering uses the canonical breakpoint and stable keyed
children under one parent. The action group stacks above a full-width wrapping
utility row; desktop retains its existing order and density. Rendered captures
match UI-02 with either pointer, including long labels and sparse controls.

The browser red gate failed on the previous production layout: workflow button
bottom was 660px while the first utility started at 566px. After the layout
change the focused case passed. The updated component regression confirms both
child instances survive repeated responsive reorderings.

| Check | Result |
| --- | --- |
| Three Vitest suites in Verification | 61 passed |
| ESLint and Prettier commands in Verification | Passed |
| Managed Chromium workflow suite, fresh production build | 6 passed, retries disabled |
| Final full Chromium suite with capture enabled and the built assets | 6 passed (59.3s), retries disabled |
| Managed mobile workflow + status-bar suites with built assets | 8 passed (1.1m), retries disabled |
| Final focused desktop capture after removing transient hover/focus chrome | 1 passed (15.6s) |
| Final focused touch-phone capture after the same capture-only adjustment | 1 passed (15.6s) |
| Catalog and full spec lint | 381 decisions and 1596 specifications; passed |
| Prospective complete-PR documentation coverage | Covered, zero errors |
| `git diff --check` | Passed |

The final browser commands use `CAPTURE_PR_ASSETS=1`; focused captures add
`--no-build` and their existing name filters to the corresponding project
command. No production code changed after the successful fresh build. Capture
preparation clears transient tooltip/focus chrome only when capture is enabled.
All browser runs were sequential with one worker and bounded Go build resources.

Mobile coverage verifies upper-row geometry while a real move request is held,
then releases the request and proves the target step and absence of a hidden
workflow spacer. Disabled buttons retain the geometry checks; their deliberate
non-interactive state is excluded from the enabled-button center hit test.
Keyboard traversal, dense/long and sparse/short cases, phone fine-pointer sizing,
767/768px transitions, 320px notice containment, and coarse-tablet cases passed.

Public documentation review found the existing phone-controls guide; added one
sentence describing the revised task Chat row order. No labels, public API,
workflow semantics, or localization values changed. Requirements are active and
the design is current. Task 01 remains the historical alignment delivery record.

Three fresh manifest assets were inspected against UI-02, compressed, and
published in the [native screenshot comment](https://github.com/kdlbs/kandev/pull/4372#issuecomment-6164987618). Exact-head CI/review confirmation
remains pending for the implementation push; earlier head evidence is historical.
