# ADR-2026-08-27-bounded-agent-profile-recency: Store Agent Profile Recency in Bounded Context Rows

**Status:** accepted
**Date:** 2026-08-27
**Area:** backend, frontend, protocol

## Context

Operational agent-profile selectors need a portable, per-user recent-use order.
The order changes after successful launches and must remain consistent across
browsers. The existing `users.settings` JSON is already large enough to carry
saved layouts, views, presets, and preference blobs. Updating that JSON for
every successful launch would rewrite the full settings value and publish the
full `user.settings.updated` payload even though recency is small and changes
frequently.

The recency contract has a fixed set of contexts and needs only the ten newest
distinct profile IDs in each context. It does not need to grow with the number
of workspaces, tasks, sessions, or profiles.

## Decision

The backend is the durable owner of agent-profile recent-use order. Store it in
a dedicated `user_agent_profile_recent_use` table with at most one compact row
per user and supported context. Each row contains an ordered JSON array of at
most ten distinct profile IDs, a monotonic revision, and an update timestamp.
Do not add this data to `users.settings`.

Load the bounded rows into a separate frontend state field in the authenticated
boot payload. A focused mutation records successful use and publishes a compact,
user-routed WebSocket event containing only the changed context, ordered IDs,
revision, and timestamp. Browser storage is not a source of truth; the frontend
can retain only an in-memory projection while the app is running.

The context set is fixed in the product contract. Recency is user-scoped rather
than workspace- or task-scoped, so storage remains bounded. Selectors filter
ineligible or unavailable profiles before they apply the remembered order.

## Consequences

- Each user has at most four small rows and forty retained profile IDs.
- A recency write updates one bounded row and does not rewrite or rebroadcast
  the user-settings JSON.
- Revisions make boot, mutation responses, and concurrent WebSocket delivery
  deterministic for each context.
- Cross-device ordering remains portable under the backend-ownership rule in
  [ADR 0041](0041-backend-owned-portable-user-settings.md).
- Recording is best effort after the primary launch succeeds. A recency failure
  does not turn a successful task or session launch into a failure.
- Deleted, disabled, incompatible, or otherwise unknown profile IDs can remain
  in bounded storage until a later write, but selectors ignore them.

## Alternatives Considered

1. **Add ordered arrays to `users.settings`.** Rejected because frequent writes
   would amplify database I/O, CAS contention, and full settings WebSocket
   payloads.
2. **Store one row per user, context, and profile.** Rejected because it needs
   more rows and index entries, plus additional ordering or revision machinery,
   without improving the bounded selector read path.
3. **Persist in localStorage or IndexedDB.** Rejected because the order would be
   device-local, could become stale, and would conflict with ADR 0041.
4. **Derive recency from tasks and sessions.** Rejected because those records do
   not preserve selector context, and ephemeral or superseded chats can be
   deleted.
