---
id: "01-log-onenter-deferral"
title: "Log the on-enter deferral"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WIP-LIMIT-PULL-SYSTEM-001
acceptance_criteria:
  - AC-TASKS-WIP-LIMIT-PULL-SYSTEM-001.3
  - AC-TASKS-WIP-LIMIT-PULL-SYSTEM-001.11
system_design:
  - ../../specs/tasks/system-design/wip-limit-pull-system.md
---

# Task 01: Log the On-Enter Deferral

## Change

In `apps/backend/internal/orchestrator/event_handlers_workflow.go`, report the
task, session, source and destination steps, destination WIP limit, and
`wip_limit_reached` after a session-lifecycle transition commits but before
returning with its destination entry deferred. Do not alter queue placement,
promotion or session state, and do not use the session log for guarded decisions.

## Validation

The operating scenario was run as a disposable Go overlay: unmodified upstream
failed because the deferral log was missing, and the candidate passed while
preserving queued placement and deferred entry. Existing guarded-decision
lifecycle tests, specification validation, and normal commit hooks passed. The
temporary overlay was removed; no permanent test was added.
