---
spec: docs/specs/tasks/requirements/mcp-task-agent-profile-default.md
created: 2026-08-12
status: complete
---

# Implementation Plan: Creator-Session Task Inheritance

## Overview

Replace task-origin profile inheritance with verified creator-session
inheritance for session-bound `create_task_kandev` calls. First add a typed
initial-session runtime seed. Then carry trusted session identity through the MCP
transport and apply the seed during profile resolution. Update the visible
setting, tool contract, public docs, and existing desktop/mobile E2E coverage.

## Backend

### Initial-session runtime seed

- Add a task metadata key in `apps/backend/internal/task/models/models.go` for
  the initial session runtime configuration.
- Add a model helper that resolves one effective `SessionRuntimeConfig` from a
  task session. Start with `AgentProfileSnapshot`, then apply
  `runtime_config`, `session_mode`, and `runtime_config_overrides` in precedence
  order. Clone option maps so source and target metadata never alias.
- In `apps/backend/internal/orchestrator/executor/executor_execute.go`, map the
  task launch seed to `SessionMetaKeyRuntimeConfigOverrides` only when
  `PrepareSession` creates the task's initial session. Remove the launch-only
  key from the session metadata copy.
- Cover profile-only state, live runtime updates, explicit overrides, option
  replacement, and initial-versus-later session preparation.

### Trusted creator context

- In `apps/backend/internal/mcp/server/handlers.go`, add the server's own
  `sessionID` to the create-task backend payload as `source_session_id`. Keep it
  absent in external mode.
- In `apps/backend/internal/mcp/handlers/handlers.go`, parse the internal field
  and load it through `taskSvc.GetTaskSession`. Require the session's `TaskID`
  to equal `source_task_id` before using any profile or runtime data.
- Refactor `resolveMCPAutoStartConfigWithError` to return profile, executor, and
  optional initial runtime seed. In `current_task` mode, a verified creator
  session wins over parent/source task profile metadata for both top-level tasks
  and subtasks.
- Preserve the existing executor chain. A subtask still inherits executor state
  from its parent. A top-level task still inherits it from its source task.
- Persist the runtime seed beside the resolved task launch profile before
  `CreateTask`, including when `start_agent=false`.
- Suppress the seed when `agent_profile_id` is explicit, when a workflow launch
  profile wins, or when the user selected `workspace_default`.
- Fail before task persistence when supplied session context cannot be verified.
  Calls with no session context retain the current parent/source compatibility
  fallback.

### MCP contract text

- Update `apps/backend/internal/mcp/server/server.go` so task-mode descriptions
  name the creating session and its effective configuration.
- Keep external-mode descriptions explicit that there is no creating session
  and that `current_task` falls back to the parent task.
- Add focused schema/description and forwarded-payload tests in
  `apps/backend/internal/mcp/server/handlers_test.go` and `server_test.go`.

## Frontend

### Task Actions setting

- Change the visible `current_task` label to **Creating session profile** in
  `apps/web/src/locales/en/settings.json` and every locale catalog.
- Rewrite the setting description and option help in
  `apps/web/components/settings/mcp-task-agent-profile-default-settings.tsx`.
  State the affected tool, effective model/options behavior, precedence, cost
  risk, external fallback, and exclusions without relying on hover help.
- Update the component test to prove that the saved enum remains `current_task`
  and that the new explanation is visible.

Mobile uses the existing Task Behavior page and radio-card layout. The nearest
mobile exemplar is the shipped settings card in
`mobile-mcp-task-agent-profile-default.spec.ts`. The page remains the single
scroll owner. Radio rows retain 44px touch targets, and all explanatory text
wraps inside the card without horizontal overflow.

## Public documentation

- Update the explanation section in `docs/public/tasks-and-workflows.md`. This
  page remains an explanation page with a short configuration procedure.
- Explain that `current_task` follows the creating session for live task-mode
  calls, including model and reasoning changes. Document the external fallback
  and the cases that prevent runtime inheritance.

## Tests

- **What:** effective creator configuration merges profile snapshot, provider
  state, session mode, and explicit overrides.
  **File:** `apps/backend/internal/task/models/session_runtime_config_test.go`.
  **How:** table-driven unit tests with cloned-map assertions.
- **What:** only the initial task session receives the persisted runtime seed.
  **File:** `apps/backend/internal/orchestrator/executor/executor_test.go`.
  **How:** prepare two sessions and inspect their persisted metadata.
- **What:** the MCP server forwards its bound session identity and external mode
  does not invent one.
  **File:** `apps/backend/internal/mcp/server/handlers_test.go`.
  **How:** focused handler payload tests.
