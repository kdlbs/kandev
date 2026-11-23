---
id: "10-review-fixes"
title: "Fix the local review findings for Windows hosts"
status: done
wave: 9
depends_on:
  - "07-windows-task-terminal"
  - "08-windows-managed-process-resume"
  - "09-windows-shim-sweep-and-port-retry"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-WINDOWS-001
  - REQ-EXECUTORS-SSH-WINDOWS-002
  - REQ-EXECUTORS-REPOSITORY-BRANCH-002
acceptance_criteria:
  - AC-EXECUTORS-SSH-WINDOWS-001.4
  - AC-EXECUTORS-SSH-WINDOWS-001.7
  - AC-EXECUTORS-SSH-WINDOWS-001.8
  - AC-EXECUTORS-SSH-WINDOWS-001.13
  - AC-EXECUTORS-SSH-WINDOWS-001.14
  - AC-EXECUTORS-REPOSITORY-BRANCH-002.1
  - AC-EXECUTORS-REPOSITORY-BRANCH-002.2
  - AC-EXECUTORS-REPOSITORY-BRANCH-002.4
system_design:
  - ../../specs/executors/system-design/ssh-windows-remote.md
  - ../../specs/executors/system-design/repository-branch-resolution.md
  - ../../specs/release/system-design/compact-runtime-distribution.md
---

# Task 10: Fix the Local Review Findings for Windows Hosts

## Summary

A local review of the branch found five defects where the code does not meet
the requirements, plus smaller gaps. Fix them so the Windows path matches the
POSIX behavior that the specifications already require.

## In scope

Defects:

1. `powerShellQuote` doubles all five PowerShell single-quote characters
   (U+0027, U+2018, U+2019, U+201A, U+201B). A value containing `’` no longer
   ends the string, so it can neither break a script nor inject code.
2. The Windows remote-contribution script uses the POSIX branch selection: it
   keeps a current branch that tracks the contribution branch, checks out an
   existing branch without resetting it, and picks a non-colliding suffix
   name. It fails on a checkout error and on a push identity conflict.
3. A selected pull request or branch is materialized strictly on Windows: a
   fetch into `refs/kandev/selected-checkout`, a branch at that commit, and a
   verified `HEAD`, before repository setup. A failure stops preparation.
4. Every Windows stop verifies, in the same script that signals it, that the
   PID is `agentctl.exe` with `--workdir "<task dir>"`. A reused PID survives.
   The orphan stop removes the session directory only when no live controller
   claims the task and the pidfile is absent or still names the stopped PID.
5. `scripts/release/existing-release.mjs` accepts the Windows helper row in the
   runtime size report.

Smaller fixes:

- A PowerShell shell on a Linux or macOS host is rejected before preparation.
- A Windows workdir root that does not expand to a drive or UNC path is
  rejected with an error that names the root.
- An unsupported SFTP chmod is tolerated only for Windows paths.
- Skills for a UNC task directory are written under that directory.
- The agent readiness probe ignores a non-PowerShell shell for a Windows host.
- A `Set-StrictMode` line in the user's `$PROFILE` does not break the loader.
- Release documents list the Windows helper.

## Exclusions

- Platform detection for the orphan-sweep root and the broker preflight.
  Both work on stock hosts today; reordering them is a separate change.
- Locking concurrent helper replacement on one host.
- Backfilling a tag that predates the Windows helper.

## Acceptance conditions

1. Tests cover each defect and fail before the fix.
2. On the test host, a path or Git identity with `’` prepares, a retained
   contribution branch keeps its commits, a selected PR is checked out, and a
   stop of a reused PID leaves that process running.
3. The release verifier tests pass.

## Verification

```bash
cd apps/backend
go test -trimpath -tags fts5 -race ./internal/agent/runtime/lifecycle -run 'Windows|PowerShell|SelectedCheckout|SSHCredential|SSHSkill'
go test -trimpath -race ./internal/ssh
GOOS=windows GOARCH=amd64 go vet -tags fts5 ./internal/agent/runtime/lifecycle
cd ../..
node --test scripts/release/existing-release.test.mjs scripts/release/remote-helper-assets.test.mjs
bash scripts/lint-go-changed <changed Go files>
```

