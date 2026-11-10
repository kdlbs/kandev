---
id: "02-transition-proof"
title: "Commit outcome proof and preserve Review handoff"
status: done
wave: 2
depends_on:
  - "01-source-claims"
plan: "plan.md"
requirements:
  - REQ-TASKS-COMPLETION-OUTCOMES-001
  - REQ-TASKS-COMPLETION-OUTCOMES-002
  - REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-003
  - REQ-TASKS-SIGNAL-PAYLOAD-DELIVERY-001
  - REQ-TASKS-SIGNAL-PAYLOAD-DELIVERY-002
acceptance_criteria:
  - AC-TASKS-COMPLETION-OUTCOMES-001.1
  - AC-TASKS-COMPLETION-OUTCOMES-001.3
  - AC-TASKS-COMPLETION-OUTCOMES-001.4
  - AC-TASKS-COMPLETION-OUTCOMES-001.5
  - AC-TASKS-COMPLETION-OUTCOMES-001.7
  - AC-TASKS-COMPLETION-OUTCOMES-001.8
  - AC-TASKS-COMPLETION-OUTCOMES-002.1
  - AC-TASKS-COMPLETION-OUTCOMES-002.2
  - AC-TASKS-COMPLETION-OUTCOMES-002.3
  - AC-TASKS-COMPLETION-OUTCOMES-002.4
  - AC-TASKS-COMPLETION-OUTCOMES-002.5
  - AC-TASKS-COMPLETION-OUTCOMES-002.6
  - AC-TASKS-COMPLETION-OUTCOMES-002.7
  - AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-003.7
  - AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-001.10
  - AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-001.11
  - AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-001.13
  - AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-002.8
system_design:
  - ../../specs/tasks/system-design/workflow-completion-outcomes.md
  - ../../specs/tasks/system-design/workflow-explicit-completion-signal.md
  - ../../specs/tasks/system-design/workflow-completion-signal-payload-delivery.md
---

# Task 02: Commit outcome proof and preserve Review handoff

## Summary

Bind completion consumption and its receipt to the exact winning signal in the move transaction.
Prove that the configured Review recipient receives the latest eligible handoff through both completion paths.
Explain asynchronous acknowledgement and recovery through the tool and public documentation.

## In scope

- Extend the completion guard with source-session and signal identity.
- Recheck the pending signal in move admission and persist its exact ledger transition ID with the move.
- Roll back move and proof together on an essential receipt error.
- Wire engine and legacy completion, conditional bag cleanup, blocked-outcome writes, and supersession reconciliation.
- Keep task state, current route, WIP queueing, and committed destination distinct in results.
- Return matching receipts after consumption, session parking/completion, and repository restart without another transition.
- Add the four service acceptance scenarios named in the plan, with actual recipient and prompt assertions.
- Preserve executor authorization, credential refusal, clarification barriers, profile policies, and source-session recoverability.
- Add MCP serialization, tool-description, and system-prompt regressions for the new result semantics.
- Update `automation-and-mcp.md` as reference documentation and `workflow-tips.md` as workflow guidance.

## Out of scope

No changes to executor grants, shared credentials, Review policies, rendered UI, signal arguments, or the carry-claim failure contract.
Do not send newer correction text into a Review conversation after the source transition commits.

## Acceptance

