---
spec: docs/specs/tasks/system-design/attach-workspace-sources.md
created: 2026-08-04
status: completed
---

# Implementation Plan: Owned Link Target Mismatch Repair

## Overview

A task's on-disk root under `~/.kandev/tasks/<taskDir>` is named from a title slug plus a **random**
3-character suffix (`SemanticWorktreeName(task.Title, SmallSuffix(3))`) and never includes task
identity. Two distinct tasks whose titles sanitize to the same 20-char slug can therefore resolve to
the same task root and contend over its sibling entries. When one task's Kandev-owned directory-link
entry (e.g. `gsp-aieng-kloud8-helm-dedicated`) already points at a different directory than another
task's durable spec target, `worktree.EnsureOwnedDirectoryLink` compares filesystem identity with
`os.SameFile` and returns `owned link target mismatch`, failing closed with no repair — so the task's
first launch **and every subsequent resume** fail identically (observed on task `61ccfd2c`).

This fix has two parts, in dependency order:

1. **Collision resistance (root cause):** derive the task-root suffix deterministically from the task
   ID so distinct tasks normally resolve to different task roots, while the ownership marker guards
   the residual collision case; the persisted name remains reproducible across launch/resume without
   relying on a stored random value.
2. **Self-heal (defense in depth):** inside a Kandev-owned task root, `EnsureOwnedDirectoryLink`
   repoints an existing Kandev-owned *directory-link* entry on target mismatch instead of failing
   closed forever. A non-link entry still fails closed. This is scoped to the owned task root and does
   **not** change the self-referential-entry behavior inside a user's own repository.

## Confirmed root cause

- Task directory name: `worktree.SemanticWorktreeName(task.Title, worktree.SmallSuffix(3))`
  (`apps/backend/internal/worktree/config.go:452`, suffix `apps/backend/internal/worktree/config.go:364`).
  Set at launch in `apps/backend/internal/orchestrator/executor/executor_execute.go:1465` and on
  resume in `resolveResumeTaskDirName` (`apps/backend/internal/orchestrator/executor/executor_resume.go:1284`).
  The task ID is never incorporated; there is no cross-task uniqueness guarantee.
- The fail-closed mismatch is in `EnsureOwnedDirectoryLink`
  (`apps/backend/internal/worktree/directory_link.go:107-109`), reached from
  `reconcileWorkspaceRepositories` / `reconcileWorkspaceSources`
  (`apps/backend/internal/agent/runtime/lifecycle/workspace_sources_reconcile.go:32,67`), which run on
  every local launch and workspace resume (`apps/backend/internal/agent/runtime/lifecycle/manager_launch.go:1076-1083`).
- No code path removes or repoints a Kandev-owned link on mismatch, so the failure is permanent.

---

## Backend

### Area 1 — Deterministic, task-unique task-root name (Task 01)

- Add a deterministic suffix derived from the task ID in
  `apps/backend/internal/worktree/config.go`: a new exported helper (e.g.
  `TaskDirSuffix(taskID string) string`) returning a short lowercase hash (same
  `branchSuffixAlphabet` alphabet, 6-8 chars) of the task ID. It must be pure and stable for a given
  ID. Keep `SmallSuffix` for existing branch-slug callers.
- Update the two task-root call sites to pass a task-ID-derived suffix instead of `SmallSuffix(3)`:
  - `executor_execute.go:1465` → `worktree.SemanticWorktreeName(task.Title, worktree.TaskDirSuffix(task.ID))`
  - `resolveResumeTaskDirName` fallback (`executor_resume.go:1288`) → same, so the fallback recomputes
    the identical name the initial launch would have used (no drift when the env row was never stamped).
- Do not change `SemanticWorktreeName`'s signature; only what the callers pass as `suffix`.
- Persisted `task_dir_name` reuse on resume is unchanged; the deterministic suffix simply makes the
  fallback reproducible and collision-resistant across tasks.

### Area 2 — Owned-link self-heal on mismatch (Task 02)

- In `apps/backend/internal/worktree/directory_link.go`, change `EnsureOwnedDirectoryLink` so that when
  the existing entry **is** a platform directory link (`isPlatformDirectoryLink`) but `os.SameFile`
  reports a different target, it removes the link and recreates it via `CreateOwnedDirectoryLink`
  (returning `created=true`), rather than returning `owned link target mismatch`. A non-link entry
  still returns the existing `owned link entry already exists` error (never deleted/overwritten).
- Removal is safe here: the entry lives under a Kandev-owned task root (built by `mkdirOwned` through
  real, non-symlink ancestors), and a directory link is a pointer, not content. This is distinct from
  `IsSelfReferentialDirectoryLink` / `warnSelfReferentialEntry`, which stay report-only because they
  concern entries inside the **user's own** repository — leave that path unchanged.
