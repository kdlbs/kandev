---
status: draft
system: plugins
requirements:
  - REQ-PLUGINS-ACTION-UX-001
  - REQ-PLUGINS-ACTION-UX-002
  - REQ-PLUGINS-ACTION-UX-003
---

# Plugin action UX system design

## Purpose and boundaries

The plugin system owns the additive author contract. UI controls supply the
presentation primitives. No backend API, manifest version, persistence schema,
permission, or registration-lifecycle change is required.

See the [decision](../../../decisions/2026-09-25-additive-plugin-action-chrome.md),
[requirements](../requirements/plugin-action-ux.md), and
[plan](../../../plans/plugin-action-ux/plan.md).
The existing [UI sizing design](../../ui/system-design/control-sizing.md) and
[status design](../../ui/system-design/app-status-bar.md) remain authoritative.

## Requirement mapping

| Requirement | Sections |
| --- | --- |
| REQ-PLUGINS-ACTION-UX-001 | Public contract, Compatibility, Lifecycle |
| REQ-PLUGINS-ACTION-UX-002 | Shared rendering, Surface matrix, Mobile composition |
| REQ-PLUGINS-ACTION-UX-003 | Public contract, Interaction, Mobile composition |

## Current source

`PluginSlot` and `PluginSlotRegistrationView` render registered components behind
`PluginErrorBoundary`. Registration IDs control React identity. Status items
also retain `orderingId` through `app-status-items.tsx`.

`PLUGIN_UI` and `createPluginUIApi` in `lib/plugins/host-api.ts` expose host
components. `apps/packages/plugin-sdk/src/index.ts` has a runtime-free
`PluginUIShape` and mapped `PluginUIApi`. Its `HostNode` type supports host React
content without a React runtime dependency.

Topbar mobile wrappers currently apply descendant sizing to `data-slot=button`.
Sidebar workspace actions use broader button/link selectors. These are legacy
compatibility rules, not a new component styling mechanism.

## Public contract

Add `host.ui.Action`, `host.ui.ActionGroup`, and exported props to the frontend SDK.
Keep all existing exports and registry signatures. The export is present on
new hosts. Authors targeting old hosts test its presence before rendering it.

`PluginActionProps` contains these deliberate fields:

| Field | Contract |
| --- | --- |
| `label: string` | Required localized accessible name, independent of changing value |
| `icon?: HostNode` | Decorative content in a host-owned glyph box, including animated SVG |
| `text?: string` | Optional visible text/value, bounded and truncated by the surface |
| `badge?: string` | Optional short badge, bounded within the control |
| `tone?` | `neutral`, `success`, `warning`, or `danger`; affects semantic color only |
| `pressed?: boolean` | Controlled toggle state and `aria-pressed` |
| `disabled?: boolean` | Native disabled behavior |
| `busy?: boolean` | `aria-busy` and stable-size busy indicator; does not imply disabled |
| `tooltip?: string` | Informational tooltip; omitted uses the icon-only label, empty string disables it |
| `ref?` | Callback or object ref to the actual HTML button |
| Event props | Click, focus, blur, keyboard, pointer down/up/cancel/enter/move/leave, lost capture, mouse enter/leave |
| Trigger metadata | `id`, `aria-expanded`, `aria-controls`, `aria-haspopup`, `aria-describedby`, `data-testid` |

Use structural event/ref types compatible with React consumers. Keep DOM target
methods needed for `setPointerCapture` and `releasePointerCapture`. Do not add a
React import to the SDK. Prove React compatibility in `sdk-contract.test.ts`.

Do not accept `className`, `style`, `size`, `variant`, `asChild`, a location
selector, arbitrary spread props, or interactive children. Use an explicit
runtime allowlist so untyped JavaScript cannot accidentally replace geometry.
The outer button always uses `type=button`. A decorative icon cannot introduce
a second activation target. Styling is a cooperative API, not a security sandbox.
Custom content that cannot fit this contract remains a legacy component.

Tooltip/menu libraries can clone a trigger and provide supported handlers,
refs, ARIA metadata, and state attributes. Allow the finite Radix trigger
attributes required by the existing host Tooltip, Popover, and DropdownMenu
primitives, with typed tests. Do not forward arbitrary DOM styles from clones.
Interactive details continue to use existing Popover/Drawer/Dialog primitives.
This release does not introduce a second overlay state machine.

## Shared rendering

Introduce `components/actions/surface-action.tsx` and
`surface-action-styles.ts` as internal modules. Native callers select a
surface explicitly. The public Action adapter obtains it from host context.
Use the existing Button behavior and `controlSizingClassName` where applicable.
Do not change `apps/packages/ui/src/button.tsx` defaults.

Extract styling from adjacent native exemplars. Migrate ordinary controls in
the affected clusters to these shared styles. Leave split buttons, selectors,
submit controls, and other specialized controls under their existing contracts.
The public adapter filters props before invoking the internal renderer.
Use a distinct `data-slot=surface-action` marker so legacy button selectors do
not override new controls. Preserve existing markers on unmigrated buttons.

The host owns group gaps. Action adds no external margin. ActionGroup owns
spacing when one contribution renders several standard actions. It accepts
`children: HostNode` and an optional localized `label` for a semantic group.
It accepts no styling or location overrides. The surface chooses inline layout,
gap, and mobile wrapping. Status-drawer groups stack full-width action rows.
Status-bar groups stay on one row, cap at 18rem, and allow their actions to
shrink so long values truncate instead of extending into neighboring items.
Authors return null for an absent contribution, including an empty action group.
ActionGroup returns null for null or empty direct children. Conditional child
components remain the author's responsibility, as in existing slots.
Existing parents still own gaps between registrations. No group wraps legacy
content automatically. Browser tests cover two actions per group and mixed
legacy/new registrations without double spacing.

