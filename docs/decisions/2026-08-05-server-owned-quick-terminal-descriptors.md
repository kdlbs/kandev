# ADR-2026-08-05-server-owned-quick-terminal-descriptors: Server-Owned Quick Terminal Descriptors

**Status:** accepted
**Date:** 2026-08-05
**Area:** backend, frontend, protocol, security

## Context

Quick Terminal currently keeps its tab descriptors only in the browser's in-memory Zustand
store. A page reload therefore removes the UI identity even though a detached host-shell PTY can
still be alive in backend memory. Browser storage would repair refreshes for one device, but it
would create a second source of truth and would not support the selected server-backed lifecycle.

## Decision

Kandev stores Quick Terminal descriptors in a backend-owned, authenticated user/workspace-scoped
store. The descriptor owns the browser-generated UUID, stable workspace sequence, latest host-shell
session association, and bounded lifecycle snapshot. Quick Terminal boot and resync read that
store, while lifecycle mutations update it and explicit descriptor deletion owns stopping/removing
the tab.

The host-shell PTY manager remains the runtime owner of processes and rolling output. Its entries
are intentionally ephemeral: after a backend restart, persisted descriptors remain discoverable but
their missing session is marked exited/unavailable and is never replaced implicitly. Session IDs
are accepted for a descriptor only when the backend can prove that the manager entry belongs to the
descriptor's bounded client key.

## Consequences

- Reloads, browser restarts, and another client for the same authenticated user/workspace can
  restore terminal tabs without duplicating host shells.
- Descriptor membership and sequence allocation are protected by backend authorization and durable
  storage rather than client-controlled browser state.
- A backend restart loses PTY output and requires an explicit new shell, but it does not silently
  erase the user's tab strip.
- Lifecycle writes add API and persistence work to terminal creation, reattachment, exit, and
  close; failures must remain visible and must not affect sibling tabs.

## Alternatives Considered

1. **Keep descriptors in memory only.** Rejected because refresh loses the tab and makes a live
   detached PTY unreachable.
2. **Use sessionStorage or localStorage.** Rejected because it is device-local, can become stale,
   cannot coordinate authenticated clients, and would compete with the backend as the durable owner.
3. **Add descriptors to the general user-settings JSON blob.** Rejected because terminal rows have
   independent create/update/delete lifecycle, per-workspace sequence allocation, and concurrent
   sibling mutations that are clearer and safer in a dedicated descriptor store.
