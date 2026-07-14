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
cd apps && pnpm --filter @kandev/web test -- --run app/settings/workspace/use-workflow-creation.test.ts components/settings/use-serialized-mutation-queue.test.ts components/settings/workflow-card-actions.test.ts
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

## Completion Report

- Behavior: Add Workflow persists before exposing the card; workflow and step mutations use separate ordered queues with a combined autosave status and exact-operation retry; failed rollback keeps the partial workflow recoverable.
- Files: creation hook/dialog/client, workflow card/actions, serialized mutation queue, and focused unit tests.
- Tests: the targeted creation, queue, and card-action Vitest files plus web typecheck passed.
- Blockers: none.
- Residual races: cross-tab concurrent edits are out of scope; same-card writes are serialized within their metadata or step stream, and stale metadata completions are ignored.
