---
id: "01-workflow-pr-lifecycle-events"
title: "Move tasks on linked PR merge or close"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-PR-EVENTS-001
acceptance_criteria:
  - AC-TASKS-WORKFLOW-PR-EVENTS-001.1
  - AC-TASKS-WORKFLOW-PR-EVENTS-001.2
  - AC-TASKS-WORKFLOW-PR-EVENTS-001.3
  - AC-TASKS-WORKFLOW-PR-EVENTS-001.4
  - AC-TASKS-WORKFLOW-PR-EVENTS-001.5
  - AC-TASKS-WORKFLOW-PR-EVENTS-001.6
  - AC-TASKS-WORKFLOW-PR-EVENTS-001.7
  - AC-TASKS-WORKFLOW-PR-EVENTS-001.8
system_design:
  - ../../specs/tasks/system-design/workflow-pr-lifecycle-events.md
---

# Task 01: Move Tasks on Linked PR Merge or Close

## Summary

Add the `on_pr_merged` and `on_pr_closed` step events end to end: model and
serialization, the orchestrator consumer of `github.task_pr.updated`, the step
editor, tests, and docs.

## In scope

- `StepEvents.OnPRMerged` / `OnPRClosed` and every generic-trigger enumeration
  listed in the system design.
- `processTaskPRWorkflowEvent` in
  `apps/backend/internal/orchestrator/event_handlers_github_pr_workflow.go`,
  called from `handleTaskPRUpdated`.
- `StepTransitionTriggerPRMerged` / `PRClosed` history mapping.
- `EventTransitionSelect` in the step editor, locale keys in every locale.
- Public docs: workflow tips events table and import/export trigger table.

## Out of scope

- GitLab, automation changes, step-entry evaluation.

## Verification

- `make fmt`, `make typecheck test lint`.
- Targeted Playwright: `tests/workflow/workflow-pr-merged.spec.ts`,
  `tests/workflow/workflow-settings.spec.ts`,
  `tests/workflow/workflow-children-completed.spec.ts`.
