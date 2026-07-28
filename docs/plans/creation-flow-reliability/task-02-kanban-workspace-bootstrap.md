---
id: "02-kanban-workspace-bootstrap"
title: "Kanban workspace workflow bootstrap"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/workspaces/creation.md"
---

# Task 02: Kanban workspace workflow bootstrap

## Acceptance

- Standard HTTP and WebSocket workspace creation produce one visible `Kanban` workflow from the
  built-in `simple` template with usable steps.
- Standard creation persists the workspace, Kanban workflow, and template-derived steps in one
  transaction; cancellation or any persistence failure leaves none of those rows visible.
- A workflow bootstrap failure makes the standard creation request fail.
- Office onboarding's direct workspace creation path does not implicitly add a Kanban workflow.

## Verification

- `cd apps/backend && go test -run 'Test.*CreateWorkspace.*Kanban|Test.*Office.*Workspace' ./internal/task/handlers ./internal/backendapp`
- Run any directly affected backend integration package tests identified while updating fixtures.

## Files likely touched

- `apps/backend/internal/task/service/service_requests.go`
- `apps/backend/internal/task/service/service_resources.go`
- `apps/backend/internal/task/handlers/workspace_handlers.go`
- Focused `*_test.go` files beside the changed handler/service or in backend integration tests.

## Dependencies

None.

## Inputs

- `docs/specs/workspaces/creation.md`.
- `docs/plans/creation-flow-reliability/plan.md` backend section.
- Built-in template `apps/backend/config/workflows/kanban.yml`.
- Office adapter `apps/backend/internal/backendapp/adapters_office.go`.

## Output contract

Return a compact handoff capsule with intent/acceptance, base/head SHA, changed files and entry
points, risk tags, exact commands/results, uncertainties, and this task status set to `done`.

## Completion

- Standard HTTP and WebSocket creation opt into the built-in `simple` Kanban workflow bootstrap.
- Bootstrap is strict and atomic: workflow or template-step creation errors, including request
  cancellation, roll back the workspace, workflow, and any partial steps together. No compensating
  cleanup path is used because no bootstrap row becomes visible before commit.
- Successful creation publishes the workspace event before the child workflow event.
- Office's direct service adapter remains opt-out and is covered by regression tests.
