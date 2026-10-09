---
id: "02-inline-pr-tab-close"
title: "Place unlink controls inside PR tabs"
status: done
wave: 2
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-UNLINK-MENUS-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-PR-UNLINK-MENUS-001.8
  - AC-INTEGRATIONS-GITHUB-PR-UNLINK-MENUS-001.9
  - AC-INTEGRATIONS-GITHUB-PR-UNLINK-MENUS-001.10
system_design:
  - ../../specs/integrations/system-design/github-pr-unlink-menus.md
---

# Task 02: Place unlink controls inside PR tabs

## Summary

Put each PR label and its unlink button inside one visual tab boundary.
Reveal the desktop control on hover or focus, with immediate visible access on phones and coarse pointers.

## In scope

- Update `PRTab` styling and shared wrapper in `multi-pr-ci-popover.tsx`.
- Preserve semantic sibling buttons, roving tab selection, and localized unlink labels.
- Reserve close-control space and keep pending feedback visible.
- Add desktop geometry/visibility tests and extend the phone unlink check.

## Out of scope

Association mutations, single-PR unlink eligibility, global Dockview styles,
provider actions, new copy, and changes to the selected PR's detail body.

## Acceptance

1. The shared tab outline contains the unlink button, with stable widths during reveal (AC 001.8).
2. Fine-pointer desktop controls reveal on hover/focus and remain visible while pending (AC 001.9).
3. Phone/coarse targets stay visible, reachable, and at least 44 by 44 pixels. Existing unlink behavior passes (AC 001.10).

## ASCII UI preview

See [the combined previews](plan.md#ascii-ui-preview).

```text
UI-03 desktop idle:  [ repo #3660   ] [ plugin #10   ]
      hover/focus:   [ repo #3660 x ] [ plugin #10   ]
UI-04 phone drawer: [ repo #3660 x ] [ plugin #10 x ] ->
                    selected PR details below
```

The shared outline and reserved trailing space are required.
Phone controls stay visible, including during tab-row scrolling.
Use existing localized names. `x` represents the existing unlink action.

## Implementation sequence

1. Add a rendered regression that fails on the close control outside the highlighted tab boundary.
2. Move active and hover styling to the shared wrapper without nested buttons.
3. Add local hover/focus visibility and pending feedback rules.
4. Preserve visible phone/coarse controls and the existing event boundary.
5. Run the focused checks and inspect desktop/phone captures.

## Verification

Run from the repository root after the workspace installation from Task 01.

```bash
(cd apps/web && pnpm exec vitest run components/github/pr-ci-popover.automation.test.tsx components/github/pr-status-refresh-routes.test.tsx components/github/pr-status-chip.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/github/multi-pr-ci-popover.tsx components/github/pr-ci-popover.automation.test.tsx e2e/tests/pr/pr-multi-popover.spec.ts e2e/tests/pr/mobile-pr-ci-chip.spec.ts)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --host --project chromium tests/pr/pr-multi-popover.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/pr/mobile-pr-ci-chip.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Desktop browser assertions use computed opacity and pointer events, not only `toBeVisible()`.
Measure the unlink bounds against the shared wrapper and compare width before/after hover.
Cover active and inactive tabs, keyboard focus, pending spinner, and a failed unlink.
Preserve adjacent selection and two-to-one collapse tests.
The mobile unlink test measures both height and width, tab containment, and document overflow.
Add a 767px fine-pointer case and a desktop-width coarse-pointer case for responsive controls.
Use the configured mobile project without per-test device overrides.

## Files likely touched

- `apps/web/components/github/multi-pr-ci-popover.tsx`
- `apps/web/components/github/pr-ci-popover.automation.test.tsx`
- `apps/web/e2e/tests/pr/pr-multi-popover.spec.ts`
- `apps/web/e2e/tests/pr/mobile-pr-ci-chip.spec.ts`

## Dependencies

No functional dependency on Task 01. Execute sequentially in this package.

## Risks

An invisible control can capture clicks. Keyboard users need a visible focus path.
Moving border styles can change tab sizing. Preserve compact desktop density and local horizontal scrolling.

## Parallelism

`sequential`

## Inputs

- [Unlink requirements](../../specs/integrations/requirements/github-pr-unlink-menus.md), AC 001.8 through 001.10.
- [Tab design](../../specs/integrations/system-design/github-pr-unlink-menus.md#multi-pr-information-tabs).
- `apps/web/components/task/session-tab-close-action.tsx` and `apps/web/app/dockview-theme.css` for visual precedent.
- Existing multi-PR component tests and phone unlink E2E.

## Results

Completed on 2026-10-09.

- Moved active and hover styling to the shared tab wrapper while preserving semantic sibling buttons, stable width, and pending unlink feedback.
- Kept controls visible and at least 44 by 44 pixels for phone/coarse-pointer use; the rendered phone control measures 48 by 48 pixels.
- Desktop PR E2E passed: 14 tests, including pending unlink feedback, tab geometry, hover/focus reveal, and fine/coarse pointer cases.
- Mobile PR E2E passed: 12 tests, including unlink, terminal sibling removal, drawer behavior, and overflow checks.
- Focused component tests, typecheck, targeted ESLint, i18n ratchet, production web build, catalog validation, full specification lint, and `git diff --check` passed.
