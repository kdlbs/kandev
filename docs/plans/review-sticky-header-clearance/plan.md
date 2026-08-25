---
spec: docs/specs/ui/requirements/submodule-review.md
created: 2026-08-21
status: implemented
---

# Implementation Plan: Review sticky header clearance

## Overview

Keep Review's repository-scope header sticky while reserving a separate sticky lane for the current file header. Prove the overlap and hit-target failure first in the existing nested-submodule desktop and phone flows, then add one shared grouped-header offset used by both responsive file-header compositions.

The repair changes only Review header geometry. Repository grouping, labels, ordering, file identity, actions, scrolling ownership, and Changes-panel headers remain unchanged.

## Root cause

`apps/web/components/review/review-diff-list.tsx` renders repository headers when the diff contains multiple repository scopes or its sole scope has a name. In the nested-submodule flow, workspace-root files form an empty-name group that `RepoGroupHeader` labels **Other changes**.

`RepoGroupHeader` in `apps/web/components/review/review-diff-list-groups.tsx` and `ReviewDiffHeader` in `apps/web/components/review/review-diff-header.tsx` both use `position: sticky` with `top: 0`. The repository header has `z-20`, while the file header has `z-10`. Once scrolling clamps both headers to the scroll owner's top edge, the repository header paints over the file name and controls. A single unnamed repository does not reproduce the bug because `showRepoHeaders` is false and no repository header renders.

## Frontend

### Sticky repository and file lanes

- `apps/web/components/review/review-diff-list-groups.tsx`: give `RepoGroupHeader` one explicit, stable header height using the existing spacing scale. Keep its sticky position, content, truncation, borders, and z-index.
- `apps/web/components/review/review-diff-list.tsx`: propagate whether repository headers are rendered into each `FileDiffSection`. Apply matching scroll margin to grouped file sections so programmatic file navigation still aligns the file header beneath the repository lane.
- `apps/web/components/review/review-diff-header.tsx`: use `top: 0` when no repository header exists and the repository-header height as the sticky inset when one does. Reuse the same inset for desktop and mobile header compositions; do not duplicate breakpoint-specific positioning.

### Mobile design contract

- **Desktop outcome:** the repository scope remains visible at the top of the Review diff, with the current file name and actions directly below it and fully clickable.
- **Mobile entry point:** the existing Changes action opens the same full-height Review dialog. No navigation or control moves.
- **Nearest shipped exemplar:** `apps/web/components/review/review-diff-header.tsx` and `apps/web/e2e/tests/review/mobile-submodule-review.spec.ts` remain the phone Review pattern; this repair changes only their shared sticky geometry.
- **Hierarchy and presentation:** repository scope first, current file identity second, diff content third. The existing full-height surface remains appropriate for dense diff content.
- **Scroll, viewport, and touch:** `review-diff-scroll` remains the single vertical scroll owner. Dialog viewport sizing, safe-area behavior, and existing touch-target sizes remain unchanged; the file disclosure control must stay the topmost hit target at its center.
- **Shared behavior:** repository grouping and the grouped-header flag remain shared. Desktop and mobile render different file-header content but consume the same sticky inset.
- **Parity proof:** the existing desktop and `mobile-chrome` nested-submodule scenarios assert identical non-overlap and hit-target contracts for the **Other changes** group.

## Tests

This is browser layout behavior. Happy DOM component tests cannot evaluate sticky positioning, stacking contexts, bounding boxes, or `document.elementFromPoint()`, so no React component test can provide a meaningful regression.

- **What:** a grouped file header receives the repository-header sticky inset while an ungrouped file header retains `top: 0` behavior.
  **File:** exercised through `apps/web/e2e/tests/review/submodule-review-helpers.ts` from both owning E2E specs.
  **How:** scroll the first workspace-root file header to the scroll-owner start, compare the repository and file header bounding boxes, and check that the file disclosure control owns the element at its center point.

## E2E Tests

- **Scenario:** GIVEN root and nested-submodule changes, WHEN the desktop reviewer scrolls the root file beneath **Other changes**, THEN the repository header bottom does not cross the file header top and the file disclosure remains the real hit target.
  **File:** `apps/web/e2e/tests/review/submodule-review.spec.ts` with shared geometry logic in `submodule-review-helpers.ts`.
- **Scenario:** GIVEN the same mixed scopes on a phone, WHEN the reviewer scrolls the root file beneath **Other changes**, THEN the same geometry and hit-target guarantees hold without changing the existing mobile Review composition.
  **File:** `apps/web/e2e/tests/review/mobile-submodule-review.spec.ts` in the `mobile-chrome` project.
- **TDD order:** add both assertions and run them against current source. They must fail because both sticky headers resolve to `top: 0`; then make the minimal source change and rerun both against a fresh production build.

## Verification Results

- RED, desktop: the focused Chromium scenario failed with
  `headersSeparated: false` and `disclosureHit: false` after the root file was
  scrolled under **Other changes**.
- RED, phone: the focused `mobile-chrome` scenario failed with the same two
  values. Both RED commands used `--workers=1 --retries=0`.
- GREEN, desktop: the focused Chromium scenario passed against a fresh
  production build (`1 passed`). Its final section-scroll rerun also passed
  (`1 passed`, 13.8s).
- GREEN, phone: the final `mobile-chrome` section-scroll run passed (`1 passed`,
  9.9s). One earlier attempt never reached the UI because the disposable Git
  fixture encountered an `index.lock`; a clean run with Playwright retries
  still disabled passed.
- Rendered geometry changed from a 30px repository header and `top: 0` file
  header (30px nominal overlap) to a fixed 32px repository lane and 32px grouped
  file inset (0px nominal overlap). Browser geometry and center-point hit tests
  changed from false to true on both viewports.
- `pnpm exec vitest run components/review/review-diff-header.test.tsx` passed all
  8 tests. `pnpm run typecheck`, scoped ESLint, scoped Prettier, and
  `git diff --check` passed.
- Managed E2E runs cleaned their disposable repositories. No generated output
  is present in the tracked diff.

## Implementation Waves And Parallel Candidates

Wave 1 (sequential):

- [x] [Task 01: Separate Review sticky headers](task-01-separate-review-sticky-headers.md)

The E2E regression and shared header geometry describe one visual contract and touch overlapping files, so the task is not parallel-safe.

## Risks

- The grouped offset must match the repository header's rendered height; define both from the same spacing value instead of duplicating unrelated pixel values.
- Programmatic file jumps use `scrollIntoView({ block: "start" })`; grouped sections need matching scroll margin so the fixed lane does not obscure the selected file's first diff content.
- The ungrouped single-repository path must keep its current zero inset and must not gain blank space above file headers.

## Out of scope

- Changing repository grouping, the **Other changes** label, section ordering, or submodule identity.
- Making repository headers non-sticky or changing their z-index hierarchy.
- Redesigning Review, its toolbar, file tree, diff viewer, or mobile navigation.
- Changing the dockview Changes panel, backend Git data, APIs, persistence, or localization.

## Open Questions

None.
