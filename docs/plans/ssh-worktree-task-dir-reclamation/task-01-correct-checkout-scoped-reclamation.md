---
id: "01-correct-checkout-scoped-reclamation"
title: "Correct checkout-scoped SSH reclamation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-SSH-TASKDIR-RECLAMATION-001
  - REQ-SSH-TASKDIR-RECLAMATION-002
  - REQ-SSH-TASKDIR-RECLAMATION-003
  - REQ-SSH-TASKDIR-RECLAMATION-004
  - REQ-SSH-TASKDIR-RECLAMATION-005
  - REQ-SSH-TASKDIR-RECLAMATION-006
  - REQ-SSH-TASKDIR-RECLAMATION-007
acceptance_criteria:
  - AC-SSH-TASKDIR-RECLAMATION-001.1
  - AC-SSH-TASKDIR-RECLAMATION-001.2
  - AC-SSH-TASKDIR-RECLAMATION-001.5
  - AC-SSH-TASKDIR-RECLAMATION-002.1
  - AC-SSH-TASKDIR-RECLAMATION-002.2
  - AC-SSH-TASKDIR-RECLAMATION-002.3
  - AC-SSH-TASKDIR-RECLAMATION-002.4
  - AC-SSH-TASKDIR-RECLAMATION-002.5
  - AC-SSH-TASKDIR-RECLAMATION-002.6
  - AC-SSH-TASKDIR-RECLAMATION-002.7
  - AC-SSH-TASKDIR-RECLAMATION-002.8
  - AC-SSH-TASKDIR-RECLAMATION-002.9
  - AC-SSH-TASKDIR-RECLAMATION-002.10
  - AC-SSH-TASKDIR-RECLAMATION-002.11
  - AC-SSH-TASKDIR-RECLAMATION-002.12
  - AC-SSH-TASKDIR-RECLAMATION-002.13
  - AC-SSH-TASKDIR-RECLAMATION-003.4
  - AC-SSH-TASKDIR-RECLAMATION-004.1
  - AC-SSH-TASKDIR-RECLAMATION-004.2
  - AC-SSH-TASKDIR-RECLAMATION-004.3
  - AC-SSH-TASKDIR-RECLAMATION-005.1
  - AC-SSH-TASKDIR-RECLAMATION-005.2
  - AC-SSH-TASKDIR-RECLAMATION-005.3
  - AC-SSH-TASKDIR-RECLAMATION-005.4
  - AC-SSH-TASKDIR-RECLAMATION-006.2
  - AC-SSH-TASKDIR-RECLAMATION-007.2
system_design:
  - ../../specs/executors/system-design/remote-task-directory-reclamation.md
---

# Task 01: Correct Checkout-Scoped SSH Reclamation

## Summary

Correct ignored-file handling, scope linked-worktree probes to the task, and
remove linked worktrees through Git. Keep the changes in the existing reclaimer
with focused regression coverage.

## In scope

- Read the plan, requirements, system design, ADR, backend guidance, and `/tdd`.
- Add a test-only local `sshRemoteRunner` with isolated Git fixtures.
- Run production commands through `WrapLoginShell("bash", cmd)` and an outer
  shell, matching `sshClientRunner.Run`. Keep `RunDirect` unwrapped.
- Include paths with spaces and apostrophes. Run common-dir removal from an
  unrelated cwd and assert preservation of the non-bare main checkout and sibling.
- First add `TestSSHTaskDirReclaimGitIgnoredFiles` and
  `TestSSHTaskDirReclaimGitLinkedRemoval`.
- Prove their expected failures on the existing ignored-file check and
  stale-registration behavior.
- Add the remaining test targets from the plan's test matrix.
- Recognize standard clones and linked worktrees through Git's directory values.
- Use the design's absolute path-format command and physical path comparisons.
  Cover a symlinked profile root and rejection of unsupported option output.
- Preserve standalone clones with additional worktrees and unsupported layouts.
- Find nested repository entries at any depth and preserve the task directory
  when a checkout is embedded below the supported root or direct-child layout.
- Read worktree inventory for both checkout kinds. Preserve known locks as
  safety skips with `shared` and a lock-specific detail.
- Preserve every checkout with registered submodules as an unsupported-layout
  skip, including initialized and uninitialized submodules.
- Cover a published shallow standalone checkout and a child-repositories-only root.
- Apply the design's content checks and removal commands with small helpers in
  the current file. One unsafe checkout must veto the task before mutation.
- Replace the old ignored-file preservation expectation with the revised contract.
- Keep exact stash attribution, unknown-state preservation, standalone protection,
  cancellation, and quoted command arguments.
