---
created: 2026-10-28
status: complete
requirements:
  - REQ-UI-TASK-PAGINATION-PRESENTATION-001
system_design:
  - ../../specs/ui/system-design/task-pagination-presentation.md
legacy_specs: []
---

# Implementation Plan: Task Pagination on Phones

## Overview

On the real `/tasks` list, a seven-page result at page 4 renders five numeric
controls, two ellipses, Previous, and Next in a non-wrapping row. Qualification
with a real isolated backend and 70 synthetic rows found Next at x=358..402:
it is clipped at 393px and wholly offscreen at 320px. Page 1 and its Next
operation passed. A horizontal pan can recover the action; this is a footer
containment defect, not a global navigation outage.

Implement one local presentation repair and its independent causal regression
in [Task 01](task-01-pagination-containment.md), after the reviewed-package and
resource release. The UI system owns this reusable presentation contract; the
existing listing-preference spec explicitly excludes changing pagination.

## Scope

In scope: local wrapping and bounded width for the existing task pagination,
preserving direct actions and current status; component and native browser proof.
Out of scope: shared primitives/sizing, global breakpoints or hooks, task query,
controller, settings, navigation, dependencies, and adjacent saved-view work.

## Technical approach

In `apps/web/app/tasks/tasks-pagination.tsx`, keep `getPageNumbers`, its current
handlers, labels, and state. Allow `PaginationContent` to wrap only in narrow
layout and ensure its ancestors can shrink to the available width; restore the
single row at the existing appropriate breakpoint. Verify the existing sizing
contract at phone widths, including a fine pointer, and only adjust this local
caller if required. No new strings or locale changes are expected.

## UI composition

**UI-01, phone, page 4 of 7:** the existing range and rows-per-page control
precede the pagination. Previous, direct page controls, ellipses, and Next stay
in their existing logical order, wrapping onto another line when needed. Every
visible action remains fully contained and tappable. The task list supplies the
vertical scroll. The footer reserves bottom clearance for the existing floating
add button and safe area. No horizontal scrolling, drawer, or new fixed control
is added.

**UI-02, desktop, page 4 of 7:** the range and selector retain their current
layout; first, Previous, page numbers, Next, and last remain a compact single
row. Boundary actions remain disabled at their respective ends. These are
structural requirements rather than pixel spacing specifications; existing
localized labels and styling remain in use. UI-01 maps to AC .1 and .2; UI-02
to AC .1 and .3. The work order records the rendered checks.

## Tests

`apps/web/app/tasks/tasks-pagination.test.tsx` independently exercises the
real component: page 4 current state, previous/next and direct-page callbacks,
range, first/last disabled states, and page-size reset. Geometry is established
by browser tests, not jsdom. The browser RED below is the causal layout test;
ordinary first-page and action controls must pass before the containment failure.

## E2E tests

`apps/web/e2e/tests/task/mobile-task-pagination.spec.ts` uses `mobile-chrome`
with 70 isolated backend-created rows and 10 rows per page. At 320, 393, and 430px,
use real taps to reach page 4; after every page change scroll the list vertically
back to its footer before measuring or capturing it. Assert visible enabled
controls are nonoverlapping, 44px in both dimensions, fully within both their
surface and viewport, and no document or list horizontal overflow exists.
Tap Next 4 to 5, Previous, and a displayed direct page; assert current page and
range after each. Preserve native footer screenshots separately from managed
runner clearing. `apps/web/e2e/tests/task/task-pagination.spec.ts` uses
`chromium` for a 1280px desktop control and narrow fine-pointer geometry plus
breakpoint-adjacent behavior; check keyboard activation and existing first/last
and direct-page actions. A small shared helper may be owned at
`apps/web/e2e/helpers/task-pagination.ts` and included in focused lint.

## Work orders

- [x] [Task 01: Contain pagination and prove navigation](task-01-pagination-containment.md)

## Verification results

Independent native RED failed on page 4 at 320 and 393px after the page 1
controls and navigation passed. The initially repaired component passed two component
tests, two mobile browser cases, and one desktop browser case; the expanded
mobile matrix also passed at 640, 767, 768, 1023, and 1024px. Scoped lint and
the canonical 6 GiB web typecheck passed. A subsequent 430px native hit test
found Next covered by the existing floating add button. After local footer
clearance, three native phone cases and the desktop/narrow-fine control passed,
as did the affected component suite and scoped lint. Inspected phone screenshots
show the scrolled footer.

## Risks

- Intrinsic flex widths or an ancestor clip can defeat wrapping; measure each
  action against the viewport and the list surface, not just document width.
- Navigation resets vertical scroll; a screenshot must be captured after a new
  vertical-only scroll to the footer and must actually show that footer.
- Large page counts and disabled boundary controls must keep their semantics.
