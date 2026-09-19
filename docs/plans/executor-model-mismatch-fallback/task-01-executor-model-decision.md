---
id: "01-executor-model-decision"
title: "Define executor model decisions"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/requirements/no-silent-model-fallback.md"
---

# Task 01: Define executor model decisions

## Acceptance

- An unadvertised requested model causes no model-selection call, and the agent default continues.
- An explicit fallback applies only when advertised, while advertised apply errors remain explicit.
- Initial launch, context reset, and workspace rebind use one typed decision helper without duplicate calls.

## TDD scenarios

1. RED: Replace strict-gone tests with no-call and provider-default expectations.
2. RED: Add absent fallback, empty catalog, method-not-supported, and auto-fallback cases.
3. RED: Add context-reset and workspace-rebind decision cases.
4. GREEN: Add the typed decision and apply the executor-authoritative order.
5. REFACTOR: Remove duplicated model-selection branches and keep one policy owner.

## Verification

- `cd apps/backend && go test -tags fts5 -run 'TestApplyStartModelPolicy|TestInitializeAndPrompt.*Model|TestReapplySessionModel|TestWorkspaceRebind.*Model' ./internal/agent/runtime/lifecycle`
- `cd apps/backend && go test -tags fts5 ./internal/agent/runtime/lifecycle`
- `git diff --check`

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/start_model.go`
- `apps/backend/internal/agent/runtime/lifecycle/start_model_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/session.go`
- `apps/backend/internal/agent/runtime/lifecycle/session_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_workspace_rebind.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_workspace_rebind_test.go`

## Dependencies

None.

## Parallelism

Sequential. Tasks 02 and 03 consume the typed decision contract.

## Inputs

- The amended no-silent-model-fallback specification.
- The executor-authoritative model-selection ADR.
- The current strict `applyStartModelPolicy` implementation.

## Output contract

Report the decision outcomes, model-selection call lists, RED evidence, GREEN evidence, and test results.

## Risks

- An empty catalog can mean unsupported selection or incomplete provider data.
- Runtime model overrides must follow the same executor catalog rules.
- A handled default decision must prevent a later profile-layer retry.

## Results

Implemented the typed executor-authoritative decision and reused it for initial
launch, context reset, and workspace rebind. Lifecycle coverage passed with
1,857 tests.
