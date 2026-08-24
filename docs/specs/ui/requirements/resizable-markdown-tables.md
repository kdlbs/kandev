---
status: active
system: ui
created: 2026-08-05
owners:
  - kandev
---
# Resizable Markdown Table Columns Requirements

## Overview

Model-generated Markdown tables often choose column proportions that are valid but inconvenient for the current chat width. Automatic wrapping must remain the safe default, but desktop users need a quick way to rebalance a table without editing the source Markdown or opening another view.

## Requirements

### REQ-UI-RESIZABLE-MARKDOWN-TABLES-001: Resizable Markdown Table Columns

**Intent:** Model-generated Markdown tables often choose column proportions that are valid but inconvenient for the current chat width. Automatic wrapping must remain the safe default, but desktop users need a quick way to rebalance a table without editing the source Markdown or opening another view.

#### Acceptance criteria

- **AC-UI-RESIZABLE-MARKDOWN-TABLES-001.1:** Tables rendered through Kandev's shared GitHub-flavored Markdown renderer expose a resize separator at every internal column boundary on non-phone, fine-pointer layouts.
- **AC-UI-RESIZABLE-MARKDOWN-TABLES-001.2:** Each separator has a narrow visual line and a forgiving hit area spanning the full rendered table height, including the header and body.
- **AC-UI-RESIZABLE-MARKDOWN-TABLES-001.3:** Dragging a separator changes only the two adjacent columns. Their combined width and the table's total width remain unchanged, and neither column may be reduced below 64 CSS pixels. Separator positions are re-measured after every controlled width change and streamed cell-text mutation so the drag target remains on the rendered boundary.
- **AC-UI-RESIZABLE-MARKDOWN-TABLES-001.4:** A separator is exposed only while both adjacent measured columns meet the 64-pixel minimum. This omits every pair measuring less than 128 CSS pixels and keeps each exposed separator's current ARIA value inside its declared range.
- **AC-UI-RESIZABLE-MARKDOWN-TABLES-001.5:** The first resize begins from the browser's measured automatic layout. Custom widths activate only after a non-zero pointer movement, then remain in component memory for that mounted table only. Clicking a separator solely to focus it leaves automatic layout active.
- **AC-UI-RESIZABLE-MARKDOWN-TABLES-001.6:** Double-clicking any separator clears every custom width on that table and returns it to the current automatic layout. It does not restore a stale pixel snapshot.
- **AC-UI-RESIZABLE-MARKDOWN-TABLES-001.7:** A focused separator supports `ArrowLeft` and `ArrowRight` in 8-pixel steps. `Enter` resets the whole table. Each control has `role="separator"`, an accessible name identifying its adjacent one-based column numbers, `aria-orientation="vertical"`, `aria-valuemin="64"`, `aria-valuenow` equal to the rounded current left-column width, and `aria-valuemax` equal to the adjacent pair total minus the 64-pixel right-column minimum.
- **AC-UI-RESIZABLE-MARKDOWN-TABLES-001.8:** Hover, keyboard focus, and active dragging make the complete boundary line visible. Normal links, selection, and scrolling remain interactive outside the separator's narrow hit area.

## Migrated source detail

## Why

Model-generated Markdown tables often choose column proportions that are valid
but inconvenient for the current chat width. Automatic wrapping must remain the
safe default, but desktop users need a quick way to rebalance a table without
editing the source Markdown or opening another view.

## What

- Tables rendered through Kandev's shared GitHub-flavored Markdown renderer
  expose a resize separator at every internal column boundary on non-phone,
  fine-pointer layouts.
- Each separator has a narrow visual line and a forgiving hit area spanning the
  full rendered table height, including the header and body.
- Dragging a separator changes only the two adjacent columns. Their combined
  width and the table's total width remain unchanged, and neither column may be
  reduced below 64 CSS pixels. Separator positions are re-measured after every
  controlled width change and streamed cell-text mutation so the drag target
  remains on the rendered boundary.
- A separator is exposed only while both adjacent measured columns meet the
  64-pixel minimum. This omits every pair measuring less than 128 CSS pixels and
  keeps each exposed separator's current ARIA value inside its declared range.
- The first resize begins from the browser's measured automatic layout. Custom
  widths activate only after a non-zero pointer movement, then remain in
  component memory for that mounted table only. Clicking a separator solely to
  focus it leaves automatic layout active.
