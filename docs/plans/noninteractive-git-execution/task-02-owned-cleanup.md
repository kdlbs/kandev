---
id: "02-owned-cleanup"
title: "Bound Git helpers and streaming cleanup"
status: done
wave: 2
depends_on: ["01-prompt-policy"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-GIT-SUBPROCESS-ADMISSION-002
acceptance_criteria:
  - AC-PLATFORM-GIT-SUBPROCESS-ADMISSION-002.3
  - AC-PLATFORM-GIT-SUBPROCESS-ADMISSION-002.4
system_design:
  - ../../specs/platform/system-design/git-subprocess-execution.md
---

# Task 02: Bound Git helpers and streaming cleanup

## Summary

Helper hangs and inherited pipes return within execution plus cleanup bounds; slots and owned resources are released on every terminal path.

## In scope

Add command-owned lifecycle support below agentctl. Preserve existing agents through compatibility wrappers if generic Windows job code moves.
The execution design defines Unix session isolation, Windows suspended-start job attachment, 500 ms pipe waiting, and two-second cleanup limits.
Reject incompatible process attributes before start and preserve stronger compatible cleanup.

Write `TestGitHelperCancellationReleasesSlot` and `TestGitDescendantPipeCleanup` before changing lifecycle execution.
Use helpers which hang, spawn pipe-holding descendants, and exit while descendants retain pipes.
Add a marker proving owned descendants stop and an unrelated control process remains alive.
Cover start failure, cancellation races, normal exit, cleanup failure, and repeated preparation.
Use `TestWindowsManagedGitJobCleanup` on Windows to prove attachment precedes descendant execution.

Migrate all manual AcquireGit/Run paths in `manager_git.go` and streaming `capDiffOutput`.
Write `TestCapDiffOutputCancellationClosesReader` with a pipe-holding descendant before fixing the pre-Wait read/drain.
Keep the full Start-to-Wait lifetime inside one slot. Extend `raw_git_audit_test.go` to detect bypasses and test its rejection fixtures.
Retain the narrow descriptor-based gitinit exception; test its existing ownership contract unchanged.

## Out of scope

Credential-authority changes, transport fallback, live-instance changes, and unrelated refactors.

## Acceptance

- Helper hangs and inherited pipes return within execution plus cleanup bounds; slots and owned resources are released on every terminal path.
- Run new behavioral tests before implementation and record the expected failure, then the passing result.
- Preserve the contracts and exclusions in the linked design.

## Verification

```bash
(cd apps/backend && go test ./internal/common/... ./internal/agentctl/server/winproc ./internal/agentctl/server/process ./internal/worktree ./internal/task/gitinit -count=1 -timeout=5m)
(cd apps/backend && go test -race ./internal/common/subproc -count=1 -timeout=5m)
```

Run the same applicable package tests on native Windows. Skip only Unix PTY assertions; record platform evidence separately.

## Files likely touched

- `apps/backend/internal/common/subproc/shared.go`
- `apps/backend/internal/common/subproc/git_lifecycle*.go (new)`
- `apps/backend/internal/common/subproc/raw_git_audit_test.go`
- `apps/backend/internal/common/winproc/ (generic lifecycle code, if extracted)`
- `apps/backend/internal/agentctl/server/winproc/lifecycle_windows.go`
- `apps/backend/internal/agentctl/server/winproc/lifecycle_other.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_diff.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_diff_test.go`
- `apps/backend/internal/worktree/manager_git.go`
- `apps/backend/internal/task/gitinit/command_descriptor.go`

## Dependencies

01-prompt-policy.

## Risks

Credential helper and process-lifecycle compatibility require real subprocess evidence. Do not infer success from environment assertions alone.

## Parallelism

`sequential`

## Inputs

- [Execution design](../../specs/platform/system-design/git-subprocess-execution.md).
- [Plan evidence and caller inventory](plan.md).
- Scoped AGENTS.md and relevant fix, TDD, and E2E skills.

## Results

Implemented after Task 01 on the verified PR #3635 descendant `9cc146ee21c296d553ae684531219a5c711a4213`.

Results: shared lifecycle code now owns Git Start-to-Wait, Unix session/process-group cleanup, Windows suspended Job Object attachment, pipe closure, and admission release. Manual worktree and streaming diff paths use the shared lifecycle; incompatible terminal and process-group attributes fail before start. The narrow `task/gitinit` descriptor exception remains unchanged.

Checks passed:

- `(cd apps/backend && go test ./internal/common/... ./internal/agentctl/server/winproc ./internal/agentctl/server/process ./internal/worktree ./internal/task/gitinit -count=1 -timeout=10m)`
- `(cd apps/backend && go test -race ./internal/common/subproc -count=1 -timeout=10m)`
- Windows cross-compilation of the shared subprocess, agentctl process, and common Windows lifecycle test binaries.
