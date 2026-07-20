# ADR-2026-07-19-workspace-symlink-entries: Treat Nested Workspace Symlinks as Entries

**Status:** accepted
**Date:** 2026-07-19
**Area:** backend, infra

## Context

Kandev workspaces routinely contain repository-owned symlinks, including checked-in links such as
`CLAUDE.md -> AGENTS.md` and package-manager links beneath `node_modules`. Storage analysis and
orphan cleanup rejected any nested symlink, which made normal task roots unmeasurable and
permanently ineligible for quarantine even when authoritative inventory classified them as orphaned.

## Decision

Workspace storage operations treat a nested symlink as an opaque directory entry and never follow
its target. `internal/system/storage/workspaces` measures roots with `filepath.WalkDir`, skips
symlink entries, quarantines the complete root with an atomic rename, and permanently deletes the
quarantined tree with `os.RemoveAll`; these operations do not traverse nested symlink targets.

The fail-closed boundary remains unchanged for owned control paths. A symlinked task root, tasks or
trash ancestor, quarantine path, ownership marker, or quarantine manifest is rejected before any
mutation. Authoritative liveness inventory, containment validation, grace periods, and quarantine
retention remain mandatory.

## Consequences

- Normal Git and package-manager workspaces can be measured, quarantined, and deleted.
- Files reached only through a nested symlink target are not counted and are never changed.
- Reviewers must preserve no-follow filesystem primitives throughout workspace maintenance; a
  future traversal API that follows symlinks must supersede this decision or restore rejection.
- Size totals describe bytes stored inside the workspace tree, excluding target data referenced by
  symlinks.

## Alternatives Considered

- **Reject every nested symlink.** Rejected because ordinary Kandev repositories then cannot be
  analyzed or reclaimed.
- **Resolve each target and allow only links contained by the task root.** Rejected because cleanup
  does not need target contents, relative and temporarily broken links are legitimate repository
  state, and resolution adds race-prone complexity.
- **Follow symlinks while measuring or deleting.** Rejected because it can count or destroy data
  outside Kandev-owned task roots.