## Surface context

Introduce `PluginActionSurfaceContext` with a nullable default. Internal values
identify `topbar`, `composer`, `sidebar`, `status-bar`, and `status-drawer`, plus
host presentation. The context is not a public plugin capability.

Add an optional internal `actionSurface` prop to `PluginSlot` and
`PluginSlotRegistrationView`. Mount a DOM-free provider around the existing
error-boundary content. Use explicit surface values from each owning mount.
Never infer a surface by inspecting opaque plugin `slotProps` or DOM ancestors.
Do not change keys, add layout wrappers, or replace component instances.

Supply context at both topbars, sidebar workspace actions, chat actions,
creation composer actions, and `AppStatusBarPluginContribution`.
`task-create-dialog-selectors.tsx` mounts both creation slots.
Quick Chat already shares `chat-input-actions` with task chat.

Outside a supported action context, Action throws a clear development error.
The existing plugin error boundary contains a misuse within a slot. Authors use
`host.ui.Button` for ordinary plugin pages and dialogs. Context continues through
React portals, but Action remains documented for the originating trigger only.

## Surface matrix

All dimensions assume a 16px root font and scale with it.

| Surface | Desktop presentation | Native exemplar | Touch presentation |
| --- | --- | --- | --- |
| Main/task topbar | 28px, outline, rounded-md, 16px icon; text uses native horizontal padding | `kanban-header.tsx` controls and task topbar debug control | 44px minimum in current Plugins section or coarse-pointer cluster |
| Composer | 28px ghost, 16px icon, gap-0.5 parent | AttachFilesButton in `chat-input-toolbar-primitives.tsx` | 44px target beside active composer |
| Sidebar workspace | Compact 24px ghost action, 14px glyph, gap-1 parent | Workspace action row in `app-sidebar-new-task-item.tsx` | 44px in Plugins section |
| Status bar | 24px bar-fit inline action, transparent base, compact padding and native focus style | `lsp-status-item.tsx` | Existing compact tablet bar exception |
| Status drawer | Native full-width row with leading icon and text | `app-status-drawer.tsx` | At least 44px row height |

Compare controls with the same role, not a status chip against a submit button.
Ordinary toolbar icon boxes are 16px. Compact sidebar glyphs retain 14px.
Status glyphs match the native status role. Custom artwork scales within its box.
The glyph can animate without changing the control bounds. Metric strips that
need multiple independent controls remain custom until separately migrated.

## Interaction

The plugin retains hooks, controlled state, event callbacks, refs, and overlays.
The Action component supplies no polling, data access, automatic async retry,
permission handling, or implicit disabling while a promise runs.
A recording control can display busy/pressed state while its stop action remains enabled.
Native disabled behavior suppresses activation. Event forwarding must not
swallow pointer capture or synthesize duplicate click activation.

Informational tooltip copy defaults to the localized label for icon-only actions.
An explicit empty tooltip disables that default for externally composed triggers.
Tooltips are not essential disclosures. Plugins retain explicit click/tap access
to details through their chosen native overlay. Host-owned copy uses existing
localization rules, including all required locale catalogs.

## Mobile composition

Reuse `MobilePluginNavSection`, `AppStatusDrawer`, and the mobile composer toolbar.
Topbar and workspace actions stay in the labelled Plugins group. Existing
rendered-task ownership suppresses the same plugin's workspace toolbar only
when task contributions actually render. The context provider adds no DOM,
so null-rendering contributions remain absent to the presence observer.

Keep one navigation/drawer scroll owner, safe-area clearance, focus return,
and the existing task/workspace state. Long values truncate inside their
control; their full label remains accessible. Wrapping groups use available
width. Do not persist a mobile fallback over desktop preferences.

Phone and coarse-pointer ordinary controls use adaptive shared sizes. Test
767px and 768px plus a wide coarse-pointer viewport. The non-phone status bar
is the existing 24px specialized exception. Do not silently reroute all tablet
status content or increase bar height in this plugin API change.

## Compatibility and lifecycle

| Plugin | Host | Result |
| --- | --- | --- |
| Legacy | New | Same slots, props, styles, order, and behavior |
| Adopting with fallback | Old | Legacy control selected once |
| Adopting | New | Standard Action, without double registration |
| Adopting without fallback | Old | Author declares the first supporting `min_kandev_version` |

Do not invent a release number during implementation. Record the actual first
supporting release when packaging an adopting plugin. No manifest API-version
bump follows merely from adding this UI export.

Preserve registration identity, status ordering IDs, owner cleanup, and existing
error boundaries. Do not introduce a parallel registry or store. Disable/unload
removes the registered component normally. Existing composer capabilities remain
instance-scoped and revocable. No new context data enters the plugin boundary.

## Verification and documentation

The [plan](../../../plans/plugin-action-ux/plan.md) maps criteria to focused SDK,
component, and browser tests. Use packaged test fixtures for mixed contribution
forms. Cover unchanged legacy bundles without rewriting them to use Action.
A stub old host proves fallback selection; it does not prove every historical
host release. Cross-release plugin certification remains plugin-release work.

Update the canonical authoring guide, PLUGIN-API contract, SDK, and scoped web
AGENTS guidance together with the implementation. Include command, toggle,
labelled status, animated icon, controlled overlay trigger, and fallback examples.
Keep this design package uncommitted until the user requests delivery.
