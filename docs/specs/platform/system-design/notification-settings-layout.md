---
status: current
system: platform
created: 2026-11-03
requirements:
  - REQ-PLATFORM-NOTIFICATION-SETTINGS-LAYOUT-001
---

# Notification settings layout system design

## Purpose and boundaries

Platform owns the notification preference presentation and its existing state owners.
This design uses [settings composition](../../ui/system-design/settings-composition.md) and [manual Save](../../ui/system-design/settings-manual-save.md).
It preserves the [permission feedback contract](notification-permission-feedback.md), including conditional warnings.
The new layout replaces that design's heading arrangement after implementation; its permission policy remains authoritative.

## Requirement mapping

| Requirement | Criteria | Design sections |
| --- | --- | --- |
| REQ-PLATFORM-NOTIFICATION-SETTINGS-LAYOUT-001 | .1, .2 | Page composition |
| REQ-PLATFORM-NOTIFICATION-SETTINGS-LAYOUT-001 | .3, .5 | Sound state |
| REQ-PLATFORM-NOTIFICATION-SETTINGS-LAYOUT-001 | .4, .6 | Mobile and accessibility; Compatibility |

## Page composition

`NotificationsSettings` retains `SettingsPageTemplate` and its one page title.
Update `settings:notificationsDescription` to explain device alerts once.
Keep remote-provider descriptions within their existing sections.
Move `NotificationSoundSection` directly after `DesktopNotificationsSection`, before `WebPushSection`.

Replace the two standalone section headings and repeated descriptions with compact labeled rows.
Use existing settings typography, surface tokens, spacing, and controls.
A notification-local `notification-preference-row.tsx` can share presentation between these two consumers.
It must use `useSettingsTargetRegistration` and preserve existing target IDs and test IDs.
Use `SettingsFieldLabel` for the compact label and `UnsavedChangesBadge` for sound drafts.
Keep domain hooks, permission decisions, and save registration in their existing components.
Do not change global `SettingsGroup` geometry to achieve this local arrangement.

Desktop places each label left and its controls right within the same bordered row.
The sound controls appear in this order: switch, preset selector, preview button.
The permission row keeps its existing three actions and handlers.
Conditional warning content follows the permission row inside its section.
Render no warning element or reserved body space in normal permission states.

## Sound state

`NotificationSoundSection` retains `saved`, `prefs`, and `useSettingsSaveContributor`.
Mount the preset selector and preview button for both switch values.
Set their semantic disabled properties from `!prefs.enabled`.
Use the current preset value while disabled; never reset it on a toggle.
Retain existing playback on preset selection and preview activation while enabled.
The new composition adds no playback when toggling or rendering a disabled control.
Dirty field attributes, contributor identity, route Save, and Reset retain their current meanings.
No API, local-storage format, permission authority, or migration changes are required.

## Mobile and accessibility

The entry point remains Settings > Preferences > Notifications, including its existing route alias.
Use direct navigation because this page is a primary settings destination.
The nearest shipped exemplar is the settings shell with `SettingsRow` labels and controls.
The curated phone precedent is the focused page composition in `components/task/task-layout.tsx`.
Its contribution is one scroll owner and intentional phone composition, without an additional overlay.

At widths less than 768px, the permission label precedes its actions.
The sound row places its label and switch together, then a full-width selector beside a preview button.
Use a notification-local responsive layout with shared domain state, rather than duplicate mounted state owners.
The selector retains the shared `Select` interaction and its contained option list.
Provide at least 44px active targets on phones and coarse pointers, including the switch label.
Retain 28px ordinary controls on fine-pointer desktop layouts.
Allow labels to wrap, grow rows with content, and keep control bounds inside their surface.

The existing `settings-scroll-container` remains the single vertical scroll owner.
Keep shell safe-area handling and its fixed Save surface.
Add no viewport-height region, nested scroller, or new fixed action.
Retain localized accessible names, focus indicators, switch label association, and disabled keyboard behavior.
Refresh and test remain visible touch actions.

## Compatibility and verification

Reuse the current permission warning selector and native/browser denial distinction.
Update tests that locate Desktop Notifications as a heading to locate its labeled preference row.
Update normal-state helper assertions to prove absence of warning space without requiring obsolete group-body markup.
Retain denied-to-granted refresh coverage and native/error component coverage.

Use `t()` at render time for copy and accessible names.
Update English, Portuguese, Simplified Chinese, Japanese, and Korean catalogs.
Generate both Traditional Chinese catalogs with `pnpm run i18n:zh-hant`.
Retain discovery IDs and aliases for both settings.

Component tests cover disabled controls, preset retention, preview behavior, drafts, Save, and Reset.
Desktop and phone browser checks cover row geometry, breakpoint behavior, touch targets, and warning transitions.
Capture both sound states using `prCapture` and compare them with the plan previews.
No additional metrics or logs are required for this local layout change.

Public notification instructions retain their meaning; no public page needs an edit for the design package.
The [plan](../../../plans/notification-settings-layout/plan.md) owns delivery and verification evidence.
