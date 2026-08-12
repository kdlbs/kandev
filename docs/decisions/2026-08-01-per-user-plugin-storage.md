# ADR-2026-08-01-per-user-plugin-storage: Host-Provided Per-User Plugin Storage

**Status:** accepted (amended 2026-08-12)
**Date:** 2026-08-01
**Area:** backend, frontend, security

## Context

The plugin contract's only persistence is `plugin_state`
(`apps/backend/internal/plugins/state/store.go`), keyed
`(plugin_id, scope, scope_id, state_key)` with no user dimension, written only
by a plugin's own gRPC-connected Go backend via the Host `SetState` RPC. At
the time of this ADR, the only frontend-reachable plugin HTTP route was the
unauthenticated `/api/plugins/:id/webhooks/:key` proxy, explicitly bypassed in
`internal/auth/httpmw/middleware.go`'s allowlist.

**Amendment (2026-08-12):** that allowlist entry was removed — see
`2026-08-12-plugin-webhook-auth-gate.md` — so the webhook proxy is
authenticated by default too, unless its manifest entry opts out with
`webhooks[].public: true`. This does not change the rest of this ADR's
reasoning: what motivated `host.storage` was the *browser session's own
user identity* being reachable at all from a plugin's frontend bundle, which
the webhook proxy still does not provide (it relays a request, not a
per-user storage API).

A UI-only plugin (a native JS bundle with no Go backend) that needs to persist
data scoped to the calling browser session's user — a scratchpad, a checklist,
a per-user filter — has no host-provided way to do so. The driving case is a
follow-up Notes plugin (porting PR #2050's native task-notes feature into a
plugin) that needs exactly one per-user, per-task document with no Go binary
of its own.

`docs/specs/auth/spec.md` records hard per-user privacy: workspaces are
per-user, cross-user access returns 404, and admins get no visibility into
other users' data. Any new storage surface must hold that invariant.

## Decision

A new `plugin_user_state` SQLite table, isolated from `plugin_state`:

```sql
CREATE TABLE plugin_user_state (
  id TEXT PRIMARY KEY,
  plugin_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  scope TEXT NOT NULL DEFAULT 'instance',
  scope_id TEXT NOT NULL DEFAULT '',
  state_key TEXT NOT NULL,
  value_json TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (plugin_id, user_id, scope, scope_id, state_key)
);
```

- **A new table, not a new column.** `plugin_state`'s uniqueness is a table
  constraint, not a named index, so SQLite cannot `ALTER` it in place — adding
  `user_id` would force the create/copy/drop/rename rebuild pattern used
  elsewhere in this codebase (`internal/sentry/store.go`), on a table two
  transports (gRPC and, now, HTTP) write to. A second table keeps the trust
  boundary explicit: `plugin_state` is plugin-owned (gRPC, no caller identity),
  `plugin_user_state` is user-owned (HTTP, authenticated actor) — no migration,
  no shared-table risk.
- **Four authenticated REST routes**, gated by identity
  (`authn.FromGin`) and a new manifest capability
  (`capabilities.user_state: true`, `403` without it):
  `GET/PUT/DELETE /api/plugins/:id/user-state/:scope/:scopeId/:key` and
  `GET /api/plugins/:id/user-state/:scope/:scopeId` (list). These routes are
  **not** added to `httpmw`'s public-path allowlist — an unauthenticated
  request is rejected before the handler runs, the same as any other
  authenticated kandev API route. `scope` is one of
  `instance|workspace|task|session|repository`; `scopeId`/`key` must match
  `[A-Za-z0-9][A-Za-z0-9._:-]{0,127}`; the body is capped at 256 KiB.
- **Row-level user isolation, not just route-level.** Every query filters by
  `user_id` from the authenticated identity — a `GET` for another user's key
  returns `404`, never a value or a distinguishable "exists but not yours"
  signal.
