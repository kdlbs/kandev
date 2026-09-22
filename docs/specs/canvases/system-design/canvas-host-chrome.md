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

## Verification

Work order 03 defines component and browser evidence for desktop Dockview,
standalone navigation, phone actions, state announcements, and Retry. Runtime
startup and authentication checks remain in work orders 01 and 02.
No backend, persistence, permission, or observability changes are introduced.
