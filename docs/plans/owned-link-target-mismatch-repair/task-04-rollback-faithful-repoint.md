---
id: "04-rollback-faithful-repoint"
title: "Rollback-faithful owned-link repoint"
status: done
wave: 4
depends_on: ["03-ownership-aware-repoint"]
plan: "plan.md"
spec: "../../specs/tasks/system-design/attach-workspace-sources.md"
parallelism: sequential
---

# Task 04: Rollback-faithful owned-link repoint

Since Task 02, `EnsureOwnedDirectoryLink` returns `created=true` for a **repointed** pre-existing link,
not only a brand-new one. `materializeDirectoryLinks` and `materializeWorktreeSources` append such
entries to the `created []string` slice, and `rollbackHostWorkspaceMaterialization` does
`os.Remove(created[index])` on each. So a later failed multi-source attachment **deletes** a link that
existed before this attachment instead of restoring its prior target — breaking the spec's atomicity
guarantee. Make the materialization undo contract distinguish a repoint from a create and carry the
prior target so rollback restores it.

## Acceptance

- A per-entry undo record type (e.g. `{Path string; PriorTarget string}`) replaces the bare
  `created []string`: `PriorTarget == ""` ⇒ genuinely created-new ⇒ delete on rollback; non-empty ⇒
  repointed ⇒ restore the prior target on rollback.
- `materializeDirectoryLinks`
  (`apps/backend/internal/backendapp/workspace_source_materializer.go:200-212`, record at `:207-208`)
  and `materializeWorktreeSources` (`:569-574`) populate the undo record from the Task 03 result
  struct (`Created`, `PriorTarget`).
- `rollbackHostWorkspaceMaterialization` (`:229-244`, current `os.Remove` at `:231-233`) restores each
  repointed entry to its `PriorTarget` (remove + recreate to the prior target) and deletes only
  genuinely created-new entries.
- The `hostWorkspaceMaterialization` struct (`:66-73`, constructed at `:171-174`) carries the undo
  records in place of `created []string`.
- The two reconcile call sites (`workspace_sources_reconcile.go:32,67`) still ignore the result and
  are not part of the rollback slice — this bug is confined to the two `materialize*` sites.

## Verification

```bash
cd apps/backend
go test ./internal/backendapp/... ./internal/worktree/...
golangci-lint run ./internal/backendapp/... --timeout=5m
```

## Files likely touched

- `apps/backend/internal/backendapp/workspace_source_materializer.go` (undo-record type;
  `hostWorkspaceMaterialization` field; `materializeDirectoryLinks`, `materializeWorktreeSources`,
  `rollbackHostWorkspaceMaterialization`)
- `apps/backend/internal/backendapp/workspace_source_materializer_test.go`:
  - new/updated: repoint a pre-existing link, force a later failure at one of the rollback trigger
    points (`:134`/`:139`/`:145`; deferred rollback `:124-131`), assert the **original** target is
    restored (not deleted) and a genuinely created-new entry is still deleted

## Dependencies

Task 03 (defines the `EnsureOwnedDirectoryLink` result struct with `PriorTarget`).

## Parallelism

`sequential`. Consumes the Task 03 signature.

## Inputs

- Spec: strengthened atomicity behavior (`docs/specs/tasks/system-design/attach-workspace-sources.md:40-43`) and the
  failure-mode row for restoring a repointed pre-existing link on a failed submission. Cancel/intact
  guarantees at `:51` and `:62`.
- Plan: PR #2253 review remediation — Finding 2 (confirmed, most serious).
- Trigger points: rescan/persist/adopt at `workspace_source_materializer.go:134,139,145`; deferred
  rollback at `:124-131`.

## Output contract

Summary, files changed, tests run (including the restore-not-delete regression), blockers, risks, and
task/plan status updates in the same conversation. Reconcile **Files likely touched** with the actual
diff before marking done.

## Results

- Replaced the bare `created []string` rollback tracking with `ownedDirectoryLinkUndo{Path, PriorTarget}` records carried on `hostWorkspaceMaterialization.linkUndo`.
- `materializeDirectoryLinks`, `materializeWorktreeSources`, and `materializeHostRuntime` now preserve `PriorTarget` from the Task 03 result struct, so rollback can distinguish a brand-new link from a repointed pre-existing one.
- `rollbackHostWorkspaceMaterialization` now restores repointed entries to their prior target and deletes only genuinely new links, processing the undo records in reverse order.
- Added `TestWorkspaceSourceMaterializer_RestoresRepointedLinkWhenAdoptionFails`, which seeds a pre-existing owned link, forces adoption failure, and proves the original target is restored rather than deleted.
- PR #2253 fixup: `rollbackOwnedDirectoryLink` now delegates pre-existing entries to `worktree.RestoreOwnedDirectoryLink`, so rollback uses the same platform-safe replacement path as owned-link repair instead of doing a raw remove-then-create.
- Added `TestRestoreOwnedDirectoryLinkKeepsCurrentLinkWhenReplacementTargetIsInvalid`, which proves a failed restore request leaves the current owned link intact.
- Commands:
  - `cd apps/backend && go test ./internal/backendapp/... ./internal/worktree/...` → all `ok`.
- External side-effect boundary: rollback removes or recreates Kandev-owned directory links only under the task root, and restores a prior target only for entries this materialization repointed.
