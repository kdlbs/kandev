---
status: active
system: ui
created: 2026-07-27
updated: 2026-09-16
owners:
  - kandev
---

# Adaptive Kanban Requirements

## Overview

People use Kandev in portrait desktop windows, beside a task preview, and with a wide application sidebar. In those situations the Kanban board currently compresses every workflow column until cards and their metadata become difficult to read or escape their card boundaries. The board must remain useful when its own surface is narrow, even when the overall viewport still qualifies as desktop.

## Revision status

The 2026-09-16 revision is current and is implemented by the
[six-card swimlane scrolling plan](../../../plans/kanban-swimlane-scrolling/plan.md).
The six-row sizing and discoverable overflow criteria supersede the previous
200px-to-400px compact-lane cap. The requirement remains active as the product
contract for future Kanban presentation changes.

## Requirements

### REQ-UI-ADAPTIVE-KANBAN-001: Adaptive Kanban

**Intent:** People use Kandev in portrait desktop windows, beside a task preview, and with a wide application sidebar. In those situations the Kanban board currently compresses every workflow column until cards and their metadata become difficult to read or escape their card boundaries. The board must remain useful when its own surface is narrow, even when the overall viewport still qualifies as desktop.

#### Acceptance criteria

- **AC-UI-ADAPTIVE-KANBAN-001.1:** Desktop Kanban composition responds to the rendered board surface width after surrounding app chrome and preview panels consume space; viewport width or portrait orientation alone does not determine whether columns fit.
- **AC-UI-ADAPTIVE-KANBAN-001.2:** A desktop workflow shows every column simultaneously when each column can retain a readable minimum width.
- **AC-UI-ADAPTIVE-KANBAN-001.3:** When every column cannot fit, the workflow becomes a windowed board: complete columns keep their readable minimum width and horizontal overflow stays inside the workflow. The existing column headers and direct lane scrolling remain the desktop navigation; no additional stage selector is shown.
- **AC-UI-ADAPTIVE-KANBAN-001.4:** Existing desktop card interactions remain available in either composition, including pointer drag-and-drop between visible columns, multi-select, context actions, and `Move to` for any step.
- **AC-UI-ADAPTIVE-KANBAN-001.5:** At a fixed viewport size, each Kanban column keeps its mounted card count within the visible vertical area plus a small overscan. The mounted count does not grow with the total task count.
- **AC-UI-ADAPTIVE-KANBAN-001.6:** Users can reach every task by scrolling or searching. Vertical windowing does not change the column count, task order, WIP queue boundary, selection order, or card actions.
- **AC-UI-ADAPTIVE-KANBAN-001.7:** A column with 440 tasks mounts fewer than 100 card bodies during initial display on desktop, tablet, and phone surfaces.
- **AC-UI-ADAPTIVE-KANBAN-001.8:** Opening or resizing the task preview may change the effective desktop composition without changing the user's saved Kanban, Pipeline, workflow, or preview preferences.
- **AC-UI-ADAPTIVE-KANBAN-001.9:** When a pointer drag starts or ends without a change to the rendered workflow steps, each desktop column SHALL retain its computed width.
- **AC-UI-ADAPTIVE-KANBAN-001.10:** A desktop pointer drag MAY extend the workflow's internal scroll range for drag anchoring, but it SHALL NOT widen the document or show a transient horizontal scrollbar solely for that reserve.

### REQ-UI-ADAPTIVE-KANBAN-002: Compact workflow swimlanes

**Intent:** Users can scan several workflows without a viewport of blank space between sparse lanes.

The column area excludes the workflow header and the horizontal scrollbar. Pixel values assume the standard root font.

#### Acceptance criteria

- **AC-UI-ADAPTIVE-KANBAN-002.1:** When several workflow lanes are visible on desktop or tablet, each expanded lane shall fit the tallest initial six-card column segment, with a 200px minimum column area.
  A shorter column contributes all its cards. Card spacing, column headers, and any intervening WIP divider count toward the height.
  The height shall remain stable while users scroll through later cards. Pixel values scale with the root font.
