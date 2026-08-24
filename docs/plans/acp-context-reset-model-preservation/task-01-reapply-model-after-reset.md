---
id: "01-reapply-model-after-reset"
title: "Reapply the effective model after ACP context reset"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/acp-model-configuration-summary.md"
---

# Task 01: Reapply the effective model after ACP context reset

## Acceptance

- `ResetAgentContext` captures the effective task-session model before the
  fresh ACP session can advertise its provider default.
- A successful ACP fast-path reset sends that exact non-empty model through
  `agent.session.set_model` after `agent.session.reset`.
- A rejected or empty model does not fail the completed context reset or
  fabricate cached state; existing session-mode preservation remains intact.

## Verification

- `cd apps/backend && go test -race -run 'TestManager_ResetAgentContext_ReappliesSession(Model|Mode)$' ./internal/agent/runtime/lifecycle`

The new model regression must fail first because the current lifecycle sends no
`agent.session.set_model` request, then pass after the production change.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction_test.go`

## Dependencies

None.

## Parallelism

Sequential. This task defines the repaired backend behavior required by Task
02.

## Inputs

- Repair additions in `What`, `Scenarios`, `Failure Modes`, and `Persistence
  Guarantees` in the linked ACP model-configuration spec
- Plan sections `Capture and reapply the pre-reset model` and `Regression
  fixture`
- Existing `reapplySessionModeAfterReset`,
  `effectiveSessionRuntimeConfig`, and
  `TestManager_ResetAgentContext_ReappliesSessionMode` patterns
- ACP adapter `SetModel` convergence events as the authoritative cache and
  persistence update

## Output contract

Report the RED failure, the captured-model precedence and request ordering,
files changed, exact targeted test result, blockers, and risk notes. Mark this
task `done` and update its plan checkbox in the same conversation.
