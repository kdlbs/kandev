---
spec: docs/specs/ui/requirements/review-file-status.md
created: 2026-08-17
status: completed
---

# Implementation Plan: Preserve Review Path Punctuation

## Overview

Repair sticky Review headers that visually relocate dots from dot-prefixed directory segments. Keep the existing leading-edge directory truncation, isolate the path text as left-to-right content inside its right-to-left clipping wrapper, and prove literal character order in desktop and phone Chromium. No backend, API, state, persistence, or translated-copy changes are required.

## Confirmed root cause

`DesktopReviewFilePath` and `MobileReviewFileDetails` render directory text directly inside an element with `direction: rtl` and `unicode-bidi: isolate`. The RTL direction intentionally puts the ellipsis on the leading edge, but the Unicode bidirectional algorithm treats `.` as neutral punctuation and moves it to the trailing edge of the left-to-right directory run. A real production-build Chromium reproduction rendered `.agents/skills/pr-fixup/SKILL.md` as `agents/skills/pr-fixup./SKILL.md` while DOM data and the accessible label remained correct.

A disposable Chromium probe confirmed the smallest safe presentation change: retain the RTL overflow wrapper and render its directory text inside `<bdi dir="ltr">`. This preserves leading-edge ellipsis and the logical punctuation order.

## Frontend

### Sticky Review file paths

- `apps/web/components/review/review-diff-header.tsx`: introduce a small shared directory renderer used by `DesktopReviewFilePath` and `MobileReviewFileDetails`.
- Keep `truncate`, `direction: rtl`, and `unicode-bidi: isolate` on the outer directory element so constrained headers continue to retain the directory suffix nearest the filename.
- Put the directory string inside `<bdi dir="ltr">` so punctuation remains attached to its original left-to-right path segment.
- Expose the existing `data-review-file-directory` selector on both responsive variants. Preserve the full `title`, collapse/expand accessible label, filename element, toolbar layout, status/stat placement, and click behavior.

### Mobile design contract

- **Desktop outcome:** sticky diff headers show exact punctuation while retaining the filename and nearest directory suffix at constrained widths.
- **Mobile entry point:** the existing task bottom navigation opens Changes, and its existing Review action opens the full-height Review surface.
- **Nearest shipped exemplar:** `MobileReviewFileDetails` remains the focused file identity used by `mobile-review-file-status.spec.ts`; its filename-first hierarchy and directory/status second row stay unchanged.
- **Presentation rationale:** this is a text-direction repair inside current dense diff chrome, so no new route, drawer, menu, or touch action is warranted.
- **Geometry:** the Review dialog remains the scroll owner. Dynamic viewport sizing, safe-area handling, 44 px controls, and document overflow behavior remain unchanged.
- **Shared logic:** desktop and mobile use the same directory isolation rule; only their existing responsive composition differs.
- **Mobile proof:** the Pixel 5 Review status scenario uses a long dot-prefixed path and verifies visual character order plus the existing header and document containment contracts.

## Tests

- **What:** both responsive header variants retain the exact directory string and isolate it as left-to-right path content.
- **File:** `apps/web/components/review/review-diff-header.test.tsx`.
- **How:** render `.agents/skills/pr-fixup/SKILL.md` in desktop and mobile modes, then assert the directory selector contains an exact `<bdi dir="ltr">` value while `SKILL.md` remains the separate filename. Add this before the source change and record the expected RED failure.

## E2E Tests

- **Shared rendered-order probe:** extend `apps/web/e2e/helpers/layout-assertions.ts` with a single-line text helper that walks descendant text nodes, creates one-character DOM ranges, and returns characters sorted by rendered horizontal position. This tests browser bidi layout rather than DOM `textContent` or screenshot OCR.
- **Desktop scenario:** extend `apps/web/e2e/tests/review/review-file-status.spec.ts` to seed `.agents/skills/pr-fixup/SKILL.md`, open Review, and assert that the directory's rendered character order is exactly `.agents/skills/pr-fixup`, the filename remains `SKILL.md`, and the accessible header label retains the full path.
- **Mobile scenario:** update `apps/web/e2e/tests/review/mobile-review-file-status.spec.ts` to use a long dot-prefixed directory, assert exact rendered character order, and retain its existing sticky-header, touch-target, actions-menu, and no-horizontal-overflow checks.

## Verification Results

- RED component proof: `cd apps && pnpm --filter @kandev/web test -- components/review/review-diff-header.test.tsx` failed the two new desktop/mobile cases while the six prior cases passed. Desktop lacked the shared directory hook; mobile lacked the LTR isolate.
- RED browser proof: after a test-only locator refinement, `cd apps/web && pnpm e2e:run --no-build tests/review/review-file-status.spec.ts -- --workers=1 --retries=0` failed with expected `.agents/skills/pr-fixup` and rendered `agents/skills/pr-fixup.` against the unchanged production build.
- GREEN: the component suite passed 8 tests; desktop Chromium passed 1 test; mobile Chromium passed 1 test. The mobile case also proved real truncation (`scrollWidth > clientWidth`), retained RTL leading-edge clipping, 44 px actions, and no page overflow.
- `pnpm run typecheck`, repository web lint, and `git diff --check` passed. Managed E2E runs removed their failure artifacts and owned runtime directories/processes.

## Implementation Waves And Parallel Candidates

Sequential single-task repair:

- [x] [Task 01: Preserve Review path punctuation](task-01-preserve-review-path-punctuation.md)

No subagent delegation is planned or authorized. Component markup, shared browser assertion, and both responsive regressions form one TDD slice.

## Risks

- Removing RTL direction outright would fix punctuation but regress leading-edge truncation and hide the directory suffix nearest the filename.
- DOM text assertions cannot reproduce this defect because the logical string is already correct; browser range geometry is required for regression coverage.
- Mixed strong right-to-left Unicode path content remains outside this repair's visual-order contract.
- Public docs, localization catalogs, APIs, and persisted state are unaffected.
