# ADR-2026-07-23-workspace-source-root-move-boundary: Workspace Source Root Move Boundary

**Status:** accepted
**Date:** 2026-07-23
**Area:** backend

## Context

Workspace-source materialization performs filesystem mutations for sources rooted at canonical
workspace paths. Go's `os.Root` can safely anchor operations within one such root, but it does not
provide a portable atomic, no-replace publication operation across two roots. Treating a cross-root
move as a copy followed by deletion could race with concurrent changes, overwrite a destination, or
lose data when either phase fails.

## Decision

File mutations are descriptor-rooted: each operation uses one canonical workspace/source root as
its filesystem authority. Rename is allowed only when both paths resolve under that same root.
Cross-root moves and renames are rejected explicitly before mutation.

## Consequences

Source materializers have one clear authority boundary and do not emulate a move with an unsafe
copy/delete sequence. A future cross-root transfer requires an explicit copy workflow or a
cross-platform no-replace primitive; it must supersede this ADR before being introduced.

## Alternatives Considered

1. **Copy then delete.** Rejected because it is not atomic and can race, overwrite an existing
   destination, or lose data on partial failure.
2. **Platform-specific rename or publication syscalls.** Rejected for now because their semantics
   and availability do not supply a portable cross-root, atomic no-replace contract.
