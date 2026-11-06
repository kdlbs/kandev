---
id: "01-completion-report"
title: "Report and retry handoff completion"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-CROSS-WORKSPACE-TASK-HANDOFF-COMPLETION-001
  - REQ-TASKS-WORKSPACE-COLLABORATION-001
acceptance_criteria:
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-COMPLETION-001.1
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-COMPLETION-001.2
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-COMPLETION-001.3
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-COMPLETION-001.4
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-COMPLETION-001.5
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-COMPLETION-001.6
  - AC-TASKS-WORKSPACE-COLLABORATION-001.4
system_design:
  - ../../specs/cross-workspace-task-handoff/system-design/completion-notification.md
  - ../../specs/tasks/system-design/workspace-collaboration.md
---

# Task 01: Report and retry handoff completion

## Summary

Use protected handoff task metadata to find the source after the delivery task
reaches a terminal state. Apply the current report and prompt grants
independently. Store comments and delivery receipts atomically. Retry event
publication and source-task wakes through the Office recovery tick.

## Files and boundaries

- `apps/backend/internal/backendapp/office_handoff_completion.go` validates
  task, session, agent, reverse-link, workspace, and grant state.
- `apps/backend/internal/office/service/handoff_completion.go` owns the
  completion event, source report, prompt wake, and outbox replay.
- `apps/backend/internal/office/repository/sqlite/handoff_completion.go` owns
  the durable receipt and atomic comment write.
- `apps/backend/internal/office/service/prompt_builder.go` and
  `scheduler_integration.go` include the delivery report in the source prompt.
- `docs/specs/office/requirements/budget-run-provenance.md` owns the attended
  classification for `handoff_completed`.

## Acceptance

Report-only grants do not start a run. Prompt-only grants do not create a
comment. Duplicate completion events do not duplicate comments or wakes.
Unassigned source tasks can receive reports. A pending prompt is retried after
the source workspace resumes, and a revoked prompt grant or changed assignee
suppresses that wake. Run coalescing does not merge distinct delivery reports.

## Verification

Focused backend tests cover report-only and prompt-only grants, duplicate
terminal events, prompt text, paused replay, retention-safe provenance, and
the source reverse-link check. Hosted CI provides the broader backend and
database coverage.
