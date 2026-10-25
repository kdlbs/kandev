---
spec: docs/specs/ui/requirements/acp-model-configuration-summary.md
created: 2026-08-18
status: implemented
---

# Implementation Plan: Preserve ACP Runtime Configuration Across Context Reset

## Overview

Replace the separate model and mode reset paths with one captured `SessionRuntimeConfig` value. Restore the model, mode, and provider options before Kandev sends another prompt.

The backend regression tests define the lifecycle contract first. A workflow E2E test proves the result through the existing selectors and page hydration.

## Confirmed root cause

- An ACP session reset emits the new session's provider defaults before the reset response returns.
- The orchestrator persists each non-empty mode event immediately.
- `reapplySessionModeAfterReset` reads persisted state after reset. It can read the new default instead of the captured pre-reset mode.
- The reset path captures and restores the model separately. It does not restore `SessionRuntimeConfig.ConfigOptions`.
- The existing mode test discards stream events. It cannot reproduce the persisted-default race.

Decision: [ADR-2026-08-18-context-reset-preserves-runtime-configuration](../../decisions/2026-08-18-context-reset-preserves-runtime-configuration.md).

---

## Backend

### Capture one effective runtime configuration

Update `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`.

- Replace `effectiveSessionModelForReset` and the separate mode capture with one reset snapshot based on `models.SessionRuntimeConfig`.
- Start with the profile model, mode, and options from `resolveProfileSessionConfig`.
- Use the live model and mode caches as fallbacks for provider state that persistence has not received.
- Apply `effectiveSessionRuntimeConfig` before session creation. Persisted provider state and explicit overrides keep their existing precedence.
- Clone the configuration-option map. Fresh events and later mutations must not change the captured value.
- Keep model and mode out of `ConfigOptions`. Each top-level field has one apply operation.

Both `ResetAgentContext` and `prepareAgentRestart` capture this value before they create a provider session.

### Restore one effective runtime configuration

Add one helper in `manager_interaction.go` for both context-reset paths.

- Apply the captured model through `reapplySessionModelAfterReset` and the executor-owned model policy.
- Apply the captured permission mode to the new ACP session ID.
- Apply each non-empty provider option through `SetConfigOption` in stable option-ID order.
- Apply the model before model-dependent options. Use the adapter's authoritative model response and option cache for later option calls.
- Update in-memory state only from accepted provider operations or convergence events.
- Return a restoration error when any captured field is rejected. The caller marks the execution as failed and does not publish reset success.
- Do not mark or publish the new session as ready until restoration succeeds. Move the restart path's early `AgentBootReady` publication behind this gate.

The helper replaces `reapplySessionModeAfterReset` in the reset and restart paths. Workspace-rebind behavior is out of scope for this repair.

### Regression fixture

Extend `restartMockAgentctlServer` in `manager_interaction_test.go`.

- Record payloads for `agent.session.set_mode` and `agent.session.set_config_option`.
- Let a reset response simulate a fresh default event that changes the workspace provider before restoration.
- Let one requested option fail so the test can prove that partial restoration blocks success.

Add focused tests for the ACP fast path and the process-restart fallback. Each test uses a non-default model, mode, and option value.

---

## Frontend

No frontend production code changes are planned. The model and mode selectors already consume authoritative session events and boot state.

This repair does not change layout, navigation, touch behavior, scrolling, or viewport composition. The backend state path is shared by desktop and mobile.

The existing `apps/web/e2e/tests/chat/mobile-model-selector.spec.ts` covers mobile selector access and rendering. A new mobile test is not required for this backend-only state repair.

---

## Tests

- **What:** the ACP reset path restores one captured model, mode, and option set after fresh defaults arrive.
  **File:** `apps/backend/internal/agent/runtime/lifecycle/manager_interaction_test.go`.
  **How:** use the WebSocket mock and assert exact payloads and order.
- **What:** the process-restart fallback restores the same configuration fields.
  **File:** `apps/backend/internal/agent/runtime/lifecycle/manager_interaction_test.go`.
  **How:** use the existing restart harness and assert the new ACP session receives every field.
- **What:** a rejected mode or option does not report reset success or send an automatic workflow prompt.
  **File:** `apps/backend/internal/agent/runtime/lifecycle/manager_interaction_test.go`.
  **How:** make the mock reject one field and assert the execution failure, missing ready event, and missing reset event.
- **What:** nil, empty, model-shaped, and mode-shaped option entries do not cause duplicate or invalid apply calls.
  **File:** `apps/backend/internal/agent/runtime/lifecycle/manager_interaction_test.go`.
  **How:** use table-driven snapshot and restore cases.

## E2E Tests

- **Scenario:** **GIVEN** three selected runtime values, **WHEN** a workflow step resets context, **THEN** each value remains active after reset and reload.
- **File:** `apps/web/e2e/tests/workflow/workflow-step-proceed.spec.ts`.
- **What to verify:** assert `session-mode-selector` and `Session model settings` before reset, after the reset turn, and after page reload.

## Public documentation

Update `docs/public/tasks-and-workflows.md`, which is a how-to guide. State that reset-context actions preserve the selected ACP model, permission mode, and provider options.

Also state that a restoration error prevents the destination step's automatic prompt. No public API or import/export contract changes.

## Verification Results

Implementation is complete. The exact task verification commands passed:

- Backend focused lifecycle reset/restart tests: 4 passed.
- Full backend lifecycle package under the race detector: 1,911 passed.
- Workflow reset and hydration E2E: 1 passed.
- Public documentation tests and validator: 61 tests passed; 41 published
  pages validated.
- PR fixup coverage: live provider option snapshots replace stale persisted
  entries, option-only catalogs satisfy restoration readiness, rejected model
  restoration fails closed, and reset failure status is persisted.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [Task 01: Restore the complete runtime configuration](task-01-restore-runtime-configuration.md)

Wave 2:

- [x] [Task 02: Cover workflow reset and hydration](task-02-workflow-reset-e2e.md)
- [x] [Task 03: Document reset preservation](task-03-public-docs.md) (`parallel-safe` with Task 02 after Task 01)

Execution is sequential in the primary conversation. The wave labels do not authorize subagents.

## Risks and non-goals

- A model change can replace the provider option catalog. The restoration order must keep dependent option values valid.
- A provider can reject a value that was valid in the prior session. The reset cannot restore the old conversation after this point.
- A partial provider apply can occur before an error. Kandev must report the accepted provider state and block automatic prompting.
- The repair does not change ACP authentication, credentials, MCP attachment resolution, CLI flags, environment values, executor selection, or profile identity.
- The repair does not change workspace-rebind session recreation.
