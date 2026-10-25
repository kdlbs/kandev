---
id: "01-workspace-policy-state"
title: "Workspace policy state transport"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-IDLE-PARKING-001
acceptance_criteria:
  - AC-EXECUTORS-IDLE-PARKING-001.1
system_design:
  - ../../specs/executors/system-design/idle-runtime-parking.md
---

# Task 01: Workspace policy state transport

## Summary

Keep the saved workspace idle-suspension policy in frontend state after a page load and after a workspace lifecycle event.
This completes the existing workspace settings contract without changing policy storage or runtime ownership.

## In scope

- Include the saved enabled and timeout values in the Go boot workspace map.
- Include both values in workspace created and updated events.
- Merge fields that are present in an event, including an explicit `false` enabled value.
- Keep existing frontend values when an older event omits a policy field.
- Add regressions for settings boot, real service event publication, and explicit disabling.

## Acceptance

1. The initial settings state and other open tabs use the saved workspace policy. Partial updates preserve omitted values and accept an explicit `false` value.
2. Existing storage, defaults, authorization, suspension eligibility, and recovery ownership remain unchanged.

## Verification

```bash
(cd apps/backend && go test -tags fts5 ./internal/backendapp -run '^TestSettingsBootCarriesSavedWorkspaceIdlePolicy$' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/task/service -run '^TestWorkspaceLifecycleEventsCarrySavedIdlePolicy$' -count=1)
(cd apps/web && pnpm exec vitest run lib/routing/route-bootstrap.team-access.test.ts lib/ws/handlers/workspaces.test.ts)
(cd apps && pnpm --filter @kandev/web lint)
```

All focused tests and frontend lint passed before this work order was marked done.

## Files

- `apps/backend/internal/backendapp/boot_state_routes.go`
- `apps/backend/internal/backendapp/boot_state_workspace_idle_policy_test.go`
- `apps/backend/internal/task/service/service_events.go`
- `apps/backend/internal/task/service/service_workspace_idle_events_test.go`
- `apps/web/lib/routing/route-bootstrap.ts`
- `apps/web/lib/routing/route-bootstrap.team-access.test.ts`
- `apps/web/lib/types/backend.ts`
- `apps/web/lib/ws/handlers/workspaces.ts`
- `apps/web/lib/ws/handlers/workspaces.test.ts`
