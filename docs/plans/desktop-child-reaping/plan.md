---
created: 2026-09-30
status: done
requirements:
  - REQ-DESKTOP-DESKTOP-TAURI-APP-001
  - REQ-DESKTOP-ISOLATED-STARTUP-002
system_design:
  - ../../specs/desktop/system-design/desktop-tauri-app.md
  - ../../specs/desktop/system-design/isolated-startup.md
legacy_specs: []
---

# Implementation Plan: Desktop child reaping

## Overview

Correct the confirmed child-reaping defects associated with [issue #4100](https://github.com/kdlbs/kandev/issues/4100).
First correct macOS external-link launches and provide a managed-launch helper.
Then use that helper for temporary desktop windows.
Both work orders are implemented and validated on Linux. Native macOS acceptance remains pending because this implementation host is Linux.

This package does not resolve the issue's WebKit CPU or memory symptoms.
No profile or process sample exists for that incident.
Keep the issue open after the child-reaping repair until that separate symptom has evidence and a resolution.

## Evidence and root cause

The inspected source revision is `ecd415b7f672b32c3e8292171d54d4c841963e12`.
The desktop manifest identifies version `0.96.0`, matching the report.
The issue has no image attachments or comments at investigation time.

The external-link call chain is:

```text
Help menu or origin-checked open_external_url command
  -> external_links::open_validated_external_url
  -> tauri_plugin_opener 2.5.4, open_url(url, None)
  -> open 5.4.0, that_detached
  -> CommandExt::spawn_detached
  -> Unix pre_exec: fork, immediate intermediate-child exit
  -> Command::spawn().map(|_| ())
```

`open 5.4.0` drops the intermediate child's handle without a wait.
The macOS command is `/usr/bin/open -- <url>`.
The Unix intermediate child exits for each successful detached launch, regardless of the browser's lifetime.
Rust does not automatically reap dropped children. See the [Rust Child contract](https://doc.rust-lang.org/std/process/struct.Child.html#warning).

A temporary Cargo reproduction used the exact pinned `open 5.4.0` dependency on Linux.
It called `open::with_detached("/dev/null", "/usr/bin/true")` 17 times.
This exercises the same Unix `spawn_detached` implementation without opening a browser.
After 300 ms, `ps` showed 17 zombie children under the reproduction process.
A control with 17 `Command::status()` calls produced zero zombies.
The reproduction then dropped one `Child` from `/usr/bin/true`; that produced one zombie.
The reproduction reaped only its own test children and exited successfully.
Its temporary files were removed after investigation.

`backend::start_temporary_test_instance` has the second confirmed defect:
it directly calls `spawn().map(|_| ())`, discarding the GUI child's handle.
An exited temporary window can therefore remain a zombie while its conflict launcher stays open.
The issue does not establish whether temporary windows were used during the incident.

The evidence confirms deterministic defects and a plausible source of the reported zombie count.
It does not identify the original 17 child PIDs or establish their actual launch paths.
It also does not establish a causal link between zombies and WebKit resource growth.

## Assumption check and contract reconciliation

- Confirmed intent: investigate the issue and prepare a reliable fix package before implementation.
- Verified contract: desktop owns native child processes and scoped external-link validation.
- Verified contract: temporary windows outlive the conflict launcher and own separate backend trees.
- Missing acceptance: explicit auxiliary-child reaping outcomes. Criteria `.12` through `.14` extend the existing desktop app requirement.
- Missing acceptance: temporary GUI-child reaping. Criterion `.6` extends the existing isolated-startup requirement.
- Unresolved evidence: the cause of WebKit CPU and memory growth. It blocks a WebKit patch, not these child-reaping corrections.

Desktop owns this package because it owns the native launch and process-lifetime contracts.
The shared SPA does not own these children.

## Scope

### In scope

- Reap each macOS external-link helper through its own wait owner.
- Reap temporary GUI children without coupling their lifetime to the launcher.
- Preserve origin checks, URL validation, spawn errors, and backend ownership.
- Provide exact-PID regression evidence and a native macOS acceptance check.

### Out of scope

- WebKit heap, rendering, animation, or timer changes without incident evidence.
- Restart workarounds, automatic reloads, process quotas, or resource thresholds.
- Agentctl/ACP process reduction covered by the issue's related work.
- Post-readiness backend supervision changes. One unwatched backend exit cannot explain 17 auxiliary children.
- Linux external-opener dependency repair. Its shared Unix detachment code has the same risk, but this package changes the macOS branch only.
- Windows external-opener behavior, bridge permissions, UI markup, copy, or layout.

## Technical approach

Follow [auxiliary child ownership](../../specs/desktop/system-design/desktop-tauri-app.md#auxiliary-child-ownership).
Create `apps/desktop/src-tauri/src/child_process.rs` and declare it privately in `src/lib.rs`.
Allocate a worker before spawn, acknowledge the spawn result through a channel, and wait for the exact child inside that worker.
No global child reaper, signal handler, or automatic child-status discard is permitted.

On macOS, `external_links::open_validated_external_url` uses an absolute `/usr/bin/open` command with `--` and the validated URL as separate arguments.
The existing menu and native-command paths already share that function.
Other platforms retain the existing plugin branch.
No new Cargo dependency or capability is necessary.

Then replace the discarded temporary GUI handle in `backend::start_temporary_test_instance` with the same helper.
Keep its conflict-page authorization, `current_exe`, internal argument, and null streams.
Do not add the GUI child to `BackendState.child` or to shutdown termination.

### Compatibility matrix

| Launch path | Transport and identity | Intended behavior | Evidence and fallback |
| --- | --- | --- | --- |
| macOS Help menu | Native menu, validated URL | Managed `/usr/bin/open` child | Command arguments, exact-PID tests, native smoke. Spawn failure uses the existing menu error path. |
| macOS external-link command | Owned-origin WebView and validated URL | Same managed helper | Existing validation tests and native smoke. Invalid origin or URL rejects before spawn. |
| Temporary desktop window | Verified conflict page, fixed executable and internal argument | Managed GUI wait owner | Real-child tests and native multi-window smoke. Spawn error returns without changing conflict state. |
| Windows/Linux external link | Existing opener plugin | Existing behavior | Compile guards and existing validator tests. No new cross-platform support claim. |
| Owned backend | `BackendState` exact child | Existing startup and stop ownership | Existing backend Rust suite. Auxiliary workers cannot consume its status. |

## Tests

The following regression tests cover the acceptance criteria.

| Acceptance | Test file and regression |
| --- | --- |
| `AC-DESKTOP-DESKTOP-TAURI-APP-001.12` | `src/child_process.rs`: `managed_launch_reaps_repeated_short_lived_children` with 17 launches and exact-PID `ECHILD` proof |
| `AC-DESKTOP-DESKTOP-TAURI-APP-001.13` | `src/child_process.rs`: `managed_launch_acknowledges_before_child_exit`, `managed_launch_reports_spawn_failure`; `src/external_links.rs`: `macos_open_command_preserves_url_as_one_argument` and existing validation tests |
| `AC-DESKTOP-DESKTOP-TAURI-APP-001.14` | `src/child_process.rs`: `managed_launch_does_not_reap_unrelated_child`, including a live sentinel and an exited sentinel owned by the test |
| `AC-DESKTOP-ISOLATED-STARTUP-002.6` | `src/backend.rs`: `temporary_launch_reaps_exited_gui_children`; managed-launch parent-exit subprocess test |
| `AC-DESKTOP-ISOLATED-STARTUP-002.3` | Existing independent-backend and temporary-home shutdown tests plus multi-window native acceptance |

The repeated-launch test must fail against the old discarded-handle pattern because the PID remains waitable.
The temporary-launch test must exercise the actual extracted command launch path, rather than only the common helper.
Test-only `waitpid(pid, WNOHANG)` probes must target known test PIDs.
If a probe collects a zombie status, record the regression failure and clean up the test child.
Do not use a global wait in repository tests or production.

## Native end-to-end acceptance

No rendered interface changes are planned, so no ASCII preview or phone UI test is required.
Native process behavior needs macOS evidence that a Linux reproduction cannot provide.
Use an isolated development home and an existing prepared desktop runtime.

For Task 01, invoke Help-menu and SPA external links repeatedly, with at least 17 total opens.
Record the desktop PID and its child states before and after the launches.
After one second, no exited link-helper child can remain a zombie.
The app must remain interactive and retain its owned backend.

For Task 02, start two temporary windows from a deliberate conflict launcher.
Close one window and verify its GUI child is reaped within one second.
Keep the other window live, close the conflict launcher, and verify the remaining window and its backend still work.
Never use a developer's live database for this acceptance check.

Linux automation extends the fake-runtime multi-window smoke in `apps/desktop/e2e/desktop-launch-smoke.mjs` to record the launcher and GUI PIDs and verify exact child disappearance. It does not substitute for native macOS helper reaping.

## Work orders

- [x] [Task 01: Reap macOS external-link helpers](task-01-external-link-helpers.md)
- [x] [Task 02: Reap temporary desktop children](task-02-temporary-desktop-children.md)

Task 02 depends on Task 01's common helper. Execute sequentially.
The historical native-integration and isolated-startup packages remain completed records.
Their prior results do not count as evidence for this repair.

## Verification results

Implementation checks passed on Linux. Native macOS acceptance is pending.

Implementation checks on 2026-10-01:

- `(cd apps/desktop/src-tauri && cargo fmt --all -- --check)`: passed.
- `(cd apps/desktop/src-tauri && cargo test --locked --features desktop-runtime --lib)`: passed, 109 tests.
- `(cd apps/desktop/src-tauri && cargo test --locked --features desktop-runtime --lib backend::tests)`: passed, 51 tests.
- `(cd apps/desktop/src-tauri && cargo check --locked --features desktop-runtime --bin kandev-desktop)`: passed.
- `node --test apps/desktop/e2e/desktop-launch-smoke.test.mjs`: passed, 24 tests, including the empty-output `ps` exit-1 regression.
- `(cd apps && pnpm --filter @kandev/desktop e2e)`: passed. The Linux smoke observed conflict launcher PID `2837251`, temporary GUI PIDs `2842565` and `2847817`, reaped the closed child within the one-second check, and confirmed the sibling backend stayed healthy.
- `python3 scripts/list-docs.py validate`: passed, with 338 decisions and 1,275 specifications.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
- Post-review fixture verification on Linux: `cargo test --locked --lib child_process::tests` passed, 5 tests; source inspection confirmed both Unix live-child fixtures use `/bin/sleep`.
- Native macOS Help-menu, SPA external-link, and multi-window acceptance: pending a macOS host.
- PR fixup rerun: `(cd apps && pnpm --filter @kandev/desktop e2e)` passed. The conflict launcher PID was `3261479` with temporary GUI PIDs `3262949` and `3267663`; the smoke verified the closed child's disappearance while the sibling backend and conflict launcher stayed active.

Investigation and package checks on 2026-09-30:

- Temporary `cargo run --offline --quiet` reproduction: passed. Seventeen detached launches produced 17 zombies. The waited control produced zero.
- `python3 scripts/list-docs.py validate`: passed, with 338 decisions and 1,275 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Repository `validateCoverage` preflight from `.github/scripts/pr-docs.cjs`: passed for both new work orders and their complete references.
  The preflight included the proposed Rust source path to exercise coverage rather than the documentation-only exemption.
- `git diff --check -- docs/specs/desktop docs/plans/desktop-child-reaping`: passed.
- `git status --short -- docs/plans/desktop-child-reaping`: confirmed the untracked package. All package files remain unstaged and uncommitted.
- Temporary reproduction files: removed. All reproduction-owned child statuses were collected before exit.
- Issue assignment: verified `carlosflorencio` on #4100. The issue remains open.

Public documentation needs no update during this design turn.
The package changes internal requirements and design intent without a new user action, option, or label.
The existing native-integration and independent-window decisions remain authoritative.
No new ADR is necessary for this local child-wait correction.

## Remaining WebKit investigation

If the CPU symptom recurs, preserve evidence before restart.
Capture a WebContent process sample while CPU is high and a Safari Web Inspector timeline from the affected window.
Record desktop/WebContent PIDs, app version, uptime, active task, open panels, and whether the window is foreground or hidden.
Record CPU, resident/compressed memory, and exact zombie child identities at the same time.
Avoid publishing raw prompts, environment values, or private page content.

The sample identifies hot native/JavaScript stacks. The timeline separates scripting, layout, painting, and sustained animation work.
Only a repeatable cause and failing focused reproduction justify a subsequent WebKit or SPA work order.
These observations are an investigation procedure, not a claim that the child repair fixes CPU or memory growth.

## Risks

- The original macOS incident has no child identities or WebKit profile. Its complete resolution remains unproven.
- Worker acknowledgement must occur after spawn but before wait, or temporary-window launch will block until the window closes.
- Wait workers need exact-child ownership even when the acknowledgement receiver disappears.
- Native macOS validation requires a macOS host. Linux can prove the common Unix defect and wait semantics only.
- The unchanged Linux opener retains the dependency's known detached-child risk.
- Native `open` success confirms helper completion, not browser page-load success.
