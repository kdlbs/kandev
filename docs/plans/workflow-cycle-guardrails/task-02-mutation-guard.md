---
id: "02-mutation-guard"
title: "Workflow mutation guard"
status: done
wave: 2
depends_on: ["01-cycle-analyzer"]
plan: "plan.md"
spec: "../../specs/workflow-cycle-guardrails/spec.md"
---

# Task 02: Workflow mutation guard

## Acceptance

- Existing update/add/delete/reorder handlers analyze their complete proposed
  shape before sending the current API mutation.
- Blocking proposals send no request; warning proposals execute exactly once
  only after confirmation; existing diagnostic identities and cycle-removing
  edits are not gated unless identity comparison is conservatively truncated.
- Successful remote mutations reconcile from their authoritative responses;
  reorder remains optimistic while pending and rolls back on request failure.
- Draft workflow save preflights before `createWorkflowAction`, with cancel
  leaving the draft and sending no create or step requests.

## Verification

```bash
(cd apps && pnpm --filter @kandev/web test -- components/settings/workflow-card-actions.test.ts lib/workflows/replay-cycle-analysis.test.ts)
(cd apps/web && pnpm run typecheck)
```

## Files Likely Touched

- `apps/web/components/settings/workflow-card-actions.ts`
- `apps/web/components/settings/workflow-card-actions.test.ts`
- `apps/web/components/settings/workflow-card.tsx`

## Dependencies

Task 01.

## Inputs

- Spec scenarios for existing mutations, baseline/proposed comparison, and
  draft creation.
- Analyzer and diagnostic identity contract from Task 01.
- Existing `useWorkflowStepActions` and `useWorkflowSaveActions` behavior,
  including temp-step remapping and request-error feedback.

## Output Contract

When finished, change this task's `status` to `done`, check it in `plan.md`, and
report guarded operations, request behavior tests, files changed, blockers, and
residual risks.
