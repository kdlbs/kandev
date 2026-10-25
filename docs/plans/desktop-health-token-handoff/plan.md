---
spec: docs/specs/executors/requirements/port-collision-safety.md
created: 2026-08-11
status: in_progress
---

# Implementation Plan: Desktop Health Token Handoff

## Overview

Kandev v0.86.1 introduced a nested readiness-owner regression. The Tauri shell creates a token,
but the native Go launcher creates a second token and overwrites the inherited value. The backend
becomes ready with the launcher's token, the shell rejects it for 60 seconds, and the shell then
terminates the healthy backend.

The repair preserves the desktop-owned token only when the exact desktop launch marker and a
non-empty token are both present. Ordinary CLI, development, and service launches continue to
generate a fresh token and ignore stale inherited values. After targeted tests and a five-platform
desktop validation run, the stable release workflow publishes v0.86.2.

The companion desktop contract is
[Tauri Desktop App](../../specs/desktop/requirements/desktop-tauri-app.md). Launcher ownership remains governed by
[ADR-2026-08-08-go-launcher-owns-all-launch-modes](../../decisions/2026-08-08-go-launcher-owns-all-launch-modes.md).

## Backend

### Readiness-token ownership selection

Update `apps/backend/internal/launcher/env.go` with a small `launchHealthToken` helper. It returns
the inherited `KANDEV_DESKTOP_HEALTH_TOKEN` only when
`KANDEV_DESKTOP_NATIVE_NOTIFICATIONS` is exactly `true` and the inherited token is non-empty.
Otherwise it calls the existing `newHealthToken` generator.

Update `runManagedApp` in `apps/backend/internal/launcher/start.go` to use the selected token for
both `backendEnv` and `waitForHealthFn`. This keeps the backend, supervisor manifest, native
launcher poll, and Tauri shell on one token for desktop-owned launches while retaining a fresh
launcher-owned token everywhere else. Token values remain absent from logs and errors.

No backend health route, Tauri Rust production code, command-line option, database, or migration
changes are required.

## Tests

- **What:** A desktop-owned nested launch preserves the Tauri-generated token through the native
  launcher child environment and health poll.
  **File:** `apps/backend/internal/launcher/start_test.go`.
  **How:** Set both desktop environment values, inject the existing backend-launch and health-poll
  seams, and assert the same known token reaches both. This test must fail before the production
  change because v0.86.1 replaces the known token.
- **What:** An ordinary CLI launch does not trust a stale inherited desktop health token.
  **File:** `apps/backend/internal/launcher/start_test.go`.
  **How:** Set an inherited token without the desktop marker and assert the backend and health poll
  share a fresh non-empty value different from the stale token.
- **What:** A malformed desktop ownership environment does not bypass fresh token generation.
  **File:** `apps/backend/internal/launcher/start_test.go`.
  **How:** Cover a missing token and a marker value other than exact `true`.
- **What:** The Tauri shell still supplies both the ownership marker and generated token to the
  bundled launcher.
  **File:** `apps/desktop/src-tauri/src/backend.rs` (existing Rust unit test).
  **How:** Run the focused `command_spec_uses_headless_launcher_and_loopback_env` test alongside
  the Go regression suite.
- **What:** The changed backend code passes the repository's static checks.
  **File:** `apps/backend/`.
  **How:** Run `golangci-lint run ./... --new-from-rev=31907ba651221b3bdb09a1db2f19786b43457863
  --timeout=5m` and require zero issues.

## E2E Tests

- **Scenario:** Given a v0.86.2 desktop build starts its bundled native launcher, when the backend
  becomes ready, then the WebView leaves the startup surface and the backend remains alive past the
  former 60-second timeout.
- **Evidence:** The Go boundary regression exercises the real nested token-selection path. The
  existing Rust command-spec test proves the Tauri producer. A `desktop_validation_only` Release
  workflow run builds all five desktop targets before publication. After publication, update the
  affected macOS installation from v0.86.1 and confirm the SPA opens and remains connected.

## Verification Results

Task 01 completed locally. The required Rust toolchain was available outside `PATH`, and Cargo
needed writable temporary cache directories in this environment.

- RED: `go test ./internal/launcher -run TestRunManagedAppPreservesDesktopOwnedHealthToken
  -count=1` failed with a generated launcher token instead of the desktop-owned token.
- GREEN: `go test ./internal/launcher -run 'Test(LaunchHealthToken|RunManagedApp)' -count=1`
  passed.
- Package regression: `go test ./internal/launcher -count=1` passed.
- Desktop producer: `cargo test --features desktop-runtime
  backend::tests::command_spec_uses_headless_launcher_and_loopback_env` passed with the checked-in
  Rust toolchain and temporary `CARGO_HOME`/`CARGO_TARGET_DIR`.
- Changed-code lint: `golangci-lint run ./... --new-from-rev=31907ba651221b3bdb09a1db2f19786b43457863
  --timeout=5m` passed with zero issues after the `goconst` cleanup.

Tasks 02 and 03 remain pending until the fix is merged to `main` and the release workflow can be
run under their documented preconditions.

## Implementation Waves And Parallel Candidates

All work is sequential because validation and publication depend on the merged fix.

Wave 1:

- [x] [Task 01: Restore desktop token ownership](task-01-restore-desktop-token-ownership.md)

Wave 2:

- [ ] [Task 02: Validate desktop release candidate](task-02-validate-desktop-release-candidate.md)

Wave 3:

- [ ] [Task 03: Publish stable v0.86.2](task-03-publish-v0-86-2.md)

No task is parallel-safe, and this plan does not authorize subagents.

## Release

The current stable baseline is v0.86.1. Its GitHub Release, five desktop builds, updater feed,
five native runtime packages plus `kandev` on npm, GHCR images, and Homebrew formula were verified
before this plan was written.

After the fix PR merges, run the Release workflow from `main` first with
`desktop_validation_only=true`. If all five desktop builds succeed, run normal Stable mode with
`bump=patch`, which computes v0.86.2, creates and merges the release PR, signs and pushes the tag,
and publishes every channel. Do not use `backfill_tag`: v0.86.1 contains defective immutable code,
so the release contract requires a new patch.

Declare the release complete only after `Publish GitHub release`, `Publish npm packages`, and
`Update Homebrew tap` succeed, the versioned GHCR manifests exist, the updater feed references
v0.86.2, and the affected macOS desktop launch succeeds.

## Risks

- Reusing every inherited token would weaken launcher ownership and reintroduce wrong-process
  readiness. Reuse is limited to the exact desktop marker plus a non-empty token.
- The desktop marker currently also suppresses duplicate backend notifications. The repair reuses
  that established desktop-owned launch signal but does not change notification behavior.
- A green aggregate Release run can hide skipped publication jobs. Required publication jobs and
  artifacts must be inspected individually.
- macOS and Windows signing depend on configured release secrets. Missing signing must remain
  visible in release notes and must not be misreported as a signed release.

## Out of Scope

- Changing health header names, the 60-second timeout, default ports, or WebView navigation.
- Adding a public CLI flag or user-configurable health token.
- Changing release automation, version-channel policy, or updater behavior.
- Rolling back the v0.86.1 database migrations.

## Open Questions

None.
