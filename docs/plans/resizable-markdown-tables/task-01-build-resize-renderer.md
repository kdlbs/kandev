---
id: "01-build-resize-renderer"
title: "Build the ephemeral Markdown table resize renderer"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/resizable-markdown-tables.md"
---

# Task 01: Build the Ephemeral Resize Renderer

## Acceptance

- Shared Markdown tables render accessible full-height internal separators only
  on non-phone fine-pointer layouts with valid multi-column geometry.
- Pointer and keyboard adjustment resize only adjacent columns, preserve the
  table width, and enforce a 64-pixel minimum.
- Double-click and `Enter` clear all custom widths and restore CSS automatic
  layout; unmount, reload, and column-count changes retain no state.
- Existing two-/three-column wrapping and four-plus-column local scrolling CSS
  remains authoritative before the first user adjustment.
- Pointer cancellation restores drag-start widths and all pointer/cursor/
  selection cleanup paths are covered.

## TDD sequence

1. RED: add pure tests for equal-and-opposite width changes, both clamp
   directions, unchanged pair totals, and 8-pixel keyboard deltas.
2. RED: add component tests for separator semantics, capability gating, reset,
   pointer cancellation, and column-count invalidation using controlled geometry.
3. GREEN: implement the geometry helper and resizable table component.
4. GREEN: replace the shared Markdown table wrapper, add scoped styles and the
   localized accessible name, then rerun the existing renderer suite.
5. REFACTOR: keep DOM measurement and pointer lifecycle local to the component;
   do not leak resizing concerns into Markdown normalization or message stores.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps && pnpm --filter @kandev/web exec vitest run \
  lib/markdown/table-resize.test.ts \
  components/shared/resizable-markdown-table.test.tsx \
  components/shared/markdown-components.test.tsx
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
git diff --check
```

## Files likely touched

- `apps/web/lib/markdown/table-resize.ts`
- `apps/web/lib/markdown/table-resize.test.ts`
- `apps/web/components/shared/resizable-markdown-table.tsx`
- `apps/web/components/shared/resizable-markdown-table.test.tsx`
- `apps/web/components/shared/markdown-components.tsx`
- `apps/web/app/globals.css`
- `apps/web/src/locales/en/common.json`

## Dependencies

None.

## Parallelism

`sequential`. Measurement behavior, component semantics, and shared renderer
wiring must stay in one RED-GREEN cycle.

## Inputs

- `docs/specs/ui/resizable-markdown-tables.md`
- Existing table wrapper in `apps/web/components/shared/markdown-components.tsx`
- Existing wrapping rules in `apps/web/app/globals.css`
- Fine-pointer and phone capability in `apps/web/hooks/use-responsive-breakpoint.ts`
- Pointer-capture cleanup patterns in `apps/web/components/app-status-bar/`

## Output contract

Record the expected RED failures, focused GREEN command results, measured DOM
contract used by the component tests, files changed, and any residual geometry
that must be proven in Playwright.
