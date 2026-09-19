---
id: "01-fallback-boot-context"
title: "Restore fallback boot context"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/missing-task-route-recovery.md"
---

# Task 01: Restore fallback boot context

Use the existing Home Kanban boot builder only when a task-detail route cannot
produce task-specific route data, so the app shell retains its authorized
workspace and sidebar snapshots without changing successful task boot.

## Acceptance

- A missing or inaccessible `/t/:id` boot payload contains the selected
  authorized Kanban workspace, workflows, repositories, and workflow snapshots,
  including visible sibling tasks.
- The invalid task ID is not synthesized into boot state and
  `routeData.taskDetail` remains absent.
- A valid task route still uses only its route-specific task initial state and
  does not receive duplicate fallback Home state.
- An auth-enabled anonymous request keeps its auth-only bootstrap and does not
  invoke the fallback loaders.

## Files likely touched

- `apps/backend/internal/backendapp/helpers.go`
- `apps/backend/internal/backendapp/helpers_test.go`

## Dependencies

None.

## Parallelism

`sequential`. Task 02's production-build E2E assertions depend on this boot
payload behavior.

## Inputs

- Spec sections **Desired behavior**, **Regression scenarios**, and
  **Constraints**.
- `apps/backend/internal/backendapp/boot_state.go`:
  `taskDetailRouteData` and `addHomeKanbanRouteState`.
- `apps/backend/internal/backendapp/helpers.go`: `bootPayload`.
- ADR 0021 (Go-served SPA boot state) and ADR 0023 (active-workspace cookie
  precedence).

## TDD sequence

1. Add a failing `helpers_test.go` regression for a missing task route with a
   real sibling task and selected workspace.
2. Add the valid-route non-duplication and anonymous-bootstrap assertions.
3. Implement the minimal route-data-aware fallback in `bootPayload`.
4. Refactor only if needed to keep `bootPayload` small and explicit.

## Verification

```bash
cd apps/backend && go test ./internal/backendapp -run '^(TestBootPayloadAnonymousMissingTaskDoesNotLoadHomeState|TestBootPayloadMissingTaskFallsBackToHomeKanbanState|TestBootPayloadValidTaskKeepsRouteSpecificState)$' -count=1
```

## Risks

- Running the fallback for successful task routes would duplicate boot work and
  could expose a briefly incorrect active workspace before task hydration.
- Fallback workspace listing must keep the request identity; bypassing the
  scoped task service would leak cross-user workspace metadata.
- Auth-enabled anonymous requests must not reach any fallback loader because
  identity-free task-service calls are intentionally unscoped internal calls.

## Output contract

Report the RED failure, GREEN result, exact files changed, authorization and
workspace-precedence evidence, remaining risks, and synchronized task/plan
status. Record every exact command and outcome in `## Results`.

## Results

Implemented the conditional fallback in
`apps/backend/internal/backendapp/helpers.go` and added focused coverage in
`apps/backend/internal/backendapp/helpers_test.go`.

- RED: `cd apps/backend && go test ./internal/backendapp -run 'TestBootPayload(MissingTaskFallsBackToHomeKanbanState|ValidTaskKeepsRouteSpecificState)$' -count=1` — 1 passed, 1 failed; the missing-task assertion reported absent fallback `workspaces` state.
- GREEN: the same command after the production change — 2 passed.
- Regression check: `cd apps/backend && go test ./internal/backendapp -run '^TestBoot(RouteDataTaskDetailIncludesTaskPageData|Payload(MissingTaskFallsBackToHomeKanbanState|ValidTaskKeepsRouteSpecificState))$' -count=1` — 3 passed.

The fallback reuses the existing request-scoped Home Kanban builder, so the
active-workspace cookie/query/settings/default precedence and authorization
boundary remain unchanged. Successful task routes retain their lean
route-specific initial state.

- Security regression: `cd apps/backend && go test ./internal/backendapp -run '^TestBootPayloadAnonymousMissingTaskDoesNotLoadHomeState$' -count=1` — 1 passed after the initial RED failure; the auth-enabled anonymous payload contains no workspace, workflow, repository, or Kanban state.
