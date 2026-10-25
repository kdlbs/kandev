---
id: "02-prove-live-sidebar-regrouping"
title: "Prove live sidebar regrouping"
status: completed
wave: 2
depends_on: ["01-publish-clarification-task-state"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/runtime-state-publication-order.md"
---

# Task 02: Prove live sidebar regrouping

## Acceptance

- A State-grouped desktop sidebar moves an answered clarification task from
  `Review` to `In progress` while the same agent turn remains active, without a
  reload.
- The test uses existing sidebar and clarification controls and adds no
  frontend production behavior or test-only production hooks.
- Mobile parity remains covered by the shared task store plus the existing
  mobile clarification-answer flow; no responsive composition changes.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm e2e:run tests/chat/clarification.spec.ts -- \
  --grep 'moves answered task from Review to In progress without reload'
)
```

## Files likely touched

- `apps/web/e2e/tests/chat/clarification.spec.ts`

## Dependencies

- Task 01: the backend must publish the live task-state event.

## Parallelism

Sequential. The test is expected to stay red until Task 01 lands.

## Inputs

- Spec clarification-answer and State-grouped sidebar scenarios.
- Plan `E2E Tests` and mobile-parity note.
- `apps/web/e2e/helpers/clarification.ts` and
  `apps/web/e2e/pages/sidebar-filter-popover.ts` as existing patterns.

## Output contract

Report the failing pre-fix behavior if captured, the final Playwright result and
test count, generated failure/evidence artifacts, cleanup, and synchronized
task/plan status.

## Results

- Focused Playwright verification passed: `cd apps/web && pnpm e2e:run
  tests/chat/clarification.spec.ts -- --grep 'moves answered task from Review to
  In progress without reload'` — 1 passed.
- The test waits for the mock session to reach `WAITING_FOR_INPUT`, sets the
  persisted task state to `REVIEW`, then verifies the State-grouped sidebar
  moves the task to `IN_PROGRESS` immediately after answering, without reload.
- An earlier attempt exposed nondeterministic setup timing (the task was still
  `IN_PROGRESS` before the mock clarification parked); the final setup polls
  the session state and makes the pre-answer state explicit.
- The E2E runner cleaned its generated test-results, blob-report, and shard
  artifacts; no generated files remain in the worktree.
