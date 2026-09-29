---
created: 2026-09-27
status: complete
requirements:
  - REQ-UI-CONTROL-SIZING-001
system_design:
  - ../../specs/ui/system-design/control-sizing.md
legacy_specs: []
---

# Implementation Plan: Control sizing follow-up

## Overview

The terminal close confirmation exposes missed desktop sizing overrides from the completed control-sizing sweep. Its shared actions render at 44px because each Button has an unconditional minimum height. A focused re-audit found the same issue in other confirmation, dialog, and recovery actions. This follow-up restores the existing UI control-size contract and keeps phone and coarse-pointer targets at least 44px.

The UI system owns this behavior because the existing control-sizing requirement defines reusable dimensions across task, settings, integration, Office, and review surfaces. The active requirement and current design remain authoritative; this repair does not add a new product contract.

## Scope

### In scope

- Correct both actions in the shared ActionConfirmPopover, which serves terminal, task, settings, integration, and review callers.
- Re-audit and correct ordinary confirmation, dialog-footer, and recovery actions with unconditional desktop sizing overrides. Confirmed candidates include automation deletion, file-upload conflicts, remote-contribution resolution, plugin executor profile creation and editing, Canvas rename, Add workspace sources, task dependency retries, Office export retry, and automation run stop.
- The integration review status Open review action is rendered only in the mobile Drawer branch; retain its 44px touch size and exclude it from desktop candidates.
- Keep adjacent single-line controls aligned when they share a row, including the Canvas rename input.
- Record the corrected dispositions in the control-sizing sweep inventory and repeat the candidate search before closing the work order.

### Out of scope

- Task, provider, permission, API, persistence, or action behavior changes.
- Content-sized menu rows, selection cards, navigation rows, multiline controls, touch-only surfaces, and documented specialized chrome.
- A global CSS rule or changes to Button variant meanings.

## Root cause and evidence

apps/web/components/confirmation/action-confirm-popover.tsx applies min-h-11 to both its Cancel and Confirm buttons. The Button primitive's ordinary height is 28px, but the unqualified 44px minimum wins on fine-pointer desktop. The terminal close and Terminate flows both render this shared component, and the same component has callers across the application.

Other confirmed desktop outliers come from unqualified or desktop-sized overrides: Canvas rename and Add workspace sources use 44px minima in both their desktop and phone compositions; task dependency retry and Office export retry use 44px minima; several dialog actions resolve to 32–36px at desktop. The previous sweep inventory marked some of these as conforming, so this package corrects those dispositions.

Smallest reproduction: open a terminal tab's close confirmation or choose Terminate from its context menu. The popover shows Cancel and Close terminal actions with a 44px minimum. The existing terminal component and Playwright flows cover this surface; the work order adds exact rendered-height assertions.

## Technical approach

Use the existing @kandev/ui/control-sizing classes at ordinary call sites. Standard actions use 28px on fine-pointer desktop and at least 44px below 768px or on a coarse pointer. Explicit compact actions retain 24px desktop sizing with a touch-only 44px minimum. Do not change variants or use an unconditional desktop min-h-* to enlarge an action.

Keep handlers, disabled and busy states, focus behavior, labels, and confirmation order unchanged. Update the candidate inventory with a source-backed disposition for every touched control and every retained exception.

## ASCII UI preview

### UI-01: Terminal tab close confirmation on fine-pointer desktop

Entry: Task terminal tab close button or the tab's Terminate menu item. The anchored popover content and button order remain unchanged.

~~~text
Before                                  After
┌ Close terminal? ─────────────────┐    ┌ Close terminal? ─────────────────┐
│ This stops the Terminal shell... │    │ This stops the Terminal shell... │
│                                  │    │                                  │
│ [Cancel · 44px] [Close · 44px]   │    │ [Cancel · 28px] [Close · 28px]   │
└──────────────────────────────────┘    └──────────────────────────────────┘
~~~

### UI-02: Mobile terminal close confirmation

