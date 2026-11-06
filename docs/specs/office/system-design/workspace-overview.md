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

This design defines the read-only aggregate over several Office workspaces.
The Office system owns the aggregate read. The task service owns workspace
visibility. The task and Office repositories own their existing data. The
integrated control dashboard is the only browser consumer (see
`docs/specs/ui/system-design/integrated-control-navigation.md`).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-WORKSPACE-OVERVIEW-001` | Components, data and contracts, control flow, security |

## Components and responsibilities

- `apps/backend/internal/backendapp/office_scope.go` lists the aggregate route in `officeOwnerAggregateRoutes`. The scope guard rejects agent callers and, when authentication is enabled, requires a real user identity. The route-completeness test classifies it as an owner aggregate, not as a workspace-less route.
- `apps/backend/internal/office/dashboard/aggregate.go` gets workspaces from the task service, keeps Office workspaces, sorts them by name, and builds the response.
- `apps/backend/internal/office/repository/sqlite/aggregate.go` reads task counts, approval counts, and recent activity in batches of at most 400 workspace ids. It reads existing tables and adds no schema. Task state bucketing is shared with the per-workspace dashboard.
- `apps/backend/internal/office/dashboard/service.go` receives the identity-scoped workspace lister from the backend composition root (`backendapp/main.go`).
- `apps/web/lib/api/domains/office-extended-api.ts` exposes `getWorkspaceAggregate` and the wire types.
- `apps/web/app/office/workspace/activity/activity-row.tsx` adds `workspaceId` to run links so a cross-workspace activity row opens the run in its own workspace.

## Data and contracts

The client sends `GET /api/v1/office/workspaces/aggregate`. The response is
`{ workspaces: WorkspaceAggregateEntry[], recent_activity: ActivityEntry[] }`.

Each workspace entry contains `workspace_id`, `name`, `task_count`,
`open_tasks`, `in_progress_tasks`, `blocked_tasks`, `done_tasks`,
`pending_approvals`, `agent_count`, and `running_agents`. Task counts exclude
ephemeral, automation-origin, and archived tasks, matching the per-workspace
dashboard. `running_agents` counts agent instances in the `working` status.

The activity list contains at most 20 entries across all returned workspaces,
ordered by `created_at` then `id`, newest first. Labels are enriched per
workspace with the existing activity label resolver.

## Control flow

1. The integrated control provider calls the endpoint with `cache: "no-store"`.
2. The scope guard rejects agent tokens, then (auth enabled) missing or synthetic identities.
3. The dashboard service lists the caller's workspaces and keeps those with an Office workflow id.
4. The service reads task breakdowns, pending approvals, recent activity, and agent counts, enriches activity labels, and returns the aggregate.

## Failure and recovery

No workspace lister: HTTP 503. Any lister, repository, or agent read failure:
HTTP 500. The endpoint never returns a partial aggregate; the consumer keeps
its last good snapshot and reports the failure.

## Persistence

Read-only queries. No tables, no writes.

## Security

The route has no workspace id in its path, so the resolver tables cannot scope
it. The guard therefore runs before the authentication-disabled bypass and
rejects agent callers in every mode. The workspace set comes only from the
task service's identity-scoped `ListWorkspaces`; the endpoint builds no separate
user-to-workspace rule.

## Observability

The endpoint uses the existing Office HTTP logging and error reporting. It adds
no metric or log identity labels.
