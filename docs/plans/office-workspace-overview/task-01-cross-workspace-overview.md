---
id: "01-cross-workspace-overview"
title: "Add the read-only Office workspace overview"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-WORKSPACE-OVERVIEW-001
acceptance_criteria:
  - AC-OFFICE-WORKSPACE-OVERVIEW-001.1
  - AC-OFFICE-WORKSPACE-OVERVIEW-001.2
  - AC-OFFICE-WORKSPACE-OVERVIEW-001.3
  - AC-OFFICE-WORKSPACE-OVERVIEW-001.4
  - AC-OFFICE-WORKSPACE-OVERVIEW-001.5
  - AC-OFFICE-WORKSPACE-OVERVIEW-001.6
  - AC-OFFICE-WORKSPACE-OVERVIEW-001.7
system_design:
  - ../../specs/office/system-design/workspace-overview.md
---

# Task 01: Add the read-only Office workspace overview

## Acceptance

- The Office route `/office/overview` shows Office workspaces from the caller's workspace list.
- The backend denies agent tokens in all authentication modes. When authentication is enabled, it requires a real, non-synthetic user identity.
- Each workspace row shows task counts, pending approvals, and agent counts. The activity feed shows at most 20 rows across the returned workspaces.
- A workspace card opens the selected workspace. An activity run link keeps its workspace ID.
- The page shows loading, error, and empty states. It refreshes every 30 seconds while mounted and keeps loaded data when a refresh fails.
- The page uses the existing database records and adds no schema or write path.

## UI preview

See [UI-01 in the plan](plan.md#ui-preview). The overview uses the same stacked card and activity-row order on desktop and phone.

## Verification

- `cd apps/backend && go test -race -count=20 -run '^TestCodexAppServerProbeClassifiesTrustedManagedRuntimeETarget$' ./internal/agentctl/server/utility`
- `cd apps/backend && go test -race -count=3 ./internal/agentctl/server/utility`
- `cd apps/backend && go test ./internal/office/dashboard ./internal/office/repository/sqlite ./internal/backendapp`
- `cd apps/web && pnpm test -- lib/state/slices/office/workspace-aggregate-store.test.ts hooks/domains/office/use-workspace-aggregate.test.tsx app/office/workspace/activity/activity-row.test.tsx`
- `cd apps/web && pnpm e2e:run --project chromium tests/office/workspace-aggregate-navigation.spec.ts`
- `cd apps/web && pnpm e2e:run --project mobile-chrome tests/office/mobile-workspace-aggregate-navigation.spec.ts`
- `python3 scripts/lint-spec-files.py --all`
- `python3 scripts/list-docs.py validate`

## Implementation result

The aggregate API, route guard, batched repository queries, Office store, page hook, navigation, and desktop and phone browser tests are present in the PR.

The initial repeat test exposed a race in Codex probe error classification. Probe cleanup now waits for stderr capture before it reads the failure output. Twenty race-enabled repetitions pass after this fix.
