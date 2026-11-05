---
id: "01-bulk-move-scope"
title: "Correct bulk-move workflow scope"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-BOARD-BULK-MOVE-SCOPE-001
  - REQ-TASKS-BOARD-PRIORITY-SORT-FILTER-002
  - REQ-TASKS-KANBAN-TASK-REORDERING-001
acceptance_criteria:
  - AC-TASKS-BOARD-BULK-MOVE-SCOPE-001.1
  - AC-TASKS-BOARD-BULK-MOVE-SCOPE-001.2
  - AC-TASKS-BOARD-BULK-MOVE-SCOPE-001.3
  - AC-TASKS-BOARD-PRIORITY-SORT-FILTER-002.10
  - AC-TASKS-BOARD-PRIORITY-SORT-FILTER-002.11
  - AC-TASKS-KANBAN-TASK-REORDERING-001.29
system_design:
  - ../../specs/tasks/system-design/board-priority-sort-filter.md
  - ../../specs/tasks/system-design/kanban-task-reordering.md
---

# Task 01: Correct bulk-move workflow scope

## Summary

Make the board's same-workflow bulk step move use the eligible selection's
verified workflow, independent of the hydrated board context. Preserve the
existing batch, ordering, error, and phone interaction paths.

## In scope

- Author independent integration regressions at the real batch API boundary;
  observe wrong A and absent null requests fail before changing production.
- Validate every eligible task and the destination against consistent current
  workflow evidence; align the offered destinations with the invocation guard.
- Correct the phone destination trigger in `TaskMultiSelectToolbar` using
  `RepositorySourceMenu`'s controlled open, pending-touch, pointer-cancel, and
  synthetic-click pattern; keep the sheet modal and the nested phone menu
  non-modal. Retain desktop modal, mouse, and keyboard behavior.
- Add focused desktop and phone browser regressions and retain existing controls.

## Out of scope

- Cross-workflow actions, broad board context changes, backend changes,
  per-task requests, translation changes, and unrelated UI redesign.

## Acceptance

- Eligible B selections submit exactly one B/B-step request with hydrated A or
  null; a single-workflow A selection still submits A/A-step. Unknown, mixed,
  duplicate or inconsistent task scope and invalid, missing, or mismatched
  destination send no request and apply no local update.
- The invocation snapshots eligible tasks once, preserves existing display
  sorting and backend placement authority, and keeps selection and the existing
  error toast on rejection. Hidden/priority-filtered tasks stay excluded.
- Desktop All Workflows and phone's focused workflow complete the actual move
  through their existing controls, with task workflow unchanged. On phone,
  opening choices sends no batch; an explicit destination tap sends exactly one
  batch for that workflow and step and produces the persisted task outcome.

## Verification

The independent routing RED and desktop browser RED have already been recorded.
The phone trace records an unintended request on trigger and three failed phone
attempts; retain that evidence. The phone regression asserts zero requests when
the trigger opens, then one intended request and the persisted result after
the real destination tap. Run affected checks sequentially, using fresh
managed E2E builds after production changes.
Use the pinned toolchain and acquire the shared heavy lease before installation,
typecheck, lint, or E2E. Assign each run its own `KANDEV_RUN_QUIET_DIR`.

```bash
cd apps/web && pnpm exec vitest run --project browser-locales --maxWorkers 1 hooks/use-task-multi-select-workflow.test.tsx lib/kanban/bulk-move-scope.test.ts
cd apps/web && pnpm exec vitest run --project browser-locales --maxWorkers 1 hooks/use-task-multi-select-workflow.test.tsx lib/kanban/bulk-move-scope.test.ts hooks/use-task-multi-select-bulk-move.test.ts hooks/use-task-multi-select.test.ts components/kanban-board.test.ts hooks/use-task-workflow-move.test.ts
cd apps/web && pnpm run typecheck
cd apps/web && pnpm exec eslint hooks/use-task-multi-select.ts hooks/use-task-multi-select-workflow.test.tsx components/kanban-board.tsx components/kanban/task-multi-select-toolbar.tsx
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/kanban/task-multi-select.spec.ts --grep 'All Workflows bulk move keeps|bulk move moves tasks to target step|bulk move lands tasks' --retries=0 --trace=on
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/kanban/mobile-task-multi-select-workflow.spec.ts --retries=0 --trace=on
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

Include any additionally changed source and test paths in focused lint; run
`pnpm run i18n:ratchet` from `apps/web` after staging new web files. Record
actual command exits and browser blockers without substituting unit evidence.

## Files likely touched

- `apps/web/hooks/use-task-multi-select.ts` and its focused tests.
- `apps/web/components/kanban-board.tsx` and its focused tests.
- `apps/web/components/kanban/task-multi-select-toolbar.tsx` and its focused tests.
- `apps/web/lib/kanban/bulk-move-scope.ts` and its focused tests.
- `apps/web/e2e/tests/kanban/task-multi-select.spec.ts` and
  `apps/web/e2e/tests/kanban/mobile-task-multi-select-workflow.spec.ts`.
- `docs/public/tasks-and-workflows.md` only if the implementation review finds
  a user-facing guidance gap.

## Dependencies

The routing portion was released and implemented. The corrected phone trigger
was released after review of the corrected package.

## Risks

A stale or partially hydrated snapshot must not authorize a wrong workflow;
backend errors and local state must remain consistent. A touch can otherwise
activate a destination while opening the menu. Preserve the existing phone
sheet composition and desktop interaction.

## Parallelism

Sequential.

## Inputs

- [Requirements](../../specs/tasks/requirements/board-priority-sort-filter-order.md)
  and [design](../../specs/tasks/system-design/board-priority-sort-filter.md).
- [Ordering requirement](../../specs/tasks/requirements/kanban-task-reordering.md)
  and [design](../../specs/tasks/system-design/kanban-task-reordering.md).
- `useTaskMultiSelect`, `useMultiSelectDerived`, `resolveBoardWorkflowId`,
  `useTaskWorkflowMove`, and `validateTaskMove`.

## Results

Routing RED: two hook failures and one control pass; desktop browser RED:
selected task remained in its old step. Phone attempts 37187, 65945, and 58222
failed; the last trace shows a current-step batch during the trigger tap before
any intended destination tap. Its raw DOM event target is not recorded. The
controlled touch regression then failed twice before production correction,
with desktop controls passing, and passed after correction.

Final verification after the last code change: 15 focused scope/hook tests,
7 toolbar tests, 3 desktop Chromium cases, and the phone Chromium case passed.
The phone case asserted zero batch requests after opening, one exact request on
the explicit destination tap, and the persisted task workflow and step. Scoped
ESLint, the i18n ratchet, and canonical 6 GiB typecheck passed. The initial
4 GiB typecheck exhausted its heap; the earlier 6 GiB typecheck also passed.

Review follow-up: a task-bearing empty-step placeholder previously masked
matching hydrated-board steps. Its regression failed before the fallback;
six board cases and thirteen hook cases passed after it, as did scoped lint and
canonical 6 GiB typecheck. Other-workflow and conflicting-target controls
remained in coverage.
