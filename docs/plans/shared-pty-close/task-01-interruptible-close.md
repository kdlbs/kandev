---
id: "01-interruptible-close"
title: "Make shared Unix PTY closure interruptible"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-SHARED-PTY-001
acceptance_criteria:
  - AC-PLATFORM-SHARED-PTY-001.1
  - AC-PLATFORM-SHARED-PTY-001.2
  - AC-PLATFORM-SHARED-PTY-001.3
  - AC-PLATFORM-SHARED-PTY-001.4
system_design:
  - ../../specs/platform/system-design/shared-pty-lifecycle.md
---

# Task 01: Make shared Unix PTY closure interruptible

## Summary

Enable the existing shared PTY's close operation to release a pending read
without waiting for child output or death. Preserve caller process policy and
clean up any child started before a descriptor-setup failure.

## In scope

- Unix descriptor initialization, safe failure cleanup and focused regressions.
- Read/write/resize behavior, concurrent close and child ownership verification.
- Compatibility verification of existing interactive and login/host-shell consumers.

## Out of scope

Embedded shell migration, ConPTY changes, grace-period changes, UI and test selection.

## Acceptance

1. `TestUnixPTYCloseInterruptsRead` fails against current Start while the child
   side remains open, then passes after the correction. Synchronize a live
   reader and prove the slave was not closed to make the test pass. Repeat
   through resize with `TestUnixPTYResizePreservesInterruptibleRead`.
2. Inject an initialization failure through a private argument boundary in
   `TestUnixPTYInitializationFailureReapsChild`. Assert original-error identity,
   closed resources and reaped child. Do not introduce global mutable test hooks.
3. Existing stop, reconnect, retained host-shell, read/write and resize tests
   pass. Real Linux and macOS PTY regressions run; unchanged native Windows CI
   stays required. Failure or missing evidence cannot become a success claim.

## Verification

From repository root, run the focused regression before and after correction.
Then run the following checks, sequentially. Run the first three commands on both
Linux and macOS; record unavailable native execution as pending. The process
command selects the interactive PTY callers, including output, input, resize,
stop, user-shell ownership and first-idle behavior. The full process suite
remains covered by the existing Linux backend workflow.

```bash
(cd apps/backend && go test -trimpath -race ./internal/common/ptyexec -count=10)
(cd apps/backend && go test -trimpath -race ./internal/agent/loginpty ./internal/agentctl/server/shell -count=1)
(cd apps/backend && go test -trimpath -race ./internal/agentctl/server/process -run '^(TestInteractiveRunner_|TestWaitForFirstIdle_|TestEmitTurnComplete_|TestPTYDimension$)' -count=1)
(cd apps/backend && GOOS=windows GOARCH=amd64 go test -c -o .build/ci-half/ptyexec-windows.test.exe ./internal/common/ptyexec)
make -C apps/backend lint
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/common/ptyexec/pty_unix.go`
- `apps/backend/internal/common/ptyexec/pty_close_unix_test.go`
- `.github/workflows/pty-compatibility.yml` for native macOS verification.
- `apps/backend/internal/agent/loginpty/manager_test.go` only if caller evidence needs strengthening.
- `apps/backend/internal/agentctl/server/process/interactive_lifecycle_test.go` only if caller evidence needs strengthening.

## Dependencies

None. Implementation follows the October 13 request to continue reducing E2E time.

## Risks

`StartWithSize` has already launched the child. Failure cleanup must not leak it.
Later `Fd()` use can undo nonblocking mode. Tests must prove native I/O behavior,
not merely successful Close return or an already-dead child.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/platform/requirements/shared-pty-lifecycle.md).
- [Design](../../specs/platform/system-design/shared-pty-lifecycle.md).
- [Plan diagnosis](plan.md#evidence-and-root-cause).

## Results

The close and resize regressions failed at the intended pending-read assertion
on the baseline in three repetitions each. The failure-cleanup regression
failed because the child had not been reaped. With the correction, the complete
PTY package passed `go test -trimpath -race ./internal/common/ptyexec -count=10`.
Login and shell caller suites passed with the race detector. The process suite
failed only `TestProcessRunnerCapturesOutput` and
`TestProcessRunnerStopLogsSignalAttempts`; both fail identically with an overlay
of the unchanged parent PTY implementation. This host's `sh -lc` exits because
its login profile sources a missing `/home/jcfs/.cargo/env`, before either test
fixture runs. No shell profile or subprocess assertion was changed to hide that
baseline limitation. Native hosted caller checks remain required before merge.

Windows PTY compilation and full backend lint passed (zero issues).
Specification lint and action SHA pinning passed. Native macOS execution is
pending in the new focused compatibility workflow. The work order remains open
until native compatibility checks pass.

### Native portability correction

Hosted macOS run 38405876695 failed both pending-read regressions in all ten
repetitions: Read returned EAGAIN before Close. Its dependency path constructs
a blocking `os.NewFile`, which has no Go poller. The new
`TestUnixPTYInitializationRegistersUnpollableFile` reproduces that starting
state with a real descriptor and failed locally before correction. It requires
poller support, original-handle closure and intact I/O through the replacement.

Initialize a close-on-exec duplicate in nonblocking mode before `os.NewFile`,
require poller registration, and close the original only after the replacement
is ready. Failure paths close the duplicate and preserve existing child cleanup.
The new native run and fresh affected E2E execution remain required. No macOS
failure is suppressed, and the test is not skipped by platform.

Portable revision local verification passed: all 42 desktop cases across queue,
transcript and workflow-switch files, plus all four mobile continuity cases,
with zero skips or retries. This combined correctness run overlapped backend
lint and is not a controlled speed comparison. PTY race tests passed ten times;
login/shell suites and all interactive-runner, first-idle and PTY-dimension tests
passed. Full backend lint reports zero issues, Windows compilation passes, and
documentation validation passes. Hosted macOS and final E2E timing remain pending.

Native macOS run 38416314810 passed all ten repetitions of the shared PTY
regressions, the complete login suite and the complete shell suite. Its broad
process invocation failed two managed npm-cache repair tests and timed out in
`TestWorkspaceGitExternalHelperBudgets/environment/per_file`. Those paths do
not call the shared PTY wrapper; this is not evidence that the complete process
suite passes on macOS. The new compatibility workflow now selects all interactive
runner, first-idle, turn-complete and PTY-dimension tests, while existing Linux
backend CI retains the full process suite. This focused native caller run is
still required before completing the work order.