- **Optional conditional write.** `PUT` accepts `ifUnmodifiedSince`; the write
  folds that comparison into the upsert's own `ON CONFLICT ... DO UPDATE ...
  WHERE updated_at <= ?` clause — not a separate read-then-write, even inside
  one transaction, which only serializes under SQLite's single-writer model.
  This codebase also runs against PostgreSQL, whose default READ COMMITTED
  isolation lets two concurrent transactions both pass a plain `SELECT`-based
  check and both proceed to write. The atomic upsert makes the check and the
  write one row-locking statement regardless of isolation level: a lost race
  affects zero rows, mapped to `409` (value unchanged). Omitting the field
  preserves unconditional last-write-wins.
- **Uninstall purges every user's rows**, not just the uninstalling admin's —
  `Service.Uninstall` calls `UserStore.DeleteAllForPlugin` alongside the
  existing `plugin_state` purge, so a reinstalled or id-reused plugin never
  inherits stale per-user state.
- **Realtime notification carries keys, not values.** A successful
  `PUT`/`DELETE` publishes `plugin.user-state.updated` on the event bus,
  reused through the existing `UserEventBroadcaster` (the same user-scoped
  fan-out path `user.settings.updated` already uses) to the writing user's own
  WS connections only. The payload is
  `{ pluginId, scope, scopeId, key, updatedAt, writerId, deleted }` — the host
  never re-broadcasts plugin-supplied data to a socket; a subscriber refetches
  via `host.storage.get`. The event's `user_id` field is exported (not the
  unexported field an earlier draft used) so it survives the JSON marshal a
  NATS-backed `EventBus` performs before publishing; `UserEventBroadcaster`
  strips it from the payload every client actually receives, regardless of
  transport. A `writerId`, stamped by `host.storage.set`/`delete` and echoed
  back, lets `host.storage.subscribe` skip its own echo. A caller-supplied
  per-surface id (e.g. a dockview `panelId`) is appended to the shared
  per-tab id rather than replacing it — replacing would let a surface id
  that is the same string in every tab (a `panelId` is) make two different
  tabs look like the same writer to each other, breaking cross-tab sync;
  appending keeps two surfaces of the same plugin in one tab (a task panel
  and a kanban quick-action, say) from mistaking each other's writes for
  their own, without losing the per-tab distinctness the mechanism depends
  on in the first place.
- **No gRPC/proto change.** `Host.GetState/SetState/DeleteState/ListState` and
  `plugin_state` are unchanged; per-user storage and its notification are
  entirely browser-facing surfaces.

## Consequences

A UI-only plugin bundle can persist per-user data with zero Go backend,
turning the follow-up Notes plugin (and any future scratchpad/checklist/
per-user-preference plugin) into a pure frontend artifact. The host gains its
first plugin HTTP route reachable with the caller's own browser session —
AC15–AC18's validation (identity, capability, scope/key shape, body cap) and a
direct `httpmw` allowlist pinning test are the guardrails against that being a
privilege escalation. `plugin_user_state` and `plugin_state` are permanently
separate tables with separate write paths (HTTP vs gRPC); a future unification
would need its own ADR amending this one. The realtime path is invalidation,
not merge — two of a user's own surfaces editing the same document
concurrently still resolves last-write-wins (mitigated, not eliminated, by
`ifUnmodifiedSince`); full collaborative editing (CRDT/OT) remains out of
scope.

## Alternatives Considered

- **Add `user_id` to `plugin_state` in place.** Rejected: SQLite can't alter
  the existing table-level `UNIQUE` constraint without a rebuild, on a table
  actively written by the gRPC path; higher risk for no benefit over a new
  table.
- **Encode the user in `state_key`** (e.g. `note:<userId>`). Rejected on
  security grounds: nothing prevents a different plugin instance from reading
  another user's key — no real isolation, just an obscured one.
- **Route per-user writes through the plugin's own gRPC backend** (a plugin
  Go process reads the caller's identity from a forwarded header and calls
  `SetState` itself). Rejected: reintroduces the exact Go-backend requirement
  this ADR exists to avoid for UI-only plugins, and duplicates identity
  plumbing the host already owns for every other authenticated route.
- **Broadcast the full value in the WS payload** instead of keys only.
  Rejected: puts arbitrary plugin data (up to the body cap) on every socket of
  that user and makes the WS message a second source of truth alongside the
  stored row.
- **Plugin-side polling instead of a realtime notification.** Rejected: every
  plugin would reinvent polling at its own interval (more request volume than
  the WS message it replaces), and the multi-surface case (task panel + kanban
  modal + second tab + desktop app, all editing one document) makes silent
  staleness routine rather than rare.
