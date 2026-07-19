---
spec: docs/specs/tasks/mcp-task-agent-profile-default/spec.md
created: 2026-07-19
status: complete
---

# Implementation Plan: MCP-Created Task Agent Profile Default

## Overview

Add a normalized enum to backend-owned portable user settings, then consume it at the MCP handler's omitted-profile boundary. Once that contract is available, the MCP resolver and responsive Task Actions UI can be implemented in parallel, followed by integrated desktop/mobile Playwright coverage and public documentation.

## Backend

### User-settings contract and persistence

- In `apps/backend/internal/user/models/models.go`, define the canonical values `current_task` and `workspace_default`, add `MCPTaskAgentProfileDefault string` to `UserSettings`, and normalize missing or unknown stored values to `current_task`.
- In `apps/backend/internal/user/dto/dto.go`, add the response field and optional PATCH field `mcp_task_agent_profile_default`.
- In `apps/backend/internal/user/controller/controller.go` and `apps/backend/internal/user/service/service.go`, map and validate the field, preserve PATCH omission semantics, and include it in `user.settings.updated`.
- In `apps/backend/internal/user/store/sqlite.go`, include the value in JSON writes and normalize it on empty/legacy JSON reads. No SQL migration is needed.
- Extend `apps/backend/internal/user/store/sqlite_test.go`, `apps/backend/internal/user/service/service_test.go`, `apps/backend/internal/user/dto/dto_test.go`, and `apps/backend/internal/backendapp/boot_state_user_settings_test.go` for defaulting, round-trip, validation, PATCH omission, event/DTO mapping, and camel-case boot-state projection.

### MCP profile resolution

- In `apps/backend/internal/mcp/handlers/handlers.go`, add a narrow user-preference reader dependency to `Handlers` and wire it from `p.services.User` in `apps/backend/internal/backendapp/helpers.go`.
- Split agent-profile selection from executor inheritance in `resolveMCPAutoStartConfigWithError`. When `agent_profile_id` is explicit, retain it without reading the preference. When omitted, use the selected policy:
  - `current_task`: retain the current parent/source, workflow, workspace chain.
  - `workspace_default`: skip parent/source profile inheritance, then resolve the workflow step/default and finally the new task's target workspace `DefaultAgentProfileID`.
- Continue inheriting `ExecutorID` and `ExecutorProfileID` from parent/source tasks in both modes.
- Return a validation-classified error before `CreateTask` when workspace-default mode can resolve neither a workflow nor workspace profile. Preserve metadata resolution for `start_agent=false`.
- Update the `create_task_kandev` descriptions in `apps/backend/internal/mcp/server/server.go` so agents understand explicit override and both omitted-profile policies; keep the input schema unchanged. Cover the wording/schema in `apps/backend/internal/mcp/server/server_test.go` or the nearest registration test.
- Extend `apps/backend/internal/mcp/handlers/handlers_test.go` with top-level, subtask, workflow-precedence, cross-workspace, explicit override, deferred-start, no-resolvable-default, settings-read-error, and unchanged executor-inheritance cases. Update constructor wiring tests in `apps/backend/internal/backendapp/helpers_test.go` if needed.

## Frontend

### Settings state and wire mapping

- Add the enum field to `apps/web/lib/types/http-user-settings.ts`, `apps/web/lib/types/backend.ts`, `apps/web/lib/state/slices/settings/types.ts`, and the default state in `apps/web/lib/state/slices/settings/settings-slice.ts`.
- Normalize missing or unknown HTTP/boot and WebSocket values to `current_task` in `apps/web/lib/ssr/user-settings.ts` and `apps/web/lib/ws/handlers/users.ts`.
- Carry the value through the full-state reconstruction helpers in `apps/web/hooks/use-user-display-settings.ts` and `apps/web/components/settings/editors-settings-state.tsx` so unrelated display/editor saves cannot reset it.
- Update the affected mapping fixtures/tests, including `apps/web/lib/ssr/user-settings.test.ts`, `apps/web/lib/ws/handlers/users.test.ts`, `apps/web/hooks/use-ensure-user-settings.test.ts`, and any full `UserSettingsState` fixtures surfaced by typecheck.

### Task Actions control

- Add `apps/web/components/settings/mcp-task-agent-profile-default-settings.tsx` and its focused test.
- Render an accessible two-choice control in `TaskActionsSettings` in `apps/web/components/settings/general-settings.tsx`, with labels **Current task profile** and **Workspace default profile**. Visible plain-language copy explains the trigger, explicit-profile override, each resolution path, when to choose it, and the risk of reusing an expensive current-task profile. Register the draft with the shared settings save provider so persistence follows the page-wide **Save changes** workflow.
- Keep both options fully visible and touch-accessible at narrow widths. Use the existing settings card and radio-choice patterns; do not introduce a desktop-only interaction.
- Update `apps/web/components/settings/general-nav.ts` so the Task Actions description covers MCP task defaults as well as archive safeguards.

## Tests

