---
status: active
system: tasks
created: 2026-11-25
owners:
  - kandev
---

# HTTP task repository selection requirements

## Overview

Task creation clients can select repositories that already belong to a workspace.
The task system owns how `POST /api/v1/tasks` accepts and applies that selection.

## Requirements

### REQ-TASKS-HTTP-REPOSITORY-SELECTION-001: Repository ID compatibility

**Intent:** A successful new task includes the repositories the caller selected.

#### Acceptance criteria

- **AC-TASKS-HTTP-REPOSITORY-SELECTION-001.1:** When a new task succeeds with a nonempty `repository_ids` array, it shall attach the selected repositories in request order. Each ID shall have the same effect as an entry that supplies only `repository_id` in `repositories`.
- **AC-TASKS-HTTP-REPOSITORY-SELECTION-001.2:** An empty `repository_ids` array shall have the same effect as an empty `repositories` array. This selection shall suppress automatic project sources for a root task. For either field, `null` shall have the same effect as omission.
- **AC-TASKS-HTTP-REPOSITORY-SELECTION-001.3:** When both fields are arrays, the server shall return HTTP 400 with `creation_rejected: true` before task allocation. This rule includes empty arrays.
- **AC-TASKS-HTTP-REPOSITORY-SELECTION-001.4:** When a new task request supplies a malformed `repository_ids` value or an invalid ID, the server shall reject it before task allocation. A repository outside the selected workspace shall produce the same rejection as the equivalent `repositories` request.

## Out of scope

- Changes to repository selection through WebSocket, MCP, or plugins.
- Changes to existing task attachments, repository sets, and automation configuration.

## Related documents

- [HTTP task repository selection design](../system-design/http-task-repository-selection.md)
- [Project repository context requirements](project-repository-context.md)
- [Fix package for issue 5503](../../../plans/task-create-repository-ids/plan.md)
