---
id: "01-diagram-pinch"
title: "Handle trackpad pinch in both Mermaid renderers"
status: done
wave: 1
depends_on: []
plan: plan.md
acceptance_criteria:
  - AC-UI-MERMAID-RENDERING-003.1
  - AC-UI-MERMAID-RENDERING-003.2
  - AC-UI-MERMAID-RENDERING-003.3
  - AC-UI-MERMAID-RENDERING-003.4
requirements:
  - REQ-UI-MERMAID-RENDERING-003
system_design:
  - ../../specs/ui/system-design/mermaid-pinch-zoom.md
---

# Handle trackpad pinch in both Mermaid renderers

Follow the [gesture design](../../specs/ui/system-design/mermaid-pinch-zoom.md). Extend mermaid-block.tsx, mermaid-block-hooks.ts, and tiptap-mermaid-extension.ts with the shared viewport binding. Keep the editor document and saved plan content independent of zoom state.

1. Add failing coverage for local zoom in each renderer. Check both directions, cancellation at limits, ordinary scroll, source view, and isolation between two diagrams.
2. Implement the binding and listener cleanup. Verify that the percentage and reset follow the [interaction preview](plan.md#interaction-preview) across repeated gestures and renderer remounts.
3. From apps/web run `pnpm exec vitest run components/shared/mermaid-block.test.tsx components/shared/mermaid-block-hooks.test.ts components/editors/tiptap/tiptap-plan-editor.test.tsx`. Run `pnpm e2e:run e2e/tests/chat/mermaid-fit.spec.ts e2e/tests/chat/mobile-mermaid-fit.spec.ts`. Verify pinch on a trackpad in chat and both plan modes, including pinching over surrounding text. Check ordinary touch scrolling on a phone.


### UI-01. Zoomed diagram in agent output or Plan

```text
+-----------------------------+
|               [- 125% +]    |
|   Review --> Tests --> ...  |
+-----------------------------+
Following paragraph
```

The drawing scrolls inside the frame. The frame edges and toolbar stay fixed during zoom.

## Results

Thirty focused tests passed. Desktop Chromium passed both end-to-end cases, including a browser-generated pinch over a rendered chat diagram. The mobile Chrome project passed both existing diagram cases. WebKit event handling and plan zoom passed component tests. On the isolated preview, a Chromium pinch enlarged the artwork while preserving the frame dimensions and toolbar position in chat and plans. Vertical scrolling reached the enlarged content. Physical device checks remain unverified.
