---
id: "01-scoped-settings"
title: "Persist and migrate scoped views"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-WORKSPACE-SIDEBAR-VIEWS-001
acceptance_criteria:
  - AC-UI-WORKSPACE-SIDEBAR-VIEWS-001.1
  - AC-UI-WORKSPACE-SIDEBAR-VIEWS-001.2
  - AC-UI-WORKSPACE-SIDEBAR-VIEWS-001.3
  - AC-UI-WORKSPACE-SIDEBAR-VIEWS-001.4
  - AC-UI-WORKSPACE-SIDEBAR-VIEWS-001.5
system_design:
  - ../../specs/ui/system-design/workspace-sidebar-task-views.md
---

# Task 01: Persist and migrate scoped views

## Summary

Backend typed map, single-entry PATCH, access checks, versioned migration and every effective settings projection.

## In scope

Backend typed map, single-entry PATCH, access checks, versioned migration and every effective settings projection. Write failing targeted tests before production changes and preserve
existing sidebar normalization and optimistic-write guarantees.

## Out of scope

Frontend integration is task 02. No changes to Threads, integration queries,
shared preferences, automatic task colors, or manual task ordering.

## Acceptance

- Scoped writes use existing CAS and preserve unrelated workspace/user settings.
- Migration is repeatable and atomic; new workspaces get canonical defaults.
- HTTP/WS/boot expose consistent scoped state and reject ambiguous legacy mutations.

## Verification

Run from repository root. Installation is needed once in a fresh worktree.
Add and run the tests mapped in [plan](plan.md#tests), capturing red then green.

```bash
(cd apps/backend && go test ./internal/user/... ./internal/backendapp/...)
```

## Files likely touched

- `apps/backend/internal/user/models/models.go`
- `apps/backend/internal/user/dto/dto.go`
- `apps/backend/internal/user/service/service.go`
- `apps/backend/internal/user/store/sqlite.go`
- `apps/backend/internal/user/controller/controller.go`
- `apps/backend/internal/user/handlers/handlers.go`
- `apps/backend/internal/backendapp/services.go`
- `apps/backend/internal/backendapp/boot_state_routes.go`
- `New focused sidebar_workspace_views.go and matching tests in user packages`

## Dependencies

None. Backend expansion and its tests are independently verifiable.

## Risks

Workspace access must use the authorized listing path; generic repository listing alone may not establish access. Migration errors must not commit a marker. Legacy client writes must fail atomically.

## Parallelism

Sequential. No delegation authorized.

## Inputs

- [Requirements](../../specs/ui/requirements/workspace-sidebar-task-views.md)
- [System design](../../specs/ui/system-design/workspace-sidebar-task-views.md)
- Existing sidebar action tests and `internal/user/service/user_settings_cas_test.go`.
- Existing mobile sidebar and Task views access E2E suites for task 02.

## Results

Passed: `GOCACHE=/tmp/kandev-sidebar-go-cache go test ./internal/user/... ./internal/backendapp/...` (local socket access required for httptest servers).
Passed: `GOCACHE=/tmp/kandev-sidebar-go-cache go test -race ./internal/user/service -run TestSidebarWorkspace -count=1`.
Storage, service and DTO regressions failed before implementation and now pass.
