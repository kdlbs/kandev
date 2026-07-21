---
id: "02-workflow-drafts"
title: "Workflow drafts"
status: done
wave: 2
depends_on: ["01-save-coordinator"]
plan: "plan.md"
spec: "../../specs/ui/settings-manual-save.md"
---

# Task 02: Workflow Drafts

## Acceptance

- Workflow metadata, ordering, step fields, transitions, and additions issue no persistence request before the shared Save action.
- One Save persists all dirty workflows and maps client workflow/step identities without duplicate creation on retry.
- Confirmed deletion/migration/import/export remains immediate; deleting a persisted dirty resource warns that its draft is discarded.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run app/settings/workspace/use-workflow-creation.test.ts components/settings/workflow-card-actions.test.ts components/settings/workflow-pipeline-editor-helpers.test.ts
cd apps/web && pnpm run typecheck
```

## Files Likely Touched

- `apps/web/app/settings/workspace/workspace-workflows-client.tsx`
- `apps/web/app/settings/workspace/use-workflow-creation.ts`
- `apps/web/app/settings/workspace/use-workflow-creation.test.ts`
- `apps/web/components/settings/workflow-card.tsx`
- `apps/web/components/settings/workflow-card-actions.ts`
- `apps/web/components/settings/workflow-card-actions.test.ts`
- `apps/web/components/settings/workflow-card-dialogs.tsx`
- `apps/web/components/settings/workflow-pipeline-editor.tsx`
- `apps/web/components/settings/workflow-pipeline-editor-step-actions.tsx`
- `apps/web/components/settings/workflow-pipeline-editor-helpers.tsx`
- `apps/web/hooks/use-workflow-snapshot.ts`

## Dependencies

Task 01.

## Inputs

- Spec: Workflow settings, State Machine, workflow Scenarios.
- Existing workflow snapshots and current serialized mutation coverage.

## Output Contract

Report draft model, save operation order, identity remapping, immediate destructive paths, tests run, residual partial-save risks, and update task/plan status.

## Completion Notes

- Workflow cards register route save contributors; metadata, step fields, transitions, additions, and ordering stay in local React drafts.
- Save creates or updates workflow metadata, reconciles template steps, creates missing steps, remaps step references, updates changed steps, and finally persists step and workflow ordering.
- Mutable save progress retains the created workflow and client-to-server step IDs so a partial failure retries only missing work instead of duplicating resources.
- Persisted workflow/step deletion and migration remain confirmed immediate operations. Unsaved resources are removed locally, and destructive dialogs warn when drafts will be discarded.
- Focused workflow tests, workflow settings synchronization tests, frontend typecheck, scoped lint, and `git diff --check` pass.
- Backend endpoints remain non-transactional. A failed save can leave a partially created workflow on the server; its draft remains retryable, while Discard attempts best-effort cleanup.
