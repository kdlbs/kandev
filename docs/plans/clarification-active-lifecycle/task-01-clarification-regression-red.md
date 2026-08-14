---
id: "01-clarification-regression-red"
title: "Clarification regression red"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/clarification-active-lifecycle/spec.md"
---

# Task 01: Clarification regression red

## Acceptance

- Desktop production-build E2E reproduces the old-detached/newer-skipped sequence and fails because
  the old question or task icon resurfaces after reload or later completion.
- Desktop E2E creates a task whose non-primary session owns clarification and fails because task-row
  activation opens the clean preferred/primary session.
- Phone E2E reproduces secondary pending ownership with touch activation and fails on active chat or
  question visibility while preserving existing drawer assertions.
- Each failure is captured once for the expected product reason before production code changes.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps/web && pnpm e2e:run tests/task/sidebar-pending-question.spec.ts
cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/task/mobile-sidebar-task-actions.spec.ts
```

The focused commands are expected to exit nonzero in this RED task. Record the exact failing
assertions and fresh artifact paths; do not weaken assertions or increase timeouts.

## Files likely touched

- `apps/web/e2e/tests/task/sidebar-pending-question.spec.ts`
- `apps/web/e2e/tests/task/mobile-sidebar-task-actions.spec.ts`
- `apps/web/e2e/helpers/clarification.ts` only if shared current test setup removes duplication
- `apps/web/e2e/pages/session-page.ts` only if a stable existing surface lacks a reusable locator

## Dependencies

None.

## Parallelism

Sequential. These black-box failures define every later task's acceptance boundary.

## Inputs

- Spec scenarios for newer-turn supersession and secondary-session navigation.
- Existing `/e2e:clarification-timeout` and `/e2e:clarification` mock scenarios.
- Existing `ApiClient.addUserMessage`, `ApiClient.launchSession`, `SessionPage`, and sidebar helpers.
- E2E guidance: seed via API, assert through UI, verify reload, use `.tap()` on phone, and run managed
  production builds.

## Risks

- Confirm the second clarification has a different `pending_id` before clicking Skip; identical prompt
  text is not sufficient evidence.
- Wait on exact session states and persisted messages, not arbitrary sleeps or agent output alone.
- Keep mobile file naming/project routing intact so Playwright discovers the intended test.

## Output contract

Report scenarios added, exact commands, expected failure messages/artifact paths, and why each failure
matches the confirmed defect. Mark this task `done`, retain failing tests for later GREEN, and update
the plan checkbox/results.

## Results

Pending.
