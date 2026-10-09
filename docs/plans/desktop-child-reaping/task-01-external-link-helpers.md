---
id: "01-external-link-helpers"
title: "Reap macOS external-link helpers"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-DESKTOP-DESKTOP-TAURI-APP-001
acceptance_criteria:
  - AC-DESKTOP-DESKTOP-TAURI-APP-001.12
  - AC-DESKTOP-DESKTOP-TAURI-APP-001.13
  - AC-DESKTOP-DESKTOP-TAURI-APP-001.14
system_design:
  - ../../specs/desktop/system-design/desktop-tauri-app.md
---

# Task 01: Reap macOS external-link helpers

## Summary

Replace the macOS external-opener detachment path with an exact-child wait owner.
Provide the managed-launch helper that Task 02 also needs.
Keep launch acknowledgement separate from process exit.

## In scope

- Add a private `child_process` module with the worker-before-spawn flow in the [design](../../specs/desktop/system-design/desktop-tauri-app.md#auxiliary-child-ownership).
- Route the macOS `open_validated_external_url` branch through `/usr/bin/open -- <validated-url>` with null streams and separate arguments.
- Preserve existing validator, command origin checks, menu reuse, and spawn-error propagation.
- Add regression tests before production changes, and perform the native acceptance in the [plan](plan.md#native-end-to-end-acceptance).

## Out of scope

- Temporary-window integration, backend supervision, Cargo dependencies, Linux/Windows opener changes, and WebKit resource patches.
- UI markup, translations, permissions, and generic process APIs for the SPA.

## Acceptance

1. Seventeen short-lived managed launches leave no waitable child status within one second under normal scheduling.
2. A long-lived child does not delay spawn acknowledgement. Spawn failure returns an error, and URLs remain one validated argument.
3. An unrelated live or exited sentinel retains its original wait owner. Native macOS Help and SPA launches retain app interaction and backend liveness.

## Regression strategy

In `src/child_process.rs`, add `managed_launch_reaps_repeated_short_lived_children`,
`managed_launch_acknowledges_before_child_exit`, `managed_launch_reports_spawn_failure`,
and `managed_launch_does_not_reap_unrelated_child`.
Use real short-lived subprocesses and exact-PID `waitpid` probes on Unix.
First demonstrate the repeated-launch failure against a discarded `Child` handle.
The assertion distinguishes `ECHILD` from a positive collected zombie status.
An exited unrelated child must remain waitable by its original owner.
Clean up every test process even after an assertion failure.

In `src/external_links.rs`, add `macos_open_command_preserves_url_as_one_argument`.
Verify the absolute executable, `--`, and one URL argument with punctuation and query parameters.
Keep the command builder test available on Linux without executing `/usr/bin/open`.
Existing scheme, local-destination, and credentials tests remain authoritative.
No URL or raw subprocess output can appear in wait diagnostics.

## Verification

Run from the repository root:

```bash
(cd apps/desktop/src-tauri && cargo fmt --all -- --check)
(cd apps/desktop/src-tauri && cargo test --locked --lib child_process::tests)
(cd apps/desktop/src-tauri && cargo test --locked --lib external_links::tests)
(cd apps/desktop/src-tauri && cargo test --locked --features desktop-runtime --lib)
(cd apps/desktop/src-tauri && cargo check --locked --features desktop-runtime --bin kandev-desktop)
git diff --check
```

The feature-enabled commands need the repository's native Tauri prerequisites and prepared resources.
Perform the macOS acceptance in the plan and record observed PID states and timing.
If no macOS host is available, keep that acceptance pending and state the platform limitation.
Do not claim the complete issue is fixed from Linux-only evidence.

## Files likely touched

- `apps/desktop/src-tauri/src/child_process.rs` (new)
- `apps/desktop/src-tauri/src/lib.rs`
- `apps/desktop/src-tauri/src/external_links.rs`
- `apps/desktop/AGENTS.md` (document the managed auxiliary-child convention)

## Dependencies

None.

## Risks

- Thread allocation must precede child creation. A failed thread allocation must not discard an already spawned child.
- A dropped acknowledgement receiver must not cancel the worker's wait responsibility.
- Returning after spawn retains existing command semantics. Later helper failure uses native diagnostics rather than a second command response.

## Parallelism

`sequential`

## Inputs

- [Desktop app requirements](../../specs/desktop/requirements/desktop-tauri-app.md), criteria `.12` through `.14`.
- [Desktop design](../../specs/desktop/system-design/desktop-tauri-app.md#auxiliary-child-ownership).
- [Native integration boundary](../../decisions/0039-native-desktop-integration-boundary.md).
- Current `external_links.rs`, its validation tests, and the Help-menu call in `main.rs`.
- Pinned opener evidence and reproduction in [the plan](plan.md#evidence-and-root-cause).

## Results

Implemented the private managed child waiter and routed macOS external links through `/usr/bin/open -- <validated-url>`. The worker owns the exact child, acknowledges its PID before waiting, and reports bounded wait or exit failures.

Validation passed: `cargo fmt --all -- --check`, `cargo test --locked --lib child_process::tests`, `cargo test --locked --lib external_links::tests`, `cargo test --locked --features desktop-runtime --lib`, and `cargo check --locked --features desktop-runtime --bin kandev-desktop`.

Native macOS Help-menu and SPA acceptance remains pending; this implementation host is Linux.

Review follow-up: both Unix long-lived child fixtures use `/bin/sleep`, which is present on the supported Linux and macOS hosts. The existing live-child acknowledgement and unrelated-child ownership assertions are unchanged. `cargo test --locked --lib child_process::tests` passed on Linux after the fixture update (5 tests).
