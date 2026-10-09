---
id: "01-confirmation-and-recovery-actions"
title: "Confirmation and recovery action sizing"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-CONTROL-SIZING-001
acceptance_criteria:
  - AC-UI-CONTROL-SIZING-001.1
  - AC-UI-CONTROL-SIZING-001.3
  - AC-UI-CONTROL-SIZING-001.4
  - AC-UI-CONTROL-SIZING-001.6
  - AC-UI-CONTROL-SIZING-001.7
  - AC-UI-CONTROL-SIZING-001.8
system_design:
  - ../../specs/ui/system-design/control-sizing.md
---

# Task 01: Confirmation and recovery action sizing

## Summary

Restore the existing desktop control sizes for confirmation, dialog-footer, and recovery actions that still force 32–44px heights. Keep the same actions at least 44px high on phones and coarse pointers, and correct the prior sweep inventory.

## In scope

- Fix the shared ActionConfirmPopover Cancel and Confirm buttons and all of its callers.
- Correct confirmed ordinary action candidates in automation deletion, file-upload conflict, remote-contribution resolution, plugin executor profile creation and editing, Canvas rename, Add workspace sources, task dependency retries, Office export retry, and automation run stop.
- Align the Canvas rename input with its adjacent standard actions.
- Add desktop geometry assertions to existing terminal, Canvas rename, Add workspace sources, and automation flows. Keep existing phone flows covered.
- Update docs/plans/control-sizing/sweep-inventory.md with corrected source-backed dispositions, then repeat the search for local height overrides and review any new result.

## Out of scope

- Feature state, callbacks, permissions, persistence, or confirmation semantics.
- Menu and navigation rows, selection surfaces, drawer-only actions, multiline controls, and editor or plugin-owned geometry.
- The integration review status Open review action in `apps/web/components/integrations/integration-change-request-status-content.tsx` is mobile Drawer-only and retains its correct 44px touch size.
- Changes to the standard/compact size meanings in @kandev/ui/button.

## Acceptance

- Ordinary fine-pointer desktop actions measure 28px; deliberate compact actions measure 24px. Neighboring confirmation actions have equal heights.
- Standard actions in phone or coarse-pointer contexts retain a minimum 44px active height. Disabled and busy states keep the same size role.
- Every candidate changed or reviewed in this work order has a disposition in the inventory, and no identified ordinary action keeps an unconditional oversized desktop height.

## ASCII UI preview

