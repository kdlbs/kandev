---
created: 2026-10-08
status: in_progress
requirements:
  - REQ-DESKTOP-DEVELOPER-TOOLS-001
system_design:
  - ../../specs/desktop/system-design/developer-tools.md
legacy_specs: []
---

# Implementation Plan: Desktop Developer Tools

## Overview

Make the inspector available in normal release builds and expose it through
View > Developer Tools. Use Cmd+Option+I on macOS and Ctrl+Shift+I elsewhere.
Availability is default-on with no Kandev feature flag. The inspector starts closed.

This is a separate diagnostic capability requested during the
[issue 4100 investigation](https://github.com/kdlbs/kandev/issues/4100#issuecomment-6067024473).
It has no dependency on the [scroll settlement fix](../chat-scroll-bounded-settlement/plan.md).
One sequential work order owns implementation, tests, native checks, and public documentation.

Implementation is complete; native platform verification is pending.

## Scope

In scope: release inspection, the native View action and accelerator, Safari
inspectability on supported macOS, state preservation, and instructions for users.

Out of scope: runtime flags, backend/SPA settings, automatic profiling, remote
debugging listeners, inspector customization, and fixes for memory or CPU growth.

## Technical approach

Follow the [desktop design](../../specs/desktop/system-design/developer-tools.md):

1. Enable Tauri's compile-time `devtools` dependency capability in `Cargo.toml`.
2. Explicitly enable the capability on the existing main web-view builder.
3. Add the native action and accelerator in `shell.rs` and `main.rs`.
4. Dispatch directly to the existing main window's `open_devtools()` method.
5. Document release availability and native/Safari entry points.

Keep dependency versions pinned. Update `Cargo.lock` only if Cargo requires it.
No new crate, Objective-C bridge, Tauri invoke permission, or desktop event is needed.
The private macOS inspector API is already encapsulated by the pinned Wry dependency.
Requirements and design preserve this small feature's rationale. No new ADR is needed.

## ASCII UI preview

UI-01: Native View menu, available during startup and normal use.

```text
View
+--------------------------------------+
| Zoom In                              |
| Zoom In (=)                          |
| Zoom Out                             |
| Actual Size                          |
| ------------------------------------ |
| Toggle Full Screen                   |
| ------------------------------------ |
| Developer Tools       Cmd+Option+I    |
+--------------------------------------+
```

Windows/Linux show Ctrl+Shift+I on the same new row. The inspector is the
platform's own UI, so its docking and geometry are engine-owned. Existing
application content and scrolling do not change. The row order, label, and
accelerators are required. ASCII spacing is illustrative.

Phone/browser composition: no new Kandev control. Those clients have no Tauri
menu or native shell. The mobile-parity assessment therefore requires native
desktop evidence instead of an artificial mobile Playwright test.
UI-01 maps to AC-DESKTOP-DEVELOPER-TOOLS-001.2 and .3.

## Tests

| Criteria | Evidence |
| --- | --- |
| .1 | Release build with `desktop-runtime`, inspector closed at startup, no opt-in |
| .2, .3 | `shell.rs` route/accelerator regressions plus native menu and keyboard checks |
| .4 | Page marker survives inspector close/reopen, backend and task continue |
| .5 | Packaged macOS app appears in Safari Develop and allows inspection |
| .6 | Capability files retain their permissions, existing Rust tests pass |

All criteria refer to AC-DESKTOP-DEVELOPER-TOOLS-001.
The work order names exact commands and the required native scenario.

## End-to-end evidence

Use the existing isolated desktop smoke for Linux startup continuity. Its
current harness does not prove inspector availability, so report that limit.
Perform the work order's native scenario against release binaries on macOS,
Windows, and Linux. Record build identity, OS/WebView version, and results.
macOS is the required diagnostic target for the original issue. A Linux
Playwright WebKit run does not establish macOS WKWebView behavior.

## Work orders

- [ ] [Task 01: Enable native developer tools](task-01-enable-developer-tools.md)

Dependency order: one sequential task in the primary conversation.

## Verification results

The implementation and automated checks below are complete. Delivery remains
in progress until the native scenarios in the work order are recorded for Linux,
macOS, and Windows, including the macOS Safari Develop and heap-snapshot checks.

- Tauri 2.11.5 with Wry 0.55.1 compiled with release developer-tools support.
- `cargo test --locked --features desktop-runtime` passed all 111 tests.
- `cargo check --locked --release --features desktop-runtime --bin kandev-desktop`
  and `cargo fmt --check` passed.
- `pnpm --filter @kandev/desktop build:vite` and
  `pnpm --filter @kandev/desktop e2e` passed. The release-shaped Linux smoke
  verified startup and isolated recovery. It does not open the inspector.
- Linux package artifact: `apps/desktop/src-tauri/target/release/bundle/deb/Kandev_0.97.0_amd64.deb`.
  Source HEAD: `6254b05eb0242b67900e160ff5b1d9acbb7962ad`; the artifact includes
  the current working-tree changes.
- Public-doc tests (63), public-doc validation (47 pages), catalog validation
  (365 decisions, 1475 specifications), specification lint, and diff checks passed.
- Native inspector interaction is pending. This Linux runner did not exercise
  the View action, shortcut, page-marker preservation, or startup-error view.
  macOS Safari inspection and Windows interaction also require their native hosts.

## Risks

- Debug builds mask a missing Cargo capability. Release build evidence is required.
- Tauri uses private macOS APIs for in-app inspection. OS updates can affect that path.
- Native menu accelerators and web-engine shortcuts must not dispatch twice or close the inspector unexpectedly.
- macOS and Windows native checks require those hosts. Missing results must remain explicit and must not be inferred from Linux.

## Public documentation

The existing `docs/public/desktop-app.md` page now explains the release menu
action, platform shortcuts, page preservation, and Safari's Develop route on
supported macOS.
