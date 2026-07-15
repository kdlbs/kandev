# 0039: Native Desktop Integration Boundary

**Status:** accepted
**Date:** 2026-07-15
**Area:** desktop, frontend, backend, infra

## Context

The Tauri shell loads Kandev's Go-served SPA from a per-launch loopback origin. Native menus,
window lifecycle, signed updates, notifications, window persistence, and external URL opening
need OS integration, while contextual actions such as closing a dialog or document tab belong to
the React product model. Granting the remote WebView broad Tauri plugin permissions would weaken
the security boundary established by ADR-0026. Existing backend/browser notification paths and
the service-only updater also create duplication risks in desktop-owned launches.

## Decision

Rust owns native menus and accelerators, application/window lifecycle, zoom state, update
verification/install/restart, native notification transport, window-state persistence, focus, and
restricted OS URL opening. The React SPA owns navigation, task creation, overlay/document close
semantics, and update presentation in the existing settings page. Notification transport does not
infer SPA navigation from generic desktop focus or activation events.

The two sides communicate through a narrow, versioned desktop bridge exposed only to the
desktop-owned loopback origin. Menu actions are typed events; updater and native integration
commands accept bounded structured inputs. The bridge does not expose arbitrary shell execution,
filesystem access, unrestricted URL schemes, or a generic plugin pass-through.

Tauri capabilities must allow `http://127.0.0.1:*` because the owned backend port is selected at
launch. That static wildcard does not establish trust by itself: before each privileged updater,
native-notification, or external-link command, Rust verifies that the invoking WebView is at the
exact loopback origin recorded after the health-token-verified backend startup. The local bootstrap
page has no desktop capability, and other loopback services are denied.

`Cmd/Ctrl+W` is a Close Context command, not a native window-close accelerator. The SPA closes one
topmost dismissible overlay, otherwise one closable document tab using its normal dirty-state
guard, otherwise nothing. The macOS red close control and Quit both stop the owned backend and exit
the app. Reopen/focus applies to second-instance activation and Dock activation while the process
is alive; the app does not remain hidden after red-close quit.

Desktop updates use Tauri's signed static-manifest updater with a dedicated updater signing key.
GitHub Releases publishes `latest.json` and the platform updater artifacts. macOS uses
`.app.tar.gz`, Linux uses `.AppImage.tar.gz`, and Windows uses the NSIS updater bundle. Linux
AppImage joins the existing `.deb` and `.rpm` release packages; only AppImage is updated in-app.
The app checks on startup and periodically, but download/install/restart always requires explicit
confirmation in the existing System > Updates page. The backend is cleanly stopped before updater
relaunch.

Desktop-owned launches reuse existing notification preferences and events but route waiting-for-
input and session-failure delivery through the native notifier. They suppress the legacy backend
shell-command and Web Notification delivery for those same events so one event yields at most one
native notification. Browser, CLI, and service launches retain their existing channels. Dock
reopen and second-instance activation focus the window but never infer a notification route. The
upstream desktop notification API cannot correlate desktop body clicks with a custom payload, so
the app does not navigate from generic focus or activation events.

External `http`, `https`, and `mailto` links use a scoped native opener. Loopback app navigation,
downloads, and blob URLs remain in the WebView.

## Consequences

Native behavior is centralized where OS lifecycle and cryptographic verification can be tested,
while product context remains in the existing frontend. The loopback-served SPA needs an explicit
capability rather than inheriting Tauri access automatically, but its authority stays small and
auditable. Desktop notification startup must identify the owned backend so duplicate transports
can be suppressed without changing service/browser behavior.

The release workflow gains updater artifacts, signatures, a static manifest, AppImage builds, and
a distinct signing-key rotation responsibility. Shared Tauri files (`Cargo.toml`, capabilities,
configuration, and `main.rs`) must be changed sequentially across implementation waves to avoid
integration conflicts.

## Alternatives Considered

- **Handle every shortcut in the WebView.** Rejected because standard application menus and
  lifecycle commands must work consistently at the native window boundary.
- **Let Rust infer which React surface to close.** Rejected because native code cannot reliably
  model overlay stacking, document types, or unsaved frontend state.
- **Expose broad Tauri plugin APIs to the loopback page.** Rejected because a compromised SPA or
  injected script would gain unnecessary host authority.
- **Keep backend shell notifications alongside native notifications.** Rejected because desktop
  users would receive duplicate alerts with inconsistent activation behavior.
- **Silently install signed updates.** Rejected because an update restarts active local sessions
  and the user explicitly requires confirmation.
- **Auto-update `.deb` and `.rpm` files directly.** Rejected because those installations are owned
  by their package managers; AppImage is the supported Linux in-app update channel.
