---
spec: docs/specs/ui/requirements/acp-model-configuration-summary.md
created: 2026-07-30
status: implemented
---

# Implementation Plan: Preserve ACP Model Across Context Reset

## Overview

Capture the task session's effective model before the ACP fast-path creates a
fresh provider-native session, then reapply that model after reset. Add a
lifecycle regression test first, implement the minimal backend repair, and
cover the user-visible workflow/reset/refresh path with the existing mock ACP
agent.

## Confirmed root cause

- `ResetAgentContext` in
  `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`
  captures and reapplies only `CachedModeState`.
- The ACP adapter implements reset by calling `NewSession`, so OpenCode and the
  E2E mock agent advertise their provider-default model for the fresh session.
- That `session_models` event replaces the execution cache and is persisted as
  task-session runtime state. The frontend correctly renders the authoritative
  event/boot state, so after refresh it shows the fresh session's default model.
- Full process restart already uses `effectiveSessionRuntimeConfig` during ACP
  initialization and is not the missing path.

---

## Backend

### Capture and reapply the pre-reset model

- Update
  `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go` so
  `ResetAgentContext` captures `execution.GetModelState()` and resolves the
  effective model through the existing persisted-runtime-over-profile/cache
  precedence before calling `ResetSession`.
- Add a focused model-reapply helper beside
  `reapplySessionModeAfterReset`. After the fresh session is installed on the
  execution, call `agentctl.SetModel` with the captured effective model.
- Treat an empty model as a no-op. Match the existing mode-reset failure
  behavior: log a warning and leave provider-reported state authoritative if
  the provider rejects the model.
- Keep config-option restoration out of this repair. Session mode continues to
  use its existing independent reapply path.

### Regression fixture

- Extend `restartMockAgentctlServer` in
  `apps/backend/internal/agent/runtime/lifecycle/manager_interaction_test.go`
  to accept `agent.session.set_model` and record its `model_id` payload.
- Add `TestManager_ResetAgentContext_ReappliesSessionModel`. Seed a non-default
  persisted/current model, invoke the ACP fast-path reset, and assert that
  `mock-smart` is sent after `agent.session.reset`, rather than accepting the
  fresh session's `mock-fast` default.

---

## Frontend

No frontend production code changes are planned. The task model selector in
`apps/web/components/task/model-selector.tsx` already consumes live
`session_models` state and boot hydration correctly; the backend repair restores
the authoritative event stream it needs.

The rendered desktop and mobile surfaces share the same session-model state.
There is no composition, navigation, touch-target, scrolling, or breakpoint
change. The existing
`apps/web/e2e/tests/chat/mobile-model-selector.spec.ts` remains the mobile
reachability and rendering coverage for that shared selector.

---

## Tests

- **What:** an ACP context reset reapplies the active non-default model to the
  fresh session using the exact model ID captured before reset.
  **File:**
  `apps/backend/internal/agent/runtime/lifecycle/manager_interaction_test.go`.
  **How:** extend the WebSocket mock to record request payloads; the test must
  first fail because no `agent.session.set_model` request is sent, then pass
  after the lifecycle change.
- **What:** persisted runtime model wins over the fresh provider default and
  remains the session's hydrated model.
  **File:**
  `apps/backend/internal/agent/runtime/lifecycle/manager_interaction_test.go`.
  **How:** use `mockWorkspaceInfoProvider` plus a non-default cached model and
  assert the exact post-reset request ordering and value under `go test -race`.

## E2E Tests

- **Scenario:** **GIVEN** a task starts with the mock agent's non-default
  `Mock Smart` model, **WHEN** proceeding into a workflow step whose `on_enter`
  actions reset context and auto-start the agent, **THEN** the selector remains
  `Mock Smart` after the step completes and after a page refresh.
- **File:** `apps/web/e2e/tests/workflow/workflow-step-proceed.spec.ts`.
- **What to verify:** seed the workflow/profile through `ApiClient`, proceed
  through the task UI, assert the selector's accessible trigger before reset,
  after reset, and after reload. The mock agent deliberately advertises
  `Mock Fast` on every new session, so the assertion fails on the current bug
  without relying on OpenCode or external credentials.

The existing mobile selector spec is sufficient for mobile parity because the
repair changes only backend state convergence and does not alter the shared
selector's phone composition or interaction.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [Task 01: Reapply the effective model after ACP context reset](task-01-reapply-model-after-reset.md)

Wave 2:

- [x] [Task 02: Cover workflow reset and refresh in Playwright](task-02-workflow-reset-e2e.md)

Execution is sequential in the primary conversation. Task 02 depends on the
backend behavior from Task 01, and no task is marked parallel-safe.

## Validation commands

- `cd apps/backend && go test -race -run 'TestManager_ResetAgentContext_ReappliesSession(Model|Mode)$' ./internal/agent/runtime/lifecycle`
- `cd apps/web && pnpm e2e:run tests/workflow/workflow-step-proceed.spec.ts -- --grep "preserves selected model across context reset"`

## Risks and non-goals

- A fresh ACP session can publish its default model while reset is in flight.
  Capturing the effective model before `ResetSession` avoids treating that
  transient event as reset intent; the post-reset `SetModel` convergence event
  restores durable state.
- The provider may reject a previously valid model. The repair logs that
  failure and reports the provider's actual state instead of fabricating
  success.
- This repair does not restore arbitrary non-model ACP config options, change
  frontend model-selection precedence, or alter full-process restart behavior.
