---
status: current
system: ui
requirements:
  - REQ-UI-SIDEBAR-HOVER-001
  - REQ-UI-SIDEBAR-HOVER-002
---

# Sidebar Hover Reveal System Design

## Boundary and existing behavior

`apps/web/components/app-sidebar/app-sidebar.tsx` owns the global sidebar.
Its layout wrapper currently snaps between 56 px and the stored expanded width;
its absolutely positioned aside animates width over 300 ms. Keep that explicit
collapse/expand behavior. The hover reveal changes only the visual aside.

## Requirement mapping

| Criteria for REQ-UI-SIDEBAR-HOVER-001 | Design section |
| --- | --- |
| .1, .3, .4 | Interaction state |
| .2 | Rendering and persistence |
| .5 | Responsive behavior |

## Interaction state

Add a local hook under `hooks/domains/sidebar/use-sidebar-hover-reveal.ts` to own
one cancellable timer using the saved delay (default 500 ms) and transient reveal state. Use pointer entry/exit,
focus containment, and cleanup; do not use the persisted collapse setter for hover.
Eligibility requires a visible sidebar (`!isMobile` from `useResponsiveBreakpoint`),
fine pointer, hover capability, and a non-touch pointer event. Subscribe to hover
capability changes and clean up that listener.

Cancel on pointer exit before timeout, unmount, explicit toggles, route changes,
and eligibility loss. A stale callback must not reveal after cancellation.
An explicit collapse or Escape dismissal suppresses activation until the next
outside-to-inside pointer entry. Route changes dismiss transient state so navigation
does not leave an overlay covering the destination.

After reveal, retain it while pointer/focus remains in the sidebar or a sidebar-owned
interactive portal. Use actual open-state callbacks or scoped ownership on those
portal roots; do not keep the sidebar open for every application dialog. Inventory
workspace picker, settings/footer popovers, task menus and plugin navigation portals
before wiring containment. Pointer transitions into owned portals must not unmount
the initiating control. On final pointer/focus exit, close without an added dwell.
Respect nested Escape handling; only an unhandled Escape closes the parent reveal.
If dismissal would hide the focused control, return focus to the persistent expand
control without starting a hover timer.

## Rendering and persistence

Keep wrapper width derived from the saved `appSidebar.collapsed` value. Derive visual
collapse from saved collapse plus transient reveal, and pass it to navigation/footer.
Keep `data-collapsed` describing saved state; expose a separate `data-hover-revealed`
attribute for transient state. Reuse the existing absolute aside, stacking and
reduced-motion-aware transition. Do not mount a second sidebar.

Adapt `AppSidebarHeader` to separate expanded presentation from the saved toggle
state: during reveal it shows the workspace picker and the existing localized
“Expand sidebar” action, which invokes the existing persistent toggle. It must not
show a collapse label on an action that pins the sidebar open. Show the resize
handle only for persistent expansion. While hover-revealed, the workspace picker
uses its existing local open state: the global `setWorkspacePickerOpen` action
intentionally persists rail expansion for the keyboard shortcut and must not run
for a temporary pointer-opened picker. Existing shortcut/store changes clear
transient state. Hover state itself remains transient. The following settings extension adds two
fields to the existing user-settings contract; no new endpoint or metrics.

## Responsive behavior

The shipped `components/kanban/mobile-menu-sheet.tsx` and
`components/task/mobile/session-task-switcher-sheet.tsx` provide the phone navigation
exemplars: visible menu trigger, inset drawer, fixed header and internal scrolling.
Retain these compositions and shared workspace/task actions. Phone navigation opens
by tap, then selects a destination; no hover dependency is introduced. Preserve
safe-area clearance, dynamic-height containment and existing touch targets. On
coarse-pointer wider screens retain explicit sidebar controls. Test 767/768 px and
pointer-mode changes to ensure hidden or touch surfaces cannot activate the reveal.

## Verification

Fake-timer hook tests prove 499/500 ms boundaries, fresh dwell, cleanup and stale
callback cancellation. Component tests prove visual versus saved state, no duplicate
navigation, toggle semantics, focus retention and owned portal interactions.
Playwright proves real overlay geometry, navigation and nested menus on desktop;
phone tests open the existing task drawer and navigate with no page overflow.
Existing toggle animation and resize tests remain regression coverage.

## Related contracts

- [App status bar](app-status-bar.md): layout reservation remains unchanged on hover.
- [Requirements](../requirements/sidebar-hover-reveal.md)

## Hover settings

