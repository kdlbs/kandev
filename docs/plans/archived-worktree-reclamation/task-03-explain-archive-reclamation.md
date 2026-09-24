---
id: "03-explain-archive-reclamation"
title: "Explain retained worktrees in archive UI and docs"
status: done
wave: 3
depends_on:
  - "02-reclaim-clean-archived-worktrees"
plan: "plan.md"
requirements:
  - REQ-TASKS-DIRTY-WORKTREE-ARCHIVE-002
acceptance_criteria:
  - AC-TASKS-DIRTY-WORKTREE-ARCHIVE-002.6
system_design:
  - ../../specs/tasks/system-design/dirty-worktree-archive.md
---

# Task 03: Explain retained worktrees in archive UI and docs

## Summary

Make desktop and phone archive confirmations describe conditional worktree
removal and branch retention. Document the independent default-on task recheck
and optional storage schedule for operators.

## In scope

- Add archive-specific single-task and bulk worktree summaries in the shared
  helper. Keep delete confirmation's existing warning and discard-consent
  copy. Retain the archive preference and surface composition.
- Translate changed user-facing keys in all required locale catalogs.
- Update public task and storage guides; add focused component and desktop/
  phone rendered assertions.

## Out of scope

- A new warning dialog, dirty-file preflight API, or altered phone navigation.
- An archive action that overrides a user's disabled confirmation preference.

## Acceptance

- Existing desktop and phone confirmations explain that Git changes keep a
  checkout until it becomes clean, without claiming every branch is deleted.
- Confirmation bypass still archives immediately, and all required locale
  catalogs pass the i18n checks.
- Delete confirmation keeps its existing worktree warning and dirty-file
  consent behavior.
- Public docs distinguish task-lifecycle rechecks from optional storage
  maintenance and explain recovery of older quarantined entries.

## ASCII UI preview

UI-01 and UI-02 from the [plan](plan.md#ascii-ui-preview) apply to the shared
copy. The phone keeps its existing bottom sheet and primary action.

```text
Desktop confirmation:  Clean Git worktrees are removed.
                       A worktree with Git changes stays until clean.
Phone sheet:           The same message wraps within the sheet.
                       [Cancel]                  [Archive]
```

The message and action order are required; exact wording and line wraps are
illustrative. The sheet remains the only phone scroll surface.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task/task-cleanup-summary.test.ts components/task/task-archive-confirmation.test.tsx components/task/task-archive-confirm-dialog.test.tsx components/task/task-delete-confirm-dialog.test.tsx)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/task/archive-confirmation-preference.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/kanban/mobile-card-archive-confirmation.spec.ts)
```

## Files likely touched

- `apps/web/components/task/task-cleanup-summary.ts`
- `apps/web/components/task/task-cleanup-summary.test.ts`
- `apps/web/components/task/task-archive-confirmation.test.tsx`
- `apps/web/components/task/task-archive-confirm-dialog.test.tsx`
- `apps/web/components/task/task-delete-confirm-dialog.test.tsx`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja}/task.json`
- `apps/web/e2e/tests/task/archive-confirmation-preference.spec.ts`
- `apps/web/e2e/tests/kanban/mobile-card-archive-confirmation.spec.ts`
- `docs/public/tasks-and-workflows.md`
- `docs/public/operations.md`

## Dependencies

Task 02 delivers the behavior that this copy explains.

## Risks

- The compact inline confirmation and phone sheet share copy but wrap it
  differently; check the actual phone surface and touch action geometry.
- Current `confirm_task_archive=false` bypasses all confirmation copy by
  design; public docs must carry the lifecycle explanation for that case.

## Parallelism

`sequential`

## Inputs

- `REQ-TASKS-DIRTY-WORKTREE-ARCHIVE-002`, the paired system design, and the
  existing archive confirmation preference contract.
- `task-cleanup-summary.ts`, desktop confirmation, and the existing phone
  confirmation sheet as the nearest mobile exemplar.

## Results

- The targeted cleanup-summary and archive/delete component tests passed (95 tests).
- `pnpm run i18n:check` passed for all required catalogs.
- Desktop archive E2E passed (3 tests), including confirmation copy and preference bypass.
- Phone archive E2E passed (2 tests), including the confirmation sheet and preference bypass.
- Public-doc tests passed (62 tests), and the validator accepted all 47 published pages.
