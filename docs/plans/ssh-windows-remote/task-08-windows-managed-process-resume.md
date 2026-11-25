---
id: "08-windows-managed-process-resume"
title: "Resume managed Windows processes by handle"
status: done
wave: 8
depends_on:
  - "06-docs-copy-and-host-validation"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-WINDOWS-001
acceptance_criteria:
  - AC-EXECUTORS-SSH-WINDOWS-001.22
system_design:
  - ../../specs/executors/system-design/ssh-windows-remote.md
---

# Task 08: Resume Managed Windows Processes by Handle

## Summary

Task 06 found that the first turn of every new Windows session missed its
turn-change start checkpoint. Each managed Git command starts suspended, joins a
job, and resumes. The resume walked a system-wide thread snapshot, which cost
about 50 ms per command on a host with 9,363 threads. The checkpoint's Git
commands then exceeded the backend's request deadline.

Resume through `NtResumeProcess` on a process handle. Keep the thread walk as
the fallback.

## In scope

- `internal/common/winproc/lifecycle_windows.go`: add
  `resumeSuspendedProcessByHandle`, and use it first in
  `ResumeSuspendedProcess`.
- `lifecycle_resume_windows_test.go`: cover both resume paths with a suspended
  `cmd.exe` whose exit code proves it ran.
- Add `./internal/common/winproc` and `^TestResumeSuspendedProcess` to the
  Windows SSH CI step and `make test-windows`.

## Exclusions

- Changing the backend's turn-change request deadline or the checkpoint's Git
  command set.

## Acceptance conditions

1. Both resume paths run a suspended process to completion on Windows.
2. On the test host, the first turn of each new session captures its
   turn-change start checkpoint.

## Verification

```bash
cd apps/backend
GOOS=windows GOARCH=amd64 go test -c -o /tmp/winproc.test.exe ./internal/common/winproc
GOOS=windows GOARCH=amd64 go test -c -o /tmp/subproc.test.exe ./internal/common/subproc
python3 ../../.github/scripts/backend-tests-workflow-contract_test.py
```

Run both binaries on the test host. Then launch new sessions through an
isolated backend and check `turn_change_sets.availability` for each first turn.

## Likely files and risks

- `lifecycle_windows.go`, `lifecycle_resume_windows_test.go`,
  `backend-tests.yml`, `Makefile`.
- `NtResumeProcess` is an undocumented but stable ntdll export. The thread walk
  remains the fallback when the call is missing or fails.

## Results

Completed on 2026-11-23.

On the test host, both resume tests passed, and the same five `subproc` tests
failed with and without this change, so those failures come from the host. A
managed Git command took 73 ms before and 23.5 ms after; an unmanaged start
takes 25 ms. Through an isolated backend, 9 of 9 first-turn change sets were
`unavailable` before the change, and 3 of 3 were `ready` after it.
