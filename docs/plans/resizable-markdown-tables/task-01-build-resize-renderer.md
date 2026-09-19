---
id: "01-build-resize-renderer"
title: "Build the ephemeral Markdown table resize renderer"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/resizable-markdown-tables.md"
---

# Task 01: Build the Ephemeral Resize Renderer

## Acceptance

- Shared Markdown tables render accessible full-height internal separators only
  on non-phone fine-pointer layouts with valid multi-column geometry.
- Every separator exposes its adjacent-column accessible name, vertical
  orientation, 64-pixel minimum, rounded current left-column width, and the
  pair-derived maximum through the complete ARIA separator contract.
- Pointer and keyboard adjustment resize only adjacent columns, preserve the
  table width, and enforce a 64-pixel minimum.
- Boundaries with either adjacent measured column below 64 pixels are omitted;
  adjacent pairs below 128 pixels therefore remain unchanged without exposing
  an invalid or immovable separator.
- Double-click and `Enter` clear all custom widths and restore CSS automatic
  layout; unmount, reload, and column-count changes retain no state.
- Existing two-/three-column wrapping and four-plus-column local scrolling CSS
  remains authoritative before the first user adjustment.
- Pointer cancellation restores drag-start widths and all pointer/cursor/
  selection cleanup paths are covered.

## TDD sequence

1. RED: add pure tests for equal-and-opposite width changes, both clamp
   directions, unchanged pair totals, and 8-pixel keyboard deltas.
2. RED: add a browser regression proving the shared renderer has no resize
   separator before production behavior changes.
3. GREEN: implement the geometry helper and resizable table component. Keep
   pure UI behavior in Playwright instead of adding isolated React tests.
4. GREEN: replace the shared Markdown table wrapper, add scoped styles and the
   localized accessible name, then rerun the existing renderer suite.
5. REFACTOR: keep DOM measurement and pointer lifecycle local to the component;
   do not leak resizing concerns into Markdown normalization or message stores.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps && pnpm --filter @kandev/web exec vitest run \
  lib/markdown/table-resize.test.ts \
  components/shared/use-markdown-table-resize.test.ts \
  components/shared/markdown-components.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps && pnpm --filter @kandev/web lint)
git diff --check
```

## Files likely touched

- `apps/web/lib/markdown/table-resize.ts`
- `apps/web/lib/markdown/table-resize.test.ts`
- `apps/web/components/shared/resizable-markdown-table.tsx`
- `apps/web/components/shared/use-markdown-table-resize.ts`
- `apps/web/components/shared/markdown-components.tsx`
- `apps/web/app/globals.css`
- `apps/web/src/locales/en/common.json`

## Dependencies

None.

## Parallelism

`sequential`. Measurement behavior, component semantics, and shared renderer
wiring must stay in one RED-GREEN cycle.

## Inputs

- `docs/specs/ui/requirements/resizable-markdown-tables.md`
- Existing table wrapper in `apps/web/components/shared/markdown-components.tsx`
- Existing wrapping rules in `apps/web/app/globals.css`
- Fine-pointer and phone capability in `apps/web/hooks/use-responsive-breakpoint.ts`
- Pointer-capture cleanup patterns in `apps/web/components/app-status-bar/`

## Output contract

Record the expected RED failures, focused GREEN command results, files changed,
and the rendered geometry proven in Playwright.

## Result

- RED: three geometry assertions failed against the no-op width helper; the
  desktop Playwright regression failed because no separator existed.
- GREEN: 32 focused geometry/shared-renderer tests passed; TypeScript, frontend
  lint, and `git diff --check` passed.
- The renderer now provides fine-pointer separators, pointer capture, 64-pixel
  adjacent clamping, keyboard adjustment, ephemeral reset, structure-change
  invalidation, and cleanup on cancellation/unmount.
- Real table geometry and responsive behavior are covered by Task 02.
