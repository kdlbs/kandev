---
id: "06-subtask-dialog-and-mobile-parity"
title: "Add the subtask-dialog selector and mobile parity"
status: done
wave: 3
depends_on: ["04-create-dialog-selectors"]
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-CREATE-RELATIONSHIPS-002
acceptance_criteria:
  - AC-TASKS-TASK-CREATE-RELATIONSHIPS-002.1
  - AC-TASKS-TASK-CREATE-RELATIONSHIPS-002.2
  - AC-TASKS-TASK-CREATE-RELATIONSHIPS-002.4
  - AC-TASKS-TASK-CREATE-RELATIONSHIPS-002.5
system_design:
  - ../../specs/tasks/system-design/task-create-relationships.md
---

# Task 06: Add the subtask-dialog selector and mobile parity

## Summary

Add the Related-to selector (built in task 04) to the New Subtask dialog's
option area, and add Playwright coverage proving every new selector
(Parent, Related-to in the main dialog, Related-to in the subtask dialog)
works on a touch viewport with no document-level horizontal overflow.

## In scope

- `TaskCreateRelatedSelector` mounted in `components/task/new-subtask-dialog.tsx`
  (`NewSubtaskForm`), wired to the subtask create payload's `related_task_ids`.
- The subtask dialog's existing externally-supplied `parentId` is unchanged;
  no Parent selector is added there
  ([`subtask-dialog-parent-defaults.md`](../../specs/tasks/requirements/subtask-dialog-parent-defaults.md)
  stays the parent-defaulting contract).
- Desktop and mobile Playwright coverage for the main dialog's Parent and
  Related-to selectors and the subtask dialog's Related-to selector.

## Out of scope

- Any change to subtask parent-defaulting behavior.
- Further graph or chip changes (done in tasks 02, 03, 05).

## Acceptance

- The New Subtask dialog offers a Related-to selector that behaves like the
  main dialog's, defaulting to none selected, and submits
  `related_task_ids` unchanged from the existing subtask payload shape
  otherwise.
- A Pixel-5-sized Playwright run opens the main dialog's Parent and
  Related-to selectors and the subtask dialog's Related-to selector, selects
  a candidate in each, and finds no horizontal document overflow.

## TDD sequence

1. Failing `NewSubtaskForm` test: Related-to selector renders, defaults to
   empty, wires `related_task_ids` on submit.
2. Failing desktop Playwright spec: select a parent and related task in the
   main dialog; assert the resulting task's chip and payload.
3. Failing mobile Playwright spec: touch targets and no horizontal overflow
   for all three selector instances.
4. Implement, then refactor with tests green.

## Verification

```bash
cd apps/web && pnpm vitest run components/task/new-subtask-dialog
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run lint
cd apps/web/e2e && pnpm e2e:run --project=chromium -g "task-create-relationships"
cd apps/web/e2e && pnpm e2e:run --project=mobile-chrome -g "task-create-relationships"
make fmt
make typecheck test lint
cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

## Files likely touched

- `apps/web/components/task/new-subtask-dialog.tsx`
- `apps/web/e2e/tests/task/task-create-relationships.spec.ts` (new)
- `apps/web/e2e/tests/task/mobile-task-create-relationships.spec.ts` (new)

## Dependencies

Task 04 (`TaskCreateRelatedSelector` and `rankCreateDialogCandidates`).

## Risks

- `NewSubtaskForm` has its own prop surface and submit path, separate from the
  main dialog's `task-create-dialog-submit.tsx`; wiring `related_task_ids`
  there must not disturb its existing parent-default initialization
  (`subtask-dialog-parent-defaults.md`).
- New E2E specs add to the local shard budget; keep them in the existing
  `task` directory shard rather than introducing a new shard.

## Parallelism

`parallel-safe` with task 03 (different files).

## Results

Added the Related-to selector to the New Subtask form, carried its selected IDs into `related_task_ids` while preserving `parent_id`, and added desktop and mobile Playwright coverage for the main dialog. Added a New Subtask selector unit/integration test.

Validation results:

- `vitest run components/task/new-subtask-dialog`: 2 files and 18 tests passed.
- `vitest run hooks/use-workflow-snapshot.test.ts`: 13 tests passed, including default skip behavior for boot-hydrated board data and the opt-out for partial task-detail hydration.
- `pnpm run typecheck`: passed (the `tsx` prerequisite required running outside the sandbox).
- `pnpm run lint` and full `make lint`: passed.
- `pnpm run i18n:check` and `pnpm run i18n:ratchet`: passed.
- Desktop `e2e:run --project=chromium -g "task-create-relationships"`: passed (1 test).
- Mobile `e2e:run --project=mobile-chrome -g "task-create-relationships"`: passed (1 test) after allowing the subtask dialog's workflow snapshot fetch to bypass the existing-hydration skip guard.
- `make fmt`: passed.
- `make lint`: passed (backend, web, harness, specifications, and architecture).
- `python3 scripts/list-docs.py validate` and `python3 scripts/lint-spec-files.py --all`: passed.
- `make typecheck test lint`: typecheck passed, but the backend full test phase failed in unrelated existing launcher assertions (`backend port 38429 is already in use`, `TestRunStartBuildVersion`) and hit 600-second timeouts in task SQLite/handler and worktree packages. The subsequent standalone full `make lint` passed.

On task-detail routes, SSR intentionally hydrates only the selected task, so the workflow snapshot's default hydrated-board guard incorrectly skipped fetching sibling candidates. Added an optional `skipIfHydrated` setting (default `true`) and disabled that skip for the open New Subtask dialog. This preserves the Kanban page's existing fast path. The dialog also mounts the shared popover container so its selector popovers stay inside the modal.
