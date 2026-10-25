---
id: "01-guard-route-task-sync"
title: "Guard missing-route task synchronization"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/missing-task-route-recovery.md"
---

# Task 01: Guard Missing-Route Task Synchronization

Prevent an unchanged missing route from replacing a later session-backed sidebar selection.

## Acceptance

- Route initialization and real route changes still select the route task and its available session.
- An unchanged stale route cannot pair its task ID with a session from the active sibling task.
- The session-backed browser regression keeps the sibling URL, active row, and Dockview workbench after session updates.
- Returning from another task preserves a deliberately selected non-primary session.

## TDD sequence

1. Add the session-backed browser scenario. Run it before production changes and record the expected error-state failure.
2. Add a stale-route unit case to `syncActiveTaskSession`. Record the expected setter assertion failure.
3. Extend the helper inputs and add the minimum effect guard that passes the unit cases.
4. Run the focused unit cases and web typecheck.
5. Rebuild the production web assets through the managed E2E command. Run the browser scenario again.
6. Run the A to B to A secondary-session regression before closing review fixup.

## Verification

If `apps/node_modules` is absent, run this command first:

```bash
cd apps && pnpm install --frozen-lockfile
```

Run these commands in order:

```bash
cd apps/web && pnpm e2e:run tests/task/task-loading-state.spec.ts -- --grep "keeps a session-backed sibling active"
cd apps && pnpm --filter @kandev/web exec vitest run components/task/task-page-content-helpers.test.ts
cd apps/web && pnpm run typecheck
cd apps && pnpm exec prettier --check web/components/task/task-page-content.tsx web/components/task/task-page-content-helpers.ts web/components/task/task-page-content-helpers.test.ts web/e2e/tests/task/task-loading-state.spec.ts
cd apps && pnpm --filter @kandev/web exec eslint components/task/task-page-content.tsx components/task/task-page-content-helpers.ts components/task/task-page-content-helpers.test.ts e2e/tests/task/task-loading-state.spec.ts
cd apps/web && pnpm e2e:run tests/task/task-loading-state.spec.ts -- --grep "keeps a session-backed sibling active"
```

The first browser command is the RED run. The final browser command is the GREEN run and must rebuild the production assets.

## Files likely touched

- `apps/web/components/task/task-page-content.tsx`
- `apps/web/components/task/task-page-content-helpers.ts`
- `apps/web/components/task/task-page-content-helpers.test.ts`
- `apps/web/e2e/tests/task/task-loading-state.spec.ts`
- `docs/plans/missing-task-session-navigation/plan.md`
- `docs/plans/missing-task-session-navigation/task-01-guard-route-task-sync.md`

## Dependencies

None.

## Parallelism

`sequential`. The unit guard and browser regression cover the same state transition.

## Inputs

- The session-backed regression scenario in `docs/specs/tasks/requirements/missing-task-route-recovery.md`.
- The confirmed production-build failure in the current task conversation.
- `syncActiveTaskSession` and `useTaskPageData` as the current route synchronization path.
- `selectTaskWithLayout` and `replaceTaskUrl` as the current in-place switch path.
- The existing no-session test in `apps/web/e2e/tests/task/task-loading-state.spec.ts`.

## Risks

- An over-strict guard can block a real route change.
- An over-strict guard can block delayed session adoption for the current route.
- A URL-navigation change can remount the workbench and expand the repair scope.

## Output contract

Report the RED and GREEN results. Report all changed files and exact command outcomes. Update this task and `plan.md` in the same conversation.

## Results

- RED browser run reproduced the task-load error after selecting a
  session-backed sibling from the missing route.
- RED unit run failed because the helper returned `undefined` for the stale
  route case.
- GREEN unit run passed 22 focused tests.
- GREEN production-build E2E passed the session-backed sibling scenario.
- Web typecheck, targeted Prettier, and targeted ESLint passed.
- No new mobile test was needed because the repair changes shared route/store
  state only. The existing mobile missing-route recovery scenario is unchanged.
- Review fixup RED production-build E2E returned task A's primary session after
  an A to B to A switch.
- Review fixup GREEN production-build E2E preserved task A's selected
  non-primary session.
- The synchronization effect now reads the current active task from a ref and
  is not triggered by active-task changes alone.
- Full task-loading-state production-build E2E passed, 4 tests.
