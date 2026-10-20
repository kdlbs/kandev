---
id: "02-detach-launched-children"
title: "Detach launched children from the caller"
status: done
wave: 2
depends_on:
  - "01-windows-isolated-scripts"
plan: "plan.md"
requirements:
  - REQ-LAUNCHER-ISOLATED-SCRIPTS-001
acceptance_criteria:
  - AC-LAUNCHER-ISOLATED-SCRIPTS-001.5
system_design:
  - ../../specs/launcher/system-design/isolated-scripts.md
---

# Task 02: Detach launched children from the caller

## Summary

Keep `scripts/dev-isolated.ps1` from holding the caller's output pipe open after
it returns, while the recorded backend and web PIDs still name the actual
listeners that teardown verifies.

## In scope

- Reuse the native `winjob.exe` helper to launch backend and Vite children with
  file-backed stdout/stderr and NUL stdin. The hidden supervisor has its own
  console and assigns each suspended child to its Job Object before resuming it.
- Discover only the supervisor-published child PID and creation time, with an
  exact executable and parent check across all listener rows. Never adopt a
  compatible unrelated listener started concurrently or left by an earlier run.
- On discovery, health, or metadata-write failure, stop both retained supervisor
  handles so their Job Objects terminate every owned descendant. Create metadata
  without replacing existing files and remove only this launch's records.
- Validate host arguments before launch and pass all paths/arguments directly
  without shell interpretation, including paths containing percent signs,
  brackets, spaces, and other legal Windows filename characters.
- Preserve isolated-home/environment guards, native build stderr diagnostics,
  independent discovery/readiness timeout windows, and CRLF script checkout.
- Add permanent real Windows launch regressions and future CI path selection.

## Out of scope

- Changing the Unix helpers, native launcher modes, or production data.
- Changing normal `kandev-kill.ps1` identity and guarded-port contracts.

## Acceptance

- `dev-isolated.ps1 ... 2>&1 | Select-Object` returns when the script finishes.
- The pidfile and web pidfile record the listening `kandev.exe` and Vite
  `node.exe`, never the `cmd.exe` wrapper, so teardown verifies and stops the
  real processes.
- A leftover listener from an earlier run on the same port is never adopted.

## Verification

Run on Windows from the repository root:

```powershell
powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File scripts\tests\isolated-instance.Tests.ps1
go test -trimpath -race -count=1 ./cmd/winjob # from apps/backend
go build -trimpath -o bin/winjob.exe ./cmd/winjob # from apps/backend
powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File scripts\tests\isolated-launch.Tests.ps1
git diff --check
```

Also run the PowerShell parser check for the three launcher scripts.

## Files likely touched

- `.gitattributes`
- `scripts/dev-isolated.ps1`
- `docs/plans/windows-isolated-instance-scripts/plan.md`
- `docs/specs/launcher/system-design/isolated-scripts.md`

## Dependencies

[Task 01](task-01-windows-isolated-scripts.md).

## Risks

- A backend or Vite listener that is slower than `-Timeout` to bind is reported
  as a discovery failure instead of a health failure.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/launcher/requirements/isolated-scripts.md)
- [System design](../../specs/launcher/system-design/isolated-scripts.md)
- [Plan](plan.md)

## Results

- Permanent hosted Windows RED on the original production implementation:
  the healthy backend/Vite listener and caller-pipe positive control passed;
  a concurrent unrelated backend was adopted; discovery failure left an actual
  fixture grandchild alive; backend metadata-write failure left the backend
  alive; a repository path containing brackets failed in `Start-Process`.
- The first compiler fixture failure was setup-only. Earlier masked ownership,
  cleanup, and host cases are excluded from regression evidence.
- The fixed launcher passed all 19 permanent real Windows launch cases, plus
  native supervisor tests and the existing home/environment guards. Hosted
  workflow and documentation contracts passed. A subsequent test-only AutoRun
  control and Windows binary retention complete the review coverage; final
  exact-head full-suite acceptance remains an external delivery gate. Exact
  revisions, all-attempt raw logs, binary hashes, and final outcomes are
  retained in task evidence, outside tracked delivery documents.
