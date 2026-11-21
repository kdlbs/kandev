---
created: 2026-10-29
status: implemented
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-DISCLOSURE-001
system_design:
  - ../../specs/integrations/system-design/github-pr-status-disclosure.md
legacy_specs: []
---

# Implementation Plan: Stable PR Status Disclosure

## Overview

Keep the PR action stationary while status updates arrive. One sequential
work order delivers the desktop and phone composition with rendered regression
coverage. The user confirmed the proposed stable frame and fixed PR action.

Ownership belongs to integrations because this is the detailed GitHub status
surface, not the separate informational sidebar summary. No material product
question remains. Dimensions are implementation choices within the accepted design.

## Scope

Include both desktop entry points, single/multiple PRs, the existing phone
drawer, initial loading, cached refresh, localization, and keyboard access.
Exclude provider logic, refresh timing, sidebar tooltips, click-to-pin behavior,
and other providers' presentation. No backend change or architectural ADR is needed.

## Technical approach

Follow the [system design](../../specs/integrations/system-design/github-pr-status-disclosure.md).
The current maximum-height outer scroller lets conditional rows move the header.
Replace that layout with fixed header/footer regions and one inner scroller.
Keep GitHub's frame at a viewport-capped height from its first visible render.
Retain Radix collision handling and the existing feedback cache.

Shared anatomy changes must be opt-in. GitHub opts into stable geometry and a
labelled link. GitLab and plugin consumers retain existing defaults. Mobile
uses the current Drawer rather than a hover emulation. The review how-to guide in `docs/public/sessions-and-review.md` documents
the labelled Open PR action and fixed header. No new public page is needed.

## UI preview

UI-01: Desktop topbar or chat status chip. Initial loading and live updates.

| Region | Content | Behavior |
| --- | --- | --- |
| Tabs, when multiple PRs | Repository and PR identifiers | Fixed |
| Header | PR title, author, Open PR | Fixed action position |
| Details | Checks, review, comments, automation, merge | Only vertical scroller |
| Footer | Loading, Updating, or updated time | Reserved fixed row |

UI-02: Phone status drawer. Tap the existing CI chip.

| Region | Content | Behavior |
| --- | --- | --- |
| Drawer heading | PR status and Close | Fixed |
| PR selection and identity | Tabs, title, author, Open PR | Fixed, touch-sized action |
| Details | Same PR capabilities | One internal scroller |
| Footer | Refresh state and bottom safe area | Fixed |

Before: conditional rows increase the outer height and move the header.
After: loading placeholders become results inside the same frame. Empty and
error results occupy the details region without removing the PR action.
Region order and scroll ownership are required. Wording and spacing are
illustrative and use localized copy and existing tokens. These structured
previews follow the session instruction to avoid ASCII drawings.

## Tests

`pr-ci-popover.checks.test.tsx` covers persistent header/link during loading,
refresh and authentication errors, cached-result retention, and authentication failures. Multi-PR browser tests cover selected-PR URL
changes. Existing checks/footer/automation tests retain their behavior coverage.
Geometry assertions belong in Playwright, not jsdom.

## E2E tests

- `tests/pr/pr-status-popover-stability.spec.ts`, chromium: criteria .1 through
  .5. Parameterize chip/topbar and one/multiple PRs. Hold initial responses,
  record geometry, release responses separately, and click the recorded link
  center after each release. Confirm the external destination. Cover long/empty
  content, a short viewport, focus, scrolling, Escape, and narrow fine pointer.
- `tests/pr/mobile-pr-ci-chip.spec.ts`, mobile-chrome: criteria .2 through .6.
  Extend drawer coverage with delayed updates, fixed PR action, long-content
  scrolling, real target dimensions, and viewport containment.
- Retain `pr-topbar-popover.spec.ts` and `pr-multi-popover.spec.ts` as behavior
  regressions. Use causal waits, fixture-backed feedback, and isolated instances.

## Work orders

- [x] [Task 01: Stabilize status frames and PR actions](task-01-stable-status-frame.md)

## Verification results

Design validation on 2026-10-29:

- `python3 scripts/list-docs.py validate`: passed, 414 decisions and 1814 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check -- docs/specs docs/plans/stable-pr-status-disclosure`: passed.
  A direct whitespace check also passed for all four untracked files.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: the actual documentation-only
  changes are exempt. A prospective check including `pr-ci-popover.tsx` passed
  with the work order, requirement, and design references accepted.
- `git status --short`: only the four design-package files are new and unstaged.

Implementation completed on 2026-11-02. The [work-order results](task-01-stable-status-frame.md#results)
record the exact command scope and local environment adjustments.

- Original browser regression failed with 104 CSS pixels of link movement.
- 88 component tests passed across seven suites.
- 22 desktop and 10 phone browser tests passed. The five desktop stability
  cases passed again after adding cached-refresh and late-summary coverage.
- Typecheck passed with an 8 GiB Node heap. Targeted ESLint passed without warnings.
- Localization, 54 public docs pages, specification/catalog, documentation
  coverage, and whitespace checks passed.
- Rendered desktop and phone screenshots match the intended layout.

The 2026-11-03 horizontal-overflow follow-up passed all 10 phone tests and
all five desktop stability cases. Live checks at 320, 360, and 390 CSS pixels
confirmed zero horizontal overflow and reachable switch touch targets. The
existing drawer composition was retained.

PR review follow-up added Portuguese footer-fit checks at 320px, a long
repository-name selection case, matching visible and accessible link labels,
text spacing in tabs, and a bounded selector for many PRs on a short phone.
The many-PR regression first failed because Open PR was outside the viewport.
After the fix, all 11 phone cases and 20 affected component tests passed.
Targeted ESLint, formatting, and specification validation passed. Final CI and
review dispositions remain externally tracked on PR #5020.

The final review also exercised live association updates while the drawer
stays open. Growing from two linked PRs to twelve initially moved the action
37 CSS pixels. Reserving the phone selector height removed that shift;
all 11 phone cases passed again. Extra selector space for short lists is
intentional so later updates cannot move the action.

The compact fallback uses the full viewport at 480px height or less and combines
drawer chrome around the existing PR header. The phone regression covers
single and multiple PRs at 320x240, including the actual link hit target and
close action. The lifecycle and merge-queue phone tests now measure the inner
details scroller. All 17 cases across the two affected phone files passed;
the two compact cases passed again with footer bounds and at least 44px of
scrolling details space.

Implementation and follow-up changes are committed through normal hooks.
Publication and automated review are tracked separately from this work order.

## Risks

- Radix collision sizing must not oscillate when its available-height value changes.
- The mobile wrapper currently scrolls. Leaving it enabled creates nested scrolling.
- A stable frame uses extra blank space for short results. This is an accepted trade-off.
- Other providers consume shared anatomy. Optional defaults must preserve their behavior.
