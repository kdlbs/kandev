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
  - REQ-EXECUTORS-REPOSITORY-BRANCH-001
  - REQ-EXECUTORS-REPOSITORY-BRANCH-002
acceptance_criteria:
  - AC-EXECUTORS-SSH-WINDOWS-001.4
  - AC-EXECUTORS-SSH-WINDOWS-001.7
  - AC-EXECUTORS-SSH-WINDOWS-001.8
  - AC-EXECUTORS-SSH-WINDOWS-001.13
  - AC-EXECUTORS-SSH-WINDOWS-001.14
  - AC-EXECUTORS-REPOSITORY-BRANCH-001.1
  - AC-EXECUTORS-REPOSITORY-BRANCH-001.4
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
go test -trimpath -race ./internal/ssh ./internal/scriptengine
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
  `internal/ssh/handlers.go`, `internal/scriptengine/repository_branch.go`,
  `scripts/release/existing-release.mjs`.
- The selected-checkout and contribution scripts are long PowerShell ports.
  Native Windows execution runs only in CI and on the real host, so
  string-level tests alone are not enough.

## Results

Completed on 2026-11-23. Each defect has a test that failed before its fix.

| Finding | Tests | Real host |
| --- | --- | --- |
| Quote characters | `TestSSHWindowsPowerShellQuoteDoublesEveryQuoteCharacter` | A value with all five quotes and an injection attempt round-trips as data |
| Contribution branch | `TestSSHWindowsRemoteContributionScriptNeverResetsABranch` and three native tests | Under both shells, a local commit survives a re-prepare and a foreign push URL fails |
| Selected checkout | `TestSSHWindowsPrepareMaterializesSelectedPullRequest`, `...SelectedBranchStrictly`, `TestSSHWindowsExecPreparesSelectedPullRequest` | Under both shells, in a path containing `’`, the pull request head is checked out |
| Stop identity | `TestSSHWindowsStopGatesKillOnTaskIdentityInOneScript`, `TestSSHWindowsOrphanStopGuardsSessionDir`, `TestSSHWindowsExecStopLeavesReusedPidRunning` | A stop of a PID reused by another process leaves it running |
| Release verifier | `existing-release.test.mjs` | n/a |

The full lifecycle package showed only the 28 macOS baseline failures, the
Windows cross-builds passed, and lint reported 0 issues.

Product testing on the test host then found two more defects, both fixed with
tests that failed first:

- A selected pull request failed preparation when the repository had no setup
  script. The credential scrub was then the last statement and left `$?`
  false, so the loader exited 1. The scrub now clears the variables with one
  `Set-Item ... -Value $null`, which keeps `$?` true
  (`TestSSHWindowsPullRequestScrubSucceedsForAbsentVariables`). On the test
  host, a pull request with no setup script then prepared at its head under
  both shells.
- The Windows `repository.branch` placeholder stripped only `refs/heads/`. It
  now uses `scriptengine.RepositoryBranchName`, as the POSIX provider does
  (`TestSSHWindowsRepositoryProviderNormalizesOriginBranch`).

## Simplification (2026-11-25)

A size review before the PRs removed about 400 production and 400 test lines
without changing behavior:

- Windows and POSIX share the identity, pidfile, orphan-inventory, checkout
  verification, script invocation, and placeholder resolution flows. Only the
  host-specific commands differ.
- The Windows identity check matches the quoted `--workdir "<path>"` that the
  launch writes, as the stop script does, and the process queries match only
  `agentctl.exe`. This replaces a Win32 command-line tokenizer.
- The helper upload creates its directory over SFTP and confirms the installed
  file there. Unused wrappers, guards, and the no-op `configureShellProcess`
  hook are gone.
- The tests share one fake Windows SFTP filesystem and one launch harness. The
  launch guard, launch failure, persisted stop, skill path, and resume tests
  are tables. The persisted-stop table now fails when a kill script runs
  against a mismatched identity, and the thread-snapshot resume fallback has
  its own case.

The targeted lifecycle, `ssh`, script engine, and shell tests passed under
`-race`, the Windows cross-builds passed, and lint reported 0 issues.

On the test host, a throwaway harness drove the production executor under
`powershell` and `pwsh` with an `agentctl.exe` built from this change. Each run
created an instance with the default prepare, confirmed the live controller's
identity and rejected another task directory, and found exactly that
controller, not its `cmd.exe` wrapper, in the orphan inventory. It then
reattached the same PID after a backend shutdown, replaced the helper while
the controller ran, left a WMI-started process with a reused PID running,
stopped the controller as an orphan, and archived a relaunch. The `winproc`
resume cases and `TestSSHWindowsShellSessionRunsUnderConPTY` passed there too.
The harness was not committed, and the host was cleaned afterward.
