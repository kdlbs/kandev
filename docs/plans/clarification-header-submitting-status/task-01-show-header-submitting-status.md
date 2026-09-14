---
id: "01-show-header-submitting-status"
title: "Show clarification header submitting status"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-CLARIFICATION-SUBMIT-FEEDBACK-001
acceptance_criteria:
  - AC-UI-CLARIFICATION-SUBMIT-FEEDBACK-001.1
  - AC-UI-CLARIFICATION-SUBMIT-FEEDBACK-001.2
  - AC-UI-CLARIFICATION-SUBMIT-FEEDBACK-001.3
  - AC-UI-CLARIFICATION-SUBMIT-FEEDBACK-001.4
  - AC-UI-CLARIFICATION-SUBMIT-FEEDBACK-001.5
  - AC-UI-CLARIFICATION-SUBMIT-FEEDBACK-001.6
  - AC-UI-CLARIFICATION-SUBMIT-FEEDBACK-001.7
  - AC-UI-CLARIFICATION-SUBMIT-FEEDBACK-001.8
  - AC-UI-CLARIFICATION-SUBMIT-FEEDBACK-001.9
  - AC-UI-CLARIFICATION-SUBMIT-FEEDBACK-001.10
system_design:
  - ../../specs/ui/system-design/clarification-submit-feedback.md
---

# Task 01: Show Clarification Header Submitting Status

## Summary

Render the clarification answer's in-flight spinner in the shared expanded
header immediately before Skip, including single-question auto-submit. Prove
the shared state on desktop and mobile while preserving the current submission
lifecycle and responsive geometry.

## In scope

- Add the translated, accessible header status and stable test identifier.
- Remove the duplicate animated icon from the multi-question Submit button
  while retaining its pending label, disabled state, and idle check behavior.
- Add focused component coverage for single-question state and header order.
- Add or update desktop and Pixel 5 Playwright coverage for single- and
  multi-question submission states.

## Out of scope

- Backend or clarification-state changes.
- Translation catalog changes.
- Collapsed-header progress, new controls, or changed Skip, Collapse, Send, and
  retry behavior.

## Acceptance

- A held single-question or multi-question answer request shows exactly one
  animated status in the expanded header immediately before Skip, and idle
  state shows none. The status remains announced for both bundle types.
- The status has the translated submitting accessible name; multi-question
  Submit remains disabled with its visible pending label, stable normal submit
  accessible name, and no idle check.
- Desktop and Pixel 5 flows retain existing control order, touch targets,
  containment, lack of horizontal overflow, successful resolution, and retry
  behavior.

## Verification

Bootstrap once from the repository root if workspace dependencies are absent:

```bash
(cd apps && pnpm install --frozen-lockfile)
```

Run the focused checks from the repository root:

```bash
(cd apps/web && pnpm exec vitest run components/task/chat/clarification-panel-section.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/task/chat/clarification-overlay-header.tsx components/task/chat/clarification-panel-section.test.tsx e2e/pages/session-page.ts e2e/tests/chat/clarification.spec.ts e2e/tests/chat/mobile-clarification.spec.ts)
(cd apps/web && pnpm exec prettier --check components/task/chat/clarification-overlay-header.tsx components/task/chat/clarification-panel-section.test.tsx e2e/pages/session-page.ts e2e/tests/chat/clarification.spec.ts e2e/tests/chat/mobile-clarification.spec.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/clarification.spec.ts -- --grep "header submitting status|question shortcuts stay disabled while answers are submitting")
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-clarification.spec.ts -- --grep "header submitting status|separates batch actions from the stepper")
git diff --check
```

## Files likely touched

- `apps/web/components/task/chat/clarification-overlay-header.tsx`
- `apps/web/components/task/chat/clarification-panel-section.test.tsx`
- `apps/web/e2e/pages/session-page.ts`
- `apps/web/e2e/tests/chat/clarification.spec.ts`
- `apps/web/e2e/tests/chat/mobile-clarification.spec.ts`

## Dependencies

None.

## Risks

- The spinner's default English accessible label must be overridden with the
  existing translated submitting value.
- Response-hold cleanup must release every pending test route after assertions,
  including failure paths.

## Parallelism

`sequential`

## Inputs

- `REQ-UI-CLARIFICATION-SUBMIT-FEEDBACK-001` and all acceptance criteria.
- The paired system design's component, control-flow, responsive, and
  verification sections.
- Existing `useClarificationGroup` submit-state behavior and clarification E2E
  response-hold patterns.

## Results

Implemented the shared expanded-header submitting status. The multi-question
Submit button retains its visible pending label and disabled state with a
stable normal submit accessible name, while single- and multi-question desktop
and mobile coverage verifies the shared header location, accessible label,
control order, touch targets, containment, overflow, and successful resolution.
Focused component coverage also verifies failure recovery. Held-response tests
release their routes in cleanup, including assertion failure paths.

Verification passed:

- `pnpm install --frozen-lockfile` from `apps/`.
- Focused clarification component tests: 14 tests passed.
- Web typecheck passed.
- Focused ESLint and Prettier checks passed.
- Focused Chromium E2E: 2 tests passed.
- Focused `mobile-chrome` E2E: 2 tests passed.
- `git diff --check` passed.
