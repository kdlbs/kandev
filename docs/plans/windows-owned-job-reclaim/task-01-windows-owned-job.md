---
id: "01-windows-owned-job"
title: "Prove and contain Windows owned process trees"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-AGENT-RUNTIME-AVAILABILITY-002
acceptance_criteria:
  - AC-PLATFORM-AGENT-RUNTIME-AVAILABILITY-002.7
system_design:
  - ../../specs/platform/system-design/agent-runtime-availability.md
---

# Task 01: Prove and contain Windows owned process trees

## Summary

Give the agentctl launcher's kill-on-close job a kernel name, record it in the
process identity, and let Windows `OwnedSessionTerminated` and
`TerminateOwnedSession` use it once the original owner is confirmed exited.

## In scope

- `winproc.InstallNamedKillOnCloseJobForSuspendedProcess` and a named
  `createNamedKillOnCloseJob`; the unnamed entry points delegate to them.
- `processidentity.Identity.WindowsJobName`, `WindowsJobNamePrefix`, and
  `WindowsJobNameEnv`.
- Windows `Inspect` reports `StateExited` when the process has an exit time.
- `processidentity.TakeWindowsJobNameFromEnv`, called first in agentctl's
  `runMain`, takes the job name and unsets it, so agentctl's descendants do not
  inherit it. Windows `Capture(os.Getpid())` records the taken name only after
  `IsProcessInJob` confirms membership.
- Windows `OwnedSessionTerminated`: live owner is not terminated; exited owner
  or a reused PID with an empty or closed owned job is terminated; unknown
  state, missing or foreign job name, or unreadable job fail closed.
- Windows `TerminateOwnedSession` terminates and waits on the owned job after
  confirmed owner exit or PID reuse.
- The job name is read in the same Windows session that created it (`Local\`
  namespace); see the system design.
- The launcher names the job and passes it to the child on Windows only,
  removes an inherited copy elsewhere, and keeps the job name even when capture
  fails (the identity then has no PID or birth token, so cleanup fails closed).
- `KANDEV_INTERNAL_WINDOWS_JOB_NAME` is a startup catalog exclusion (internal
  wiring) and is listed in the public configuration exclusions.

## Out of scope

Linux and macOS containment and every caller of the identity API.

## Verification

- `go test ./internal/common/processidentity/ -run 'TestWindows(ExitedOwner|OwnedSessionWithoutOwnedJob|OwnedSessionMockProcessTable|OwnedSessionAliveIsNotTerminated|ContainmentRejectsUnknownAndLiveProcesses)' -count=1`
  (no child process is started)
- In-process tests without child processes: `TestTakeWindowsJobNameFromEnv*`,
  `TestWindowsCaptureIgnoresJobNameLeftInEnvironment`,
  `TestWindowsReusedOwnerPIDUsesOwnedJobEvidence`, the launcher
  `TestNewWindowsJobNameOnlyOnWindows`,
  `TestWithWindowsJobNameReplacesInheritedValue`, and
  `TestStartedProcessIdentityCaptureFailureKeepsOnlyJobName`, and the config
  catalog tests.
- Process tests that start child processes:
  `TestWindowsOwnedSessionNamedJobProvesTermination` and
  `TestWindowsNamedJobContainsOrphanAfterLeaderExit` (with its helper
  `TestWindowsOwnedJobHelper`).
- `GOOS=linux` and `GOOS=darwin` builds of the changed packages.


## Windows CI package coverage follow-up

Existing process identity and named-job cases are included in the existing Windows native-suite command. The pure name helper already lives in the untagged processidentity.go; no duplicate extraction or new standing case is needed.
The native/process suite split, original package targets and flags, permissions,
runner selection, timeouts and required gate are preserved.
The process-cohort runner targets only agentctl/server/process and therefore
does not supply these separate package cases. Three existing Windows workflow-contract
checks, changed-command selection checks, documentation coverage and whitespace
validation passed. Runtime Go sources/cases are unchanged and were not rerun
in this configuration-only follow-up. Hosted native execution still requires
the upstream workflow's normal approval; it is not claimed as locally executed.
