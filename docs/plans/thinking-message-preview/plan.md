---
created: 2026-08-30
status: implemented
requirements:
  - REQ-UI-THINKING-MESSAGE-PREVIEW-001
system_design:
  - ../../specs/ui/system-design/thinking-message-preview.md
legacy_specs: []
---

# Implementation Plan: Preview Collapsed Thinking Messages

## Overview

Derive a model-agnostic preview from the first meaningful reasoning line and
show it in expandable thinking-message headers. Implement the behavior as one
frontend TDD slice because the derivation, row layout, component evidence, and
mobile overflow proof share one component contract.

## Scope

### In scope

- Skip leading blank or decoration-only reasoning lines.
- Strip inline Markdown from the selected preview line.
- Keep the localized Thinking label and add a one-line truncated preview for
  expandable messages.
- Preserve compact single-line and expanded Markdown behavior; compact text
  remains complete and may wrap on narrow rows.
- Prove streaming stability and desktop/mobile containment.

### Out of scope

- Backend, ACP, persistence, WebSocket, and message-schema changes.
- Provider-specific preview parsing or generated summaries.
- New settings, copy, localization keys, or public documentation.
- Expandable-row interaction and accessibility redesign.

## Technical approach

### Preview helper and component

- Add a pure first-meaningful-line helper near `stripMarkdown` in
  `apps/web/components/task/chat/messages/thinking-message.tsx`.
- Derive the preview from `metadata.thinking` or the current content fallback
  on every render.
- Keep the current compact single-line predicate and expandable-content
  condition.
- Render the preview for expandable content instead of limiting header text to
  compact messages.
- Give the header and preview flex regions `min-w-0`, keep the localized label
  non-shrinking, truncate only the expandable preview to one visual line, and
  keep compact text in a shrinkable wrapping region.

### Component regression tests

- Add
  `apps/web/components/task/chat/messages/thinking-message.test.tsx`.
- First write a failing test for a Codex-shaped source containing leading blank
  lines, a bold first summary, and later details.
- Cover the empty-preview fallback, conservative inline Markdown stripping,
  compact single-line preservation and wrapping classes, rerendered appended
  content, expansion, and truncation classes.

### Mobile browser evidence

- Extend `apps/web/e2e/tests/chat/mobile-markdown-wrap.spec.ts` with a focused
  preview scenario.
- Seed leading blank lines, a long bold summary, and a later detail line through
  the existing mock-agent thinking command.
- Require the first summary in the collapsed row, reveal the later detail only
  after expansion, and assert that the preview truncates without chat or
  document horizontal overflow.

## Tests

| Acceptance criterion                   | Test evidence                                                                                           |
| -------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| `AC-UI-THINKING-MESSAGE-PREVIEW-001.1` | Component and mobile browser tests require the first meaningful line in the collapsed header.           |
| `AC-UI-THINKING-MESSAGE-PREVIEW-001.2` | Component tests cover leading blank, decoration-only, and empty content.                                |
| `AC-UI-THINKING-MESSAGE-PREVIEW-001.3` | A component rerender test appends a later summary and keeps the original preview.                       |
| `AC-UI-THINKING-MESSAGE-PREVIEW-001.4` | Header class assertions and the mobile browser geometry scenario prove expandable one-line containment. |
| `AC-UI-THINKING-MESSAGE-PREVIEW-001.5` | Component and mobile browser tests expand the row and find the complete later content.                  |
| `AC-UI-THINKING-MESSAGE-PREVIEW-001.6` | A component test preserves compact inline and non-expandable rendering.                                 |
| `AC-UI-THINKING-MESSAGE-PREVIEW-001.7` | The pure helper uses only the provider-neutral thinking string.                                         |

## E2E tests

- Mobile: run the new thinking-preview scenario in
  `apps/web/e2e/tests/chat/mobile-markdown-wrap.spec.ts` with the
  `mobile-chrome` project. This proves the shared component's visible preview,
  expansion path, expandable-preview truncation, compact full-text wrapping,
  and containment on the narrowest supported chat surface.
- Desktop behavior is covered by the shared component test because the change
  does not branch by viewport or pointer mode.

## Mobile design contract

- Desktop outcome: an expandable thinking row shows its subject before the
  complete Markdown block is opened.
- Mobile entry point: the same thinking row in the existing task Chat tab.
- Nearest exemplar: the thinking-message scenario in
  `apps/web/e2e/tests/chat/mobile-markdown-wrap.spec.ts`.
- Hierarchy and surface: the localized label remains first; the preview is
  secondary inline content; the existing transcript remains the scroll owner.
- Presentation rationale: this shallow scan aid belongs in the existing row;
  a drawer, new route, or separate surface would add interaction without new
  content depth.
- Shared logic: desktop and mobile use the same preview derivation, expansion
  state, and complete Markdown content.
- Mobile proof: the new `mobile-chrome` scenario verifies preview visibility,
  expansion, expandable-preview truncation, compact full-text wrapping, and
  the absence of document overflow.

## Work orders

- [x] [Task 01: Render thinking-message previews](task-01-render-thinking-message-previews.md)

## Verification results

Implementation is complete. The exact task verification commands passed:

- Thinking-message component tests: 7 passed.
- Web typecheck, targeted ESLint, and Prettier checks passed.
- Specification tests and all specification files passed lint.
- Mobile Pixel 5 thinking-preview E2E: 2 passed with expandable-preview
  truncation, compact full-text wrapping, and no chat or document overflow.
- Fresh desktop and 393px mobile PR screenshots captured, inspected, and compressed.

## Risks

- A source can begin with Markdown structure rather than a natural-language
  heading. The contract intentionally selects the first visible line instead
  of adding provider-specific or semantic ranking.
- Streaming can initially contain only blank lines. The label-only fallback
  must transition cleanly when the first meaningful content arrives.
- A long preview can widen nested flex content unless every ancestor in the
  header path permits shrinking.
