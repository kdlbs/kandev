---
id: "02-owned-link-self-heal"
title: "Self-heal owned directory-link on target mismatch"
status: done
wave: 2
depends_on: ["01-task-unique-root-name"]
plan: "plan.md"
spec: "../../specs/tasks/system-design/attach-workspace-sources.md"
parallelism: sequential
---

# Task 02: Self-heal owned directory-link on target mismatch

Inside a Kandev-owned task root, an existing Kandev-owned directory-link whose target differs from the
current durable spec must be repointed, not treated as a permanent failure. This is defense in depth
so a task that already wedged on a stale entry self-heals on the next launch/resume.

## Acceptance

- `EnsureOwnedDirectoryLink` (`apps/backend/internal/worktree/directory_link.go`) repoints an existing
  entry when it **is** a platform directory link but `os.SameFile(actual, expected)` is false: it
  removes the link and recreates it to the current target, returning `created=true` with no error.
- A non-link entry (real file/directory) at the same path still returns an error and is neither
  deleted nor overwritten.
- The self-referential-entry path (`IsSelfReferentialDirectoryLink` / `warnSelfReferentialEntry`) is
  unchanged — those concern entries inside a user's own repository and stay report-only.
- The `EnsureOwnedDirectoryLink` doc comment reflects the new repoint-on-mismatch behavior.

## Verification

```bash
cd apps/backend
go test ./internal/worktree/... ./internal/agent/runtime/lifecycle/...
golangci-lint run ./internal/worktree/... --timeout=5m
```

## Files likely touched

- `apps/backend/internal/worktree/directory_link.go` (`EnsureOwnedDirectoryLink` mismatch branch)
- `apps/backend/internal/worktree/directory_link_test.go`:
  - new: seed owned link to A, `Ensure(...B)` returns `created=true`, entry now resolves to B
    (must fail before this change, pass after)
  - new: non-link entry still fails closed and is not removed
  - update: `TestEnsureOwnedDirectoryLinkRejectsDifferentTarget` (line 169) to assert the new
    repoint behavior for a Kandev-owned link

## Dependencies

Task 01 (root-cause uniqueness) lands first; this task is defense in depth.

## Parallelism

`sequential`.

## Inputs

- Spec: Failure-modes row for a stale owned entry, and the owned-link self-heal + non-link scenarios.
- Plan: Backend Area 2.
- Existing `EnsureOwnedDirectoryLink` (`directory_link.go:92-117`), `CreateOwnedDirectoryLink`,
  `isPlatformDirectoryLink`, and the report-only `IsSelfReferentialDirectoryLink` contract.

## Output contract

Summary, files changed, tests run (including the red-before/green-after regression), blockers, risks,
and task/plan status updates in the same conversation. Reconcile **Files likely touched** with the
actual diff before marking done.

## Results

- `EnsureOwnedDirectoryLink` (`apps/backend/internal/worktree/directory_link.go`) now repoints an
  existing entry when it **is** a platform directory link but `os.SameFile(actual, expected)` is
  false: it `os.Remove`s the link and recreates it via `CreateOwnedDirectoryLink`, returning
  `created=true`. A non-link entry still returns `owned link entry already exists` and is never
  removed. The doc comment was rewritten to describe repoint-on-mismatch and to scope it to the
  Kandev-owned task root, explicitly contrasting with the report-only self-referential path.
- The self-referential path (`IsSelfReferentialDirectoryLink` / callers) is unchanged.
- Tests in `apps/backend/internal/worktree/directory_link_test.go`:
  - Replaced `TestEnsureOwnedDirectoryLinkRejectsDifferentTarget` with
    `TestEnsureOwnedDirectoryLinkRepointsOwnedLinkOnMismatch` (seed owned link → A, `Ensure(..., B)`
    returns `created=true`, entry now `SameFile` as B and reads B's content). Red before the change
    (`owned link target mismatch: api`), green after.
  - Added `TestEnsureOwnedDirectoryLinkRejectsNonLinkEntry` (real dir at `root/name` still errors and
    its file is untouched). Passed before and after (path unchanged).
- Commands:
  - `go test ./internal/worktree/ -run 'TestEnsureOwnedDirectoryLinkRepointsOwnedLinkOnMismatch|TestEnsureOwnedDirectoryLinkRejectsNonLinkEntry' -v`
    → repoint red before, both `PASS` after.
  - `go test ./internal/worktree/ -run 'TestEnsureOwnedDirectoryLink|TestCreateOwnedDirectoryLink|TestIsSelfReferential' -v`
    → all `PASS`.
  - `go test ./internal/worktree/... ./internal/agent/runtime/lifecycle/...` → all `ok`.
  - `golangci-lint run ./internal/worktree/... ./internal/orchestrator/executor/... --new-from-rev=<base>`
    → `0 issues`.
- External side-effect boundaries: the repoint removes and recreates a directory link (a pointer, not
  content) strictly under a Kandev-owned task root; no user-repository entries are touched.

### PR #2253 fixup (automated-review remediation)

- Serialized both `CreateOwnedDirectoryLink` and `EnsureOwnedDirectoryLink` with the shared
  `acquireWorktreeTargetPath` lock keyed by the owned-link path, so every Kandev writer for the same
  entry now shares one exclusive inspect/create/repoint critical section.
- Under that lock, Unix uses a sibling temp link plus rename-over replacement; Windows still removes and
  recreates the link, but no competing Kandev writer can now swap a new entry into the same path during
  the repair window.
- Extended `TestEnsureOwnedDirectoryLinkRejectsNonLinkEntry` with a regular-file collision case: a file
  at the entry name still fails closed and its bytes survive untouched.
- Follow-on work in Task 03 then threaded task identity into `EnsureOwnedDirectoryLink`, so repoints now
  also fail closed when the workspace ownership marker belongs to a different task root.
- Re-ran `go test ./internal/worktree/... ./internal/orchestrator/executor/... ./internal/agent/runtime/lifecycle/...`
  → all `ok`; changed-file `golangci-lint --new-from-rev=<base>` → `0 issues`.
