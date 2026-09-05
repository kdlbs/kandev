---
id: 04-integrate-workflow-triggers
title: Integrate workflow trigger scripts
status: done
wave: 3
depends_on:
  - 02-persist-workflow-script-runs
  - 03-add-workspace-process-execution
plan: plan.md
requirements:
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-002
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-003
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-004
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-005
acceptance_criteria:
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-002.2
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-002.3
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-002.4
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-003.1
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-003.2
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-003.3
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-003.4
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-003.5
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-003.6
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-003.7
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-003.8
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-004.4
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-004.7
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-005.1
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-005.3
system_design:
  - ../../specs/tasks/system-design/workflow-step-script-actions.md
---

# Task 04: Integrate workflow trigger scripts

## Summary

Wire one durable coordinator into every supported workflow boundary, with
profile-aware session binding, ordered policy gates, chat updates, and restart
reconciliation.

## In scope

- Entry, turn-completion, and exit coordination across all transition routes.
- Destination binding for entry and source binding for completion/exit.
- Sequential action execution and `block|continue` outcomes.
- Message creation/output coalescing, shutdown, duplicate delivery, and startup
  reconciliation.

## Out of scope

- Editor components and final transcript presentation.

## Acceptance

1. Entry scripts run after destination routing/configuration and before its
   prompt; completion and exit scripts run in the source session before commit.
2. Every automatic, manual, deferred, queued, explicit-launch, workflow-switch,
   and recovery route uses the same ordered failure gate.
3. Replay or restart updates the existing durable run/message and never starts
   a second process for the same occurrence.

## Verification

```bash
cd apps/backend && go test -tags fts5 -run 'Test.*WorkflowScript|Test.*RunScript' ./internal/orchestrator ./internal/workflow/engine
cd apps/backend && go test -tags fts5 ./internal/orchestrator ./internal/workflow/engine
cd apps/backend && go test -race -tags fts5 -run 'Test.*WorkflowScript' ./internal/orchestrator
git diff --check
```

## Files likely touched

- `apps/backend/internal/workflow/engine/`
- `apps/backend/internal/orchestrator/workflow_script_runner.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/workflow_callbacks.go`
- Backend application adapters and focused orchestrator tests.

## Dependencies

- Task 02 supplies durable claims and occurrence identities.
- Task 03 supplies idempotent workspace process execution.

## Risks

- Existing exit processing is best effort and must become a result-bearing gate.
- Entry work must not inherit a dead request or WebSocket context.
- Passthrough sessions still need a prepared workspace before script admission.

## Parallelism

`sequential`. This is the lifecycle integration boundary.

## Inputs

- Tasks 01 through 03 outputs.
- Current profile session start/end policy and step-entry dispatch split.

## Results

Implemented durable entry, completion, and exit script coordination with
source/destination session binding, ordered failure-policy gates, process
reconciliation, shutdown interruption, chat projection, and metrics. Focused
orchestrator and affected backend package tests pass.
