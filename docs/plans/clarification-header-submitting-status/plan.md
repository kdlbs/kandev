---
created: 2026-08-30
status: done
requirements:
  - REQ-UI-CLARIFICATION-SUBMIT-FEEDBACK-001
system_design:
  - ../../specs/ui/system-design/clarification-submit-feedback.md
legacy_specs: []
---

# Implementation Plan: Clarification Header Submitting Status

## Overview

Move the clarification's animated pending indicator into the shared expanded
header immediately before Skip. Implement the shared component and its focused
test first, then update the existing desktop and mobile Playwright coverage so
single-question auto-submit and multi-question explicit submit prove the same
status contract.

## Scope

### In scope

- Show one translated, accessible spinner in the expanded clarification header
  for every in-flight answer request.
- Preserve the multi-question Submit button's disabled pending label while
  moving its animated indicator to the shared header position.
- Cover task chat and Quick Chat through their shared clarification component.
- Preserve desktop density, mobile action-row geometry, coarse-pointer targets,
  and the existing response lifecycle.

### Out of scope

- Backend, API, persistence, WebSocket, and submission-state changes.
- New or changed translations.
- New single-question Submit controls or collapsed-header pending feedback.
- Changes to Skip, Collapse, custom-answer Send, or retry semantics.

## Technical approach

### Shared clarification header

- Update
  `apps/web/components/task/chat/clarification-overlay-header.tsx` so
  `ClarificationHeaderActions` conditionally renders `Spinner` as a sibling
  immediately before `ClarificationSkipButton`.
- Give the spinner `data-testid="clarification-submitting-status"` and the
  translated `task:submitting` value as its accessible label. Keep it exposed
  to assistive technology for every bundle.
- Keep the multi-question button's pending text and disabled state, and give it
  the normal translated `task:submit` value as a stable accessible name while
  its visible label changes. Replace its in-button pending spinner branch with
  no icon so only the header status animates, while the idle `IconCheck` remains
  unchanged.
- Do not add viewport-specific state or composition. The current action group
  already positions new siblings before Skip on desktop and in the mobile
  action row.

### Focused component proof

- Extend
  `apps/web/components/task/chat/clarification-panel-section.test.tsx` with a
  held single-question response. Assert the header status is absent at idle,
  appears after choosing an option, precedes Skip in DOM order, and exposes the
  translated submitting label.
- Keep the response pending for the assertion and resolve it during test cleanup
  so the test does not leak asynchronous work.

### Desktop and mobile E2E proof

- Add a held-response single-question scenario to
  `apps/web/e2e/tests/chat/clarification.spec.ts`. Assert the status is inside
  the expanded header, precedes Skip, then disappears through normal resolution.
- Update the existing multi-question submitting test in the same file to scope
  the status to the header rather than the Submit button while retaining the
  button label, disabled state, and missing-check assertions.
- Add the equivalent single-question scenario to
  `apps/web/e2e/tests/chat/mobile-clarification.spec.ts`. Use the configured
  Pixel 5 project and assert control order, 44px Skip and Collapse targets,
  containment, and no document horizontal overflow.
- Update the existing mobile multi-question geometry scenario for the header
  status location without weakening its action-row and stable-height checks.
- Add a `clarificationSubmittingStatus()` locator to
  `apps/web/e2e/pages/session-page.ts` if repeated selector use makes the page
  object clearer.

## Tests

- `AC-UI-CLARIFICATION-SUBMIT-FEEDBACK-001.1` through `.7`: focused component
  coverage in `clarification-panel-section.test.tsx` proves immediate
  single-question state mapping, ordering, accessible naming, multi-question
  stable button naming, and failure recovery.
- `AC-UI-CLARIFICATION-SUBMIT-FEEDBACK-001.1` through `.8`: desktop Playwright
  coverage proves both auto-submit and explicit multi-question submit against a
  held response.

## E2E tests

- `AC-UI-CLARIFICATION-SUBMIT-FEEDBACK-001.1` through `.8`: Chromium runs the
  single- and multi-question submitting-status scenarios in
  `tests/chat/clarification.spec.ts`.
- `AC-UI-CLARIFICATION-SUBMIT-FEEDBACK-001.8` through `.10`: `mobile-chrome`
  runs the single-question status scenario and the existing multi-question
  geometry scenario in `tests/chat/mobile-clarification.spec.ts`.

## Work orders

- [x] [Task 01: Show clarification header submitting status](task-01-show-header-submitting-status.md)

## Verification results

Implementation and focused verification passed:

- Dependencies bootstrapped with `pnpm install --frozen-lockfile` from `apps/`.
- Focused clarification component tests passed: 14 tests.
- Web typecheck passed.
- Focused ESLint and Prettier checks passed for the changed web and E2E files.
- Focused Chromium E2E passed: 2 tests covering single-question header status
  and multi-question submitting behavior.
- Focused `mobile-chrome` E2E passed: 2 tests covering single-question header
  status and multi-question mobile geometry.
- `git diff --check` passed.

## Risks

- The design-system spinner defaults to an English accessible label. The
  implementation must override it with the existing translated submitting
  value instead of hiding it. Multi-question Submit must keep a stable
  translated accessible name while its visible pending label changes.
- Multi-question tests currently scope the spinner inside Submit; they must be
  updated together with the component so assertions cannot pass on a stale or
  duplicate indicator or unstable button name.
- Held-response tests must always release their route in cleanup to avoid
  hanging the managed Playwright worker.
