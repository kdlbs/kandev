---
spec: docs/specs/tasks/agent-generated-titles.md
created: 2026-07-31
status: implemented
---

# Implementation Plan: Agent-Generated Task Titles

## Overview

Add the portable preference and backend-owned provisional-title lifecycle first, then expose the
task-bound MCP mutation and conditional first-turn instruction. After those contracts exist, wire the
shared desktop/mobile creation dialogs to opt in and finish with end-to-end coverage and public docs.
The persisted pending marker is the integration seam: frontend creation sets it through `auto_title`,
prompt composition reads it, and either an agent or human title update clears it.

---

## Backend

### Portable user setting

- Extend `apps/backend/internal/user/models/models.go`, `dto/dto.go`, and
  `service/service.go` with `AgentGeneratedTaskTitles bool` /
  `agent_generated_task_titles`, using pointer PATCH semantics and a missing-field default of `false`.
- Extend `apps/backend/internal/user/store/sqlite.go` JSON encode/decode and defaults. This remains in
  the existing `users.settings` JSON blob, so no schema migration is needed.
- Project the field through `apps/backend/internal/backendapp/boot_state_routes.go` and the existing
  `user.settings.updated` payload.
- Add focused store, DTO, service, event-payload, and boot-state tests beside the existing archive and
  MCP-profile preference tests.

### Provisional-title creation and pending state

- Add `AutoTitle bool` to `service.CreateTaskRequest` in
  `apps/backend/internal/task/service/service_requests.go` and define
  `models.MetaKeyAgentTitlePending = "agent_title_pending"` in
  `apps/backend/internal/task/models/models.go`.
- In `apps/backend/internal/task/service/service_tasks.go`, add a rune-safe helper that trims the
  prompt, selects the first six entries from `strings.Fields`, and joins them with one space. Shorter
  prompts use every word; there is no normal character truncation or ellipsis. Preserve the existing
  absolute 500-character task-title safety boundary. When `AutoTitle` is true, reject an empty
  normalized prompt, replace the request title with the derived value, and add the pending metadata
  before the task row is inserted.
- Add `auto_title` to `httpCreateTaskRequest` in
  `apps/backend/internal/task/handlers/task_http_handlers.go`. Preserve the existing required-title
  path when it is false; pass the opt-in to the service when true.
- Make an ordinary `Service.UpdateTask` call with a non-nil title remove
  `agent_title_pending` before its single task-row update. Add a task-service method for the MCP path
  that validates the 500-character title limit, accepts only a pending task, updates title and metadata,
  and publishes the standard `task.updated` event.
- Cover whitespace normalization, six-word selection, shorter prompts, the existing absolute title
  limit, empty prompt rejection, top-level and subtask creation, restart-durable metadata, ordinary
  rename precedence, repeated agent calls, and update-event publication in `service_tasks_test.go` and
  focused HTTP handler tests.

### Task-mode MCP tool

- Add internal `mcp.set_task_title` action plumbing in `apps/backend/pkg/websocket/actions.go`,
  `apps/backend/internal/mcp/server/handlers.go`, and
  `apps/backend/internal/mcp/handlers/handlers.go`.
- Add a title-pending task-mode variant in `apps/backend/internal/mcp/server/server.go`. It registers all
  regular task tools plus `set_task_title_kandev`; regular `ModeTask` and restricted modes omit the new
  tool entirely. Its public schema has one required `title`; the server injects `s.taskID` into the
  internal payload and never accepts a caller-selected task ID. Write both the tool description and
  `title` argument description to target three words, use no more than six when practical, and clarify
  that the existing title is provisional and must still be replaced.
- Extend `resolveTaskSessionMCPMode` in
  `apps/backend/internal/orchestrator/executor/executor_execute.go` to select the title-pending task mode
  from `agent_title_pending`. Config and Office modes win. Thread the new supported mode through the
  existing agentctl mode validation/configuration path without adding a second feature flag.
- Return accepted task/title data on success and an idempotent
  `{accepted:false, reason:"title_not_pending"}` when the marker is absent. Preserve validation and
  authorization errors from the task service.
