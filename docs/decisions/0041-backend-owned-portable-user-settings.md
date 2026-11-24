# 0041: Backend-Owned Portable User Settings

**Status:** accepted (amended 2026-11-19)
**Date:** 2026-07-15
**Area:** frontend, backend

## Context

Portable user preferences were migrated from browser storage to backend user
settings with temporary local fallbacks, dual writes, and retry markers. Those
paths remained long enough for active local installations to run a migration
release, but keeping them allowed stale browser values to compete with the
backend and complicated hydration.

## Decision

Backend user and workspace settings are the only durable source of truth for
portable preferences. The frontend may update in-memory state optimistically,
but it does not hydrate or persist task filters, sidebar views and ordering,
integration presets, or task-create last-used choices through localStorage or
sessionStorage.

Browser storage remains appropriate for explicitly device-local UI state and
transient drafts. User-settings PATCH semantics remain unchanged: omitted
fields are unchanged, explicit `null` clears nullable fields where supported,
and an empty array is an explicit empty value.

Manual sidebar task colors are portable personal settings. They are not
device-local state and they are not shared task fields. The backend stores a
per-user color decision for each task. A clear decision remains as a tombstone
while legacy browser import support exists.

The frontend can read legacy browser colors only as migration input. The
backend imports a legacy color only when no stored color decision exists for
that task. The frontend removes the legacy value after a successful import. It
does not use legacy data as an authoritative display fallback.

The backend also owns the effective value of an omitted portable setting. It
starts from one default settings value and overlays fields present in the
stored JSON before emitting a complete HTTP or WebSocket payload. The frontend
uses one wire-to-store mapper for boot hydration, mutation responses, and live
updates. An unloaded frontend state is only a rendering placeholder. It is not
an independent source of product defaults.

### Workspace sidebar mutation isolation

Saved-view changes carry operation intent into the settings CAS. `views_create`
inserts absent IDs only; `views_update` carries sparse fields and grouping-owned
disclosure additions/removals against an existing ID. A replayed creation and
its successor edit remain independent. `draft_update` carries sparse fields
under a base-view/group owner, including guarded discard. Explicit replacement
and upsert remain available for intentional imports, not ordinary edits.

Generated plugin-view selection uses `select_plugin_grouping` inside that same
CAS: reuse the latest unchanged matching view, otherwise allocate its unique
display name and create the candidate within the view limit. Renamed/custom
views and pending drafts remain protected. The browser waits for the
authoritative selected ID rather than installing a provisional saved view.

While a provider selection or draft save awaits its authoritative result, the
workspace exposes a transient busy state and blocks dependent view edits.
Task actions remain available. `save_draft` transfers the latest matching
base/group-owned draft inside CAS, including committed disclosure from other
tabs, into the saved base or an independently allocated copy. A changed owner
is a conflict, not permission to recreate a stale after-image.
Save and Save as are unavailable while a preceding workspace preference write
is unacknowledged. A failed prerequisite cannot be followed by a successful
transfer of older draft data; its failure remains actionable.
Save as naming captures its base/group owner when the editor opens. Replacing
that owner dismisses the editor and cannot transfer or clear the replacement
draft. Same-owner fields and disclosure remain fresh at the CAS transfer.

Client queues preserve captured operation intent across newer HTTP/WebSocket
snapshots. A skipped metadata-compaction item is not a persistence ACK and
cannot discard failed predecessor intent needed by a queued successor.
Disclosure-only compaction has no automatic transport retry: its provider
admission fence cannot be carried across an unobserved retry interval.
Provider selection also sends at most one mutation after uncertain transport;
an identical click is not safe to replay against newly renamed or selected views.
Compaction captures whether disclosure belongs to a saved row or a draft for a
different grouping. A draft-owned prune requires that draft to still exist
inside CAS; it cannot acquire a saved row's owner after another client transfers the draft.
Explicit final field assignments and disclosure removals survive uncertain
acknowledgements even when they reverse an earlier edit. An explicit queued
reorder captures only its moved ID, destination and before/after placement, then
reapplies that move over the latest admitted order. Disjoint remote moves remain
intact. Retired moves are excluded from successor replay, so a missing endpoint
cannot poison an unrelated queued edit. A dispatched full-order replacement
carries `view_order_base`; CAS accepts the observed base or an already-applied
requested order. An identical retry after a lost acknowledgement remains
idempotent. A different committed order is rejected, preserving a concurrent
move rather than silently undoing it.
For a retry combined with collection edits, compare the requested order with
the natural post-mutation order, while comparing the observed base with the
pre-mutation order. A create/delete successor must not conflict with its own
already-committed move.

Failed local prerequisites, provider selections and draft saves are not replayed
by disclosure compaction. At terminal acknowledgement the client adopts the
latest confirmed snapshot and preserves their failure indication instead of
keeping phantom state or silently discarding a transition failure. This marker
survives intermediate queued acknowledgements; replayed view mutations and
self-healing compaction keep their separate recovery policies.
Deferred snapshots carry an error value captured at receipt time. Restoring
their authoritative settings must preserve a newer unreplayed failure rather
than overwrite it with that stale error value, including skipped compaction.
Deferred settings never own the current error. Only an explicit successful
healing path clears it. A failed view mutation discarded without replay, or a
retired/rejected move, retains its notice even if an independent write succeeds.
A definitive HTTP rejection retires dispatched move intent even when both
endpoints survive. An independent successor must not give that rejected move a
fresh order baseline. Transport-uncertain failures retain the existing guarded
replay behavior; timeout and rate-limit responses are not definitive rejection.
Disclosure canonicalization deduplicates current committed keys without
replacing concurrent additions.

## Consequences

- Stale browser values cannot override backend settings during hydration.
- Failed optimistic writes are not reconstructed from browser retry markers.
  The backend value wins on the next load.
- Cross-device behavior is consistent because portable settings have one
  durable owner.
- Changing an omitted-value default no longer requires editing several
  independent frontend delivery paths.
- Frontend compatibility handling remains centralized in the common mapper.
- Device-local layout, collapse, pane-size, and transient draft storage remain
  unaffected.
- Manual task colors follow a user across browsers without changing another
  user's task presentation.
- Per-task patches and clear tombstones prevent stale browsers from replacing
  other colors or restoring a cleared color during migration.
- The legacy import path is temporary. Tombstones can be removed only after
  that path is retired.

## Alternatives Considered

1. **Keep browser storage as an offline cache.** Rejected because offline
   recovery is not part of the portable-settings contract and stale caches can
   override explicit backend clears.
2. **Keep retry markers without fallback reads.** Rejected because replay still
   requires treating browser state as a second durable source.
3. **Let every frontend consumer apply wire fallbacks.** Rejected because HTTP,
   WebSocket, and save-response paths can drift. These paths can produce
   different effective settings for the same backend value.
4. **Keep manual task colors device-local.** Rejected because users expect a
   personal color to remain consistent in each browser.
5. **Store one shared color on each task.** Rejected because manual sidebar
   colors are personal presentation and must not change another user's view.
6. **Import browser values as full replacements.** Rejected because one stale
   browser can erase newer server values or colors from another browser.
