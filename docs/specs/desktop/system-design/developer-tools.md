---
status: current
system: desktop
created: 2026-10-08
requirements:
  - REQ-DESKTOP-DEVELOPER-TOOLS-001
---

# Desktop Developer Tools System Design

## Boundary and mapping

The native shell owns inspector availability and menu dispatch. The Go backend
and shared React application do not participate. This follows
[ADR 0039](../../../decisions/0039-native-desktop-integration-boundary.md).
The existing [desktop design](desktop-tauri-app.md#native-menu-contract) owns the
surrounding menu and window lifecycle.

| Requirement | Design sections |
| --- | --- |
| REQ-DESKTOP-DEVELOPER-TOOLS-001 | Release availability, Native entry points, State and permissions |

## Release availability

In `apps/desktop/src-tauri/Cargo.toml`, enable `devtools` on the optional Tauri
dependency. Every build that enables `desktop-runtime` then includes inspection.
This Cargo capability is a compile-time dependency feature, not a Kandev feature flag.
Keep the current pinned dependency versions and normal release profile.

In `main.rs`, explicitly set `.devtools(true)` on the existing
`WebviewWindowBuilder::from_config` chain before `.build()`.
Do not call `open_devtools()` during setup. The inspector opens only on demand.
The same builder serves normal and isolated temporary desktop processes.

The pinned Tauri 2.11.5 and Wry 0.55.1 source establishes this path:

- Tauri's `devtools` feature propagates to its runtime and Wry.
- Wry's macOS builder enables `WKWebView.setInspectable(true)` when the selector exists.
- Wry also enables native developer extras and uses WebKit's private inspector
  API to open the in-app inspector. No parallel Objective-C bridge is needed.

The native private API is an accepted implementation tradeoff for the requested
in-app action. Existing distribution uses desktop installers, not the Mac App Store.
Future App Store distribution must revisit this dependency behavior. See
[Tauri's release inspector documentation](https://v2.tauri.app/develop/debug/#using-the-inspector-in-production).

## Native entry points

Extend `shell.rs` with `MENU_DEVELOPER_TOOLS`, using ID
`desktop.v1.developer-tools`, and `MenuAction::DeveloperTools`.
`menu_action` maps that ID to the native action, not an emitted SPA event.
Expose the platform accelerator alongside this action: `Cmd+Alt+KeyI` on
macOS and `Ctrl+Shift+KeyI` on Windows/Linux.

In `main.rs::build_menu`, append a separator and Developer Tools after the
existing full-screen item in View. Use `MenuItemBuilder` and its native accelerator.
The label follows the existing English native-menu convention. No React copy
or web locale catalog changes are involved.

In `handle_menu_event`, resolve `MAIN_WINDOW_LABEL` and call
`WebviewWindow::open_devtools()` on that window. The action does not depend on
SPA readiness or a particular route. Missing-window dispatch is a no-op during shutdown.
Repeated dispatch opens the inspector rather than implementing a toggle.
The handler does not reload, navigate, recreate, or reconfigure the web view.

## State and permissions

No preference, database row, runtime registry entry, or environment gate is added.
The inspector operates on the existing web view and uses engine-owned state.
Kandev performs no automatic capture or upload.

The native menu calls Rust directly. Do not add
`core:webview:allow-internal-toggle-devtools`, `core:default`, or a custom invoke
command to the startup or remote capabilities. Existing exact-origin checks and
the restricted desktop bridge remain authoritative for application scripts.
The local user can inspect the page and execute console expressions, as expected
for developer tools. This does not grant a new application-script capability.

## Compatibility and surfaces

| Surface | Entry | Verification |
| --- | --- | --- |
| macOS release app | View > Developer Tools, Cmd+Option+I | Packaged native inspection, preserved page marker and heap snapshot |
| macOS 13.3+ Safari | Develop > this Mac > Kandev page | Same existing WKWebView appears and is inspectable |
| Windows release app | View > Developer Tools, Ctrl+Shift+I | Packaged WebView2 native inspection |
| Linux release app | View > Developer Tools, Ctrl+Shift+I | Packaged WebKitGTK native inspection |
| Browser and phone | Existing browser tools | No shared web source or viewport changes |

The menu addition does not change application content layout, scrolling, touch
targets, or navigation. Mobile Playwright cannot exercise an OS-native Tauri
menu and is not a substitute for packaged desktop verification.

## Verification and documentation

Routing and accelerator tests belong in `shell.rs`. Release compilation proves
that `open_devtools()` is available outside debug builds. A debug-only build or
a menu-ID test cannot establish release inspection.

Native checks must exercise both the menu and accelerator. Set an in-memory
marker in the inspected page, close and reopen the inspector, and verify that
the marker remains. Confirm startup/error inspection and continued backend activity.
Capture a macOS heap snapshot to demonstrate the requested diagnostic workflow.
Engine snapshots can expose JavaScript wrappers without accounting for every
native allocation. Do not promise complete attribution of issue 4100 from one snapshot.

Update `docs/public/desktop-app.md` with the native entry points and Safari
fallback. Update `apps/desktop/AGENTS.md` with the native ownership and release
capability. The [implementation package](../../../plans/desktop-developer-tools/plan.md)
owns commands, platform evidence, and delivery status.