- Update regular/pending/restricted-mode catalog tests, MCP mode API tests, executor mode-resolution
  tests, handler forwarding tests, MCP integration tests, and the raw-WebSocket MCP-prefix regression
  inventory. Assert that ordinary task mode does not include the new tool schema.

### Conditional first-turn instruction

- Add a conditional placeholder to `apps/backend/config/prompts/kandev-context.md` that emits both the
  `set_task_title_kandev` inventory entry and first-call instruction only for title-pending task mode.
- Extend `sysprompt.KandevContextOptions` in `apps/backend/internal/sysprompt/sysprompt.go` with the
  pending-title capability. The instruction says to call the tool before any other work or tool call,
  even though a provisional title already exists, and requests a title targeting three words and no
  more than six when practical. It remains absent when the marker is not true.
- Pass the marker through every first-turn composition path in
  `apps/backend/internal/orchestrator/task_operations.go`,
  `event_handlers_workflow.go`, and
  `apps/backend/internal/task/handlers/message_handlers.go`. Keep canonicalization and trusted-context
  handling intact.
- For passthrough sessions, prepend the equivalent short instruction only while pending; do not add the
  full task MCP boilerplate that passthrough intentionally skips.
- Extend sysprompt placeholder/canonicalization tests, orchestration launch/auto-start tests, and
  `apps/backend/internal/mcp/server/sysprompt_sync_test.go` so prompt references and registered mode
  catalogs cannot drift. Explicitly prove that ordinary tasks get neither prompt text nor tool schema.

---

## Frontend

### User setting and hydration

- Add `agentGeneratedTaskTitles` to `UserSettingsState` and its default (`false`) in
  `apps/web/lib/state/slices/settings/types.ts` and `settings-slice.ts`.
- Add `agent_generated_task_titles` to `apps/web/lib/types/http-user-settings.ts`, boot/SSR mapping in
  `apps/web/lib/ssr/user-settings.ts`, WS mapping in `apps/web/lib/ws/handlers/users.ts`, and any shared
  settings edit-state mapper that enumerates portable fields.
- Add `apps/web/components/settings/agent-generated-task-title-settings.tsx` under
  `TaskActionsSettings`. Register it with `useSettingsSaveContributor`, and explain visibly that the
  title input disappears for new tasks/subtasks, a prompt is required, a provisional title from the
  prompt's first six words appears immediately, and the fallback remains if the agent cannot rename it.
- Add focused component, SSR, store/hydration, and WS mapping tests.

### New Task dialog

- Read the preference once from the hydrated app store and thread an `autoTitle`/`requiresManualTitle`
  contract through `task-create-dialog.tsx`, form-body props, footer props, submit handlers, and
  `buildCreateTaskPayload`.
- In create mode with the preference enabled, suppress `InlineTaskName`, focus the description input,
  disable all creation variants until the prompt has content, omit the manual title, and send
  `auto_title: true`. Edit mode and session-only mode retain their existing title behavior.
- Ensure GitHub/Jira/Linear title autofill does not become a hidden source of truth in auto-title mode.
  Voice auto-send, create-only, plan-mode, repository selection, and task-create-last-used persistence
  continue through the shared submit path.
- Update helper, footer, payload-builder, form-body, state, and dialog tests for enabled/disabled/edit
  cases and empty-prompt gating.

### New Subtask dialog and mobile contract

- Read the same preference in `new-subtask-dialog.tsx`. When enabled, omit the title state/input from
  `new-subtask-form-parts.tsx`, require the existing prompt, and have `use-subtask-submit.ts` send
  `auto_title: true` without a manual title. When disabled, preserve the proposed
  `Parent / Subtask N` title and editable input.
- Keep the existing full-height phone dialogs, fixed headers/footers, internal scroll owner, safe-area
  behavior, and touch-sized actions. No new drawer or route is needed: the prompt simply becomes the
  first editable field in the existing task and subtask surfaces.
- Use the shipped `TaskCreateDialog` and `NewSubtaskDialog` phone compositions as the mobile exemplars;
  share all preference, payload, validation, and submit logic across viewports.
- Add component/hook tests for both setting states and payloads.

---

## Tests

