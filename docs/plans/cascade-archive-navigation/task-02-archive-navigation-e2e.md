---
id: "02-archive-navigation-e2e"
title: "Add archive navigation E2E coverage"
status: done
wave: 2
depends_on: ["01-cascade-aware-switch"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/archive-confirmation.md"
---

# Task 02: Add Archive Navigation E2E Coverage

Reproduce the recent-child cascade sequence on desktop and phone. Prove that the
safe task, URL, rendered content, and later task navigation remain synchronized
without a hard refresh.

## Acceptance

- The desktop test makes a child the most recent alternative, archives its
  active parent with cascade enabled, and opens the only unrelated live task.
- The phone test performs the same archive through the task drawer and renders
  the unrelated task in the mobile task layout.
- The phone test can open a later live task from the same SPA instance after the
  cascade archive.
- The parent and both children disappear from active task projections.
- Main-frame document request counts do not increase during the archive switch
  or later phone task switch.
- Existing active-task and last-task archive redirect coverage remains green.

## Files likely touched

- `apps/web/e2e/pages/session-page.ts`
- `apps/web/e2e/tests/task/archive-task-redirect.spec.ts`
- `apps/web/e2e/tests/task/mobile-archive-task-redirect.spec.ts`

## Dependencies

- Task 01 must be complete.

## Parallelism

`sequential`. Both scenarios consume Task 01 and share the managed production
build.

## Inputs

- Plan sections **E2E tests** and **Mobile design contract**.
- `apps/web/e2e/tests/task/archive-task-redirect.spec.ts` for desktop archive
  and session-content assertions.
- `apps/web/e2e/tests/task/mobile-sidebar-task-actions.spec.ts` for phone drawer
  and task action interactions.
- `apps/web/e2e/pages/mobile-kanban-page.ts` for mobile task-card assertions if
  the scenario reaches Home during failure diagnosis.

## TDD sequence

1. Extend `SessionPage.archiveTaskInSidebar` with an optional cascade input.
2. Add the desktop recent-child regression and run it against the current code
   to capture the wrong child or Home destination.
3. Add the phone regression with the shipped drawer and dialog controls. Run it
   against the current code to capture the route and rendered-state mismatch.
4. Run both scenarios after Task 01 is green.
5. Run the existing tests in the desktop redirect file to protect normal and
   last-task behavior.

## Verification

Run the workspace install once first if `apps/node_modules` is absent:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm e2e:run tests/task/archive-task-redirect.spec.ts)
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/task/mobile-archive-task-redirect.spec.ts -- --grep 'keeps navigation responsive')
```

Confirm that Playwright discovers the intended mobile test. `No tests found` is
not passing evidence.

## Risks

- The regression must establish recent-task order by visiting a child and then
  its parent. Creation order alone is not deterministic evidence.
- Seeded tasks need primary sessions so the test can distinguish a real task
  render from a URL-only change.
- Count only main-frame document navigation requests. API and asset requests are
  expected and do not represent a hard refresh.
- Wait for the archive result and task WebSocket updates through user-visible
  assertions. Do not add fixed sleeps.

## Output contract

Report the original RED symptom, GREEN desktop and phone outcomes, and
main-frame document request evidence. Include exact commands, managed runner
cleanup, remaining risks, and synchronized task and plan status. Record each
command and outcome in `## Results`.

## Results

- The first desktop fixture attempt exposed that the E2E task API rejects a
  depth-two task tree. The regression fixtures use two direct children, while
  the unit suite covers transitive descendants and malformed cycles.
- `cd apps/web && pnpm e2e:run tests/task/archive-task-redirect.spec.ts -- --grep 'skips cascaded descendants'` passed with 1 test in the managed production runner.
- `cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/task/mobile-archive-task-redirect.spec.ts -- --grep 'keeps navigation responsive'` passed with 1 test in the managed production runner.
- `cd apps/web && pnpm e2e:run tests/task/archive-task-redirect.spec.ts` passed with 2 tests, retaining normal and last-task archive redirect coverage.
- Desktop and phone assertions kept the URL and rendered task synchronized,
  removed the parent and both children from active projections, and opened a
  later phone task from the drawer without increasing the main-frame document
  request count. The desktop archive count was 0 to 0, and the phone archive
  and later drawer navigation counts were each 0 to 0.
- The original RED symptom was the recent child becoming the temporary target
  of a parent cascade archive, after which the WebSocket redirect could leave
  the URL and rendered route out of sync. The GREEN tests now choose the
  unrelated task and preserve later SPA navigation.
- Remaining risks are limited to asynchronous WebSocket update ordering and
  cached projection freshness; user navigation remains protected by the
  existing active-task guard. Task 02 is done and the parent plan is complete.
- The managed E2E runners cleaned up their isolated runtime and database after
  each run.
