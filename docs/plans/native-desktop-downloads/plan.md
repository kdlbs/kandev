---
created: 2026-09-25
status: in_progress
requirements:
  - REQ-DESKTOP-NATIVE-DOWNLOADS-001
system_design:
  - ../../specs/desktop/system-design/native-downloads.md
legacy_specs: []
---

# Implementation Plan: Native Desktop Downloads

## Overview

Give explicit in-app file downloads a native Save dialog and a dependable
result. First install the desktop transfer boundary, then align first-party
export feedback and object-URL lifetime with that boundary.

## Initial diagnosis

The log bundle is prepared, then `log-viewer.tsx` clicks an HTTP anchor.
At investigation time, `main.rs` had no Tauri download callback, so the shell
did not ask for a save destination or receive a completion result. The pinned
macOS WebView defaults to the Downloads folder silently. The implementation
now registers a callback; a packaged macOS reproduction is still needed to
verify that the reported click saves bytes to the selected destination.

## Scope

### In scope

- Native Save dialog and completion handling for explicit HTTP and Blob
  downloads from the owned desktop WebView.
- Diagnostic bundle feedback and object-URL lifetime corrections.
- Focused first-party download regression coverage across the audited sources.

### Out of scope

- Desktop updater transfers and external browser downloads.
- Changes to export contents, API permissions, and backend archive formats.

## Technical approach

Register `on_download` while constructing the configured main WebView in
`apps/desktop/src-tauri/src/main.rs`. Add a small desktop download module for
URL/origin decisions, safe suggested names, asynchronous Save dialog
selection, retry correlation, completion correlation, and failure signaling.
The initial WebView request opens the dialog and is rejected; a native event
asks the SPA to retry the same URL with the selected destination. Use the
installed `tauri-plugin-dialog`; keep bytes in the WebView's transfer path.
Preserve the existing startup and folder-picker boundaries.

Review all direct object-URL callers found in the source audit. Give their
URLs through the native WebView terminal event, with bounded cleanup for an
abandoned attempt. Centralize that rule in `apps/web/lib/utils/file-download.ts`.
Make the System
Logs message reflect a requested save, cancellation, and actual completion in
desktop, while browser and phone continue their existing browser flow. Add
localized copy only if the existing catalog cannot express those states.
Update `docs/public/desktop-app.md`, whose external-link section currently
says downloads stay in the WebView, in the same implementation change.

The audited first-party entry points are System Logs, System Backups, Office
task documents, task files and full chat text, automation ZIP, canvas
bundle/source, selected Office configuration, organization chart SVG, and
agent memory JSON. The native handler covers both transfer forms. Tests must
exercise representative HTTP and Blob sources, then check the remaining
callers for direct, early object-URL revocation.

## ASCII UI preview

`UI-01: Desktop export`, Settings > System > Logs, after bundle preparation.
The same native panel applies to other in-app downloads.

```text
Current:                         Proposed:
Export logs bundle              Export logs bundle
  -> no Kandev save feedback      -> macOS Save dialog
                                   Name: kandev-diagnostic-logs.zip
                                   Where: [user-selected folder]
                                   [Cancel] [Save]
                                -> Saved / visible failure
```

`UI-02: Phone and browser` retain the current export action and browser-managed
download. The existing phone customizer drawer and touch target stay in place.
Native dialog appearance is owned by the operating system; the drawing only
specifies action order and states. `UI-01` maps to AC-001.1 through AC-001.4;
`UI-02` maps to AC-001.5.

## Tests

| Criterion          | Evidence                                                               |
| ------------------ | ---------------------------------------------------------------------- |
| AC-001.1, AC-001.3 | Rust decision tests pass; packaged Save/Cancel check is pending        |
| AC-001.2           | Packaged HTTP/Blob byte-hash check is pending; criterion is unverified |
| AC-001.4           | Rust completion/failure tests and LogViewer component tests pass       |
| AC-001.5           | Browser and mobile Playwright download tests pass                      |

## E2E tests

`apps/desktop/e2e/desktop-launch-smoke.mjs` verifies startup and readiness. It
does not exercise downloads or native dialogs. The required native transfer
check needs a packaged macOS app and must verify Save, Cancel, suggested names,
and HTTP/Blob byte hashes before claiming AC-001.2. Exercise
`apps/web/e2e/tests/system/mobile-logs-bundle.spec.ts`,
`apps/web/e2e/tests/task/file-tree-download.spec.ts`, and
`apps/web/e2e/tests/system/logs-page.spec.ts` so phone and browser behavior
remain covered.

## Work orders

- [ ] [Task 01: Handle downloads in the desktop shell](task-01-desktop-save-handler.md) (blocked on native transfer verification)
- [ ] [Task 02: Align export feedback and Blob lifetime](task-02-export-feedback.md) (implementation checks pass; package gate remains blocked)

## Verification results

The full Rust suite passed (79 tests), the Rust lockfile audit passed with its
existing allowed warnings, and the Linux desktop package built and passed its
startup smoke. The focused web suite passed (25 tests), along with type-check,
targeted lint, production build, localization checks, browser Logs E2E (3/3),
and mobile Logs E2E (3/3). The desktop smoke validates startup and readiness;
it does not exercise a native Save dialog or file transfer. Criterion
`AC-DESKTOP-NATIVE-DOWNLOADS-001.2` remains unverified. A packaged macOS
Save/Cancel and HTTP/Blob byte-hash run is required before this plan can be
completed, and this Linux host cannot run that WKWebView check.

## Risks

- WKWebView and Linux WebKit can differ in whether an anchor or object URL
  raises a download event; the macOS packaged-app check is required.
- Tauri's macOS finished event can omit the path, so completion must use the
  chosen destination and `success` flag.
- The asynchronous Save panel and retried transfer must be exercised in a
  packaged macOS build.
- Bounded cleanup can expire an abandoned native Blob download; terminal
  feedback normally releases its object URL sooner.
