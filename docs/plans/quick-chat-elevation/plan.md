---
spec: docs/specs/ui/quick-chat-elevation.md
created: 2026-08-04
status: done
---

# Implementation Plan: Quick Chat elevation

## Overview

Reuse the existing Radix dialog overlay and Quick Chat panel shadow. Change only the Quick Chat
overlay from transparent to the established subtle backdrop treatment, then add focused desktop
rendered coverage and retain the existing mobile entry/containment coverage. The change is a single
frontend vertical slice with no backend, state, API, or persistence work.

## Backend

No backend changes. Quick Chat state and session behavior are unchanged.

## Frontend

### Quick Chat dialog surface

- Update `apps/web/components/quick-chat/quick-chat-modal.tsx` so the `DialogContent` overlay uses
  the existing subtle backdrop treatment (`bg-black/20`) instead of `bg-transparent`.
- Keep the existing `shadow-2xl` panel class, responsive dimensions, full-screen phone layout,
  focus behavior, and close controls unchanged.

## Tests

- **What:** Opening Quick Chat on a tablet/desktop viewport renders a non-transparent backdrop and
  leaves the dialog panel elevated with a non-empty box shadow; closing it removes the backdrop.
  **File:** `apps/web/e2e/tests/chat/quick-chat.spec.ts`.
  **How:** Open the existing Quick Chat setup with the shared helper, inspect the portaled
  `[data-slot="dialog-overlay"]` and dialog computed styles, then close and assert both are no
  longer visible. The new test must fail before the overlay class change because the backdrop is
  currently transparent.
- **What:** The mobile Quick Chat path still opens from a touch entry point, stays viewport
  contained, and closes through its explicit control.
  **File:** existing `apps/web/e2e/tests/chat/mobile-quick-chat-entry.spec.ts` coverage.
  **How:** Run the existing mobile scenario; no alternate mobile composition or new test file is
  needed because this visual change does not alter the full-screen phone surface.

## E2E Tests

- **Scenario:** GIVEN Quick Chat is closed on a tablet or desktop viewport, WHEN the user opens it,
  THEN the dialog is visibly elevated above a non-transparent backdrop.
  **File:** `apps/web/e2e/tests/chat/quick-chat.spec.ts`.
  **What to verify:** The dialog is visible, the portaled overlay has a non-transparent computed
  background, and the panel's computed `box-shadow` is not `none`.
- **Scenario:** GIVEN a phone viewport, WHEN the user opens Quick Chat from the Home header, THEN
  the existing full-screen dialog and close control remain usable without document horizontal
  overflow.
  **File:** `apps/web/e2e/tests/chat/mobile-quick-chat-entry.spec.ts`.
  **What to verify:** Existing mobile entry, containment, close, and overflow assertions continue
  to pass.

## Implementation Waves

- [x] [Task 01: Add Quick Chat elevation](task-01-quick-chat-elevation.md)

## Verification Results

Completed implementation and focused verification:

- `pnpm install --frozen-lockfile` — passed.
- `pnpm --filter @kandev/web run typecheck` — passed.
- RED proof: the new desktop E2E failed before the class change because the overlay computed to
  `rgba(0, 0, 0, 0)`.
- `pnpm --dir web e2e:run --project chromium tests/chat/quick-chat.spec.ts -- --grep "elevation"` —
  passed, 1 test.
- `pnpm --dir web e2e:run --project mobile-chrome tests/chat/mobile-quick-chat-entry.spec.ts` —
  passed, 4 tests.
- Managed PR capture — passed, with desktop and mobile Quick Chat screenshots in
  `apps/web/.pr-assets/`.

## Risks

- The backdrop is visible around the panel on tablet/desktop but naturally hidden behind the
  full-screen phone composition; the mobile check must verify usability rather than expect visible
  page dimming.
- The overlay remains a Radix dialog overlay, so changing only its color must preserve focus
  trapping, Escape dismissal, and page interaction blocking.
