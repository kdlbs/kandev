---
status: shipped
created: 2026-07-27
owner: kandev
---

# Adaptive Kanban

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

## Out of scope

- Wrapping workflow columns onto multiple rows or replacing Kanban with a vertically stacked list.
- Redesigning Pipeline or List views.
- Persisting a separate portrait-layout preference or horizontal scroll position.
- Changing backend task, workflow, WIP-limit, or preview contracts.

## Implementation plan

[Adaptive Kanban implementation plan](../../plans/adaptive-kanban/plan.md)
