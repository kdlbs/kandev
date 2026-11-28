---
id: "01-initial-target-unrouted-retry"
title: "Admit initial-target retry before route commit"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-002
acceptance_criteria:
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.13
system_design:
  - ../../specs/tasks/system-design/initial-target-first-launch-retry.md
---

# Task 01: Admit initial-target retry before route commit

## Summary

Allow `fresh_start` and `runtime_retry` recovery of a task's original session
when its initial-target step has no route yet. Fence the change by the
write-once initial snapshot, so other sessions, later entries, and earlier-step
targets are still refused.

## Files

- `apps/backend/internal/orchestrator/ceiling_entry.go`: in
  `validateUnroutedWorkflowEntry`, replace the unconditional
  `step.SessionTarget != nil` refusal with an initial-target branch that
  checks the snapshot. Put it in a helper to stay within the lint limits.
- `apps/backend/internal/orchestrator/initial_recovery_prompt.go`: in
  `initialRecoveryWorkflowBinding`, when the step targets `initial` and no
  route exists, refuse unless the snapshot session equals `sessionID`.
- `apps/backend/internal/orchestrator/fresh_start_initial_target_test.go`
  (new): reuse `newFreshStartWorkflowFixture`. Set the step's
  `SessionTarget{Kind: initial}` and seed `MetaKeyWorkflowInitialSession`.

## Steps

1. Write `TestFreshStartInitialTargetWithoutRoute` with these cases:
   `fresh_start` and `runtime_retry` each give one launch, deliver the receipt
   content and step prompt, and show the receipt as accepted; a snapshot naming
   another session gives superseded with 0 launches; `kind: step` without a
   route is still refused with 0 launches.
2. Write `TestValidateUnroutedInitialTarget`: a changed step or entry identity
   or bound destination session gives superseded; a matching payload session
   stays valid, and a missing snapshot gives permanently unavailable.
3. Run both tests. Confirm the accept cases fail with
   `workflow entry route is unavailable` (RED).
4. Implement the minimum change described in the plan.
5. Run the commands below, then set this work order to `done` and start
   Task 02. Complete `plan.md` after both work orders pass.

## Commands

```bash
cd apps/backend
go test -trimpath ./internal/orchestrator -run 'TestFreshStart|TestValidateUnrouted|TestCeiling|TestRecover|TestWorkflowSessionTarget' -count=1
go test -trimpath -race ./internal/orchestrator -run 'TestFreshStartInitialTargetWithoutRoute' -count=1
cd ../.. && bash scripts/lint-go-changed apps/backend/internal/orchestrator/ceiling_entry.go apps/backend/internal/orchestrator/initial_recovery_prompt.go apps/backend/internal/orchestrator/fresh_start_initial_target_test.go
```

## Acceptance

- The accept cases pass. The refusal cases keep 0 launches.
- No existing ceiling, fresh-start, or workflow-target test regresses.

## Lifecycle evidence

| Scenario | Owning criterion | Observable invariant | Regression evidence | Command and result |
| --- | --- | --- | --- | --- |
| Restart and recovery | `002.13` | The original session can retry while its durable initial prompt receipt remains accepted with the step instructions | `TestFreshStartInitialTargetWithoutRoute` uses the SQLite workflow fixture and checks receipt content/state plus the launched prompt | `go test -trimpath ./internal/orchestrator -run 'TestFreshStart|TestValidateUnrouted|TestCeiling|TestRecover|TestWorkflowSessionTarget' -count=1` — passed; `go test -trimpath -race ./internal/orchestrator -run 'TestFreshStartInitialTargetWithoutRoute' -count=1` — passed |
| Ownership changes | `002.13` | A foreign snapshot session, changed step, changed entry identity, or bound destination session is superseded; a matching payload session remains valid | `TestFreshStartInitialTargetWithoutRoute` and `TestValidateUnroutedInitialTarget` cover the snapshot and direct validation paths | `go test -trimpath ./internal/orchestrator -run 'TestFreshStart|TestValidateUnrouted|TestCeiling|TestRecover|TestWorkflowSessionTarget' -count=1` — passed |
| Duplicate and stale delivery | `002.13` | Stale step and entry bindings cannot start a successor launch | `TestValidateUnroutedInitialTarget` covers changed step and entry identity | `go test -trimpath ./internal/orchestrator -run 'TestFreshStart|TestValidateUnrouted|TestCeiling|TestRecover|TestWorkflowSessionTarget' -count=1` — passed |
| Entry paths | `002.13` | `fresh_start` and normalized `runtime_retry` admit only the original initial-target entry; earlier-step targets still require a route | `TestFreshStartInitialTargetWithoutRoute` covers both admitted actions and the earlier-step refusal | `go test -trimpath ./internal/orchestrator -run 'TestFreshStart|TestValidateUnrouted|TestCeiling|TestRecover|TestWorkflowSessionTarget' -count=1` — passed |
| Pause and resume | — | Not applicable: this change does not alter pause state or queue retention | Excluded; recovery admission is unchanged for resume operations | — |
| Concurrent or merged work | — | Not applicable: no concurrent launch or merge behavior changes | Excluded; the change only narrows the unrouted-entry validation case | — |

## Results

- `go test -trimpath ./internal/orchestrator -run 'TestFreshStart|TestValidateUnrouted|TestSessionFailureMetadata|TestHandleAgentFailure|TestRecover|TestWorkflowSessionTarget' -count=1` — passed. `TestValidateUnroutedInitialTarget` checks that a mismatched bound session is superseded
  and a matching payload session remains valid.
- `bash scripts/lint-go-changed apps/backend/internal/orchestrator/ceiling_entry.go apps/backend/internal/orchestrator/initial_recovery_prompt.go apps/backend/internal/orchestrator/fresh_start_initial_target_test.go apps/backend/internal/orchestrator/event_handlers_agent.go apps/backend/internal/orchestrator/event_handlers_agent_recovery_workspace_test.go` — passed, 0 issues.

## Out of scope

Writing routes during `launchPrepare`, changing behavior for `kind: step`
targets, fetch timeouts, and UI changes.
