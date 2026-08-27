---
spec: docs/specs/ui/requirements/clarification-submit-feedback.md
created: 2026-08-23
status: done
---

# Implementation Plan: Clarification mobile Submit target

## Overview

Increase touch targets in the shared multi-question clarification header and
separate phone progress from batch actions. Add phone geometry regression
coverage first, then preserve compact desktop behavior. No backend, state, API,
or copy changes are required.

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

### Shared clarification header

- Update
  `apps/web/components/task/chat/clarification-overlay-header.tsx` so the
  existing button gains a 44px minimum height only for coarse pointers, using
  the repository's pointer-media utility pattern.
- Apply the height to both idle and submitting states. This avoids a layout
  jump when the check icon changes to the longer label and spinner.
- On phone viewports, place progress above a full-width batch-action row. Let
  Submit use available width and keep Skip and Collapse at least 44px square.
- Preserve fine-pointer sizing, labels, spinner, disabled behavior, colors,
  handlers, and compact inline placement.

### Mobile design contract

- **Desktop outcome:** clarification answers submit through the existing compact
  header button; geometry and behavior stay unchanged.
- **Mobile entry point:** existing multi-question clarification header in task
  chat or Quick Chat.
- **Nearest shipped exemplar:** the same header's 44px collapse control supplies
  the vertical geometry; coarse-pointer action sizing in
  `queued-ghost-panel-header.tsx` supplies the responsive utility pattern.
- **Hierarchy and primary action:** Submit remains the labelled primary action
  in a distinct action row beside secondary Skip and Collapse controls.
- **Presentation:** phone progress and actions use two rows; desktop retains the
  compact inline header. No drawer or navigation change is needed.
- **Scroll, viewport, and safe area:** existing clarification scroll region
  remains the sole owner; no fixed positioning or safe-area behavior changes.
- **Touch target:** Submit is at least 44px tall on coarse pointers. Skip and
  Collapse are at least 44px in each dimension. All remain contained without
  document horizontal overflow.
- **Shared logic:** `useClarificationGroup`, answer state, and handlers remain
  shared across viewports; only presentation changes.
- **Mobile proof:** Pixel 5 E2E measures idle and pending Submit-button height,
  checks stable geometry, and completes the existing submission flow.

## Tests

- **What:** phone progress stays above the batch actions; Submit remains at
  least 44px tall before and during submission; Skip and Collapse remain at
  least 44px square; no pending-state height collapse or overflow occurs.
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
- Post-feedback refinement split phone progress and actions into separate rows,
  expanded secondary action targets, and passed the focused Pixel 5 test.
- Existing fine-pointer desktop pending-state E2E passed after a fresh
  production build: 1 test.
- `cd apps/web && pnpm run typecheck` passed.
- Focused ESLint and Prettier checks passed for the changed component and E2E.
- `git diff --check` passed.
- Fresh pending-state capture showed a full-width Submit action below progress,
  separate 44px Skip and Collapse targets, and no clipping or overflow.
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
