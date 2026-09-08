---
id: "04-mobile-e2e"
title: "Mobile remote entry coverage"
status: done
wave: 3
depends_on: ["02-explicit-url-entry", "03-resolution-errors-and-retry"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/multi-branch.md"
---

# Task 04: Mobile remote entry coverage

## Acceptance

- Phone E2E proves pasted URL text remains editable until Enter and the hint is visible.
- Phone E2E proves a transient resolution error preserves the row and Retry completes branch selection.
- The popover/error controls remain viewport-contained, touch-usable, and introduce no document horizontal overflow.

## Verification

- `make build-backend build-web`
- `cd apps && pnpm --filter @kandev/web e2e -- e2e/tests/task/mobile-create-task-remote-repo.spec.ts --project=mobile-chrome`

## Files likely touched

- `apps/web/e2e/tests/task/mobile-create-task-remote-repo.spec.ts`

## Dependencies

- Tasks 02 and 03.

## Inputs

- Mobile design contract in `plan.md`.
- Existing `openRemotePicker` and viewport-containment helpers in the target spec.

## Output contract

Report the exact E2E result, screenshots/failure artifacts if any, risk tags, and uncertainties; update this task to `done` only after targeted verification passes.

## Completion evidence

- **Entry point:** `mobile-create-task-remote-repo.spec.ts` on the `mobile-chrome` project.
- **Result:** mobile E2E passed: 5 tests, with no failure artifacts; fresh PR screenshots exist under ignored `.pr-assets`.
- **Risks/uncertainties:** coverage proves the current Pixel 5 flow and viewport containment; provider responses remain externally dependent.
