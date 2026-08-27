---
id: "06-save-selection-as-set"
title: "Save the current selection as a set"
status: done
wave: 6
depends_on: ["05-apply-set-in-picker"]
plan: "plan.md"
spec: "../../specs/workspaces/requirements/repository-sets.md"
---

# Task 06: Save The Current Selection As A Set

## Acceptance

- The Sets control offers **Save as set**, opening a small dialog that takes a name and optional
  description and creates a set from the workspace repositories currently selected in the form, in
  row order, deduped, ignoring blank rows and ignoring branches.
- Saving leaves the in-progress task untouched: no rows change, nothing is submitted, and the new set
  appears in the list immediately. The action is unavailable when no workspace repository row is
  filled.
- A duplicate name returns the service conflict and is shown inline in the dialog naming the existing
  set, with the dialog staying open and the draft intact. Other failures surface an error without
  losing the entered name.
- Copy exists in all five locales with no em dashes, and the dialog follows the phone pattern used by
  the Sets control.

## Verification

```bash
cd apps/web && pnpm run typecheck
cd apps/web && pnpm vitest run components/task-create-dialog-repository-sets-save.test.tsx components/task-create-dialog-repository-sets-control.test.tsx
cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet
```

## Files likely touched

- `apps/web/components/task-create-dialog-repository-sets-save.tsx` (new) and its `.test.tsx`
- `apps/web/components/task-create-dialog-repository-sets.ts` (selection-to-request mapping) and its
  `.test.ts`
- `apps/web/components/task-create-dialog-repository-sets-control.tsx`
- `apps/web/lib/api/domains/workspace-api.ts` (reuses `createRepositorySet`)
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw}/`

## Dependencies

Task 05 for the control this action lives in; task 04 for the API client and slice upsert.

## Inputs

- Spec: Defining a set (save-from-dialog path, name rules), Failure modes (`409` naming the existing
  set).
- Only rows carrying a workspace `repositoryId` are eligible; discovered `localPath` rows and remote
  rows are not workspace repository entities and are excluded.
- The selection-to-request mapping is pure and unit-tested separately from the dialog.

## Risks

- A discovered-but-not-imported row has a `localPath` and no `repositoryId`; including it would send
  an id the backend rejects with `422`. Filter client-side and say so in the dialog when rows are
  excluded.
- Creating a set mid-draft must not trigger the dialog's dirty/reset paths.

## Output contract

Summary, files changed, tests run with results, blockers, risks, divergence from the plan, and
task/plan status updates.
