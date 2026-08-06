# ADR-2026-08-04-plugin-contribution-lifecycle-authority: Make Plugin Contribution Lifecycle Authoritative

**Status:** accepted
**Date:** 2026-08-04
**Area:** frontend, security

## Context

Plugin task panels are registered asynchronously. Boot loads plugins sequentially,
updates unregister the previous version before awaiting the replacement, and plugin
initialization may legitimately take several seconds. Treating a registration as
removed after a fixed 500 millisecond delay can therefore delete a valid open or saved
panel. Mobile also gives every registered panel an unbounded bottom-navigation item and
does not clear a removed focused panel, while kanban menu actions cannot distinguish the
existing phone board because the card context is always labelled desktop.

Separately, per-user plugin storage promises that successful uninstall prevents a
reinstalled or ID-reused plugin from inheriting prior users' data. Best-effort cleanup
after deleting the package and plugin record cannot uphold that security invariant.

## Decision

The frontend plugin host owns an internal, reactive lifecycle state per plugin. Loads
enter `loading` before bundle import or registration revocation. They finish as `ready`
only after `initialize()` completes for the current generation, and as `failed` after a
terminal import/registration/initialization failure. Explicit disable and uninstall
enter `removed`. Panel reconciliation follows lifecycle transitions rather than a
timer:

- `loading` and `failed` preserve open and serialized panel identities;
- `ready` closes identities the successfully initialized plugin did not register;
- `removed` closes every owned panel immediately;
- stale generations cannot publish lifecycle completion or registrations over a newer
  load.

Desktop dockview and the phone task layout consume the same lifecycle authority. If a
removed plugin panel is focused on a phone, the mobile session state changes to `chat`.
Mobile-enabled registrations are exposed through one touch-sized **Panels** bottom-nav
entry. It opens the existing `MobilePickerSheet` pattern with a single internally
scrolling list of all available plugin panels; choosing a row closes the picker and
focuses the selected full-height panel. Core navigation targets therefore retain usable
width as plugin count grows.

Kanban composition passes an explicit `"desktop" | "mobile"` presentation down to
`KanbanCard`; plugin menu contexts use that value rather than inferring or hardcoding
it inside each card.

Successful uninstall is a fail-closed deletion boundary for `plugin_user_state`.
Deleting all users' rows happens before package and record removal; failure returns an
error and leaves the stopped plugin installed so an operator can retry. This tightens
the implementation of ADR-2026-08-01-per-user-plugin-storage without changing its
table, HTTP, WebSocket, or capability contracts.

## Consequences

Slow loads and failed updates no longer destroy user layout choices, while successful
updates can still revoke panels intentionally removed by a new plugin version. The
registry gains internal lifecycle bookkeeping and load/unload callers must state
whether a transition is reload, disable, or uninstall. This state is host-internal and
does not extend the frozen plugin-facing `PluginRegistry` interface.

Phone navigation remains bounded and touch-usable for any plugin count at the cost of
one extra tap to choose a plugin panel. Per-user state cleanup failures become visible
uninstall failures instead of warnings, so an operator may need to retry after a
database problem; this is preferable to reporting success while retaining private
data.

## Alternatives Considered

- **Keep a longer revocation timeout.** Rejected because no finite duration proves that
  import or initialization is complete, especially with sequential plugins.
- **Never close unresolved panels.** Rejected because disable, uninstall, and a
  successful update that removes a panel must revoke live UI and stale layout state.
- **Keep one bottom-nav item per plugin panel with horizontal scrolling.** Rejected
  because it creates two-dimensional navigation, weakens touch-target geometry, and
  scales core navigation according to third-party plugin count.
- **Derive mobile presentation inside every card with a responsive hook.** Rejected in
  favor of passing the already-known layout presentation through kanban composition,
  keeping breakpoint policy out of repeated card instances.
- **Delete user state best-effort after uninstall.** Rejected because there is no
  retryable plugin record after failure and a later installation can inherit stale
  per-user data.
