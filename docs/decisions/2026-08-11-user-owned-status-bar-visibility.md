# ADR-2026-08-11-user-owned-status-bar-visibility: Make Status Bar Visibility a Portable User Preference

**Status:** accepted
**Date:** 2026-08-11
**Area:** backend, frontend, protocol

## Context

The App status bar shipped behind the install-wide `features.appStatusBar`
runtime flag. That flag was appropriate while the surface needed a staged
rollout, but it gives an administrator one restart-required choice for every
user and client. Whether to show ordinary status chrome is now a personal
appearance choice. Kandev already treats status item order and other portable
appearance choices as backend-owned user settings.

The existing `system_metrics_display.show_in_topbar` field cannot own whole
surface visibility. It controls only whether optional host metrics appear and
must remain independent from connection state, plugin contributions, and LSP
placement.

## Decision

Retire the active runtime registration for `features.appStatusBar` and
`KANDEV_FEATURES_APP_STATUS_BAR`. Move that exact key and environment variable
to the append-only retired runtime-flag identities. Persisted overrides for the
now-unknown key remain inert, and neither identity may be reused.

Backend-owned user settings become the sole durable owner of a top-level
`app_status_bar_enabled` boolean:

- a missing stored value or initial compatibility payload defaults to `false`;
- an omitted PATCH field preserves the current value;
- an omitted field in a partial live update preserves the current value;
- an explicit `true` or `false` is preserved;
- boot hydration, PATCH responses, and `user.settings.updated` events carry the
  effective value to every client.

Settings > General > Appearance exposes the preference as **Show status bar**
through the existing shared save coordinator. A successful save changes the
active responsive presentation without restarting Kandev. Desktop and tablet
show or hide the bottom bar; phone shows or hides ordinary Status entry points
and the general Status drawer.

The preference gates only ordinary status surfaces. An active WebSocket
connectivity warning still bypasses the visibility choice through its
problem-only sidebar or connection-only phone fallback. Turning the bar off
also preserves the existing top-bar metrics fallback and LSP toolbar fallback.
The independent agent-runtime availability alert is never gated by this
preference.

No value is migrated from the former install-wide runtime flag or override into
individual users. Missing preferences remain off, preserving the former
production and development default; users explicitly opt in portably.

## Consequences

- The stable status surface is available to every user without a release toggle
  or restart, but ordinary status chrome remains off until that user enables it.
- Each user's choice follows their backend identity and updates other connected
  clients through the existing user-settings event.
- Operators lose the install-wide status-bar kill switch and its environment
  override.
- Existing runtime override rows need no destructive cleanup and cannot affect
  the new preference.
- No relational migration, endpoint, or new WebSocket action is required
  because user settings already live in a JSON blob with PATCH and event
  delivery.
- Visibility, metrics inclusion, metrics style, item order, and LSP preferred
  location remain separate settings with separate responsibilities.

## Alternatives Considered

1. **Promote the runtime flag to on while keeping it as a kill switch.**
   Rejected because a restart-required install-wide control has the wrong owner
   for a stable appearance choice.
2. **Store visibility in browser local storage.** Rejected because the choice is
   portable and ADR 0041 makes backend user settings authoritative.
3. **Reuse `system_metrics_display.show_in_topbar`.** Rejected because hiding
   optional host metrics must not also hide connection and plugin status or
   change LSP placement.
4. **Copy the old runtime value into every user.** Rejected because one
   deployment value cannot express per-user intent, would preserve the rollout
   gate after promotion, and has no unambiguous mapping for users created later.
5. **Default the portable preference on.** Rejected because that would enable
   new ordinary status chrome for users who never opted in and would not
   preserve the former production and development default.
