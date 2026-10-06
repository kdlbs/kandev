---
created: 2026-10-06
status: completed
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
system_design:
  - ../../specs/platform/system-design/git-refresh-continuity.md
legacy_specs: []
---

# Implementation Plan: Git Refresh Reading Continuity

## Overview

Keep existing file counts and diff content visible during background refresh.
Leave identical returned content mounted and preserve the reading anchor when patches change.
One work order implements the shared projection, renderer continuity, and desktop/phone proof together.

## Scope

In scope: workspace Changes counts, open single-file and combined Review diffs, mixed layers, and both shipped diff renderers.
Include pending/failure freshness, identical-result view state, changed-result scroll anchors, and scope cleanup.
Retain the current phone drawer, desktop dockview layout, toolbar feedback, and recovery schedule.

Out of scope: backend publication changes, wire formats, new polling, persistent diffs, historical commit/PR cache redesign, and unrelated UI layout.

## Evidence and assumptions

The user requests readable old content throughout refresh and updates only for new content.
The user also requests preservation of scroll position where possible.
Platform owns this repair because it owns workspace Git freshness and preservation across observation phases.
No unanswered product choice blocks planning.

`applyGitStatus` replaces prior details with pending metadata, and `FileRowStats` hides pending counts.
The earlier investigation reproduced count disappearance and restoration on desktop and phone.
Three temporary checks and 32 existing targeted tests passed.
The second temporary test rendered `ReviewFileDiffContent` through ready, pending, and identical ready data.
The mocked viewer's mount count increased from one to two; the pending render displayed the real loading placeholder.
That run passed one diagnostic test and eight existing store/Review tests. Temporary tests were removed.
These checks prove renderer replacement, not physical browser scroll behavior. Playwright supplies that remaining evidence during implementation.

Existing Platform criteria `.25` and `.29` already require prior-data preservation.
Criteria `.47` through `.49` make count/patch continuity and reading-position outcomes explicit.
The companion [loading package](../changes-loading-feedback/plan.md) retains toolbar ownership.
The [progressive publication package](../changes-panel-git-refresh/plan.md) remains the historical backend delivery record.
Neither package's execution results change here.

## Technical approach

Keep canonical snapshots unmodified and add the shared runtime display companion described in the design.
Update only accepted snapshots; preserve individual eligible representations during pending/failure, keep compact and faceted flat-patch identity, and retire removed or replaced targets.
Integrate the projection with Changes counts, mixed-layer selection, review-source conversion, and review progress.
Keep current readiness separate from displayed patch values, including patch-dependent action guards.

Keep `FileDiffViewer` mounted and its transformed data stable for identical content.
Capture visible line content and nearby same-side context, then restore that content and viewport offset after changed patches, including insertions and deletions above the anchor.
Use Monaco view-state APIs for its internal scroll and selections; use Pierre line metadata and code content for its rendered anchors.
Avoid freshness-triggered navigation, expansion resets, review marking, or focus changes.
Reuse existing localized loading/unavailable copy. Add complete locale catalogs only if new copy is necessary.

| Renderer/surface | Scroll owner | Evidence | Fallback |
| --- | --- | --- | --- |
| Pierre single-file/combined diff | `ReviewDiffList` container | Content identity and viewport geometry | Nearest surviving same-side context, then numeric/clamped offsets |
| Monaco single-file/combined diff | Editor internal view plus list container | Content-mapped view state and browser geometry | Nearest surviving same-side context, then numeric/clamped offsets |
| Phone full-height drawer | Same shared viewer inside `MobileDiffSheet` | `mobile-chrome` touch/Back/containment checks | Same anchor policy |

## ASCII UI preview

UI-01: Desktop file diff and Changes, refresh after ready content.

```text
BEFORE
| Diff: a.ts                  | Changes (spinner) |
| Diff is loading             | a.ts       [M]    |

AFTER
| Diff: a.ts  (refresh status) | Changes (spinner) |
| 40  old/new readable line    | a.ts +18 -3 [M]   |
| 41  next readable line       | b.ts  +2 -0 [M]   |
| <same reading position>     |                   |
```

UI-02: Phone file tap, existing full-height diff drawer.

```text
| File changes       Close |
| a.ts     (refresh status)|
| 40 readable diff line    |
| 41 readable diff line    |
| <existing content scroll>|
```

