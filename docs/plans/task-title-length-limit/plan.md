---
spec: docs/specs/tasks/requirements/title-length-limit.md
created: 2026-08-01
status: implemented
---

# Implementation Plan: Task Title Length Limit

## Overview

Add one backend validation boundary for task creates and title-bearing updates, expose the same limit in the MCP tool schema, and share a frontend title helper across all task-title inputs and remote-prefill paths. Existing rows are not migrated. Targeted service/transport tests prove the non-UI boundary, while desktop and mobile Playwright coverage proves manual input and remote prefill.

## Backend

### Task title contract

- Add `apps/backend/internal/task/service/task_title.go` with the exported `TaskTitleMaxLength = 60`, a typed validation sentinel, and a small validator used by both `Service.CreateTask` and `Service.UpdateTask` in `service_tasks.go`.
- Validate before any repository write or task lifecycle event. An update validates only when `UpdateTaskRequest.Title` is non-nil, preserving legacy overlong titles during unrelated updates.
- Update `apps/backend/internal/task/handlers/errors.go` and the WebSocket update error mapping in `task_ws_handlers.go` so the typed title error becomes the existing HTTP/WS validation response rather than a generic internal error.

### MCP contract

- Update `apps/backend/internal/mcp/server/server.go` so `create_task_kandev.title` declares `mcp.MaxLength(service.TaskTitleMaxLength)` and asks for a concise, few-word title of at most 60 characters.
- Update `apps/backend/internal/mcp/handlers/handlers.go` classification so backend title validation is returned as an MCP validation error even if a client skips JSON-schema validation.
- Keep `apps/backend/config/prompts/kandev-context.md` aligned with the registered schema and recommendation.

## Frontend

### Shared title normalization

- Add `apps/web/lib/task-title.ts` with `TASK_TITLE_MAX_LENGTH = 60`, a user-input clamp, and a remote-prefill truncator that reserves the final character for `…`.
- Apply the prefill helper in `apps/web/components/task-create-dialog-state.ts` for initial values, restored drafts, and GitHub URL suggestions, in the Jira/Linear import handlers in `task-create-dialog.tsx`, and in the GitHub, GitLab, Jira, Linear, and Azure DevOps launchers. Provider launchers continue to preserve the full remote title in their generated descriptions.

### Editable title surfaces

- Apply the shared limit to `InlineTaskName` in `task-create-dialog-selectors.tsx`, including the native input length hint and change-handler clamp used by create and edit mode.
- Reuse the same helper and constant in `apps/web/components/task/task-rename-dialog.tsx`, `apps/web/components/task/task-top-bar-title.tsx`, `apps/web/components/task/new-subtask-form-parts.tsx`, `apps/web/components/automations/automation-editor-sections.tsx`, and `apps/web/app/office/components/new-task-dialog.tsx` so backend validation never appears as a late surprise in another title editor.
- Preserve the existing responsive composition. Desktop and phone use the current shared dialog/full-height mobile surface, existing scroll owner, actions, and focus behavior.

## Mobile design contract

- **Outcome and entry point:** creating, editing, renaming, and nesting a task keeps the same mobile entry points; users can enter a valid title without hidden desktop-only behavior.
- **Nearest exemplar:** the existing `TaskCreateDialog` phone composition and `apps/web/e2e/tests/task/mobile-create-task-remote-repo.spec.ts`; this change reuses their full-height dialog, title placement, and internal scroll behavior.
- **Hierarchy and action:** the title remains the first editable content field and existing footer actions remain primary. No new drawer, navigation, fixed control, or touch target is introduced.
- **State and scrolling:** title normalization is shared business logic across viewports; the existing dialog remains the single scroll owner and retains dynamic viewport/safe-area handling.
- **Proof:** update the existing mobile remote-repository scenario to assert overlong issue prefill is shortened and the input cannot exceed 60 characters.

## Tests

- **What:** create accepts 60 characters and rejects 61 before persistence; unrelated updates preserve legacy titles; rename accepts 60 and rejects 61. **File:** `apps/backend/internal/task/service/task_title_test.go`. **How:** service tests against the real SQLite repository and event bus harness.
- **What:** HTTP create/update and WebSocket create/update expose the typed title failure as validation. **Files:** `apps/backend/internal/task/handlers/task_http_handlers_test.go` and `task_ws_handlers_test.go`. **How:** focused handler integration tests through the service into the repository.
- **What:** MCP schema advertises `maxLength: 60`, and a handler call that bypasses schema validation still rejects 61 characters without creating a task. **Files:** `apps/backend/internal/mcp/server/handlers_test.go` and `apps/backend/internal/mcp/handlers/handlers_test.go`. **How:** inspect the registered tool schema and call the MCP handler with the existing test harness.
- **What:** input clamp and remote ellipsis behavior at 59/60/61-character boundaries. **File:** `apps/web/lib/task-title.test.ts`. **How:** table-driven Vitest unit tests.
- **What:** initial values, drafts, GitHub URL autofill, Jira/Linear import, shared create/edit input, rename, subtask, and Office title fields use the shared cap. **Files:** existing focused tests beside `task-create-dialog-state.ts`, `task-create-dialog-form-body.tsx`, `task-rename-dialog.tsx`, `new-subtask-form-parts.tsx`, and `new-task-dialog.tsx`. **How:** Vitest hook/component tests.

## E2E Tests

- **Scenario:** an overlong GitHub PR title opens the desktop create-task dialog with a 60-character title ending in `…`, while the remote PR source remains selected. **File:** `apps/web/e2e/tests/github/pr-action-create-task-dialog.spec.ts`. **What to verify:** exact input value/length and remote URL association.
- **Scenario:** an overlong GitHub issue title is pasted from the mobile remote picker and the user cannot extend the task title past 60 characters. **File:** `apps/web/e2e/tests/task/mobile-create-task-remote-repo.spec.ts`. **What to verify:** exact shortened prefill, native input limit, retained phone viewport containment, and no document horizontal overflow.

## Public documentation

- Update `docs/public/tasks-and-workflows.md` to state the 60-character task title rule and remote-prefill behavior.
- Update `docs/public/automation-and-mcp.md` to document the `create_task_kandev.title` maximum and validation failure.
- Validate both pages with the repository public-doc checks.

## Implementation waves and parallel candidates

Wave 1 (parallel candidates only with explicit user authorization; default execution is sequential):

- [x] [Task 01: Backend and MCP title contract](task-01-backend-mcp-contract.md)
- [x] [Task 02: Frontend title inputs and remote prefill](task-02-frontend-title-limit.md)

Wave 2:

- [x] [Task 03: E2E coverage and public documentation](task-03-e2e-docs.md)

## Risks

- Backend enforcement can surface previously hidden overlong titles produced by automations or third-party callers; typed validation and public/MCP guidance make that failure explicit.
- Remote prefixes such as `Review:`, `PR #123:`, or `[KEY]` consume part of the 60-character allowance because the stored task title, not only the provider title, is bounded.
- Existing overlong rows must remain updateable when a request does not include `title`; the service tests pin this compatibility behavior.
