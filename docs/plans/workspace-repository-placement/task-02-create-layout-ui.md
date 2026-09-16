---
id: "02-create-layout-ui"
title: "Offer parent-root task creation"
status: completed
wave: 2
depends_on: ["01-durable-layout"]
plan: "plan.md"
requirements:
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-002
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-004
acceptance_criteria:
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.1
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.2
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.3
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.4
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.5
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-004.3
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-004.4
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-004.5
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-004.6
system_design:
  - ../../specs/tasks/system-design/workspace-repository-placement.md
---

# Task 02: Offer parent-root task creation

## Summary

Add the accepted Advanced control and accessible help to task creation.
Use the backend layout contract from Task 01 and prove the resulting initial workspace.

## In scope

- Shared creation state, defaults, reset, request builders, and Advanced rendering.
- Off-by-default single-repo Worktree choice. Multi-repo explanation and unsupported-source/executor handling.
- Desktop hover/focus help and phone/coarse-pointer help drawer.
- Localized copy in all supported catalogs and generated pseudo/Traditional Chinese values.

## Out of scope

Add sources placement controls, remembered global preferences, and session expansion.

## Acceptance

1. UI-01 and UI-02 match the rendered structure. The choice reaches the create request and produces the selected CWD.
2. Executor/repository changes cannot submit a hidden stale value. New unrelated dialogs reset to the default.
3. Desktop keyboard and phone touch users can read the same help and complete creation.

## ASCII UI preview

### UI-01: Create task, Advanced, desktop

Entry: New task dialog, one repository, Worktree executor, Advanced open. AC-002.1-.5 and AC-004.3.
The new control follows existing dependency and priority controls. Existing controls are not removed.

```text
+----------------------------------------------------------------+
| Create task                                                [X] |
| ... existing task, repository, agent and executor controls ...  |
|                                                                |
| v Advanced                                                     |
| Depends on [None v]                  Priority [Normal v]       |
|                                                                |
| [ ] Start in a parent workspace folder                     (i) |
|                                                                |
| Help on hover or keyboard focus:                               |
| Start above the repository so you can add sibling repositories  |
| later without moving the agent's working directory.            |
| Some agents may discover repository instructions and skills     |
| differently with this layout.                                  |
|                                                                |
|                                        [Cancel] [Create task]  |
+----------------------------------------------------------------+
```

### UI-02: Create task, Advanced, phone

Same entry and criteria as UI-01. One-column Advanced controls. Tapping (i) opens an inset help drawer, not a hover tooltip.

```text
+------------------------------------+
| Create task                    [X] |
| ... existing fields ...            |
| v Advanced                         |
| Depends on [None v]                |
| Priority   [Normal v]              |
|                                    |
| [ ] Start in a parent              |
|     workspace folder           (i) |
|------------------------------------|
| [Cancel]            [Create task]  |
+------------------------------------+

        Help drawer after tapping (i)
+------------------------------------+
| Parent workspace folder        [X] |
| Start above the repository to add  |
| siblings later without moving the |
| agent's working directory.        |
|                                    |
| Instruction and skill discovery   |
| can differ between agents.        |
+------------------------------------+
```

Multiple initial repositories replace the switch with: “A parent workspace is already used for multiple repositories.”
Unsupported executors and repositoryless tasks omit this control. Switching away must not submit a hidden enabled value.
Reopening an unrelated creation dialog returns to the default, not a prior task's selection.

Full preview: [plan](plan.md#ascii-ui-preview). Preserve existing dependency/priority controls.

## Tests and TDD

Extend `task-create-dialog-advanced-settings.test.tsx`, defaults, form-reset, and request-builder tests.
Create the initial-layout scenarios in the two proposed placement E2E files. Task 05 adds attachment scenarios later.
Assert actual created workspace paths, not only the checked state. Compare rendered UI-01/UI-02 and retain screenshots.

## Verification

Install workspace dependencies once before the first pnpm command in this worktree.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task-create-dialog-advanced-settings.test.tsx components/task-create-dialog-defaults.test.ts components/task-create-dialog-form-reset.test.ts components/task-create-dialog-prop-builders.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/task/workspace-repository-placement.spec.ts -- --grep 'initial layout')
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-workspace-repository-placement.spec.ts -- --grep 'initial layout')
```

## Files likely touched

- `apps/web/components/task-create-dialog-advanced-settings.tsx`
- `apps/web/components/task-create-dialog-state.ts`, `task-create-dialog-defaults.ts`, `task-create-dialog-form-reset.ts`
- `apps/web/components/task-create-dialog-prop-builders.ts`, `task-create-dialog-types.ts`, `task-create-dialog-submit.tsx`
- New focused parent-workspace help component if needed to retain component limits
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,pseudo}/task.json`
- Proposed `apps/web/e2e/tests/task/workspace-repository-placement.spec.ts`
- Proposed `apps/web/e2e/tests/task/mobile-workspace-repository-placement.spec.ts`

## Dependencies

Task 01. No dependency on the open recovery PR for creation-time layout.

## Inputs

Design: Presentation; Launch and reuse. Mobile exemplar: existing Advanced single-column layout and touch help drawer pattern.

## Risks

Mock E2E proves layout propagation, not native skill discovery. Record representative live-agent discovery smoke evidence in the final package results.


## Parallelism

`sequential`

## Results

Implemented the optional parent-root control, capability filtering, reset behavior, localized desktop and phone help, and create-request persistence. Added focused component/API tests and desktop/mobile E2E coverage. Desktop and phone creation flows passed against the built backend, including the persisted `task_root` value. `pnpm run typecheck`, `pnpm run lint`, `pnpm run i18n:check`, and `pnpm run build:e2e` passed. Mock E2E does not prove provider-specific instruction or skill discovery.
