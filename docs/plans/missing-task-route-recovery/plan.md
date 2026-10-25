---
spec: docs/specs/tasks/requirements/missing-task-route-recovery.md
created: 2026-08-03
status: completed
---

# Implementation Plan: Missing task route recovery

## Overview

Make task-detail boot construction aware of whether route-specific data was
actually produced. When the requested task cannot be loaded, reuse the existing
home Kanban boot builder to hydrate only the selected authorized workspace and
its sidebar snapshots; keep successful task routes on their current lean,
task-owned hydration path. Then add a workspace-preserving overview action to
the unavailable state and prove the full cold-load behavior on desktop and
phone.

## Root cause

`bootInitialState` deliberately adds the full workspace/workflow/Kanban state
for Home and unknown routes, but not for `RouteTaskDetail`. A valid task route
receives that context inside `routeData.taskDetail.initialState`. When
`taskDetailRouteData` cannot load the requested task, it returns `nil`, leaving
the top-level state without `workspaces.activeId`, workflows, or snapshots.
The client retry then renders the unavailable state, while
`useWorkspaceSidebarTasks(null)` intentionally scopes the sidebar to an empty
list to avoid leaking stale workspace data.

## Backend

### Conditional task-detail fallback boot state

- In `apps/backend/internal/backendapp/helpers.go`, update `bootPayload` to
  construct `initialState` and `routeData` before assembling the final payload.
- When the classified route is `webapp.RouteTaskDetail` and route data is
  unavailable, call the existing
  `bootStateBuilder.addHomeKanbanRouteState(ctx, req, initialState)` path. This
  preserves URL/cookie/settings/default workspace precedence, Office-workflow
  exclusion, repository hydration, and all-workflow snapshots without adding a
  second data-loading implementation.
- Only run that fallback for auth-disabled instances or requests carrying an
  authenticated identity. Auth-enabled anonymous bootstrap requests must keep
  the existing auth-only payload and must never invoke unscoped workspace or
  task loaders.
- Leave successful task-detail boot unchanged: its top-level initial state
  remains lean and `routeData.taskDetail.initialState` remains authoritative for
  the task's actual workspace.
- Preserve the existing not-found/access-denied ambiguity and request identity
  when listing fallback workspaces and tasks. No unscoped service call or new
  endpoint is introduced.
- Route-data errors remain intentionally represented by the existing generic
  unavailable state; the client-side task fetch still retries and determines
  whether the task can be rendered after the shell loads.

## Frontend

### Unavailable task recovery action

- In `apps/web/components/task/task-page-content.tsx`, give
  `TaskLoadErrorState` the resolved `workspaces.activeId` and render an existing
  `Button`/routing `Link` to
  `linkToTaskOverview({ workspaceId: activeWorkspaceId ?? undefined })`.
- Keep the error copy generic and the main error surface unchanged. Add a stable
  test ID to the overview action for rendered regression coverage.
- Externalize the loading label, unavailable title/description, and new overview
  action through `react-i18next`; add matching English and pseudo-locale keys in
  `apps/web/src/locales/en/common.json` and
  `apps/web/src/locales/pseudo/common.json`.

### Mobile design contract

- **Desktop outcome:** the global task sidebar remains populated from the
  fallback workspace, and a valid row can replace the unavailable route.
- **Mobile entry point:** the unavailable-state task-overview action is the
  direct recovery path because `AppSidebar` is intentionally hidden below the
  desktop breakpoint.
- **Nearest shipped exemplar:** the task-overview back link in
  `apps/web/components/task/mobile/session-mobile-top-bar.tsx`; reuse its
  `linkToTaskOverview` destination and active-workspace context. The unavailable
  action uses a normal button/link rather than mounting the full task workbench
  or stacking a drawer onto an error state.
- **Hierarchy and presentation:** the error remains the single focal surface;
  the recovery action follows its explanation and navigates directly to Home.
  No overlay, new scroll owner, or persisted responsive state is introduced.
- **Geometry and input:** the error container remains viewport-contained, and
  the action has at least a 44px active height on phone while retaining semantic
  link and keyboard behavior.
- **Shared logic:** both viewports use the same resolved workspace ID and link
  builder. Only responsive sizing differs.

