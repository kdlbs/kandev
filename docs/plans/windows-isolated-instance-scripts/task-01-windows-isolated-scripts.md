---
id: "01-windows-isolated-scripts"
title: "Add Windows isolated-instance scripts"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-LAUNCHER-ISOLATED-SCRIPTS-001
acceptance_criteria:
  - AC-LAUNCHER-ISOLATED-SCRIPTS-001.1
  - AC-LAUNCHER-ISOLATED-SCRIPTS-001.2
  - AC-LAUNCHER-ISOLATED-SCRIPTS-001.3
  - AC-LAUNCHER-ISOLATED-SCRIPTS-001.4
  - AC-LAUNCHER-ISOLATED-SCRIPTS-001.5
  - AC-LAUNCHER-ISOLATED-SCRIPTS-001.6
  - AC-LAUNCHER-ISOLATED-SCRIPTS-001.7
  - AC-LAUNCHER-ISOLATED-SCRIPTS-001.8
  - AC-LAUNCHER-ISOLATED-SCRIPTS-001.9
system_design:
  - ../../specs/launcher/system-design/isolated-scripts.md
---

# Task 01: Windows isolated-instance scripts

## Summary

Add the Windows PowerShell mirror of the Unix isolated-instance helpers: launch
a fully isolated instance on safe ports and a safe home, default the Vite host to
loopback, and tear the instance down exactly without orphaning the Vite listener.

## In scope

- Add `scripts/dev-isolated.ps1`, `scripts/kandev-kill.ps1`, and
  `scripts/kandev-instances.ps1`.
- Select non-colliding ports and refuse guarded production ports.
- Resolve a fail-closed home guard before creating any directory or file.
- Bind the backend and Vite dev server to `127.0.0.1` by default with a
  `-WebHost` opt-in.
- Expand the recorded web process's descendant tree during teardown.
- Document the helpers in `docs/public/windows-support.md`.

## Out of scope

- Changing the Unix helpers, the native launcher, backend routes, or production
  data.
- Remote-executor, desktop, or container behavior.

## Acceptance

- Port selection skips in-use and guarded production ports, and agentctl
  instance ranges do not overlap across automatic launches.
- Each default launch uses a distinct isolated home and fresh database.
- A resolved `-HomeDir` that is the real user profile root, the production
  `~/.kandev` home or any configured KANDEV home, a drive/filesystem root, or
  a git workspace is refused before any write. Paths through junctions are
  refused, including data and application-data paths inside a reused home.
- The backend and Vite dev server bind `127.0.0.1` by default. The Go backend
  remains the browser entry point, and its Vite proxy uses the selected host.
- Backend and Vite child processes receive only an allowlisted environment and
  explicit isolated settings; operator config discovery is disabled.
- Teardown validates recorded process identities and terminates the backend and
  full web process tree even if the backend already exited.
- Teardown refuses a guarded production port unless `-Force` is given.
- Instance listing reports the recorded home and produces pipeline-friendly raw
  rows.
- Windows CI runs focused PowerShell tests for path and environment isolation.

## Verification

Run from the repository root:

```bash
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

PowerShell parser check for the scripts and automated isolation tests in the
Windows CI job. The tests use a disposable fake profile.

## Files likely touched

- `scripts/dev-isolated.ps1`
- `scripts/isolated-instance.psm1`
- `scripts/tests/isolated-instance.Tests.ps1`
- `scripts/kandev-kill.ps1`
- `scripts/kandev-instances.ps1`
- `.github/workflows/backend-tests.yml`
- `docs/public/windows-support.md`
- `docs/specs/launcher/requirements/isolated-scripts.md`
- `docs/specs/launcher/system-design/isolated-scripts.md`
- `docs/plans/windows-isolated-instance-scripts/plan.md`

## Dependencies

None.

## Risks

- The helper depends on Windows-only cmdlets; acceptable for the Windows mirror.
- A missing or stale process identity record prevents automatic termination of
  that process; the operator must inspect it before cleanup.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/launcher/requirements/isolated-scripts.md)
- [System design](../../specs/launcher/system-design/isolated-scripts.md)
- [Plan](plan.md)

## Results

Added the three PowerShell helpers with non-overlapping agentctl port slots,
guarded-port refusal, a unique default home, and fail-closed
`Resolve-SafeIsolatedHome` validation before any write. The scripts refuse to
replace an existing git configuration or copied database. Backend, agentctl,
and Vite bind loopback by default; Vite runs directly so its recorded PID is
the listener. Teardown verifies process start times before stopping the backend
and web trees, including when the backend has already exited. The home guard
was verified with a disposable fake profile (never a real user home).

Initial PR verification:

- PowerShell parser: zero errors on all three scripts.
- `node scripts/validate-public-docs.mjs`: passed.
- `python3 scripts/list-docs.py validate`: passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
- Disposable instances: loopback binding for backend/web/agentctl, scriptable
  instance listing, port-based teardown, and backend-exited web teardown passed.
- A deliberately mismatched test-owned web start-time record was refused; the
  web process stayed live until its correct record was restored and teardown
  completed.

Follow-up remediation:

- Added a dedicated module for safe home resolution and allowlisted child
  environments. The guard rejects paths inside default or configured live
  homes and paths that pass through junctions, including reused-home data and
  application-data paths.
- The backend receives a blank isolated config file. Backend and Vite child
  processes do not inherit arbitrary host variables, and the caller environment
  is restored after each launch.
- The Go backend remains the browser URL. `KANDEV_WEB_INTERNAL_URL` now targets
  the selected Vite host, and the direct Vite API-port override is removed.
- Added PowerShell tests for live-home and junction guards, environment
  allowlisting, explicit settings, and environment restoration. Windows CI runs
  these tests and parses the three launcher scripts. The current Linux workspace
  has no PowerShell runtime, so they could not be executed locally.
