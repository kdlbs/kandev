---
id: "22-task-work-item-links"
title: "Task work-item associations"
status: completed
wave: 11
depends_on: ["20-work-item-detail"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/azure-devops-integration.md"
---

# Task 22: Task Work-Item Associations

## Acceptance

- A workspace-authorized service validates an Azure work item before persisting
  the unique task/workspace/project/item association and returns associations
  grouped for browse/detail lookup.
- Task deletion and workspace deletion remove associations; restart preserves
  them.
- The mock and frontend API/store shapes can immediately cache a newly created
  association without a page reload.

## Verification

- `make -C apps/backend test`.
- `pnpm --filter @kandev/web test -- --run hooks/domains/azure-devops/use-azure-devops-task-work-items.test.tsx` from `apps`.

## Files Likely Touched

- `apps/backend/internal/azuredevops/models.go`
- `apps/backend/internal/azuredevops/store.go`
- `apps/backend/internal/azuredevops/store_task_work_item.go`
- `apps/backend/internal/azuredevops/service_task_work_item.go`
- `apps/backend/internal/azuredevops/service_task_work_item_test.go`
- `apps/backend/internal/azuredevops/controller.go`
- `apps/backend/internal/azuredevops/lifecycle.go`
- `apps/web/lib/types/azure-devops.ts`
- `apps/web/lib/api/domains/azure-devops-api.ts`
- `apps/web/hooks/domains/azure-devops/use-azure-devops-task-work-items.ts`
- `apps/web/hooks/domains/azure-devops/use-azure-devops-task-work-items.test.tsx`

## Dependencies

Task 20.

## Parallelism

Sequential. It introduces persistence and lifecycle ownership.

## Inputs

- Spec: `azure_devops_task_work_items`, API, permissions, task creation scenario.
- Existing `store_task_pr.go`, `service_task_pr.go`, and task-PR frontend cache.

## Risks

- Revalidate workspace/project/item identity server-side; a browser-provided URL
  is a hint, not an authorization or canonical identity.

## Output Contract

Report schema/lifecycle behavior, RED/GREEN commands, files changed, migration
result, risks, and update task/plan status.
