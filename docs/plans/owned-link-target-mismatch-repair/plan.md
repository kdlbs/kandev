---
spec: docs/specs/tasks/attach-workspace-sources.md
created: 2026-08-04
status: complete
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

1. **Uniqueness (root cause):** derive the task-root suffix deterministically from the task ID so two
   distinct tasks never resolve to the same task root, and the persisted name is reproducible across
   launch/resume without relying on a stored random value.
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
  fallback reproducible and cross-task-unique.

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

- **Deterministic suffix is stable and task-unique** — `apps/backend/internal/worktree/config_test.go`:
  table test that `TaskDirSuffix(id)` is non-empty, uses only the safe alphabet, is identical across
  repeated calls for the same ID, and differs for two different IDs. (Task 01)
- **Same-title tasks get different roots** — `config_test.go`: two different task IDs with an identical
  title produce different `SemanticWorktreeName(...)` results. (Task 01)
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

---

## Implementation Waves And Parallel Candidates

Two tasks touch different files (`config.go` + executor call sites vs `directory_link.go`) and could
run in parallel, but Task 01 is the root-cause fix and Task 02 is defense in depth; keep them
sequential by default. The default is sequential execution in the primary conversation; waves do not
authorize subagents.

```
Wave 1:
- [x] [task-01-task-unique-root-name](task-01-task-unique-root-name.md)

Wave 2:
- [x] [task-02-owned-link-self-heal](task-02-owned-link-self-heal.md)
```
