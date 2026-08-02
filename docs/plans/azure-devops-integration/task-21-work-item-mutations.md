---
id: "21-work-item-mutations"
title: "Constrained work-item mutations"
status: done
wave: 11
depends_on: ["20-work-item-detail"]
plan: "plan.md"
spec: "../../specs/azure-devops-integration/spec.md"
---

# Task 21: Constrained Work-Item Mutations

## Acceptance

- Board update requests accept only revision, column/split state, and
  `assign_current_user`/`unassign`; title, tags, descriptions, effort, and
  browser-supplied identities are rejected.
- Assign to me resolves the current PAT identity server-side and every mutation
  keeps the `/rev` optimistic concurrency test plus board-derived field paths.
- Conflict and permission failures preserve readable board/detail data and
  return codes the frontend can distinguish.

## Verification

- `go test ./internal/azuredevops -run 'TestBoardWorkItemPatch(AssignCurrentUser|Unassign|RejectsUnsupportedFields)|TestServiceUpdateBoardWorkItemIdentity'` from `apps/backend`.

## Files Likely Touched

- `apps/backend/internal/azuredevops/board_models.go`
- `apps/backend/internal/azuredevops/client.go`
- `apps/backend/internal/azuredevops/rest_client.go`
- `apps/backend/internal/azuredevops/rest_client_test.go`
- `apps/backend/internal/azuredevops/service_board.go`
- `apps/backend/internal/azuredevops/service_board_test.go`
- `apps/backend/internal/azuredevops/mock_client.go`
- `apps/backend/internal/azuredevops/controller.go`
- `apps/web/lib/types/azure-devops.ts`
- `apps/web/lib/api/domains/azure-devops-api.ts`

## Dependencies

Task 20.

## Parallelism

Sequential. It narrows the existing public mutation contract.

## Inputs

- Spec: constrained update request, permissions, conflicts, assignment scenarios.
- Existing board field resolution and revision-safe JSON Patch tests.

## Risks

- Keep move behavior compatible with split columns while removing obsolete
  title/tag mutation paths from API types, mocks, and tests together.

## Output Contract

Report rejected/allowed fields, identity resolution, RED/GREEN commands, files
changed, compatibility risks, and update task/plan status.
