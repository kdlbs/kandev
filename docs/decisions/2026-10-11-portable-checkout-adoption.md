# ADR-2026-10-11-portable-checkout-adoption: Explicit checkout adoption

**Status:** accepted
**Date:** 2026-10-11
**Area:** backend

## Context

Healthy independent Git checkout admission already exists in PR #4057. A separately
prepared clone does not, however, repair the authoritative binding of a damaged
linked checkout. Automatic metadata recovery creates a new checkout and logical
worktree identity. The operator may instead need to retain the existing task,
session, environment and logical worktree while selecting an existing clone.

## Decision

The backend owns explicit preview, apply and validation of checkout adoption.
The operator must use a verified administrator session or personal access token.
Auth-disabled synthetic administrators are refused: a local agent can omit its
credentials and otherwise receive the same synthetic identity as an operator.
Only one selected host Worktree repository slot may change. Existing recovery
claims serialize adoption with startup, cleanup and ownership transfer. Publication
compares the complete selected inventory, session incarnation and task attachments,
and writes the binding and a provenance receipt in one transaction.

The candidate must be a pinned independent clone under the same owned task root,
with the registered repository origin and selected branch. The old checkout and
its broken marker remain untouched. No copying, reset, session creation or prompt
replay is authorized. Receipt validation proves only an exact obsolete artifact;
it grants no filesystem or execution permissions.

Raw operator SQLite repair scripts and outer guard policy remain deployment-owned.
They are not imported, and existing local receipts are not automatically migrated.
Server Docker hosting does not change the host Worktree ownership contract or the
task Docker executor's semantics.

## Consequences

Adoption retains logical identities and both directories, but requires explicit
operator action and stopped runtime consumers. A missing source identity, changed
marker, restored administration, foreign clone or stale snapshot refuses recovery.
Receipts remain subject to current ownership rather than becoming permanent grants.

## Alternatives Considered

- Close all recovery work as superseded: admission does not provide adoption.
- Port the SQLite repair helper: deployment policy cannot own portable lifecycle authority.
- Extend automatic recovery to discover clones: ambiguous discovery is not explicit selection.

## Related specification

[Portable workspace adoption](../specs/tasks/requirements/portable-workspace-adoption.md).
