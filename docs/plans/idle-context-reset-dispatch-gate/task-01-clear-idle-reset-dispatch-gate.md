---
id: "01-clear-idle-reset-dispatch-gate"
title: "Clear the idle reset dispatch gate"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002
acceptance_criteria:
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.3
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.5
system_design:
  - ../../specs/tasks/system-design/workflow-step-agent-start-ownership.md
---

# Task 01: Clear the Idle Reset Dispatch Gate

## Summary

Clear the pending dispatch-only gate when a successful idle context reset removes its completion signal. Apply the correction to both provider-reset paths.

## In scope

- Add a failing regression test for an idle ACP reset with a buffered dispatch-only completion.
- Prove that a follow-up dispatch-only prompt reaches agentctl after the reset.
- Add pending-gate coverage to the existing process-restart state test.
- Clear the old pending flag after both reset paths drain the old signal.

## Out of scope

- Clear the gate when `waitForPendingDispatchedPrompt` times out.
- Change cancellation escalation or active-turn reset behavior.
- Add frontend behavior or browser tests.

## Acceptance

- An ACP session reset removes the old completion signal and clears its matching pending flag.
- The next prompt reaches agentctl without a predecessor timeout.
- The process-restart fallback clears the same signal and flag pair.

## Verification

```bash
go test -race ./internal/agent/runtime/lifecycle -run 'TestManager_(ResetAgentContext_ClearsIdleDispatchGate|RestartAgentProcess_Success)$' -count=1
git diff --check
```

Run the commands from `apps/backend` and the repository root, respectively.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction_test.go`
- `docs/plans/idle-context-reset-dispatch-gate/plan.md`
- `docs/plans/idle-context-reset-dispatch-gate/task-01-clear-idle-reset-dispatch-gate.md`

## Dependencies

None.

## Risks

- A reset path that drains the signal without clearing the flag will keep the execution unusable.
- A flag clear before successful context replacement can release a valid active generation.

## Parallelism

`sequential`

## Inputs

- `REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002`
- `docs/specs/tasks/system-design/workflow-step-agent-start-ownership.md`
- `docs/decisions/2026-08-30-context-reset-quiesces-active-turn.md`
- GitHub issue `#3210`
- Existing reset and pending-prompt tests in the lifecycle package.

## Results

- RED: The focused command failed 2 tests before the production correction.
- `TestManager_RestartAgentProcess_Success` found that process replacement kept the stale gate.
- `TestManager_ResetAgentContext_ClearsIdleDispatchGate` found the same state after an ACP reset.
- GREEN: The focused command passed 2 tests after both reset paths cleared the gate.
- Final: The race-enabled verification passed 2 tests. `git diff --check` passed.
- Changed files: `manager_interaction.go`, `manager_interaction_test.go`, and this plan package.
- No known blockers remain.
