---
id: "06-completion-gate-on-entry"
title: "Completion gate on workflow entry"
status: done
wave: 4
depends_on: ["02-completion-policy-hook", "04-completion-effects", "05-bulk-move-admission"]
plan: "plan.md"
requirements:
  - REQ-TASKS-COMPLETION-005
acceptance_criteria:
  - AC-TASKS-COMPLETION-005.2
  - AC-TASKS-COMPLETION-005.4
  - AC-TASKS-COMPLETION-005.5
system_design:
  - ../../specs/tasks/system-design/task-completion.md
---

# Task 06: Completion gate on workflow entry

## Summary

Issue #5486 allows a workflow move into a completing step before checking
completion criteria. The later state update can refuse completion and leave
the task in that step. A separate move also changes the task revision and can
invalidate evidence that was current when the move began.

## Scope

- Check completion criteria in the repository transaction before committing
  entry into a completing step. Recheck queued tasks when they are promoted
  and audit a valid native override in the same transaction.
- Commit the completing step and state together for automatic admissions
  outside Office. Publish completion events from the committed state change.
  Office destinations use their approval and status pipeline.
- Settle refused workflow transitions, retain source plan mode on a refused
  completing move, and record the receipt and delivery outcome.
- Include completing moves that preserve terminal state in bulk preflight.

## Verification

- Run the focused Go race tests for task repository admission, orchestrator
  completion, and bulk preflight.
- Run changed-package Go lint and the PR documentation coverage evaluator.

## Results

Focused race tests passed in the orchestrator, SQLite task repository, and task
service packages. The documentation coverage evaluator, catalog validation,
and specification lint passed.
