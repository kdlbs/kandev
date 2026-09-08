---
id: "03-ownership-aware-repoint"
title: "Ownership-aware owned-link repoint"
status: done
wave: 3
depends_on: ["02-owned-link-self-heal"]
plan: "plan.md"
spec: "../../specs/tasks/system-design/attach-workspace-sources.md"
parallelism: sequential
---

# Task 03: Ownership-aware owned-link repoint

Task 02's repoint receives no task identity and never reads the ownership marker before removing and
recreating a Kandev-owned link, so on a shared legacy task root the task that reconciles last can
repoint another task's live workspace entry to its own repository. Thread the current task identity
into the reconcile and materialize paths and only allow a repoint when the owned root's
`.kandev-workspace.json` marker names **this** task; otherwise fail closed and leave the other task's
entry intact.

This is the **single-signature-change** task. Design the new `EnsureOwnedDirectoryLink` contract once
here — an ownership descriptor input plus a result struct — and have Task 04 and Task 05 build on it.

## Acceptance

- `EnsureOwnedDirectoryLink` (`apps/backend/internal/worktree/directory_link.go:92-125`) takes an
  ownership descriptor (`TaskID`, `TaskDirName`) and returns a result struct
  (`{Path string; Created bool; PriorTarget string}`) instead of the current `(entry, created)` pair.
- A repoint proceeds only when `ReadOwnershipMarker(root)`
  (`apps/backend/internal/system/storage/workspaces/marker.go:120-122`) is **absent**, or is present
  and its `TaskID` **and** `TaskDirName` match the descriptor. A present marker naming a different
  task fails closed with a marker-conflict error and neither removes nor repoints the entry. This
  mirrors `existingMarkerMatches` fail-closed-on-mismatch, allow-on-absent semantics
  (`marker.go:79-91`).
- `reconcileWorkspaceSources` / `reconcileWorkspaceRepositories`
  (`apps/backend/internal/agent/runtime/lifecycle/workspace_sources_reconcile.go:16,40`) thread the
  identity they already hold (`workspacePath` = root; the launch/resume req carries `TaskID`,
  `WorkspaceID`, `TaskDirName`) into the `EnsureOwnedDirectoryLink` calls at
  `workspace_sources_reconcile.go:32,67`.
- The two `materialize*` call sites (`materializeDirectoryLinks`
  `apps/backend/internal/backendapp/workspace_source_materializer.go:200-212` and
  `materializeWorktreeSources` `:569-574`) pass the identity already available to
  `materializeHostRuntime` (it receives `taskID`).
- All four call sites compile against the new signature; the two reconcile sites continue to ignore
  the `Created`/`PriorTarget` result fields (Task 04 consumes them).

## Verification

```bash
cd apps/backend
go test ./internal/worktree/... ./internal/agent/runtime/lifecycle/... ./internal/backendapp/...
golangci-lint run ./internal/worktree/... ./internal/agent/runtime/lifecycle/... ./internal/backendapp/... --timeout=5m
```

## Files likely touched

- `apps/backend/internal/worktree/directory_link.go` (ownership descriptor param + result struct;
  read/verify marker before the repoint branch)
- `apps/backend/internal/worktree/directory_link_test.go`:
  - new: marker absent → repoint allowed (`created=true`)
  - new: marker present and names this task → repoint allowed
  - new: marker present and names a **different** task → fails closed with a marker-conflict error,
    entry unchanged (must fail before this change if the guard is missing)
- `apps/backend/internal/agent/runtime/lifecycle/workspace_sources_reconcile.go` (thread identity into
  both calls at `:32,:67`)
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go` (~`:1077-1083`) and
  `manager_execution.go` (~`:471-477`) if the reconcile signatures must widen to carry identity
- `apps/backend/internal/backendapp/workspace_source_materializer.go` (pass identity at the two
  `materialize*` call sites)

## Dependencies

Task 02 (owned-link self-heal) lands first; this task makes that repoint ownership-aware.

## Parallelism

`sequential`. It defines the shared `EnsureOwnedDirectoryLink` signature that Tasks 04 and 05 consume.

## Inputs

- Spec: the failure-mode row and scenario for an ownership-marker-mismatch repoint that fails closed
  and leaves the other task's entry intact.
- Plan: PR #2253 review remediation — Finding 1 and the single-signature-change decision.
- `WriteOwnershipMarker` / `existingMarkerMatches` / `ReadOwnershipMarker`
  (`apps/backend/internal/system/storage/workspaces/marker.go:17-62,79-91,120-122`) and
  `OwnershipMarker` (`.../workspaces/types.go:23-28`). Marker is written only in
  `prepareTaskWorktreePath` (`apps/backend/internal/worktree/manager_lifecycle.go:324-329`) and for
  scratch workspaces (`manager_launch.go:418-421`); a plain local-executor reconcile root may lack
  one, which is why absent → allowed.

## Output contract

Summary, files changed, tests run (including the marker-mismatch fail-closed regression), blockers,
risks, and task/plan status updates in the same conversation. Reconcile **Files likely touched** with
the actual diff before marking done.

## Results

- Added `OwnedDirectoryLinkOwner` and `OwnedDirectoryLinkResult` to `apps/backend/internal/worktree/directory_link.go`, and changed `EnsureOwnedDirectoryLink` to take the owner descriptor and return the result struct in one signature update.
- The repoint branch now reads `.kandev-workspace.json` via `ReadOwnershipMarker(root)` and allows repoint only when the marker is absent or matches `TaskID` plus `TaskDirName`; a mismatched marker fails closed with `workspace ownership marker conflicts with requested task root` and leaves the existing entry untouched.
- Threaded the new contract through all four callers:
  - `apps/backend/internal/agent/runtime/lifecycle/workspace_sources_reconcile.go`
  - `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
  - `apps/backend/internal/agent/runtime/lifecycle/manager_execution.go`
  - `apps/backend/internal/backendapp/workspace_source_materializer.go`
- Tests:
  - `TestEnsureOwnedDirectoryLinkRepointsOwnedLinkOnMismatch` now covers the marker-absent allow path.
  - Added `TestEnsureOwnedDirectoryLinkRepointsOwnedLinkWithMatchingMarker`.
  - Added `TestEnsureOwnedDirectoryLinkRejectsMarkerConflictOnMismatch`.
  - Updated lifecycle reconcile tests to compile against the new owner-aware signatures.
- Commands:
  - `cd apps/backend && go test ./internal/worktree/... ./internal/agent/runtime/lifecycle/... ./internal/backendapp/...` → all `ok`.
- External side-effect boundary: marker reads plus owned-link repoint/create operations strictly under the Kandev-owned task root; no user-owned repository entries are deleted or overwritten.
