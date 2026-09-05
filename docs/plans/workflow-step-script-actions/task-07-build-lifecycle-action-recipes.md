---
id: 07-build-lifecycle-action-recipes
title: Build lifecycle action recipes
status: done
wave: 6
depends_on:
  - 06-build-desktop-workflow-inspector
plan: plan.md
requirements:
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-006
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-008
acceptance_criteria:
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-006.1
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-006.2
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-006.3
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-008.3
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-008.4
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-008.5
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-008.6
system_design:
  - ../../specs/tasks/system-design/workflow-step-script-actions.md
---

# Task 07: Build lifecycle action recipes

## Summary

Replace the legacy action controls with ordered lifecycle recipe groups and one
focused action editor, including complete `run_script` authoring.

## In scope

- Entry, agent-finish, and exit action groups with compact summaries.
- Contextual add palette, focused action selection/back, ordering, and removal.
- Focused editors for every existing action type and `run_script`.
- Inline validation, transition projection, issue navigation, manual save,
  read-only state, and localization.

## Out of scope

- Live Test action execution and mobile-specific screen composition.

## Acceptance

1. Every current action appears in exactly one compatible lifecycle group and
   retains its persisted order and runtime semantics.
2. Script editing exposes command, timeout, policy, lifecycle/session help, and
   validation without expanding other action forms.
3. Local action changes update summaries, issues, pipeline transitions, dirty
   markers, and Save eligibility before any persistence request.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps && pnpm --filter @kandev/web test -- --run components/settings/workflow-editor/actions components/settings/workflow-step-mutations components/settings/workflow-dirty-state
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
cd apps/web && pnpm run i18n:check
cd apps/web && pnpm run i18n:ratchet
git diff --check
```

## Files likely touched

- `apps/web/components/settings/workflow-editor/automation-tab.tsx`
- `apps/web/components/settings/workflow-editor/action-list.tsx`
- `apps/web/components/settings/workflow-editor/action-editor.tsx`
- `apps/web/components/settings/workflow-editor/actions/`
- `apps/web/components/settings/workflow-pipeline-editor-panels.tsx`
- Workflow locale catalogs and focused component tests.

## Dependencies

- Task 06 supplies the dedicated route and inspector shell.
- Task 05 supplies the action catalog and mutations.

## Risks

- Leaving legacy transition controls active would create two editors for one
  array.
- Reordering one trigger must not reorder actions in another trigger.
- Long commands must not widen the inspector.

## Parallelism

`sequential`. Mobile editing composes these action editors after they stabilize.

## Inputs

- Current step behavior/transition panels and prompt editor.
- Shared catalog, summaries, diagnostics, and route draft.

## Results

Implemented ordered lifecycle action recipes, compatible add palettes,
focused action editors, all current on-enter action descriptors, script
validation/defaults, transition projection, and localized copy. Focused
component/catalog tests and i18n gates pass.
