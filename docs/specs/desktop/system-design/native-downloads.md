---
status: current
system: desktop
requirements:
  - REQ-DESKTOP-NATIVE-DOWNLOADS-001
created: 2026-09-25
owners:
  - kandev
---

# Native Downloads System Design

## Purpose and boundaries

The Tauri shell owns the destination and completion of downloads initiated by
its owned WebView. Existing backend routes and React export actions own the
file bytes, names, authentication, and feature-specific preparation. The web
browser and phone continue using their normal download behavior.

## Requirement mapping

| Requirement                        | Design section                                                                                 |
| ---------------------------------- | ---------------------------------------------------------------------------------------------- |
| `REQ-DESKTOP-NATIVE-DOWNLOADS-001` | [Desktop download flow](#desktop-download-flow), [Failure and recovery](#failure-and-recovery) |

## Implementation and verification status

`apps/desktop/src-tauri/src/main.rs` constructs the configured main WebView
with a download callback before navigation. The callback checks the owned
origin and uses the non-blocking native Save dialog. After selection, the SPA
retries the same URL through the WebView with the selected destination. The
shell reports selection, cancellation, and transfer completion. The SPA shows
diagnostic-bundle results and global results for other in-app downloads.

The Linux desktop smoke checks startup and readiness only. It does not automate
the native Save panel or compare downloaded bytes. AC-DESKTOP-NATIVE-DOWNLOADS-001.2
remains unverified until a packaged macOS run checks Save, Cancel, and the exact
bytes of HTTP and Blob downloads.

## Desktop download flow

Create the main WebView through `WebviewWindowBuilder::from_config` so its
`on_download` callback is registered before navigation. Disable automatic
creation of the same configured window. Preserve its configured label, size,
visibility, capabilities, and existing startup handoff.

For `DownloadEvent::Requested`, check that the current WebView is the owned,
health-verified Kandev origin. Accept only a download from that same origin or
an object URL created by that origin. Use the engine's suggested destination
only for a safe suggested filename. Reserve the URL and open the existing
native dialog plugin's Save panel asynchronously, parented to the main window;
reject this first request while the user chooses a destination. On selection,
record the absolute destination and notify the SPA to retry the same URL. For
that retry, set the callback's destination and allow the WebView to transfer
the original response bytes. On cancellation, release the reservation and
reject the request. Expire abandoned selections after a bounded interval. Do
not accept a caller-supplied path or a new general filesystem command from the
SPA. The backend keeps enforcing authentication and authorization for HTTP
downloads.

For `DownloadEvent::Finished`, treat `success` as authoritative. On macOS the
event's path can be absent even after success, so correlate completion with the
destination selected at request time. A failed transfer produces a visible
localized error in the SPA or an equivalent native error surface. The frontend
must not claim that the diagnostic ZIP is downloading or saved immediately
after invoking its anchor; it must reflect a pending or finished result in the
desktop path. Cancellation is neutral. Handle consecutive and simultaneous
requests without mixing their filenames or completion results.

The source audit found these first-party entry points:

| Source                                                                   | Transfer form                           |
| ------------------------------------------------------------------------ | --------------------------------------- |
| System Logs bundle and System Backups                                    | authenticated HTTP anchor               |
| Office task documents                                                    | authenticated HTTP anchor               |
| Task files, file viewer, and full chat text                              | object URL through `file-download.ts`   |
| Automation ZIP and canvas bundle/source                                  | fetched Blob through `file-download.ts` |
| Selected Office configuration, organization chart SVG, agent memory JSON | local Blob and object URL               |

Preserve desktop object URLs through the native terminal event. If completion
feedback does not arrive, use bounded cleanup for an abandoned attempt. Keep
the browser download helper's existing behavior. Do not divert the desktop
updater's signed package transfer or links opened in the system browser.

## Failure and recovery

Register the diagnostic result listener before triggering its download. If
listener setup fails, do not start the request; show a retryable error. A
missing terminal result has a bounded transfer timeout. While the Save panel is
open, use a longer bounded selection timeout so a user taking time to choose a
location does not receive a premature failure. If the user cancels or a
transfer fails, release the reservation and object URL without success copy.
Never silently write to a fallback directory. Test the browser path separately
to catch regressions in HTTP and Blob downloads.

## Validation

Rust tests cover origin and URL checks, safe suggested names, cancellation,
selection, and completion correlation. A desktop integration smoke covers both
HTTP and Blob sources, cancellation, and bytes at the chosen destination. Run
the full user flow on macOS because Linux WebKit and macOS WKWebView can differ.
Existing browser and mobile Playwright download tests cover unchanged web
behavior; add focused coverage where the source audit found an untested flow.

## Related decisions

- [ADR-2026-09-25-native-desktop-downloads](../../../decisions/2026-09-25-native-desktop-downloads.md)
