---
status: current
system: ui
requirements:
  - REQ-UI-PORT-FORWARDING-DISCOVERY-001
---

# Task-scoped port-forwarding discovery System Design

## Boundary and source evidence

UI owns the [discovery contract](../requirements/port-forwarding-discovery.md).
`SessionTaskSwitcherSheet` previously exposed the preference action in both
its task picker and the inline Tasks body embedded by `AppNavSheet`. Phone
presentations now omit it. `SessionMobileBottomNav` exposes Panels for task
port management as well as canvases/plugins; `PluginPanelPicker` owns that picker.

The change owns navigation and responsive presentation, not runtime or task
metadata storage. No new Dockview component, plugin registration, mobile panel
ID, schema, endpoint, or authorization is introduced.

## Requirement mapping

| Discovery criteria | Design section |
| --- | --- |
| .1-.3, .9, .14 | Discovery and compatibility |
| .4-.10 | Open state and shortcut preference |
| .10-.12 | Responsive surface and focus |
| .3, .8, .13 | Failure and context changes |

## Discovery and compatibility

Use `useResponsiveBreakpoint().isMobile` below 768px independently of pointer
mode. Port visibility context for a task is another reason for
`SessionMobileBottomNav` to show Panels, even without plugins/canvases and while
runtime access is unavailable. In `PluginPanelPicker`, put a Task tools group
before existing options, containing a network icon, Port forwarding command,
and disclosure chevron. Retain current canvas/plugin selection and filtering.
The command has no checked state and never calls `onSelect` or persists a panel.

Use provider eligibility and pending state to disable the command. Supply
visible localized supporting text for missing/unready/archived session or
saving state with an accessible description. Suppress `PortForwardingTaskAction`
only in phone presentations, including the retained inline navigation controller
and task-title picker. Keep tablet behavior and its context bridge.
Desktop `DockviewAddPanelItems` remains checkable.

Preserve the dock's existing composition at normal widths. At 320px with all
Review/Status/Panels entries, keep controls nonshrinking and at least 44px. If
necessary, contain horizontal overflow in the dock and scroll focused controls
into view; never overflow the document or hide required actions.

## Open state and shortcut preference

`PortForwardingVisibilityProvider` remains the single owner of `enabled`,
`isUpdating`, and `dialogOpen`. The phone command checks eligibility/pending
state, closes Panels, and opens `dialogOpen` without invoking a preference write.
Opening with a false preference must not implicitly enable it.

`TaskPageInner` hosts `PortForwardingManager` above responsive headers.
`PortForwardButton` renders only the eligible, enabled header trigger. The
manager retains one session-scoped controller and mounts its responsive surface
while eligible and `enabled || dialogOpen`. Preference-off/unopened tasks
remain lazy: no runtime reads occur before opening. Hiding the shortcut while
management is open cannot unmount the controller or reset manual ports.

Place a phone-only Show in task header switch below manual addition. Its
supporting text explains that the shortcut preference is shared across clients
for this task. Reuse `togglePortForwarding` with an explicit optional
`preserveDialogOpen` behavior; default callers keep existing wider toggle/close
semantics. The phone switch changes preference only, not port state.

`usePortForwardManagement` owns detected/manual state, draft, refresh, tunnel
map, hydration cancellation, and the mutated-port merge. `PortForwardContent`
uses `PortListSection` and `useTunnelActions` under thin Drawer/Dialog wrappers.
The controller resets on session identity changes and guards asynchronous state
updates with the initiating scope; `useTunnelActions` also resets pending state
and ignores late results/toasts after a scope change. Runtime calls and business
logic remain shared. Resize preserves the controller and manual draft.

## Responsive surface and focus

Scene: a developer checks or stops a development service from a phone during
an active task. Retain their theme, restrained tokens, and existing typography.
Reuse `MobilePickerSheet` for the picker and inset `MobileMenuSheet` Drawer
geometry for management. A temporary multi-action tool belongs in a tall bottom
drawer rather than a persistent content destination. Wider layouts retain Dialog.

Use `@kandev/ui/drawer` on phones: fixed title and 44px Close control, bounded
`dvh` height, one `min-h-0` content scroller including manual input/preference,
and bottom safe-area padding. Preserve shared input anti-zoom sizing. Ensure
input and Add can scroll above the keyboard. All row details/proxy operations
remain available; URLs stack and wrap without shrinking controls.

The [active-first design](port-forwarding-active-first.md) remains authoritative
for projection, stable row keys, grouping, URL ordering, mutation pending state,
and Browser gating. The discovery contract supplies the phone Drawer wrapper;
the completed active-first plan retains its historical delivery results.

Close the picker before opening management, suppress its close autofocus during
handoff, and track the actual visible opener. On dismissal return to Panels for
picker-origin visits, or the header opener if still visible. Never focus the
unmounted picker row. Keep Escape/back handling provided by existing primitives;
create no new browser-history entry for this temporary tool.

## Data, persistence, and authorization

`metadata.port_forwarding_enabled` remains an optional per-task boolean; missing
and false mean shortcut off. `PATCH /api/v1/tasks/:id/port-forwarding` accepts
`{ "enabled": boolean }`, returns TaskDTO and publishes `task.updated`.
Keep unrelated metadata merging and existing workspace/task authorization.
Malformed payloads remain 400, inaccessible/missing tasks 404, failures 500.
An event without metadata preserves cached metadata; explicit metadata is
server-authoritative. Retain existing stale-payload reconciliation guards.

Existing `lib/api/domains/port-api.ts` supplies `port.list`, `port.tunnel.list`,
`port.tunnel.start`, and `port.tunnel.stop`. Runtime data, cleanup, port-proxy
URLs, and authorization remain session-scoped. Shortcut preference survives
browser/backend restarts; tunnels retain current runtime-only restart behavior.
Open state/manual drafts stay transient, never stored in task metadata/layouts.
Closing or hiding UI never stops tunnels. No telemetry, polling, or flags added.

## Failure and context changes

Preserve preference rollback/toast and runtime action feedback. A failed phone
switch write leaves management open; a failed wider enable never opens it.
Existing list clients map failures to empty arrays; do not promise a new list
error distinction or let empty detection replace known active rows.

Task changes already invalidate pending preference results. Extend dismissal
and pending-open guards to session identity, readiness loss, and archival.
A late write may reconcile its originating task's preference but must not open a
replacement session's UI. Keep session-keyed runtime response guards. Preference
off alone is not runtime ineligibility for phone management.

## Localization and delivery

Use `t()` for copy and accessible names; add en, pt-pt, zh-cn, zh-hk, zh-tw, ja,
ko and generated pseudo keys, using `pnpm run i18n:zh-hant` for the Traditional
Chinese pair. Reuse existing Panels/Port forwarding/Close labels where suitable.
The how-to in `docs/public/tasks-and-workflows.md` describes Panels access and
the separate task-wide header shortcut.

## Implementation Plans

- [Mobile port forwarding](../../../plans/mobile-port-forwarding/plan.md).
