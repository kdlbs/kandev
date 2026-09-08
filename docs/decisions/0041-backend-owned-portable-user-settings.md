# 0041: Backend-Owned Portable User Settings

**Status:** accepted (amended 2026-09-03)
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
