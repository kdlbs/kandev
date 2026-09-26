---
id: "01-default-desktop-dev-state"
title: "Default desktop dev to repo-local state"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-DESKTOP-DEV-ENV-001
acceptance_criteria:
  - AC-DESKTOP-DEV-ENV-001.1
  - AC-DESKTOP-DEV-ENV-001.2
  - AC-DESKTOP-DEV-ENV-001.3
  - AC-DESKTOP-DEV-ENV-001.4
  - AC-DESKTOP-DEV-ENV-001.5
system_design:
  - ../../specs/desktop/system-design/desktop-dev-environment.md
---

# Task 01: Default Desktop Dev to Repo-Local State

## Summary

Make `make desktop-dev` supply the same repo-local development home and dev
profile by default as `make dev`. Prove the values reach the actual backend on
macOS and document the behavior for contributors.

## In scope

- Add target-scoped development environment values to the `desktop-dev`
  recipe in the root `Makefile`.
- Add a focused command test that verifies the launched Tauri environment
  with and without inherited production paths.
- Add contributor guidance for the selected state, logs, profile, rebuild
  behavior, and concurrent `make dev` contention.
- Run a macOS smoke against the real Tauri app and backend.

## Out of scope

- Change Tauri's installed-app environment, updater, or health-token contract.
- Add live product UI or Go backend reload.
- Change the browser `make dev` launcher.

## Acceptance

1. A plain `make desktop-dev` selects the checkout's `.kandev-dev` home,
   `data/kandev.db`, `logs`, and dev profile without manual environment setup.
2. Inherited home and database paths cannot redirect this target, while
   `desktop-build` and installed desktop launches retain their current defaults.
3. A real macOS launch confirms the backend-selected database and profile; a
   concurrent `make dev` produces the existing single-owner failure.

## Verification

From the repository root:

```bash
bash scripts/desktop-dev-env.test.sh
node scripts/validate-public-docs.mjs
git diff --check -- Makefile scripts/desktop-dev-env.test.sh docs/public/contributing.md
```

On a macOS development host with port `38430` free, start the app in one
terminal:

```bash
make desktop-dev
```

In a second terminal from the same checkout, check the selected database and
the dev-only diagnostics endpoint:

```bash
rg -n 'SQLite database selected.*\.kandev-dev/data/kandev\.db' .kandev-dev/logs/backend-logs.log
curl -fsS -o /dev/null http://127.0.0.1:38430/debug/vars
```

After stopping that app, start `make dev` in the first terminal and
`make desktop-dev` in the second. Confirm the second command reports the
existing home-owner failure. Record all macOS observations in this work
order. The implementation may add a macOS-only smoke helper if these checks
cannot be captured reliably with the existing command output.

## Files likely touched

- `Makefile`
- `scripts/desktop-dev-env.test.sh` (new)
- `docs/public/contributing.md`

## Dependencies

None.

## Risks

- A Make-level dry run can pass while the Tauri-to-backend environment handoff
  fails, so the macOS launch observation is required.
- Running another dev instance with the same home causes an expected lock
  failure; do not delete that home to make the smoke pass.

## Parallelism

`sequential`

## Inputs

- [Desktop development requirements](../../specs/desktop/requirements/desktop-dev-environment.md)
- [Desktop development system design](../../specs/desktop/system-design/desktop-dev-environment.md)
- [Platform dev launcher requirements](../../specs/platform/requirements/go-dev-launcher.md)
- `Makefile` target definitions and `apps/desktop/src-tauri/src/backend.rs` environment handoff.

## Results

Implemented the `desktop-dev` environment defaults and contributor guidance.
The targeted child-environment test, `make test-scripts`, `make build`, public
documentation validation, and whitespace checks passed. The build reported
that `codesign`/`rcodesign` is unavailable, so the Darwin helper binaries were
left unsigned; compilation completed successfully.

The real Tauri launch and shared-home contention smoke remains pending. This
host is Linux x86_64, and `desktop-runtime` only runs on macOS. Keep this work
order in progress until a macOS host records the backend database, log path,
dev profile, and lock-failure observations.
