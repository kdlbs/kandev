---
id: 03-add-workspace-process-execution
title: Add workspace process execution
status: done
wave: 2
depends_on:
  - 01-define-script-action-contract
plan: plan.md
requirements:
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-002
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-003
acceptance_criteria:
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-002.1
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-002.5
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-002.6
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-002.7
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-003.9
system_design:
  - ../../specs/tasks/system-design/workflow-step-script-actions.md
---

# Task 03: Add workspace process execution

## Summary

Expose an idempotent runtime seam for starting, observing, and stopping a
managed command in an exact session execution workspace.

## In scope

- Runtime start/get/stop interface and exact session/execution validation.
- Agentctl stable request identity and conflicting-reuse rejection.
- Process-group timeout/stop, bounded UTF-8 output, final status, and exit code.
- Execution before the agent subprocess is prompted.

## Out of scope

- Workflow occurrence claims, trigger ordering, and chat persistence.

## Acceptance

1. The runtime resolves the selected execution workspace and managed
   environment on every supported managed executor without using the host setup
   runner.
2. Repeating an identical stable request returns the same process; conflicting
   reuse is rejected.
3. Timeout and stop terminate the complete process group and leave bounded,
   recoverable output and a typed terminal result.

## Verification

```bash
cd apps/backend && go test ./internal/agentctl/server/process ./internal/agentctl/server/api
cd apps/backend && go test ./internal/agent/runtime/lifecycle
cd apps/backend && go test -race ./internal/agentctl/server/process
git diff --check
```

## Files likely touched

- `apps/backend/internal/agentctl/server/process/`
- `apps/backend/internal/agentctl/server/api/processes.go`
- `apps/backend/internal/agentctl/client_process.go`
- `apps/backend/internal/agent/runtime/runtime.go`
- `apps/backend/internal/agent/runtime/lifecycle/process_runner.go`
- Tests beside each owner.

## Dependencies

- Task 01 supplies timeout limits and process config.

## Risks

- Caller-supplied workspace values must not escape the bound execution.
- Late output can race with terminal status and corrupt ordering.

## Parallelism

`parallel-safe` with Task 02 after Task 01. This task does not edit task
repositories or occurrence helpers.

## Inputs

- Existing agentctl process ring buffer and process-group termination.
- Runtime interfaces that hide lifecycle implementation from consumers.

## Results

Implemented the narrow runtime workspace-process seam and extended agentctl
with stable request identity, conflicting-reuse rejection, timeout status,
bounded UTF-8 output, process-group cleanup, and retained terminal results for
retry reconciliation. Start, get, and stop resolve the exact execution and
workspace, verify session ownership, and preserve the managed execution
environment. Timed-out processes release their activity lease and are terminal
for broker probing.

Verification:

- `go test ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agent/runtime/lifecycle`
- `go test -race ./internal/agentctl/server/process`
- `git diff --check`
