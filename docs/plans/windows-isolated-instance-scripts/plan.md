---
created: 2026-09-28
status: done
requirements:
  - REQ-LAUNCHER-ISOLATED-SCRIPTS-001
system_design:
  - ../../specs/launcher/system-design/isolated-scripts.md
legacy_specs: []
---

# Implementation plan: Windows isolated-instance scripts

## Overview

Add the Windows PowerShell equivalent of the Unix isolated-instance helpers so a
contributor can run and tear down a throwaway instance without touching the live
installation. The launcher system owns the package because it owns port
selection, managed-process startup, and shutdown.

## Scope

In scope: the three PowerShell helpers, a fail-closed home guard, loopback
binding by default, exact teardown, and the public Windows support note.

Out of scope: changing the Unix helpers, the native launcher, backend routes, or
production data; remote-executor or desktop behavior.

## Technical approach

Mirror the Unix helper flow in `scripts/dev-isolated.ps1`,
`scripts/kandev-kill.ps1`, and `scripts/kandev-instances.ps1`. Reuse the native
launcher binaries and the `dev` profile via `make -C apps/backend build`. Keep
the numeric backend pidfile and record process start times and the isolated home
in sidecars. Launch the backend and Vite through the existing native `winjob.exe` helper,
which assigns each suspended child to a Job Object before resuming it. Record
only the exact child identity published by that supervisor and verify its
executable and listener rows. Retain supervisor handles for complete startup
failure cleanup; verify recorded process identity during normal teardown.
Default the Vite host to `127.0.0.1` and refuse a resolved home that is a
live-state boundary before any write.

## Verification

| Criteria | Evidence |
| --- | --- |
| `AC-LAUNCHER-ISOLATED-SCRIPTS-001.1` | Guarded-port refusal and free-port selection in `scripts/dev-isolated.ps1` |
| `AC-LAUNCHER-ISOLATED-SCRIPTS-001.2` | Unique default home, isolated `KANDEV_HOME_DIR`, `HOME`, `USERPROFILE`, and fresh `data` database |
| `AC-LAUNCHER-ISOLATED-SCRIPTS-001.3` | `Resolve-SafeIsolatedHome` refuses live-home descendants, reparse aliases, drive/workspace boundaries before any write |
| `AC-LAUNCHER-ISOLATED-SCRIPTS-001.4` | `-WebHost` defaults to `127.0.0.1`; the Go backend proxies to the selected Vite host |
| `AC-LAUNCHER-ISOLATED-SCRIPTS-001.5` | Supervisor-published exact Vite listener identity, start-time verification, and full-tree teardown after backend exit |
| `AC-LAUNCHER-ISOLATED-SCRIPTS-001.6` | `kandev-kill.ps1` guarded-port refusal unless `-Force` |
| `AC-LAUNCHER-ISOLATED-SCRIPTS-001.7` | Recorded home and pipeline-friendly `-Raw` listing |
| `AC-LAUNCHER-ISOLATED-SCRIPTS-001.8` | Backend and Vite child environments are allowlisted; backend config discovery is disabled |
| `AC-LAUNCHER-ISOLATED-SCRIPTS-001.9` | Focused PowerShell safety tests run on the Windows CI runner |

Run from the repository root:

```bash
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Hosted Windows runs the existing home/environment guards, native supervisor
tests, and `scripts/tests/isolated-launch.Tests.ps1`. The launch suite exercises
real executable fixtures and sockets, with positive controls for both listeners,
caller-pipe EOF, concurrency, cleanup, literal paths, and native build stderr.
Backend path-filter contract tests keep PowerShell inputs selected in future CI.

## Work orders

- [x] [Task 01: Windows isolated-instance scripts](task-01-windows-isolated-scripts.md)
- [x] [Task 02: Detach launched children from the caller](task-02-detach-launched-children.md)

## Risks

- The helper depends on Windows-only cmdlets, which is acceptable for the
  Windows mirror of the Unix helpers.
- A stale pidfile is rejected rather than used to stop an unrelated process.

## References

- [Requirements](../../specs/launcher/requirements/isolated-scripts.md)
- [System design](../../specs/launcher/system-design/isolated-scripts.md)
