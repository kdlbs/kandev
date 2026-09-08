---
id: "01-reclaim-probe-and-removal"
title: "SSH task-directory safety probe and guarded removal"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements: "../../specs/executors/requirements/remote-task-directory-reclamation.md"
acceptance_criteria:
  - AC-SSH-TASKDIR-RECLAMATION-002.1
  - AC-SSH-TASKDIR-RECLAMATION-002.2
  - AC-SSH-TASKDIR-RECLAMATION-002.3
  - AC-SSH-TASKDIR-RECLAMATION-002.4
  - AC-SSH-TASKDIR-RECLAMATION-002.5
  - AC-SSH-TASKDIR-RECLAMATION-002.6
  - AC-SSH-TASKDIR-RECLAMATION-003.4
  - AC-SSH-TASKDIR-RECLAMATION-001.2
system_design: "../../specs/executors/system-design/remote-task-directory-reclamation.md"
---

# Task 01: SSH task-directory safety probe and guarded removal

## Summary

Add `SSHTaskDirReclaimer` to the `lifecycle` package: the read-only safety
probe that decides whether a remote task directory is disposable, the path-shape
guard, and the guarded removal. This task has **no callers** — it is built and
proven in isolation so that a defect in the verdict logic cannot reach a real
host through a partially-wired cleanup job.

## Scope

- New `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_reclaim.go`.
- A `sshRemoteRunner` interface (`Run(ctx, cmd) (stdout, stderr string, err error)`)
  with the production implementation delegating to the existing
  `runSSHCommand`, so tests never open a connection.
- Probe enumeration of the task-directory root plus each direct child directory
  containing a `.git` entry.
- The four-probe verdict table from the system design, returning a typed verdict
  of `safe`, or `skip` with an enumerated reason
  (`dirty_worktree`, `unpushed_commits`, `stash_present`, `not_a_checkout`,
  `probe_failed`).
- `validateSSHReclaimPath(workdirRoot, path)` refusing anything that is not exactly
  one non-empty segment under `<workdir_root>/tasks/`, and refusing a segment
  containing `/`, `.`, or `..`.
- `rm -rf -- <shellQuote(path)>` followed by a post-removal existence check.
- Bounded per-target timeout.

## Exclusions

- No cleanup-job wiring (task 03).
- No profile setting (task 02).
- No fetch, prune, gc, reset, or any other repository mutation.

## Implementation acceptance conditions

1. The verdict function returns `safe` only when every discovered checkout
   reports a clean `git status --porcelain --untracked-files=all --ignored`, `0` from
   `git rev-list --count HEAD --branches --not --remotes`, and no stash entries;
   every other input, including any command error, non-zero exit, timeout, or
   unparseable output, returns a `skip` verdict with its reason.
2. `validateSSHReclaimPath` refuses `<workdir_root>/tasks`, `<workdir_root>/tasks/`,
   an empty segment, `..` traversal, a nested segment, and any absolute path
   outside the root; removal is unreachable without a passing validation.
3. Removal reports an error when `rm` exits non-zero or when the path still
   exists afterwards, and leaves `<workdir_root>/tasks/` and sibling directories
   untouched.

## Verification

```bash
make -C apps/backend test ARGS='-run TestSSHTaskDirReclaim ./internal/agent/runtime/lifecycle/...'
make -C apps/backend lint
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_reclaim.go` (new)
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_reclaim_test.go` (new)

## Dependencies

None.

## Inputs

- System design sections `Safety probe` and `Control flow`.
- `executor_ssh_operations.go` for `runSSHCommand`, `shellQuote`,
  `WrapLoginShell`, and `expandRemoteHome`.
- `manager_lifecycle.go:resolveRetryStartPoint` for the existing precedent that
  an unanswerable containment probe preserves rather than discards local work.

## Risks

A false `safe` verdict is irreversible data loss. Prefer an over-broad skip.

## Safety

Tests drive `sshRemoteRunner` with recorded outputs and never open SSH. The
removal test operates on `t.TempDir()` only. No test references a path under
`/Users/neo/kandev-workspaces/` or any live workspace root.

## Output contract

Set `status` to `done`, tick the box in `plan.md`, and report the verdict API,
the reason enumeration, tests run, and residual risks.
