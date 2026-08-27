---
spec: docs/specs/ui/requirements/pr-detail-header-width.md
created: 2026-08-14
status: done
---

# Implementation Plan: Responsive PR Detail Header

## Overview

Make the provider-neutral change-request header respond to its own content and
rendered width instead of allowing its action cluster to consume the title.
Use intrinsic flex wrapping so actions share the row only with a single-line
title, and prove the behavior in desktop and mobile PR flows.

## Confirmed root cause

`ChangeRequestDetailHeader` renders the title and every header action in one
non-wrapping flex row. The title is the only shrinking item, so GitHub's
**Approve as...**, **Squash and merge**, optional merge-method trigger, and
refresh controls can reduce it to almost no width. Browser breakpoints cannot
solve this because a Dockview PR Details panel can be narrow inside a wide
desktop viewport.

The first title-wrapping follow-up used `text-wrap: balance` and kept the
stacked action cluster right-aligned. At intermediate widths, balancing breaks
the title early into similarly sized lines while the actions form a separate
right-aligned island below it. The intended composition needs normal line flow
and a shared leading edge for title and controls.

The subsequent fixed 640px container query still permits a long title to wrap
beside the action cluster at any width above that threshold. Width alone cannot
encode the invariant because title text, localization, authenticated actor, and
visible action labels all change the combined intrinsic width.

## Frontend

### Responsive header composition

- In
  `apps/web/components/integrations/change-request-detail-header.tsx`, give the
  title and action cluster stable test IDs for rendered geometry checks.
- Make the primary header row a wrapping flex row driven by intrinsic content
  width, with no fixed container or viewport breakpoint.
- Keep the title's automatic flex basis so line collection measures its
  unwrapped content. Let it grow across a line when the action cluster moves to
  the next flex line.
- Let the action cluster shrink only when it owns its flex line, so its controls
  can wrap internally on phones without forcing the title to shrink beside it.
- Remove single-line truncation from the title. Let it wrap naturally, with
  normal greedy line filling and long-word protection so title text stays
  inside its content box. Do not balance title lines.
- Move the complete existing action cluster together, including normalized
  provider actions, GitHub `headerActions`, and refresh. Preserve action order,
  handlers, pending and disabled states, accessible names, and button sizing.
- Add no new copy, state, API calls, or provider-specific layout branch.

### Mobile design contract

- **Desktop outcome:** title/actions remain inline only when the complete title
  stays on one line; otherwise actions move below before the title loses width.
- **Mobile entry point:** the existing **Review** bottom-navigation item opens
  `MobileReviewPanel`; navigation does not change.
- **Nearest shipped exemplar:**
  `apps/web/components/task/mobile/mobile-review-panel.tsx` remains the focused,
  full-height Review surface and supplies the existing review selector.
- **Hierarchy and primary actions:** review identity stays first; approval,
  merge, provider actions, and refresh remain in a wrapping cluster directly
  below it when narrow.
- **Presentation rationale:** these are frequent, already-visible review
  controls, so an inline second row is clearer than an overflow drawer or new
  route.
- **Geometry:** the header remains fixed above
  `change-request-detail-scroll`, that ScrollArea remains the only vertical
  scroll owner, safe-area handling is unchanged, and phone buttons retain their
  existing 44px minimum height.
- **Shared logic:** provider data, eligibility, mutations, and selection remain
  shared; only header presentation responds to container width.
- **Mobile proof:** extend the existing 320px mobile PR scenario to assert title
  wrapping and placement, action hit areas and containment, and zero
  document-level horizontal overflow.

## Tests

No unit test is appropriate for the new behavior because jsdom does not
evaluate browser flex wrapping or rendered geometry. Existing
`change-request-detail.test.tsx` interaction coverage remains unchanged; the
responsive contract is tested in Chromium.

## E2E Tests

- **Scenario:** a wide desktop viewport shows title/actions inline when their
  intrinsic widths fit, moves actions below before they would wrap the title,
  then lets the full-width title wrap at a narrower panel width.
  - **File:** `apps/web/e2e/tests/pr/pr-detail-layout.spec.ts`
  - **Method:** seed a merge-ready PR authored by another user so both GitHub
    actions render, resize the center group indirectly through the existing
    `resizeColumnViaSplitview` right-column helper, and assert a single-line
    inline state, a squeezed single-line title with actions below, and a narrow
    full-width wrapping title with actions below.
- **Scenario:** the 320px phone Review surface places approval and squash-merge
  actions below the title while preserving the existing re-request flow.
  - **File:**
    `apps/web/e2e/tests/pr/mobile-pr-rerequest-review.spec.ts`
  - **Method:** make the existing first PR fixture merge-ready, assert title and
    action geometry plus 44px touch targets before re-requesting the dismissed
    review, then retain the existing no-horizontal-overflow assertion.

## Verification Results

- Content-aware RED: at the initial 1000px detail width, the fixed container
  query kept actions inline and forced the title onto two lines; the regression
  expected one full-width title line with actions below.
- Desktop Chromium passed 1/1 after intrinsic flex wrapping. The scenario
  proves a 1200px inline single-line state, a content-derived squeezed
  single-line title with actions below, and a 600px full-width wrapping title
  with actions below.
- CI follow-up replaced the fixed squeezed width with one derived from rendered
  title and action sizes so font metrics cannot move the test across the flex
  boundary; desktop Chromium passed 1/1.
- Mobile Chromium passed 1/1, preserving 44px action targets, natural title
  wrapping, the dismissed-review re-request flow, and zero document horizontal
  overflow.
- Live isolated rendering matched the three desktop geometries: at 1200px the
  title and actions shared `y=92`; at 1000px the 976px one-line title ended
  before actions at `y=120`; at 600px the 576px two-line title ended before
  actions at `y=140`.
- Live 320px rendering kept the three-line 296px title above the action cluster,
  with document `scrollWidth` equal to its 320px `clientWidth`.
- Targeted ESLint and Prettier checks passed; `git diff --check` was clean and
  no fixed change-request breakpoint classes remained.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [Task 01: Make the PR detail header panel-responsive](task-01-panel-responsive-header.md)

Execution is sequential in the primary conversation. No subagent delegation is
planned or authorized.

## Risks

- The title must keep `flex-basis: auto`; a zero basis would let actions remain
  inline while the title shrinks and wraps, recreating the regression.
- The action cluster needs `min-width: 0` and a bounded shrink path so it can
  wrap its own controls on phones after moving to a separate flex line.
- Header actions appear asynchronously as GitHub status and merge-method data
  settle. Geometry assertions must wait for both named buttons instead of using
  a fixed delay.
- Long unbroken title tokens must wrap inside the title content box rather than
  create horizontal overflow.
- `ChangeRequestDetail` is a host-owned provider-neutral surface. Applying the
  responsive composition there intentionally keeps GitHub and compatible
  plugin review panels aligned with ADR
  `2026-08-06-plugin-code-host-dashboard-parity`; provider mutation behavior
  remains untouched.
