---
spec: docs/specs/ui/clarification-submit-feedback.md
created: 2026-08-23
status: done
---

# Implementation Plan: Clarification mobile Submit target

## Overview

Increase only the touch/coarse-pointer height of the shared multi-question
clarification Submit button. Add the phone geometry regression first, apply the
responsive class change, then run focused desktop and mobile submission checks.
No backend, state, API, copy, or surface-composition changes are required.

## Root cause

`ClarificationHeaderActions` in
`apps/web/components/task/chat/clarification-overlay-header.tsx` renders the
Submit button with `text-xs px-3 py-1`. Its 16px line box plus 4px vertical
padding on each side produces a roughly 24px-tall button on every viewport.
The same header renders the collapse button at 44px (`h-11 w-11`), exposing the
height mismatch shown in the phone screenshot. The existing mobile loading
test verifies spinner state and horizontal overflow, but never checks the
Submit button's bounding box.

## Frontend

### Shared clarification Submit button

- Update
  `apps/web/components/task/chat/clarification-overlay-header.tsx` so the
  existing button gains a 44px minimum height only for coarse pointers, using
  the repository's pointer-media utility pattern.
- Apply the height to both idle and submitting states. This avoids a layout
  jump when the check icon changes to the longer label and spinner.
- Preserve current fine-pointer sizing, labels, spinner, disabled behavior,
  colors, horizontal padding, handler, and header placement.

### Mobile design contract

- **Desktop outcome:** clarification answers submit through the existing compact
  header button; geometry and behavior stay unchanged.
- **Mobile entry point:** existing multi-question clarification header in task
  chat or Quick Chat.
- **Nearest shipped exemplar:** the same header's 44px collapse control supplies
  the vertical geometry; coarse-pointer action sizing in
  `queued-ghost-panel-header.tsx` supplies the responsive utility pattern.
- **Hierarchy and primary action:** Submit remains the labelled primary action
  beside secondary Skip and Collapse controls.
- **Presentation:** unchanged inline header. A drawer or navigation change would
  add unnecessary depth to this frequent, one-step action.
- **Scroll, viewport, and safe area:** existing clarification scroll region
  remains the sole owner; no fixed positioning or safe-area behavior changes.
- **Touch target:** Submit is at least 44px tall on coarse pointers and remains
  contained without document horizontal overflow.
- **Shared logic:** `useClarificationGroup`, answer state, and handlers remain
  shared across viewports; only presentation changes.
- **Mobile proof:** Pixel 5 E2E measures idle and pending Submit-button height,
  checks stable geometry, and completes the existing submission flow.

## Tests

- **What:** the phone Submit button stays at least 44px tall before and during
  submission, with no pending-state height collapse.
  **File:** `apps/web/e2e/tests/chat/mobile-clarification.spec.ts`.
  **How:** extend the existing held-response loading-spinner scenario to record
  the idle and pending `boundingBox()` values, assert pending height is at least
  44px, and assert both heights match within 1px. Write and run this assertion
  before the component change; current CSS must fail at roughly 24px.
- **What:** existing desktop loading feedback and submission behavior remain
  intact.
  **File:** `apps/web/e2e/tests/chat/clarification.spec.ts`.
  **How:** run the existing held-response submitting-state scenario unchanged.

## E2E Tests

- **Scenario:** GIVEN a fully answered clarification on a Pixel 5 viewport,
  WHEN the answer request remains pending, THEN the Submit button remains at
  least 44px tall, keeps its idle height, shows its loading status, and creates
  no document overflow.
  **File:** `apps/web/e2e/tests/chat/mobile-clarification.spec.ts`.
- **Scenario:** GIVEN the same clarification on a fine-pointer desktop,
  WHEN submission remains pending, THEN the existing compact control still
  shows its disabled loading feedback and completes normally.
  **File:** `apps/web/e2e/tests/chat/clarification.spec.ts`.

## Verification Results

Passed.

- Dependency bootstrap passed: `cd apps && pnpm install --frozen-lockfile`.
- RED confirmed on Pixel 5: pending Submit height was 24px against the 44px
  requirement; both configured attempts failed on the intended assertion.
- GREEN passed on Pixel 5 after a fresh production build, then passed again
  after capture-only code was removed: 1 test each run.
- Existing fine-pointer desktop pending-state E2E passed after a fresh
  production build: 1 test.
- `cd apps/web && pnpm run typecheck` passed.
- Focused ESLint and Prettier checks passed for the changed component and E2E.
- `git diff --check` passed.
- Captured pending state at
  `/tmp/kandev-mobile-clarification-submit-green.png`; visual inspection showed
  the 44px button aligned with the collapse control, centered label/spinner,
  and no clipping. Temporary capture code and PNG were removed.
- Managed E2E runs cleaned their isolated backends. Build outputs remain in
  ignored `apps/web/dist/` and backend build paths.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [Task 01: Expand mobile Submit target](task-01-expand-mobile-submit-target.md) (`done`)

Single sequential task. Component and E2E touch the same behavior and must
preserve the red-green ordering.

## Risks

- Translated labels may be wider than English. The change must grow only
  vertical touch geometry and retain the existing no-horizontal-overflow proof.
- Pointer media, not phone width alone, intentionally includes touch tablets.
- No subagents are authorized; execution remains sequential in the primary
  conversation.

## Open Questions

None.