Drive the changed scripts against the test host through a throwaway local
test, as in task 06.

## Likely files and risks

- `executor_ssh_powershell.go`, `executor_ssh_windows_scripts.go`,
  `executor_ssh_winremote.go`, `executor_ssh_windows_guards.go`,
  `executor_ssh_credentials.go`, `executor_ssh_skills.go`,
  `internal/ssh/handlers.go`, `scripts/release/existing-release.mjs`.
- The selected-checkout and contribution scripts are long PowerShell ports.
  Native Windows execution runs only in CI and on the real host, so
  string-level tests alone are not enough.

## Results

Completed on 2026-11-23.

| Finding | Tests | Real host |
| --- | --- | --- |
| Quote characters | `TestSSHWindowsPowerShellQuoteDoublesEveryQuoteCharacter` failed for the four Unicode quotes before the fix | A value with all five quotes and an injection attempt round-trips as data |
| Contribution branch | `TestSSHWindowsRemoteContributionScriptNeverResetsABranch` failed before the fix. Three native tests cover reuse, diverged branches, and push conflicts | Under `powershell` and `pwsh`: a local commit survives a re-prepare; diverged branches stay put while checkout moves to `-kandev-<suffix>-1`; a foreign push URL fails |
| Selected checkout | `TestSSHWindowsPrepareMaterializesSelectedPullRequest` and `...SelectedBranchStrictly` failed before the fix. `TestSSHWindowsExecPreparesSelectedPullRequest` runs natively | Under both shells, in a task path containing `’`: the PR head is checked out before repository setup runs, and a missing PR ref fails preparation |
| Stop identity | `TestSSHWindowsStopGatesKillOnTaskIdentityInOneScript`, `TestSSHWindowsOrphanStopGuardsSessionDir`, and the updated persisted-stop and orphan-sweep tests. `TestSSHWindowsExecStopLeavesReusedPidRunning` runs natively | A tracked stop and an orphan stop both left an unrelated process holding the PID running. The orphan stop kept a session directory whose pidfile names another PID. A real `agentctl.exe` launched for the task was stopped |
| Release verifier | 4 of 16 `existing-release.test.mjs` tests failed before the fix; 24 of 24 pass with `remote-helper-assets.test.mjs` | n/a |
| Smaller fixes | `TestSSHPOSIXLaunchRejectsPowerShellShell`, `TestSSHWindowsTaskDirRejectsNonWindowsWorkdirRoot`, `TestSSHSFTPChmodFailureIsToleratedOnlyForWindowsPaths`, `TestSSHWindowsMaterializeSkillManifestUNCTaskDir`, `TestReadinessShellForRequest`, and `TestSSHWindowsLoaderResetsStrictModeAfterProfile` each failed first | Skills land under a drive task directory. A dot-sourced strict-mode script stops throwing after `Set-StrictMode -Off` |

Commands:

| Command | Result |
| --- | --- |
| `go test -trimpath -tags fts5 -race ./internal/agent/runtime/lifecycle -run 'Windows\|PowerShell\|SelectedCheckout\|SSHCredential\|SSHSkill\|SSHRemoteContribution\|SSHPOSIX\|SSHSFTP\|Orphan'` | Passed |
| `go test -trimpath -race ./internal/ssh` | Passed |
| `go test -trimpath -tags fts5 ./internal/agent/runtime/lifecycle` | 28 failures, all also failing at HEAD on macOS (`/var` symlink and Kubernetes fixtures). None is new |
| `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./internal/agent/runtime/lifecycle ./internal/ssh` | Passed |
| `node --test scripts/release/existing-release.test.mjs scripts/release/remote-helper-assets.test.mjs` | 24 passed |
| `python3 .github/scripts/release-workflow-contract_test.py -k test_agentctl_dependency_guard` | Passed |
| `bash scripts/lint-go-changed <changed Go files>` | 0 issues |

The `TestSSHWindowsExec*` tests skip on macOS. They run in the Windows `native`
CI job, whose filter `^TestSSHWindows` already selects them.

