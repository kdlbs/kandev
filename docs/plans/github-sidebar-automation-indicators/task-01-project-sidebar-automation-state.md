---
id: "01-project-sidebar-automation-state"
title: "Project sidebar automation state"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002
  - REQ-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.10
  - AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.1
  - AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.2
  - AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.3
system_design:
  - ../../specs/integrations/system-design/github-pr-merge-queue.md
  - ../../specs/platform/system-design/bounded-task-status-delivery.md
---
# Task 01: Project Sidebar Automation State

## Summary

Extend the bounded task status summary with active GitHub auto-fix and
auto-merge flags. Keep the projection authoritative across live option and
pull-request changes, list hydration, and backend restart.

## In scope

- Extend backend and frontend task-summary contracts with two optional flags.
- Include repository-qualified per-pull-request automation options in the
  authoritative pull-request loader.
- Aggregate only open linked pull requests.
- Refresh the complete pull-request observation set after an automation-option
  update.
- Pipe the flags through desktop and mobile task-row item models.
- Add backend and frontend contract tests before implementation.

## Out of scope

- Rendering dots, tooltips, or drawers.
- Loading detailed automation options from task rows.
- Changing automation settings or provider actions.

## Acceptance

- Independent and combined active settings produce the correct two bounded
  flags, while merged and closed pull requests do not contribute.
- Boot, restart, option-update, pull-request-update, and compare-and-set paths
  converge on the same complete replacement summary.
- Desktop and mobile item models receive the flags without mounting a per-row
  automation-options request.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/task/statussummary ./internal/backendapp ./internal/github
cd apps/web && pnpm test -- lib/ws/handlers/tasks-status-summary.test.ts components/task/task-session-sidebar-item.test.ts components/task/mobile/session-task-switcher-sheet-hooks.test.ts
```

## Files likely touched

- `apps/backend/internal/task/statussummary/model.go`
- `apps/backend/internal/task/statussummary/rebuild.go`
- `apps/backend/internal/task/statussummary/projector.go`
- `apps/backend/internal/task/statussummary/projector_events.go`
- `apps/backend/internal/task/statussummary/projector_pr.go`
- `apps/backend/internal/backendapp/status_summary_adapter.go`
- `apps/web/lib/types/task-status-summary.ts`
- `apps/web/components/task/task-session-sidebar-item.ts`
- `apps/web/components/task/mobile/session-task-switcher-sheet-item.ts`

## Dependencies

None.

## Risks

- Per-pull-request option matching must use repository ID and PR number.
- The projector must replace its complete PR observation map on option updates
  so disabled or terminal siblings cannot leave a stale aggregate flag.

## Parallelism

`sequential`

## Inputs

- Integration acceptance criterion 002.10.
- Bounded task-status delivery criteria 001.1 through 001.3.
- Existing pull-request summary projector and authoritative loader.

## Results

Implemented and verified. The bounded summary now carries active-only
auto-fix and auto-merge flags, refreshes the complete PR observation set after
option updates, and hydrates repository-qualified options through the existing
batch reader. Desktop and mobile row mappers preserve the flags without adding
per-row requests.

Verification: focused backend packages passed (2,473 tests), focused mapper
and hydration suites passed, and the desktop/mobile sidebar E2E scenarios
passed.
