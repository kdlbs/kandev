---
spec: docs/specs/ui/requirements/walkthrough-navigation-layout.md
created: 2026-07-28
status: done
---

# Fix Plan: Stable Walkthrough Navigation

## Root cause

`WalkthroughStepInner` only constrains its maximum height. Its explanation,
navigation, and comment form therefore size the card together; the body's
`flex-1` has no definite height to fill. A longer or shorter step consequently
moves the navigation row.

## Overview

Give the walkthrough surface a viewport-bounded, definite height and make the
step explanation its single internal scroll owner. Render the navigation as the
final fixed card footer so **Next** has a stable location while the current
step changes. Prove the geometry in existing desktop and mobile walkthrough
Playwright coverage.

## Frontend

### Walkthrough card composition

- `apps/web/components/diff/walkthrough-step-card.tsx` — make the shared inner
  card a definite-height flex column; preserve a scrollable step body and move
  the navigation row into the fixed footer after the feedback form.
- `apps/web/components/diff/walkthrough-floating-window.tsx` — give the
  desktop floating card and phone bottom sheet the available bounded height,
  while retaining the desktop drag position, mobile bottom alignment, and
  app-status-bar clearance.

### Mobile design contract

- **Outcome / entry point:** advance an open walkthrough from its existing
  bottom-sheet launcher.
- **Exemplar:** the existing `WalkthroughFloatingWindow` bottom-sheet
  presentation supplies the surface and dismissal behavior; this change keeps
  its primary action visible rather than introducing a new phone pattern.
- **Hierarchy / primary action:** explanation scrolls above; **Next** remains
  the final, visible primary action.
- **Surface / rationale:** a bottom sheet remains appropriate because the
  walkthrough is a temporary companion to the open code editor, not a primary
  route. One bounded sheet and its interior own the vertical scroll.
- **Geometry:** use dynamic viewport sizing, preserve safe-area/status-bar
  clearance, keep controls at least their existing touch target size, and keep
  document horizontal overflow at zero.

## Tests

- No new non-trivial pure helper is introduced; the regression is rendered
  geometry and is covered at the browser level.

## E2E Tests

- **Scenario:** desktop steps with different content lengths. **File:**
  `apps/web/e2e/tests/review/walkthrough.spec.ts`. **Proof:** compare the
  navigation-footer bounding box before and after advancing, then assert the
  next step renders.
- **Scenario:** mobile bottom-sheet walkthrough. **File:**
  `apps/web/e2e/tests/review/mobile-walkthrough.spec.ts`. **Proof:** advance
  through steps and assert the navigation footer remains visible, contained by
  the viewport, and in the same card position.

## Implementation Wave

1. [Stable navigation footer](task-01-stable-navigation-footer.md) — done, sequential.

## Risks

- A fixed height can hide long text if the body is not the sole scroll owner.
- Phone viewport and app-status-bar offsets can put a bottom action behind
  browser chrome if dynamic height and existing bottom clearance are not kept.
- Hiding an occluded anchor must not also hide the editor range indicator;
  preserve the range while clearing only the connector anchor.
