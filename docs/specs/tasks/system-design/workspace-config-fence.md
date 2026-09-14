---
status: draft
system: tasks
requirements:
  - REQ-TASKS-RUNTIME-CLEANUP-001
created: 2026-09-06
updated: 2026-09-06
owners:
  - cfl
---
# Workspace Configuration Fence

## Authority

This document owns workspace configuration identity, replacement APIs, claim
state, filesystem verification, and migration. [Archive Cascade Boundary
Contracts](archive-cascade-boundary-contracts.md) owns workspace deletion and
aftermath orchestration. [Archive Cascade Execution
Contracts](archive-cascade-execution-contracts.md) owns database and physical
lock ordering.

## Capability and retained claim

`internal/workspaceconfig` owns the cycle-free capability API and retained
`workspace_config_path_claims(canonical_path_hash, workspace_id,
config_generation, state, deletion_operation_id, marker_key, marker_hash)`.
`state` is `active|deleting|quarantined|released|blocked_legacy`.

```
ResolveActive(ctx, WorkspaceID) (WorkspaceConfigFence, error)
ResolveDeleting(ctx, WorkspaceID, DeletionOperationID) (WorkspaceConfigFence, error)
ResolvePathEvent(ctx, CanonicalPathHash) (WorkspaceConfigFence, error)
Acquire(ctx, WorkspaceConfigFence, AccessMode) (VerifiedConfigRoot, error)
```

`WorkspaceConfigFence` has no public constructor and exposes read-only workspace
ID/name, canonical path, generation, marker identity, claim state, and optional
deletion operation. `AccessMode` is `read|write|git|delete`; only `active`
permits the first three and only exact `deleting` operation permits delete.
Missing/released/wrong-generation/wrong-operation returns not-found/conflict;
`blocked_legacy` returns `config_claim_blocked`. `ResolvePathEvent` maps a
filesystem event by canonical path hash under the claim lock; unknown, deleting,
or ambiguous paths are ignored and diagnosed, never name-resolved.

`Acquire` reloads claim state/generation, takes the canonical physical guard,
verifies the protected HMAC marker, opens the root without following links, and
rechecks filesystem identity. `VerifiedConfigRoot` owns the handle/guard, gives
private helpers a canonical path, provides `Recheck` before/after Git subprocess
I/O, and must close. Cache entries are keyed only by `(workspace_id,
config_generation)` and evicted on claim transition or failed verification.

## Exact replacement surface

| Existing surface | Required replacement |
| --- | --- |
| `ConfigLoader.Load` | `Load(ctx, []WorkspaceConfigFence)`; DB supplies active fences, unclaimed directories are diagnostic-only |
| `GetWorkspaces`, `GetErrors` | `GetWorkspaces/GetErrors(ctx, []WorkspaceConfigFence)` filtered by ID/generation |
| `GetWorkspace`, `GetAgents`, `GetSkills`, `GetProjects`, `GetRoutines`, `Reload` | same name with `(ctx, WorkspaceConfigFence, ...)` |
| `WorkspaceNameFromPath` | removed; watcher uses `ResolvePathEvent` |
| `ConfigLoader.BasePath` | removed; protected base root stays private to composition |
| `loadWorkspaceLocked`, `loadAgentsLocked`, `loadSkillsLocked`, `loadProjectsLocked`, `loadRoutinesLocked`, `recordErrorLocked` | accept workspace ID/generation and `VerifiedConfigRoot`; no name/path derivation |
| `FileWriter.WorkspacePath`, `memoryPath` | unexported helpers accept `VerifiedConfigRoot`, never name |
| `WriteAgent`, `DeleteAgent`, `WriteSkill`, `DeleteSkill` | `(ctx, WorkspaceConfigFence, entity...)` |
| `WriteProject`, `DeleteProject`, `WriteRoutine`, `DeleteRoutine` | `(ctx, WorkspaceConfigFence, entity...)` |
| `WriteRawSettings` | `(ctx, WorkspaceConfigFence, data)` |
| `WriteMemoryEntry`, `ReadMemoryEntry`, `ListMemoryEntries`, `DeleteMemoryEntry` | `(ctx, WorkspaceConfigFence, agent, layer, key...)` |
| `FileWriter.DeleteWorkspace` | removed; cleanup worker uses `DeleteWorkspaceConfig(ctx, deletionFence)` |
| `IsGitWorkspace`, `CloneWorkspace`, `PullWorkspace`, `PushWorkspace`, `GetWorkspaceGitStatus` | `(ctx, WorkspaceConfigFence, operation args...)`; status/is-git return typed error |
| Office config export/import/sync and config-sync poller | accept workspace ID, call `ResolveActive`, propagate fence |
| `defaultWorkspaceName` and name settings bridge | removed; default resolves stored workspace ID then active fence |
| `workspaceSettingsProviderAdapter` and backend settings adapter | accept workspace ID/fence and propagate it to every loader/writer |

## Claim lifecycle

Create/rename reserves canonical path and writes its marker in one fenced
workflow: absent/released to active with monotonically increasing generation.
Normal callers resolve once per operation, and `Acquire` reloads before each
filesystem access. Deletion atomically changes active to
`deleting(operation_id)` and invalidates cache before queuing cleanup. Cleanup
acquires delete access, verifies, and atomically renames the exact tree to managed
quarantine; transaction records `quarantined`, after which a new generation may
claim the original path. Old cleanup continues only on the operation-specific
quarantine path, then records released. It never re-resolves by name.

Migration creates active claims/markers only for unambiguous DB workspace/path
pairs. Duplicate ownership, an unclaimed directory, or marker mismatch becomes
`blocked_legacy`; no public read/write/Git/delete I/O occurs. The config-surface
registry is the checked-in [Archive Cascade Registries](archive-cascade-registries.md)
artifact `apps/backend/internal/workspaceconfig/registry.go`; it assigns every
existing caller to a replacement row and compile-time/AST checks reject
string-workspace or raw-path APIs.

## Verification ownership

Work order 02 implements the resolver, capability, exact cutover, marker/guard,
claim lifecycle, cache, path-event behavior, and migration. Work order 04 verifies
production composition and independently proves no old entry point remains.
Tests race stale delete and filesystem events against same-name create, rename,
generation/state change, marker/path replacement, and legacy ambiguity. Every
replacement method is exercised; rejected access performs no filesystem or
subprocess I/O.
