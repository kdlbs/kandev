---
id: "01-restore-desktop-token-ownership"
title: "Restore desktop token ownership"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/executors/requirements/port-collision-safety.md"
---

# Task 01: Restore desktop token ownership

## Acceptance

- A desktop-owned launch reuses its exact non-empty inherited health token for the backend child,
  native launcher poll, and supervisor restart manifest.
- An ordinary CLI, development, or service launch generates a fresh token even if its environment
  contains a stale `KANDEV_DESKTOP_HEALTH_TOKEN`.
- Missing or non-exact desktop ownership values fall back to fresh token generation, and no token
  value appears in logs or errors.

## Verification

Use TDD. Add the desktop-owned `runManagedApp` regression first and confirm it fails because the
known token is replaced. Implement the smallest selection helper, then run:

```bash
cd apps/backend && \
  go test ./internal/launcher -run 'Test(LaunchHealthToken|RunManagedApp)' -count=1 && \
  go test ./internal/launcher -count=1 && \
  cd ../../apps/desktop/src-tauri && \
  cargo test --features desktop-runtime backend::tests::command_spec_uses_headless_launcher_and_loopback_env && \
  cd ../../../apps/backend && \
  golangci-lint run ./... --new-from-rev=31907ba651221b3bdb09a1db2f19786b43457863 --timeout=5m
```

## Files likely touched

- `apps/backend/internal/launcher/env.go`
- `apps/backend/internal/launcher/start.go`
- `apps/backend/internal/launcher/start_test.go`
- `docs/plans/desktop-health-token-handoff/plan.md`
- `docs/plans/desktop-health-token-handoff/task-01-restore-desktop-token-ownership.md`

## Dependencies

None.

## Parallelism

`sequential`. The test seams and launcher environment are shared within one package.

## Inputs

- `docs/specs/executors/requirements/port-collision-safety.md`, Backend readiness ownership and Issue #2372 scenarios.
- `docs/specs/desktop/requirements/desktop-tauri-app.md`, Existing launch and data contract.
- `apps/desktop/src-tauri/src/backend.rs`, `launch_and_wait` and
  `command_spec_uses_headless_launcher_and_loopback_env`.
- `apps/backend/internal/launcher/start.go`, `runManagedApp`.
- `apps/backend/internal/launcher/env.go`, `newHealthToken` and `backendEnv`.

## Output contract

Report the red test failure, minimal production change, exact command results, files changed,
security boundary, blockers, risks, and synchronized task/plan status in the primary conversation.

## Results

RED: `go test ./internal/launcher -run TestRunManagedAppPreservesDesktopOwnedHealthToken -count=1`
failed because the launcher replaced the known desktop-owned token with a generated value.

GREEN and verification:

- `go test ./internal/launcher -run 'Test(LaunchHealthToken|RunManagedApp)' -count=1` passed.
- `go test ./internal/launcher -count=1` passed.
- `cargo test --features desktop-runtime
  backend::tests::command_spec_uses_headless_launcher_and_loopback_env` passed using the checked-in
  Rust toolchain with temporary writable Cargo cache and target directories.
- `golangci-lint run ./... --new-from-rev=31907ba651221b3bdb09a1db2f19786b43457863
  --timeout=5m` passed with zero issues after the `goconst` cleanup.

The launcher now reuses the inherited token only when the desktop-owned marker is exactly `true`
and the token is non-empty. Ordinary and malformed launches still generate a fresh token. The
backend child environment and health poll receive the same selected token, and no token is added
to logs or errors.
