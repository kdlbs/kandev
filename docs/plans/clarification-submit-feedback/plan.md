---
spec: docs/specs/ui/requirements/clarification-submit-feedback.md
created: 2026-08-20
status: done
---

# Implementation Plan: Clarification submit feedback

## Overview

Update the shared clarification overlay header to use the existing design-system
`Spinner` while a multi-question answer submission is pending. Extend the
existing desktop clarification E2E coverage and add the same pending-state
assertion to the mobile clarification flow. No backend, state, translation, or
responsive composition changes are required.

## Frontend

### Shared clarification submit button

- `apps/web/components/task/chat/clarification-overlay-header.tsx`
  - Import `Spinner` from `@kandev/ui/spinner`.
  - Render the spinner instead of `IconCheck` when `isSubmitting` is true.
  - Mark the decorative spinner `aria-hidden` so the translated submitting
    label remains the accessible status without exposing the shared English
    `Loading` label.
  - Preserve the existing translated labels, disabled state, tooltip gating,
    button dimensions, and idle check icon.
- The component is shared by task chat and Quick Chat, so both surfaces inherit
  the same behavior. No `useResponsiveBreakpoint` branch is needed because the
  change is an inline status indicator inside an existing control.

### Mobile parity contract

- Desktop outcome: the operator can see that the batch answer request is still
  in flight and cannot submit it again.
- Mobile entry point: the existing clarification overlay Submit button in the
  shared chat panel; no new navigation or surface is introduced.
- Nearest shipped exemplar: existing `Spinner` usage inside submit controls in
  `apps/web/components/settings/system/bundle-customizer.tsx`.
- Mobile hierarchy, scroll owner, safe area, and touch target remain unchanged;
  the status icon stays inside the existing reachable button.
- Shared state and handler logic remain in `useClarificationGroup` and the
  existing `ClarificationHeaderActions` props.

## Tests

- **What:** The shared submit button shows an animated loading status, keeps the
  `Submitting...` label, remains disabled, and hides the idle check icon while a
  response is held.
  **File:** `apps/web/e2e/tests/chat/clarification.spec.ts`
  **How:** Extend the existing “question shortcuts stay disabled while answers
  are submitting” test, which already holds the response route open, to assert
  a `role="status"` spinner inside `clarification-submit`.
- **What:** The same loading status is reachable and contained on a phone.
  **File:** `apps/web/e2e/tests/chat/mobile-clarification.spec.ts`
  **How:** Add a focused multi-question mobile scenario that answers the
  bundle, holds the response route, and asserts the spinner inside the Submit
  button before releasing the response.

## E2E Tests

- **Scenario:** GIVEN a multi-question clarification is fully answered on
  desktop, WHEN its response is held in flight, THEN the Submit button shows
  `Submitting...`, is disabled, and contains the loading status.
  **File:** `apps/web/e2e/tests/chat/clarification.spec.ts`
- **Scenario:** GIVEN the same fully answered clarification is shown on a
  Pixel 5 viewport, WHEN its response is held in flight, THEN the Submit button
  remains visible and contains the loading status without page overflow.
  **File:** `apps/web/e2e/tests/chat/mobile-clarification.spec.ts`

## Verification Results

Passed.

- `cd apps && pnpm install --frozen-lockfile` passed.
- `cd apps/web && pnpm run typecheck` passed.
- `cd apps/web && pnpm run lint` passed with no errors or warnings.
- `cd apps/web && pnpm exec prettier --check components/task/chat/clarification-overlay-header.tsx e2e/tests/chat/clarification.spec.ts e2e/tests/chat/mobile-clarification.spec.ts` passed.
- `cd apps/web && pnpm e2e:run --project chromium tests/chat/clarification.spec.ts -- --grep "question shortcuts stay disabled while answers are submitting"` passed (1 test).
- `cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-clarification.spec.ts -- --grep "loading spinner"` passed (1 test).
- `cd ../.. && git diff --check` passed.
- The managed E2E runs rebuilt the backend and pseudo-locale Vite assets and cleaned their isolated test artifacts. No blockers remain.
- Fresh phone and desktop pending-state screenshots were captured with synthetic E2E data, visually inspected, compressed, and validated through `apps/web/.pr-assets/manifest.json`; the disposable capture specs were removed afterward.
- Fixup: addressed the P2 accessibility review by hiding the decorative spinner from assistive technology; the translated `Submitting...` button text remains exposed.
- Fixup verification: `cd apps/web && pnpm run typecheck`, `cd apps/web && pnpm run lint`, and the focused desktop/mobile E2E commands above all passed.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [task-01-submit-feedback](task-01-submit-feedback.md)

The task is sequential because the component and both E2E assertions form one
small vertical slice.

## Open Questions

None.