- **What:** a verified non-primary session wins task profile resolution for a
  subtask and a top-level task, while executor inheritance remains unchanged.
  **File:** `apps/backend/internal/mcp/handlers/create_task_creator_session_test.go`.
  **How:** service-backed handler tests with two source-task sessions.
- **What:** explicit, workflow, and workspace-default profiles suppress copied
  runtime values. Mismatched session identity fails before persistence.
  **File:** `apps/backend/internal/mcp/handlers/create_task_creator_session_test.go`.
  **How:** table-driven handler and resolver tests.
- **What:** the renamed setting retains the `current_task` wire value and shows
  the precedence contract.
  **File:**
  `apps/web/components/settings/mcp-task-agent-profile-default-settings.test.tsx`.
  **How:** rendered component assertions and save-payload assertion.

## E2E Tests

- **Scenario:** a second session uses another profile and changes model/options,
  then creates a subtask and a top-level task.
  **File:** `apps/web/e2e/tests/task/subtask.spec.ts`.
  **What to verify:** both created initial sessions use the creator profile and
  effective runtime values. The subtask keeps the parent executor profile.
- **Scenario:** workspace-default policy prevents creator-session inheritance.
  **File:**
  `apps/web/e2e/tests/task/mcp-task-agent-profile-default.spec.ts`.
  **What to verify:** the created session uses the target workspace profile and
  does not contain the creator runtime seed.
- **Scenario:** the revised setting is touch-readable and viewport-safe.
  **File:**
  `apps/web/e2e/tests/task/mobile-mcp-task-agent-profile-default.spec.ts`.
  **What to verify:** the new label and explanation are visible, the option
  persists, each choice remains touch-usable, and no horizontal overflow occurs.

## Verification Results

All implementation waves are complete. The focused backend, frontend, i18n,
documentation, and managed E2E checks pass:

```text
rtk go test ./internal/mcp/server ./internal/mcp/handlers ./internal/task/models ./internal/task/service ./internal/orchestrator/executor -count=1  PASS
rtk go test ./internal/task/repository/sqlite -run TestCreateTaskSessionWithInitialRuntimeSeedConsumesOnceAcrossConcurrentAndReplacementSessions -count=1  PASS
GOCACHE=/tmp/kandev-go-build GOLANGCI_LINT_CACHE=/tmp/kandev-golangci-lint make lint  PASS
pnpm --filter @kandev/web test -- components/settings/mcp-task-agent-profile-default-settings.test.tsx  PASS (4 tests)
pnpm --filter @kandev/web typecheck  PASS
pnpm --filter @kandev/web i18n:check  PASS (existing locale parity/orphan advisories only)
node --test scripts/validate-public-docs.test.mjs  PASS (60 tests)
node scripts/validate-public-docs.mjs  PASS (41 public pages)
pnpm e2e:run --no-build tests/task/subtask.spec.ts  PASS (15 tests)
pnpm e2e:run --no-build tests/task/mcp-task-agent-profile-default.spec.ts  PASS (1 test)
pnpm e2e:run --no-build --project mobile-chrome tests/task/mobile-mcp-task-agent-profile-default.spec.ts  PASS (1 test)
```

The PR capture run also passed and produced validated, compressed desktop and
mobile settings screenshots in the ignored `.pr-assets` directory.

Review follow-up added the repository-level persistence guarantee for the
launch-only seed. Session creation now reads and removes the seed in the same
transaction as the first-session insert, and the SQLite regression covers
concurrent launches plus deletion and replacement of the seed-bearing session.
The MCP server also omits `source_session_id` unless its bound task context is
present.

## Implementation Waves And Parallel Candidates

The default is sequential execution in the primary conversation. These waves do
not authorize subagents.

Wave 1:
- [x] [task-01-initial-runtime-seed](task-01-initial-runtime-seed.md)

Wave 2:
- [x] [task-02-creator-session-resolution](task-02-creator-session-resolution.md)

Wave 3:
- [x] [task-03-explain-session-default](task-03-explain-session-default.md)

Wave 4:
- [x] [task-04-end-to-end-inheritance](task-04-end-to-end-inheritance.md)

## Risks

- Task metadata is copied into every prepared session today. The executor must
  translate the launch-only seed only for the initial session and must not leak
  the task-only key into session metadata.
- Runtime options are provider-specific. Copy them only with the verified
  creator profile. Never mix them into another resolved profile.
- Workflow profile precedence is subtle. Tests must cover a pinned step, an
  unpinned step with a workflow default, and a stepless workflow.
- `source_session_id` is trusted attribution. It must stay server-injected and
  must be checked against `source_task_id` before use.
