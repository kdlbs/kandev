---
status: current
system: ui
created: 2026-11-26
requirements:
  - REQ-UI-MERMAID-RENDERING-004
owners:
  - kandev
---

# Diagram frame height

In Plan, the viewport and resize strip span the available text width. Center artwork that fits and allow horizontal scrolling when zoomed artwork exceeds that width. Anchor the toolbar to the frame and keep the zoom percentage on one line.

Place a shallow strip with a centered horizontal grip directly below the scrollable drawing. Keep it outside the scroll region and separate from the zoom toolbar. The entire strip accepts a vertical drag. Reuse the sidebar navigation separator's focus ring, row-resize cursor and coarse-pointer target size (44 CSS pixels).

Start at the existing natural height for small diagrams. For tall diagrams, limit the initial drawing viewport to half the available viewport height so the reader can reach its lower edge. Dragging down reveals more at the current scale and moves following prose down. Dragging up reclaims space. Use a 64px minimum for a manually sized viewport and an upper bound of the greater of the natural scaled drawing height, available panel height, and current height at drag start. Allow expansion after zooming in. Keep normal scrolling available inside the frame.

Use pointer capture on the grip and measure vertical movement from the starting height. Apply touch-action: none only to the grip. Bind a focusable horizontal separator to the controlled viewport, with localized accessible name and size values. Arrow Down and Arrow Up adjust by 16px. Enter or double-click restores automatic height. Keep the zoom reset independent. On touch, a tap on the grip offers a reset-to-auto action without requiring double-tap.

React MermaidBlock and Tiptap MermaidNodeView should share the bounds and input behavior. Store the height per mounted diagram. Preserve it across zoom, pan, editor updates to the same node, and source toggles. Hide the grip in source and error views. Release pointer capture and listeners on teardown. In the editor, resize the presentation DOM without dispatching a document transaction or moving the caret.

## Verification

Check the separator with mouse, keyboard and touch. Measure frame and artwork bounds separately. Use a tall flowchart and a wide sequence diagram. Cover both plan modes, the chat renderer, a narrow panel and a phone-sized viewport.
