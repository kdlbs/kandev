---
id: "04-boot-and-web-data-layer"
title: "Boot payload and web data layer for sets"
status: done
wave: 4
depends_on: ["03-http-ws-surface"]
plan: "plan.md"
spec: "../../specs/workspaces/requirements/repository-sets.md"
---

# Task 04: Boot Payload And Web Data Layer For Sets

## Acceptance

- The boot payload carries a workspace-keyed `repositorySets` collection
  (`itemsByWorkspaceId` / `loadingByWorkspaceId` / `loadedByWorkspaceId`) on the task-page, route
  context, and home-kanban branches, present as an empty shape for a workspace with no sets and on a
  listing failure. A boot test asserts the hydrated shape, not just the status code.
- The web store exposes a `repositorySets` slice with set/upsert/remove/invalidate actions, a
  `useRepositorySets(workspaceId)` hook that reads the store and lazily fetches, a typed API client
  for all four operations, and a WS handler so `repository_set.created|updated|deleted` update the
  slice without a reload.
- Unit tests cover the slice reducers, the API client request shapes, the WS handler, and the hook's
  hydrated-versus-fetch paths.

## Verification

```bash
cd apps/backend && go test ./internal/backendapp/...
cd apps/web && pnpm run typecheck
cd apps/web && pnpm vitest run lib/state/slices/workspace lib/api/domains/workspace-api.test.ts lib/ws/handlers hooks/domains/workspace
```

## Files likely touched

- `apps/backend/internal/backendapp/boot_state_routes.go` (`repositoriesForState:202`,
  `routeContextBootData:143`, `tasksPageBootData:77`, `routeData` maps `:80-89`, `:145-150`)
- `apps/backend/internal/backendapp/boot_state.go` (`addRepositoriesState:354`,
  `addHomeKanbanRouteState:319`)
- `apps/backend/internal/backendapp/boot_state_routes_test.go`
- `apps/backend/internal/backendapp/e2e_reset.go` (workspace-scoped delete list)
- `apps/web/lib/types/http.ts` (beside `Repository` `:245`)
- `apps/web/lib/api/domains/workspace-api.ts` (beside `listRepositories` `:28`)
- `apps/web/lib/state/slices/workspace/types.ts` (`:20-24`), `workspace-slice.ts`, `selectors.ts`
- `apps/web/src/boot-payload.ts` (`BootRouteData` `:38-56`)
- `apps/web/hooks/domains/workspace/use-repository-sets.ts` (new, modelled on
  `use-repositories.ts:28`)
- `apps/web/lib/ws/handlers/repository-sets.ts` (new) and `apps/web/lib/ws/router.ts`
  (`registerWsHandlers:39`)

## Dependencies

Task 03 for the REST contract the client mirrors.

## Inputs

- Spec: API surface, Live updates, Failure modes (boot failure is non-fatal).
- Patterns: `repositoriesForState` for the boot shape; `use-repositories.ts` for the hook;
  `lib/ws/handlers/workspaces.ts` for handler registration.
- The boot mappers are explicit camelCase whitelists (`boot_state_routes.go:631-634`): a DTO field
  not listed is silently absent from hydration.

## Risks

- Omitting the collection from an empty-workspace early return makes the key absent rather than
  empty, and the hook then treats a hydrated workspace as unloaded and refetches on every open.
- Repositories have no WS events today, so there is no existing handler to copy exactly; the new
  handler must resolve the workspace id from the payload the service publishes.

## Output contract

Summary, files changed, tests run with results, blockers, risks, divergence from the plan, and
task/plan status updates.
