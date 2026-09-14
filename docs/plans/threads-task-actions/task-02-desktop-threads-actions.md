---
id: "02-desktop-threads-actions"
title: "Integrate desktop Threads task actions"
status: done
wave: 2
depends_on:
  - "01-shared-task-action-flow"
plan: "plan.md"
requirements:
  - REQ-TASKS-THREADS-ACTIONS-001
  - REQ-TASKS-THREADS-ACTIONS-002
  - REQ-TASKS-THREADS-ACTIONS-003
  - REQ-TASKS-THREADS-ACTIONS-004
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
  - AC-TASKS-THREADS-ACTIONS-003.1
  - AC-TASKS-THREADS-ACTIONS-003.2
  - AC-TASKS-THREADS-ACTIONS-003.3
  - AC-TASKS-THREADS-ACTIONS-003.4
  - AC-TASKS-THREADS-ACTIONS-003.5
  - AC-TASKS-THREADS-ACTIONS-003.6
  - AC-TASKS-THREADS-ACTIONS-004.1
  - AC-TASKS-THREADS-ACTIONS-004.4
  - AC-TASKS-THREADS-ACTIONS-004.5
  - AC-TASKS-THREADS-ACTIONS-004.6
  - AC-TASKS-THREADS-ACTIONS-004.7
system_design:
  - ../../specs/tasks/system-design/threads-task-actions.md
---

# Task 02: Integrate Desktop Threads Task Actions

## Summary

Expose the shared task actions from the desktop thread header and explicit
overflow button. Keep task/session identity stable and reconcile the deck when
confirmed mutations or view changes remove a task.

## In scope

- Compose the shared action host above the column list, fed by scoped task
  snapshots. Attach a header-only context entry and visible `TaskMenuButton`;
  preserve keyboard anchoring and exclude chat, editors and session controls.
- Add the pure successor/predecessor fallback helper and wire it to admitted
  order plus the parent's viewport state. Preserve a surviving thread's scroll
  offset and selected session when earlier or unrelated tasks disappear.
- Keep the `/threads` route and existing view/temporary-admission semantics;
  do not add unnecessary deep-link rewrites. Make empty-state focus possible.
- Handle source unmount, response/event ordering, concurrent user focus,
  confirmation cancellation, and deterministic focus restoration.
- Add desktop Playwright outcomes for all six actions, nested choices,
  permission/provider eligibility, cancellation, failures, long content and
  keyboard/native-context coexistence; share test fixtures with Task 03.

## Out of scope

- New task business logic, provider forms, mobile drawer implementation, parent
  header/swiper replacement, or changes to task/session selection persistence.

## Acceptance

1. Desktop right-click and keyboard overflow expose the six groups and mutate
   only the opened task through shared hooks. Chat/editor context and normal
   session switching retain their behavior; linking and confirmation keep A
   despite changes to B or the globally selected session.
2. Pure/component tests prove survivor ordering, entire-view replacement,
   removal of descendants, empty/loading distinctions, late response guards,
   focus restoration, and no `/t/:id` navigation. A failed mutation neither
   removes the target permanently nor resets the reader's newer context.
3. The new desktop browser suite proves each persisted action outcome and an
   unchanged sibling, including reload. It covers Cancel, Escape/outside,
   nested keyboard navigation, long labels/lists at viewport edges, and existing
   Threads draft/order/activation regressions.

## Verification

Run from `apps/web`, with dependencies installed by Task 01. Add the pure,
component, and browser regressions and record RED before integrating the UI:

```bash
pnpm exec vitest run lib/threads/thread-selection-fallback.test.ts components/threads/thread-task-actions.test.tsx components/threads/threads-board.test.tsx app/threads/threads-page-client.test.tsx
pnpm e2e:run --project chromium tests/task/threads-task-actions.spec.ts -- --retries=0
```

After implementation, rerun the commands above, then the affected existing
regressions:

```bash
pnpm exec vitest run lib/threads/stable-order.test.ts lib/threads/thread-view-query.test.ts components/threads/thread-column-activation.test.tsx components/threads/thread-session-switcher.test.tsx
pnpm e2e:run --project chromium tests/task/threads-view.spec.ts tests/kanban/cross-workflow-task-move.spec.ts -- --retries=0
pnpm run typecheck
pnpm exec eslint --max-warnings 0 components/threads/thread-task-actions.tsx components/threads/thread-column.tsx components/threads/threads-board.tsx app/threads/threads-page-client.tsx lib/threads/thread-selection-fallback.ts components/task/task-item-menu-button.tsx e2e/tests/task/threads-task-actions.spec.ts e2e/tests/task/threads-task-actions-helpers.ts
```

