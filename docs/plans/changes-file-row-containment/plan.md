---
created: 2026-08-25
status: complete
requirements:
  - REQ-UI-CHANGES-FILE-ROW-CONTAINMENT-001
system_design:
  - ../../specs/ui/system-design/changes-file-row-containment.md
legacy_specs: []
---

# Implementation Plan: Changes File Row Containment

## Overview

Bring pull-request file rows onto the shrink-safe layout already used by local
file rows, then prove the shared desktop and phone Changes surfaces keep long
paths clear of trailing change metadata. One vertical TDD work order keeps the
component correction and rendered geometry evidence coupled.

## Scope

### In scope

- Make PR file paths shrink and truncate within their leading row region.
- Keep PR line statistics and status markers fixed at the trailing edge.
- Preserve full-path discovery and PR/repository-aware diff opening.
- Add focused desktop and mobile Playwright regressions.

### Out of scope

- Refactoring working-tree and PR rows into a new shared component.
- Changing row density, actions, grouping, status visuals, or data sources.
- Changing Dockview constraints or mobile Changes navigation.

## Technical approach

### Confirmed root cause

`PRFileRow` in
`apps/web/components/task/changes-panel-pr-files.tsx` gives its basename
`whitespace-nowrap shrink-0` and lets the trailing statistics/status container
shrink. At narrow widths the path therefore keeps its intrinsic width and
crosses the trailing metadata. The working-tree `FileRow` had the same defect
corrected in commit `8694de20e` by making both path segments truncatable and
the trailing region `shrink-0`; the PR row retained the older geometry.

### Component correction

- Update only `PRFileRow` layout classes to mirror `FileRow` shrink priority:
  the directory segment yields first, the basename can truncate, and the
  trailing metadata remains fixed.
- Keep the current PR glyph, full-path title, status marker, row click handler,
  and `OpenDiffOptions` unchanged.
- Keep the existing `changes-panel-pr-files.test.tsx` diff-routing assertions as
  a safety check. Do not add a React class-only test for browser flex geometry.

### Rendered regression coverage

- Add a shared E2E seed helper for a linked PR containing a path with a long
  basename and known addition/deletion counts.
- In Chromium, open Changes in a wide desktop task, resize the right Dockview
  column to the supported 180px minimum, and assert the path box ends before
  trailing metadata, the marker remains inside the row and hit-testable, the
  row still opens the PR diff, and the row and document have no horizontal
  overflow.
- In the Pixel 5 project, enter the existing focused mobile Changes surface and
  assert the same geometry, tap outcome, and absence of document overflow.

### Mobile design contract

- Desktop outcome: resizable file rows contain long paths at the legal minimum.
- Mobile entry point: existing task bottom-navigation Changes item.
- Nearest exemplar: shared working-tree `FileRow` geometry inside
  `MobileChangesPanel`.
- Hierarchy and primary action: file identity leads, change evidence trails,
  row tap opens the diff.
- Presentation and rationale: keep the existing inline list because it is the
  focused, frequently scanned Changes content; no new drawer or route.
- Scroll/safe area/touch: existing `PanelBody` remains the single scroll owner;
  dynamic viewport, safe-area, and touch sizing remain unchanged.
- Shared logic: path data and diff handlers stay common across viewports; only
  PR row CSS changes.
- Mobile proof: `mobile-chrome` geometry, overflow, hit-test, and diff-open
  assertions.

## Tests

- No new component test is planned: jsdom cannot prove flex shrink, rendered
  overlap, or hit-testing. The existing
  `apps/web/components/task/changes-panel-pr-files.test.tsx` diff-context test
  remains green as a routing safety check.
- `AC-UI-CHANGES-FILE-ROW-CONTAINMENT-001.1` through `.4` are proved by the
  rendered browser scenarios below.

## E2E tests

- `AC-UI-CHANGES-FILE-ROW-CONTAINMENT-001.1` through `.3`:
  `apps/web/e2e/tests/git/pr-file-row-containment.spec.ts`, Chromium project,
  verifies the 180px Dockview panel geometry and click outcome.
- `AC-UI-CHANGES-FILE-ROW-CONTAINMENT-001.1` through `.4`:
  `apps/web/e2e/tests/git/mobile-pr-file-row-containment.spec.ts`,
  `mobile-chrome` project, verifies Pixel 5 geometry, tap outcome, and document
  containment.

## Work orders

- [x] [Task 01: Contain PR file rows](task-01-contain-pr-file-rows.md)

## Verification results

- RED desktop: the long filename ended at `1743.25px` while additions began at
  `1270.1875px`, proving overlap at the 180px Changes-column minimum.
- RED Pixel 5: the filename ended at `569.25px` while additions began at
  `285.1875px`, proving the shared phone defect.
- GREEN desktop and Pixel 5: both focused Playwright scenarios pass geometry,
  row/document overflow, status hit-testing, full-path title, and diff-opening
  assertions.
- The focused PR-row Vitest suite passes 3 tests. Full web lint and typecheck
  pass.

## Risks

- A component test cannot prove browser flex geometry; Playwright remains the
  acceptance evidence.
- Fixing only the basename or only the trailing region can leave the other flex
  item able to cross or squeeze metadata; tests cover both constraints.
- Desktop and phone share the row component, but separate rendered checks are
  needed because their panel widths and entry paths differ.
