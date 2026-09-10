---
id: "01-removal-coordinator"
title: "Coordinate local removal"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-REMOVAL-NAVIGATION-001
  - REQ-TASKS-REMOVAL-NAVIGATION-002
acceptance_criteria:
  - AC-TASKS-REMOVAL-NAVIGATION-001.3
  - AC-TASKS-REMOVAL-NAVIGATION-002.1
  - AC-TASKS-REMOVAL-NAVIGATION-002.2
  - AC-TASKS-REMOVAL-NAVIGATION-002.3
  - AC-TASKS-REMOVAL-NAVIGATION-002.4
system_design:
  - ../../specs/tasks/system-design/removal-navigation.md
---

# Task 01: Coordinate local removal

## Summary

Implement the store-backed removal operation and its guarded navigation flow.
Separate request outcomes from destination preparation, keeping transport APIs
and authoritative task state unchanged.

## In scope

- Pure operation transitions, per-target deduplication, batch/cascade exclusion,
  navigation revisions, and request/departure settlement.
- Extend `useTaskRemoval` and `useArchiveAndSwitchTask`; share the path with delete.
- Candidate validation, uncertain ancestry, workspace boundaries, session/layout
  identity, SPA overview fallback, and safe failure revalidation.
- Match local navigation ownership in lifecycle WS handlers while retaining
  cache cleanup, remote redirects, and Office refetch effects.
- Tests using real stores and deferred API/session promises.

## Out of scope

Rendered boundary and menu integration (02); browser flows (03); backend changes.

## Acceptance

- Deferred responses cannot commit selection, URL, layout, or rollback after
  a newer navigation revision, including leave-and-return to the same task.
- Every fallback excludes the complete batch/cascade set; no-candidate and
  destination-load failures reach the overview without a full reload.
- Request settlement is idempotent, partial failure preserves failed targets,
  and unknown outcomes never resurrect tasks or fabricate archive state.

## Verification

Run from the repository root. Install dependencies once if absent.

```bash
(cd apps && rtk pnpm install --frozen-lockfile)
(cd apps/web && rtk pnpm exec vitest run lib/state/task-removal.test.ts hooks/use-task-removal.test.ts hooks/use-task-removal-coordinator.test.ts hooks/use-task-actions.test.ts lib/ws/handlers/tasks-archive.test.ts lib/ws/handlers/tasks.test.ts lib/ws/handlers/tasks.deleted.test.ts lib/ws/handlers/tasks-unarchive.test.ts lib/routing/client-router.test.ts)
(cd apps/web && rtk pnpm run typecheck)
rtk git diff --check
```

The new `task-removal.test.ts` must exist after this order. Extend routing
tests only as required by the navigation integration. Record behavioral RED,
then GREEN results; do not treat missing modules as the regression.

## Files likely touched

- `apps/web/lib/state/task-removal.ts` and `task-removal.test.ts` (new)
- `apps/web/lib/state/store.ts`
- `apps/web/lib/state/slices/kanban/kanban-slice.ts` and `types.ts`
- `apps/web/hooks/use-task-removal.ts` and its tests
- `apps/web/hooks/use-task-actions.ts` and its tests
- `apps/web/lib/ws/handlers/task-lifecycle-side-effects.ts`, `tasks.ts`, and listed tests
- `apps/web/lib/links.ts`, `lib/routing/client-router.ts`, `navigation-guard.ts`, and listed tests

## Dependencies

None.

## Risks

Store selection and native history changes have different notification paths.
Do not add a global router rewrite or bypass existing unsaved-change guards.
The action must complete applicable navigation admission before unmounting
content or dispatching removal; a cancelled admission changes neither.
Preserve existing archive-confirmation and delete-discard choices.

## Parallelism

`sequential`

## Inputs

- Removal design: Operation state, Navigation and request flow, WebSocket
  reconciliation, Failure recovery.
- Existing `use-task-removal.test.ts` and `use-task-actions.test.ts`.
- Existing lifecycle tests for local, remote, and Office task removal.
- `lib/routing/navigation-guard.ts` and `client-router.ts`.

## Results

Done on 2026-09-10.

- RED: `cd apps/web && pnpm exec vitest run hooks/use-task-removal-coordinator.test.ts`
  failed on the stale `replaceTaskUrl` rollback expectation after navigation
  ownership moved into the coordinator.
- GREEN: `cd apps/web && pnpm exec vitest run lib/state/task-removal.test.ts
  hooks/use-task-removal.test.ts hooks/use-task-removal-coordinator.test.ts
  hooks/use-task-actions.test.ts lib/ws/handlers/tasks-archive.test.ts
  lib/ws/handlers/tasks.test.ts lib/ws/handlers/tasks.deleted.test.ts
  lib/ws/handlers/tasks-unarchive.test.ts lib/routing/client-router.test.ts`
  passed with 9 files and 86 tests.
- Fixup rerun of the same command passed with 9 files and 96 tests after the
  review regressions were added.
- Added browser-local operation state, per-target settlement, duplicate-target
  suppression, navigation ownership, cascade exclusion, workspace-scoped
  candidate validation, and guarded SPA destination navigation.
- Coordinator suite: 1 file, 4 tests passed.
- Full coordinator verification command: 9 files, 86 tests passed.
- Typecheck reached the repository's existing duplicate declarations in
  `lib/types/http.ts` and `lib/ws/handlers/workflows.ts`; no diagnostics point
  to the coordinator changes.
- `rtk git diff --check`: passed.
