---
spec: docs/specs/plugins/spec.md
adr: docs/decisions/0043-plugin-host-data-api.md
created: 2026-07-17
status: draft
---

# Implementation Plan: Plugin Host data API

## Overview

Give plugins a typed, capability-gated path to read (phase 1) kandev domain data
over the existing `kandev.plugin.v1` Host gRPC channel, replacing direct SQLite
access. Land the contract first (proto + regen) and the one net-new aggregation
(per-session LOC), then the SDK's Go-native accessors, then kandev's server-side
implementation, and finally the proof (rewrite the agent-stats plugin) plus tests
and docs. Write RPCs (`CreateTask`/`UpdateTask`/`CreateComment`) are specified in
the proto but deferred; handlers return `Unimplemented` this phase.

The draft contract at `docs/plans/plugins/HOST-DATA-API.proto` is largely settled
— refine field-level details against the real DTOs (`apps/backend/pkg/api/v1/`),
do not redesign.

---

## Backend

### Proto contract (`apps/backend/proto/kandev/plugin/v1/plugin.proto`)
Fold the read + (deferred) write RPCs and DTOs from
`docs/plans/plugins/HOST-DATA-API.proto` into the frozen plugin proto. Decide
placement: add the RPCs to the existing `service Host` (simplest — reuses the
single Host broker connection and the existing capability interceptor) rather
than a separate `service HostData`, unless a separate service is needed to keep
the interceptor's per-RPC capability lookup clean. Regenerate stubs with
`make -C apps/backend proto`. Read RPCs: `ListTasks`, `GetTask`, `ListWorkspaces`,
`ListWorkflows`, `ListWorkflowSteps`, `ListAgentProfiles`, `ListRepositories`,
`ListSessions`, `ListSessionCodeStats`. DTOs mirror
`apps/backend/pkg/api/v1/{task,workflow,agent,workspace}.go` field-for-field with
RFC3339 string timestamps and `optional` nullables.

### Per-session LOC aggregation (`internal/analytics/repository/sqlite/`)
No existing method computes per-session committed + peak-pending LOC.
`internal/analytics/repository/sqlite/stats.go` aggregates git stats at workspace
and per-repository granularity only. Add a read-only aggregation returning, per
session id: `SUM(insertions)` / `SUM(deletions)` from `task_session_commits`, and
the peak pending-diff (`MAX` over each snapshot's `SUM(json_each(files).additions
/ .deletions)`) from `task_session_git_snapshots`. This mirrors the SQL the
agent-stats plugin runs today (`stats.go:sessionsQuery`). Use the `dialect`
helpers for SQLite/Postgres portability; filter by session-id set / workspace and
support pagination bounds. Expose it through the analytics (or task) service so
the Host handler calls a service method, not the repository.

### Host data RPC implementation (`internal/plugins/`)
Extend `pluginHost` (or add a sibling under `internal/plugins/`) to implement the
new Host data interface methods. Each read handler:
1. checks `manifest.CanRead("<resource>")` — reuse the existing per-plugin
   binding and `permissionDenied("api_read:<resource>")` helper shape;
2. calls the relevant service method (`taskSvc` for tasks/workspaces/repositories/
   sessions, workflow service for workflows/steps, agent settings for profiles,
   the new aggregation for code stats) — never a repository directly;
3. maps internal models → the Go-native DTOs with explicit code.
Wire the new service dependencies into `plugins.Service` /
`plugins.NewService` / `Provide`, and thread them into `hostForPlugin`. Write
handlers return `codes.Unimplemented`.

---

## Frontend

None. This feature has no SPA surface; it is a plugin-facing gRPC contract.

---

## SDK (`apps/backend/pkg/pluginsdk/`)

Extend the Go-native Host surface authors and kandev share:
- Add Go-native DTO structs (`Task`, `Session`, `SessionCodeStats`, `Workspace`,
  `Workflow`, `WorkflowStep`, `AgentProfile`, `Repository`, page types) mirroring
  the proto, with `toProto`/`fromProto` helpers alongside the existing ones in
  `types.go`.
- Add data methods to the `Host` interface (or a `HostData` sub-interface exposed
  via `host.Tasks()`, `host.Sessions()`, ...), with `grpcHostClient` (plugin-side)
  and `grpcHostServer` (kandev-side) conversions in `host.go`.
- Author-facing ergonomics: typed accessors returning Go-native structs, e.g.
  `host.Tasks().List(ctx, filter, page)`, `host.Sessions().List(...)`,
  `host.Sessions().CodeStats(...)`.

---

## Tests

- **DTO mapping (`internal/plugins/*_test.go`):** table-driven — internal model →
  DTO for each resource, including nullable/optional and timestamp formatting.
- **Capability gating (`internal/plugins/host_test.go`):** each read RPC returns
  `PermissionDenied` with `capability 'api_read:<resource>' not declared` when the
  manifest omits the resource, and succeeds when it declares it. Write RPC returns
  `Unimplemented`.
- **Per-session LOC aggregation (`internal/analytics/repository/sqlite/*_test.go`):**
  real SQLite fixture with commits + snapshots; assert committed sums and the
  peak-pending `MAX` semantics (peak, not latest; not double-counted).
- **SDK conversions (`pkg/pluginsdk/*_test.go`):** proto↔native round-trip for the
  new DTOs.
- **Integration:** a plugin (via the SDK Host client over the broker) calls
  `ListSessions` + `ListSessionCodeStats` and gets rows without DB access — extend
  the existing `internal/plugins/host_test.go` / integration harness.

---

## E2E Tests

None. No user-visible UI surface. The integration test above covers the plugin↔host
path.

---

## Implementation Waves

```
Wave 1 (parallel):
- [ ] [task-01-proto-contract](task-01-proto-contract.md)
- [ ] [task-02-session-loc-aggregation](task-02-session-loc-aggregation.md)

Wave 2:
- [ ] [task-03-sdk-data-accessors](task-03-sdk-data-accessors.md)

Wave 3:
- [ ] [task-04-host-data-impl](task-04-host-data-impl.md)

Wave 4 (parallel):
- [ ] [task-05-rewrite-agent-stats-plugin](task-05-rewrite-agent-stats-plugin.md)
- [ ] [task-06-tests](task-06-tests.md)
- [x] [task-07-docs](task-07-docs.md)
```

Rationale: task-01 (contract) and task-02 (aggregation) touch disjoint packages
and can run in parallel. The SDK (task-03) needs generated stubs from task-01. The
kandev impl (task-04) needs the stubs (task-01), the aggregation (task-02), and
the Go-native interface/DTOs (task-03). Wave 4 tasks touch disjoint trees (a
separate plugin repo, backend tests, public docs) and parallelize.

---

## Open Questions

- **Proto placement:** RPCs on the existing `service Host` vs a new
  `service HostData` served on the same broker connection. Recommendation: extend
  `service Host` to reuse the existing capability interceptor and single broker
  wiring; revisit only if the interceptor's per-RPC capability mapping gets
  unwieldy. Settle in task-01.
- **Aggregation home:** `internal/analytics` (where git stats already live) vs a
  new method on the task repository's `git_snapshots.go`. Recommendation:
  analytics repository, consistent with `GetGitStats`/`GetRepositoryStats`.
  Settle in task-02.
</content>