- Double-clicking any separator clears every custom width on that table and
  returns it to the current automatic layout. It does not restore a stale pixel
  snapshot.
- A focused separator supports `ArrowLeft` and `ArrowRight` in 8-pixel steps.
  `Enter` resets the whole table. Each control has `role="separator"`, an
  accessible name identifying its adjacent one-based column numbers,
  `aria-orientation="vertical"`, `aria-valuemin="64"`, `aria-valuenow` equal to
  the rounded current left-column width, and `aria-valuemax` equal to the
  adjacent pair total minus the 64-pixel right-column minimum.
- Hover, keyboard focus, and active dragging make the complete boundary line
  visible. Normal links, selection, and scrolling remain interactive outside
  the separator's narrow hit area.
- Resize state is ephemeral: it is not stored in local storage, backend state,
  message content, or shared with another rendering of the same Markdown.
- Phone and coarse-pointer layouts do not show or activate resize separators.
  They keep the existing automatic wrapping for ordinary tables and local
  horizontal scrolling for wide tables.
- If resize capability becomes unavailable during a drag, the drag ends and
  any document-level cursor or text-selection override is cleared.
- Table-local scrolling remains the only horizontal scroll owner. Resizing must
  not create document-level or chat-level horizontal overflow.

## Scenarios

- **GIVEN** a three-column Markdown table on desktop, **WHEN** the user drags the
  first separator from a point beside a body row, **THEN** the first column grows,
  the second shrinks by the same amount, and the third column is unchanged.
- **GIVEN** a drag has started, **WHEN** the pointer leaves the table before
  release, **THEN** pointer capture keeps the resize active until release.
- **GIVEN** an adjacent column reaches 64 pixels, **WHEN** the user continues
  dragging toward it, **THEN** that boundary stops moving while the table width
  stays constant.
- **GIVEN** a resized table, **WHEN** the user double-clicks any separator,
  **THEN** all custom widths are removed and the browser recalculates the table's
  automatic layout from current content and available width.
- **GIVEN** a focused separator, **WHEN** the user presses an arrow key,
  **THEN** the adjacent widths change by 8 pixels subject to the same minimum;
  **WHEN** the user presses `Enter`, **THEN** the table resets.
- **GIVEN** a resize is interrupted by pointer cancellation, **WHEN** cancellation
  is received, **THEN** the table returns to the widths captured at drag start.
- **GIVEN** a resized streamed message, **WHEN** its table column count changes,
  **THEN** custom widths are discarded and the new table uses automatic layout.
- **GIVEN** a resized table, **WHEN** it unmounts or the page reloads, **THEN** no
  width preference survives.
- **GIVEN** the same Markdown table on a phone or coarse-pointer device, **WHEN**
  it renders, **THEN** no resize control obstructs the cells and the existing
  wrapping or table-local scrolling behavior remains intact.
- **GIVEN** a wide table that already scrolls locally, **WHEN** it is resized,
  **THEN** separators stay aligned with their boundaries while the table scrolls,
  and the chat and document do not overflow.

## Responsive and mobile contract

- **Desktop entry point:** inline separators on the rendered table; no menu or
  configuration panel.
- **Mobile counterpart:** the existing readable automatic layout. Resizing is an
  optional precision adjustment, so touch users retain full content access
  without controls that would cover narrow cells.
- **Presentation:** the existing inline Markdown table and its local scroll
  wrapper remain authoritative at every breakpoint.
- **Shared behavior:** Markdown parsing, table content, automatic wrapping, and
  wide-table scroll policy are shared. Only the separator affordance is gated to
  non-phone fine pointers.
- **Mobile verification:** phone tests assert that controls are absent, ordinary
  two-column tables wrap, wide tables scroll locally, and neither chat nor the
  document overflows.

## Failure modes

- If column geometry cannot be measured or fewer than two columns are present,
  the table remains in automatic layout and exposes no separators.
- Pointer capture, document cursor, and text-selection overrides are always
  cleaned up after release, cancellation, or unmount.
- A content update that changes the detected column count clears custom widths
  instead of applying stale dimensions to a different structure.

## Out of scope

- Persisting widths across reloads, sessions, messages, or render locations.
- Encoding widths in Markdown or modifying stored message content.
- Touch dragging, a mobile column-width editor, or large touch resize targets.
- Resizing rows, reordering columns, hiding columns, or resizing non-Markdown
  data grids.
- Choosing an exact width from a numeric input.

## Open questions

None.
