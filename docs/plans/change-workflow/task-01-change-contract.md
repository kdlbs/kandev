---
id: "01-change-contract"
title: "Guarded workflow change and preview"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-CHANGE-WORKFLOW-001
  - REQ-TASKS-CHANGE-WORKFLOW-002
acceptance_criteria:
  - AC-TASKS-CHANGE-WORKFLOW-001.2
  - AC-TASKS-CHANGE-WORKFLOW-001.3
  - AC-TASKS-CHANGE-WORKFLOW-001.4
  - AC-TASKS-CHANGE-WORKFLOW-001.6
  - AC-TASKS-CHANGE-WORKFLOW-002.1
  - AC-TASKS-CHANGE-WORKFLOW-002.2
  - AC-TASKS-CHANGE-WORKFLOW-002.3
  - AC-TASKS-CHANGE-WORKFLOW-002.4
  - AC-TASKS-CHANGE-WORKFLOW-002.5
  - AC-TASKS-CHANGE-WORKFLOW-002.6
system_design:
  - ../../specs/tasks/system-design/change-workflow.md
---

# Task 01: Guarded workflow change and preview

## Summary

Add the opt-in mapping contract to move and preview APIs. Persist destination and
overrides together, with source/version protection and existing routing semantics.
This independently testable boundary precedes the form that submits it.

## In scope

- HTTP and WS parsing, shared service validation, and structured conflict/row errors.
- Reusable profile eligibility checks against the task's current executor.
- Atomic override, membership, WIP admission, and lifecycle-marker persistence.
- Candidate-map preflight and read-only preview, preserving real source policy.
- Tests for legacy omission, explicit defaults, failures, concurrency, and reload.
- Routing tests for later steps, initial/earlier targets, queued promotion, missing
  replacements, active-primary allowance, and stale source completion fencing.

## Out of scope

UI, public documentation, new database columns, new session policies, and bulk mapping.

## Acceptance

1. New requests validate consistently and commit the selected destination and map
   together. Stale/repeated requests and injected write failures do not partially mutate state.
2. Preview and preflight resolve draft mappings without writes. Execution and future
   entry use the persisted map under existing session, WIP, and completion policies.
3. Requests without the new object retain current behavior. Task resources and
   other tasks remain unchanged. Targeted tests pass after a demonstrated red phase.

## Verification

Run from the repository root. The new test methods use `WorkflowChange` in their
names so the first command discovers them. Do not treat an empty match as evidence.

```bash
(cd apps/backend && go test ./internal/task/models ./internal/task/service ./internal/task/handlers ./internal/task/repository/sqlite ./internal/orchestrator -run 'WorkflowChange|WorkflowAgentOverride|MoveTask|WorkflowMovePreview' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Add PostgreSQL coverage with the repository's existing DSN-gated fixture if new
SQL or locks differ by driver. Run the same targeted repository command with
`KANDEV_TEST_POSTGRES_DSN` supplied by the test environment. Record an unavailable
DSN as a verification limit; never claim a skipped database test passed.

## Files likely touched

- `apps/backend/internal/task/service/service_workflow.go`
- `apps/backend/internal/task/service/workflow_agent_overrides.go`
- `apps/backend/internal/task/service/change_workflow_test.go` (new)
- `apps/backend/internal/task/handlers/task_http_handlers.go`
- `apps/backend/internal/task/handlers/task_ws_handlers.go`
- `apps/backend/internal/task/handlers/task_move_preview_handlers.go`
- `apps/backend/internal/task/handlers/change_workflow_test.go` (new)
- `apps/backend/internal/task/repository/sqlite/task.go`
- `apps/backend/internal/task/repository/sqlite/change_workflow_test.go` (new)
- `apps/backend/internal/orchestrator/workflow_move_preview.go`
- Existing workflow-move preflight interfaces/adapters found by symbol search.
- `apps/backend/internal/orchestrator/change_workflow_test.go` (new)

## Dependencies

None. Existing override, preview, and session-targeting foundations are present.

## Risks

The current CAS rebase copies a restricted field set. Extending it carelessly can
drop the new map or manual-move markers. Task reloads can also discard draft choices.
Preserve lock order and use a candidate task without overwriting source context.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/change-workflow.md), both requirements.
- [Design](../../specs/tasks/system-design/change-workflow.md), request through preview sections.
- Existing `workflow_agent_overrides_test.go`, `service_workflow_race_test.go`,
  `service_workflow_test.go`, and `workflow_move_preview_test.go` patterns.

## Results

Implemented the explicit `workflow_change` request for HTTP and WS moves, shared
destination-agent validation, a source/version guarded admission write, and
read-only candidate-map preflight/preview. The explicit path replaces prior
overrides atomically with the destination; legacy requests keep their prior
behavior.

Verification passed:

- `(cd apps/backend && go test ./internal/task/models ./internal/task/service ./internal/task/handlers ./internal/task/repository/sqlite ./internal/orchestrator -run 'WorkflowChange|WorkflowAgentOverride|MoveTask|WorkflowMovePreview' -count=1)`
- `python3 scripts/list-docs.py validate` (299 decisions, 1129 specifications)
- `python3 scripts/lint-spec-files.py --all`
- `git diff --check`

Added a DSN-gated PostgreSQL repository test for stale-source rejection and a
successful guarded write. It was discovered and skipped because
`KANDEV_TEST_POSTGRES_DSN` is not set in this environment; PostgreSQL behavior
is therefore not reported as verified here. SQLite repository, service,
handler, and orchestrator tests passed.

### Review follow-up: explicit-target preview and preflight

- Explicit earlier-step previews resolve both the source-step profile and the
  source-session binding against the candidate override map. The preview remains
  read-only and preserves the actual source task's entry policy.
- Candidate credential preflight uses the same candidate-aware target resolver.
  The regression test proves that a draft profile B binding selects B's SSH
  executor profile instead of the persisted profile A during preflight.
- The preflight regression failed against the persisted-task resolver with an
  invalid managed GitHub remote, then passed after restoring candidate-aware
  binding resolution.
- `go test ./internal/orchestrator -count=1` passed before adding the focused
  preflight case. The final preview and preflight regression tests passed with:
  `go test ./internal/orchestrator -run '^(TestWorkflowChangeCredentialPreflightUsesCandidateBoundSession|TestPreviewWorkflowMoveUsesDraftProfileForExplicitStepTarget)$' -count=1`.
- The PostgreSQL variant remains unverified because
  `KANDEV_TEST_POSTGRES_DSN` is unset.

### Additional PR review follow-up

- Terminal explicit-target bindings now resolve as a fresh-session route for
  candidate credential preflight. `TestCandidatePreflightIgnoresTerminalStepBinding`
  covers a terminal binding whose executor profile is invalid; the preflight
  correctly uses the real source-session policy instead.
- A candidate override map for a different destination workflow now returns a
  typed invalid-change error, so the preview API classifies it as a client
  validation failure. Equivalent source timestamps in a non-UTC offset remain
  valid through the guarded write.
- `go test ./internal/orchestrator ./internal/task/service` and
  `make -C apps/backend lint` passed. The PostgreSQL-gated case remains skipped
  because `KANDEV_TEST_POSTGRES_DSN` is unset.