Entry: Phone terminal picker, after selecting a terminal's close action. The existing inline confirmation remains in the picker; its touch actions stay at least 44px.

~~~text
┌ Terminals drawer ─────────────────┐
│ Close terminal?                   │
│ Build shell will stop.             │
│                                    │
│ [Back · 44px] [Close · 44px]       │
└────────────────────────────────────┘
~~~

The required structure is unchanged confirmation content, equal action heights, 28px standard desktop actions, and 44px touch targets. Spacing in this sketch is illustrative. UI-01 maps to AC-UI-CONTROL-SIZING-001.1, .3, and .6; UI-02 maps to .4 and .8.

## Tests

- Component behavior: apps/web/components/confirmation/action-confirm-popover.test.tsx, apps/web/components/task/close-terminal-confirm-popover.test.tsx, apps/web/components/automations/automation-delete-confirm-dialog.test.tsx, apps/web/components/task/file-upload-conflict-dialog.test.tsx, apps/web/components/task/remote-contribution-resolution-dialog.test.tsx, apps/web/components/settings/canvas-rename-dialog.test.tsx, and apps/web/components/task/add-workspace-sources/add-workspace-sources-dialog.test.tsx.
- Add a focused component test for TaskEditDialogDependencies if no existing test covers its retry controls.
- Rendered geometry is authoritative. Assert desktop standard buttons at 28px, compact buttons at 24px, and phone/coarse-pointer actions at least 44px; compare adjacent confirmation actions.

## E2E tests

| Flow | Evidence | Criteria |
| --- | --- | --- |
| Terminal tab close confirmation | terminal/terminal-dockview-ui.spec.ts; measure Cancel and Close terminal in the existing confirmed-close flow | .1, .3, .6 |
| Canvas rename and Add workspace sources | canvas/canvas-host-rename.spec.ts and task/add-workspace-sources.spec.ts; measure dialog actions and adjacent field | .1, .3, .7 |
| Automation deletion and settings actions | automations-settings.spec.ts and settings/plugin-executor-profiles.spec.ts; measure desktop footer actions | .1, .3, .6 |
| Compact desktop actions | automations-run-detail.spec.ts, office/config-export.spec.ts, task/workspace-file-transfer.spec.ts, and task/create-task-dependency-selector.spec.ts; measure Stop, stale-export Retry, conflict choices, and dependency info at 24px | .2 |
| Phone terminal and form actions | terminal/mobile-terminal-close.spec.ts, canvas/mobile-canvas-host-rename.spec.ts, task/mobile-add-workspace-sources.spec.ts, and settings/mobile-automations-settings.spec.ts | .4, .8 |

Run the mobile project after the desktop build. Use the configured Pixel 5 project and a coarse-pointer tablet case where the existing terminal fixture supports it.

## Work orders

- [x] [Task 01: Confirmation and recovery action sizing](task-01-confirmation-and-recovery-actions.md)

## Verification results

Completed on 2026-09-27. The shared confirmation and audited recovery actions now use their existing standard or compact desktop sizes, with phone/coarse-pointer targets retained. Focused component tests passed (8 files, 46 tests). Desktop geometry E2E checks passed (33 tests in the initial selected run and 12 tests in the final targeted run, including the coarse-pointer tablet, source-row removal, and profile edit controls). The selected mobile flows passed 5 tests, and a final mobile profile-page check passed 1 test. Review follow-up geometry checks passed 4 targeted E2E tests for 24px automation Stop, stale-export Retry, upload-conflict choices, and dependency info controls. ESLint passed on all 24 changed TypeScript files, the E2E runner's production bundle build passed, and `git diff --check` passed. The integration review status Open review action remains a documented mobile Drawer-only 44px touch target.

## Risks

- Some unqualified height classes belong to menu rows, selection surfaces, or touch-only branches; classify source behavior before changing them.
- Text scaling and long localized labels may expose wrapping after controls return to standard height.
- Coarse-pointer tablets need the 44px target even when their viewport is wider than the phone breakpoint.

## Open questions

None.
