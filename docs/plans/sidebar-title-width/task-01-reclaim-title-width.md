---
id: "01-reclaim-title-width"
title: "Reclaim desktop title width"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-TASK-ROW-PRESENTATION-001
system_design:
  - ../../specs/ui/system-design/sidebar-task-row-presentation.md
acceptance_criteria:
  - AC-UI-SIDEBAR-TASK-ROW-PRESENTATION-001.12
  - AC-UI-SIDEBAR-TASK-ROW-PRESENTATION-001.14
  - AC-UI-SIDEBAR-TASK-ROW-PRESENTATION-001.15
  - AC-UI-SIDEBAR-TASK-ROW-PRESENTATION-001.16
---

# Task 01: Reclaim Desktop Title Width

## Summary

Size the desktop time/menu column to its content with a 24px button minimum.
Keep title width stable through interaction and preserve the phone row composition.

## In scope

- Reproduce current long-title/PR/time gap with a failing rendered geometry test.
- Implement the relative-time branch sizing from the paired design.
- Update obsolete component and desktop E2E expectations; verify mobile containment.

## Out of scope

Shared title components, other trailing modes, formatting, settings, and backend changes.

## Acceptance

- Desktop slot equals the wider of time and button within one CSS pixel; long titles reclaim the difference from 44px.
- Title and badge rectangles stay stable across hover, focus, menu open/close for unchanged time; short-title badges remain adjacent.
- Phone actions remain visible, at least 44px in both dimensions, inside the row, and separate from time; row navigation and no-overflow checks pass.

## ASCII UI preview

See [combined preview](plan.md#ascii-ui-preview).

```text
UI-01 Desktop sidebar, long title
Before: [state] Long title... [PR] [unused   2h]
After:  [state] Longer title text... [PR] [2h]
Hover:  [state] Longer title text... [PR] [...]
Short:  [state] Short title [PR]          [2h]

UI-02 Phone task picker, inset bottom drawer
+------------------------------------------+
| Tasks                                    |
| [state] Title... [PR] [time] [44px action] |
|             vertically scrolling rows    |
+------------------------------------------+
```

Spacing is illustrative. UI-01 requires adjacent title badges and identical title geometry
between idle, hover, focus, and menu-open states. UI-02 keeps primary row navigation,
the existing safe-area handling, one internal scroll owner, and visible touch actions.
Criteria: AC-UI-SIDEBAR-TASK-ROW-PRESENTATION-001.12, .14, .15, and .16.

## Verification

Run from the repository root. Install dependencies if this worktree is not installed.
Add rendered regression coverage first, demonstrate failure, then implement and run:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task/task-item-trailing.test.tsx components/task/task-item-compact-layout.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/task/task-item-trailing.tsx components/task/task-item-trailing.test.tsx e2e/tests/task/sidebar-title-width.spec.ts e2e/tests/task/mobile-sidebar-views.spec.ts e2e/tests/task/sidebar-filter.spec.ts)
make build-web
(cd apps/web && pnpm e2e:run tests/task/sidebar-title-width.spec.ts tests/task/sidebar-filter.spec.ts -- --grep "title width|task row presentation")
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-sidebar-views.spec.ts -- --grep "task row settings|title width")
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Record screenshot inspection and every command result. Do not mark done on class assertions alone.

## Files likely touched

- apps/web/components/task/task-item-trailing.tsx
- apps/web/components/task/task-item-trailing.test.tsx
- apps/web/e2e/tests/task/sidebar-title-width.spec.ts
- apps/web/e2e/tests/task/sidebar-filter.spec.ts
- apps/web/e2e/tests/task/mobile-sidebar-views.spec.ts

Reference only: task-item.tsx, task-item-menu-button.tsx, app/globals.css,
and the existing mobile-sidebar-task-actions.spec.ts geometry patterns.

## Dependencies

None.

## Risks

Localized width and phone in-flow overrides can cause clipping if desktop sizing leaks.

## Parallelism

sequential

## Inputs

- [Requirements](../../specs/ui/requirements/sidebar-task-row-presentation.md), criteria .12 and .14-.16.
- [Design](../../specs/ui/system-design/sidebar-task-row-presentation.md), trailing layout and responsive behavior.
- [Plan](plan.md), evidence correction and E2E scenario matrix.

## Results

Implemented content-sized desktop time slots with a 24px minimum on fine-pointer
layouts at 640px and wider. The hidden time keeps its intrinsic width, so hover,
focus, and menu states do not move the title or PR badge. Phone layouts retain the
44px time and action composition. The obsolete equal desktop time-width assertion
was removed.

