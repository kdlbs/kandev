---
created: 2026-09-13
status: implemented
requirements:
  - REQ-TASKS-BOARD-PRIORITY-SORT-FILTER-001
  - REQ-TASKS-BOARD-PRIORITY-SORT-FILTER-002
  - REQ-TASKS-BOARD-PRIORITY-SORT-FILTER-003
  - REQ-TASKS-BOARD-PRIORITY-SORT-FILTER-004
  - REQ-TASKS-BOARD-PRIORITY-SORT-FILTER-005
system_design:
  - ../../specs/tasks/system-design/board-priority-sort-filter.md
---

# Implementation Plan: Board priority sort and filter

## Overview

Add persisted priority filtering and board sorting to the kanban and pipeline
views. Keep the existing task-settings persistence path and native ordering
rules, then apply priority as a view-level ordering and filtering choice.

## Scope

- Add the board sort and priority filter to desktop and mobile display controls.
- Carry both values through user settings, boot hydration, and WebSocket updates.
- Apply the filter to board projections without changing occupancy semantics.
- Apply deterministic priority ordering to kanban, pipeline, and bulk-move paths.
- Cover hidden workflow selection and equal unknown pipeline step indices.

## Architecture

The backend user-settings contract stores the two view values. The frontend
settings hook owns the current selection and uses the existing revision-aware
settings channel. Board projection helpers derive visible tasks from the
canonical snapshots. View components render the derived lists; they do not own
another copy of the persisted settings.

The pipeline comparator uses the effective workflow selection. It compares
equal step indices with the within-step comparator, which keeps priority and
position ordering deterministic for unknown or explicitly hidden workflows.

## Work orders

- [x] [Task 01: Implement board priority sort and filter](task-01-board-priority-sort-filter.md)

## Verification

- Focused Vitest coverage passed for board settings, task projection, ordering,
  multi-select ordering, and the kanban menu actions.
- Web typecheck and lint passed.
- The branch was merged with the current `main` before delivery.

## Risks

- Board display settings are a snapshot payload. Concurrent changes to fields
  in that payload keep the existing last-commit semantics.
- Pipeline ordering must use the same workflow selection as the rendered board;
  otherwise hidden workflows can receive infinite indices and lose ordering.