- Keep the doc comment on `EnsureOwnedDirectoryLink` accurate to the new repoint-on-mismatch behavior.

---

## Tests

- **Deterministic suffix is stable and collision-resistant** — `apps/backend/internal/worktree/config_test.go`:
  table test that `TaskDirSuffix(id)` is non-empty, uses only the safe alphabet, is identical across
  repeated calls for the same ID, and differs for the covered different IDs. (Task 01)
- **Same-title tasks normally get different roots** — `config_test.go`: two different task IDs with an
  identical title produce different `SemanticWorktreeName(...)` results for the covered IDs. (Task 01)
- **Regression: self-heal repoints an owned link on mismatch** — `directory_link_test.go`: seed an
  owned link to target A, call `EnsureOwnedDirectoryLink(root, name, B)`, assert no error,
  `created=true`, and that the entry now resolves to B. This test must **fail before** the Task 02
  change and pass after. (Task 02)
- **Non-link entry still fails closed** — `directory_link_test.go`: a real directory/file at
  `root/name` still returns an error and is not deleted. (Task 02)
- **Update existing behavior test** — `TestEnsureOwnedDirectoryLinkRejectsDifferentTarget`
  (`directory_link_test.go:169`) currently asserts fail-closed on a different target; it must be
  updated to assert the new repoint-on-mismatch behavior for a Kandev-owned link. (Task 02)

---

## Verification Results

- Task 01 (`config.go` + executor call sites): `go test ./internal/worktree/... ./internal/orchestrator/executor/...` → all `ok`. New `TestTaskDirSuffix` and `TestSemanticWorktreeNameTaskUnique` failed red (build: `undefined: TaskDirSuffix`) before the helper, pass after.
- Task 02 (`directory_link.go`): `go test ./internal/worktree/... ./internal/agent/runtime/lifecycle/...` → all `ok`. `TestEnsureOwnedDirectoryLinkRepointsOwnedLinkOnMismatch` failed red (`owned link target mismatch: api`) before, passes after; `TestEnsureOwnedDirectoryLinkRejectsNonLinkEntry` passes.
- Changed-file lint: `golangci-lint run ./internal/worktree/... ./internal/orchestrator/executor/... --new-from-rev=<base>` → `0 issues`.
- Task 03 (`ownership-aware repoint`): `go test ./internal/worktree/... ./internal/agent/runtime/lifecycle/... ./internal/backendapp/...` → all `ok`. Added marker-absent, marker-match, and marker-conflict coverage around `EnsureOwnedDirectoryLink`, and threaded the new owner/result contract through lifecycle reconcile and host materialization.
- Task 04 (`rollback-faithful repoint`): `go test ./internal/backendapp/... ./internal/worktree/...` → all `ok`. `TestWorkspaceSourceMaterializer_RestoresRepointedLinkWhenAdoptionFails` proves rollback restores the prior target instead of deleting the pre-existing owned entry.
- Task 05 (`safe atomic replacement`): `go test ./internal/worktree/... ./internal/agent/runtime/lifecycle/... ./internal/backendapp/...` → all `ok`. New traversal-before-repoint, invalid-target-keeps-original-link, and changed-entry rename-guard tests pass. Changed-file lint with base `d6b95500eb3662ecd7fd41f67a5ae4c0ddbfb376`: `golangci-lint run ./internal/worktree/... ./internal/agent/runtime/lifecycle/... ./internal/backendapp/... --new-from-rev=<base> --timeout=5m` → `0 issues`.

---

## PR #2253 review remediation

An automated review of the landed fix (head `5c2873d76`, before the review-fixup commit) raised four
blockers. Findings 1-3 are genuine gaps the two landed tasks did not fully close; Finding 4 was
already corrected by the fixup and needs no further code change. Tasks 03-05 address them.

**Finding 1 — legacy-colliding roots reassigned; repoint does no identity check (Task 03).** Resume
reuses the persisted legacy `TaskDirName`
(`apps/backend/internal/orchestrator/executor/executor_resume.go:1285-1286`), so two roots created
under the old random suffix still collide. `EnsureOwnedDirectoryLink`
(`apps/backend/internal/worktree/directory_link.go:104-133`) receives no task identity and never reads
the ownership marker before repointing, so on a shared legacy root the task that reconciles last can
redirect another task's live entry. The marker
(`WriteOwnershipMarker`/`existingMarkerMatches`/`ReadOwnershipMarker`,
`apps/backend/internal/system/storage/workspaces/marker.go:17-62,79-91,120-122`) already fails closed
on a `TaskID`/`TaskDirName` conflict, but it is written only in `prepareTaskWorktreePath`
(`apps/backend/internal/worktree/manager_lifecycle.go:324-329`) and for scratch workspaces
(`manager_launch.go:418-421`) — **not** on the plain local reconcile path — so the repoint helper must
verify it itself. Reconcile runs at `manager_launch.go:1077-1083` and `manager_execution.go:471-477`,
calling `reconcileWorkspaceSources`/`reconcileWorkspaceRepositories`
(`apps/backend/internal/agent/runtime/lifecycle/workspace_sources_reconcile.go:32,67`), both of which
already hold the identity to thread through.

