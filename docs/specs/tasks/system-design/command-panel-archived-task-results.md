---
status: current
system: tasks
requirements:
  - REQ-TASKS-COMMAND-PANEL-ARCHIVED-TASKS-001
---

# Command Panel Archived Task Results System Design

## Purpose and boundaries

This design defines the task-system contract for archive state in command-panel
task results. It does not change archive state or task navigation.

The workspace task-list response already includes `archived_at`. No backend,
WebSocket, or persistence change is necessary.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-COMMAND-PANEL-ARCHIVED-TASKS-001` | [Archive classification](#archive-classification), [Result-row presentation](#result-row-presentation), [Responsive behavior](#responsive-behavior), [Tests](#tests) |

## Components and responsibilities

- `useInlineTaskSearchEffect` queries unarchived matches first and requests only the remaining archived matches when needed.
- `TaskResultItem` derives archive presentation from the HTTP task result.
- The existing task-list `Archived` translation supplies the visible and accessible label.
- The existing command item remains the only interactive element in the result row.

## Archive classification

`Task.archived_at` is the only archive source for command-panel task results.
Terminal workflow states do not mean that a task is archived.

`fetchMatchingTasks` first requests page one with the default archive filter and
the requested display limit. If fewer non-archived matches are returned, it
requests page one in `onlyArchived` mode with the remaining limit. It
concatenates the two responses, preserving backend order within each group.
This keeps the search bounded to at most two requests and ensures an active
match remains visible when archived matches are newer.

The empty-query task preview continues to request active tasks only. Its
defensive filter also uses `archived_at` and does not exclude terminal states.

## Result-row presentation

When `archived_at` is present, `TaskResultItem` changes three visual cues:

- An `IconArchive` glyph replaces `TaskStateIcon` in the leading icon slot.
- An outline **Archived** badge replaces the workflow-step badge.
- Semantic muted-foreground classes replace active task colors for the icon, title, badge, and metadata.

The archive glyph uses `role="img"` and the localized **Archived** label. It
also has a stable test identifier.

The command item keeps its selected background and focus behavior. Muted
classes do not use row opacity because opacity also weakens selection feedback.

Non-archived results keep the shared activity icon and workflow-step badge.
Selection continues to call the existing task navigation handler.

## Responsive behavior

Desktop and phone layouts continue to use the same command-panel result row.
The archived badge replaces one existing badge, so the row gains no new width.

The title remains the flexible, truncated element. Metadata remains hidden on a
phone, and the command panel remains the scroll owner.

The nearest phone example is the task row in
`mobile-command-palette-scopes.spec.ts`. This change affects content and color
only.

## Failure and recovery

If `archived_at` is absent, the result uses its normal activity icon and
workflow-step badge. Unknown optional task fields do not remove the result.

If an archived task is selected, the existing task route shows the archived
detail and its current recovery action.

## Tests

Hook tests cover archive ordering, bounded active-then-archived requests, and
prove that terminal workflow state does not imply archive state.

Component tests cover the archive icon, accessible label, badge replacement,
muted classes, and unchanged selection callback.

Desktop Playwright coverage searches for active and archived matches. It proves
their order and the archive cues before selection.

Phone Playwright coverage proves the same cues, readable title space, and no
document-level horizontal overflow.

## Related specifications

- [Task Archive Confirmation](archive-confirmation.md)
- [Task Activity Icons](../../ui/system-design/command-panel-task-activity-icons.md)
- [Task Workspace Content Search](../../ui/requirements/task-workspace-content-search.md)
