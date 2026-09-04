---
status: current
system: plugins
requirements:
  - REQ-PLUGINS-PLUGIN-SHORTCUT-SETTINGS-001
created: 2026-09-04
owners:
  - kandev
---
# Plugin Shortcut Settings System Design

## Purpose and boundaries

The plugin system owns plugin keybinding declarations and their namespaced
override identities, so it also owns where users configure those bindings.
This design moves their presentation from the core shortcut list to each
installed plugin's existing detail route without changing the manifest,
frontend registry, dispatcher, HTTP settings contract, or backend storage.

The UI system continues to own the shared settings-save coordinator described
by [Settings Manual Save](../../ui/requirements/settings-manual-save.md).
Portable persistence continues to follow
[ADR 0041](../../../decisions/0041-backend-owned-portable-user-settings.md).

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-PLUGINS-PLUGIN-SHORTCUT-SETTINGS-001` | [Settings surfaces](#settings-surfaces), [Draft and save flow](#draft-and-save-flow), [Conflict and dispatch behavior](#conflict-and-dispatch-behavior), [Responsive behavior](#responsive-behavior), [Failure and compatibility](#failure-and-compatibility) |

## Settings surfaces

`KeyboardShortcutsSettings` in
`apps/web/components/settings/general-settings.tsx` continues to load all
installed plugin declarations for conflict calculation, but
`KeyboardShortcutsCard` renders only the static entries from
`CONFIGURABLE_SHORTCUTS`. It no longer renders a dynamic Plugin Shortcuts
group.

`PluginDetail` in
`apps/web/components/settings/plugins/plugin-detail.tsx` adds a host-owned
plugin-shortcut card between the plugin's operator configuration area and its
manifest card. The card renders only when the selected `PluginRecord` has at
least one parseable `ui.keybindings` declaration. It remains outside the
`canManage` gate so a member can manage a personal shortcut while the existing
plugin-authored settings slot, schema-driven configuration, lifecycle actions,
and danger zone remain administrator-only.

An active bundle may register a replacement component at the exact
`/settings/plugins/{id}` route. `SettingsRoutes` keeps that plugin-owned
content but wraps it in `PluginRootSettingsRoute`, which appends the same
host-owned shortcut card. Nested plugin settings routes remain plugin-owned
without the card. This preserves the public route extension while ensuring a
plugin cannot displace the user's only shortcut configuration surface.

A focused component under
`apps/web/components/settings/plugins/` owns the plugin shortcut draft and
card. It reuses the existing `ShortcutRecorder` behavior and shared conflict
label derivation from the keyboard-shortcut components instead of copying the
recording, clear, reset, dirty, or warning logic. Within the plugin detail
context, a row displays the manifest keybinding description without repeating
the plugin name; conflict labels stay qualified by plugin name so warnings are
unambiguous.

The card's visible description explains that these shortcuts are personal and
run only while the plugin is active. A disabled or errored plugin therefore
keeps the editor available without implying that its handler is currently
registered.

## Data and contracts

`PluginRecord.ui.keybindings` remains the declaration source. The helpers in
`apps/web/lib/keyboard/plugin-shortcuts.ts` continue to parse each manifest
default and derive the stable override key
`plugin:{pluginId}:{keybindingId}`. `StoredShortcutOverrides` remains one map
containing both core and plugin entries under
`userSettings.keyboardShortcuts`.

No backend DTO or endpoint changes. The editor persists through the existing
user-settings update request's `keyboard_shortcuts` field. Plugin configuration
continues to use the plugin config endpoint and is a separate save contributor.

## Draft and save flow

1. The plugin detail route reads the selected plugin plus the complete installed
   plugin list through `usePlugins`, and reads the current portable shortcut
   overrides from the settings store.
2. The shortcut component builds all plugin entries for conflict comparison,
   filters the visible list by the selected plugin ID, and seeds saved and draft
   override snapshots from `userSettings.keyboardShortcuts`. Initial hydration
   and later higher-revision settings updates replace the saved baseline and
   rebase locally changed or deleted keys onto that complete incoming map.
3. A recorder mutation changes only the local draft. The component registers a
   contributor such as `plugin-shortcuts:{pluginId}` with
   `useSettingsSaveContributor`; discard restores the saved snapshot. A draft
   cannot become saveable until the initial portable-settings baseline has been
   incorporated.
4. Save rebases the local delta onto the latest settings-store map immediately
   before sending the complete override map through `updateUserSettings`. A
   response advances the baseline only when its revision is not older than the
   store, and edits made while the request is pending remain in the draft.
5. If an administrator also changed schema-driven plugin configuration, the
   route coordinator invokes both independent contributors. Existing partial
   success and retry behavior applies; a shortcut-only save never calls the
   plugin config endpoint and therefore never restarts the plugin.

## Conflict and dispatch behavior

Both shortcut surfaces resolve conflicts from the union of translated core
entries and every installed plugin entry using
`findShortcutConflicts`. Each surface filters only the rows it displays, not
the comparison set. This retains core-versus-plugin and plugin-versus-plugin
warnings after the rows move to separate pages.

`usePluginShortcuts` is unchanged. It reads the same override map on each
keydown, invokes only currently registered plugin handlers, skips disallowed
editable targets, and suppresses a plugin handler when its effective combo
matches a Kandev-owned shortcut. Disabled and errored plugins have no live
handler registration even though their saved declaration and override remain
editable.

## Responsive behavior

Desktop keeps the existing direct plugin detail route and adds the shortcut
card inline in that page. On phones, the entry path remains the shipped
Settings navigation sheet, Plugins list, and full-card tap into the direct
plugin detail route, following
`apps/web/e2e/tests/plugins/mobile-plugin-settings-row.spec.ts`. A drawer is
not introduced because the shortcut list is persistent detail content rather
than a temporary choice.

The detail page's settings scroll container remains the single vertical scroll
owner. Shortcut rows can stack their label and controls on narrow widths;
record, clear, and reset actions provide at least a 44 px active touch
dimension on phone/coarse-pointer presentation. The shared floating save
surface remains the primary persistence action and retains its existing
dynamic-viewport and safe-area behavior. Shared manifest data, drafts,
conflict calculation, and save callbacks are identical across viewports;
only row composition and touch geometry vary.

## Failure and compatibility

An unparseable manifest default remains a skipped declaration with the existing
developer warning; backend manifest validation should make this exceptional.
If no parseable bindings remain, no empty card is rendered. A failed
user-settings save leaves the contributor dirty and retryable under the shared
save coordinator.

Namespaced override IDs and the backend field do not change, so the relocation
requires no migration and does not reset existing user choices. Uninstalling a
plugin continues to make any orphaned override inert; reinstalling the same
plugin identity can resolve that key again under existing behavior.

## Verification

Component tests cover core-only rendering, selected-plugin rendering, empty
declarations, qualified conflict warnings, drafts, resets, route replacement,
and persistence boundaries. Deferred-response cases cover initial hydration,
higher-revision synchronization, complete-map rebasing, stale save responses,
and edits made during an in-flight save. The existing authenticated member
plugin-settings scenario proves that the shortcut editor remains available
while operator controls stay hidden and that the General shortcut page no
longer contains the plugin row. The existing mobile plugin-detail scenario
proves direct entry, touch geometry, single-axis containment, an acknowledged
Save, and reload while the fixture plugin is disabled. Existing plugin
dispatcher tests remain the regression evidence for dispatch precedence and
editor-target rules.

## Related decisions

- [ADR 0041: Backend-Owned Portable User Settings](../../../decisions/0041-backend-owned-portable-user-settings.md)
