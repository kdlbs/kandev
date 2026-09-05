---
id: "02-context-window-unmeasured-e2e"
title: "Cover pending state on desktop and mobile"
status: done
wave: 2
depends_on: ["01-render-unmeasured-context-state"]
plan: "plan.md"
spec: "../../specs/ui/requirements/context-window-unmeasured-state.md"
---

# Task 02: Cover pending state on desktop and mobile

## Intent

Extend the existing context-window source fixtures so browser tests prove the unmeasured state
through the same store-to-tooltip path on desktop and mobile, while retaining the positive-sample
regression scenario.

## Acceptance

- The desktop context-window E2E case seeds used: 0, opens the control, and verifies the pending
  accessible label, em-dash values, known size/source, and absence of numeric zero usage.
- The mobile-chrome case reaches the same pending content through a tap-pinned tooltip and proves
  there is no document-level horizontal overflow.
- Existing positive-sample source/compaction assertions continue to pass, proving the measured
  state was not regressed.

## TDD sequence

1. Parameterize the shared context-window seed for a zero-usage fixture and add desktop/mobile
   pending assertions; run the focused E2E specs against the current UI to observe RED.
2. After Task 01 is present, run the managed production-build desktop and mobile projects.
3. Tighten selectors to the translated accessible/content contract and inspect any failure
   artifacts without changing the mobile composition.

## Files likely touched

- apps/web/e2e/tests/chat/context-window-source-helpers.ts
- apps/web/e2e/tests/chat/context-window-source.spec.ts
- apps/web/e2e/tests/chat/mobile-context-window-source.spec.ts

## Dependencies

Task 01.

## Parallelism

sequential. The browser tests depend on the completed pending UI and translation contract.

## Verification

- cd apps && rtk pnpm install --frozen-lockfile
- cd apps/web && rtk pnpm e2e:run tests/chat/context-window-source.spec.ts
- cd apps/web && rtk pnpm e2e:run --project mobile-chrome tests/chat/mobile-context-window-source.spec.ts

## Inputs

- Spec pending desktop/mobile scenarios and Out of scope.
- Plan Mobile design contract and E2E Tests.
- Existing seed and interaction helpers in
  apps/web/e2e/tests/chat/context-window-source-helpers.ts.
- Mobile E2E conventions in .agents/skills/e2e/SKILL.md.

## Output contract

Report the exact managed E2E commands, browser project and test counts, pending/positive
assertions, overflow result, artifacts or teardown issues, changed files, remaining risks, and
synchronized task/plan/spec status.

## Results

Implemented.

Files changed:

- `apps/web/e2e/tests/chat/context-window-source-helpers.ts` — `seedContextWindowTask` now accepts an
  optional `ContextWindowSeed` partial merged over the default fixture, and derives the expected
  trigger accessible name (pending label for `used: 0`, else the rounded percentage).
- `apps/web/e2e/tests/chat/context-window-source.spec.ts` — new pending-state scenario: seeds
  `used: 0`, opens the control, asserts the pending accessible label, "—%" and "— of 258.4K tokens",
  the pending explanation, retained source/compaction rows, and the absence of numeric 0%/0-of usage.
- `apps/web/e2e/tests/chat/mobile-context-window-source.spec.ts` — same pending content asserted
  through a tap-pinned tooltip on the mobile-chrome project, plus the document-level no-horizontal-
  overflow check.

Managed E2E commands (docker mode, `ghcr.io/kdlbs/kandev-ci:runtime-latest`):

- `pnpm e2e:run tests/chat/context-window-source.spec.ts` — 2 passed (pending + positive-sample
  regression).
- `pnpm e2e:run --project mobile-chrome tests/chat/mobile-context-window-source.spec.ts` — 2 passed
  (pending touch + positive-sample touch regression, both with the overflow check).

The existing positive-sample source/compaction assertions still pass, proving the measured state was
not regressed. No artifacts or teardown issues; first run revealed and fixed a
`page.evaluate` argument-serialization bug in the helper (fixture now travels as a single object
argument). Remaining risk: none beyond the recorded zero-token-sample display trade-off.

