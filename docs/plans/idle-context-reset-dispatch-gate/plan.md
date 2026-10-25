---
created: 2026-08-31
status: done
requirements:
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002
system_design:
  - ../../specs/tasks/system-design/workflow-step-agent-start-ownership.md
legacy_specs: []
---

# Implementation Plan: Clear the Idle Context-Reset Dispatch Gate

## Overview

Clear the dispatch-only completion gate when an idle context reset removes its buffered completion signal. Cover the ACP reset path and the process-restart fallback.

## Scope

### In scope

- Reproduce the idle dispatch-only state in lifecycle manager tests.
- Clear `dispatchedPromptPending` after each successful reset path drains `promptDoneCh`.
- Prove that the next prompt can reach agentctl after an ACP session reset.
- Prove that the process-restart fallback also clears the old gate.

### Out of scope

- Change the timeout behavior in `waitForPendingDispatchedPrompt`.
- Change active-turn quiescence or cancellation ownership.
- Add UI recovery for sessions that entered this state before the correction.
- Explain unrelated differences between prompt-completion log counts.

## Technical approach

Update `Manager.ResetAgentContext` in `manager_interaction.go`. Clear the atomic pending flag in the same locked state update that drains the old completion signal.

Update `Manager.resetAgentRestartState` in the same file. Apply the same state transition to the process-restart fallback.

Add focused tests in `manager_interaction_test.go`. The ACP test will reset an idle execution and send a follow-up dispatch-only prompt through the real manager path.

Extend the existing restart-state test with a pending gate. The test will prove that process replacement clears the signal and its matching flag.

## Tests

- `AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.3`: `TestManager_ResetAgentContext_ClearsIdleDispatchGate` proves that the follow-up prompt reaches the new ACP session.
- `AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.3`: `TestManager_RestartAgentProcess_Success` proves that the process-restart fallback clears the prior gate.
- `AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.5`: Existing pending-wait tests remain unchanged because the timeout still preserves active predecessor ownership.

## Work orders

- [x] [Task 01: Clear the idle reset dispatch gate](task-01-clear-idle-reset-dispatch-gate.md)

## Verification results

- RED: Both focused regression tests failed because the reset paths kept `dispatchedPromptPending` set.
- GREEN: The focused tests passed after both reset paths cleared the flag after the signal drain.
- Final: The race-enabled verification passed 2 tests. `git diff --check` also passed.

## Risks

- Clearing the gate before the old turn is quiescent can admit concurrent prompts. The existing orchestrator reset boundary must remain unchanged.
- Clearing only the ACP fast path leaves adapters that use process replacement exposed to the same error.
