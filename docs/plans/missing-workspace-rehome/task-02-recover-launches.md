---
id: "02-recover-launches"
title: "Recover launches with one idempotent retry"
status: completed
wave: 2
depends_on:
  - "01-persist-rehome-generations"
plan: "plan.md"
requirements:
  - REQ-TASKS-MISSING-WORKSPACE-REHOME-001
  - REQ-TASKS-MISSING-WORKSPACE-REHOME-002
  - REQ-EXECUTORS-CODER-TASK-ROOT-DURABILITY-001
acceptance_criteria:
  - AC-TASKS-MISSING-WORKSPACE-REHOME-001.1
  - AC-TASKS-MISSING-WORKSPACE-REHOME-001.3
  - AC-TASKS-MISSING-WORKSPACE-REHOME-001.4
  - AC-TASKS-MISSING-WORKSPACE-REHOME-001.5
  - AC-TASKS-MISSING-WORKSPACE-REHOME-001.6
  - AC-TASKS-MISSING-WORKSPACE-REHOME-002.3
  - AC-TASKS-MISSING-WORKSPACE-REHOME-002.4
  - AC-EXECUTORS-CODER-TASK-ROOT-DURABILITY-001.4
system_design:
  - ../../specs/tasks/system-design/missing-workspace-rehome.md
---

# Task 02: Recover launches with one idempotent retry

## Summary

Classify a physically missing workspace without string matching and coordinate
one replacement launch across workflow auto-start, explicit start, resume, and
authorized recovery. Persist complete failure outcomes and fence stale events.

## In scope

- Typed lifecycle error reason for missing canonical task workspace.
- Exact-root checks in default SSH and remote-contribution materialization.
- Task-scoped orchestrator recovery coordinator and restart reconciliation.
- Step-entry, created-session, resume, and `task.launch.recover` integration.
- Exactly-one retry, concurrent follower joining, and stale-generation fences.
- Durable original/recovery errors and replacement terminalization.

## Out of scope

- User-interface rendering.
- Other reuse-unsafe categories.

## Acceptance

- Phase transition recovery retains task ID and current workflow step and starts
  exactly one replacement live session under concurrency.
- Attempt one cannot recursively rehome; a replacement failure is durable and
  visible through the task status projection.
- Healthy reuse executes the existing path without allocating recovery state.
- A task root nested beneath another checkout initializes and reaches its own
  repository root instead of adopting the ancestor origin.

## Verification

```bash
cd apps/backend && go test -race ./internal/agent/runtime/lifecycle ./internal/orchestrator -run 'Test.*(MissingWorkspace|Rehome|PhaseTransition)'
```

## Files likely touched

- `apps/backend/internal/task/models/workspace_binding.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_operations.go`
- `apps/backend/internal/orchestrator/session_launch.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/task_launch_recovery.go`
- `apps/backend/internal/task/statussummary/projector_helpers.go`

## Dependencies

- Task 01 durable claim and generation model.

## Risks

- Session lifecycle and cancellation locks currently key by session, while this
  operation crosses two session identities.
- Initial-prompt failures must use the replacement execution stamp.

## Parallelism

`sequential`

## Inputs

- Task 01 results.
- Existing launch-failure recovery, workflow step-entry, and lifecycle
  singleflight patterns.

## Results

Pending.
