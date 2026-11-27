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
This design places their presentation on each installed plugin's existing
detail route. Conflict detection and plugin dispatch enforce existing
core-shortcut precedence without changing the manifest, frontend registration
contract, accepted chords, HTTP settings contract, or backend storage.

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
`findShortcutConflicts`, with the existing integration entries included by
`useShortcutConflictLabels`. Each surface filters only the rows it displays,
not the comparison set. Installed declarations remain warning inputs even
when their plugin is inactive.

### Accepted-event overlap

`matchesShortcut` in `apps/web/lib/keyboard/utils.ts` is the authority for
accepted events. Keys compare case-insensitively. Without `ctrlOrCmd`, Ctrl,
Meta, Alt, and Shift are exact Boolean requirements. With `ctrlOrCmd`, either
Ctrl or Meta or both is accepted on every platform; explicit `ctrl` and `cmd`
fields in the same modifiers object do not narrow that branch. Alt and Shift
remain exact. An unbound entry contributes no conflict.

Two effective bindings conflict if there exists at least one key/modifier event
accepted by both. `shortcut-conflicts.ts` evaluates this bounded intersection
using `matchesShortcut` over the 16 Boolean modifier combinations for the
shared key. Platform-dependent display formatting is not conflict evidence.
Remove the platform-specific `comboKey` comparison and its internal callers;
do not change `matchesShortcut`, platform detection, or CodeMirror conversion.

`findShortcutConflicts` returns one two-entry group per directly overlapping
pair, in input order. The group needs only its entries; a single canonical
combo identity cannot represent this relation. Ctrl-only and Meta-only
bindings are disjoint even when a third Mod binding overlaps each. Do not
merge connected groups into a transitive conflict group.

`buildConflictLabels` in
`apps/web/components/settings/keyboard-shortcuts-card.tsx` accumulates each
action's directly overlapping peers across groups rather than overwriting a
previous group. Deduplicate peers by action identity and displayed labels in
stable encounter order. Exclude the action itself. Both existing settings
surfaces use this lookup with their current translated warning copy; neither
surface changes its rows, controls, drafts, or save behavior.

### Core reservation during dispatch

`usePluginShortcuts` reads the current override map on each keydown. Resolve
the configurable core entries with that snapshot and retain the fixed defaults
from `NON_CONFIGURABLE_CORE_SHORTCUT_IDS` (`FIND_IN_PANEL`, `SAVE`, and
`COMMAND_PANEL_SHIFT`). Those fixed reservations do not become configurable
or start honoring an override under an otherwise unknown key.

Reserve an event if **at least one** bound effective core shortcut accepts
that actual event through `matchesShortcut`. A matching plugin handler is
skipped without invocation or `preventDefault` when the event is reserved,
leaving independent core listeners able to process it. Reservation does not
depend on whether a core listener is currently mounted. Do not suppress all
events for a plugin binding merely because it overlaps a core binding on some
other event: a Mod plugin binding may run on Meta+Q when a core binding reserves
only Ctrl+Q.

Preserve existing `defaultPrevented` admission, editable-target eligibility,
editor opt-in and non-Shift-modifier guards, handler registration order,
multiple matching plugin callbacks, and exception isolation. Disabled and
errored plugins have no live handler registration even though their saved
declarations and overrides remain editable.

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

The overlap and dispatch repair is shared state/data and action logic. A phone
with an attached keyboard uses the same accepted-event and reservation path.
It introduces no viewport-specific behavior, geometry, navigation, touch, or
scrolling changes; targeted hook and conflict-label consumer tests satisfy
mobile parity for this repair without new layout E2E coverage.

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
editor-target rules. The conflict-dispatch repair adds an independently authored
test mounting real `usePluginShortcuts` with real
`useKeyboardShortcut(SHORTCUTS.FIND_IN_PANEL)` and dispatching cancelable
keydown events under a restored Mac platform stub. Ctrl+F must invoke only
core Find; canonical Meta+F and noncolliding Ctrl+Q are controls. This is
simulated DOM dispatch evidence, not proof of physical OS key delivery.

Pure conflict tests cover accepted intersections and disjoint modifier/key
cases, including mixed `ctrlOrCmd` plus explicit modifier fields. Actual
`buildConflictLabels`/`useShortcutConflictLabels` consumers cover the Mod
bridge and duplicate-peer cases, with rendered warnings on both existing
settings cards. Partial-overlap dispatch tests ensure an event outside the
core reservation still reaches the plugin. See the
[repair plan](../../../plans/shortcut-conflict-dispatch/plan.md) for exact
checks; the completed relocation package remains historical evidence for its
unchanged persistence and responsive surfaces.

## Related decisions

- [ADR 0041: Backend-Owned Portable User Settings](../../../decisions/0041-backend-owned-portable-user-settings.md)