- **Legacy/default normalization:** `apps/backend/internal/user/store/sqlite_test.go`, `apps/web/lib/ssr/user-settings.test.ts`, and `apps/web/lib/ws/handlers/users.test.ts` prove missing/unknown values become `current_task`.
- **Persistence and validation:** `apps/backend/internal/user/service/service_test.go` and `apps/backend/internal/user/dto/dto_test.go` prove both values round-trip, omission preserves state, invalid PATCH values fail, and events carry the saved value.
- **Resolver compatibility:** `apps/backend/internal/mcp/handlers/handlers_test.go` proves existing current-task priority is unchanged.
- **Workspace-default resolution:** the same Go test file covers top-level tasks, subtasks, workflow precedence, target-workspace selection, deferred starts, explicit overrides, unchanged executor inheritance, missing workflow/workspace defaults, and settings lookup errors.
- **MCP discoverability:** `apps/backend/internal/mcp/server/server_test.go` or the nearest tool-registration test asserts the description documents explicit override and the saved omitted-profile policy without changing the schema.
- **Optimistic UI:** `apps/web/components/settings/mcp-task-agent-profile-default-settings.test.tsx` proves the payload, saving state, successful selection, guarded rollback, and protection against overwriting a newer workspace/live value.

## E2E Tests

- Add `apps/web/e2e/tests/task/mcp-task-agent-profile-default.spec.ts`: select **Workspace default profile** in Task Actions, persist it, run a mock agent whose profile differs from the seeded workspace default, invoke `create_task_kandev` without `agent_profile_id`, and assert the child session/task metadata uses the workspace default. Retain or extend the existing inheritance test in `apps/web/e2e/tests/task/subtask.spec.ts` to prove default compatibility.
- Add `apps/web/e2e/tests/task/mobile-mcp-task-agent-profile-default.spec.ts`: navigate to Task Actions using the mobile settings path, change the choice, assert persistence after reload, and check that the card/control fits the mobile viewport without horizontal overflow. The viewport-independent MCP routing outcome is covered by the desktop integration test and Go resolver tests.
- Extend `apps/web/e2e/helpers/api-client.ts` and reset payloads in `apps/web/e2e/fixtures/test-base.ts` with `mcp_task_agent_profile_default: "current_task"` so tests remain isolated.

## Public Documentation

- Update `docs/public/tasks-and-workflows.md` to describe the Task Actions choice, compatibility default, explicit tool override, workflow-before-workspace precedence, and unresolved-default validation behavior.
- Update `docs/public/feature-status.md` only if its Task Actions summary needs terminology changes after implementation.

## Implementation Waves

Wave 1:

- [x] [Task 01 - Backend preference contract](task-01-backend-preference-contract.md) (`done`)

Wave 2 (parallel after Task 01; disjoint backend/frontend ownership):

- [x] [Task 02 - MCP profile resolution](task-02-mcp-profile-resolution.md) (`done`)
- [x] [Task 03 - Responsive Task Actions setting](task-03-responsive-task-actions-setting.md) (`done`)

Wave 3 (parallel after integrated backend/frontend behavior):

- [x] [Task 04 - Desktop and mobile E2E](task-04-desktop-mobile-e2e.md) (`done`)
- [x] [Task 05 - Public documentation](task-05-public-documentation.md) (`done`)

## Verification

Run formatting before broad verification:

```bash
make -C apps/backend fmt
cd apps && pnpm --filter @kandev/web exec prettier --write \
  components/settings/mcp-task-agent-profile-default-settings.tsx \
  components/settings/mcp-task-agent-profile-default-settings.test.tsx
```

Targeted and full checks:

```bash
cd apps/backend && go test ./internal/user/... ./internal/mcp/handlers/... ./internal/mcp/server/... ./internal/backendapp/...
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web test -- \
  lib/ssr/user-settings.test.ts \
  lib/ws/handlers/users.test.ts \
  components/settings/mcp-task-agent-profile-default-settings.test.tsx
cd apps/web && pnpm e2e -- --project=chromium e2e/tests/task/mcp-task-agent-profile-default.spec.ts
cd apps/web && pnpm e2e -- --project=mobile-chrome e2e/tests/task/mobile-mcp-task-agent-profile-default.spec.ts
make -C apps/backend test
make -C apps/backend lint
cd apps && pnpm --filter @kandev/web build:vite
cd apps && pnpm --filter @kandev/web lint
```

The Playwright commands must run after rebuilding the production web/backend artifacts through the repository's E2E build path; use `make test-e2e` when the focused command does not rebuild them.

## Risks and Assumptions

- The preference is deliberately per-user rather than per-workspace; `workspace_default` resolves dynamically from the target workspace.
- Workspace-default mode deliberately preserves workflow step/default policy, then fails closed when neither workflow nor target workspace provides a profile. It never falls back to the caller/parent profile.
- Profile policy and executor inheritance currently share one helper. The implementation must separate those decisions without changing executor behavior.
- E2E reset must restore `current_task`; otherwise a worker-scoped user setting can leak into unrelated MCP subtask tests.
