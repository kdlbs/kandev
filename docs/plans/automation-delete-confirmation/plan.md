---
created: 2026-08-28
status: done
requirements:
  - REQ-OFFICE-AUTOMATIONS-SETTINGS-001
system_design:
  - ../../specs/office/system-design/automations-settings-01.md
  - ../../specs/office/system-design/automations-settings-02.md
legacy_specs: []
---

# Implementation Plan: Automation deletion confirmation

## Overview

Add one shared confirmation surface to both automation deletion entry points in
Settings. The list and editor keep their existing mutation and navigation
ownership, while the shared dialog prevents accidental deletion and gives the
same desktop, keyboard, and phone outcome.

## Scope

### In scope

- Confirmation before deleting an automation from the workspace list.
- Confirmation before deleting an automation from the automation editor.
- Localized dialog title and permanent-deletion warning in all supported locales.
- Desktop and mobile browser coverage for cancel and confirm behavior.

### Out of scope

- Changes to the automation delete API or backend lifecycle.
- Confirmation for deleting individual automation runs or triggers.
- Changes to the settings unsaved-changes save coordinator.

## Technical approach

- Add `apps/web/components/automations/automation-delete-confirm-dialog.tsx`,
  wrapping the existing `@kandev/ui/alert-dialog` primitives. The component
  receives the automation name and an async confirmation callback; it does not
  own API or routing state.
- Keep the list page responsible for the selected row and call its existing
  `useAutomations.remove` mutation only after confirmation.
- Keep the editor's existing `useRemoveAutomation` callback responsible for
  deletion and redirect, with `EditorFooter` becoming the dialog trigger.
- Add the dialog copy to the five complete automation catalogs and preserve
  the current `automations:delete` action label. Keep the pseudo catalog in
  sync with the English catalog.

### Mobile design contract

- Desktop outcome: the list trash action and editor Delete button open a
  blocking confirmation; Confirm/Delete performs the existing deletion result.
- Mobile entry point: the same visible trash/Delete control remains available;
  tapping it opens the shared alert dialog.
- Nearest shipped exemplars: `quick-chat-delete-dialog.tsx` for the existing
  short destructive alert-dialog anatomy and
  `settings/mobile-agent-profile-delete.spec.ts` for settings deletion focus
  and 44px touch-target checks.
- Hierarchy and surface: title, named automation warning, then Cancel and
  Delete in the existing centered `AlertDialog`; the content is short, so a
  bottom drawer would add no value to the blocking decision.
- Scroll and safe area: the intrinsic alert dialog is viewport-contained, has
  no internal scroll owner, and uses full-width touch-sized footer actions on
  phone-sized viewports.
- State: list/editor mutation, routing, and error handling remain shared with
  desktop; only the dialog presentation is responsive.
- Mobile proof: a `mobile-*.spec.ts` flow opens the editor on a Pixel 5,
  verifies cancel leaves the automation intact, then confirms deletion and
  verifies the list destination and backend-visible removal.

## Tests

- `AC-OFFICE-AUTOMATIONS-SETTINGS-001.9` and `.001.10`: extend
  `apps/web/e2e/tests/automations-settings.spec.ts` to cover the editor
  dialog, cancellation, and confirmation; add a list-row confirmation check.
- `AC-OFFICE-AUTOMATIONS-SETTINGS-001.11`: add
  `apps/web/e2e/tests/settings/mobile-automations-settings.spec.ts` using the
  `mobile-chrome` project, including viewport containment and touch hit-area
  assertions.
- Run the automation catalog and specification checks after the implementation.

## E2E tests

- Desktop Chromium: `tests/automations-settings.spec.ts`, deletion from the
  editor and list.
- Mobile Chrome: `tests/settings/mobile-automations-settings.spec.ts`,
  deletion from the editor with cancel and confirm paths.

## Work orders

- [x] [Task 01: Add automation deletion confirmation](task-01-add-confirmation.md)

## Verification results

- Passed `pnpm --filter @kandev/web run typecheck`.
- Passed `pnpm --filter @kandev/web run lint`.
- Passed `pnpm --filter @kandev/web run i18n:check` for all five complete
  catalogs and the pseudo catalog.
- Passed focused Chromium E2E: 2 tests.
- Passed focused mobile Chrome E2E: 1 test.
- Passed focused automation unit tests: 6 tests, including failed-deletion
  dialog retention and controlled dialog behavior.
- Passed specification tests and all-file specification lint.
- Passed `git diff --check`.

## Risks

- The editor participates in the shared unsaved-settings navigation guard;
  confirmation must not bypass that guard until the delete mutation itself
  succeeds.
- The list action is inside a clickable table row, so opening the dialog must
  continue to stop row navigation.
- The production E2E build must be refreshed after frontend changes.