- Let Git own registration removal. Assert the resulting inventory in tests.
- Extend mutating-command assertions to cover Git worktree removal as well as `rm`.
- Retain the existing public runtime result and cleanup-job integration contracts.

## Out of scope

- Public documentation, owned by task 02.
- New profile settings, migrations, runtime flags, or persisted topology evidence.
- A topology framework, repeated full-probe pass, captured-identity snapshots,
  or a second inventory verification in production.
- Fetch, push, broad prune, stash/branch removal, forced removal, or live-host tests.
- Commit, push, or PR creation.

## Acceptance

1. Every applicable case in the plan's test matrix passes through the production
   probe or `Reclaim` path. The initial regressions fail against the old code.
2. Unknown evidence preserves the task. Unsafe children prevent all removal.
   Standalone branches and stashes remain protected, while unrelated retained
   repository activity permits reclamation. Known locks and submodules produce
   safety skips before mutation, rather than removal errors and job retries.
3. Successful linked removal leaves no target registration and preserves distinct
   sibling content and shared refs. Failed Git removal never permits a recursive
   fallback. Existing cancellation and path guards remain effective.

## Verification

Run from the repository root. The Make `test` target cannot filter packages,
so these commands use the Go runner directly with the repository's build flags.

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/agent/runtime/lifecycle -run '^Test(SSHTaskDirReclaim|SSHReclaim|WrapLoginShell|SSHStopInstanceRunsTerminalCleanupBeforeControllerTeardown)' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/agent/runtime -run '^TestSSHTaskDirReclaimer' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/service -run '^(TestSSHReclaim.*|TestCancelIfTaskUnarchivedCancelsPendingJobWithoutClaim)$' -count=1)
bash scripts/lint-go-changed apps/backend/internal/agent/runtime/lifecycle/executor_ssh_reclaim.go apps/backend/internal/agent/runtime/lifecycle/executor_ssh_reclaim_test.go apps/backend/internal/agent/runtime/lifecycle/executor_ssh_reclaim_git_test.go apps/backend/internal/agent/runtime/lifecycle/executor_ssh_reclaim_optin_test.go
git diff --check
```

The lint list includes the opt-in test suite in case its fixtures change.
If another Go file changes, add it to this command and record the exact final
command in Results before marking the work order complete.

## Files likely touched

Owned implementation and test files:

- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_reclaim.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_reclaim_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_reclaim_git_test.go` (new)
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_reclaim_optin_test.go` (only if fixtures need adjustment)

Read-only regression inputs:

- `apps/backend/internal/agent/runtime/ssh_reclaim.go` and `ssh_reclaim_test.go`
- `apps/backend/internal/task/service/resource_cleanup_ssh.go` and its test file

## Dependencies

None. The primary session updates this work order and the manifest serially.

## Risks

All filesystem mutations must remain inside fixture `t.TempDir()` roots.
Keep new test files separate from the existing large suite to respect file limits.
Git descriptions do not provide durable branch ownership. Unknown attribution
must preserve the target rather than assume that the stash belongs elsewhere.

## Parallelism

`sequential`

## Inputs

- Owning requirements: `REQ-SSH-TASKDIR-RECLAMATION-001` through `-007`.
- Owning design: **Safety probe**, **Registration-aware removal**, and **Testing**.
- [Storage-scope decision](../../decisions/2026-11-12-scope-ssh-reclamation-to-deleted-storage.md).
- Existing `fakeSSHRunner`, `cleanCheckout`, and path/refusal test patterns.
- `.agents/skills/tdd/references/backend-tests.md` for subprocess isolation and cleanup.

## Results

Completed on 2026-11-14. Real-Git regressions first reproduced ignored-file
preservation and stale linked-worktree registrations. The implementation now
uses non-ignored status, validates checkout storage and registration, scopes
linked-worktree commits and stashes to the task, preserves standalone repository
protection, and removes linked worktrees through Git. Lock and submodule layouts
skip before mutation. Git tests verify child-first removal and sibling storage
preservation.

Results:

- `(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/agent/runtime/lifecycle -run '^Test(SSHTaskDirReclaim|SSHReclaim|WrapLoginShell|SSHStopInstanceRunsTerminalCleanupBeforeControllerTeardown|IsTaskDirectoryCheckout)' -count=1)` — passed after the final read-only probe and path-helper changes.
- `(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/agent/runtime -run '^TestSSHTaskDirReclaimer' -count=1)` — passed.
- `(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/service -run '^(TestSSHReclaim.*|TestCancelIfTaskUnarchivedCancelsPendingJobWithoutClaim)$' -count=1)` — passed.
- `(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/runtime/lifecycle -run '^(TestSSHTaskDirReclaimProbeIsReadOnly|TestIsTaskDirectoryCheckout)$' -count=1)` — passed.
- `bash scripts/lint-go-changed apps/backend/internal/agent/runtime/lifecycle/executor_ssh_reclaim.go apps/backend/internal/agent/runtime/lifecycle/executor_ssh_reclaim_test.go apps/backend/internal/agent/runtime/lifecycle/executor_ssh_reclaim_git_test.go apps/backend/internal/agent/runtime/lifecycle/executor_ssh_reclaim_optin_test.go` — passed, 0 issues.
- `make -C apps/backend build` — passed for the backend, agentctl targets, mock-agent, acpdbg, codexdbg, and winjob. macOS targets were left unsigned because codesign/rcodesign is unavailable.
- `git diff --check` — passed.

Code-review follow-up on 2026-11-14 bound the physical deletion target to the
recorded task name, preserved spaces in path-probe output, read full symbolic
branch refs, and treated detached-origin stash descriptions as unknown. New
real-Git regressions first failed against the prior implementation, then passed:

- `(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/runtime/lifecycle -run '^(TestSSHTaskDirReclaimDoesNotFollowTaskDirectorySymlink|TestSSHTaskDirReclaimPreservesTrailingSpaceTaskName|TestSSHTaskDirReclaimGitStashScope|TestSSHTaskDirReclaimGitCheckoutKinds|TestSSHTaskDirReclaimGitFailures)$' -count=1)` — passed.
- `(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/agent/runtime/lifecycle -run '^Test(SSHTaskDirReclaim|SSHReclaim|WrapLoginShell|SSHStopInstanceRunsTerminalCleanupBeforeControllerTeardown|IsTaskDirectoryCheckout)' -count=1)` — passed.
- The changed-Go lint command above and `git diff --check` — passed, 0 issues.

PR fixup on 2026-11-14 made the primary, standalone-clone, and separate-storage
repositories set a local name and email with `user.useConfigOnly=true`. The CI
artifact confirmed author-identity failures in the standalone-clone commit case
and the separate-Git-storage case. Requiring explicit identity reproduced both
failures before the shared fixture configuration supplied it.

- `(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle -run '^TestSSHTaskDirReclaimGit' -count=1)` — passed, 7.850s.
- `bash scripts/lint-go-changed apps/backend/internal/agent/runtime/lifecycle/executor_ssh_reclaim_git_test.go` — passed, 0 issues.
- `git diff --check` — passed.

CodeRabbit review follow-up on 2026-11-14 added a real-Git regression for an
ignored `cache/private` repository with an unpublished commit. It failed before
the fix because reclamation removed the parent. Discovery now scans `.git`
entries at any depth and preserves deeper repositories as unsupported. The
requirements and design record this limit. Public docs now state Git 2.36 as the
complete reclamation minimum and explain that enabled cleanup can remove ignored
files, including secrets.

- `(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/agent/runtime/lifecycle -run '^Test(SSHTaskDirReclaim|SSHReclaim|WrapLoginShell|SSHStopInstanceRunsTerminalCleanupBeforeControllerTeardown|IsTaskDirectoryCheckout)' -count=1)` — passed, 14.03s.
- `python3 scripts/list-docs.py validate` — passed, 451 decisions and 1995 specifications.
- `python3 scripts/lint-spec-files.test.py` — 36 tests passed; `python3 scripts/lint-spec-files.py --all` — passed.
- `node --test scripts/validate-public-docs.test.mjs` — 63 tests passed; `node scripts/validate-public-docs.mjs` — 55 pages validated.
- `(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/agent/runtime/lifecycle -run '^Test(SSHTaskDirReclaim|SSHReclaim|WrapLoginShell|SSHStopInstanceRunsTerminalCleanupBeforeControllerTeardown|IsTaskDirectoryCheckout)' -count=1)` — passed, 8.284s.
- `bash scripts/lint-go-changed apps/backend/internal/agent/runtime/lifecycle/executor_ssh_reclaim.go apps/backend/internal/agent/runtime/lifecycle/executor_ssh_reclaim_test.go apps/backend/internal/agent/runtime/lifecycle/executor_ssh_reclaim_git_test.go apps/backend/internal/agent/runtime/lifecycle/executor_ssh_reclaim_optin_test.go` — passed, 0 issues.
- `make -C apps/backend build` — passed for all backend and helper targets. macOS artifacts remain unsigned because codesign/rcodesign are unavailable.
- The PR-doc coverage preflight against the current PR diff covered both work orders with zero errors.
- `git diff --check` — passed.
