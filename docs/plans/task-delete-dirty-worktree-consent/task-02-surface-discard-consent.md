---
id: "02-surface-discard-consent"
title: "Add explicit discard consent to task deletion"
status: done
wave: 2
depends_on:
  - "01-enforce-delete-boundary"
plan: "plan.md"
spec: "../../specs/ui/requirements/confirmation-warning-hierarchy.md"
---

# Task 02: Add Explicit Discard Consent to Task Deletion

## Outcome

Every desktop and mobile task-delete surface gives the user an explicit choice
before local worktree changes can be discarded. A typed backend conflict keeps
the task visible and explains how to retry.

## In scope

- Extend the shared delete-dialog callback and API request options.
- Show and reset the worktree discard choice for single, bulk, and cascade cases.
- Disable Delete until required consent is selected.
- Handle the typed conflict across all task-delete entry points.
- Add copy in all five locales and regenerate Traditional Chinese and pseudo
  catalogs with the repository commands.
- Add desktop and mobile browser proof.
- Update the public Git operations cleanup guidance.

## Exclusions

- A cleanup-job status or replay UI.
- Automatic commit, push, patch, or backup actions.
- Changes to compact archive confirmation.

## Requirements and design

- `REQ-UI-TASK-CLEANUP-CONFIRMATION-001`
- `AC-UI-TASK-CLEANUP-CONFIRMATION-001.8`
- `AC-UI-TASK-CLEANUP-CONFIRMATION-001.9`
- `AC-UI-TASK-CLEANUP-CONFIRMATION-001.10`
- `AC-TASKS-RUNTIME-CLEANUP-001.10`
- `AC-TASKS-RUNTIME-CLEANUP-001.11`
- `AC-TASKS-RUNTIME-CLEANUP-001.12`
- `docs/specs/ui/system-design/confirmation-warning-hierarchy.md`
- `docs/specs/tasks/system-design/dirty-worktree-deletion.md`
- `docs/specs/tasks/system-design/runtime-cleanup.md`

## Acceptance

- The shared dialog requires the explicit choice whenever the selected delete
  can remove a worktree and passes both cascade and discard values unchanged.
- A typed dirty-worktree conflict preserves the task in client state and shows
  localized retry guidance on every single and bulk delete surface.
- Desktop and phone E2E flows prove preservation without consent and removal
  with consent; the phone choice has a 44 CSS px target inside the scroll body.

## Verification

```bash
(cd apps/web && pnpm exec vitest run components/task/task-delete-confirm-dialog.test.tsx lib/api/domains/kanban-api.test.ts hooks/use-task-actions.test.ts)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:pseudo)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run tests/task/sidebar-delete-confirm.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-confirmation-text-hierarchy.spec.ts)
node --test scripts/validate-public-docs.test.mjs && node scripts/validate-public-docs.mjs
```

Write the dialog and API tests first. They must fail because the current callback
contains only `cascade` and the current request has no discard query parameter.

## Files likely touched

- `apps/web/components/task/task-delete-confirm-dialog.tsx`
- `apps/web/components/task/task-delete-confirm-dialog.test.tsx`
- `apps/web/components/kanban-card.tsx`
- `apps/web/components/task/task-session-sidebar-dialogs.tsx`
- `apps/web/components/task/task-session-sidebar-selection.tsx`
- `apps/web/components/task/mobile/session-task-switcher-sheet.tsx`
- `apps/web/components/kanban/graph2-task-pipeline.tsx`
- `apps/web/components/kanban/task-multi-select-toolbar.tsx`
- `apps/web/hooks/use-task-actions.ts`
- `apps/web/hooks/use-task-actions.test.ts`
- `apps/web/hooks/use-task-multi-select.ts`
- `apps/web/hooks/use-sidebar-multi-select.ts`
- `apps/web/lib/api/domains/kanban-api.ts`
- `apps/web/lib/api/domains/kanban-api.test.ts`
- `apps/web/src/locales/en/task.json`
- `apps/web/src/locales/pt-pt/task.json`
- `apps/web/src/locales/zh-cn/task.json`
- `apps/web/src/locales/zh-hk/task.json`
- `apps/web/src/locales/zh-tw/task.json`
- `apps/web/e2e/tests/task/sidebar-delete-confirm.spec.ts`
- `apps/web/e2e/tests/task/mobile-confirmation-text-hierarchy.spec.ts`
- `docs/public/git-operations.md`

## Dependencies

- Task 01 must define the typed conflict and request contract.

## Parallelism

Sequential after Task 01. The browser proof depends on the real backend contract.

## Risks

- The dialog must reset consent after cancel or close.
- Bulk requests can have partial success if checkout state changes after the
  shared confirmation.
- A cascade can contain worktree executors that are not visible in the root task.

## Output contract

Report the RED tests, every delete entry point updated, localized keys, desktop
and mobile outcomes, public documentation changes, and exact verification
results. Update this work order and `plan.md` in the same conversation.

## Results

- Extended the shared delete dialog and API contract with independent cascade and discard values.
  The unchecked discard choice appears for worktree, bulk, and cascade-capable deletes, uses a
  44-CSS-pixel label target, resets on close, and keeps Delete disabled until consent is explicit.
- Propagated the option through sidebar, mobile, Kanban card, graph, list, bulk-selection, and
  task-message delete entry points. Typed conflicts now use shared localized retry guidance without
  removing the task from client state.
- Added the four new task-copy keys to `en`, `pt-pt`, `zh-cn`, `zh-hk`, `zh-tw`, and regenerated
  pseudo output. Updated public Git operations documentation.
- Verification passed: 29 focused Vitest tests; web lint, typecheck, and i18n checks; desktop
  sidebar E2E (1 passed); mobile confirmation/layout E2E (4 passed); and public-doc validation
  (61 tests, 45 pages). The backend suite above proves physical dirty-worktree preservation and
  consented cleanup; the browser flows prove the shared desktop/mobile consent state.
- Review follow-up moves `useTranslation` before the discard-option early return, preserving hook
  order while the async cascade capability resolves.
- The mobile confirmation flow now materializes a real worktree, writes an uncommitted marker,
  proves cancel and unconsented deletion preserve both task and file, then proves consented
  deletion removes both. Follow-up verification passed 58 focused Vitest tests, web typecheck,
  targeted ESLint, and `pnpm e2e:run --project mobile-chrome
  tests/task/mobile-confirmation-text-hierarchy.spec.ts` (4 passed).
- PR follow-up aligned the test-only E2E reset and existing browser deletion fixtures with the
  explicit discard-consent contract. Focused Chromium deletion/reset coverage passed 37 tests,
  and the mobile dirty-worktree flow passed 4 tests.
