---
status: current
system: ui
requirements:
  - REQ-UI-INTEGRATION-PAGE-SHORTCUTS-001
created: 2026-10-06
owners:
  - kandev
---

# Integration Page Shortcuts System Design

## Purpose and boundaries

UI owns the host's personal page-navigation shortcuts. This extends the existing
shortcut override map and app navigation catalog, without a backend schema,
plugin SDK, credential, or release-toggle change. Plugin-declared action
shortcuts remain governed by [Plugin Shortcut Settings](../../plugins/system-design/plugin-shortcut-settings.md).
Manual saving follows [Settings Manual Save](../requirements/settings-manual-save.md);
portable storage follows [ADR 0041](../../../decisions/0041-backend-owned-portable-user-settings.md).

## Requirement mapping

| Criteria under REQ-UI-INTEGRATION-PAGE-SHORTCUTS-001 | Design sections |
| --- | --- |
| .1, .2, .4 | Destination catalog and identity |
| .3 | Draft and persistence |
| .5, .6 | Dispatch and conflicts |
| .7, .8 | Settings and responsive composition |

## Destination catalog and identity

Add a pure `lib/keyboard/integration-shortcuts.ts` module. Build the built-in
list from `WORKSPACE_INTEGRATIONS` in the settings-discovery catalog, resolving
dashboard destinations from `APP_DESTINATIONS` in `lib/navigation/core-destinations.ts`.
Sentry has no dashboard or top-level navigation destination; use its existing
`/settings/integrations/sentry` connection route as an explicit fallback. A
completeness test must fail when a new integration lacks a destination decision.

Do not change sidebar/menu availability filtering or add Sentry to those menus
as part of this work. The settings list is connection-independent, and direct
page navigation leaves the route's existing connection and authorization checks
in charge. No new provider health polling is needed for this catalog.

| Built-in | Destination | Override identity |
| --- | --- | --- |
| Azure DevOps | `/azure-devops` | `integration:azure-devops` |
| GitHub | `/github` | `integration:github` |
| GitLab | `/gitlab` | `integration:gitlab` |
| Jira | `/jira` | `integration:jira` |
| Linear | `/linear` | `integration:linear` |
| Sentry | `/settings/integrations/sentry` | `integration:sentry` |

Merge registered plugin navigation items through `pluginDestinations`, filtered
to `section === "integrations"`. Use the existing owner-qualified, percent-encoded
`pluginDestinationId` to form `integration:plugin:<encoded-plugin-id>:<encoded-nav-id>`.
This identity cannot overlap the plugin action namespace
`plugin:{pluginId}:{keybindingId}`. Multiple links from one plugin remain distinct.
Preserve built-in catalog order followed by registry order for both rendering
and conflict tie-breaking. Resolve hrefs with the current `NavContext` via
`resolveHref`; never cache a workspace-dependent destination from recording time.

Extend `ShortcutEntry` in `lib/keyboard/plugin-shortcuts.ts` with an explicit
host-owned integration-navigation source. Keep `ConfigurableShortcutId` closed.
All integration entries default to `UNBOUND_SHORTCUT`; use `resolveShortcutEntry`
for override lookup. Registered navigation, not a plugin's action declarations,
determines dynamic page-opening entries. Registry subscriptions remove entries
and handlers together on plugin disable/uninstall.

## Draft and persistence

`StoredShortcutOverrides` already supports string keys. Save navigation bindings
through the existing `userSettings.keyboardShortcuts` / HTTP `keyboard_shortcuts`
map and its existing settings contributor. No migration or browser storage is
introduced. Reset removes the navigation key: the backend's
`validateKeyboardShortcuts` rejects an empty key, so do not persist an explicit
empty-key sentinel for these default-unbound entries.

`KeyboardShortcutsSettings` in `components/settings/general-settings.tsx` owns
the route's saved baseline and local draft. The new group edits that same map.
Resolve runtime bindings only from acknowledged store settings, not drafts.
Preserve all existing core, plugin-action, and orphaned navigation keys on every
edit and save. A missing plugin registration never deletes its override.

