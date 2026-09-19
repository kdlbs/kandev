---
status: draft
system: ui
created: 2026-08-09
updated: 2026-08-10
owners:
  - nova28
---
# Per-workflow column visibility on the kanban board Requirements

## Overview

Users want to declutter the kanban board by hiding tasks that sit in steps they do not currently care about. The motivating request is "show all tasks except the ones in Done" — but it generalises to any step on any board (hide `Backlog`, hide `Review`, etc.).

## Requirements

### REQ-UI-BOARD-STEP-VISIBILITY-FILTER-001: Per-workflow column visibility on the kanban board

**Intent:** Users want to declutter the kanban board by hiding tasks that sit in steps they do not currently care about. The motivating request is "show all tasks except the ones in Done" — but it generalises to any step on any board (hide `Backlog`, hide `Review`, etc.).

#### Acceptance criteria

- **AC-UI-BOARD-STEP-VISIBILITY-FILTER-001.1:** Each rendered workflow lane SHALL offer a **Columns** menu listing **that workflow's steps only**, as checkbox items in `position` order (tiebreak by `id`, see [Ordering](#ordering--determinism)).
- **AC-UI-BOARD-STEP-VISIBILITY-FILTER-001.2:** Every step SHALL be **shown (ticked) by default**. A step is hidden only when the user explicitly unticks it.
- **AC-UI-BOARD-STEP-VISIBILITY-FILTER-001.3:** Unticking a step SHALL, on every board surface that renders that workflow (single-workflow board and multi-workflow swimlane, kanban and pipeline views): 1. **hide that step's tasks**, and 2. **collapse (remove) that step's now-empty column** — not merely empty it.
- **AC-UI-BOARD-STEP-VISIBILITY-FILTER-001.4:** The selection SHALL be **scoped strictly per workflow id**: hiding step `S` in workflow `A` SHALL NOT affect any step of workflow `B`, including a step in `B` that shares `S`'s title. Cross-workflow bleed SHALL be impossible by construction (the selection is keyed by workflow id, and a task is matched only against its own workflow's hidden set).
- **AC-UI-BOARD-STEP-VISIBILITY-FILTER-001.5:** Re-ticking a step SHALL restore its column and its tasks with no other side effect.
- **AC-UI-BOARD-STEP-VISIBILITY-FILTER-001.6:** The selection SHALL **persist** across reloads and sessions in backend user display settings (the same tier as the Workflow and Repository filters), not session-only.
- **AC-UI-BOARD-STEP-VISIBILITY-FILTER-001.7:** The selection SHALL track step **id**, never title, so renaming a step keeps its hidden/shown state.
- **AC-UI-BOARD-STEP-VISIBILITY-FILTER-001.8:** Column visibility SHALL **compose (AND)** with the Workflow filter, Repository filter, search query, and any plugin task filters: a task is visible only if it passes all of them.

## System design

The migrated technical source is split into [part 1](../system-design/board-step-visibility-filter-01.md), [part 2](../system-design/board-step-visibility-filter-02.md).