See [UI-01 and UI-02 in the plan](plan.md#ascii-ui-preview). UI-01 applies to desktop terminal confirmation and other shared confirmation callers; UI-02 records the existing phone interaction. Criteria: AC-UI-CONTROL-SIZING-001.1, .3, .4, .6, and .8.

## Verification

Run from the repository root. The E2E runner builds the desktop bundle before the desktop project; the mobile command reuses that build.

~~~bash
pnpm --dir apps/web exec vitest run components/confirmation/action-confirm-popover.test.tsx components/task/close-terminal-confirm-popover.test.tsx components/task/add-workspace-sources/add-workspace-sources-dialog.test.tsx components/settings/canvas-rename-dialog.test.tsx components/automations/automation-delete-confirm-dialog.test.tsx components/task/file-upload-conflict-dialog.test.tsx components/task/remote-contribution-resolution-dialog.test.tsx components/task-edit-dialog-dependencies.test.tsx
pnpm --dir apps/web e2e:run --project chromium tests/terminal/terminal-dockview-ui.spec.ts tests/canvas/canvas-host-rename.spec.ts tests/task/add-workspace-sources.spec.ts tests/automations-settings.spec.ts tests/settings/plugin-executor-profiles.spec.ts
pnpm --dir apps/web e2e:run --no-build --project chromium --grep 'stops the selected running run|configuration export retry stays compact|prompts on a name collision|edits a started task.s predecessor set' tests/automations-run-detail.spec.ts tests/office/config-export.spec.ts tests/task/workspace-file-transfer.spec.ts tests/task/create-task-dependency-selector.spec.ts
pnpm --dir apps/web e2e:run --no-build --project mobile-chrome tests/terminal/mobile-terminal-close.spec.ts tests/canvas/mobile-canvas-host-rename.spec.ts tests/task/mobile-add-workspace-sources.spec.ts tests/settings/mobile-automations-settings.spec.ts tests/settings/mobile-plugin-executor-profiles.spec.ts
git diff --check
~~~

Before completion, run ESLint on the exact changed TypeScript files and record the expanded list in Results. Do not use class-string assertions as a substitute for browser geometry.

## Files likely touched

- apps/web/components/confirmation/action-confirm-popover.tsx
- apps/web/components/confirmation/action-confirm-popover.test.tsx
- apps/web/components/task/close-terminal-confirm-popover.test.tsx
- apps/web/components/automations/automation-delete-confirm-dialog.tsx
- apps/web/components/task/file-upload-conflict-dialog.tsx
- apps/web/components/task/remote-contribution-resolution-dialog.tsx
- apps/web/components/settings/plugin-executor-profile-dialog.tsx
- apps/web/components/settings/plugin-executor-profile-page.tsx
- apps/web/components/settings/canvas-rename-dialog.tsx
- apps/web/components/task/add-workspace-sources/add-workspace-sources-dialog.tsx
- apps/web/components/task-edit-dialog-dependencies.tsx
- apps/web/components/task-edit-dialog-dependencies.test.tsx
- apps/web/app/office/workspace/settings/export/export-preview.tsx
- apps/web/components/runs/automation-detail-page.tsx
- apps/web/e2e/tests/terminal/terminal-dockview-ui.spec.ts
- apps/web/e2e/tests/canvas/canvas-host-rename.spec.ts
- apps/web/e2e/tests/task/add-workspace-sources.spec.ts
- apps/web/e2e/tests/automations-settings.spec.ts
- apps/web/e2e/tests/automations-run-detail.spec.ts
- apps/web/e2e/tests/office/config-export.spec.ts
- apps/web/e2e/tests/task/workspace-file-transfer.spec.ts
- apps/web/e2e/tests/task/create-task-dependency-selector.spec.ts
- docs/plans/control-sizing/sweep-inventory.md

## Dependencies

None. Use the active control-sizing requirement and current system design.

## Risks

- Shared confirmation actions have many callers; preserve each caller's labels, handlers, focus, and disabled behavior.
- Mobile drawers and coarse-pointer tablets need touch sizing even when the action is shared with desktop.
- Inventory hits include intentional content-sized and touch-only surfaces; retain those with a source-backed reason.

## Parallelism

sequential

## Inputs

- [Control sizing requirement](../../specs/ui/requirements/control-sizing.md)
- [Control sizing design](../../specs/ui/system-design/control-sizing.md)
- [Control sizing sweep inventory](../control-sizing/sweep-inventory.md)
- [Plan](plan.md)

## Results

Completed on 2026-09-27. Red/green browser checks confirmed the terminal confirmation, Canvas rename, Add workspace sources, automation deletion, and plugin executor profile dimensions. The initial desktop run passed 33 tests; the final targeted Chromium run passed 12 tests, including the coarse-pointer tablet confirmation, profile edit Delete action, and source-row removal control. The selected mobile flows passed 5 tests, followed by 1 passing mobile profile-page check. Focused Vitest passed 8 files / 46 tests. Review follow-up geometry checks passed 4 targeted E2E tests for automation Stop, the 409 export Retry branch, upload-conflict choices, and dependency info. The E2E-managed production bundle build passed, and `git diff --check` passed.

ESLint passed on the exact 24 changed TypeScript files:

~~~text
apps/web/app/office/workspace/settings/export/export-preview.tsx
apps/web/components/automations/automation-delete-confirm-dialog.tsx
apps/web/components/confirmation/action-confirm-popover.test.tsx
apps/web/components/confirmation/action-confirm-popover.tsx
apps/web/components/runs/automation-detail-page.tsx
apps/web/components/settings/canvas-rename-dialog.tsx
apps/web/components/settings/plugin-executor-profile-dialog.tsx
apps/web/components/settings/plugin-executor-profile-page.tsx
apps/web/components/task-edit-dialog-dependencies.test.tsx
apps/web/components/task-edit-dialog-dependencies.tsx
apps/web/components/task/add-workspace-sources/add-workspace-sources-dialog.tsx
apps/web/components/task/close-terminal-confirm-popover.test.tsx
apps/web/components/task/file-upload-conflict-dialog.tsx
apps/web/components/task/remote-contribution-resolution-dialog.tsx
apps/web/e2e/tests/automations-run-detail.spec.ts
apps/web/e2e/tests/automations-settings.spec.ts
apps/web/e2e/tests/canvas/canvas-host-rename.spec.ts
apps/web/e2e/tests/office/config-export.spec.ts
apps/web/e2e/tests/settings/mobile-automations-settings.spec.ts
apps/web/e2e/tests/settings/plugin-executor-profiles.spec.ts
apps/web/e2e/tests/task/add-workspace-sources.spec.ts
apps/web/e2e/tests/task/create-task-dependency-selector.spec.ts
apps/web/e2e/tests/task/workspace-file-transfer.spec.ts
apps/web/e2e/tests/terminal/terminal-dockview-ui.spec.ts
~~~
