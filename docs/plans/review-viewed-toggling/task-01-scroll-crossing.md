---
id: "01-scroll-crossing"
title: "Preserve manual clearing until a fresh scroll pass"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-REVIEW-VIEWED-001
acceptance_criteria:
  - AC-UI-REVIEW-VIEWED-001.1
  - AC-UI-REVIEW-VIEWED-001.2
  - AC-UI-REVIEW-VIEWED-001.3
  - AC-UI-REVIEW-VIEWED-001.4
  - AC-UI-REVIEW-VIEWED-001.5
system_design:
  - ../../specs/ui/system-design/review-viewed-toggling.md
---

# Task 01: Preserve manual clearing until a fresh scroll pass

## Summary

Repair per-file auto-mark crossing state and add focused regression evidence.
Start only after a later explicit implementation request. Follow TDD: reproduce
the manual-clear failure with the current code before changing it.

## In scope

Stable observer lifecycle, transient crossing baseline, latest eligibility and
callback, controlled component regression harness, desktop/phone E2E evidence.
Inspect the existing interaction hook if layout-only movement needs a narrower
scroll authorization; retain existing navigation suppression.

## Out of scope

New settings, API/storage changes, global scroll changes, dialog redesign,
review hash semantics and unrelated refactors.

## Acceptance

1. Targeted unit regressions fail for the current immediate-reselection defect,
   then pass with the transition model, including cleanup and isolation cases.
2. Desktop/phone browser regressions prove the .1/.2/.5 transitions with actual
   scroll geometry and no scroll caused by toggling the checkbox; .3/.4 gates
   remain covered by focused component tests and disabled browser coverage.
3. All commands below pass; record actual evidence and synchronize plan/spec
   lifecycle only once implementation satisfies the complete contract.

## ASCII UI preview

UI-01 desktop and UI-02 phone, [full preview](plan.md#ascii-ui-preview):

```text
Desktop tree: [x] a.ts -> clear -> [ ] a.ts at same diff position
Phone header: [x] a.ts -> clear -> [ ] a.ts at same diff position
Both: return to file start -> pass start again -> [x] a.ts
```

Checkbox clearing changes only review state and count. Desktop tree and phone
file header remain the entry points. Retain the existing fixed toolbar and
single diff scroller. This maps to AC-UI-REVIEW-VIEWED-001.1/.2/.5.

## Verification

Run from repository root. Ensure Node/pnpm are available. Install dependencies
once in this worktree before package commands when absent:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/review/review-diff-list-auto-mark.test.tsx hooks/use-session-file-reviews.test.tsx)
(cd apps/web && pnpm exec eslint components/review/review-diff-list.tsx components/review/review-diff-list-auto-mark.test.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project chromium tests/review/review-viewed-toggling.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/review/mobile-review-viewed-toggling.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

E2E commands run sequentially with fresh builds through the managed runner.
Do not use worker overrides. Lint/test any additional changed interaction hook. The CI follow-up also runs
`components/review/use-review-diff-scroll-interaction.test.ts` and
`components/review/use-review-scroll-anchor.test.ts` with the existing focused
component/persistence tests.
Verify browser tests are discovered and save screenshot evidence against the
preview. Confirm repeated downward scroll leaves a cleared file unchecked until
its sentinel returns and crosses again. Assert progress count and the shared
checkbox state, retain the existing session-persistence unit coverage.

## Files likely touched

- `apps/web/components/review/review-diff-list.tsx`
- `apps/web/components/review/review-diff-list-auto-mark.test.tsx`
- `apps/web/e2e/tests/review/review-viewed-toggling-flow.ts` (shared desktop/phone flow)
- `apps/web/e2e/tests/review/review-viewed-toggling.spec.ts` (new)
- `apps/web/e2e/tests/review/mobile-review-viewed-toggling.spec.ts` (new)

## Dependencies

None. Existing ready diff eligibility and session review persistence are reused.

## Risks

Initial observer samples must establish state without writing reviews. Do not
leave stale callbacks active or lose crossing state when callback identity
changes. Geometry movement without a user reading pass must remain suppressed.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/review-viewed-toggling.md).
- [Design](../../specs/ui/system-design/review-viewed-toggling.md).
- Existing auto-mark component tests and desktop/mobile review-file-status E2E.

## Results

Implemented. Five regressions failed before the fix; the final focused run passed
29 tests. Desktop and Pixel 5 browser scenarios passed, including disabled
auto-marking and real touch scrolling. ESLint, typecheck (8 GiB heap), public
documentation checks and specification checks passed. Fresh desktop/phone
captures match UI-01/UI-02. Full command results are in [the plan](plan.md#verification-results).


### CI follow-up: cancellation and manual-input precedence

The SDK publishing PR's required browser run exposes an initial manual scroll
that does not mark the target. Both failed attempts retain traces. The earlier
fixture contamination is absent. Three unchanged local repetitions pass; the
exact browser cause remains under investigation.

A separate controlled regression proves that cancelling a pending content
restoration restores its saved suppression after explicit user input has
allowed marking. Wheel/touch, scroll-key, and scrollbar paths all finish
suppressed. Repair their ordering: cancel restoration first, then apply the
input's marking permission. Preserve descendant-pointer and unrelated-key
gates, navigation suppression, crossing geometry, and existing eligibility.
Use the real anchor hook with a queued frame in regression tests. Run affected
component, anchor, persistence, desktop, and phone coverage before publishing.
This is a verified source defect; it is not yet the established CI cause.


Follow-up validation: all 52 focused unit cases, full typecheck, changed-file
lint, documentation validators, and three desktop plus three phone browser
repetitions pass after rebuilding web assets. Independent read-only review
confirms the real-hook regression and narrow ordering correction. The original
CI trace does not expose saved restoration state, so its precise cause remains
unconfirmed. Fresh exact-head CI is required after publication.