Do not overlap managed E2E runs or override their worker budget. Confirm test
discovery. Use existing mock-agent/provider fixtures, causal waits and backend
state polling. A cancellation asserts both the surviving UI and no mutation
request; visibility of the confirmation alone is insufficient.

## Files likely touched

- `apps/web/app/threads/threads-page-client.tsx` and `.test.tsx`.
- `apps/web/components/threads/thread-task-actions.tsx` and `.test.tsx` (new),
  `thread-column.tsx`, `threads-board.tsx`, and focused existing tests.
- `apps/web/lib/threads/thread-selection-fallback.ts` and `.test.ts` (new).
- `apps/web/components/task/task-item-menu-button.tsx` and its focused tests.
- `apps/web/e2e/tests/task/threads-task-actions.spec.ts` and
  `threads-task-actions-helpers.ts` (new).

## Dependencies

Task 01. Use the integrated parent viewport/scroll helpers, not the old
pre-parent layout. Share only compact task metadata with the action host.

## Risks

An old global active task can equal the menu target even while Threads shows a
different conversation. Test this explicitly. Deep links can temporarily admit
a task despite view filters, and removal of a preceding column changes scroll
geometry even when the current task survives. Neither case justifies reranking
the deck or resetting saved preferences.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/threads-task-actions.md).
- [Design](../../specs/tasks/system-design/threads-task-actions.md): Header
  entry points, admission and selection recovery, dismissal and focus.
- `components/threads/AGENTS.md`, parent `mobile-threads` work orders,
  `threads-view.spec.ts`, and `cross-workflow-task-move.spec.ts`.
- `/tdd` and `/e2e` fixture/cleanup guidance.

## Results

Completed. The existing desktop header now has an always-visible overflow and
header-only context entry. Chat/editor/session context remains native. The
task-owned surface owns operation lifetime; Threads only provides entry,
viewport position and focus recovery. The page and deep-link admission policy
are unchanged.

RED/GREEN evidence includes the named overflow entry, pure fallback ordering,
removed-current-column focus, shared pending-flow regressions, and desktop
browser failures before the complete surface was wired. The final unit run in
`plan.md` passed **217 tests in 25 files**, including existing page, stable-order,
view-query, session and activation cases.

After `make -C apps/backend build` and `pnpm run build:e2e`, these managed
commands ran sequentially from `apps/web`:

```bash
pnpm e2e:run --host --no-build --project chromium tests/task/threads-task-actions.spec.ts tests/task/threads-view.spec.ts tests/kanban/cross-workflow-task-move.spec.ts -- --retries=0
pnpm e2e:run --host --no-build --project chromium tests/task/threads-task-actions.spec.ts -- --retries=0
```

The first run passed all **17 existing regressions** and five new tests. Its
remaining new test exposed a test-driver issue: teleporting the pointer across
a flipped submenu violated Radix pointer grace. The driver now uses stepped
pointer movement and explicit submenu activation; keyboard traversal has its
own outcome test. The final rerun passed **all six new tests**, without retries.

The six tests cover persisted six-action outcomes and unchanged B, failure and
retry for each mutation, late archive completion with B's menu open, archive
classification after filtering removes A's opener, viewport-bound long nested
menus, and header/keyboard/native-editor context. Cancellation asserts no
destructive request; successful final deletion focuses the real empty state.
Typecheck, affected-file ESLint and formatting passed.

### Header alignment correction (2026-09-10)

User screenshot feedback exposed a 2px vertical mismatch between the 28px Open
task button and the 24px overflow button. A centered inline action group now
aligns them without changing the title/status placement, desktop hit sizes, or
task-action behavior. It also aligns the larger coarse-pointer overflow.

The existing keyboard/context-menu regression gained scoped SVG-center
assertions. RED measured centers at 75px and 73px. After a fresh
`pnpm --filter @kandev/web build:e2e` from `apps`, this command from `apps/web`
passed one test without retries:

```bash
pnpm e2e:run --host --no-build --project chromium tests/task/threads-task-actions.spec.ts -- --grep 'keeps native chat context' --retries=0
```

Typecheck, ESLint for the two header components and three changed browser-test
files, formatting, and `git diff --check` passed. The shared action handlers and
selection logic were not changed.
