---
id: "02-temporary-desktop-children"
title: "Reap temporary desktop children"
status: done
wave: 2
depends_on:
  - "01-external-link-helpers"
plan: "plan.md"
requirements:
  - REQ-DESKTOP-ISOLATED-STARTUP-002
acceptance_criteria:
  - AC-DESKTOP-ISOLATED-STARTUP-002.3
  - AC-DESKTOP-ISOLATED-STARTUP-002.6
system_design:
  - ../../specs/desktop/system-design/isolated-startup.md
---

# Task 02: Reap temporary desktop children

## Summary

Use Task 01's managed-launch helper for each temporary desktop GUI process.
Retain independent window lifetimes while the conflict launcher reaps exited children.
Preserve the existing conflict authorization and failure response.

## In scope

- Extract the fixed temporary command construction and launch into a testable private path in `backend.rs`.
- Keep `require_conflict_startup`, `current_exe`, `TEMPORARY_TEST_ARGUMENT`, and null standard streams.
- Replace `spawn().map(|_| ())` with managed launch, mapping its successful acknowledgement to the existing unit result.
- Add an actual launch-path regression and a subprocess test for survival after the launching parent exits.
- Extend the existing Linux smoke to assert an exited GUI child no longer appears under its conflict-launcher PID.

## Out of scope

- Adding GUI children to the backend shutdown tree, changing temporary-home cleanup, or changing the conflict UI.
- Backend runtime watchers, external-opener changes, or WebKit CPU/memory changes.

## Acceptance

1. Repeated short-lived GUI stand-ins leave no waitable status within one second, through the actual temporary command launch path.
2. The conflict launcher acknowledges spawn before GUI exit. Failure permits another attempt and preserves conflict state.
3. Closing one temporary window preserves its sibling and normal instance. Closing the conflict launcher preserves an already running temporary window and backend.

## Regression strategy

In `backend.rs`, add `temporary_launch_reaps_exited_gui_children` with a test-owned executable stand-in.
Verify the internal temporary-test argument and exercise the production launch helper.
The old discarded-handle path must fail exact-PID reaping assertions.
Retain the existing conflict-state, independent-backend, and clean/uncertain-home-stop tests.

In `child_process.rs`, add `managed_launch_child_survives_launching_parent_exit`.
Use a test subprocess as the launcher and a long-lived child that writes a readiness marker.
Exit the launcher, then verify the child remains alive and responsive.
Give the test an explicit child termination protocol and bounded cleanup.
Do not rely on an orphaned `sleep` process or global process termination.

Extend `desktop-launch-smoke.mjs` after its existing two-window launch flow.
Record the GUI child PID and conflict-launcher PID before closing one child.
Assert the exited child disappears while the sibling remains live and its backend still answers.
The existing Node smoke-unit suite must cover any new PID-query helper.

## Verification

Run from the repository root:

```bash
(cd apps/desktop/src-tauri && cargo fmt --all -- --check)
(cd apps/desktop/src-tauri && cargo test --locked --lib child_process::tests)
(cd apps/desktop/src-tauri && cargo test --locked --features desktop-runtime --lib backend::tests)
node --test apps/desktop/e2e/desktop-launch-smoke.test.mjs
(cd apps && pnpm --filter @kandev/desktop e2e)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/desktop-child-reaping
```

In a fresh worktree, run `pnpm install --frozen-lockfile` from `apps/` before the first pnpm command.
Native Tauri build prerequisites also apply.
Perform the macOS multi-window acceptance in the [plan](plan.md#native-end-to-end-acceptance).
Record platform, PIDs, child states, timing, and surviving window/backend results.
If macOS evidence is unavailable, keep that acceptance pending.

## Files likely touched

- `apps/desktop/src-tauri/src/backend.rs`
- `apps/desktop/src-tauri/src/child_process.rs` (parent-exit regression)
- `apps/desktop/e2e/desktop-launch-smoke.mjs`
- `apps/desktop/e2e/desktop-launch-smoke.test.mjs`

## Dependencies

[Task 01](task-01-external-link-helpers.md) supplies the managed-launch helper.

## Risks

- Waiting inside the command handler would block temporary launch for the GUI's entire lifetime.
- A parent-lifetime watchdog would violate the existing independent-window contract.
- A fake-runtime test proves process ownership and reaping, not macOS WebKit CPU behavior.

## Parallelism

`sequential`

## Inputs

- [Isolated-startup requirements](../../specs/desktop/requirements/isolated-startup.md), criteria `.3` and `.6` of requirement `002`.
- [Process-mode design](../../specs/desktop/system-design/isolated-startup.md#process-modes).
- [Temporary-process decision](../../decisions/2026-09-25-temporary-desktop-test-processes.md).
- Current `start_temporary_test_instance`, nearby `backend.rs` child tests, and the existing multi-window smoke.

## Results

Implemented the temporary-window command helper with the existing executable, internal argument, null streams, authorization, and error contract. It uses Task 01's exact-child wait worker. Added 17-child reaping coverage, a parent-exit survival and responsiveness test, and Linux PID checks in the two-window smoke.

Validation passed: `cargo fmt --all -- --check`, `cargo test --locked --lib child_process::tests`, `cargo test --locked --features desktop-runtime --lib backend::tests`, `node --test apps/desktop/e2e/desktop-launch-smoke.test.mjs`, and `pnpm --filter @kandev/desktop e2e` from `apps/`.

The Linux smoke observed conflict launcher PID `2837251` and temporary GUI PIDs `2842565` and `2847817`. After the first GUI window closed, its child process disappeared from the launcher process list within the one-second check; the sibling window and backend stayed active.

Native macOS multi-window acceptance remains pending; this implementation host is Linux. Documentation validation results are recorded in [the implementation plan](plan.md#verification-results).
