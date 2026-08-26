---
id: "14-board-discovery"
title: "Board discovery and snapshots"
status: done
wave: 8
depends_on: []
plan: "plan.md"
spec: "../../specs/integrations/requirements/azure-devops-integration.md"
---

# Task 14: Board Discovery And Snapshots

## Acceptance

- Workspace-scoped routes list project teams and their visible boards/backlog
  levels, then return the selected board's columns and hydrated cards.
- Board snapshots preserve Azure backlog order, derive column and split-done
  placement from the board's returned field references, batch hydration at 200
  IDs, and omit deleted references without failing the remaining board.
- The Azure mock client can seed teams, boards, columns, and card placement for
  deterministic service and browser tests.

## Verification

- `rtk make -C apps/backend test` (including `TestRESTClientBoardReads`, `TestServiceBoardReads`, `TestControllerBoardReads`, and `TestMockClientBoard`) from the repository root.

## Files Likely Touched

- `apps/backend/internal/azuredevops/client.go`
- `apps/backend/internal/azuredevops/client_models.go`
- `apps/backend/internal/azuredevops/rest_client.go`
- `apps/backend/internal/azuredevops/service_board.go`
- `apps/backend/internal/azuredevops/controller.go`
- `apps/backend/internal/azuredevops/mock_client.go`
- `apps/backend/internal/azuredevops/mock_controller.go`
- Corresponding `*_test.go` files.

## Dependencies

None. Extend the shipped Azure REST/config foundation without changing its
workspace credential boundary.

## Parallelism

Sequential. Task 15 extends the same client, controller, and mock contracts.

## Inputs

- Spec sections: What (Board mode reads), API Surface, Failure Modes, Scenarios.
- Existing patterns: `QueryWIQL`, `hydrateWorkItems`, and the Azure mock
  controller.
- Azure REST 7.1 team, backlog, board, and backlog-level work-item contracts.

## Output Contract

Report route/DTO contracts, files changed, RED/GREEN commands, batching and
deleted-reference behavior, blockers and risks, then set this task and its plan
checkbox to done.
