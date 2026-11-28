---
status: current
system: ui
created: 2026-11-26
requirements:
  - REQ-UI-MERMAID-RENDERING-003
owners:
  - kandev
---

# Mermaid trackpad pinch zoom

## Gesture ownership

[Requirement 003](../requirements/mermaid-rendering.md#req-ui-mermaid-rendering-003-trackpad-pinch-zoom) applies to two renderers. Chat uses the React MermaidBlock. The editable and read-only plan editors use the Tiptap MermaidNodeView. Each owns its scale and handles button clicks. Both bind pinch input to their diagram viewport.

The renderers share one DOM event binding between the two renderers. Attach it to the diagram scroll region when it displays an SVG. Use a native non-passive listener to cancel the browser default synchronously for a Ctrl-modified wheel event, including at the scale limits. Convert wheel delta units to a bounded proportional change. Leave unmodified wheel events alone. For WebKit trackpad gesture events, cancel the native gesture from its start and apply incremental ratios between successive scale readings. Consume each physical update once on engines with overlapping event streams.

Feed each factor into the current local scale using the existing minimum and maximum. Chat uses a functional scale update and enters manual zoom mode. Its reset restores fit-to-width. The plan node view calls its existing setScale method. Its reset restores its default scale. Both paths update the SVG, wrapper dimensions, and percentage through the same state as the buttons.

Size the viewport from the initial fit in chat and the default scale in plans. Only the inner SVG and its scrollable footprint follow manual zoom. Keep the controls outside the scroll region, with space for a three-digit percentage.

Release listeners on element replacement or unmount and in the node view destroy lifecycle. Limit gesture handling to trackpad input over the visible diagram. Keep native touchscreen page zoom, source selection, and ordinary horizontal and vertical scrolling available. No document-level listener or viewport setting is required.

## Verification

Exercise both views with two diagrams to detect shared state. Check browser default cancellation at both limits. Verify actual page scale and surrounding text on a physical trackpad in Chromium and Safari because synthetic events alone cannot demonstrate browser zoom suppression.
