---
spec: docs/specs/ui/requirements/changes-walkthrough-toolbar-width.md
created: 2026-07-31
status: implemented
---

# Implementation Plan: Responsive Changes Walkthrough Action

## Overview

Replace the Walkthrough label's viewport breakpoint with a named inline-size
container rooted at the Changes panel. Prove the 349px/350px boundary and
toolbar containment in a real browser before applying the minimal class-name
change, then rerun the existing mobile request flow with responsive geometry
assertions.

## Confirmed root cause

`ChangesPanelWalkthroughButton` uses `min-[430px]`, which queries the browser
viewport. The desktop Changes panel is a nested Dockview pane whose width can be
349px while the laptop viewport remains much wider, so the label stays rendered
and `PanelHeaderBarSplit` clips the left slot to preserve its right-side branch
and Pull actions.

## Frontend

### Changes panel container

- Mark the desktop `PanelRoot` in
  `apps/web/components/task/changes-panel.tsx` and the phone `PanelRoot` in
  `apps/web/components/task/mobile/mobile-changes-panel.tsx` as the same named
  inline-size container.
- In `apps/web/components/task/changes-panel-header.tsx`, keep the label hidden
  by default and reveal it only when that named panel container is at least
  350px wide.
- Preserve the icon, tooltip, accessible name, click handler, disabled state,
  and the existing `PanelHeaderBarSplit` overflow policy.

### Mobile design contract

- **Desktop outcome:** the action is icon-only at 349px and narrower, and
  icon-plus-label at 350px and wider.
- **Mobile entry point:** the existing **Changes** item in
  `SessionMobileBottomNav` opens `MobileChangesPanel`; no navigation changes.
- **Nearest shipped exemplar:** `mobile-changes-panel.tsx` continues to provide
  the focused, full-width phone Changes surface and shared action behavior.
- **Hierarchy and primary action:** Diff, Review, Walkthrough, branch, and Pull
  remain in the existing inline header; requesting a walkthrough remains the
  action's only outcome.
- **Presentation rationale:** this is a compact, frequent toolbar action, so an
  inline icon fallback is preferable to adding an overflow drawer or route.
- **Geometry:** scroll ownership, dynamic viewport sizing, safe areas, and touch
  targets are unchanged; the test guards panel containment and document-level
  horizontal overflow.
- **Shared logic:** request state and handlers remain shared; only responsive
  presentation changes.
- **Mobile proof:** extend the existing Pixel 5 walkthrough-request scenario to
  assert the visible label fits and the document does not scroll horizontally.

## Tests

No unit test is appropriate because jsdom does not evaluate CSS container
queries or rendered geometry. The behavior is covered at the browser level.

## E2E Tests

- **Scenario:** at 349px the action is icon-only and contained; at 350px its
  label appears and remains contained, even on a wide laptop viewport.
  - **File:** `apps/web/e2e/tests/review/walkthrough.spec.ts`
  - **Method:** resize the right Dockview column through the existing
    `resizeColumnViaSplitview` helper, assert the panel's rendered width, label
    visibility, accessible button visibility, and bounding-box containment.
- **Scenario:** the Pixel 5 Changes surface remains usable when its 393px panel
  shows the label.
  - **File:** `apps/web/e2e/tests/review/mobile-walkthrough.spec.ts`
  - **Method:** extend the existing request test with label visibility,
    bounding-box containment, and zero document-level horizontal overflow
    before requesting the walkthrough.

## Implementation Tasks

- [x] [Task 01: Make the Walkthrough action panel-responsive](task-01-panel-responsive-walkthrough.md)

Execution is sequential in the primary conversation. No subagent delegation is
planned or authorized.

## Risks

- Container queries measure the nearest named container's content box. Naming
  the `PanelRoot`, rather than the padded header, keeps the 349px/350px contract
  aligned with the user-observed panel width.
- Dockview can round resize targets. The E2E regression must poll the rendered
  panel width and fail with the observed value rather than masking it with a
  broad tolerance.
