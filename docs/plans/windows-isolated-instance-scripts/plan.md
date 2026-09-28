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
launcher binaries and the `dev` profile via `make -C apps/backend build`. Reuse
the existing pidfile shape; fix teardown by expanding the recorded web process's
descendant tree instead of trusting the single wrapper PID. Default the Vite
host to `127.0.0.1` and refuse a resolved home that is a live-state boundary
before any write.

## Verification

| Criteria | Evidence |
| --- | --- |
| `AC-LAUNCHER-ISOLATED-SCRIPTS-001.1` | Guarded-port refusal and free-port selection in `scripts/dev-isolated.ps1` |
| `AC-LAUNCHER-ISOLATED-SCRIPTS-001.2` | Isolated `KANDEV_HOME_DIR`, `HOME`, `USERPROFILE`, and fresh `data` database |
| `AC-LAUNCHER-ISOLATED-SCRIPTS-001.3` | `Resolve-SafeIsolatedHome` refuses profile/production/drive/workspace boundaries before any write |
| `AC-LAUNCHER-ISOLATED-SCRIPTS-001.4` | `-WebHost` default `127.0.0.1` and `pnpm exec vite --host` |
| `AC-LAUNCHER-ISOLATED-SCRIPTS-001.5` | `Get-DescendantPids` full-tree teardown of the web process |
| `AC-LAUNCHER-ISOLATED-SCRIPTS-001.6` | `kandev-kill.ps1` guarded-port refusal unless `-Force` |

Run from the repository root:

```bash
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

PowerShell parser check for the three scripts, plus a one-off guard test with a
disposable fake profile.

## Work orders

- [x] [Task 01: Windows isolated-instance scripts](task-01-windows-isolated-scripts.md)

## Risks

- The helper depends on Windows-only cmdlets, which is acceptable for the
  Windows mirror of the Unix helpers.
- A descendant walk covers the common case where the web wrapper is still alive;
  an orphaned grandchild with no live ancestor is outside this package.

## References

- [Requirements](../../specs/launcher/requirements/isolated-scripts.md)
- [System design](../../specs/launcher/system-design/isolated-scripts.md)
