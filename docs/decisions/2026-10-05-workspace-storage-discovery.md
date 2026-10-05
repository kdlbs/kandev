# ADR-2026-10-05-workspace-storage-discovery: Require workspace layout evidence during discovery

**Status:** accepted
**Date:** 2026-10-05
**Area:** backend

## Context

Storage discovery treats every unmarked nonsemantic directory as a scratch container.
It treats every real child directory as a task root and rejects every child symlink.
An unrelated checkout beneath the tasks root therefore prevents measurement of valid workspaces.
Removing its symlink exposes a second defect: ordinary repository directories can become orphan candidates.

Kandev creates unmarked legacy scratch paths with UUID workspace and task IDs.
Current scratch roots carry ownership markers. Semantic task roots retain a separate legacy naming rule.

## Decision

Discovery requires a valid ownership marker or an existing supported legacy layout.
The unmarked scratch fallback requires canonical dashed UUID names for both the workspace parent and task child.
Marker-backed scratch roots retain non-UUID names.

Unrecognized directories remain untouched and absent from workspace measurements and cleanup candidates.
Discovery reports their omission through existing warnings.
Ordinary symlinks in an unrecognized parent do not prevent discovery of other task roots.

Recognized scratch containers retain rejection of child symlinks.
Owned roots and control paths retain existing symlink guards.
Nested entries inside a recognized task root follow the existing opaque-symlink decision.
All storage consumers use the same discovery rules and existing authoritative inventory gates.

## Consequences

Valid task measurements remain available beside unrelated checkouts.
The correction cannot turn repository folders into orphan tasks after skipping a link.
Unmarked custom non-UUID scratch paths remain intact but no longer participate without a valid ownership marker.
The standard generated layouts remain supported. No migration writes or automatic adoption occur.

The linked implementation package records the conformance and verification evidence for this decision.

## Alternatives Considered

- Skip all child symlinks while preserving arbitrary scratch inference. This restores measurements but can admit unrelated repository folders to cleanup.
- Require a marker for every root. This rejects supported unmarked legacy semantic and UUID scratch layouts.
- Recognize only currently live database paths. This misses orphaned legacy roots after their task rows disappear.
- Treat every discovery error as a warning. This conceals unsafe control paths and incomplete ownership evidence.
- Move or remove the unrelated checkout. This changes user files and leaves the discovery defect intact.

## References

- [Workspace discovery design](../specs/system-page/system-design/workspace-storage-discovery.md).
- [Fix package](../plans/workspace-storage-discovery/plan.md).
- [Nested workspace symlink decision](2026-07-19-workspace-symlink-entries.md).
