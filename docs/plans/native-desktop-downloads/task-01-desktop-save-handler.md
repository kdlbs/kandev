---
id: "01-desktop-save-handler"
title: "Handle downloads in the desktop shell"
status: blocked
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-DESKTOP-NATIVE-DOWNLOADS-001
acceptance_criteria:
  - AC-DESKTOP-NATIVE-DOWNLOADS-001.1
  - AC-DESKTOP-NATIVE-DOWNLOADS-001.2
  - AC-DESKTOP-NATIVE-DOWNLOADS-001.3
  - AC-DESKTOP-NATIVE-DOWNLOADS-001.4
system_design:
  - ../../specs/desktop/system-design/native-downloads.md
---

# Task 01: Handle downloads in the desktop shell

## Summary

Register a native Save dialog for downloads from the owned WebView. Preserve
the original response bytes and report completion or failure.

## In scope

- Construct the configured main WebView with a registered download callback.
- Check the owned origin, sanitize suggested filenames, handle Save/Cancel,
  and correlate the finished event with the selected destination.
- Exercise both HTTP and Blob downloads in the desktop smoke harness.

## Out of scope

- Frontend export copy and object-URL caller changes.
- Updater package transfers and external links.

## Acceptance

1. An owned HTTP or Blob download opens a parented Save panel asynchronously
   with the suggested filename, then retries the same URL and writes exact
   bytes only after selection.
2. Cancel, untrusted origin, dialog error, and failed transfer do not claim
   success and leave the user able to retry.
3. Existing desktop startup, capabilities, and folder selection still work.

## ASCII UI preview

`UI-01: Desktop export` from the [plan](plan.md#ascii-ui-preview):

```text
Export logs bundle -> Save dialog -> [Cancel] or [Save] -> result
```

The Save dialog uses the operating system's layout. AC-001.1 through AC-001.4
control its sequence and outcome.

## Verification

```bash
(cd apps/desktop/src-tauri && cargo test --features desktop-runtime)
(cd apps && pnpm --filter @kandev/desktop e2e)
```

Run the desktop smoke on macOS as a required platform check, with an HTTP
diagnostic ZIP and a generated Blob; record the app version, Save/Cancel
outcomes, and saved file hashes in Results. Linux success alone does not
confirm the macOS complaint is fixed.

## Files likely touched

- `apps/desktop/src-tauri/tauri.conf.json`
- `apps/desktop/src-tauri/src/main.rs`
- `apps/desktop/src-tauri/src/downloads.rs` (new)
- `apps/desktop/e2e/desktop-launch-smoke.mjs`
- `apps/desktop/e2e/desktop-launch-smoke.test.mjs`
- `apps/desktop/AGENTS.md` if the shell contract changes its guidance

## Dependencies

None.

## Risks

The asynchronous download callback and native dialog must cooperate on each
supported WebView. macOS may omit the finished path even after success.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/desktop/requirements/native-downloads.md)
- [System design](../../specs/desktop/system-design/native-downloads.md)
- [Native download decision](../../decisions/2026-09-25-native-desktop-downloads.md)

## Results

- `cargo test --features desktop-runtime downloads::tests`: all 13 download tests passed.
- `cargo test --features desktop-runtime`: all 79 Rust tests passed.
- `cargo fmt --check`: passed.
- `cargo audit`: passed after updating `rustls` to 0.23.45; eight existing allowed warnings remain.
- `node --test e2e/desktop-launch-smoke.test.mjs`: all 9 tests passed.
- `pnpm --filter @kandev/desktop e2e`: Linux desktop build and startup smoke passed.
- The startup smoke verifies backend readiness and WebView navigation. It does
  not automate the native Save dialog or verify downloaded bytes.
- A packaged macOS Save/Cancel and HTTP/Blob byte check is still required. This
  Linux host cannot verify WKWebView dialog behavior or saved-file hashes.
- The Save dialog is asynchronous on the WebView main thread. After selection,
  the app retries the same URL with the selected destination; Rust tests cover
  selection, retry, expiry, cancellation, and completion correlation.
- Status remains blocked until packaged native Save/Cancel and byte-preservation
  evidence is recorded. AC-DESKTOP-NATIVE-DOWNLOADS-001.2 is unverified.
