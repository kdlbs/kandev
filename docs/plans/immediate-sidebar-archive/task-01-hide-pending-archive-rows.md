---
id: "01-hide-pending-archive-rows"
title: "Present pending archive rows"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-REMOVAL-NAVIGATION-003
acceptance_criteria:
  - AC-TASKS-REMOVAL-NAVIGATION-003.1
  - AC-TASKS-REMOVAL-NAVIGATION-003.2
  - AC-TASKS-REMOVAL-NAVIGATION-003.3
system_design:
  - ../../specs/tasks/system-design/removal-navigation.md
---

# Task 01: Present pending archive rows

## Summary

Subscribe the shared sidebar data projection to accepted archive operations.
Dim pending active rows with a spinner, then remove them on success while
preserving existing recovery semantics.

## In scope

Archive-only pending markers, real-store deferred regressions, desktop and phone proof.

## Out of scope

Delete changes, API changes, persisted state, new controls, and navigation redesign.

## Acceptance

- Prove the dimmed spinner presentation before network/destination settlement,
  including unselected, bulk, cascade and last-row cases.
- Remove successful targets after release while preserving new cache data,
  failure recovery, concurrent navigation and saved archived-inclusive views.
- Desktop and phone rendered regressions pass with requests held before processing.

## ASCII UI preview

UI-01: Active task navigation, archive A accepted (AC-003.1 through AC-003.3).

```text
Desktop sidebar              Phone task picker (reopened)
Before      Pending/Success  Before      Pending/Success
[A ...]     [A ...] (dim/spin) [A ...]     [A ...] (dim/spin)
[B ...]     [B ...]          [B ...]     [B ...]

Failure, A still active: [A ...] [B ...]
Last row archived: existing empty task list
```

The pending row keeps its place; success removes it and collapses the gap.
Spacing is illustrative.
Phone keeps its existing sheet, fixed header, safe-area padding, and single
scrolling list. Its visible menu is the archive entry point; acceptance retains
existing dismissal. No new mobile surface is needed for this frequent navigation
action. Existing `session-task-switcher-sheet.tsx` is the shipped exemplar.

Full preview and scenario mapping: [plan](plan.md#ascii-ui-preview).

## Verification

Run from the repository root. Install dependencies once for a fresh worktree.
Write the deferred regression first and record its expected failure, then fix.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run hooks/domains/kanban/use-workspace-sidebar-tasks.test.ts hooks/domains/kanban/use-workspace-sidebar-tasks.archive.test.ts hooks/use-task-removal-coordinator.test.ts lib/state/task-removal.test.ts lib/ws/handlers/tasks-archive.test.ts components/task/task-item.test.tsx components/task/task-item-archive-pending.test.tsx components/task/task-session-sidebar-item.test.ts components/task/task-session-sidebar-item-pending-archive.test.ts components/task/mobile/session-task-switcher-sheet-item.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint hooks/domains/kanban/use-workspace-sidebar-tasks.ts hooks/domains/kanban/use-workspace-sidebar-tasks.test.ts hooks/domains/kanban/use-workspace-sidebar-tasks.archive.test.ts hooks/use-task-removal-coordinator.test.ts e2e/tests/task/sidebar-immediate-archive-helpers.ts e2e/tests/task/sidebar-immediate-archive.spec.ts e2e/tests/task/mobile-sidebar-immediate-archive.spec.ts)
(cd apps/web && GOCACHE=/tmp/kandev-sidebar-go-cache pnpm e2e:run --project=chromium e2e/tests/task/sidebar-immediate-archive.spec.ts)
(cd apps/web && GOCACHE=/tmp/kandev-sidebar-go-cache pnpm e2e:run --no-build --project=mobile-chrome e2e/tests/task/mobile-sidebar-immediate-archive.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Managed E2E rebuilds production assets. If a helper or additional suite changes,
include its targeted lint/test command in Results. Compare rendered rows with
UI-01. Read `/tdd`, `/e2e` and scoped web guidance before implementation.

## Files likely touched

- `apps/web/hooks/domains/kanban/use-workspace-sidebar-tasks.ts`
- `apps/web/hooks/domains/kanban/use-workspace-sidebar-tasks.archive.test.ts` (new real-store regressions)
- `apps/web/components/task/task-item.tsx` and shared task-state row components
- `apps/web/components/task/task-item-archive-pending.test.tsx` and projection tests
- `apps/web/e2e/tests/task/sidebar-immediate-archive-helpers.ts` (shared browser scenario)
- `apps/web/e2e/tests/task/sidebar-immediate-archive.spec.ts` (new)
- `apps/web/e2e/tests/task/mobile-sidebar-immediate-archive.spec.ts` (new)

## Dependencies

None. Existing task-removal coordinator is present.

## Risks

Use operation membership, not departure ownership. Do not mutate caches to
implement temporary presentation. Preserve archived views and existing empty states.

## Parallelism

sequential

## Inputs

- [Requirements](../../specs/tasks/requirements/removal-navigation.md), REQ-003.
- [Design](../../specs/tasks/system-design/removal-navigation.md), Immediate sidebar archive projection.
- Existing coordinator tests and `task/removal-transition-helpers.ts` E2E pattern.

## Results

- Dependencies installed with the frozen lockfile.
- RED: two real-store tests failed because pending archive rows were not yet
  projected.
- Final unit verification: all 115 targeted tests passed across the listed
  suites, including the pending-row dimming and spinner assertion.
- Web typecheck passed. Targeted ESLint passed without warnings; formatting passed.
- Full backend and production web build passed using the writable Go cache.
- Initial browser attempts could not bind a socket inside the sandbox. Browser
  runs use an isolated backend with permitted local sockets and `--no-build`
  against those fresh assets; no developer instance or data was used.
- Phone regression passed after explicitly reopening the picker through its
  button rather than treating a closing animation as an open sheet. Its pending
  screenshot was inspected: the target remains dimmed with a spinner, and its
  settled screenshot shows one surviving row with no archived row or retained
  gap.
- Final browser commands passed (1 test each):
  `GOCACHE=/tmp/kandev-sidebar-go-cache pnpm e2e:run --host --no-build --project chromium e2e/tests/task/sidebar-immediate-archive.spec.ts`
  and
  `GOCACHE=/tmp/kandev-sidebar-go-cache pnpm e2e:run --host --no-build --project mobile-chrome e2e/tests/task/mobile-sidebar-immediate-archive.spec.ts`.
- Desktop pending screenshot inspected against UI-01: the target remains in its
  original place, dimmed with a spinner, and the neighboring row does not jump.
- Catalog validation, full specification lint, and diff checks passed.

The phone's existing archive-error path logs the failure rather than displaying
Desktop's toast. The shared test asserts pending-row recovery on both and the
existing desktop error notification; no notification behavior was changed.
