---
status: active
system: tasks
created: 2026-08-01
updated: 2026-08-11
owners:
  - Kandev
---
# Task Title Length Limit Requirements

## Overview

This document is the migrated task-system source for the capability. The source detail below remains authoritative while the system is migrated into separate requirement and design records.

## Requirements

### REQ-TASKS-TITLE-LENGTH-LIMIT-001: Task Title Length Limit

**Intent:** Preserve the observable task or workflow behavior recorded by the legacy specification.

#### Acceptance criteria

- **AC-TASKS-TITLE-LENGTH-LIMIT-001.1:** When a consumer uses this capability, the system shall provide the observable behavior and exclusions documented below.

## Migrated source detail

## Why

Long task titles crowd navigation and task chrome, especially when a remote pull request or issue supplies the title automatically. Agents and API clients can also bypass the task dialog, so the product needs one predictable title boundary across every creation path.

## What

- New and renamed task titles must contain no more than 60 characters.
- Every editable task-title field enforces the same limit, including the shared create/edit dialog, task rename dialog, subtask form, and Office new-task dialog on desktop and mobile.
- Enforcing the limit never disturbs the user's caret: while a field's value is at the limit, typing mid-title keeps the caret immediately after the inserted characters instead of resetting it to the end of the field.
- When a remote pull request, merge request, or issue supplies a title longer than 60 characters, the task dialog uses the first 59 characters followed by `…`. The full remote title remains available in the generated task description or linked remote item; only the task title is shortened.
- The `create_task_kandev` MCP tool advertises and enforces the 60-character limit and asks agents to use a concise, few-word title.
- Backend task creation and title updates reject titles longer than 60 characters regardless of whether the caller uses HTTP, WebSocket, MCP, Office, automation, or another service adapter.
- The GitHub review-PR and GitHub issue watchers, which construct `PR #<n>: <title>` and `Issue #<n>: <title>` respectively, shorten the complete generated title (including the `PR #<n>:` or `Issue #<n>:` prefix) to at most 60 characters before it reaches task creation. A title that is shortened ends with a single `…` and never exceeds 60 runes counted by Unicode code point, so multibyte titles are not split mid-character. Because backend task creation rejects overlong titles, a watcher that skips this shortening drops the review/issue task entirely instead of creating a shortened one — this is the concrete regression this limit fixes for the GitHub paths.
- Every other backend watcher that turns a remote item into a task through the shared watcher-dispatch pipeline — Linear (`[<identifier>] <title>`), Jira (`[<key>] <summary>`), GitLab merge-request and issue (`[<path>!<n>]`/`[<path>#<n>] <title>`), and Azure DevOps work-item and pull-request (`[<project>#<n>] <title>`) — has its complete generated title shortened to at most 60 runes (ending with a single `…` when shortened, counted by Unicode code point) before it reaches task creation. The dispatch coordinator applies this once at the shared seam after each source builds its request, so a source cannot skip it and every current and future watcher source inherits the guarantee automatically. The automation-trigger path likewise routes its generated title through the shared truncation helper. New watcher paths that auto-generate a title from a remote item are covered by the coordinator with no per-source work.
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
- **GIVEN** a task-title field whose current value is at the 60-character limit (for example a remote-imported title shortened with `…`), **WHEN** the user places the caret mid-title and types, **THEN** the inserted characters appear in place, the field stays at 60 characters, and the caret remains immediately after the inserted text instead of jumping to the end of the field.
- **GIVEN** a remote pull request, merge request, or issue whose proposed task title exceeds 60 characters, **WHEN** the create-task dialog opens, **THEN** its title field contains the first 59 characters plus `…` and the complete remote context remains in the description or remote link.
- **GIVEN** an MCP caller supplies a title of 60 characters, **WHEN** it calls `create_task_kandev`, **THEN** task creation proceeds subject to the existing validation rules.
- **GIVEN** an MCP caller supplies a title longer than 60 characters, **WHEN** it calls `create_task_kandev`, **THEN** the call returns a validation error and creates no task.
- **GIVEN** an HTTP or WebSocket caller supplies an overlong title for task creation or rename, **WHEN** the request is handled, **THEN** it returns the channel's validation response and leaves stored data unchanged.
- **GIVEN** an existing task with a legacy title longer than 60 characters, **WHEN** a caller updates another task field without submitting `title`, **THEN** the existing title remains unchanged.
- **GIVEN** a GitHub review watcher observes a pull request whose `PR #<n>: <title>` string exceeds 60 characters, **WHEN** the review task is created, **THEN** the generated title is shortened to at most 60 runes ending with `…`, the `PR #<n>:` prefix is retained, and the review task is created rather than dropped.
- **GIVEN** a GitHub issue watcher observes an issue whose `Issue #<n>: <title>` string exceeds 60 characters, **WHEN** the issue task is created, **THEN** the generated title is shortened to at most 60 runes ending with `…`, the `Issue #<n>:` prefix is retained, and the issue task is created rather than dropped.
- **GIVEN** a watcher-generated title contains multibyte characters near the 60-rune boundary, **WHEN** it is shortened, **THEN** the result is counted by Unicode code point and no character is split.

## Out of scope

- Rewriting or migrating existing task titles.
- Changing remote pull request, merge request, issue, project, workflow, session, or document title limits.
- Making the 60-character value configurable.

## Implementation plan

See [the implementation plan](../../../plans/task-title-length-limit/plan.md) and the [caret-preservation fix plan](../../../plans/task-title-caret/plan.md).