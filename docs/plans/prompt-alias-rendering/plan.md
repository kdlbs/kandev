---
status: pending
initiative: prompt-alias-rendering
requirements:
  - docs/specs/ui/requirements/prompt-alias-rendering.md
system_design:
  - docs/specs/ui/system-design/prompt-alias-rendering.md
---

# Prompt alias rendering

## Overview

Make recognized saved-prompt `@name` aliases render consistently as prompt chips
in the transcript, the desktop anchored last-prompt bar, and desktop/mobile
Prompt history without changing prompt delivery or persistence.

## Delivery order

1. Extract the existing transcript prompt-chip renderer into a reusable UI
   module, keeping the existing matcher and entity-reference composition intact.
2. Wire the anchored last-prompt bar and Prompt history rows to that shared
   renderer.
3. Add focused component tests for recognized and unknown aliases in both
   surfaces, then extend the existing chat/pinned/history E2E coverage with one
   end-to-end alias assertion where the seeded prompt is visible.

The work is one vertical frontend slice with no dependency on backend changes.

## Work orders

- [task-01-render-prompt-aliases.md](task-01-render-prompt-aliases.md)

## Verification strategy

- Focused Vitest coverage for the shared renderer, `ChatMessage`,
  `AnchoredLastPromptBar`, and `PromptHistoryPanelContent`.
- Existing prompt alias and last-prompt Playwright flows, extended to assert
  the chip in the pinned and history surfaces.
- Frontend typecheck and lint after the targeted tests pass.

## Risks and exclusions

- Prompt history currently uses a single-line text projection; the fix must not
  replace that layout with a second Markdown layout or break overflow detection.
- Pinned content has no message metadata, so it reuses alias rendering but does
  not gain entity-reference chips.
- No changes to alias parsing, backend expansion, prompt persistence, or raw
  message rendering.