REQ-UI-SIDEBAR-HOVER-002 maps .1-.3 to Preference storage and Settings UI,
.4 to Runtime updates, and .5 to Settings UI and the existing Responsive behavior.
UI remains the owner because these settings control reusable navigation presentation.
Reuse [ADR 0041](../../../decisions/0041-backend-owned-portable-user-settings.md)
and [ADR 0046](../../../decisions/0046-settings-route-save-coordinator.md); no new
persistence owner or ADR is needed.

### Preference storage

Extend `internal/user/models/models.go`, `dto/dto.go`, `service/service.go` and
`store/sqlite.go` with `sidebar_hover_enabled` (boolean, default true) and
`sidebar_hover_delay_ms` (integer 0..5000, default 500). Use pointer fields for
partial updates so omitted values remain unchanged and false/zero are explicit.
Validate before mutation; invalid requests must neither save nor broadcast.
Use `internal/user/controller/controller.go` for the DTO-to-service adapter and
register the fields in `internal/settingscatalog/defaults.go` for configuration
discovery. Include the Go boot projection in `internal/backendapp/boot_state_routes.go`. Extend the existing JSON payload serialization,
default overlay, complete response DTO and revisioned user-settings broadcast.
No SQL schema migration is needed. Test legacy JSON with missing fields separately
from explicit false/zero; malformed legacy delay falls back to 500 without
resetting other settings. Reuse existing per-user identity and authorization.

Extend `lib/types/http-user-settings.ts`, `lib/state/slices/settings/types.ts`
and `lib/ssr/user-settings.ts` with camel-case store fields `sidebarHoverEnabled`
and `sidebarHoverDelayMs`. Reuse the common mapper for boot, save responses and WS
updates; partial payloads retain current values. Do not introduce browser persistence
or consumer-specific defaults. Cover hydration and revision ordering regressions.

### Settings UI

Use an icon-bearing `SettingsSection` heading above the sidebar card, matching
the surrounding Appearance sections; keep controls inside the card. Add
`components/settings/sidebar-hover-settings-card.tsx` to
`AppearanceAccountSections` alongside the existing status-bar appearance card.
Extend `AppearanceState`, saved-state construction, minimal patch builder, draft
rebase and revision functions in `appearance-settings-state.ts`; integrate validation
with the existing contributor in `general-settings.tsx`. Preserve unsaved values
when remote settings arrive and use accepted backend responses as saved state.
Keep numeric input text as a draft so clearing a field cannot silently become zero;
only a finite integer within bounds may enter the save payload. Disabling while
input is invalid restores the last valid delay before disabling the editor.
Use the route Save changes / discard surface; no new independent Save button.
Failures leave the contributor dirty and the effective saved hover state unchanged.

Register searchable toggle and delay entries in
`lib/settings-discovery/catalog/preferences.ts` with anchors targeting their actual
controls. Localize labels, helper text and errors in all five supported locale
catalogs; generate Traditional Chinese with the existing script.

Reuse SettingsCard, Label, Input, Switch and settings control sizing helpers.
Desktop numeric inputs use 28 px ordinary control height; phone and coarse-pointer
controls have at least 44 px hit areas. Stack the numeric label above its input on
phone, retain the existing settings page vertical scroll owner and floating save
surface, and avoid new drawers. The shipped `mobile-general-settings.spec.ts`
is the settings composition/test exemplar. Settings remain available on phones
because they configure the account's mouse/trackpad behavior.

### Runtime updates

Pass the saved enabled/delay fields from AppSidebar into `useSidebarHoverReveal`.
Enabled is part of eligibility; the timer uses the delay argument. Include both
values in cancellation dependencies so stale callbacks cannot reveal using old
preferences. Preference changes dismiss transient state and require fresh pointer entry;
restore focus to the visible toggle if dismissal would hide focus. Gate that
restoration on a preference change so an ordinary explicit collapse cannot retain
stale focus and hold a later hover open. Explicit
expansion remains independent. Zero schedules activation without dwell, still
using cancellable timer ownership. No saved collapse writes occur on setting changes.

### Extension verification

Backend tests cover defaults, false/zero preservation, independent patches, bounds,
invalid-request atomicity, persistence and emitted values. Mapper/draft tests cover
boot/WS/partial updates, save/discard/failure and invalid numeric drafts. Fake-timer
hook tests prove disabled activation, 0 ms and custom timing, cancellation during
pending/revealed states and focus safety. Desktop browser tests save both controls,
reload, check disabled/manual behavior and a custom delay. Phone tests edit/save/reload
both controls, measure touch targets and overflow, and preserve tap navigation.
