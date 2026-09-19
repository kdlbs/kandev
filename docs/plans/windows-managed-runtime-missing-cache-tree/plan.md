---
created: 2026-08-27
status: done
requirements:
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-001
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-002
system_design:
  - ../../specs/agents/system-design/managed-npm-runtime-recovery.md
legacy_specs: []
---

# Implementation Plan: Windows Managed Runtime Missing Cache Tree

## Overview

Correct Windows cache repair when the npm `_npx` directory exists but the
derived execution-tree directory does not exist. One work order owns the
native-error classification, the Windows regression test, and Windows CI
coverage.

## Confirmed root cause

`openManagedRuntimeHandle` calls `windows.NtCreateFile`. This function returns
a `windows.NTStatus` for a missing child. The three callers compare this value
directly with Win32 `syscall.Errno` values through `errors.Is`.

`windows.NTStatus` does not implement `Is` or `Unwrap`. As a result, the
comparison fails for `STATUS_OBJECT_NAME_NOT_FOUND` and
`STATUS_OBJECT_PATH_NOT_FOUND`. The cache repair returns an error instead of
treating the missing execution tree as an idempotent result.

The existing `os.Lstat` check only covers a missing `_npx` directory. It does
not cover a missing `_npx/<key>` directory. It also separates the path check
from the handle open.

## Scope

### In scope

- Classify Win32 and NT status not-found errors through one Windows helper.
- Use the helper for the cache-root, `_npx`, and execution-tree opens.
- Remove the separate `_npx` `os.Lstat` check.
- Add a native Windows regression for an existing `_npx` directory with an
  absent execution-tree key.
- Add the focused regression to the repository Windows test target and CI job.

### Out of scope

- Make all cache-repair errors best-effort in the lifecycle manager.
- Change recovery classification, retry count, or command construction.
- Add new agentctl error details or managed-runtime stderr logs.
- Remove cached npm packument metadata or other global npm cache data.
- Change Unix cache deletion.

## Technical approach

Add a small helper in
`apps/backend/internal/agent/managedruntime/cache_windows.go`. The helper will
keep the existing Win32 error comparisons. It will also extract a
`windows.NTStatus` with `errors.As` and convert it with `NTStatus.Errno()`.

Use this helper after each `NtCreateFile`-based directory open. A missing
cache root, `_npx` directory, or exact execution tree will return success. All
other errors will keep their current wrapped error and fail-closed behavior.

Remove the `_npx` `os.Lstat` check. The handle-relative open and shared error
classifier will provide the same idempotent result without a separate path
check.

Add a Windows-only test file that creates the cache root and `_npx` directory.
The test will leave the derived key absent and call the public removal helper.
Before the correction, the test returns `Object Name not found.`. After the
correction, the call returns nil.

Add the focused test to `test-windows` in `apps/backend/Makefile` and to the
Windows backend workflow. This package has Windows-specific code but is not in
the current native Windows test list.

## Tests

- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.3:**
  `TestRemoveNpxExecutionTreeTreatsMissingWindowsChildAsAbsent` proves that an
  absent execution tree does not block the online retry.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.2:** The existing exact-key deletion
  test continues to prove that repair removes only the derived execution tree.
- **AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.4:** Existing symlink and unsafe-spec
  tests continue to prove fail-closed path handling.

## Work orders

- [done] [Task 01: Correct Windows Missing-Tree Detection](task-01-correct-windows-missing-tree-detection.md)

## Verification results

- RED evidence: issue #3092 includes a native Windows probe that returns
  `Object Name not found.` for the absent child. The Linux executor cannot run
  the Windows-only regression before the correction.
- `go test ./internal/agent/managedruntime` passed with 36 tests.
- The Windows managed-runtime test binary compiled for `windows/amd64` with
  `CGO_ENABLED=0`.
- `make -n test-windows` includes the exact managed-runtime regression command.
- `python3 scripts/lint-spec-files.py --all` passed.
- The native Windows race test remains a CI check because this executor runs
  Linux.

## Risks

- The NT status conversion must not classify access, reparse-point, or sharing
  errors as absence.
- The regression needs a native Windows runner. A cross-compiled test binary
  proves compilation only.
- The Make target and CI workflow must use the same focused Windows test.
