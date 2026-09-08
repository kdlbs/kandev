---
id: "15-board-mutations"
title: "Revision-safe work-item mutations"
status: done
wave: 8
depends_on: ["14-board-discovery"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/azure-devops-integration.md"
---

# Task 15: Revision-Safe Work-Item Mutations

## Acceptance

- The board work-item PATCH route accepts only title, assignee, tags, target
  column, and split-column done state; the backend derives every Azure field
  path/value from the selected board and rejects empty updates or unknown
  columns.
- Azure requests use `application/json-patch+json`, prepend a `/rev` test, keep
  Azure rule/notification behavior enabled, and return the normalized updated
  card.
- Stale revisions return HTTP 409 with
  `azure_devops_revision_conflict`; permission failures preserve the existing
  403 behavior, and mock mutations match the production revision semantics.

## Verification

- `rtk make -C apps/backend test` (including `TestRESTClientUpdateBoardWorkItem`, `TestServiceUpdateBoardWorkItem`, `TestControllerUpdateBoardWorkItem`, and `TestMockClientUpdateBoardWorkItem`) from the repository root.

## Files Likely Touched

- `apps/backend/internal/azuredevops/client.go`
- `apps/backend/internal/azuredevops/client_models.go`
- `apps/backend/internal/azuredevops/rest_client.go`
- `apps/backend/internal/azuredevops/service_board.go`
- `apps/backend/internal/azuredevops/controller.go`
- `apps/backend/internal/azuredevops/mock_client.go`
- Corresponding `*_test.go` files.

## Dependencies

Task 14 board definitions and card placement.

## Parallelism

Sequential. It shares the backend contracts and mock state with Task 14 and
must land before frontend mutation wiring.

## Inputs

- Spec sections: What (card edits), API Surface, Permissions, Failure Modes,
  Scenarios.
- Existing patterns: bounded/redacted `doJSON`, controller error mapping, and
  optimistic revision values already present on `WorkItem`.
- Azure REST 7.1 Work Items Update JSON Patch contract.

## Risks

- Never accept arbitrary JSON Patch operations, field reference names,
  `bypassRules`, or `suppressNotifications` from the browser.
- Empty assignee and tag collections are intentional clears; omitted fields
  must remain unchanged.

## Output Contract

Report the allowlist and conflict contract, files changed, RED/GREEN commands,
permission behavior, blockers and risks, then set this task and its plan
checkbox to done.
