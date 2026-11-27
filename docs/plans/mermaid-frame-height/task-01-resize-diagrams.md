---
id: "01-resize-diagrams"
title: "Resize diagram viewports"
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-UI-MERMAID-RENDERING-004
system_design:
  - ../../specs/ui/system-design/mermaid-frame-height.md
acceptance_criteria:
  - AC-UI-MERMAID-RENDERING-004.1
  - AC-UI-MERMAID-RENDERING-004.2
  - AC-UI-MERMAID-RENDERING-004.3
---

# Resize diagram viewports

## Interaction preview

UI-01 shows the diagram in chat or a plan. The zoom toolbar stays at the upper right. The bottom strip stays within reach when the drawing scrolls.

```text
+-----------------------+
|            - 100% +   |
|   Release workflow    |
|   Review -> Tests     |
+----------===----------+
Following paragraph
```

Dragging the grip down reveals additional rows above the strip. Touch uses a taller hit area around the same grip.

## Implementation and checks

Follow the [plan](plan.md) and [interaction design](../../specs/ui/system-design/mermaid-frame-height.md). Extend MermaidBlock, MermaidNodeView, plan diagram CSS and the supported locale catalogs.

1. Extend the existing desktop Mermaid fit spec to capture a drag across the bottom border. Assert unchanged SVG scale, toolbar offset and page zoom, increased frame height, and independent zoom and height resets.
2. Verify keyboard sizing and reset in both plan modes. Check the saved plan content and revision count before and after resizing.
3. Extend the mobile Mermaid fit spec to drag the grip and scroll the drawing separately. Run both Chromium projects from apps/web with `pnpm e2e:run -- e2e/tests/chat/mermaid-fit.spec.ts` and `pnpm e2e:run --no-build --project mobile-chrome -- e2e/tests/chat/mobile-mermaid-fit.spec.ts`. Run `pnpm run typecheck`.

## Results

Implemented for chat and both plan modes. Desktop Chromium verified drag, keyboard resize, reset, zoom isolation and preserved plan revisions. Mobile Chrome emulation verified touch drag and tap-to-reset. All 30 focused component and hook tests passed. Physical device testing is pending.
