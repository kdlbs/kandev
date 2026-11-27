# ADR-2026-09-25-native-desktop-downloads: Save desktop downloads through the WebView

**Status:** accepted
**Date:** 2026-09-25
**Area:** frontend, protocol

## Context

The desktop app displays the same SPA as the browser. Log bundles and other
exports use browser download links, while the desktop shell has no configured
destination or completion handler. A user can click Export and see no save
location or reliable result. File sources include authenticated HTTP responses
and locally generated object URLs.

## Decision

The desktop shell handles downloads from its owned, verified WebView with
Tauri's download callback. It offers a native Save panel and supplies the
user-selected destination to the WebView transfer. The shell does not expose a
general path-based write command to the SPA. Browser and phone downloads keep
their normal browser handling. Desktop updater transfers stay in their signed
update flow.

## Consequences

One destination policy covers current and future in-app HTTP and Blob exports
without copying entire files across the Tauri IPC boundary. The shell must
create its main WebView with the handler installed, correlate completion, and
verify platform behavior on macOS. Failed transfers need a visible result.

## Alternatives considered

- Add a save command to every export action: duplicates handling and risks
  missing new downloads; large files would cross the IPC boundary.
- Keep WebView default downloads: does not offer a dependable destination or
  completion experience in the installed app.
- Ask the backend to write the selected destination: crosses the backend's
  storage boundary and makes browser and remote-backend cases ambiguous.
