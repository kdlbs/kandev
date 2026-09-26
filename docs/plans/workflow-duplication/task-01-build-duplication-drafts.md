---
id: "01-build-duplication-drafts"
title: "Build duplication drafts"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/workflow-duplication.md"
---

# Task 01: Build Duplication Drafts

## Acceptance

- A pure constructor creates an independent temporary workflow and step graph with the next collision-free copy name and no source/template/sync identity.
- Nested transition references and pull-from relationships resolve only to copied step IDs, while every editable workflow setting except the generated workflow `name`, and every editable step setting, remains equal to the source.
- A persisted semantic step type is carried through the existing list, create, and update transport so draft save does not reset it to `custom`.
- `useWorkflowCreation` inserts the draft directly after the source and registers its initial steps without issuing a persistence request.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web test -- --run app/settings/workspace/workflow-duplication.test.ts app/settings/workspace/use-workflow-creation.test.ts && cd web && pnpm run typecheck
```

## Files Likely Touched

- `apps/web/app/settings/workspace/workflow-duplication.ts`
- `apps/web/app/settings/workspace/workflow-duplication.test.ts`
- `apps/web/app/settings/workspace/use-workflow-creation.ts`
- `apps/web/app/settings/workspace/use-workflow-creation.test.ts`
- `apps/web/app/settings/workspace/workspace-workflows-client.tsx`
- `apps/web/app/actions/workspaces.ts`
- `apps/web/app/actions/workspaces.test.ts`
- `apps/backend/internal/workflow/controller/controller.go`
- `apps/backend/internal/workflow/handlers/step_events_test.go`

## Dependencies

None.

## Parallelism

Sequential. This task defines the draft contract consumed by the UI task.

## Inputs

- Spec sections: What, Persistence Guarantees, naming/reference Scenarios.
- Existing temporary workflow creation and identity remapping in `use-workflow-creation.ts` and `workflow-card-actions.ts`.
- Existing route-level workflow ordering and save-contributor flow in `workspace-workflows-client.tsx`.

## Risks

- A shallow copy can let editor mutations change the source's nested event objects.
- Missing a nested `step_id` or `pull_from_step_id` remap can make the copied workflow transition back into the source graph or fail validation.
- Carrying `workflow_template_id` or sync provenance can create unwanted server steps or make the copy read-only.

## Output Contract

Report the naming rules, copied/omitted fields, reference-remapping behavior, files changed, exact tests run, blockers, risks, and task/plan status updates.

## Results

- Added pure copy-name and workflow/step graph helpers with new temporary IDs,
  independent nested event objects, and remapped step references.
- Added route-local insertion after the source and initial-step registration in
  `useWorkflowCreation`.
- Checks passed:
  - `cd apps && pnpm --filter @kandev/web test -- --run app/settings/workspace/workflow-duplication.test.ts app/settings/workspace/use-workflow-creation.test.ts`
    (2 files, 11 tests).
  - `cd apps && pnpm --filter @kandev/web test -- --run app/actions/workspaces.test.ts app/settings/workspace/use-workflow-duplication.test.ts app/settings/workspace/workflow-duplication.test.ts components/settings/workflow-card-actions.test.ts`
    (4 files, 41 tests after review fixes).
  - `cd apps/backend && go test ./internal/workflow/handlers -run 'TestHTTP(UpdateStepPublishesUpdatedEvent|CreateStepPublishesCreatedEvent)' -count=1`.
  - `cd apps/web && pnpm run typecheck`.
