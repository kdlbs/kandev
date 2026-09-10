---
id: "01-shared-task-action-flow"
title: "Share the task action flow"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-THREADS-ACTIONS-001
  - REQ-TASKS-THREADS-ACTIONS-002
acceptance_criteria:
  - AC-TASKS-THREADS-ACTIONS-001.1
  - AC-TASKS-THREADS-ACTIONS-001.2
  - AC-TASKS-THREADS-ACTIONS-001.3
  - AC-TASKS-THREADS-ACTIONS-001.4
  - AC-TASKS-THREADS-ACTIONS-001.5
  - AC-TASKS-THREADS-ACTIONS-001.6
  - AC-TASKS-THREADS-ACTIONS-002.1
  - AC-TASKS-THREADS-ACTIONS-002.2
  - AC-TASKS-THREADS-ACTIONS-002.3
  - AC-TASKS-THREADS-ACTIONS-002.4
  - AC-TASKS-THREADS-ACTIONS-002.5
  - AC-TASKS-THREADS-ACTIONS-002.6
system_design:
  - ../../specs/tasks/system-design/threads-task-actions.md
---

# Task 01: Share the Task Action Flow

## Summary

Provide the shared task action composition and lifetime needed by Threads.
Keep existing API, confirmation, linking, and option rules in task-owned code
and make target identity independent of any selected task or session.

## In scope

- Add `useTaskManagementFlow` above removable rows/columns, using existing task
  hooks. Extract archive/delete lifecycle glue into `useTaskMenuActions` and
  use it from an existing task-menu caller to prevent parallel logic.
- Resolve current task metadata by captured workspace/task ID, including
  priority, workflow step, archive state, repository links, executor type and
  task-wide busy state; fail closed when authoritative target data is missing.
- Add a shared six-group menu composition using the existing action items.
  Preserve the full switcher's current default composition and bulk behavior.
- Reuse existing link selection/dialog helpers and plugin registration rules;
  retain original target during handoff, source updates, and asynchronous work.
- Add an explicit stay-on-listing option to shared removal cleanup, retaining
  existing navigation defaults and cascade exclusions. Guard duplicate pending
  submissions and use existing localized success/failure feedback.

## Out of scope

- Threads header markup, deck selection, phone drawer pages, new mutation APIs,
  and a generic action registry.

## Acceptance

1. A menu opened for A dispatches each of the six operations only for A through
   existing task behavior, despite re-renders, B selection, a sibling session
   switch, title updates, or a dialog handoff. Live access/provider/target loss
   disables or ends the flow without a stale dispatch.
2. Shared tests prove success, rejection, repeat-confirm blocking, archive
   preference/cascade behavior, deletion consent, link context, and cleanup
   without listing navigation. A failure leaves confirmed state intact and
   displays one appropriate error.
3. Existing switcher, task-detail removal, link, priority and confirmation
   behavior stays covered while Threads can request exactly the six groups.

## Verification

After the later implementation request, integrate the pinned parent commit
from the repository root before feature edits. Preserve this package and any
new user changes while resolving the normal branch merge:

```bash
git merge --no-edit 0b4253fa71ba0fe85188bff8cd27c2860425c516
git merge-base --is-ancestor 0b4253fa71ba0fe85188bff8cd27c2860425c516 HEAD
```

Bootstrap once from the repository root if this worktree has no workspace install
(do this before the merge too if a repository hook needs pnpm):

```bash
cd apps && pnpm install --frozen-lockfile
```

Run from `apps/web`. Write failing tests first and record their specific
behavioral failures before implementation:

```bash
pnpm exec vitest run hooks/use-task-menu-actions.test.ts components/task/task-management-menu.test.tsx
pnpm exec vitest run hooks/use-task-actions.test.ts hooks/use-task-removal.test.ts hooks/use-task-workflow-move.test.ts hooks/use-update-task-priority.test.ts components/task/task-switcher-context-menu.test.tsx components/task/task-session-sidebar-link-actions.test.ts components/task/task-session-sidebar-move.test.ts components/task/task-archive-confirmation.test.tsx components/task/task-delete-confirm-dialog.test.tsx
pnpm run typecheck
pnpm exec eslint --max-warnings 0 hooks/use-task-menu-actions.ts hooks/use-task-removal.ts components/task/task-management-menu.tsx components/task/task-switcher-context-menu.tsx
```

The first command covers captured-target dispatch, live eligibility, deferred
success/failure, idempotent cleanup, and preserved full-menu defaults. The
second protects shared consumers. If extraction touches another shared helper,
include its existing focused tests and lint path in this work order's results.
Browser evidence for the shared flow belongs to Tasks 02 and 03.

## Files likely touched

