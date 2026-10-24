---
status: draft
system: office
requirements:
  - REQ-OFFICE-WORKSPACE-OVERVIEW-001
created: 2026-09-29
owners:
  - kandev
---

# Office Workspace Overview System Design

## Purpose and boundaries

This design defines the read-only overview for several Office workspaces.
The Office system owns the aggregate view. The task service owns workspace visibility. The task and Office repositories own their existing data.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-WORKSPACE-OVERVIEW-001` | Components, control flow, and security |

## Components and responsibilities

- `apps/backend/internal/backendapp/office_scope.go` marks the aggregate route as a user-scoped route. It rejects agent callers and checks for a real user identity when authentication is enabled.
- `apps/backend/internal/office/dashboard/aggregate.go` gets workspaces from the task service. It filters for Office workspaces, sorts them by name, and builds the response.
- `apps/backend/internal/office/repository/sqlite/aggregate.go` reads task counts, approval counts, and recent activity in batches. It reads existing tables and adds no schema.
- `apps/backend/internal/office/dashboard/service.go` receives the identity-scoped workspace lister from the backend composition root.
- `apps/web/src/office-routes.tsx` maps `/office/overview` to the overview page. The Office shell provides the page title and navigation.
- `apps/web/hooks/domains/office/use-workspace-aggregate.ts` fetches the page data and refreshes it every 30 seconds while the page is mounted.
- `apps/web/lib/state/slices/office/office-slice.ts` owns the in-memory aggregate. The hook owns the local loading state and ignores stale responses.
- `apps/web/app/office/overview/workspace-aggregate-page-client.tsx` shows workspace cards and the merged activity feed. The shared activity row adds `workspaceId` to run links.

## Data and contracts

The frontend sends `GET /api/v1/office/workspaces/aggregate`. The response contains `workspaces` and `recent_activity`.

Each workspace entry contains its ID and name, task counts, pending approvals, agent count, and running-agent count. The activity list contains at most 20 entries across all returned workspaces.

The workspace rows come from the task service's `ListWorkspaces` call. The dashboard service keeps only rows with an Office workflow ID. The repository groups task and approval counts by workspace ID. It also selects the latest activity rows across the supplied IDs.

The overview state is not stored as a separate record. The database remains the source of task, approval, agent, and activity data. The frontend store keeps only the latest response in memory.

## Control flow

1. The user opens `/office/overview` from Office navigation.
2. The page hook calls the overview API with `cache: "no-store"`.
3. The backend route guard rejects agent tokens. When authentication is enabled, it also rejects missing or synthetic user identities.
4. The dashboard service calls the task service for the current caller's workspace list and keeps Office workspaces.
5. The service reads task counts, pending approvals, recent activity, and agent counts. It enriches activity labels and returns the aggregate.
6. The hook normalizes activity entries and writes the response to the Office store.
7. The page shows one card per workspace and one merged activity list. Workspace cards select a workspace through `workspaceId`.
8. The hook refreshes the response every 30 seconds while mounted.

## Failure and recovery

If the workspace lister is not configured, the endpoint returns HTTP 503. If a repository read fails, the endpoint returns HTTP 500.

The page shows an error state when its first request fails. A periodic refresh retries the request. If a later request fails, the page keeps the last loaded response.

The hook uses a request generation to ignore responses after unmount. One in-flight request serves concurrent refresh calls.

## Persistence

The endpoint performs read-only queries. It adds no tables and writes no records. The browser keeps the response only in the in-memory Office store.

## Security

The aggregate route has no workspace ID in its path. The route guard therefore rejects agent callers before the authentication-disabled bypass. When authentication is enabled, the guard requires a real, non-synthetic user identity.

The dashboard service takes its workspace list from the task service. It does not build a separate user-to-workspace rule. The endpoint returns only Office workspaces from that list.

## Observability

The endpoint uses the existing Office HTTP logging and error reporting. It adds no metric or log identity labels.