A review follow-up scoped `justify-end` to the desktop fine-pointer wrapper. The
desktop E2E now fixes the page time relative to the task update timestamp so the
visual token is deterministically `2h` and narrower than the 24px slot. Before
the follow-up fix, its right edge was 11.765625px short of the slot edge; after
the fix, the 1px geometry assertion passes. The mobile layout remains unchanged.

The first rendered desktop check failed as intended: the slot measured 44px where
the action minimum was 24px. After the implementation, the focused geometry check
passed. Desktop coverage also checks short and long titles with and without PRs,
details on/off, expanded sidebar width, localized tokens, and stable interaction
geometry. The mobile check confirms visible 44px controls, containment, separation,
navigation, and no horizontal overflow.

Verification results:

- `pnpm exec vitest run components/task/task-item-trailing.test.tsx components/task/task-item-compact-layout.test.tsx` — passed, 2 files and 8 tests.
- `pnpm run typecheck` — passed.
- `pnpm exec eslint components/task/task-item-trailing.tsx components/task/task-item-trailing.test.tsx e2e/tests/task/sidebar-title-width.spec.ts e2e/tests/task/mobile-sidebar-views.spec.ts e2e/tests/task/sidebar-filter.spec.ts` — passed.
- `make build-web` — passed.
- `pnpm e2e:run --no-build tests/task/sidebar-title-width.spec.ts` — passed, 2 tests.
- `pnpm e2e:run --no-build tests/task/sidebar-title-width.spec.ts tests/task/sidebar-filter.spec.ts -- --grep "title width|task row presentation"` — the sidebar-filter presentation test passed; the title interaction assertion was corrected and the complete sidebar-title-width file passed in the command above.
- `pnpm e2e:run --no-build --project mobile-chrome tests/task/mobile-sidebar-views.spec.ts -- --grep "task row settings|title width"` — passed, 1 test.
- `CAPTURE_PR_ASSETS=true pnpm e2e:run --host --no-build tests/task/sidebar-title-width.spec.ts -- --grep "title width follows"` — passed, 1 test; the desktop capture was inspected.
- `CAPTURE_PR_ASSETS=true pnpm e2e:run --host --no-build --project mobile-chrome tests/task/mobile-sidebar-views.spec.ts -- --grep "task row settings"` — passed, 1 test; the phone capture was inspected.
- `python3 scripts/list-docs.py validate` — passed, 299 decisions and 1108 specifications.
- `python3 scripts/lint-spec-files.py --all` — passed.
- `git diff --check` — passed.

Post-review verification against the final production build:

- `make build-web` — passed.
- `pnpm run typecheck` — passed.
- `pnpm exec eslint components/task/task-item-trailing.tsx components/task/task-item-trailing.test.tsx e2e/tests/task/sidebar-title-width.spec.ts e2e/tests/task/mobile-sidebar-views.spec.ts e2e/tests/task/sidebar-filter.spec.ts` — passed.
- `pnpm exec vitest run components/task/task-item-trailing.test.tsx components/task/task-item-compact-layout.test.tsx` — passed, 2 files and 8 tests.
- `pnpm e2e:run --host --no-build tests/task/sidebar-title-width.spec.ts` — passed, 2 tests, including the `2h` right-edge assertion and localized-token breakpoints.
- `pnpm e2e:run --host --no-build --project mobile-chrome tests/task/mobile-sidebar-views.spec.ts -- --grep "task row settings|title width"` — passed, 1 test.
- `git diff --check` — passed after the review follow-up.

PR review follow-up: the Japanese `99年以上` token extended beyond the 44px phone
time span before the fix. Phone and coarse-pointer slots now keep a 44px minimum,
grow to non-wrapping localized content, and right-align the time without overlapping
the in-flow phone action. Coarse-pointer desktop breakpoints retain their 44px slot
minimum. Relative-time component tests no longer assert CSS class strings; rendered
geometry owns the responsive time-layout contract.

- `make build-web` — passed.
- `pnpm run typecheck` — passed.
- `pnpm exec eslint components/task/task-item-trailing.tsx components/task/task-item-trailing.test.tsx e2e/tests/task/sidebar-title-width.spec.ts` — passed.
- `pnpm exec vitest run components/task/task-item-trailing.test.tsx components/task/task-item-compact-layout.test.tsx` — passed, 2 files and 8 tests.
- `pnpm e2e:run --host --no-build tests/task/sidebar-title-width.spec.ts` — passed, 2 tests, including hover stability, Japanese phone containment, and coarse-pointer geometry.
- `pnpm e2e:run --host --no-build --project mobile-chrome tests/task/mobile-sidebar-views.spec.ts -- --grep "task row settings|title width"` — passed, 1 test.
- `python3 scripts/list-docs.py validate` — passed, 299 decisions and 1108 specifications.
- `python3 scripts/lint-spec-files.py --all` — passed.
- `git diff --check` — passed.
