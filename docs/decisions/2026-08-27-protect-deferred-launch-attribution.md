# ADR-2026-08-27-protect-deferred-launch-attribution: Protect Deferred Launch Attribution

**Status:** accepted
**Date:** 2026-08-27
**Area:** backend, protocol

## Context

Deferred task-create launches can outlive the HTTP or WebSocket request that
created them. A later WIP promotion or dependency-unblock event therefore needs
enough server-owned origin data to record the creator's successful selector
choice. The deferred launch intent currently lives in task metadata, and task
metadata is also exposed through public task projections and replaceable by
generic task updates.

The recency feature is about selector choices, not every way an agent profile
can enter a launch request. MCP supplies operational profile input but has no
profile selector, so an MCP-created deferred task must not change task-create
recency.

## Decision

Keep deferred task-create attribution in the server-owned portion of the
deferred launch intent. The task service strips client-provided deferred launch
records and attribution, and only HTTP/WebSocket selector handlers set the
internal opt-in before task creation. Generic task metadata replacement
preserves an existing deferred launch intent instead of replacing or removing
its ownership data. Deferred consumers require the explicit attribution marker
before recording recency.

Public task DTO and API projections remove the creator user ID and internal
marker. MCP can create and promote deferred launches, but its profile input is
not attributed to task-create selector history.

## Consequences

- Deferred launches can recover creator attribution after the originating
  request has ended.
- A metadata PATCH cannot redirect a pending launch's recency write to another
  user or erase its launch ownership state.
- Task responses retain the useful deferred launch fields without exposing the
  creator identity.
- The task metadata boundary remains compatible with the existing deferred
  launch consumer, but every future metadata endpoint must preserve its
  server-owned subfields.

## Alternatives Considered

1. **Infer attribution from every authenticated deferred create.** Rejected
   because programmatic MCP launches would reorder selector history.
2. **Accept user identity from mutable task metadata.** Rejected because a
   metadata update could target another user's recent-use row.
3. **Create a separate attribution table or task column immediately.** Deferred
   because it would add a second persistence and lifecycle path for a small
   subfield; the protected service/API boundary supplies the required contract
   while the existing deferred intent remains the launch source of truth.
