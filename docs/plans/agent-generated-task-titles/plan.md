---
spec: docs/specs/tasks/agent-generated-titles.md
created: 2026-07-31
updated: 2026-08-02
status: done
---

# Implementation Plan: Agent-Generated Task Titles

## Overview

Waves 1–4 delivered the portable preference, backend-owned provisional-title lifecycle, task-bound MCP
mutation, conditional first-turn instruction, shared desktop/mobile dialogs, E2E coverage, and public
docs. The 2026-08-02 continuation makes that shipped flow default-on and replaces pending-only launch
gating with durable session ownership. The integration seam becomes the pair of persisted task metadata
keys: creation sets pending intent, the first eligible initial-turn launch atomically records its owner,
only that session receives title guidance and schema, and either the owner or a human title update clears
both keys.

---

## Backend

### Portable user setting

- Extend `apps/backend/internal/user/models/models.go`, `dto/dto.go`, and
  `service/service.go` with `AgentGeneratedTaskTitles bool` /
  `agent_generated_task_titles`, using pointer PATCH semantics and a missing-field default of `true`.
  Preserve an explicitly stored `false`.
- Extend `apps/backend/internal/user/store/sqlite.go` JSON encode/decode and defaults. This remains in
  the existing `users.settings` JSON blob, so no schema migration is needed.
- Project the field through `apps/backend/internal/backendapp/boot_state_routes.go` and the existing
  `user.settings.updated` payload.
- Add focused store, DTO, service, event-payload, and boot-state tests beside the existing archive and
  MCP-profile preference tests.

### Provisional-title creation and pending state

- Add `AutoTitle bool` to `service.CreateTaskRequest` in
  `apps/backend/internal/task/service/service_requests.go` and define
  `models.MetaKeyAgentTitlePending = "agent_title_pending"` and
  `models.MetaKeyAgentTitleOwnerSessionID = "agent_title_owner_session_id"` in
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
- Make an ordinary `Service.UpdateTask` call with a non-nil title remove both title-lifecycle keys
  before its single task-row update. Add a task-service method for the MCP path that validates the
  500-character title limit, accepts only a pending task whose persisted owner matches the
  server-injected session ID, updates title and metadata atomically, and publishes the standard
  `task.updated` event.
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
  `title` argument description to target about six words in sentence case and use a short title phrase
  rather than a sentence or progress update, and clarify that the existing title is provisional and must
  still be replaced.
- Extend `resolveTaskSessionMCPMode` in
  `apps/backend/internal/orchestrator/executor/executor_execute.go` to select the existing title-pending
  task mode only when `agent_title_pending` is true and `agent_title_owner_session_id` equals the
  launching session. Config and Office modes win. Keep the existing internal mode string stable and
  thread it through the existing agentctl mode validation/configuration path without another flag.
- Return accepted task/title data on success, an idempotent
  `{accepted:false, reason:"title_not_pending"}` when the marker is absent, and
  `{accepted:false, reason:"title_not_owner"}` when the bound session is not the owner. Preserve
  validation and authorization errors from the task service.
- Update regular/pending/restricted-mode catalog tests, MCP mode API tests, executor mode-resolution
  tests, handler forwarding tests, MCP integration tests, and the raw-WebSocket MCP-prefix regression
  inventory. Assert that ordinary task mode does not include the new tool schema.

### Conditional first-turn instruction

- Add a SQLite/PostgreSQL-safe task repository compare-and-set that persists exactly one owner while
  pending. It must distinguish a new claim, an idempotent same-session claim, and a denied claim; the
  orchestrator publishes `task.updated` only when it newly persists ownership. Claim failure aborts the
  launch before prompt persistence/composition, title-capable MCP configuration, or agent-process
  startup. Workspace/agentctl-only preparation stays in ordinary task mode and does not claim.
- Claim after a concrete session exists and its task/config/Office eligibility is known, but before the
  initial turn is recorded or wrapped. Route direct starts, prepared-session starts, message-start,
  workflow auto-start, structured prompts, and passthrough prompts through the same ownership helper.
  Pre-wrapping paths may call it before `StartCreatedSession`; the repeated same-session call is
  idempotent.
- Audit every `StartAgent: true`, resume, and execution-profile switch path. A pending unowned session
  must be claimed before its first agent process starts; an already owned session resolves title mode
  only when its ID matches the owner. Workspace-only `StartAgent: false` preparation never exposes the
  title mode.

- Add a conditional placeholder to `apps/backend/config/prompts/kandev-context.md` that emits both the
  `set_task_title_kandev` inventory entry and first-call instruction only for title-pending task mode.
- Extend `sysprompt.KandevContextOptions` in `apps/backend/internal/sysprompt/sysprompt.go` with the
  pending-title capability. The instruction says to call the tool before any other work or tool call,
  even though a provisional title already exists, and requests a short title phrase targeting about six
  words in sentence case rather than a sentence or progress update. It remains absent when the marker is
  not true.
