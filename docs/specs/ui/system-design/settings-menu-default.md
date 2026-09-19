---
status: current
system: ui
requirements:
  - REQ-UI-SETTINGS-MENU-DEFAULT-001
---

# Settings Menu Default System Design

## Purpose and boundaries

The UI system owns the Settings Menu Shape presentation preference because it
is a device-local navigation choice. The design changes the fallback mode only;
the Settings tree, Appearance save coordinator, and local storage contract
remain the existing boundaries.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-SETTINGS-MENU-DEFAULT-001` | [Components and responsibilities](#components-and-responsibilities), [Persistence and compatibility](#persistence-and-compatibility), [Responsive behavior](#responsive-behavior) |

## Components and responsibilities

- `apps/web/lib/settings/settings-menu-mode.ts` owns the supported mode list,
  default constant, local storage read validation, and write helper.
- `apps/web/lib/state/slices/ui/settings-menu-actions.ts` loads the validated
  mode into the UI store and keeps preview, commit, and restore behavior for
  Appearance settings.
- `apps/web/lib/state/slices/ui/ui-slice.ts` uses the default constant for its
  static state and replaces it with the device-local loaded state when the UI
  slice is created.
- `SettingsTree` consumes `settingsMenu.mode`; it continues to render the
  existing flat, accordion, or persistent composition without owning default
  selection logic.
- The Settings Menu Shape card owns the translated descriptions. It identifies
  the default in the Accordion tree description so the visible explanation
  matches the runtime fallback.

## Data and contracts

The supported modes remain `flat`, `accordion`, and `persistent`. The existing
`STORAGE_KEYS.SETTINGS_MENU_MODE` key remains the device-local storage key.

The default contract changes from `flat` to `accordion`:

- `DEFAULT_SETTINGS_MENU_MODE` is `"accordion"`.
- `readSettingsMenuMode()` returns a validated stored mode when one exists.
- `readSettingsMenuMode()` returns `DEFAULT_SETTINGS_MENU_MODE` when storage is
  absent, malformed, or contains an unsupported value.
- `writeSettingsMenuMode()` continues to persist only an explicit committed
  user choice.

No API, WebSocket payload, user-settings field, or database value changes.

## Control flow

1. The UI slice initializes its static state from the Accordion tree default.
2. Browser-side slice creation reads and validates the device-local mode.
3. A valid stored mode replaces the static fallback for the current device.
4. With no valid stored mode, `SettingsTree` receives Accordion tree and opens
   the route-owned branch according to its existing accordion behavior.
5. Choosing another mode in Appearance still previews it immediately and the
   existing shared Save action commits it to local storage.
6. Discarding an unsaved Appearance change still restores the last committed
   mode.

## Persistence and compatibility

This is a fallback-only change. Existing valid local storage values are read as
before, so a user who selected Flat list or Persistent tree does not experience
an unsolicited migration. Invalid values continue to be rejected, but now use
Accordion tree as the fallback. No storage key version or cleanup is needed.

## Responsive behavior

Desktop Settings takeover and the phone `/settings` index share the same store
mode and branch model. The default therefore produces Accordion tree in both
surfaces. The phone keeps its existing full-page Settings index, one navigation
scroll owner, and current touch interaction. No responsive markup or breakpoint
logic changes.

## Failure and recovery

The existing local storage helper remains the boundary for parse and access
failures. A malformed or unsupported value cannot select an unknown renderer;
the validated read returns Accordion tree and Settings remains usable. A later
explicit selection follows the existing preview, save, and discard flow.

## Test strategy

- Extend `apps/web/lib/settings/settings-menu-mode.test.ts` to assert the
  Accordion tree default and the same fallback for malformed and unsupported
  values, while retaining round-trip coverage for explicit modes.
- Update the Settings Menu Mode desktop E2E flow to prove Accordion tree is
  visible before any selection and that changing to Flat list remains a local
  preview until the existing save flow is used.
- Add or update a phone Settings E2E assertion that a fresh device uses the
  Accordion tree and can still navigate through the existing phone index.

## Related decisions

No architecture decision record applies. The change does not introduce a new
boundary, persistence format, or interaction pattern.
