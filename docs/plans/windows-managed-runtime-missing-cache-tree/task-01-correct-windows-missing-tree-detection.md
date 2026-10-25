---
id: "01-correct-windows-missing-tree-detection"
title: "Correct Windows missing-tree detection"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-001
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-002
acceptance_criteria:
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.3
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.2
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.4
system_design:
  - ../../specs/agents/system-design/managed-npm-runtime-recovery.md
---

# Task 01: Correct Windows Missing-Tree Detection

## Summary

Treat native Windows not-found status values as an absent managed-runtime
cache tree. Add focused native Windows coverage so the recovery path cannot
return an HTTP 500 for this state.

## In scope

- Add one helper that recognizes Win32 and converted NT status not-found
  errors.
- Use the helper for all three directory opens in the Windows cache remover.
- Remove the separate `_npx` path precheck.
- Add the missing-child regression to the managed-runtime package.
- Run the regression in the Make target and Windows CI job.

## Out of scope

- Changes to lifecycle recovery policy, retry construction, error payloads,
  logging, Unix behavior, or npm metadata storage.
- Changes that convert unrelated cache-repair errors into successful results.

## Acceptance

- If the cache root and `_npx` directory exist but the derived key does not,
  cache repair returns nil on Windows.
- If `NtCreateFile` reports another error, cache repair keeps the current
  fail-closed behavior and error context.
- The repository Windows test target and CI job run the focused regression.

## Verification

Run the new regression on Windows before the production change. Make sure that
it fails with `Object Name not found.`. After the correction, run:

```bash
# From apps/backend on Windows:
rtk go test -race -v ./internal/agent/managedruntime -run '^TestRemoveNpxExecutionTreeTreatsMissingWindowsChildAsAbsent$'
rtk make test-windows

# From apps/backend on any supported development host:
rtk go test ./internal/agent/managedruntime
```

## Files likely touched

- `apps/backend/internal/agent/managedruntime/cache_windows.go`
- `apps/backend/internal/agent/managedruntime/cache_windows_test.go`
- `apps/backend/Makefile`
- `.github/workflows/backend-tests.yml`
- `docs/plans/windows-managed-runtime-missing-cache-tree/plan.md`
- `docs/plans/windows-managed-runtime-missing-cache-tree/task-01-correct-windows-missing-tree-detection.md`

## Dependencies

None.

## Risks

- `NTStatus.Errno()` uses the Windows status-to-DOS mapping. The helper must
  compare only the two existing not-found Win32 values.
- Linux cannot run the native Windows regression.

## Parallelism

`sequential`

## Inputs

- `REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-001` and
  `AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.3`.
- `REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-002`,
  `AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.2`, and
  `AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.4`.
- `docs/specs/agents/system-design/managed-npm-runtime-recovery.md`.
- `docs/decisions/2026-08-24-agentctl-local-managed-runtime-cache-repair.md`.
- Existing Windows handle-open and cache-removal tests.

## Results

- Added `isManagedRuntimeNotFound`. The helper converts `windows.NTStatus`
  values through `Errno()` before it compares the Win32 not-found values.
- Used the helper for the cache root, `_npx` directory, and derived execution
  tree.
- Removed the separate `_npx` `os.Lstat` check.
- Added the missing-child regression and included it in both native Windows
  test entry points.
- The managed-runtime package passed 36 tests on Linux. The Windows test binary
  compiled for `windows/amd64`.
- The native Windows race test remains a CI check because this executor runs
  Linux.
