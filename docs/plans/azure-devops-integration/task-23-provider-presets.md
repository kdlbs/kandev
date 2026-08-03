---
id: "23-provider-presets"
title: "Azure provider presets"
status: completed
wave: 11
depends_on: []
plan: "plan.md"
spec: "../../specs/azure-devops-integration/spec.md"
---

# Task 23: Azure Provider Presets

## Acceptance

- Workspace settings return current built-in work-item/PR default queries and
  quick actions when overrides are absent, and preserve explicit customized
  lists.
- PATCH distinguishes omitted fields from explicit `null`; reset removes one
  override so future built-in improvements apply.
- Browse exposes the resolved provider presets and quick-action Task menus.
  Task creation interpolates provider context and persists the work-item link
  in the existing workspace association cache.
- Azure settings follows GitHub's section order for analogous capabilities:
  connection, pull-request watches, work-item watches, quick actions, and
  default queries.
- Quick actions and default queries use the GitHub settings interaction model:
  pull requests first, work items second, tabbed editable rows, Reset,
  dirty-state highlighting, and the shared floating Save changes control.
  Quick actions expose icon, label, hint, and expandable prompt fields; query
  rows expose provider-native filters.
- The Azure browse scope bar consumes the resolved workspace query presets, so
  settings changes affect the actual query shortcuts rather than only the
  settings API response.

## Verification

- `go test ./internal/azuredevops -run 'Test(WorkspaceSettings|DefaultQueryPresets|ActionPresets)'` from `apps/backend`.
- `pnpm --filter @kandev/web test -- --run components/azure-devops/azure-devops-quick-actions.test.tsx components/azure-devops/azure-devops-task-launcher.test.tsx hooks/domains/azure-devops/use-azure-devops-task-work-items.test.tsx` from `apps`.
- `pnpm --filter @kandev/web test -- --run components/azure-devops/azure-devops-default-queries.test.tsx components/azure-devops/azure-devops-quick-actions.test.tsx components/azure-devops/azure-devops-settings.test.tsx components/azure-devops/azure-devops-presets.test.ts` from `apps`.
- Desktop and `mobile-chrome` Azure settings Playwright scenarios verify section
  order, Reset/Save behavior, query availability, touch reachability, and no
  document horizontal overflow.

## Files Likely Touched

- `apps/backend/internal/azuredevops/models.go`
- `apps/backend/internal/azuredevops/store.go`
- `apps/backend/internal/azuredevops/workspace_settings.go`
- `apps/backend/internal/azuredevops/workspace_settings_test.go`
- `apps/backend/internal/azuredevops/controller.go`
- `apps/backend/internal/azuredevops/controller_test.go`
- `apps/web/lib/types/azure-devops.ts`
- `apps/web/lib/api/domains/azure-devops-api.ts`
- `apps/web/hooks/domains/azure-devops/use-azure-devops-task-work-items.ts`
- `apps/web/components/azure-devops/azure-devops-settings.tsx`
- `apps/web/components/azure-devops/azure-devops-presets.ts`
- `apps/web/components/azure-devops/azure-devops-quick-actions.tsx`
- `apps/web/components/azure-devops/azure-devops-task-launcher.tsx`

## Dependencies

None.

## Parallelism

Sequential. Query/action preset families share one workspace settings patch.

## Inputs

- Spec: query/action preset data and reset scenarios.
- GitHub `workspace_settings_service.go`, `action_presets_service.go`, and
  frontend default-query/action preset resolvers.
- ADR 0030 workspace-scoped integration settings.

## Risks

- Preserve existing Azure saved views while adding patchable settings; do not
  make config replacement erase unrelated preset fields.

## Output Contract

Report default/override semantics, RED/GREEN commands, files changed, migration
behavior, risks, and update task/plan status.

## Results (2026-07-31)

- RED: default-preset coverage found only one built-in query per Azure item
  family; component tests found no default-query editor, no GitHub-style action
  tabs, and no shared settings save flow.
- GREEN: Azure returns four built-in queries per family, both settings editors
  pass their shared-save/reset tests, and custom query mappings pass focused
  coverage.
- Desktop Playwright saves a renamed work-item query, verifies it on the live
  Azure scope bar, changes the selected board, and verifies the selection after
  reload. Mobile Playwright verifies both editors, 44px Reset reachability, and
  no document overflow.
- `make fmt` and `make typecheck test lint` pass from the repository root.
