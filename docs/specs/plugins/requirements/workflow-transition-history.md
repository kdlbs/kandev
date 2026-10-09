---
id: plugins-workflow-transition-history
title: Workflow transition history for plugin and canvas reads
status: draft
system: plugins
owners:
  - kandev
created: 2026-09-22
last_updated: 2026-09-22
---

# Workflow transition history for plugin and canvas reads Requirements

## Overview

A user investigating a task needs to see where it moved, when it moved, and
whether a move went backward through the current workflow order. A workspace
view also needs current task counts and recorded route counts. The task system
already records step changes; this capability makes that evidence available to
an authorized plugin or canvas and uses it in the workflow movement canvas.

Plugins owns the read contract because it owns the capability-gated Host data
API and the canvas browser data protocol. [Tasks](../../tasks/README.md) remains
authoritative for the transition ledger. [Canvases](../../canvases/README.md)
owns task scope and user-controlled workspace promotion.

## Terminology

- **Recorded move:** A committed row in the task step-transition ledger.
- **Backward move (return):** A move between two known steps in one workflow
  whose destination has an earlier or equal position in the current workflow
  definition. This does not establish why the move happened or whether a review
  rejected work.
- **Route count:** The number of retained recorded moves between a source and
  destination in one workflow. It is historical, while task counts describe
  current unarchived tasks.

## Requirements

### REQ-PLUGINS-WORKFLOW-HISTORY-001: Task transition trail

**Intent:** An authorized reader can inspect the recorded path of a task without
accessing Kandev's database file or reconstructing moves from current state.

#### Acceptance criteria

- **AC-PLUGINS-WORKFLOW-HISTORY-001.1:** When a task has recorded moves, the
  read API shall return each retained move's source and destination workflow
  and step identifiers, occurrence time, and trigger in a stable newest-first
  order with bounded pagination.
- **AC-PLUGINS-WORKFLOW-HISTORY-001.2:** A move shall remain in the trail when
  its workflow or step has since been removed. The response shall preserve the
  recorded identifiers and null endpoints without inventing a current name.
- **AC-PLUGINS-WORKFLOW-HISTORY-001.3:** When a task has no recorded moves, the
  API shall return an empty trail. It shall not reconstruct moves from messages,
  current step, or dates before ledger activation.
- **AC-PLUGINS-WORKFLOW-HISTORY-001.4:** A canvas with `api_read:tasks` shall
  read the trail only for a task admitted by its current scope. A caller without
  the read grant or task access shall receive an error and no history.
- **AC-PLUGINS-WORKFLOW-HISTORY-001.5:** Reading or refreshing a trail shall
  never change a task, step, workflow, or ledger row. The public trail shall not
  expose session IDs or actor identifiers.

### REQ-PLUGINS-WORKFLOW-HISTORY-002: Workspace route overview

**Intent:** A workspace canvas can show actual traffic through one workflow
without fetching every task's entire history.

#### Acceptance criteria

- **AC-PLUGINS-WORKFLOW-HISTORY-002.1:** For a workflow in the canvas workspace,
  a workspace-scoped canvas with both `api_read:tasks` and
  `api_read:workflows` shall receive bounded, paginated counts of retained
  moves within that workflow, including moves belonging to archived tasks.
- **AC-PLUGINS-WORKFLOW-HISTORY-002.2:** The overview shall distinguish
  within-workflow routes from entries into and exits from the workflow. It
  shall retain route groups involving removed steps as unresolved identifiers.
- **AC-PLUGINS-WORKFLOW-HISTORY-002.3:** A task-scoped canvas shall not receive
  workflow-wide route counts. An out-of-scope or unknown workflow shall not
  disclose route data.
- **AC-PLUGINS-WORKFLOW-HISTORY-002.4:** The canvas shall show current task
  counts from the existing scoped task list, with archived tasks excluded, and
  label route counts as recorded history. A concurrent move may appear on one
  read before the other; a refresh shall reconcile the display.

### REQ-PLUGINS-WORKFLOW-HISTORY-003: Live workflow movement canvas

**Intent:** The canvas from issue #3583 shows useful live evidence at the scope
the user has authorized.

#### Acceptance criteria

- **AC-PLUGINS-WORKFLOW-HISTORY-003.1:** In task scope, the canvas shall show
  its task's current workflow step, state, timestamps, and recorded trail. It
  shall not present sample moves as that task's history.
- **AC-PLUGINS-WORKFLOW-HISTORY-003.2:** After user-controlled promotion to
  workspace scope, the same canvas shall let the user choose a workflow and an
  unarchived task, show step counts and route counts, and inspect that task's
  recorded trail.
- **AC-PLUGINS-WORKFLOW-HISTORY-003.3:** The canvas shall label known backward
  moves separately from forward moves. A move involving a missing step or a
  different workflow shall be shown as unclassified rather than called a
  rejection or return.
- **AC-PLUGINS-WORKFLOW-HISTORY-003.4:** While open, the canvas shall detect
  a watched task's step change within 15 seconds and refresh its current state
  and trail. An audio cue shall play only after the user enables sound and only
  for a subsequent observed step change.
- **AC-PLUGINS-WORKFLOW-HISTORY-003.5:** Desktop and phone views shall offer
  the same workflow and task selection, trail, and source status. On phones,
  selection and the trail shall be reachable by touch without horizontal page
  scrolling; controls shall have visible focus and accessible names.
- **AC-PLUGINS-WORKFLOW-HISTORY-003.6:** Loading, empty history, an older host
  without the history route, permission denial, and transient read failure
  shall have distinct visible states. A failed read shall offer retry without
  displaying stale data as current.

## Out of scope

- Reconstructing moves before the ledger started recording.
- Treating a backward move as proof of a rejected review.
- Writing, moving, or approving tasks from the diagram.
- A separate process that reads SQLite while Kandev's backend is unavailable.
- Automatic promotion or permission approval on behalf of the user.

## Implementation plan

- [Live workflow transition canvas](../../../plans/workflow-transition-canvas/plan.md)
