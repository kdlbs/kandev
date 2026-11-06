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
- Review corrections: session-read error propagation, terminal guarded-commit
  errors, and an immutable task/source-step arrival fence in
  `task/models/workflow_step_transition_guard.go` and
  `task/repository/sqlite/task.go`.
- Permanent registered-handler/repository retry, arrival, completion and watcher
  capacity regressions beside the orchestrator handler; desktop and phone PR
  settings/outcome tests, including touch help through the existing drawer.

## Out of scope

- GitLab, automation changes, step-entry evaluation.

## Verification

- `make fmt`, `make typecheck test lint`.
- Targeted Playwright: `tests/workflow/workflow-pr-merged.spec.ts`,
  `tests/workflow/workflow-settings.spec.ts`,
  `tests/workflow/workflow-children-completed.spec.ts`.
- Review browser coverage also includes
  `tests/workflow/mobile-workflow-pr-events.spec.ts` on `mobile-chrome`.

## Delivery review evidence

The initial hosted registered-seam run reached five causal assertion failures:
three session-read recovery schedules, terminal-session commit recovery, and
same-step departure/reentry. Live commit recovery, healthy terminal commit,
stale-CAS, genuine no-session, unchanged-arrival and all four completion/watcher
capacity controls passed. The healthy fallback moved correctly but its exact
read-count assertion also saw unrelated running-to-waiting reconciliation.
The corrected fixture starts that live session idle; assertions are retained.

The four completion controls exercise actual persisted task state and the real
watcher counter/admission consumer, including unchecked-final steps. No change
to completion behavior is justified by those passing controls.

Corrected-source baseline RED/shared controls and candidate GREEN, full final CI,
and actual desktop/phone retry-zero browser results remain pending. Source,
attempt, raw logs and artifacts are retained in the delivery session's evidence.
No contributor code, compiler, dependencies, tests or hooks ran on the review host.
