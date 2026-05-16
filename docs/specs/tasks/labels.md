---
status: shipped
created: 2026-04-29
owner: cfl
---

# Task Labels

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
