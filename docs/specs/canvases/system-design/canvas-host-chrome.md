---
status: draft
system: canvases
requirements:
  - REQ-CANVASES-AGENT-WEB-APPS-006
  - REQ-CANVASES-AGENT-WEB-APPS-007
---

# Canvas host chrome design

## Purpose and boundaries

Canvases owns its host presentation and recovery. UI owns reusable toolbar
geometry. The runtime lifecycle and authentication remain unchanged.

## Requirement mapping

| Criteria | Design section |
| --- | --- |
| 006.10, 006.11 | Single canvas host header |
| 006.1-.6, 007.1-.3, 007.5-.6 | Responsive composition and state preservation below |
| 007.5, 007.9, 007.10 | Startup-failure cause propagation |

## Single canvas host header

For AC-CANVASES-AGENT-WEB-APPS-006.10/.11, give `CanvasHostRoute` an explicit
embedded presentation prop from `CanvasContent` in `dockview-panel-content.tsx`.
The direct caller in `apps/web/src/canvas-route.tsx` retains standalone behavior.
Embedded mode uses the existing shared panel header; standalone mode retains
`PageShell` navigation. Do not shrink PageTopbar globally to fix a panel.

Remove the separate `CanvasHostHeader` status row from `CanvasHostBody`. Move
its phone action trigger into the one remaining header, keeping the existing
`MobileCanvasActions` drawer and picker reachable. Preserve the title, release,
share, promotion and edit callbacks. Use compact/overflow presentation for narrow
embedded panels, with the shared [toolbar design](../../ui/system-design/panel-toolbars.md).

Keep loading and blocking-state text in the body exactly once. Ready renders
application content and a visually hidden polite announcement. A nonblocking
connection warning can use an inline header status without adding another row.
Do not remove runtime state transitions, the startup handshake, timeout, Retry,
or release actions. Update tests that locate the removed `canvas-host-state`
row to assert the surviving state presentation and accessible announcement.

The full-height phone route keeps a single content scroll owner and the inset
actions drawer. Derive remaining viewport space from the actual page header;
remove assumptions that subtract a second header. Preserve safe-area handling
and at least 44px action targets. Header restructuring must not remount the
runtime when actions, status, or viewport width change.

Delivery and browser coverage are in
[work order 03](../../../plans/canvas-same-origin-auth/task-03-panel-toolbars.md).

## Startup-failure cause propagation

The guest bootstrap already reports why startup failed. The host must carry that
cause to the state panel instead of collapsing every failure onto the
release-unavailable state.

Name the reported causes in one host-facing union owned by the web-app startup
contract: an application error raised while starting, an unreachable runtime
capability API, a browser document load failure, and an absent acknowledgement at
the startup deadline. The first two come from the guest message; the last two are
observed by the host, which never invents a cause it did not observe.

`WebAppFrame` settles each startup attempt with either readiness or one of those
causes, and its error callback carries the cause. `CanvasPage` forwards it
unchanged. `CanvasHostRoute` records the cause and enters a runtime-startup-failure
state that is separate from the release-unavailable state. Release status keeps
producing the release-unavailable state, so a valid release is never described as
unavailable.

`CanvasHostStatePanel` resolves the runtime-startup-failure description from the
recorded cause and keeps the existing Retry and release actions. Host request
failures keep their current state and continue to display the returned error text.

## Verification

Work order 03 defines component and browser evidence for desktop Dockview,
standalone navigation, phone actions, state announcements, and Retry. Runtime
startup and authentication checks remain in work orders 01 and 02.
No backend, persistence, permission, or observability changes are introduced.

Startup-failure cause propagation is verified by component tests in
`web-app-frame.test.tsx`, `canvas-host-route.test.tsx`, and
`canvas-host-components.test.tsx`. No backend change is introduced.
