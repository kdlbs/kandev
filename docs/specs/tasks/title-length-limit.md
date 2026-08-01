---
status: complete
created: 2026-08-01
owner: Kandev
---

# Task Title Length Limit

## Why

Long task titles crowd navigation and task chrome, especially when a remote pull request or issue supplies the title automatically. Agents and API clients can also bypass the task dialog, so the product needs one predictable title boundary across every creation path.

## What

- New and renamed task titles must contain no more than 60 characters.
- Every editable task-title field enforces the same limit, including the shared create/edit dialog, task rename dialog, subtask form, and Office new-task dialog on desktop and mobile.
- When a remote pull request, merge request, or issue supplies a title longer than 60 characters, the task dialog uses the first 59 characters followed by `…`. The full remote title remains available in the generated task description or linked remote item; only the task title is shortened.
- The `create_task_kandev` MCP tool advertises and enforces the 60-character limit and asks agents to use a concise, few-word title.
- Backend task creation and title updates reject titles longer than 60 characters regardless of whether the caller uses HTTP, WebSocket, MCP, Office, automation, or another service adapter.
- Existing stored titles longer than 60 characters remain readable and are not rewritten. Updates that omit `title` continue to work; any submitted replacement title must satisfy the new limit.

## API surface

- Task create and update requests keep their existing request and response shapes.
- A submitted title longer than 60 characters fails validation and does not create or rename a task.
- HTTP callers receive `400 Bad Request`; WebSocket and MCP callers receive their existing validation-error response shape with a message stating that task titles must be 60 characters or fewer.
- The `create_task_kandev.title` input schema declares a maximum length of 60 and describes the title as concise.

## Failure modes

- An overlong title from a user, agent, automation, plugin, or API client is rejected without persisting a task or title change.
- Remote prefill does not fail the dialog: it is shortened before it reaches the editable title state.
- Existing legacy titles are not changed merely by loading or displaying them.

## Scenarios

- **GIVEN** the create-task dialog on desktop or mobile, **WHEN** a user types or pastes more than 60 characters into the title field, **THEN** the field contains at most 60 characters and a valid title can be submitted.
- **GIVEN** any other task-title editor, **WHEN** a user types or pastes more than 60 characters, **THEN** the field contains at most 60 characters.
- **GIVEN** a remote pull request, merge request, or issue whose proposed task title exceeds 60 characters, **WHEN** the create-task dialog opens, **THEN** its title field contains the first 59 characters plus `…` and the complete remote context remains in the description or remote link.
- **GIVEN** an MCP caller supplies a title of 60 characters, **WHEN** it calls `create_task_kandev`, **THEN** task creation proceeds subject to the existing validation rules.
- **GIVEN** an MCP caller supplies a title longer than 60 characters, **WHEN** it calls `create_task_kandev`, **THEN** the call returns a validation error and creates no task.
- **GIVEN** an HTTP or WebSocket caller supplies an overlong title for task creation or rename, **WHEN** the request is handled, **THEN** it returns the channel's validation response and leaves stored data unchanged.
- **GIVEN** an existing task with a legacy title longer than 60 characters, **WHEN** a caller updates another task field without submitting `title`, **THEN** the existing title remains unchanged.

## Out of scope

- Rewriting or migrating existing task titles.
- Changing remote pull request, merge request, issue, project, workflow, session, or document title limits.
- Making the 60-character value configurable.

## Implementation plan

See [the implementation plan](../../plans/task-title-length-limit/plan.md).
