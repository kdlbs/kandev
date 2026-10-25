---
created: 2026-10-13
status: in_progress
requirements:
  - REQ-PLATFORM-SHARED-PTY-001
  - REQ-PLATFORM-CI-PERFORMANCE-004
  - REQ-PLATFORM-CI-PERFORMANCE-005
system_design:
  - ../../specs/platform/system-design/shared-pty-lifecycle.md
  - ../../specs/platform/system-design/ci-performance.md
legacy_specs: []
---

# Implementation Plan: Interruptible shared PTY closure

## Overview

Make shared Unix terminal Close interrupt pending I/O, then measure the saved
cleanup time with all test and process-lifecycle guarantees preserved. This
package is in implementation following the October 13 request to continue the
E2E reduction. Platform owns this shared resource contract. Existing instance-stop
specifications concern idempotency and port leases, and task-terminal
persistence explicitly excludes PTY lifetime.

## Scope

### In scope

- Shared Unix master descriptor initialization and failed-start cleanup.
- Real PTY, resize, child-reaping and caller regressions.
- Local paired cleanup measurements and subsequent hosted CI attribution.

### Out of scope

- Embedded raw shell PTYs, Windows implementation changes and UI changes.
- New workers, reduced waits, removed resets, weakened assertions or increased retries.
- Terminal ownership, persistence, output-drain or respawn policy changes.

## Evidence and root cause

At `7c63e69947635ab424edbc43ae65ed26a2668908`, 29 queue and transcript tests
passed without skips or retries in 477.909 seconds. Temporary instrumentation
measured 60.525 seconds inside workspace resets, median 2574.86 milliseconds;
all other measured setup operations totaled about 1.95 seconds. Sources were
restored byte-for-byte. Backend traces show interactive shells reaching the
two-second forced-kill fallback and separate embedded-shell cleanup afterward.

The shared Unix wrapper returns creack/pty's blocking master. With a Read
pending and the slave open, Close cannot release the underlying descriptor
until that read finishes. Existing resize tests close the slave before awaiting
the reader, concealing this boundary.

Disposable Go reproductions ran three times. All three blocking-handle cases
failed a 250ms close/read bound; all three nonblocking counterparts returned in
tens of microseconds. Real Bash and Zsh probes with clean startup environments
each ran three times. All six blocked-reader cases required force killing
after 250ms. Ending the reader first or making the master nonblocking allowed
all corresponding children to exit in under 1ms. Every disposable child was
reaped and both temporary Go test files were removed.

Local raw evidence remains in ignored `apps/backend/.build/ci-half/`:
`reset-profile.json`, `reset-profile-timings.json`, `reset-profile-results.json`,
`pty-close-repro.log`, `pty-shell-repro.log`, `repro-pty-close.py` and
`repro-pty-shell.py`. These observations establish causality, not an E2E
percentage reduction. The independent embedded-shell path remains a cost.

## Technical approach

