---
id: "01-resolve-task-workflow-next-step"
title: "Resolve composer next step from the task workflow"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-002
acceptance_criteria:
  - AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-002.1
  - AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-002.3
  - AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-002.4
  - AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-002.5
  - AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-002.6
system_design:
  - ../../specs/tasks/system-design/workflow-signal-gated-manual-move-visibility.md
---

# Task 01: Resolve composer next step from the task workflow

## Summary

The standard and passthrough composers show the adjacent step of the open
task's workflow when the selected board uses another workflow. The label,
preview, and manual move payload all identify the same task-owned target.

## In scope

- Start with the permanent cross-workflow hook test and observe its expected
  `null` versus `Implement` failure before changing the hook.
- Resolve the task and ordered steps from the task's own active or cached
  workflow projection, including hydration and cross-workflow task switches.
- Preserve the existing auto-transition, busy, clarification, and move-error
  behavior in both composer surfaces.
- Add focused Chromium and phone browser flows that click the existing button
  while a different board workflow is selected.

## Out of scope

- Backend task movement, completion signals, new UI controls or copy, and
  responsive layout changes.

## Acceptance

- With a selected Kanban board and an idle Feature/Analysis task present only
  in the Feature snapshot, both composers resolve `Implement` and the Feature
  workflow target; clicking moves to Feature/Implement.
- A missing or placeholder task workflow snapshot cannot produce a next-step
  action from the selected board. Arrival of the snapshot restores the
  correct action without a reload.
- Existing signal-gated, automatic, busy, clarification, and rejected-move
  behavior remains covered by the focused tests.

## ASCII UI preview

### UI-01: Task composer, Feature/Analysis task while Kanban is selected

See the combined preview in [plan.md](plan.md#ui-01-task-composer-featureanalysis-task-while-kanban-is-selected).

```text
Desktop, before: [Analysis]  Implement     Chat input (no action)
Desktop, after:  [Analysis]  Implement     [ Implement -> ] above chat input
Phone, after:    Feature / Analysis       [ Implement -> ] above chat input
```

The existing desktop hover options and phone tap/long-press Drawer remain. The
task-owned label and target satisfy `AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-002.6`;
spacing is illustrative and no control geometry changes.

## Verification

Run from the repository root:

```bash
(cd apps/web && pnpm exec vitest run hooks/domains/kanban/use-plan-actions.test.ts components/task/chat/chat-input-area.test.ts components/task/chat/chat-input-area.test.tsx components/task/passthrough-toolbar.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint hooks/domains/kanban/use-plan-actions.ts hooks/domains/kanban/use-plan-actions.test.ts components/task/chat/chat-input-area.tsx components/task/passthrough-toolbar.tsx e2e/tests/workflow/workflow-step-proceed.spec.ts e2e/tests/workflow/mobile-workflow-step-proceed.spec.ts)
(cd apps/web && pnpm e2e:run tests/workflow/workflow-step-proceed.spec.ts -- --grep "uses the task workflow when another board is selected" --retries=0)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/workflow/mobile-workflow-step-proceed.spec.ts -- --retries=0)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs/tasks docs/plans/fix-cross-workflow-proceed-control apps/web
```

The existing `apps/node_modules` is installed in this worktree. If work moves
to a fresh worktree, run `(cd apps && pnpm install --frozen-lockfile)` before
these commands.

## Files likely touched

- `apps/web/hooks/domains/kanban/use-plan-actions.ts`
- `apps/web/hooks/domains/kanban/use-plan-actions.test.ts`
- `apps/web/e2e/tests/workflow/workflow-step-proceed.spec.ts`
- `apps/web/e2e/tests/workflow/mobile-workflow-step-proceed.spec.ts`

Use an existing task-projection helper if it fits; keep any new helper local
to this path. The existing composer components should not need markup changes.

## Dependencies

None.

## Parallelism

`sequential`

## Inputs

- `REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-002` and its paired design.
- The exact failing diagnostic scenario in the plan overview.
- `resolveLatestTaskProjection` in
  `apps/web/components/task/task-page-content-helpers.ts` and the task-step
  selection pattern in `useWorkflowStepsById`.
- Existing desktop and phone workflow proceed browser tests.

## Risks

- An old workflow snapshot may retain a task after a move. Use the freshest
  task projection and select steps by that projection's workflow identity.
- A label-only correction can still submit the wrong `workflow_id`; assert the
  preview and move request as well as the text.
- A delayed snapshot must not bring back a stale button for the prior task.

## Results

- The hook derives the task workflow, current step, adjacent step, preview,
  work-step classification, and move payload from one task-owned projection.
- Missing and placeholder workflow snapshots keep the action hidden; a loaded
  snapshot restores it without a reload.
- Unit coverage proved the pre-fix `null` label and `Wrong next step` leakage.
  The focused suite passed: 4 files, 96 tests.
- Frontend typecheck, scoped ESLint, document catalog validation, spec-linter
  tests, full spec lint, and scoped diff checks passed.
- Chromium and `mobile-chrome` tests both switched from a task on workflow A
  to an idle Feature task and verified moving to Feature/Implement.
- The managed E2E runner built the backend and Vite frontend. Its default
  `/tmp` volume was full, so the run used a task-local temporary directory on
  the root filesystem.