The headers stay fixed. Existing content scroll owners remain.
Refresh status does not replace the body or add a padded content row.
Phone entry, hierarchy, Back/dismiss, and safe areas follow `MobileDiffSheet`.
Initial loading without previous content retains its existing placeholder.
Failed refresh keeps readable content and displays unavailable freshness.
Changed ready content replaces lines while preserving their anchor; identical content leaves the viewer alone.
These structures are required by `.47` through `.49`; ASCII spacing and shown text are illustrative.

## Tests

| Criteria | Required regression evidence |
| --- | --- |
| .21, .25, .27, .47 | New `git-status-display-state.test.ts`: accepted-only retention, independent layers/repos, cleanup, ready empty data, failed detail |
| .10, .11, .47 | Existing facet and Changes tests: separate staged/unstaged values and consumed-layer removal |
| .24, .29, .47, .48 | New `review-file-diff-content.test.tsx`: same mounted viewer through pending/failure and identical completion; initial placeholder and action guards |
| .48, .49 | New `use-review-scroll-anchor.test.ts`: stable anchor, removal fallback, changed upper sections, target replacement, user-scroll cancellation |
| .48, .49 | New `monaco-diff-viewer.test.tsx`: unchanged model/view state and changed-result restoration |
| .47, .48 | Pierre and Monaco comment-provider tests: ready to stale/unavailable to ready, no stale gutter selection or draft submission, and draft reuse after readiness returns |
| .47, .48 | Existing `changes-panel-file-row.test.tsx`, review progress and layer tests: retained desktop/phone counts and no false review certification |

The regression named `keeps ready diff mounted through pending and identical ready refresh` must fail before the correction.
The counts regression must also fail on the current desktop pending gate.
Keep the existing raw-store test that forbids old enrichment in new authoritative snapshots.

## E2E tests

Add `tests/git/diff-refresh-continuity.spec.ts` for `chromium` and `tests/git/mobile-diff-refresh-continuity.spec.ts` for `mobile-chrome`.
Share controlled-enrichment and reading-position helpers without introducing arbitrary sleeps.
The existing transport bridge suppresses pending notifications; the new flow must forward or observe them faithfully.

Both suites cover a long ready diff, manual scroll, held enrichment after an unrelated-file edit, and retained counts/content.
Then cover identical selected-file completion, changed selected-file completion, failed detail, and confirmed removal.
For changes above the visible line, assert the same identifiable content remains within two CSS pixels of its old viewport offset even when its line number changes.
For removed anchors, check the documented nearest-content/numeric/clamped fallback rather than exact old scrollTop.
Include changed preceding sections in a combined diff, mixed-layer identity, and checkout/comparison replacement.
Run the core unchanged/changed reading flow under Pierre and Monaco.
Phone checks include the actual full-height drawer, narrow fine-pointer entry, touch scrolling, dismissal/focus return, and no document overflow.

## Work orders

- [x] [Task 01: Preserve counts, diff content, and reading position](task-01-preserve-reading-state.md)

## Verification results

Implementation is complete. The focused Vitest suite passed 21 files / 175 tests, and the final Monaco view-state regression rerun passed 4 tests. TypeScript typecheck and the production Vite build passed. i18n checks, documentation validation, full specification lint, and `git diff --check` passed. Strict ESLint passed with zero warnings across all touched frontend files. The desktop Chromium and mobile Chrome continuity suites each passed 2 tests with capture enabled. All 16 captured PNGs for both renderers and surfaces are listed in `apps/web/.pr-assets/manifest.json`.
The PR fixup regression suite passed 5 files / 55 tests, including the raw checkout-status invalidation boundary, stale review controls on desktop and phone layouts, and scroll-restoration cancellation.

Package validation passed: specification catalog, full specification lint, 36 specification-linter tests, and diff whitespace checks.
The repository documentation-coverage validator accepted the work order references using the planned renderer change as its coverage trigger.

## Risks

- Scope eligibility must retire old representations on checkout and comparison changes without treating enrichment-only missing metadata as replacement.
- Monaco owns internal view state; external list scrollTop alone does not prove its reading position.
- Changed content above an anchor can alter layout; preserve semantic position after renderer layout completes.
- Retained patches cannot authorize new hunk mutations or review state.
- The old enrichment bridge deliberately drops pending events and can hide this regression unless adjusted for the new tests.
