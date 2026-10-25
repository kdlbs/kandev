---
id: "02-workflow-reset-e2e"
title: "Cover workflow reset and hydration"
status: done
wave: 2
depends_on:
  - "01-restore-runtime-configuration"
plan: "plan.md"
spec: "../../specs/ui/requirements/acp-model-configuration-summary.md"
---

# Task 02: Cover Workflow Reset and Hydration

## Acceptance

- The workflow reset scenario starts with `Mock Smart`, `Plan Mock`, and effort `Max`.
- The model and mode selectors show the same values after reset and after page reload.
- The test uses the isolated mock agent and the existing workflow page without production frontend changes.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile && cd web && pnpm e2e:run tests/workflow/workflow-step-proceed.spec.ts -- --grep "preserves session settings across context reset"
```

Update the test first. The RED run must show at least one fresh provider default instead of the selected value.

## Files likely touched

- `apps/web/e2e/tests/workflow/workflow-step-proceed.spec.ts`

## Dependencies

Task 01.

## Parallelism

Sequential after Task 01. The file is disjoint from Task 03, but this test needs the backend repair.

## Inputs

- Reset scenarios in the linked spec
- Plan section `E2E Tests`
- Existing `preserves selected model across context reset` scenario
- Existing `data-testid="session-mode-selector"` and `Session model settings` selector contracts
- E2E capability-readiness and session-lifecycle guidance

## Mobile parity

The change has no frontend composition or viewport branch. Existing mobile selector tests cover access and rendering of the shared state.

## Output contract

Report the RED and GREEN values for model, mode, and effort. Include the exact Playwright result and cleanup evidence.

Update this task and `plan.md` in the same conversation.

## Results

The seeded profile selects `Mock Smart`, `Plan Mock`, and effort `Max`. The
test asserts the model and mode selector text before reset, after the reset
step settles, and after a full page reload. The test uses the isolated mock
agent and the existing workflow page; no frontend production code changed.

The backend RED run demonstrated the fresh-default and missing-option behavior
that the E2E protects. The GREEN Playwright run completed with one passing test
in 20.0 seconds. The E2E fixture packaging and isolated remote repository
setup completed as part of the run, and the command exited successfully.

Verification:

- `cd apps && pnpm install --frozen-lockfile` — completed.
- `cd apps/web && pnpm e2e:run tests/workflow/workflow-step-proceed.spec.ts -- --grep "preserves session settings across context reset"` — 1 passed.
