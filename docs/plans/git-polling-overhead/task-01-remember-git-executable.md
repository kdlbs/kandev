---
id: "01-remember-git-executable"
title: "Remember the resolved Git executable"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-GIT-SUBPROCESS-ADMISSION-002
acceptance_criteria:
  - AC-PLATFORM-GIT-SUBPROCESS-ADMISSION-002.7
  - AC-PLATFORM-GIT-SUBPROCESS-ADMISSION-002.1
system_design:
  - ../../specs/platform/system-design/git-subprocess-execution.md
---

# Task 01: Remember the resolved Git executable

## Summary

Build managed Git commands from a remembered absolute Git path while the
process `PATH` and `PATHEXT` are unchanged and the target remains executable, instead of walking
`PATH` for every command.

## In scope

1. Add source-adjacent tests in `internal/common/subproc` before the change:
   - Under an unchanged `PATH`, a second command reuses the first resolved
     executable even after an earlier `PATH` directory gains a `git`
     candidate. This is the observable form of "no new `PATH` search" and
     fails on the baseline, which picks the new candidate.
   - A changed `PATH` or `PATHEXT` resolves again.
   - Removing the remembered file, replacing it with a directory, or removing
     Unix execute permission resolves again; absent later candidates preserve
     the normal not-found error.
   - `cmd.Args[0]` stays `git`; an unresolvable Git keeps the `git` name and
     the existing not-found error.
   Fixtures are empty placeholder files (`git.exe` on Windows, mode `0755`
   elsewhere); they are resolved, never executed. Each test installs its own
   temporary `PATH`, so no test hook resets the memo.
2. Add a mutex-guarded memo of the resolved path and its `PATH` and `PATHEXT`
   lookup environment. `GitExecutablePath` checks the remembered file with a direct
   `exec.LookPath`, resolves again on a `PATH` or `PATHEXT` change or unusable target, and clears the
   memo when resolution fails.
3. `NewGitCommand` builds the command from the remembered path when available
   and falls back to `git`; `cmd.Args` stays `git` followed by caller arguments.
   Keep the comment that callers validate refs, paths, and options.

## Out of scope

Environment preparation, admission, lifecycle helpers, the raw Git audit, and
any other executable lookup.

## Acceptance

- The reuse test fails on the baseline and passes after the change; the other
  new tests pass on both.
- Existing environment and audit tests still pass.

## Verification

```bash
(cd apps/backend && go test -trimpath ./internal/common/subproc -run '^Test(NewGitCommand|GitExecutablePath)' -count=1)
(cd apps/backend && go test -trimpath ./internal/common/subproc -run '^Test(GitFinalEnvironment|PrepareGitCommandPreservesNilAndDuplicateEnvironment|GitRunnersApplyFinalEnvironment|ProductionGitCommandsUseTheAdmissionSeam|ScanGoSource)' -count=1)
(cd apps/backend && go build ./... && go vet ./internal/common/subproc/...)
```

## Files likely touched

- `apps/backend/internal/common/subproc/git_command.go`
- `apps/backend/internal/common/subproc/git_command_test.go`

## Dependencies

None.

## Risks

Package-level state is shared by every test in the package; tests that change
`PATH` already resolve again because the key changes.

## Results

Implemented on 2026-10-12 against `add376a2af`.

- RED (baseline `git_command.go`, new tests): only
  `TestNewGitCommandReusesResolvedGitUnderUnchangedPath` failed; the baseline
  picked the new earlier candidate. The other three tests passed.
- GREEN: the four `^Test(NewGitCommand|GitExecutablePath)` tests pass.
- Neighbours: `TestPrepareGitCommandPreservesNilAndDuplicateEnvironment`,
  `TestGitSSHCommandPreservation`, `TestGitCredentialHTTPSelectedScope`,
  `TestProductionGitCommandsUseTheAdmissionSeam`, and `TestScanGoSource*` pass.
  `TestGitFinalEnvironment` and `TestGitRunnersApplyFinalEnvironment` use a
  POSIX shell fake `git` and fail on Windows with the baseline file as well;
  they are not affected by this change.
- `go build ./...` and `go vet ./internal/common/subproc/` pass.

Review remediation on 2026-10-13: cached-path validity now uses direct
`exec.LookPath`, and `PATHEXT` joins `PATH` in the lookup environment key.
New regressions failed on the original implementation for permission loss,
directory replacement, and extension-policy invalidation. The exact source
plus tests passed with `go test -trimpath -race -v -count=1` in a nonroot,
network-disabled sandbox without external dependencies. This local proof covers
the isolated executable seam; complete package and Windows validation is hosted.

Independent root review found that the native Windows CI suite previously
compiled this package but selected only helper-console tests. Added an additive
`Test Windows Git executable memo` step and mirrored it in `make test-windows`:
`go test -tags fts5 -race -v -timeout 10m ./internal/common/subproc -run '^TestNewGitCommand'`.
A workflow contract regression requires native-suite placement, the existing
race/build tags and finite budget, selection of every executable-memo test,
and the local Makefile mirror. It failed before the step existed. Actual
Windows execution of these tests, including PATHEXT, must be observed in the
final candidate job log; dependency compilation does not establish coverage.
