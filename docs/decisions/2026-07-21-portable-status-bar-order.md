# ADR-2026-07-21-portable-status-bar-order: Portable Status Bar Order

**Status:** accepted
**Date:** 2026-07-21
**Area:** backend, frontend, protocol

## Context

The app status surface exposes separate `app-status-bar-left` and
`app-status-bar-right` plugin slots, with built-in connection and metrics items on
opposite sides of a flexible spacer. Users need to move any visible item across
that spacer and keep the resulting layout after reloads, restarts, plugin
enable/disable cycles, and use from another client. Browser storage would make
the preference device-local and conflict with the backend-owned portable-settings
rule in ADR 0041.

## Decision

The slot name determines a plugin contribution's default side only. The host
treats the connection indicator, the complete metrics cluster, and each plugin
component registration as opaque, reorderable items. A plugin registration's
ordering identity is derived deterministically from its plugin ID, original slot,
and zero-based registration ordinal within that plugin and slot; plugin children
are never inspected or reordered independently.

The existing backend user-settings JSON owns an `app_status_bar_order` object
with ordered `left_item_ids` and `right_item_ids` arrays. Moving an item across
the flexible spacer transfers its ID between arrays. Temporarily unavailable IDs
remain in the arrays so disabled plugins and hidden metrics can recover their
positions. New items that have no saved identity enter at the end of their
default side. Existing PATCH omission semantics and the
`user.settings.updated` event carry the setting; no relational schema or new
endpoint is introduced.

Desktop and tablet provide Cmd-drag on macOS and Ctrl-drag elsewhere. Phone has
no drag interaction and renders the saved left sequence followed by the saved
right sequence, without the desktop spacer.

## Consequences

- User order is portable and has one durable owner instead of competing browser
  and backend copies.
- `left` and `right` remain useful defaults for plugin authors but are not
  permanent placement guarantees after user customization.
- Plugins must register multiple contributions in deterministic order if they
  want ordinal-derived identities to keep matching across reloads.
- The host can preserve disabled contributions without mounting them, at the
  cost of retaining opaque stale IDs until their plugin returns or the user
  resets the preference in a future surface.
- Phone reflects desktop customization while retaining its native drawer and no
  second persistent bar.

## Alternatives Considered

1. **Reorder only inside each named slot.** Rejected because the requested
   interaction explicitly allows crossing the full bar.
2. **Persist in localStorage.** Rejected because order is a portable preference
   and ADR 0041 makes backend user settings authoritative.
3. **Let plugins declare global priorities.** Rejected because it would compete
   with user order and expand the public plugin contract.
4. **Add keyboard or touch reordering.** Not selected for this interaction; the
   confirmed scope is modifier-plus-mouse dragging on the desktop/tablet bar.