- Pass the claim result—not a fresh pending-only check—through every first-turn composition path in
  `apps/backend/internal/orchestrator/task_operations.go`,
  `event_handlers_workflow.go`, and
  `apps/backend/internal/task/handlers/message_handlers.go`. Keep canonicalization and trusted-context
  handling intact.
- For passthrough sessions, prepend the equivalent short instruction only while pending; do not add the
  full task MCP boilerplate that passthrough intentionally skips.
- Preserve both internal metadata keys against stale task snapshots while pending, and exclude both
  from stale merge payloads after a human or owner resolution so neither key can be resurrected.
- Extend sysprompt placeholder/canonicalization tests, orchestration launch/auto-start tests, and
  `apps/backend/internal/mcp/server/sysprompt_sync_test.go` so prompt references and registered mode
  catalogs cannot drift. Explicitly prove that ordinary tasks get neither prompt text nor tool schema.

---

## Frontend

### User setting and hydration

- Add `agentGeneratedTaskTitles` to `UserSettingsState` and its default (`true`) in
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

- **What:** portable preference defaults true when missing, PATCH preserves omitted and explicit-false
  values, and boot/WS payloads carry it. **Files:** user store/DTO/service tests, backend boot-state
  tests, frontend SSR/WS/settings tests. **How:** table-driven Go JSON tests plus Vitest
  mapping/component tests.
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
  tests. **How:** regular-versus-pending catalog assertions include the six-word sentence-case target,
  and captured launch-prompt tests verify the same gating, guidance, and call ordering.
- **What:** two concurrent initial-turn launches produce one durable owner; owner retry is idempotent;
  non-owner, config, Office, External, and merely prepared sessions never claim; owner failure does not
  reassign; human/agent resolution clears both keys; stale updates cannot restore them. **Files:** task
  repository/service, executor, orchestrator, message handler, and MCP handler tests. **How:** real
  SQLite concurrency tests, environment-gated PostgreSQL parity, and captured launch/catalog tests.
- **What:** enabled/disabled UI states produce the correct title visibility, validation, and HTTP
  payload for tasks and subtasks. **Files:** focused `*.test.ts(x)` files beside settings and dialogs.
  **How:** Vitest component/hook tests with mocked API calls.

## E2E Tests

- **Scenario:** GIVEN a fresh or field-missing preference, WHEN Settings and task/subtask creation load,
  THEN the toggle is enabled and prompt-first behavior is active; GIVEN an explicit opt-out, the manual
  title flow remains. Keep the shared E2E fixture explicitly opted out so unrelated manual-title tests
  retain their intended baseline. **Files:** existing desktop/mobile agent-title specs and
  `apps/web/e2e/fixtures/test-base.ts`.

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

- Update `docs/public/tasks-and-workflows.md` with the default-on setting, explicit opt-out, single-owner
  fallback lifecycle, empty-prompt rule, and manual-rename fallback.
- Update `docs/public/coordination.md` so subtask title requirements describe both setting states.
- Update `docs/public/automation-and-mcp.md`, `docs/public/agent-communication.md`, and
  `docs/public/coverage.json` with the task-bound `set_task_title_kandev` contract, six-word
  sentence-case target, exactly-one-session ownership, and mode boundary.
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

Wave 5:
- [x] [Task 05: Single-owner title handoff](task-05-single-owner-title-handoff.md) — done

Wave 6:
- [x] [Task 06: Default-on title preference](task-06-default-on-title-preference.md) — done

Wave 7:
- [x] [Task 07: Regression coverage and public docs](task-07-regression-and-docs.md) — done

## Previous shipped verification

- Backend focused tests: 3,665 passing across 15 affected packages; MCP server tests: 149 passing.

## Completion

- [x] [Task 05: Single-owner title handoff](task-05-single-owner-title-handoff.md) — done
- [x] [Task 06: Default-on title preference](task-06-default-on-title-preference.md) — done
- [x] [Task 07: Regression coverage and public docs](task-07-regression-and-docs.md) — done

The continuation was verified with focused backend tests, full backend FTS5 race tests,
golangci-lint, targeted frontend tests and lint, and public-document validators. PostgreSQL parity
remains environment-gated and was not run locally because no test DSN was available; the existing PR
E2E checks passed at the prior head and the shared fixture now opts those scenarios out explicitly.
- Backend lint: `make -C apps/backend lint` — 0 issues.
- Frontend focused Vitest suite: 98 passing across 9 files.
- Frontend lint and TypeScript typecheck — passing.
- Desktop and mobile Playwright scenarios — 1 passing each.
- Public documentation tests and validator — 58 tests passing; 41 published pages validated.

## Extension verification

- [x] SQLite repository, service, orchestrator, executor, task-handler, and MCP tests prove atomic
  one-owner claims and owner-bound title compare-and-set behavior.
- [x] User-store and frontend mapping tests prove missing/default `true` and explicit `false`
  preservation.
- [x] Backend race suite, backend lint, frontend lint, targeted frontend tests, and public-doc
  validation passed.
- [ ] PostgreSQL parity and the new desktop/mobile browser runs remain CI/environment-gated.