Share the existing plugin editor's draft/save hook as
`components/settings/use-shortcut-draft.ts` (`useShortcutDraft`). Both editors
use its per-key `rebaseShortcutOverrides` behavior. Rebase local additions,
changes, and deletions onto the latest store map at save time. Incorporate initial
and higher-revision settings baselines without erasing local edits. Apply save
responses through `mapUserSettingsResponse` and `compareUserSettingsRevisions`,
preserving edits made during the pending request and rejecting older responses.
These are the already shipped plugin-editor persistence semantics, applied to
the new navigation editor within the keyboard settings contributor. Treat an
omitted `keyboard_shortcuts` field in a successful full-map PATCH response as the
submitted map, including an empty map; otherwise resetting the final binding can
leave the old override active. The revision guard still rejects stale responses.
Chat-submit-key uses its own contributor under the same floating Save surface;
keep its existing semantics intact. A rejected save leaves the draft dirty
under the existing coordinator and does not activate the new navigation binding.

## Dispatch and conflicts

Add one app-root `useIntegrationShortcuts` hook, mounted in `GlobalCommands`
after `useAppShortcuts` and before `usePluginShortcuts`. Its capture listener
reads saved overrides fresh for each event and subscribes to registry changes.
Use the existing router adapter's `push` and current navigation context.

Before matching, yield for `defaultPrevented`, editable targets, repeats, or a
recording shortcut control. Expose the shared `ShortcutRecorder` recording state
through an explicit DOM marker and check it in the navigation dispatcher. The
integration handler must not navigate away while any recorder is capturing a
new binding, including when re-recording an already saved navigation chord.
Only prevent default and stop propagation after selecting an eligible target.
Return after one navigation.

To preserve existing Kandev shortcuts, yield when the combination matches any
effective configurable core entry or the existing reserved `SAVE` /
`FIND_IN_PANEL` combinations. Share the existing reserved-combination logic
with `usePluginShortcuts` rather than create divergent lists. Plugin page
navigation is a host action: an eligible navigation match runs before plugin
action dispatch, whose existing `defaultPrevented` guard prevents a second action.
If two navigation bindings collide, catalog order chooses the first match.
Do not rewrite unrelated existing core/plugin dispatch behavior.

Include navigation entries in `useShortcutConflictLabels`, the core settings
card, and plugin-detail conflict calculation. Use one resolved comparison union:
configurable core actions, navigation entries, and installed plugin action
declarations. Render only host navigation in the new group and only plugin
actions on plugin detail pages. Warnings remain advisory like existing warnings.
The union must contain each identity once.

## Settings and responsive composition

Add an Integrations subgroup after the existing core rows on
`/settings/preferences/keyboard-shortcuts`. Reuse `ShortcutRecorder`, baseline
dirty markers, warnings, recording feedback, and reset behavior. Use localized
`Open {{integration}}` labels; qualify plugin labels using installed plugin
display names and navigation labels. Keep plugin-supplied data as data.
New host copy must ship in all seven real languages, generating Traditional
Chinese through the existing conversion command and regenerating pseudo copy.

Desktop uses the existing settings page and compact label/control rows. Phone
entry is the Settings index > Keyboard Shortcuts route, following
`mobile-general-settings.spec.ts`'s direct settings-index navigation pattern.
Use the existing `ShortcutRecorder` touch-sized stacked composition selected by
`useResponsiveBreakpoint`; share draft, catalog, and handlers with desktop.

The settings content container remains the sole vertical scroller. Label comes
first, followed by wrapping recorder/reset controls. Use 28px ordinary desktop
controls and at least 44px phone/coarse-pointer hit areas through existing sizing
helpers. Keep floating Save within its existing safe-area-aware surface with
clearance for the final row. Inline content fits this persistent preference
better than a drawer; the phone software keyboard is not a new chord picker.
An attached keyboard can record bindings; touch-only navigation remains through
the existing integration links.

## Failure, compatibility, and validation

Unavailable plugin registrations produce no runtime destination; retained keys
become usable again if the same registration returns. Malformed stored bindings
must be ignored by the new dispatcher rather than crash navigation. Built-in
connection failures remain the existing destination's visible behavior. No
credentials are read or changed by shortcut dispatch. No new metrics are needed.

Targeted unit tests prove destination completeness, identity isolation, default
unbound behavior, per-key save rebasing, conflicts, dispatch ordering, recorder
suppression, editable/repeat handling, dynamic removal, and current workspace
resolution. Desktop E2E proves all six destinations, record/save/reload/reset,
retryable save failure, and plugin destination dispatch using the packaged
fixture. Phone E2E proves index entry, contained touch controls, recording with
an attached keyboard, acknowledged save/reload, and ordinary touch navigation.
Exact commands and test names belong to the linked work order.

## Implementation Plans

- [Integration page shortcuts](../../../plans/integration-page-shortcuts/plan.md)
