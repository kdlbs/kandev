---
created: 2026-11-26
status: implemented
requirements:
  - REQ-UI-MERMAID-RENDERING-003
system_design:
  - ../../specs/ui/system-design/mermaid-pinch-zoom.md
legacy_specs: []
---

# Keep trackpad zoom inside a diagram

A pinch previously reached the browser's default zoom action. Source inspection found separate button-driven scale paths in chat Markdown and Tiptap plans. A single work order covers both paths.

1. [Handle and verify local pinch gestures](task-01-diagram-pinch.md). This work has no prerequisite changes.

The main risk is native browser gesture handling. Use focused event tests for cancellation and scale updates, then verify both entry points with a physical trackpad. The existing toolbar and phone layout serve both input methods without additional controls.

## Interaction preview

Before and after a pinch in either surface, the reader sees the same toolbar. For example, a diagram at 100% can reach 125% while the surrounding paragraph keeps its text size.


### UI-01. Zoomed diagram in agent output or Plan

```text
+-----------------------------+
|               [- 125% +]    |
|   Review --> Tests --> ...  |
+-----------------------------+
Following paragraph
```

The drawing scrolls inside the frame. The frame edges and toolbar stay fixed during zoom.

## Status

Implemented in chat and both plan modes. Chromium gesture, desktop, and emulated phone checks passed. Physical trackpad and phone checks remain unverified.