- **AC-UI-ADAPTIVE-KANBAN-002.2:** Columns in one lane shall share a height. Content beyond the height limit shall remain reachable through each column's internal scroll.
- **AC-UI-ADAPTIVE-KANBAN-002.3:** A collapsed lane shall occupy only its header. Other visible lanes shall retain compact sizing, even when only one remains expanded.
- **AC-UI-ADAPTIVE-KANBAN-002.4:** When exactly one workflow lane is visible, its expanded board shall fill the available height. Filters shall determine visibility before sizing.
- **AC-UI-ADAPTIVE-KANBAN-002.5:** Task changes, column visibility changes, and preview resizing shall update compact heights. Empty retained lanes shall keep their header and existing recovery controls.
- **AC-UI-ADAPTIVE-KANBAN-002.6:** Phone Kanban shall retain one focused workflow and column, its navigator, internal scroll, and direct task navigation.
- **AC-UI-ADAPTIVE-KANBAN-002.7:** Height changes shall preserve virtualization, task order, WIP boundaries, drag actions, and saved display preferences. Workflow overflow shall remain inside the board.

### REQ-UI-ADAPTIVE-KANBAN-003: Discoverable swimlane scrolling

**Intent:** Users can scan all workflow lanes with less scrollbar clutter and still discover content outside each visible area.

#### Acceptance criteria

- **AC-UI-ADAPTIVE-KANBAN-003.1:** Expanded workflows shall remain vertically stacked with their existing separators and column order. Each lane shall retain independent horizontal scrolling.
- **AC-UI-ADAPTIVE-KANBAN-003.2:** Overflowing columns shall show a bottom fade at the start, both vertical fades between boundaries, and a top fade at the end. Non-overflowing columns shall show neither fade. Each visible vertical fade shall include a neutral directional chevron that remains visible over empty background.
- **AC-UI-ADAPTIVE-KANBAN-003.3:** Overflowing workflow lanes shall show equivalent left and right fades. Fades shall update after scrolling, resizing, filtering, and task changes.
- **AC-UI-ADAPTIVE-KANBAN-003.4:** With a fine pointer, an overflowing column shall reveal its scrollbar on column hover, keyboard focus within the column, or active scrolling. An overflowing lane shall reveal its horizontal scrollbar on lane interaction. Idle scrollbars shall become transparent without moving cards or headers. The horizontal scrollbar shall remain hidden during a task drag.
- **AC-UI-ADAPTIVE-KANBAN-003.5:** Fades shall not intercept pointer input or conceal headers, scrollbar controls, or the final card at the scroll boundary. Keyboard users shall reach and scroll each overflowing region without activating a card action.
- **AC-UI-ADAPTIVE-KANBAN-003.6:** At a column's vertical boundary, further vertical wheel input shall scroll the outer workflow container in that direction. Non-overflowing columns shall permit the same outer scrolling. Column scrolling shall not move sibling columns or change another lane's horizontal position.
- **AC-UI-ADAPTIVE-KANBAN-003.7:** Touch users shall not need hover to discover overflow or scroll. Phone Kanban shall retain its full-height focused column, workflow navigator, direct task navigation, and safe-area controls. Tablet shall retain two-column navigation. Desktop preferences shall remain unchanged.
- **AC-UI-ADAPTIVE-KANBAN-003.8:** Reduced-motion mode shall suppress scrollbar and fade transitions. Forced-color mode shall preserve visible native scrollbars and clear focus indicators without relying on gradients.
- **AC-UI-ADAPTIVE-KANBAN-003.9:** Overflow indicators shall preserve virtualized task access, card actions, WIP boundaries, multi-selection, drag auto-scroll, and drag-anchor geometry. Loading or empty content shall not show an overflow cue without an actual scroll range.

## Migrated source detail

## Why

People use Kandev in portrait desktop windows, beside a task preview, and with a wide application
sidebar. In those situations the Kanban board currently compresses every workflow column until
cards and their metadata become difficult to read or escape their card boundaries. The board must
remain useful when its own surface is narrow, even when the overall viewport still qualifies as
desktop.

## What

- Desktop Kanban composition responds to the rendered board surface width after surrounding app
  chrome and preview panels consume space; viewport width or portrait orientation alone does not
  determine whether columns fit.
- A desktop workflow shows every column simultaneously when each column can retain a readable
  minimum width.
- When every column cannot fit, the workflow becomes a windowed board: complete columns keep their
  readable minimum width and horizontal overflow stays inside the workflow. The existing column
  headers and direct lane scrolling remain the desktop navigation; no additional stage selector is
  shown.
- Existing desktop card interactions remain available in either composition, including pointer
  drag-and-drop between visible columns, multi-select, context actions, and `Move to` for any step.
- At a fixed viewport size, each Kanban column keeps its mounted card count within the visible
  vertical area plus a small overscan. The mounted count does not grow with the total task count.
