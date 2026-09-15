---
status: current
system: tasks
requirements:
  - REQ-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001
created: 2026-09-03
owners:
  - kandev
---

# Edit task dependencies System Design

## Purpose and boundaries

This design adds dependency selection to the existing Edit task dialog. The
task-detail dependency chip stays read-only. The core dependency graph and
launch rules remain unchanged.

The task system owns this design because it owns dependency state, permissions, and lifecycle effects. The UI provides the interaction surface only.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-TASK-DEPENDENCY-DETAIL-EDITING-001` | [Components and responsibilities](#components-and-responsibilities), [Control flow](#control-flow), [Failure behavior](#failure-behavior), [Responsive behavior](#responsive-behavior) |

## Existing contracts

The implementation keeps the one-edge contracts in
[`task-dependencies.md`](task-dependencies.md):

- `POST /api/v1/tasks/:id/dependencies` adds one predecessor.
- `DELETE /api/v1/tasks/:id/dependencies/:depId` removes one predecessor.
- Both routes authorize both task IDs and reject cross-workspace access.
- The add route rejects cycles with HTTP `409` and a `cycle` array.
- A mutation publishes `task.updated` for both ends of the edge.
- A remove operation never consumes a deferred launch intent.

It adds one task-scoped replacement contract:

- `PUT /api/v1/tasks/:id/dependencies` accepts
  `{ "depends_on_task_ids": ["task-id"] }`.
- The route replaces the complete direct predecessor set for one task.
- An empty list removes every direct predecessor.
- The response uses the existing dependency mutation projection.

The replacement is one atomic dependency operation. The service validates the
complete desired set before storage changes. Storage applies the edge diff in
one transaction. A validation or storage error leaves the prior dependency set
unchanged.

No database migration or WebSocket event change is necessary. The one-edge
routes remain available for Office, MCP, and other callers.

### Replacement rationale

The Edit task dialog stages several fields and applies them through Update.
Immediate one-edge mutations make Cancel misleading. A sequence of one-edge
requests can also stop after only part of a multi-selection succeeds. The
full-set route keeps the dependency part of the update atomic while it leaves
the established one-edge contracts intact.

## Components and responsibilities

### API client

`apps/web/lib/api/domains/task-dependencies-api.ts` owns the typed replacement
function. It returns the dependency projection from the mutation response. The
module also parses the structured cycle response.

The picker uses `listTasksByWorkspace` from `kanban-api.ts`. It sends the active
search text to the server and requests non-archived tasks only.

### Dependency service and storage

`Service.ReplaceDependencies` authorizes the edited task and every desired
predecessor. It rejects an empty ID, duplicate ID, self-edge, inaccessible task,
cross-workspace edge, and cycle.

The existing process-wide dependency lock covers validation and replacement.
The production blocker repository replaces the edge diff in one SQL
transaction. It preserves unchanged edges and their creation times.

After commit, the service publishes `task.updated` for the edited task and for
every predecessor that was added or removed. Publication remains outside the
lock.

### Edit task dialog

`TaskCreateDialog` remains the shared create, session, and edit shell. A focused
dependency field mounts only in edit mode. It stays outside the create-only
advanced-settings disclosure.

When the dialog opens, the field loads the confirmed dependency projection for
the edited task. It initializes a draft list from `depends_on`. The Update
actions stay unavailable until this load finishes.

The field owns these functions:

- show current predecessors as selected
- search non-archived tasks in the edited task's workspace
- add or remove a predecessor from the draft
- show loading, empty, and error states.

The picker excludes the edited task. The server remains the authority for
workspace and cycle validation. The same field remains available for started
tasks even when the prompt and workspace-source fields are locked.

The current task-detail chip stays read-only and keeps its present visibility
rule. A task with no dependency edges has no chip. Users enter the editor from
the existing Edit actions.

### Shared picker presentation

The create and edit flows share the searchable dependency picker presentation.
They keep separate state and save behavior. Create mode still writes
`blocked_by` during task creation. Edit mode submits the complete draft through
the replacement route.

The picker sends search text to `listTasksByWorkspace`. It ignores stale search
responses and never treats a partial page as the complete workspace task set.

### State convergence

The successful replacement response updates the dialog's confirmed projection.
The existing `task.updated` events then update the Kanban caches for all changed
tasks.

The local response state is temporary. When the store contains the same
projection, the component uses the store projection again.

If the store has no dependency projection, `getTaskDependencies` remains the
read fallback. An omitted projection does not mean that the task has no edges.

## Control flow

1. The user activates an existing Edit action.
2. The dialog loads the task's dependency projection and workspace candidates.
3. The user searches and changes the selected predecessor draft.
4. Cancel closes the dialog without a dependency request.
5. Update first runs the existing task-field update.
6. If the dependency draft changed, the client calls the replacement route.
7. The replacement response updates the dialog's confirmed projection.
8. WebSocket events update all affected task projections in board stores.
9. The dialog closes after every required update succeeds.

The task-field update and dependency replacement remain separate HTTP
operations. If the task-field update succeeds and dependency replacement fails,
the dialog stays open and reloads the confirmed task fields. The dependency set
does not change. This keeps the new dependency operation atomic without
expanding the task update transaction and its repository replacement behavior.

## Failure behavior

For HTTP `409`, the dialog translates the cycle path and shows it in the normal
error surface. The dialog does not close or change the confirmed edge list. It
keeps the draft so the user can remove the invalid selection.

For all other replacement errors, the dialog shows localized error copy. It
keeps the prior projection, preserves the draft, and enables Update again.

An empty or failed candidate search shows a localized state. It does not alter
dependency data.

## Security

The picker requests tasks for the edited task's workspace. The server authorizes
the edited task and every submitted predecessor before replacement. It returns
not-found behavior for inaccessible tasks.

The client does not infer permission from the picker contents. It treats the
server response as authoritative.

## Responsive behavior

Desktop uses the current centered Edit task dialog with its contained searchable
popover. Phones use the existing full-screen Edit task dialog. Tablets keep the
task-switcher sheet behind the same editor.

The nearest mobile exemplars are `TaskCreateDialog` and
`session-task-switcher-sheet.tsx`. The phone task-actions menu closes before the
editor opens. The dialog form remains the single vertical scroll owner. The
dependency command list owns scrolling only while its contained popover is
open.

Touch actions and task rows use a target of at least 44 CSS pixels. Long titles
truncate inside the surface. The document does not gain horizontal scrolling.

The editor shares loading, candidate search, draft selection, submission, and
error logic across layouts. Existing dialog breakpoints own the presentation.

## Test strategy

Backend tests cover full-set validation, atomic replacement, cycle errors,
authorization, rollback, and publication for added and removed predecessors.

Frontend tests cover edit-mode initialization, filtering, draft changes, cancel,
successful replacement, cycle errors, request errors, and started-task access.

Desktop Playwright coverage proves edit, update, live blocked state, removal,
cancel, and cycle feedback. Mobile Playwright coverage proves the same value
through the visible task-actions menu. It also checks dialog containment, touch
targets, and horizontal overflow.

## Related designs

- [Task Dependencies and Auto-Start Chains](task-dependencies.md)
- [Task-create dependency selector refinement](../requirements/task-dependencies-create-dialog-dependency-selector.md)
