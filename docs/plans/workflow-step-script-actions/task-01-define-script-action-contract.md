---
id: 01-define-script-action-contract
title: Define the script action contract
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-001
acceptance_criteria:
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-001.1
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-001.2
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-001.3
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-001.4
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-001.5
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-001.6
system_design:
  - ../../specs/tasks/system-design/workflow-step-script-actions.md
---

# Task 01: Define the script action contract

## Summary

Add the normalized backend and portable workflow contract for ordered
`run_script` actions on entry, turn completion, and exit.

## In scope

- Typed command, timeout, and failure-policy config with documented defaults.
- Trigger-specific enum support and ordered compilation.
- Create/update, duplicate, import/export, sync, and embedded-template
  preservation and validation.
- Characterization tests for workflows without the action.

## Out of scope

- Process execution, run persistence, chat messages, and frontend authoring.

## Acceptance

1. Every supported trigger accepts multiple ordered scripts and applies the
   exact command, timeout range, and `block|continue` defaults.
2. Every workflow ingestion and serialization boundary preserves valid config
   and rejects a recognized malformed script instead of skipping it.
3. Existing workflow definitions and unknown-action compatibility retain their
   current behavior.

## Verification

```bash
cd apps/backend && go test ./internal/workflow/models ./internal/workflow/engine ./internal/workflow/service ./internal/workflow/handlers
cd apps/backend && go test ./internal/workflow/repository/...
git diff --check
```

## Files likely touched

- `apps/backend/internal/workflow/models/workflow.go`
- `apps/backend/internal/workflow/models/portable.go`
- `apps/backend/internal/workflow/engine/types.go`
- `apps/backend/internal/workflow/engine/engine.go`
- Workflow service, handler, sync, and converter tests beside their owners.

## Dependencies

- None.

## Risks

- Trigger enums differ today; adding only one occurrence would produce a
  partially portable contract.
- Normalizing whitespace must not change the command that is executed.

## Parallelism

`sequential`. This establishes the contract used by all later work.

## Inputs

- Existing trigger action types and portable workflow converters.
- Existing validation and unknown-action compatibility behavior.

## Results

Implemented lifecycle `run_script` action types, typed validation/defaults, and
engine compilation for entry, turn-complete, and exit. Portable export now
writes normalized timeout and failure-policy defaults while retaining command
text. Existing unknown actions and workflows without scripts retain their
behavior.

Verification: `go test ./internal/workflow/models ./internal/workflow/engine`
passed (116 models tests and 212 engine tests); `gofmt` completed cleanly.
