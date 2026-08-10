---
id: "01-profile-selector-contract"
title: "Separate debug profile selector"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/dev-preview-title-prefixes/spec.md"
---

# Task 01: Separate debug profile selector

## Acceptance

- `KANDEV_DEBUG_PPROF_ENABLED=true` without `KANDEV_DEBUG_DEV_MODE=true` selects
  `prod` and does not receive the `Dev` profile prefix.
- `KANDEV_DEBUG_DEV_MODE=true` still selects `dev` and supplies the `Dev`
  title prefix.
- The native and direct backend `start-debug` launchers supply the `Debug`
  prefix by default, while preserving an explicit prefix.
- A supervised backend restart preserves the configured title prefix.
- The direct backend `dev` target exports `KANDEV_DEBUG_DEV_MODE=true`, while
  `start-debug` retains pprof and debug logging without that selector.
- The profile ADR and its index describe the corrected boundary.

## Verification

- `cd apps/backend && go test ./internal/profiles`
- `cd apps/backend && go test ./internal/launcher`
- `make check-make-shells`
- `git diff --check`

## Files likely touched

- `apps/backend/internal/profiles/profiles.go`
- `apps/backend/internal/profiles/profiles_test.go`
- `apps/backend/internal/launcher/env.go`
- `apps/backend/internal/launcher/start_test.go`
- `apps/backend/internal/launcher/supervisor.go`
- `apps/backend/internal/launcher/supervisor_test.go`
- `apps/backend/Makefile`
- `scripts/check-make-shells`
- `docs/decisions/0007-runtime-feature-flags.md`
- `docs/decisions/2026-08-10-debug-launcher-profile-selection.md`
- `docs/decisions/INDEX.md`

## Dependencies

None.

## Parallelism

Sequential. The profile selector and launcher environment are one shared
startup contract.

## Inputs

- The `What`, API surface, failure modes, and scenarios in the repair spec.
- The confirmed root cause in `plan.md`.
- `profiles.DetectEnvironment`, `profiles.ApplyProfile`, and the existing shell
  dispatch harness.

## Output contract

Report the changed files, exact test commands and results, any risks, and the
updated task and plan statuses in the same conversation. Do not delegate this
task to a subagent without explicit user authorization.

## Results

- `cd apps/backend && go test ./internal/profiles` — passed (11 tests).
- `cd apps/backend && go test ./internal/launcher` — passed (133 tests).
- `cd apps/backend && make check-make-shells` — passed (Unix, native Windows,
  and Git Bash/MSYS dispatch).
- `git diff --check` — passed.
