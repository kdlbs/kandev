---
plan: docs/plans/workflow-move-profile-override-repair/plan.md
status: completed
---

# Task 01: Preserve Prepared Target Profile

## Dependencies

None.

## Files

- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow_profile_test.go`

## RED

Add a test that enters an auto-start step with a one-time profile override, forces the target session through the `CREATED` prepared-session path, and asserts the persisted session and launch request retain the override profile.

## GREEN

Pass the move entry options through the auto-start prompt to the prepared-session start path, and make that path skip the generic workflow-profile replacement when an explicit move profile is present.

## Acceptance

The new regression fails on the current implementation, passes after the minimal fix, and ordinary workflow-default start behavior remains unchanged.