- **What:** portable preference defaults false, PATCH preserves omitted values, and boot/WS payloads
  carry it. **Files:** user store/DTO/service tests, backend boot-state tests, frontend SSR/WS/settings
  tests. **How:** table-driven Go JSON tests plus Vitest mapping/component tests.
- **What:** provisional title whitespace normalization, first-six-word selection, shorter prompts,
  empty-prompt rejection, and pending metadata persistence. **Files:**
  `apps/backend/internal/task/service/service_tasks_test.go` and task HTTP handler tests. **How:**
  table-driven service tests and handler-to-service integration.
- **What:** ordinary title edits win, the first pending MCP title succeeds, repeats are idempotent, and
  `task.updated` is published. **Files:** task-service and MCP handler tests. **How:** real repository +
  event bus tests and WS action handler tests.
- **What:** ordinary task sessions receive no title prompt/tool schema, while pending-task catalog and
  conditional first-turn instruction stay aligned across direct launch, workflow auto-start,
  message-start, and passthrough. **Files:** MCP server/sysprompt/executor/orchestrator/task handler
  tests. **How:** regular-versus-pending catalog assertions include the three-word target and six-word
  practical cap, and captured launch-prompt tests verify the same gating, guidance, and call ordering.
- **What:** enabled/disabled UI states produce the correct title visibility, validation, and HTTP
  payload for tasks and subtasks. **Files:** focused `*.test.ts(x)` files beside settings and dialogs.
  **How:** Vitest component/hook tests with mocked API calls.

## E2E Tests

- **Scenario:** GIVEN the setting is enabled, WHEN a user creates a task from the desktop dialog, THEN
  the title input is absent, empty prompt is blocked, and the six-word provisional title appears.
  Backend service/MCP tests cover the mock-agent `set_task_title_kandev` replacement and its
  idempotent late-call behavior. **File:**
  `apps/web/e2e/tests/task/agent-generated-task-titles.spec.ts`.
- **Scenario:** GIVEN the setting is enabled on a phone viewport, WHEN the user opens the subtask
  creation surface, THEN the prompt is reachable and usable without a title control, actions remain
  within the viewport, and the document has no horizontal overflow. The shared task dialog contract is
  covered by focused frontend tests. **File:**
  `apps/web/e2e/tests/task/mobile-agent-generated-task-titles.spec.ts`.
- **Scenario:** GIVEN the user saves the setting and reloads, WHEN they reopen task creation, THEN the
  enabled behavior persists. **File:** desktop spec; restore the prior user setting in `afterEach`.

## Public documentation

- Update `docs/public/tasks-and-workflows.md` with the setting, fallback lifecycle, empty-prompt rule,
  and manual-rename fallback.
- Update `docs/public/coordination.md` so subtask title requirements describe both setting states.
- Update `docs/public/automation-and-mcp.md`, `docs/public/agent-communication.md`, and
  `docs/public/coverage.json` with the task-bound `set_task_title_kandev` contract, three-word target,
  six-word practical cap, and mode boundary.
- Validate public docs with `node --test scripts/validate-public-docs.test.mjs` and
  `node scripts/validate-public-docs.mjs`.

---

## Implementation Waves And Parallel Candidates

The default execution order is sequential in the primary conversation. These waves do not authorize
subagents.

Wave 1:
- [x] [Task 01: Backend preference and provisional-title lifecycle](task-01-backend-title-lifecycle.md) — done

Wave 2:
- [x] [Task 02: Task MCP tool and first-turn instruction](task-02-mcp-title-tool.md) — done

Wave 3:
- [x] [Task 03: Settings and task/subtask dialogs](task-03-frontend-title-flow.md) — done

Wave 4:
- [x] [Task 04: End-to-end coverage and public docs](task-04-e2e-and-docs.md) — done

## Final verification

- Backend focused tests: 3,665 passing across 15 affected packages; MCP server tests: 149 passing.
- Backend lint: `make -C apps/backend lint` — 0 issues.
- Frontend focused Vitest suite: 98 passing across 9 files.
- Frontend lint and TypeScript typecheck — passing.
- Desktop and mobile Playwright scenarios — 1 passing each.
- Public documentation tests and validator — 58 tests passing; 41 published pages validated.
