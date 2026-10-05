---
status: current
system: system-page
requirements:
  - REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-001
created: 2026-10-05
owners:
  - kandev
---

# Workspace storage discovery

## Purpose and boundaries

The system-page system owns storage measurement and maintenance eligibility. Workspace creation remains with the workspace and agent-runtime systems.
This design defines discovery for storage analysis, orphan quarantine, and dependency cleanup.
It clarifies the discovery boundary in [storage maintenance](storage-maintenance-01.md#task-cleanup-and-orphan-workspaces).

## Requirement mapping

| Requirement | Criteria | Design sections |
| --- | --- | --- |
| `REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-001` | `001.7`, `001.9` | Recognition, Measurement and presentation |
| `REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-001` | `001.10`, `001.11` | Recognition, Failure and safety |

## Components and responsibilities

`internal/system/storage/workspaces.Provider` loads authoritative inventory before discovery.
`discoverTaskRoots`, `discoverTaskRoot`, and `discoverScratchRoots` return recognized roots and warnings.
`Analyze`, `Cleanup`, and `CleanupDependencies` consume the same recognition rules.
`internal/backendapp.storageOverview` projects analysis through the existing storage overview API.

## Recognition

Discovery uses directory entries and `Lstat`. It never resolves symlink targets.
It preserves current marker precedence and the existing `looksSemanticTaskDir` rule.

| On-disk shape | Recognition | Result |
| --- | --- | --- |
| Direct child with a valid ownership marker | Existing marker rules | Measure the task root once |
| Unmarked semantic task directory | Existing slug and three-character suffix rule | Retain legacy semantic behavior |
| Scratch child with a valid layout-2 marker | Marker evidence, including existing non-UUID fixture names | Measure the child once |
| Unmarked scratch child | Both parent and child names use canonical dashed UUID syntax | Retain legacy scratch behavior |
| Other unmarked directory or child | No recognized task layout | Preserve and omit it, with a warning |

Canonical UUID syntax means 36 characters with the standard dashed UUID representation.
The existing `github.com/google/uuid` dependency parses the value. Comparison with its normalized representation rejects compact and URN forms.
Uppercase hex remains equivalent to lowercase hex. No new dependency or inventory field is necessary.

The unmarked fallback matches `lifecycle.Manager.scratchWorkspacePath`: `tasks/<workspace-id>/<task-id>`.
Task creation uses UUIDs in `task/service.service_tasks.go`.
Workspace creation generates UUIDs in `task/repository/sqlite/workspace.go`.
Marker-backed scratch paths retain support for non-UUID names.
Unmarked custom non-UUID scratch paths remain intact but need a valid existing marker to participate in maintenance.

Scratch discovery examines only immediate children of a real parent directory.
It inspects markers only on real child directories.
An arbitrary real directory never becomes a scratch candidate solely because it sits two levels beneath the tasks root.
Rejected unmarked child directories produce warnings even when another child proves that the parent is a scratch container.

A canonical UUID parent or at least one valid scratch child marker identifies a scratch container.
All symlink entries beneath that recognized container retain discovery rejection.
For an unrecognized parent, ordinary symlink entries remain opaque and do not abort discovery.
Container recognition must complete before applying this symlink rule, regardless of entry order.
Invalid markers and unexpected nested semantic markers retain their existing errors.

## Measurement and presentation

`Analyze` submits recognized, non-overlapping roots to `filescan.Limiter` with `filescan.SkipSymlinks`.
Nested repository and package-manager symlinks within recognized task roots remain opaque entries.
Only regular files inside those roots contribute bytes. Symlink target data never contributes through the link.

An omitted unclassified directory does not invalidate successfully measured workspace totals.
`Analysis.Warnings` identifies the preserved path. Existing API fields carry these diagnostics without a new response shape.
This warning does not claim that unknown host data belongs to the Task workspaces category.

`storageOverview` returns the successful workspace result through its existing projection.
`workspaceResource` then shows measured GB rather than Unavailable.
The existing desktop and phone cards use the same response. No component, copy, navigation, or composition change is required.
This package does not add a new rendered warning surface for the existing `warnings` array.

Manual Analyze replaces a previously cached unavailable result through the existing refresh path.
The scan cache, progress lifecycle, and refresh interval remain unchanged.

## Failure and safety

All consumers share discovery. Analysis cannot silently use broader roots than cleanup.
Every cleanup candidate still requires complete inventory, absence from every protection source, and expiry of the grace period.
One live descendant protects its full task root. An inventory error never becomes an empty authoritative inventory.

The recognition rule does not grant ownership through a symlink.
Symlinked tasks roots, owned ancestors, recognized scratch-root paths, trash paths, ownership markers, and quarantine manifests retain rejection.
Unknown paths never enter orphan quarantine or dependency pruning.
Recognized roots still use existing atomic quarantine and deletion primitives.

Real I/O errors, unsafe control paths, invalid ownership markers, and caller cancellation retain their existing failure behavior.
This correction does not introduce a catch-all rule that converts discovery errors into warnings.
Missing measurement bytes never become measured zero.

## Compatibility and verification

Tests must combine a valid task root with an unrelated checkout that contains `CLAUDE.md -> AGENTS.md` and ordinary repository folders.
The analysis must return exactly the recognized root bytes. Cleanup must preserve the unrelated checkout and all external link targets.
The same fixture without the symlink must remain unclassified, preventing a link-only workaround.

Compatibility tests cover marked semantic and scratch roots, unmarked semantic roots, and unmarked canonical UUID scratch roots.
Mixed scratch containers include a valid task plus an unrelated child. Only the task participates.
Existing synthetic legacy fixtures must use the actual UUID layout for unmarked scratch paths.
Marked fixtures retain their existing names.

Real-backend desktop and phone E2E tests seed this failure condition beneath the isolated backend home.
They exercise Analyze, inspect the actual API bytes, and expand the existing workspace row.
They must not mock the storage overview response.

## Related decisions

- [Discovery evidence](../../../decisions/2026-10-05-workspace-storage-discovery.md).
- [Opaque nested symlinks](../../../decisions/2026-07-19-workspace-symlink-entries.md).
- [Fail-closed cleanup](../../../decisions/0009-fail-closed-gc-semantics.md).

## Implementation plan

[Workspace storage discovery fix package](../../../plans/workspace-storage-discovery/plan.md).
