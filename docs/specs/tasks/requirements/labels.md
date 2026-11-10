---
status: active
system: tasks
created: 2026-04-29
owners:
  - cfl
---
# Task Labels Requirements

## Overview

Tasks own label identity and task associations, including when Office exposes
the controls. The migrated source detail remains authoritative for the broader
catalog capability. Exact-name task removal is defined below and designed in
[Task labels system design](../system-design/labels.md).

## Requirements

### REQ-TASKS-LABELS-001: Task Labels

**Intent:** Preserve the observable task or workflow behavior recorded by the legacy specification.

#### Acceptance criteria

- **AC-TASKS-LABELS-001.1:** When a consumer uses this capability, the system shall provide the observable behavior and exclusions documented below.

### REQ-TASKS-LABELS-002: Exact-name task label removal

**Intent:** A label that can be attached to a task can be removed from that task
without changing its identity or removing someone else's association.

#### Acceptance criteria

- **AC-TASKS-LABELS-002.1:** When an attached label has a nonempty name containing
  one or multiple slashes, percent signs, plus signs, or spaces, removal shall
  identify the exact stored name. It shall not interpret separators, decode a
  literal escape sequence again, trim stored whitespace, or normalize the name
  into another label.
- **AC-TASKS-LABELS-002.2:** When removal succeeds, only the named association on
  the selected task shall disappear. The workspace catalog entry, its identity
  and color, other labels on that task, and every other task's associations
  shall remain unchanged.
- **AC-TASKS-LABELS-002.3:** When an existing client removes a simple or
  space-containing name using the existing path-based API, the system shall
  preserve that API's successful response and removal behavior. An additional
  removal API shall support the full accepted name without requiring a routing
  setting or a migration of stored names.
- **AC-TASKS-LABELS-002.4:** When the additional removal API receives an absent,
  empty, repeated, or malformed name parameter, it shall reject the request
  without mutating any association. When a valid exact name or association is
  absent, removal shall remain an idempotent success.
- **AC-TASKS-LABELS-002.5:** When removal is requested through either API, the
  existing caller authorization and workspace ownership rules shall apply to
  both the addressed workspace and task. A foreign or mismatched workspace/task
  request shall not mutate labels.
- **AC-TASKS-LABELS-002.6:** When users remove a slash-containing label through
  the Office task detail on desktop or phone, the same exact-name action shall
  persist after reload. Other chips shall remain; a rejected request shall use
  the existing optimistic rollback and error presentation rather than leaving
  a false successful removal visible.

## Implementation plans

- [Exact-name task label removal](../../../plans/task-label-name-removal/plan.md)

## Migrated source detail

## Why

Tasks have a `labels` field stored as an inline JSON array (`["bug","urgent"]`). There is no label catalog — users and agents can't discover which labels exist, labels have no colors for visual distinction, and filtering tasks by label requires JSON parsing. A normalized label system enables label reuse, discovery, color coding, and efficient queries.

## What

### Label catalog

- Each workspace has a set of labels: name + color. Names are unique per workspace.
- Labels are created on first use — when a label name is added to a task and doesn't exist in the catalog, it is auto-created with a default color.
- Labels can also be created/edited/deleted explicitly via the API and UI (workspace settings).
- Predefined colors: a small palette (8-10 options). Default is assigned round-robin on auto-create.

### Task labeling

- A task has zero or more labels (many-to-many via junction table).
- Agents add/remove labels via `kandev label add <task-id> <label-name>` and `kandev label remove <task-id> <label-name>`.
- The MCP `createTask` handler accepts a `labels` field (array of label names, not IDs). Labels are resolved by name; missing names are auto-created.
- The existing `labels` JSON string field on the task model is replaced by the junction table. Migration converts existing JSON arrays to junction rows.

### Querying

- Tasks can be filtered by label in list/search endpoints: `GET /tasks?labels=bug,urgent`.
- The task response includes labels as an array of `{name, color}` objects (not raw strings).

### UI

- Task cards show label badges (colored pills with label name).
- Task detail shows labels with add/remove buttons.
- Workspace settings page has a label management section (list, rename, change color, delete).
- Issue list sidebar filter includes a label multi-select.

## Scenarios

- **GIVEN** a workspace with no labels, **WHEN** an agent adds label "bug" to a task, **THEN** a "bug" label is auto-created in the catalog with a default color, and the task shows the "bug" badge.

- **GIVEN** a workspace with labels "bug" (red) and "feature" (blue), **WHEN** viewing the task list filtered by "bug", **THEN** only tasks with the "bug" label are shown.

- **GIVEN** a task with labels ["bug", "urgent"], **WHEN** the API returns the task, **THEN** labels are `[{"name":"bug","color":"#ef4444"},{"name":"urgent","color":"#f59e0b"}]`.

- **GIVEN** a label "bug" used on 5 tasks, **WHEN** the label is deleted from the catalog, **THEN** it is removed from all 5 tasks.

## Out of scope

- Label groups or categories (e.g. "priority: high" vs "type: bug")
- Label descriptions
- Per-project label scoping (all labels are workspace-wide)
- Label-based automation triggers (e.g. "when label X added, assign to agent Y")