- Users can reach every task by scrolling or searching. Vertical windowing does not change the
  column count, task order, WIP queue boundary, selection order, or card actions.
- A column with 440 tasks mounts fewer than 100 card bodies during initial display on desktop,
  tablet, and phone surfaces.
- Opening or resizing the task preview may change the effective desktop composition without
  changing the user's saved Kanban, Pipeline, workflow, or preview preferences.
- A subtask card presents its parent relationship as contained hierarchy metadata. Long or missing
  parent titles never widen the card; visible text truncates while the full available title remains
  accessible.
- Phone Kanban retains its single focused workflow-and-step view, workflow/step drawer, swipe navigation,
  native card scrolling, menu-based task moves, direct card navigation, and safe-area FAB.
- Tablet Kanban retains its two-column snap-scrolling composition and existing task actions.

## Failure modes

- Before the board surface has a measurable width, desktop columns retain the readable minimum and
  any overflow remains inside the workflow instead of compressing or widening the document.
- If a workflow update changes its columns while the board is scrolled, the browser retains its
  normal scroll position where possible; an empty workflow keeps its existing empty-state behavior.
- A subtask whose parent title is unavailable still renders a generic subtask relationship without
  exposing an empty or overflowing element.

## Persistence guarantees

- Adaptive composition is presentation state only. It is not persisted and does not overwrite saved
  view, workflow, repository, or preview preferences.
- Existing task, workflow, and preview persistence contracts are unchanged.

## Scenarios

- **GIVEN** a desktop board surface wide enough for every workflow step at the readable minimum,
  **WHEN** Kanban renders, **THEN** all columns share the available width and no additional stage
  selector is shown.
- **GIVEN** a portrait or otherwise constrained desktop board surface, **WHEN** all workflow columns
  cannot fit at the readable minimum, **THEN** complete columns appear in an internally scrollable
  window without document-level horizontal overflow or an additional stage selector.
- **GIVEN** an inline preview or surrounding app chrome that reduces the board surface width,
  **WHEN** the available width shrinks, **THEN** Kanban retains readable columns with internal lane
  scrolling, without closing the preview or mutating saved preferences.
- **GIVEN** a subtask whose parent has a long title, **WHEN** its card renders in the narrowest
  supported column, **THEN** the relationship remains inside the card and the parent title truncates.
- **GIVEN** the phone Kanban, **WHEN** the same workflows render, **THEN** the existing focused-column
  navigator and mobile actions remain available and no desktop stage selector is mounted.
- **GIVEN** the tablet Kanban, **WHEN** the same workflows render, **THEN** the existing two-column
  snap-scrolling layout remains active and no desktop stage selector is mounted.
- **GIVEN** a desktop column with 440 tasks, **WHEN** Kanban renders, **THEN** fewer than 100 card
  bodies are mounted and the visible cards respond to user input.
- **GIVEN** a desktop board whose rendered workflow steps do not change, **WHEN** pointer drag starts
  or ends, **THEN** each column keeps its computed width.
- **GIVEN** a desktop board whose columns fit before a pointer drag, **WHEN** drag anchoring adds
  internal end space, **THEN** the document width stays fixed and no transient horizontal scrollbar
  appears.
- **GIVEN** a phone column with 440 tasks, **WHEN** Kanban renders, **THEN** fewer than 100 card
  bodies are mounted inside the existing focused-column scroll area.
- **GIVEN** a windowed column, **WHEN** the user scrolls from the first task to the last task,
  **THEN** each reached card keeps its normal actions and the mounted card count stays bounded.
- **GIVEN** a windowed WIP column with queued tasks, **WHEN** the user scrolls through the admission
  boundary, **THEN** the queue divider appears at the correct logical position.

## Out of scope

- Wrapping workflow columns onto multiple rows or replacing Kanban with a vertically stacked list.
- Redesigning Pipeline or List views.
- Persisting a separate portrait-layout preference or horizontal scroll position.
- Changing backend task, workflow, WIP-limit, or preview contracts.
- Removing task descriptions from the board API response.
- Adding pagination or a manual show-more control to Kanban columns.

## Implementation plans

[Adaptive Kanban implementation plan](../../../plans/adaptive-kanban/plan.md)

[Large-column virtualization repair plan](../../../plans/kanban-large-column-virtualization/plan.md)

[Compact workflow swimlane repair plan](../../../plans/kanban-swimlane-height/plan.md)

[Six-card swimlane scrolling plan](../../../plans/kanban-swimlane-scrolling/plan.md)
