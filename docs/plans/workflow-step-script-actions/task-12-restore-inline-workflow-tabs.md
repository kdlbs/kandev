---
id: 12-restore-inline-workflow-tabs
title: Restore inline workflow editing with compact tabs
status: pending
wave: 9
depends_on:
  - 05-build-workflow-editor-view-model
  - 07-build-lifecycle-action-recipes
plan: plan.md
requirements:
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-006
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-008
acceptance_criteria:
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-006.1
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-006.4
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-008.1
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-008.2
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-008.3
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-008.6
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-008.7
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-008.8
system_design:
  - ../../specs/tasks/system-design/workflow-step-script-actions.md
---

# Task 12: Restore inline workflow editing with compact tabs

## Summary

Remove the dedicated workflow-editor experience and integrate compact Agent,
Automation, and Policies tabs into the existing inline selected-step panel.

## In scope

- Keep workflow-level fields, new-workflow creation, the horizontal step strip,
  selected-step panel, and save contributor inside the existing workflow card.
- Move shared tab content, lifecycle recipes, focused action editing, validation,
  and selection repair into that panel.
- Keep step name and color in the panel identity row; preserve turn-start,
  child-task-completion, auto-archive, WIP/pull, and every other existing editor
  capability under the appropriate tab.
- Use compact desktop tabs and touch-safe phone tabs without a journey or
  full-height editor route.
- Remove dedicated route registration, links, route draft/navigation helpers,
  desktop editor shell, mobile journey screens, and superseded copy/tests.
- Preserve default workflow agent-profile and description editing on every
  supported viewport.

## Out of scope

- Runtime or workflow wire-format changes.
- A freeform canvas or new transition semantics.

## Acceptance

1. Existing and new workflows are fully editable without leaving the Workflows
   settings page, and all workflow-level and step-level controls remain
   available.
2. Selecting a step reveals one inline panel whose compact tabs organize all
   current controls and lifecycle actions without duplicate editors.
3. Multiple dirty workflow cards save through the existing shared coordinator.
4. Phone layouts retain the inline hierarchy, 44-pixel targets, explicit move
   controls, and no document-level horizontal overflow.

## Verification

```bash
cd apps/web && pnpm exec vitest run components/settings/workflow-card-actions.test.ts components/settings/workflow-editor lib/workflows/workflow-editor-view-model.test.ts
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet
git diff --check
```

## Files likely touched

- `apps/web/app/settings/workspace/workspace-workflows-client.tsx`
- `apps/web/components/settings/workflow-card.tsx`
- `apps/web/components/settings/workflow-pipeline-editor*.tsx`
- `apps/web/components/settings/workflow-editor/`
- `apps/web/src/settings-routes*.tsx`
- Workflow locale catalogs and component tests.

## Dependencies

- Tasks 05 and 07 supply reusable view-model, catalog, and action-editor pieces.

## Risks

- Keeping both old transition controls and recipe actions would create two
  writers for the same events arrays.
- Removing route state must not drop focused-action selection repair or dirty
  drafts.

## Parallelism

`parallel-safe` with Task 11. This task owns frontend editor composition only.

## Results

Pending implementation.
