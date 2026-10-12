---
id: "01-workspace-recovery"
title: "Preserve workspace navigation during failures"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-READ-RECOVERY-001
acceptance_criteria:
  - AC-WORKSPACES-READ-RECOVERY-001.1
  - AC-WORKSPACES-READ-RECOVERY-001.2
  - AC-WORKSPACES-READ-RECOVERY-001.3
  - AC-WORKSPACES-READ-RECOVERY-001.4
  - AC-WORKSPACES-READ-RECOVERY-001.5
  - AC-WORKSPACES-READ-RECOVERY-001.6
  - AC-WORKSPACES-READ-RECOVERY-001.7
system_design:
  - ../../specs/workspaces/system-design/workspace-read-recovery.md
---

# Task 01: Preserve workspace navigation during failures

## Summary

Correct failed context hydration and surface a shared navigation refresh status.
Keep same-workspace rows usable while retrying and isolate all late responses.

## In scope

- Own route/kanban bootstrap outcome handling, context generation guards, failed snapshot refetch, bounded retries, and shared navigation status.
- Own UI-01 desktop/phone status and localized copy; preserve current drawer and list geometry.

## Out of scope

Other work orders, new product metrics, health-policy changes, and live-instance mutation.

## Acceptance

- Failed reads retain eligible current data, while successful emptiness and access denial receive distinct treatment.
- Mixed outcomes update independently; cancellation/context changes prevent all stale writes, including loading and error flags.
- Desktop and phone can recover without reload; timer/foreground/manual triggers coalesce and stop after the bounded cycle.

## Regression tests

Name route regressions `retains workflows after a failed refresh`, `accepts a successful empty list`, `applies partial successes`, and `ignores a late response from another workspace`. Add fake-timer retry exhaustion and cancellation tests in the same route suite, plus a failed-snapshot-key recovery case in the existing snapshot suite. Assert visible rows using real store projections; testing only the HTTP mock does not prove the defect.

## ASCII UI preview

UI-01, [full preview](plan.md#ascii-ui-preview):

```text
Desktop sidebar          Phone task drawer
TASKS                    Tasks             Close
Refresh failed. [Retry]   Refresh failed. [Retry]
Task A                   Task A
Task B                   Task B
```

Map to AC-WORKSPACES-READ-RECOVERY-001.1/.5/.6. Keep the phone header fixed and
notice/rows in its existing scroll body. Initial failure has an error region;
retry disables its action; successful emptiness uses the existing empty state.

## Verification

Run from the repository root. Before the first pnpm command in a fresh worktree,
run `(cd apps && pnpm install --frozen-lockfile)`. New test paths below must be
created by this work order; verify test discovery before claiming a pass.

```bash
(cd apps/web && pnpm exec vitest run src/spa-routes.workspace.test.tsx src/kanban-route-startup.test.tsx hooks/use-workflows.test.ts lib/state/slices/kanban/kanban-slice.test.ts hooks/domains/kanban/use-all-workflow-snapshots.test.ts hooks/domains/kanban/use-workspace-sidebar-tasks.test.ts components/app-sidebar/sections/tasks-section.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/layout/sidebar-read-recovery.spec.ts tests/task/workspace-switch-sidebar-isolation.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/layout/mobile-sidebar-read-recovery.spec.ts tests/task/mobile-workspace-switch-sidebar-isolation.spec.ts)
```

If additional test files are changed during extraction, add their exact commands
here and run them before completion. Record skipped external services as blockers
to that validation, never as passing evidence.

## Files likely touched

- `apps/web/src/spa-routes.tsx`, `src/spa-routes.workspace.test.tsx`
- `apps/web/src/kanban-route.tsx`, `src/kanban-route-startup.test.tsx`
- `apps/web/lib/state/slices/kanban/{types.ts,kanban-slice.ts}` and their tests if state actions change
- `apps/web/hooks/domains/kanban/use-all-workflow-snapshots.ts` and its named tests
- `apps/web/hooks/domains/kanban/use-workspace-sidebar-tasks.ts` and its test
- `apps/web/components/app-sidebar/sections/tasks-section.tsx` and its test
- `apps/web/components/task/mobile/session-task-switcher-sheet.tsx`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,pseudo}/sidebar.json`
- New E2E files named in Verification.

## Dependencies

None. Follow the manifest order for delivery.

## Risks

Workspace/identity leakage and retry loops.

## Parallelism

`sequential`

## Inputs

- Applicable requirements and system designs from frontmatter, read in full.
- Investigation evidence and current source pointers in [plan.md](plan.md).
- Existing tests adjacent to owned files; E2E uses `test-base`, API seeding, and causal waits.

## Results

Implemented generation-scoped workspace context reads. Successful empty results
replace the matching collection, failed reads retain same-workspace data, and
access denial is shown separately. Route bootstrap, kanban bootstrap, workflow
snapshot recovery, desktop navigation, and the phone task drawer all reject
late writes from an older workspace generation. Failed snapshot keys retry
without refetching healthy keys, and foreground/manual/timer recovery shares the
same bounded retry cycle.

The incident reproduction supplied the RED evidence: a rejected route read was
converted into an empty workflow collection, which made the sidebar projection
hide existing tasks. The new route and store regressions are GREEN:

- `cd apps/web && pnpm exec vitest run src/spa-routes.workspace.test.tsx src/kanban-route-startup.test.tsx hooks/use-workflows.test.ts lib/state/slices/kanban/kanban-slice.test.ts hooks/domains/kanban/use-all-workflow-snapshots.test.ts hooks/domains/kanban/use-workspace-sidebar-tasks.test.ts components/app-sidebar/sections/tasks-section.test.tsx` passes 88 tests in 7 files.
- `cd apps/web && pnpm run typecheck` passes.
- `cd apps/web && pnpm run lint` passes with zero warnings, and `pnpm run i18n:check` passes all locale and copy guards.
- Desktop Playwright recovery tests pass for retained sidebar rows and Stats
  recovery. Mobile Playwright recovery tests pass for the task drawer and Stats
  page, including touch-sized Retry controls.
- Existing desktop and phone workspace-switch isolation tests pass, proving a
  late response from one workspace does not repopulate another workspace.

The managed E2E runner required the standard disposable plugin package before
the browser suite could start; `make e2e-plugin-package` produced it. No live
instance or user data was changed.