**Finding 2 — repointing breaks workspace-source rollback (Task 04, most serious).**
`materializeDirectoryLinks` (`apps/backend/internal/backendapp/workspace_source_materializer.go:200-212`)
and `materializeWorktreeSources` (`:569-574`) append an entry to the `created` slice whenever
`wasCreated==true`. Task 02 now returns `created=true` for a **repointed** pre-existing link, and
`rollbackHostWorkspaceMaterialization` (`:229-244`) `os.Remove`s every `created` entry (`:231-233`) on
a later failure (rescan/persist/adopt at `:134,:139,:145`). So a failed submission **deletes** a link
that existed before the attachment instead of restoring its prior target — violating the spec's
atomicity guarantee (`docs/specs/tasks/system-design/attach-workspace-sources.md:40-43`,`:51`,`:62`). The two
reconcile call sites ignore the return values, so the rollback bug is confined to the two
`materialize*` sites.

**Finding 3 — remove-and-recreate not atomic; containment validated after the destructive remove
(Task 05).** In the repoint branch, `removeInspectedDirectoryLink` (`directory_link.go:122`) runs
**before** `CreateOwnedDirectoryLink`'s containment check (`isOwnedDirectoryLinkPath`,
`directory_link.go:15`, reached at `:125`), so a traversal/out-of-root name is rejected only after the
remove. A create failure after the remove leaves the entry missing with no restore. There is no atomic
temp+rename wrapper for directory links (see the file pattern at
`apps/backend/internal/agent/usage/fileutil.go:11-33`); Unix `os.Symlink` is a single atomic syscall,
but a Windows junction cannot be atomically renamed over an existing target
(`directory_link_unix.go`, `directory_link_windows.go`).

**Finding 4 — suffix uniqueness (reviewed, no code change).** The review read the pre-fixup comments.
`config.go:382-407` already states the 8-char base-36 projection of `sha256(taskID)` is
collision-resistant (~36^8 ≈ 2.8e12), not injective, and that residual owned-root contention is caught
by the fail-closed ownership marker; `config_test.go` already asserts the exact suffix length. The
suffix stays intentionally probabilistic; Task 03 is what makes that marker backstop effective on the
reconcile path. No change required.

### Single-signature-change decision

Tasks 03-05 all touch `EnsureOwnedDirectoryLink`. Rather than three overlapping signature edits, Task
03 changes it **once**: it takes an ownership descriptor (`TaskID`, `TaskDirName`) and returns a result
struct (`{Path string; Created bool; PriorTarget string}`). Task 04 consumes `PriorTarget` for
rollback-faithful undo; Task 05 adds the containment-before-remove ordering and the safe/atomic
replacement inside that same function. The four call sites are updated in Task 03; the two reconcile
sites keep ignoring the extra result fields.

### Tests (Tasks 03-05)

- **Marker-mismatch repoint fails closed** — `directory_link_test.go`: seed an owned link under a root
  whose `.kandev-workspace.json` names task A; `EnsureOwnedDirectoryLink` for task B fails closed with
  a marker-conflict error and leaves the entry unchanged. Marker absent and marker-matches-this-task
  both allow the repoint. (Task 03)
- **Rollback restores a repointed link's prior target** — `workspace_source_materializer` test: repoint
  a pre-existing entry (A→B), force a later rescan/persist failure, assert the entry is restored to A
  (not deleted); a genuinely created entry is still deleted. (Task 04)
- **Containment before remove + recreate-failure restore** — `directory_link_test.go`: a traversal /
  out-of-root name is rejected before any removal; a forced recreate failure leaves the original link
  intact; the existing swap-race guard still holds. (Task 05)

## Implementation Waves And Parallel Candidates

Tasks 01-02 landed. Tasks 03-05 all touch `EnsureOwnedDirectoryLink` and its callers and must run
sequentially in dependency order (03 defines the shared signature; 04 and 05 build on it). The default
is sequential execution in the primary conversation; waves do not authorize subagents.

```text
Wave 1:
- [x] [task-01-task-unique-root-name](task-01-task-unique-root-name.md)

Wave 2:
- [x] [task-02-owned-link-self-heal](task-02-owned-link-self-heal.md)

Wave 3:
- [x] [task-03-ownership-aware-repoint](task-03-ownership-aware-repoint.md)

Wave 4:
- [x] [task-04-rollback-faithful-repoint](task-04-rollback-faithful-repoint.md)

Wave 5:
- [x] [task-05-safe-atomic-link-replacement](task-05-safe-atomic-link-replacement.md)
```