## Tests

- **Missing task fallback boot:** add a focused backend test in
  `apps/backend/internal/backendapp/helpers_test.go` that creates a visible
  sibling task, cold-builds `/t/<missing-id>` with an active-workspace cookie,
  and asserts that top-level boot state contains the selected workspace,
  workflows, and a snapshot containing the sibling while route-specific task
  data remains absent.
- **Anonymous bootstrap guard:** assert that an auth-enabled request without a
  request identity keeps only auth/features state and does not load workspace,
  workflow, repository, or Kanban snapshots.
- **Successful task boot remains lean:** extend the backend coverage to assert
  that a valid task route still provides `routeData.taskDetail` and does not
  add fallback workspace/Kanban data to the top-level initial state.
- **Client route failure:** extend
  `apps/web/src/task-detail-route.test.tsx` so a rejected task-data request
  reaches the null-task shell without a state hydrator; this preserves the
  boundary between route failure and the unavailable surface.
- The new overview action is component markup with no new branching helper; its
  destination, accessibility, and responsive behavior are covered by the
  desktop and mobile Playwright scenarios below.

## E2E Tests

- **Desktop scenario:** extend
  `apps/web/e2e/tests/task/task-loading-state.spec.ts`. Seed a sibling task,
  cold-load a missing `/t/:id`, assert `task-load-error-state`, assert the global
  sidebar shows the sibling and not **No tasks yet**, then select the sibling
  and verify its valid task route opens.
- **Mobile scenario:** extend
  `apps/web/e2e/tests/task/mobile-task-loading-state.spec.ts`. Cold-load a
  missing task route, assert the unavailable state and overview action fit the
  Pixel 5 viewport with a touch-sized target and no document horizontal
  overflow, tap the action, and verify the seeded task appears on mobile Home.
- Both scenarios use the production Go-served Vite build through the managed
  E2E runner so the route-aware boot payload—not only client navigation—is
  exercised.

## Verification Results

- Backend contracts: `cd apps/backend && go test ./internal/backendapp -run '^(TestBootPayloadAnonymousMissingTaskDoesNotLoadHomeState|TestBootPayloadMissingTaskFallsBackToHomeKanbanState|TestBootPayloadValidTaskKeepsRouteSpecificState)$' -count=1` — 3 passed.
- Anonymous regression: `cd apps/backend && go test ./internal/backendapp -run '^TestBootPayloadAnonymousMissingTaskDoesNotLoadHomeState$' -count=1` — 1 passed.
- Client route boundary: `cd apps && pnpm --filter @kandev/web exec vitest run src/task-detail-route.test.tsx` — 3 passed.
- Recovery-link component contract: `cd apps && pnpm --filter @kandev/web exec vitest run components/task/task-page-content.test.tsx` — 2 passed.
- Frontend: `cd apps/web && pnpm run typecheck` — passed; `pnpm run i18n:check && pnpm run i18n:ratchet` — passed.
- Desktop E2E: `cd apps/web && pnpm e2e:run tests/task/task-loading-state.spec.ts -- --grep 'keeps sibling tasks available'` — 1 passed.
- Mobile E2E: `cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/task/mobile-task-loading-state.spec.ts -- --grep 'returns to task overview'` — 1 passed.
- The managed runner rebuilt the production Go-served Vite bundle and cleaned temporary reports.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [Task 01: Restore fallback boot context](task-01-fallback-boot-context.md)

Wave 2 (depends on Task 01):

- [x] [Task 02: Add unavailable-route recovery UI and E2E](task-02-recovery-ui-and-e2e.md)

Both tasks are sequential. The rendered regression depends on the backend boot
payload change, and no subagent execution is authorized by this plan.

## Risks and out of scope

- The fallback intentionally loads the same snapshots as Home only after task
  route-data failure; accidentally running it for successful routes would add
  duplicate work and could briefly select the wrong workspace.
- Authorization must remain request-scoped. Tests should assert selected
  authorized state, not weaken task `NotFound` behavior to distinguish missing
  from inaccessible IDs.
- Office task routes, task API semantics, sidebar presentation, and retry policy
  remain out of scope.
