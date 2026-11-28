---
created: 2026-11-26
status: implemented
requirements:
  - REQ-UI-MERMAID-RENDERING-004
system_design:
  - ../../specs/ui/system-design/mermaid-frame-height.md
legacy_specs: []
---

# Adjust diagram frame height

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

## Delivery

[Implement and verify the resize grip](task-01-resize-diagrams.md) in one work order. The existing local zoom handler supplies the starting viewport. Test editor selection and scrolling on touch, where a grip can otherwise intercept normal navigation.
