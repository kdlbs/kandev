---
created: 2026-10-20
status: implemented
requirements:
  - REQ-UI-REVIEW-VIEWED-001
system_design:
  - ../../specs/ui/system-design/review-viewed-toggling.md
legacy_specs: []
---

# Implementation plan: Review viewed toggling

## Overview

One sequential work order repairs the observer lifecycle and proves the user
flow on desktop and phone. The missing manual-clear/revisit contract is recorded
in the paired requirement and design. Implementation and targeted checks are complete.

## Scope

Include per-file crossing detection and focused unit/browser regression coverage.
Exclude storage changes, new settings, unrelated review behavior and visual
redesign. No subagents are authorized.

## Evidence and technical approach

Source trace: `useAutoMarkOnScroll` resets its guard while reviewed, reconnects
on uncheck, and interprets an initial above-root observation as a fresh pass.
A read-only Python evaluation of the current callback predicate with
isReviewed=false, isIntersecting=false, sentinelAboveRoot=true,
suppressed=false and autoMarked=false returns true without scroll movement.
Five new regression cases failed before the production correction, including
the manual-uncheck case observing two automatic writes instead of one. Node
and pnpm were available through the local mise installation after updating PATH.

Use the paired design's transition baseline, latest callback/eligibility and
existing scroll suppression. Keep geometry tracking alive across review changes.
The existing test observer mock must distinguish active and disconnected
observers so old callbacks cannot produce false-positive tests.

## ASCII UI preview

UI-01: Desktop Review Changes, target file already passed.

```text
Fixed file tree             Existing diff scroll owner
[x] a.ts -> clear -> [ ]     b.ts content at current position
                            scroll up to a.ts start
[x] a.ts <- pass again       scroll down past a.ts start
```

UI-02: Phone Review Changes, existing full-height reading surface.

```text
Fixed review toolbar and close action
[ ] a.ts  (existing header viewed control)
Diff content in the existing vertical scroller
Scroll back, then past a.ts start -> [x] a.ts
```

The transition is required; spacing and sample filenames are illustrative.
Desktop tree remains hidden on phone. Use a short target file or its existing
sticky header so the phone checkbox is reachable at the regression position.
Keep existing controls, scroll ownership, containment and touch geometry.
Both views map to AC-UI-REVIEW-VIEWED-001.1, .2 and .5. Loading/stale and disabled
states retain their existing presentation and eligibility under .4.

## Tests

Extend `apps/web/components/review/review-diff-list-auto-mark.test.tsx` with
`keeps a manual uncheck until returning and passing the file again` (.1/.2),
`does not mark an initial above-root sample or layout-only crossing` (.3),
and disabled/stale/pending and composite-key cases (.4/.5).
Retain existing initial-layout and programmatic-jump regressions.

## E2E tests

New `e2e/tests/review/review-viewed-toggling.spec.ts` (chromium) and
`e2e/tests/review/mobile-review-viewed-toggling.spec.ts` (mobile-chrome) prove
clear-at-current-position, unrelated downward scrolling, and return/pass
reselection (.1/.2/.5), with automatic-mark-disabled coverage (.4).
Use existing review fixture patterns and localized accessible checkbox names.
Compare the rendered structure with UI-01/UI-02 and capture both views.

## Work orders

- [x] [Task 01: Preserve manual clearing until a fresh scroll pass](task-01-scroll-crossing.md)

## Verification results

- `(cd apps && pnpm install --frozen-lockfile)`: passed.
- `(cd apps/web && pnpm exec vitest run components/review/review-diff-list-auto-mark.test.tsx hooks/use-session-file-reviews.test.tsx)`: passed, 29 tests in two files. Before the correction, five new regressions failed.
- `(cd apps/web && pnpm exec eslint components/review/review-diff-list.tsx components/review/review-diff-list-auto-mark.test.tsx)`: passed without warnings.
- `(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)`: passed. The first attempt exhausted Node's default 4 GiB heap.
- `(cd apps/web && CAPTURE_PR_ASSETS=true pnpm e2e:run --host --no-build --project chromium tests/review/review-viewed-toggling.spec.ts)`: passed, one test. An earlier managed run built the backend, fixture plugin and fresh production web assets; subsequent runs reused those unchanged assets. Initial browser attempts exposed an incorrect test-helper argument, which was corrected before the final passing run.
- `(cd apps/web && CAPTURE_PR_ASSETS=true pnpm e2e:run --host --no-build --project mobile-chrome tests/review/mobile-review-viewed-toggling.spec.ts)`: passed, one test, including real touch scrolling. Both viewports prove manual clearing, further downward scroll, revisit/reselection, disabled auto-mark, synchronized counts and document containment.
- Fresh desktop/phone screenshots inspected against UI-01/UI-02, retained in ignored `.pr-assets` for PR publication.
- `node --test scripts/validate-public-docs.test.mjs`: passed, 63 tests.
- `node scripts/validate-public-docs.mjs`: passed, 54 pages.
- `python3 scripts/list-docs.py validate`: passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `git diff --check`: passed.
- Cross-reference inspection: all work-order ACs exist in the paired requirement; the design declares that requirement and is included by the plan. PR documentation coverage has no local CLI entry point; its remote status remains a publication gate.
- Public how-to documentation updated in `docs/public/sessions-and-review.md` to explain manual clearing and the subsequent scroll pass.
- Exact-head CI, automated review and merge remain delivery work outside this implementation status.

## Risks

Observer initial delivery, callback churn, stale eligibility, layout-only
movement, programmatic navigation, and source changes must not simulate a
reading pass. Preserve the current input suppression behavior. Node/pnpm are supplied by the local mise installation; typechecking requires
a larger heap than Node's default for this repository.

## PR review remediation

A new regression proved that a scroll without an observer callback could be
mistaken for a later layout crossing. Crossing detection now requires the
marker displacement to agree with the root scroll delta on actual root scroll
events. Intersection observations only update the baseline; this also avoids
comparing delayed observer snapshots against newer touch-scroll positions. The new regression
failed before the correction and passed afterward; all 29 focused tests pass.
Review tests now independently exercise unchecking and handler replacement.
The browser flow waits for the matching session Git-status notification
containing both fixture files before asserting row visibility with the default
timeout. Typecheck, ESLint and both final browser scenarios passed again. Fresh
desktop and phone captures were inspected after the production correction;
the live PR screenshot section is refreshed during publication.


## CI follow-up checkpoint

PR #4534 validates a narrow input-cancellation ordering correction in the
existing work order. The real pending anchor restoration must be cancelled
before explicit manual input authorizes marking. Three controlled input
regressions fail before implementation. Preserve the non-scroll interaction
gates and require fresh focused and desktop/phone validation. The original CI
first-scroll cause remains unproven; do not treat this correction as complete
transport or browser-timing attribution.


Follow-up validation: all 52 focused unit cases, full typecheck, changed-file
lint, documentation validators, and three desktop plus three phone browser
repetitions pass after rebuilding web assets. Independent read-only review
confirms the real-hook regression and narrow ordering correction. The original
CI trace does not expose saved restoration state, so its precise cause remains
unconfirmed. Fresh exact-head CI is required after publication.
