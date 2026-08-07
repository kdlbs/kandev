---
id: "28-automation-settings"
title: "Azure watcher settings"
status: completed
wave: 13
depends_on:
  ["24-watcher-persistence", "25-watcher-polling", "26-watcher-dispatch"]
plan: "plan.md"
spec: "../../specs/azure-devops-integration/spec.md"
---

# Task 28: Azure Watcher Settings

## Acceptance

- Responsive Work-item watches and Pull-request watches support create/edit,
  enable draft, Run now, reset preview/reset, delete, error display, workflow/
  step/repository/branch/profile selection, cleanup, interval, and in-flight cap.
- Work-item forms require Azure project plus WIQL. PR forms require Azure
  project and keep optional Azure repository/status/creator/reviewer filters
  visually distinct from the required Kandev task repository/base branch.
- Creating or enabling a draft validates all provider and task-creation
  dependencies before POST/PATCH. Run now reports bounded check success/error
  without waiting for the next poll. Reset confirmation shows the previewed
  affected tasks and cleanup policy before mutation.
- Desktop tables and mobile cards expose the same actions; phone create/edit
  uses a full-height internally scrolling dialog/drawer with safe-area clearance
  and no hover-only control. Loading, empty, disabled, last-error, and
  last-checked states are available in both compositions.

## TDD Sequence

1. Add API/domain-hook tests for list/create/update/delete/trigger/reset paths,
   enable drafts, stale request handling, and query invalidation; run red before
   implementing Azure watch hooks.
2. Add pure form tests for both watch kinds, interval/in-flight semantics,
   provider-vs-Kandev repository fields, and dependency validation.
3. Add component tests for reset previews, error/empty/loading states, desktop
   table actions, and mobile card/drawer parity. Implement shared domain/form
   logic first, then both compositions.
4. Add failing desktop/mobile Playwright watcher assertions to the existing
   Azure specs; Task 29 finishes fixture-backed integrated coverage.

## Verification

- `pnpm test -- --run hooks/domains/azure-devops/use-azure-devops-watches.test.ts` from `apps/web` — passed.
- `pnpm --filter @kandev/web typecheck` from `apps` — passed.
- Desktop and mobile Playwright watcher flows cover create, edit, enable/disable, Run now, reset, delete, and responsive drawer/card rendering.

## Files Likely Touched

- `apps/web/lib/types/azure-devops.ts`
- `apps/web/lib/api/domains/azure-devops-api.ts`
- `apps/web/lib/state/slices/azure-devops/types.ts`
- `apps/web/lib/state/slices/azure-devops/azure-devops-slice.ts`
- `apps/web/hooks/domains/azure-devops/use-azure-devops-watches.ts`
- `apps/web/components/azure-devops/azure-devops-settings.tsx`
- `apps/web/components/azure-devops/azure-devops-watch-settings.tsx`
- `apps/web/components/azure-devops/azure-devops-watch-dialog.tsx`
- `apps/web/components/azure-devops/azure-devops-watch-table.tsx`
- `apps/web/components/azure-devops/azure-devops-watch-cards.tsx`

## Dependencies

Tasks 24-26.

## Parallelism

Sequential. Watch settings state and the Azure store slice are common to all
controls.

## Inputs

- Spec: watcher settings contracts and scenarios.
- GitLab watch settings/form/table as the closest responsive two-kind watcher
  composition.
- Shared `WatcherSettingsCard`, `useWatcherEnabledDrafts`, and reset dialog.
- Required skills during implementation: `/tdd`, `/mobile-parity`, `/e2e`.

## Mobile Design Contract

- Desktop outcome: manage both watcher kinds in place.
- Phone entry point: the same settings sections render stacked watcher cards;
  visible Add Watch buttons open the corresponding full-height editor.
- The editor header/actions remain fixed, one body owns vertical scrolling,
  picker/menu rows are at least 44px, and bottom actions clear safe area.
- Domain hooks and form normalization are shared; only table-versus-card and
  dialog-versus-drawer presentation differ.

## Risks

- Watch mutation responses must invalidate only Azure watch/settings queries;
  avoid refreshing credentials or resetting unsaved connection fields.

## Output Contract

Report settings behavior, responsive composition, RED/GREEN commands, rendered
mobile inspection, files changed, risks, and update task/plan status.