Keep the existing `PtyHandle` abstraction. Configure the Unix master for
interruptible I/O before returning it. Preserve `SyscallConn().Control` resize.
Clean up the newly started child on setup failure before handing ownership to
callers. See the [design compatibility matrix](../../specs/platform/system-design/shared-pty-lifecycle.md#caller-compatibility).

## Tests

Task 01 adds `TestUnixPTYCloseInterruptsRead`,
`TestUnixPTYResizePreservesInterruptibleRead`, and
`TestUnixPTYInitializationFailureReapsChild` in
`apps/backend/internal/common/ptyexec/pty_close_unix_test.go` for criteria .1 through .3.
Existing login/host-shell and interactive-process tests protect criterion .4.
Linux and macOS native execution is required; unavailable native evidence stays
pending. A path-scoped `Shared PTY compatibility` workflow executes the PTY and
caller commands on macOS. Windows compilation does not replace its existing
native CI checks.

## E2E tests

Task 02 compares complete Chromium `chat/message-queue.spec.ts` and
`chat/last-prompt-scroll.spec.ts`, including real lifecycle cleanup, then checks
mobile session continuity. No new artificial browser test is needed for the
descriptor contract. Report before/after identities, outcomes, retries and
fixture timings separately from wall time.

## Work orders

- [ ] [Task 01: Make shared Unix PTY closure interruptible](task-01-interruptible-close.md)
- [ ] [Task 02: Measure cleanup savings with retained coverage](task-02-measure-cleanup.md)

## Verification results

Diagnosis completed as recorded above. The fresh complete-file E2E baseline
passed all 29 cases without retries or skips in 479.519 seconds, with 60.592
seconds in workspace resets (2581.10ms median). Parent PR
#4440 merged as `b706496d5b2e54e1cebc1e13f38b516e041d8ec1` after 46 passing
checks and no failed, pending, or unresolved review gates.

The user clarified that the 50% target applies to E2E, not frontend tests.
Keep the fixed 2159-second longest-normal-E2E reference and 1079.5-second
target, with the same runner and worker budget. Frontend savings do not satisfy
this target. This PTY fix is one measured contribution, not a promise to halve
the entire suite by itself; full E2E workflow elapsed time remains a separate
measure from execution and queue delay.

Both pending-read regressions failed on the unchanged implementation in three
repetitions, then passed with nonblocking initialization. The injected
initialization-failure test failed because the child was not reaped; after
cleanup correction all PTY tests passed ten times with the race detector.
Login and shell caller suites passed. Two non-PTY subprocess cases fail on both
parent and candidate because the local login shell sources a missing Cargo
environment file. Windows compilation and full backend lint passed. Native
Linux/macOS compatibility remains a pre-merge hosted gate.

Three alternating pairs of the initial Linux implementation retained all 29 exact identities and outcomes without
skips or retries. Median complete-file duration fell from 479.358s to 448.100s,
6.52% lower locally. Median per-run reset total fell from 60.592s to 27.934s;
median per-run reset median fell from 2581.10ms to 584.05ms. All temporary
instrumentation was restored byte-for-byte. An interrupted earlier candidate
is excluded. See Task 02 for all six samples. No hosted saving is claimed.

The first hosted macOS run failed because creack/pty constructs its master
with a blocking `os.NewFile` there, leaving the handle outside Go's poller.
Nonblocking flags alone made reads return EAGAIN. A local raw-descriptor
regression reproduced the missing poller before the correction. Initialization
now creates a nonblocking close-on-exec duplicate, registers a new Go handle,
checks deadline support and closes the original. Linux PTY race repetitions
pass; fresh E2E and native compatibility evidence is required for this revised
implementation. The earlier 6.52% local result belongs to the initial Linux
implementation, not an accepted portable revision.

The complete mobile continuity file passed all four cases without skips or
retries. Web typecheck and complete E2E discovery passed (4019 configured
instances across 1393 files). The separate workflow consolidation retained all
18 scenario identities across 13 passing cases, verified through executed
named steps, and reduced its complete-file local time by 14.14%.

Public docs review found no changed controls, settings, or ownership policy.
The implementation restores the existing close contract; no public guide
changes are required.

Design-package validation passed: `list-docs.py validate` (374 decisions and
1533 specifications), all 36 specification-linter tests, full specification
lint, diff whitespace checks and the PR documentation validator for both work
orders against their planned runtime boundary. That earlier design handoff is
complete; these files now track the subsequently authorized implementation.

## Risks

- Calling `Fd()` after initialization can silently restore blocking behavior.
- Initialization failure happens after child creation and must not leak or double-wait it.
- Native PTY hangup behavior differs between operating systems and shells.
- This common wrapper does not cover every terminal in the application.

Portable revision local verification passed: all 42 desktop cases across queue,
transcript and workflow-switch files, plus all four mobile continuity cases,
with zero skips or retries. This combined correctness run overlapped backend
lint and is not a controlled speed comparison. PTY race tests passed ten times;
login/shell suites and all interactive-runner, first-idle and PTY-dimension tests
passed. Full backend lint reports zero issues, Windows compilation passes, and
documentation validation passes. Hosted macOS and final E2E timing remain pending.
