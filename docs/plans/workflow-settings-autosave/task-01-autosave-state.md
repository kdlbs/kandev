---
id: "01-autosave-state"
title: "Autosave state"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/workflow-settings-autosave/spec.md"
---

# Task 01: Autosave State

## Acceptance

- Add Workflow persists the selected template or default custom steps without a second Save action.
- Workflow metadata and step mutations report one card-level Saving/Saved/Error state and the most recent failed operation can be retried.
- No workflow card renders a manual Save control.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/settings/workflow-card-actions.test.ts
cd apps/web && pnpm run typecheck
```

## Files Likely Touched

- `apps/web/app/settings/workspace/workspace-workflows-client.tsx`
- `apps/web/app/settings/workspace/workspace-workflows-dialogs.tsx`
- `apps/web/components/settings/workflow-card.tsx`
- `apps/web/components/settings/workflow-card-actions.ts`
- `apps/web/components/settings/workflow-card-actions.test.ts`

## Inputs

- Spec: What, Failure Modes, and the first four Scenarios.
- Existing `useRequest` status pattern and workflow-step reconciliation helpers.

## Output Contract

Report behavior implemented, files changed, targeted tests run, blockers, residual races, and update this task plus `plan.md` to done.
