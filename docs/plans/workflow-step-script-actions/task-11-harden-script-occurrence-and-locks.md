---
id: 11-harden-script-occurrence-and-locks
title: Harden script occurrence and lock ownership
status: done
wave: 9
depends_on:
  - 04-integrate-workflow-triggers
plan: plan.md
requirements:
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-002
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-004
acceptance_criteria:
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-002.2
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-002.3
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-002.4
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-004.1
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-004.2
system_design:
  - ../../specs/tasks/system-design/workflow-step-script-actions.md
---

# Task 11: Harden script occurrence and lock ownership

## Summary

Close the review gaps in repeated-transition identity and in-process run-lock
retention before the workflow script runtime is considered complete.

## In scope

- Propagate a unique durable turn or transition occurrence through legacy entry
  and exit paths instead of deriving identity only from task, session, and step
  coordinates.
- Preserve duplicate-event reuse for one occurrence while allowing a later
  cycle through the same steps to execute a new script run.
- Replace the unbounded per-run `sync.Map` with a keyed lock whose entry is
  removed after the last holder or waiter releases it.
- Add a true profile-switch orchestration test that proves completion and exit
  messages bind to the source session while entry binds to the selected
  destination session.

## Out of scope

- New triggers, retries, or changes to failure policy.

## Acceptance

1. Two distinct cycles through the same source, destination, and session create
   distinct script runs; replaying either cycle reuses only that cycle's run.
2. Concurrent dispatch remains serialized, and the keyed lock registry returns
   to zero after every waiter leaves.
3. Tests cross the actual profile-switch boundary and assert both session and
   execution identities for source and destination messages.

## Verification

```bash
cd apps/backend && go test -race -tags fts5 -run 'Test.*WorkflowScript|Test.*RunScript|Test.*ProfileSwitch' ./internal/orchestrator
cd apps/backend && go test -tags fts5 ./internal/orchestrator ./internal/workflow/engine
git diff --check
```

## Files likely touched

- `apps/backend/internal/orchestrator/workflow_script_runner.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- Focused orchestrator tests for repeated transitions and profile switching.

## Dependencies

- Task 04 supplies the current coordinator and lifecycle wiring.

## Risks

- Naively deleting a mutex after unlock can let a third caller create a second
  lock while an earlier waiter still owns the first one.
- A random occurrence generated at dispatch time breaks replay deduplication;
  identity must come from the durable lifecycle event.

## Parallelism

`parallel-safe` with Task 12. This task owns backend orchestration only.

## Results

Implemented durable occurrence propagation through legacy and engine workflow
transition paths, so duplicate delivery reuses one run while later cycles get
new runs. Replaced the unbounded per-run lock map with a keyed reference-counted
registry and added profile-switch coverage asserting source and destination
session/execution identities. Race-enabled focused orchestrator tests pass.
