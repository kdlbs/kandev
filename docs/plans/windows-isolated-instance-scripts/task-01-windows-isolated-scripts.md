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

- Port selection skips in-use and guarded production ports, and an explicit
  guarded port is refused.
- The instance uses a dedicated isolated home and fresh database.
- A resolved `-HomeDir` that is the real user profile root, the production
  `~/.kandev` home, a drive/filesystem root, or a git workspace root is refused
  before any write.
- The backend and Vite dev server bind `127.0.0.1` by default.
- Teardown terminates the recorded backend and the full web process tree.
- Teardown refuses a guarded production port unless `-Force` is given.

## Verification

Run from the repository root:

```bash
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

PowerShell parser check for the three scripts, plus a one-off guard test using a
disposable fake profile.

## Files likely touched

- `scripts/dev-isolated.ps1`
- `scripts/kandev-kill.ps1`
- `scripts/kandev-instances.ps1`
- `docs/public/windows-support.md`
- `docs/specs/launcher/requirements/isolated-scripts.md`
- `docs/specs/launcher/system-design/isolated-scripts.md`
- `docs/plans/windows-isolated-instance-scripts/plan.md`

## Dependencies

None.

## Risks

- The helper depends on Windows-only cmdlets; acceptable for the Windows mirror.
- Descendant teardown covers the common case where the recorded wrapper is still
  alive.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/launcher/requirements/isolated-scripts.md)
- [System design](../../specs/launcher/system-design/isolated-scripts.md)
- [Plan](plan.md)

## Results

Added the three PowerShell helpers with safe port selection and guarded-port
refusal, a fail-closed `Resolve-SafeIsolatedHome` guard, default `127.0.0.1`
binding with `-WebHost`, and full descendant-tree teardown of the web process.
The guard rejects the real user profile root, the production kandev home, a
drive/filesystem root, and a git workspace root before any write, and was
verified with a disposable fake profile (never a real home).

Verification:

- PowerShell parser: zero errors on all three scripts.
- `node scripts/validate-public-docs.mjs`: passed.
- `python3 scripts/list-docs.py validate`: passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