- `apps/web/hooks/use-task-menu-actions.ts` and `.test.ts` (new).
- `apps/web/hooks/use-task-actions.ts`, `use-task-removal.ts`, and their tests.
- `apps/web/components/task/task-management-menu.tsx` and `.test.tsx` (new).
- `apps/web/components/task/task-switcher-context-menu.tsx`, action items,
  priority/move/link helpers, archive adapter, and existing tests.
- `apps/web/components/task/task-session-sidebar.tsx`, link-actions,
  task-linking and dialog composition for consuming/exposing the shared flow.

## Dependencies

The parent mobile Threads/shared-header commits must be integrated and recorded
in `plan.md`. There is no earlier work order. Re-read any parent changes to
shared menu and confirmation components first.

## Risks

Existing hooks differ in error behavior: priority consumes failure after its
toast, workflow move toasts then rejects, and archive/delete propagate errors.
Do not infer success from a swallowed rejection or show duplicate error toasts.
Avoid broad sidebar refactoring; extract only the action boundary being reused.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/threads-task-actions.md): 001 and 002.
- [Design](../../specs/tasks/system-design/threads-task-actions.md): Target
  lifetime, shared task operations, menu composition, state delivery.
- Existing task switcher context menu, `TaskMenuButton`, link hooks,
  `TaskArchiveConfirmation`, `TaskDeleteConfirmDialog`, and removal tests.
- `/tdd`, frontend `AGENTS.md`, and the integrated parent work orders.

## Results

Completed in the primary session after the user's implementation request.

- Parent commit merged as `facf9aef7` and dependencies installed before edits.
- Shared removal lifecycle now serves both the sidebar and Threads. The default
  sidebar/task-detail navigation is unchanged; Threads opts into listing-only
  cleanup. Priority, movement, provider/plugin linking and confirmations retain
  their existing task-owned implementations.
- Captured workspace/task lookup fails closed. Keyed provider forms isolate
  late A responses from B; the pending mutation owner outlives those forms.
  Filtering can remove an opener without destroying A's archive confirmation.
- RED evidence: listing cleanup attempted an unwanted task-detail fetch;
  returning to an old workspace revived its dismissed menu; an old link result
  could close a newer form; removing the filtered header lost the pending
  archive surface. Focused regressions now pass after their respective fixes.
- GREEN: the final command in `plan.md` passed **217 tests in 25 files**,
  including all shared action, target, flow, confirmation and existing sidebar
  tests. Archive/delete tests assert captured A, one request during duplicate
  submit, preserved B, failure feedback, retained state and successful retry.
  Affected-file ESLint, typecheck and locale checks passed. Both browser suites
  verify six real persisted outcomes and rejected/cancelled requests.

### PR feedback regression coverage (2026-09-10)

New failing tests reproduced three gaps: moves within a hidden current workflow
were silently rejected; an active workflow's loaded steps were masked by a
placeholder multi-workflow snapshot; and the archived eligibility fixture did
not actually contain an archived task. The shared surface now permits only the
same-workflow hidden destination, step lookup falls back only for missing or
placeholder snapshots of the matching workflow, and target lookup rejects the
normalized `isArchived` flag. Authoritative empty snapshots remain empty.

All 15 tests across these focused files pass after their RED failures:

```sh
pnpm exec vitest run --maxWorkers=2 hooks/use-task-management-flow.test.ts components/task/task-management-surface.test.tsx lib/tasks/task-menu-target.test.ts
```

The broader affected component, board, priority and move suite passed 64 tests
in ten files. Existing error feedback, captured identity and fallback contracts
are unchanged. The ineffective props memo and duplicate destination filter were
removed; comments and the empty-board dependency were clarified.

Main-base integration preserves the landed shared archive/delete switching
helper and dialog `focusReturnRef` contract. Threads passes its listing-only
option through the shared helper and retains its explicit close-focus override.
Two added dialog compatibility cases verify both focus paths. The integrated
suite passed 252 tests in 29 files; 27 desktop browser cases also passed,
including task-detail and preview archive/delete behavior and dismissal focus.

### CI concurrency regression (2026-09-10)

The existing desktop archive redirect test exposed a global duplicate guard
rejecting B after optimistic navigation while A's archive request was pending.
Four new held-request tests reproduced that failure for archive/delete, then
passed with the shared guard keyed by task ID. They retain same-task exclusion,
prove independent completion, and keep B guarded while a failed A is retried.
The shared hook's public busy indicator and navigation/error owners remain
unchanged. CodeRabbit's handler-map typing and awaited inline-containment test
improvements also pass. The exact commands in the plan's CI correction section
passed 141 focused unit/component tests, eight desktop browser tests and nine
phone browser tests with one browser worker and no retries. Typecheck, affected
ESLint and locale checks passed. No public contract or product copy changed.
