---
id: "02-apply-saved-bases"
title: "Apply saved bases to task drafts"
status: done
wave: 2
depends_on:
  - "01-persist-member-base-branches"
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-REPOSITORY-SETS-003
acceptance_criteria:
  - AC-WORKSPACES-REPOSITORY-SETS-003.2
  - AC-WORKSPACES-REPOSITORY-SETS-003.3
  - AC-WORKSPACES-REPOSITORY-SETS-003.4
  - AC-WORKSPACES-REPOSITORY-SETS-003.5
  - AC-WORKSPACES-REPOSITORY-SETS-003.6
  - AC-WORKSPACES-REPOSITORY-SETS-003.7
  - AC-WORKSPACES-REPOSITORY-SETS-003.8
  - AC-WORKSPACES-REPOSITORY-SETS-003.9
system_design:
  - ../../specs/workspaces/system-design/repository-sets.md
---

# Task 02: Apply Saved Bases To Task Drafts

## Summary

Carry member bases through frontend types and set application. Keep the base
separate from local checkout state during task submission.

## In scope

- Extend frontend HTTP, WebSocket, and store member types.
- Copy saved bases into new task rows.
- Preserve existing rows during repeated or overlapping application.
- Block silent fallback for an unavailable copied base.
- Send separate base and checkout values for local execution.
- Make `Save as set` send ordered members with effective bases.

## Out of scope

- Settings editor composition.
- Translation catalogs except copy owned by `Save as set`.
- Browser E2E coverage.

## Acceptance

- Applying a set copies each available saved base into a new row.
- Existing rows remain unchanged and duplicate application stays idempotent.
- Local payloads never use a copied base as checkout without user action.

## Verification

Run this command from `apps/web`.

```bash
pnpm exec vitest run lib/api/domains/repository-sets-api.test.ts lib/ws/handlers/repository-sets.test.ts lib/state/slices/workspace/repository-sets-slice.test.ts components/task-create-dialog-repository-sets.test.ts components/task-create-dialog-helpers.multi-repo.test.ts components/task-create-dialog-repository-sets-save.test.tsx
```

## Files likely touched

- `apps/web/lib/types/http.ts`
- `apps/web/lib/types/backend.ts`
- `apps/web/lib/api/domains/workspace-api.ts`
- `apps/web/lib/ws/handlers/repository-sets.ts`
- `apps/web/lib/state/slices/workspace/workspace-slice.ts`
- `apps/web/components/task-create-dialog-types.ts`
- `apps/web/components/task-create-dialog-repository-sets.ts`
- `apps/web/components/task-create-dialog-repository-sets-apply.ts`
- `apps/web/components/task-create-dialog-repository-sets-save.tsx`
- `apps/web/components/task-create-dialog-helpers.ts`
- Related unit tests

## Dependencies

- Task 01 supplies the member transport shape.

## Risks

- Existing auto-selection can overwrite a copied saved base.
- Local and worktree executors interpret the current `branch` field differently.

## Parallelism

`sequential`

## Inputs

- `REQ-WORKSPACES-REPOSITORY-SETS-003`
- System-design sections for task application and local executor semantics
- Existing pure apply tests and multi-repository payload tests

## Results

- Copied saved member bases into new task rows while preserving empty branch
  state for local checkout semantics.
- Kept application additive and idempotent, blocked unavailable copied bases,
  and preserved separate local base and checkout payload values.
- Updated Save as set to send ordered member objects with effective bases.
- Verification: the work-order Vitest command passed 6 files and 82 tests.
