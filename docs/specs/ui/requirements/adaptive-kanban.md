---
status: active
system: ui
created: 2026-07-27
owners:
  - kandev
---
# Adaptive Kanban Requirements

## Overview

People use Kandev in portrait desktop windows, beside a task preview, and with a wide application sidebar. In those situations the Kanban board currently compresses every workflow column until cards and their metadata become difficult to read or escape their card boundaries. The board must remain useful when its own surface is narrow, even when the overall viewport still qualifies as desktop.

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
  fixed drop targets, direct card navigation, and safe-area FAB.
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
