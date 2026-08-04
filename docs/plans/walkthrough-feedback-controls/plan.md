---
spec: docs/specs/walkthrough-feedback-controls/spec.md
created: 2026-08-03
status: done
---

# Fix Plan: Walkthrough Feedback Controls

## Root cause

`WalkthroughStepInner` always passes `onCancel={() => {}}` to `CommentForm`.
`CommentForm` always renders a **Cancel** button and routes clicks and Escape to
that callback. The walkthrough note form has no temporary editing or disclosure
state, so the callback is intentionally empty and the rendered control cannot
produce an observable result.

## Overview

Represent cancellation as an optional `CommentForm` capability. Render the
button and handle Escape only when a caller provides a cancellation callback,
then omit the no-op callback from the walkthrough note form. Existing diff and
editor comment call sites continue to provide their real callbacks.

## Frontend

### Shared comment form capability

- `apps/web/components/diff/comment-form.tsx` — make `onCancel` optional, render
  the secondary **Cancel** action only when it exists, and only consume Escape
  when cancellation is available.
- `apps/web/components/diff/walkthrough-step-card.tsx` — remove the no-op
  callback from the persistent walkthrough note form. Keep **Add**, **Run**,
  close, discard, and navigation behavior unchanged.

### Mobile parity

- **Nearest exemplar:** the existing `WalkthroughFloatingWindow` phone bottom
  sheet remains the shipped mobile surface.
- **Presentation:** no composition, navigation, scroll ownership, safe-area, or
  touch-target behavior changes. The inert secondary action disappears while
  **Add** and **Run** remain in the existing action row.
- **Shared behavior:** desktop floating, mobile bottom-sheet, and inline
  walkthrough variants continue to share `WalkthroughStepInner` and its note
  handlers.
- **Rendered proof:** the focused Pixel 5 walkthrough E2E asserts that the
  bottom sheet has no **Cancel** action while retaining the note actions.

## Tests

This is rendered UI behavior with no new pure logic. Per frontend testing
conventions, use Playwright rather than adding a React component test.
`pnpm run typecheck` verifies that all cancellable `CommentForm` consumers still
provide valid callbacks after the prop contract changes. Runtime cancellation
in those unchanged consumers and the legacy inline walkthrough are outside this
fix's acceptance scope.

## E2E Tests

- **Scenario:** desktop walkthrough note controls. **File:**
  `apps/web/e2e/tests/review/walkthrough.spec.ts`. **Proof:** extend the existing
  ask-box test to assert that **Cancel** is absent before proving **Add** and
  **Run** still work.
- **Scenario:** phone walkthrough note controls. **File:**
  `apps/web/e2e/tests/review/mobile-walkthrough.spec.ts`. **Proof:** extend the
  existing bottom-sheet test to assert that **Cancel** is absent and **Add** and
  **Run** remain rendered inside the mobile surface.

## Verification Results

- RED: the focused desktop and mobile tests each failed on the new absence
  assertion with `Expected: 0, Received: 1` while the inert button remained.
- GREEN: the focused Chromium desktop and `mobile-chrome` tests each passed one
  intended test; desktop and mobile screenshots confirmed **Add** and **Run**
  remain visible without **Cancel**.
- Structural inspection confirmed the legacy inline walkthrough shares
  `WalkthroughStepInner`, and existing cancellable `CommentForm` consumers still
  provide callbacks; neither path received separate runtime coverage.
- `pnpm run typecheck` and `pnpm run i18n:ratchet` passed.
- The managed E2E commands exited cleanly, and no matching E2E, Playwright, or
  Vite process remained afterward.

## Implementation Wave

1. [Remove inert walkthrough Cancel](task-01-remove-inert-cancel.md) — done,
   sequential.

## Risks

- Making cancellation optional must not hide **Cancel** from existing diff or
  editor comment forms whose callbacks dismiss or exit editing.
- Escape must remain functional for cancellable forms while no longer being
  intercepted by the non-cancellable walkthrough note form.

## Out of scope

- Refactoring other comment form actions or translating untouched copy.
- Changing walkthrough layout, navigation, persistence, or delivery behavior.
