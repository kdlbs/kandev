---
id: 05-build-workflow-editor-view-model
title: Build the workflow editor view model
status: done
wave: 4
depends_on:
  - 04-integrate-workflow-triggers
plan: plan.md
requirements:
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-006
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-008
acceptance_criteria:
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-006.1
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-006.3
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-008.2
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-008.4
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-008.5
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-008.10
system_design:
  - ../../specs/tasks/system-design/workflow-step-script-actions.md
---

# Task 05: Build the workflow editor view model

## Summary

Create a shared action catalog, immutable mutations, compact workflow
summaries, and resolvable diagnostics over the existing workflow wire model.

## In scope

- Characterization of every current action and transition serialization shape.
- Catalog descriptors for labels, summaries, compatible triggers, editors, and
  validation, including `run_script`.
- Derived step summaries, transition edges, effective profile labels, dirty
  state, and configuration issue targets.
- Selection repair after action deletion/reordering.

## Out of scope

- Desktop/mobile layout, routing, process execution, and new persisted defaults.

## Acceptance

1. The catalog exposes only compatible actions for a lifecycle trigger and
   round-trips every existing and script action without semantic loss.
2. One pure view model derives pipeline summaries and issue targets from saved
   baseline plus route-local draft.
3. Desktop and mobile can invoke the same typed add/edit/reorder/delete
   mutations, including deterministic selection repair.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps && pnpm --filter @kandev/web test -- --run components/settings/workflow-editor lib/workflows
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
git diff --check
```

## Files likely touched

- `apps/web/lib/types/http.ts`
- `apps/web/lib/types/workflow-actions.ts`
- `apps/web/lib/workflows/workflow-action-catalog.ts`
- `apps/web/lib/workflows/workflow-editor-view-model.ts`
- `apps/web/components/settings/workflow-step-mutations.ts`
- `apps/web/components/settings/workflow-dirty-state.ts`
- Focused unit tests beside these files.

## Dependencies

- Task 04 establishes the final server action and metadata contract.

## Risks

- Object normalization can drop unknown synchronized action fields.
- Position-based transient selection can point at a different action after
  reorder unless repaired as one mutation.

## Parallelism

`sequential`. Both viewport compositions depend on this shared layer.

## Inputs

- Existing `WorkflowStep` event arrays, transition diagnostics, dirty helpers,
  and profile inheritance logic.
- Task 01 action defaults and Task 04 server response shapes.

## Results

Implemented the portable action catalog, immutable lifecycle mutations,
selection repair, pipeline transition projection, workflow summaries, and
configuration issue targets. Focused frontend tests and typecheck pass.
