---
id: "01-distinguish-recorded-waits"
title: "Distinguish recorded waiting reasons"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-WAIT-REASONS-001
acceptance_criteria:
  - AC-TASKS-TASK-WAIT-REASONS-001.1
  - AC-TASKS-TASK-WAIT-REASONS-001.2
  - AC-TASKS-TASK-WAIT-REASONS-001.3
  - AC-TASKS-TASK-WAIT-REASONS-001.4
system_design:
  - ../../specs/tasks/system-design/task-wait-reasons.md
---

# Task 01: Distinguish recorded waiting reasons

## Outcome

Every task surface names a recorded quota, session-capacity, or WIP wait instead
of showing the task as ordinary work in progress.

## In scope

- Backend: `statussummary` quota-wait model, derivation, projector events,
  rebuild, the SQLite quota read, and the service repair path.
- Frontend: `resolveWaitReason`, state icons, task-list rows, kanban cards,
  graph step nodes, the desktop sidebar, the mobile switcher, and locale
  catalogs.

## Exclusions

- Provider block policy and model order are unchanged.
- No new settings, permissions, or migrations.

## Acceptance conditions

1. Each surface shows the recorded wait with quota first and WIP last
   (`AC-TASKS-TASK-WAIT-REASONS-001.1`).
2. Pending questions, live activity, terminal state, and interruption keep their
   own presentation (`AC-TASKS-TASK-WAIT-REASONS-001.2`).
3. Only the current session's route generation supplies a quota wait
   (`AC-TASKS-TASK-WAIT-REASONS-001.3`).
4. Quota read failure, rebuild, and revisioned repair behave as specified
   (`AC-TASKS-TASK-WAIT-REASONS-001.4`).

## Verification

- Existing task status-summary Go tests and focused temporary SQLite and
  service checks for current, stale, and terminal route identity.
- Existing web suites for the touched components, web typecheck, and
  changed-file Oxlint.

## Files likely touched

- `apps/backend/internal/task/statussummary/quota_wait.go`
- `apps/backend/internal/task/statussummary/projector.go`
- `apps/backend/internal/task/repository/sqlite/status_summary_quota_wait.go`
- `apps/backend/internal/task/service/service_status_summary_quota_wait.go`
- `apps/web/lib/ui/waiting-reason.tsx`
- `apps/web/components/task/task-state-icon.tsx`

## Dependencies

None.
