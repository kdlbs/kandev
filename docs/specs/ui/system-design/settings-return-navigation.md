---
status: current
system: ui
requirements:
  - REQ-UI-SETTINGS-RETURN-001
---

# Settings return navigation design

## Purpose and boundaries

Keep one transient origin for the current Settings visit. Reuse the client
router and its navigation guard. Task route loading retains authorization,
workspace resolution, and missing-task handling.

## Requirement mapping

REQ-UI-SETTINGS-RETURN-001 maps to Origin tracking, Close navigation, and
Responsive presentation below. The work order maps each criterion to tests.

## Components and responsibilities

- `components/settings/settings-navigation-provider.tsx` provides
  `SettingsNavigationProvider` and `useSettingsReturnHref`.
- Mount the provider in `src/app-shell.tsx` around the sidebar and route content.
  It must survive route changes, sidebar collapse, and viewport changes.
- Use a small dependency-neutral transition helper in
  `lib/settings/settings-return-navigation.ts` to identify Settings paths and
  update the origin. Reuse that predicate from `app-sidebar-route.ts` so route
  recognition has one authority.
- Replace the Home-only close destination in
  `components/app-sidebar/app-sidebar-footer.tsx::useSettingsGearToggle` with
  the provider's destination. Preserve its existing successful-navigation callback.
- Add a phone close control in `components/settings/settings-layout-client.tsx`.
  Both controls consume the shared destination and guarded navigation behavior.
- Extend the existing history tracking in `lib/routing/navigation-guard.ts`
  with the `subscribeHistoryCommit` listener API. It reports actual
  before/after locations after successful native push or replace mutations.

## Origin tracking

The provider owns the last committed location and an optional return href.
Store root-relative `pathname + search + hash` without reconstructing task URLs.
This preserves session selection, repeated parameters, layout, and fragments.

Initialize from the actual browser location. A mount on Settings has no origin.
Subscribe to committed history mutations and guard-approved `popstate` events.
Do not use `hashchange` to commit departures: it can accompany a blocked traversal.
The actual source URL at a committed mutation still preserves its fragment.
The existing `trackNativeHistoryMutations` wrapper is the mutation authority.
Capture the actual source URL immediately before its native mutation. Notify
subscribers with source and destination only after that mutation succeeds.
Do not add another history wrapper or change history-state fields.

This notification is separate from the SPA's location event. In particular,
`lib/links.ts::replaceTaskUrl` intentionally updates the address without route
navigation. Tracking that mutation must not force a route render or reload.
A subsequent Settings entry therefore sees the selected task/session URL.

Process notifications synchronously using refs. A React pathname effect alone
can miss rapid Settings redirects or query-only task changes. Publish only
return-destination changes to consumers. Subscribe to `popstate` after the
router's capture-phase guard so blocked traversals do not update the origin.
Register and remove listeners with the provider lifetime, including Strict Mode
remounts. The subscriber API must not install or retain a component-specific
listener after unsubscribe.

1. On non-Settings to Settings navigation, retain the preceding location.
2. On Settings to Settings navigation, retain the origin.
3. On committed departure from Settings, clear that visit's origin.
4. On a later Settings entry, capture the new origin.

Capture only browser-observed same-origin application locations. Do not read
`document.referrer`, an arbitrary return query parameter, or external URLs.
Reject Settings routes as return targets. Home is the fallback when no eligible
origin exists. Do not infer an origin from the active task store: it can retain
a task that did not open Settings.

## Close navigation

Use `useRouter().push(returnHref ?? workspaceHomeHref(...))` through the existing
guard. Import the Home helper from `lib/navigation/workspace-home.ts`.
Read the active workspace and startup preference for fallback at close time.

Keep `onNavigated` coordination in the desktop footer. A cancelled request must
not toggle sidebar mode or consume the origin. The existing
`useSettingsTakeover` route synchronization handles a successful phone departure.
Do not replace the close action with `history.back()`: nested Settings visits
can put another Settings page immediately behind the current page.

The origin is a route, not a task existence cache. A deleted task or revoked
access follows existing route handling. Do not fetch task lists, select a new
session, or change task state to validate the destination.

## Responsive presentation

The desktop sidebar retains its current labelled action and collapsed icon.
Phones enter Settings through the existing app navigation drawer and retain
the full-page Settings index and forms. Dense settings content still needs a
page, not another drawer.

Add an `IconX` button to the phone Settings header actions. Reuse
`t("sidebar:closeSettings")` for its accessible name and the shared Button.
`AppNavSurface` is the nearest shipped close-control exemplar. Preserve existing
integration copy actions when composing the header action slot.

The close control is available below 768px regardless of pointer precision.
Provide at least 44px width and height, with enough header height to avoid
clipping. Keep the existing desktop header geometry. `settings-scroll-container`
remains the single content scroller. Existing dynamic viewport and safe-area
handling remain in the shell. No animation is needed for this route action.

Home breadcrumbs retain their explicit Home meaning. Settings breadcrumbs
continue to navigate within Settings. Do not repurpose either as Close settings.
If navigation is cancelled, focus remains in the existing confirmation flow.
Committed navigation uses the existing destination focus behavior.

## Persistence and compatibility

Keep context in provider memory. No local storage, session storage, history-state
format, backend API, or preference schema changes are needed. Refreshing Settings
uses the established Home fallback. Browser Back and Forward retain normal
router behavior and guard protection. Returning to Settings through history
captures the immediately preceding non-Settings page as a new visit.

## Verification

Unit tests exercise transition sequences and URL preservation. Provider tests use
real client-router events to cover rapid redirects, guards, query-only changes,
remounts, and repeated visits. Component tests prove desktop and phone wiring.
Playwright tests prove the original task and session become visible after close.
They also assert a document marker survives navigation and check phone geometry.

## Related decisions

- [Settings save coordination](../../../decisions/0046-settings-route-save-coordinator.md)
- [Navigation manifest boundaries](../../../decisions/2026-08-04-navigation-manifest-boundaries.md)

These existing ownership boundaries remain sufficient. This change does not
introduce another navigation registry or persistence contract.

## Implementation plans

- [Settings return navigation](../../../plans/settings-return-navigation/plan.md)
