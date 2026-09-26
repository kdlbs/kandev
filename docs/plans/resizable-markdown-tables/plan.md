---
spec: docs/specs/ui/requirements/resizable-markdown-tables.md
created: 2026-08-05
status: complete
---

# Implementation Plan: Resizable Markdown Table Columns

## Overview

Add ephemeral, accessible column resizing to the shared Markdown table renderer
without weakening the wrapping repair already covered by chat browser tests.
Desktop fine-pointer users receive full-height inline separators; phone and
coarse-pointer layouts retain automatic wrapping and local scrolling with no
resize controls.

## Frontend design

### Width geometry

Add a small pure geometry module under `apps/web/lib/markdown/` that adjusts an
adjacent width pair while preserving its sum and enforcing the 64-pixel minimum.
Use the same function for pointer and keyboard input so clamping behavior has one
testable source of truth. If the pair totals less than 128 pixels, leave both
widths unchanged because the total-width and dual-minimum constraints cannot
otherwise both hold.

### Resizable table component

Extract the shared renderer's current table wrapper into a client component,
likely `apps/web/components/shared/resizable-markdown-table.tsx`.

- Render the semantic `<table>` unchanged in automatic mode.
- Measure the table and its header cells after layout. A `ResizeObserver` keeps
  separator positions aligned while automatic content or container geometry
  changes, and controlled column-width updates explicitly trigger another
  measurement even when the table's outer box is unchanged. Observe character
  data as well as child-list mutations so streamed text updates also remeasure.
- Place focusable `role="separator"` controls in an overlay within the table's
  scroll content. Each control is centered on an internal boundary, spans the
  full table height, and scrolls with the table. Omit a boundary while either
  adjacent measured column is below the 64-pixel minimum.
- Give each separator an adjacent-column accessible name, vertical orientation,
  a 64-pixel minimum, the rounded current left-column width, and a maximum equal
  to the pair total minus the right-column minimum. Verify those rendered ARIA
  semantics in Playwright alongside keyboard behavior.
- On first adjustment, snapshot measured widths and apply them through a
  `<colgroup>` with fixed table layout. Keep the measured table width constant so
  one adjacent column grows exactly as the other shrinks. Do not activate fixed
  widths until pointer movement has a non-zero delta.
- Use pointer capture for dragging. Restore the drag-start snapshot on
  `pointercancel`, and clean up cursor and selection overrides on every exit
  path.
- Double-click or `Enter` clears the custom width state and returns the table to
  CSS automatic layout. A changed column count does the same.
- Render controls only when the responsive capability is non-phone and
  fine-pointer. Tables that cannot provide valid multi-column geometry stay
  automatic. A capability change during a drag also clears the active drag and
  document-level cursor and selection overrides.

### Styling and localization

Add narrowly scoped Markdown-table classes in `apps/web/app/globals.css` for the
relative stage, separator hit area, and hover/focus/active line. Preserve the
existing `overflow-x-auto` wrapper and the two-/three-column wrapping and
four-plus-column minimum-width policies.

Add the separator's accessible name to the shared English locale and access it
through `t()`; no visible copy or public documentation is added.

## Responsive and mobile implementation

The nearest mobile exemplar is the current task-chat Markdown table tested by
`mobile-markdown-wrap.spec.ts`. The component shares parsing, measurement-safe
markup, wrapping, and the local scroll owner at every viewport. It suppresses
the separator overlay on phones and coarse pointers because a usable touch hit
target would obstruct narrow table content. Mobile capability remains complete
for reading and scrolling; only the optional precision adjustment is desktop
specific.

## Tests

### Unit tests

- RED: adjacent-pair resizing grows and shrinks by equal amounts.
- RED: both drag directions clamp at 64 pixels without changing the pair total.
- RED: keyboard deltas use the same geometry behavior.
- Cover the resize hook's keyboard branches, capability-disable reset, and
  column-count-change reset in a focused hook test.
- GREEN: implement the geometry module and table component, then wire it into
  `markdown-components.tsx`.
- The repository does not add isolated React component tests for pure UI.
  Playwright covers separator semantics, rendered geometry, reset behavior, and
  responsive capability gating through the real shared renderer.

Likely files:

- `apps/web/lib/markdown/table-resize.ts`
- `apps/web/lib/markdown/table-resize.test.ts`
- `apps/web/components/shared/resizable-markdown-table.tsx`
- `apps/web/components/shared/use-markdown-table-resize.ts`
- `apps/web/components/shared/use-markdown-table-resize.test.ts`
- `apps/web/components/shared/markdown-components.tsx`
- `apps/web/app/globals.css`
- `apps/web/src/locales/en/common.json`

### Browser tests

Extend the existing chat Markdown wrapping suites rather than creating a second
fixture path.

- Desktop: render a three-column table, start a drag at a body-row point on the
  full-height boundary, and assert adjacent widths change, the untouched column
  and total width remain stable, and minimum-width clamping works.
- Desktop: assert double-click returns near the initial automatic proportions;
  assert arrow keys resize and `Enter` resets.
- Desktop wide table: scroll its local wrapper and assert separators remain
  aligned and no chat/document overflow appears.
- Mobile Chrome: assert no separators are exposed while the existing ordinary
  wrapping and wide-table local scrolling checks still pass.

Likely files:

- `apps/web/e2e/tests/chat/markdown-wrap.spec.ts`
- `apps/web/e2e/tests/chat/mobile-markdown-wrap.spec.ts`

## Implementation waves

Execution is sequential in the primary conversation. The E2E work depends on
the renderer contract, and this package does not authorize subagent use.

- [x] [Task 01: Build the ephemeral resize renderer](task-01-build-resize-renderer.md)
- [x] [Task 02: Prove desktop interaction and mobile parity](task-02-prove-resize-interaction.md)

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps && pnpm --filter @kandev/web exec vitest run \
  lib/markdown/table-resize.test.ts \
  components/shared/use-markdown-table-resize.test.ts \
  components/shared/markdown-components.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps && pnpm --filter @kandev/web lint)
make build-web
(cd apps/web && pnpm e2e:run tests/chat/markdown-wrap.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-markdown-wrap.spec.ts)
```

## Documentation impact

The durable UI spec and plan cover the new behavior. No public CLI, config, API,
or workflow documentation changes are required.

## Risks

- Separator geometry must use the scroll content's coordinate system; viewport
  coordinates would drift after horizontal scrolling.
- Streaming updates can change row height or column count after initial render.
  Observation must update positions, while structure changes must discard custom
  widths.
- Fixed widths must activate only after user adjustment. Applying them on first
  render would regress responsive automatic wrapping.
- Pointer cancellation and unmount must restore global cursor and text-selection
  state to avoid a sticky resize cursor elsewhere in the app.
- Component tests run in jsdom, so deterministic width math belongs in a pure
  helper and real geometry assertions belong in Playwright.

## Open questions

None.

## Verification results

- `vitest`: 42 focused geometry/hook/shared-renderer tests passed.
- `pnpm run typecheck`: passed.
- `pnpm --filter @kandev/web lint`: passed with zero warnings.
- `make build-web`: passed.
- Desktop `markdown-wrap.spec.ts`: 8 tests passed.
- Mobile Chrome `mobile-markdown-wrap.spec.ts`: 2 tests passed.
- `git diff --check`: passed.