1. A forced replacement race produces one move and one matching receipt, carry, and audit payload. A delayed consumer cannot clear its replacement.
2. Both legacy and engine paths deliver the winning handoff to the configured Review session. The Review/Iterate/Review cycle and queued-restart flow preserve recipient isolation.
3. Refused or unconfigured transitions report honest outcomes. Duplicate receipts survive restart and never imply reviewer dispatch or bypass task access.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/models ./internal/task/repository/sqlite ./internal/mcp/handlers -run 'Test(StepCompletionOutcome|PostgresStepCompletionOutcome|StepCompletionEntry|HandleStepComplete_)' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/orchestrator -run 'Test(StepCompletionOutcome_|StepCompletionEntryHandoff|WorkflowCredentialPreflightFailureDispatchesFollowUp)' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/mcp/server ./internal/sysprompt -run 'Test.*StepComplete|Test.*StepCompletion' -count=1)
(cd apps/backend && go run -trimpath ./cmd/sqlguard ./internal)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/persistence/storeconformance -count=1)
bash scripts/lint-go-changed apps/backend/internal/task/models/workflow_step_transition_guard.go apps/backend/internal/task/repository/sqlite/task.go apps/backend/internal/task/repository/sqlite/step_completion_entry.go apps/backend/internal/task/repository/sqlite/step_completion_outcomes_test.go apps/backend/internal/task/repository/sqlite/step_completion_outcomes_postgres_test.go apps/backend/internal/orchestrator/step_completion_entry.go apps/backend/internal/orchestrator/event_handlers_step_completion.go apps/backend/internal/orchestrator/event_handlers_workflow.go apps/backend/internal/orchestrator/workflow_store.go apps/backend/internal/orchestrator/step_completion_outcomes_test.go apps/backend/internal/orchestrator/step_completion_outcomes_acceptance_test.go apps/backend/internal/mcp/handlers/step_complete_outcomes.go apps/backend/internal/mcp/handlers/step_complete_outcomes_test.go apps/backend/internal/mcp/server/server.go apps/backend/internal/mcp/server/step_complete_outcomes_test.go apps/backend/internal/sysprompt/sysprompt.go apps/backend/internal/sysprompt/step_complete_outcomes_test.go
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/step-completion-outcomes
```

The PostgreSQL cases run with `KANDEV_TEST_POSTGRES_DSN` against a disposable database.
Record a skip separately if the database is unavailable.
Also run the plan's [documentation preflight](plan.md#documentation-preflight) against the actual changed files.
Keep command results in both this work order and the plan.

## Files likely touched

- `apps/backend/internal/task/models/workflow_step_transition_guard.go`
- `apps/backend/internal/task/repository/sqlite/task.go`
- `apps/backend/internal/task/repository/sqlite/step_completion_entry.go`
- Repository outcome tests from Task 01, including the PostgreSQL cases.
- `apps/backend/internal/orchestrator/step_completion_entry.go`
- `apps/backend/internal/orchestrator/event_handlers_step_completion.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/workflow_store.go`
- `apps/backend/internal/orchestrator/step_completion_outcomes_test.go` (new)
- `apps/backend/internal/orchestrator/step_completion_outcomes_acceptance_test.go` (new)
- `apps/backend/internal/task/repository/sqlite/step_transitions.go`
- `apps/backend/internal/mcp/handlers/step_complete_outcomes.go`
- Handler outcome code and tests from Task 01.
- `apps/backend/internal/mcp/server/server.go` and `step_complete_outcomes_test.go` (new)
- `apps/backend/internal/sysprompt/sysprompt.go` and `step_complete_outcomes_test.go` (new)
- `docs/public/automation-and-mcp.md`
- `docs/public/workflow-tips.md`
- This package's requirements, designs, ADR status, plan, and work-order results.

## Dependencies

Task `01-source-claims` supplies source identity, receipt models, and duplicate projection.

## Risks

The step-history worker is best-effort and cannot supply transaction proof.
A move receipt proves no prompt delivery by itself.
Every competing source needs the entry guard, including a mixed valid/stale-session race.
The acceptance harness must observe actual asynchronous prompt dispatch through a barrier.

## Parallelism

`sequential`

## Inputs

- [Plan tests and service acceptance matrix](plan.md)
- [Outcomes design: commit guard, reconciliation, result projection](../../specs/tasks/system-design/workflow-completion-outcomes.md)
- [Existing fixed-profile routing](../../specs/tasks/system-design/workflow-step-fixed-profile-routing.md)
- [Existing profile-session lifecycle](../../specs/tasks/system-design/workflow-profile-session-lifecycle.md)
- Existing `step_completion_handoff_entry_test.go`, `step_handoff_carry_dispatch_test.go`, and `event_handlers_workflow_dynamic_completion_test.go` harnesses.
- `/tdd`, `/docs-maintainer`, and backend test guidance.

## Results

Done.

- The completion guard now carries the source session and exact signal ID; move admission re-reads the persisted bag under the task transaction and refuses a removed or replaced source.
- The move transaction writes the committed ledger ID and destination onto the matching receipt. A missing receipt rolls back the move so the signal stays retryable.
- Carry and audit capture come from the same guarded signal; conditional bag cleanup cannot remove a replacement.
- `not_configured` and blocked refusal outcomes are projected; credential or destination refusals leave the source route intact.
- A superseded consumer republishes one wakeup for the current pending signal after releasing its guard, and the stuck-signal watchdog remains the fallback.
- A transient commit or proof failure releases the generation that exact completion attempt consumed, so the same durable signal can retry without a newer session snapshot.
- Result projection reads state, workflow, step, entry, queue placement, and primary session from one statement-consistent repository snapshot after publication.
- Tool description, system prompt, `docs/public/automation-and-mcp.md`, and `docs/public/workflow-tips.md` explain acceptance versus transition proof and later-turn replacement.

Verification: full orchestrator package passed; orchestrator race subset passed; server serialization and sysprompt regressions passed; public-docs validation, specification catalog validation, specification lint, and the plan documentation preflight passed. PostgreSQL cases were skipped because `KANDEV_TEST_POSTGRES_DSN` was unset. Service acceptance now observes the configured Review recipient and actual prompt text for the latest correction, Review/Iterate/Review, queued promotion, and credential refusal paths.
