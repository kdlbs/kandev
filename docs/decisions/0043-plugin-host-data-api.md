# 0043 — Plugins read/write kandev data via capability-gated Host gRPC RPCs

- Status: accepted
- Date: 2026-07-17
- Area: backend, protocol
- Related: [docs/specs/plugins/spec.md](../specs/plugins/spec.md) ("Host data API"),
  [docs/plans/plugins/GRPC-CONTRACT.md](../plans/plugins/GRPC-CONTRACT.md),
  [docs/plans/plugins/host-data-api/plan.md](../plans/plugins/host-data-api/plan.md)

## Context

The plugin system (spec `docs/specs/plugins/spec.md`) gives plugins a
capability-gated `Host` gRPC service — `GetState`/`SetState`/`DeleteState`/
`ListState`/`RevealSecret`/`EmitEvent` — but **no sanctioned way to read or write
kandev's own domain data** (tasks, sessions, workspaces, workflows, agent
profiles, repositories, comments). The manifest already reserves
`capabilities.api_read` / `capabilities.api_write` (string lists) for this, and
`manifest.CanRead(resource)` / `CanWrite(resource)` already exist, but no Host
RPC consumes them.

A real plugin exposed the gap. `kandev-plugin-agent-stats`
(`/home/jcfs/.kandev/tasks/tokens-per-task-loc_9tk/kandev-plugin-agent-stats/`)
needs per-session lines-of-code plus each session's agent/model/`acp_session_id`
to attribute externally-measured token usage (from `tokscale`, keyed by
`acp_session_id`) to agents. With no data API, it **opens the kandev SQLite file
directly, read-only** (`~/.kandev/data/kandev.db`) and runs SQL against
`task_sessions`, `task_session_commits`, `task_session_git_snapshots`, and
`executors_running`.

Direct DB access is only possible because the backend spawns plugin subprocesses
unsandboxed on the same host, and the plugin can guess the default DB path. It is
wrong on every axis:

- **Schema coupling.** The plugin's SQL hard-codes column names, the
  `agent_profile_snapshot` JSON shape, and the `task_session_git_snapshots.files`
  JSON shape. Any internal schema change silently breaks it.
- **No events on write.** kandev's convention (`apps/backend/CLAUDE.md`: "any code
  path that mutates a task row must publish `task.*` events") cannot hold for a
  plugin writing SQL — it would bypass the event bus and break WS-driven UI.
- **Breaks on Postgres.** kandev supports a Postgres backend; a plugin that opens
  a SQLite file has no data there at all.
- **No auth or scoping.** The plugin reads every row in the database regardless of
  what it declared or should see.

We want a "carrot" that makes the sanctioned path strictly better than touching
the file, so we can then pursue the "stick" (env-scrubbing, not shipping the DB
path into the subprocess, sandboxing) without stranding a legitimate use case.

## Decision

Extend the existing `kandev.plugin.v1` `Host` service with typed data RPCs.
Plugins read (phase 1) and later write (phase 2) kandev data over the same
capability-gated Host gRPC channel they already use for state and secrets. They
never open the database file and never receive internal domain structs over the
wire.

Concretely:

1. **New RPCs on the Host surface.** Add read RPCs now — `ListTasks`, `GetTask`,
   `ListWorkspaces`, `ListWorkflows`, `ListWorkflowSteps`, `ListAgentProfiles`,
   `ListRepositories`, `ListSessions`, `ListSessionCodeStats` — and defer write
   RPCs (`CreateTask`, `UpdateTask`, `CreateComment`) to phase 2. The frozen
   draft lives at `docs/plans/plugins/HOST-DATA-API.proto`; it is folded into
   `apps/backend/proto/kandev/plugin/v1/plugin.proto`. (Whether these land on the
   existing `service Host` or a sibling `service HostData` served on the same
   broker connection is an implementation detail settled in the plan; the wire
   package and capability model are identical either way.)

2. **Capability gating per resource.** Each read RPC requires
   `api_read:<resource>` in the plugin's manifest; each write RPC requires
   `api_write:<resource>` (e.g. `api_read:tasks`, `api_read:sessions`,
   `api_write:tasks`, `api_write:comments`). Enforcement reuses the existing
   per-plugin binding and `manifest.CanRead`/`CanWrite`; an undeclared capability
   returns gRPC `PermissionDenied` with `capability 'api_read:tasks' not declared`
   — identical to today's state/secrets gating. The gate lives at the RPC handler
   (or the existing unary interceptor), not in the plugin.

3. **Reads go through the service layer; writes go through service methods that
   publish events.** Read handlers call the relevant service (`taskSvc`,
   workflow service, analytics/session aggregation), never a repository directly,
   so future access rules and derived fields stay in one place. Write handlers
   call `Service.CreateTask` / `UpdateTask` / comment-create methods that publish
   `task.*` events — never `repository.TaskRepository`. Publishing events is the
   whole reason plugins must not write the DB.

4. **Hand-mapped, versioned DTOs — never internal structs.** The proto messages
   are a stable public contract. The backend maps internal models → these DTOs
   with explicit code; that mapping *is* the decoupling. We do not marshal domain
   structs through `structpb`, do not generate DTOs from models, and do not
   expose raw table rows. `SessionCodeStats` is a deliberately **computed** shape
   (committed insertions/deletions + peak pending-diff additions/deletions),
   never the raw `task_session_commits` / `task_session_git_snapshots` rows whose
   JSON schema churns.

5. **Conventions across the contract.**
   - **Pagination:** opaque cursor (`Page{limit, cursor}` → `PageInfo{next_cursor,
     has_more}`), so the server can change ordering or store without breaking
     plugins.
   - **Timestamps:** RFC3339 strings, matching the `Event` envelope and the JSON
     API — one time representation across the whole plugin contract.
   - **Nullables:** proto `optional` so absent (NULL) is distinguishable from
     empty.
   - **Write provenance:** the server stamps `source = "plugin:<id>"` on created
     rows/comments; a plugin cannot set it.

## Open decisions (recorded, with recommendation)

- **(a) Workspace scoping.** Plugins are instance-global today (installed by the
  operator, no per-user access). Reads could either require a `workspace_id` /
  become workspace-scoped, or stay global with a filter hook. **Recommendation
  for v1: global-with-hook** — reads return across all workspaces the instance
  holds, filters (`workspace_ids`, `task_ids`) narrow results, and a single
  server-side scoping hook is left in place so a future per-plugin or per-user
  workspace restriction can be enforced without a contract change. Matches the
  existing "plugins are global to the instance" permission model in the spec.

- **(b) SessionCodeStats freshness.** Computed **on demand per request** in v1
  (one aggregation query per `ListSessionCodeStats` call, filtered and paginated),
  not precomputed or cached. The agent-stats plugin already caches its own report
  for 60s, so per-request compute is acceptable; a materialized/precomputed path
  is future work if the query proves hot.

## Consequences

- Plugins get a first-class, stable data path; the agent-stats plugin is
  rewritten to use it (`host.Sessions().List(...)` +
  `host.Sessions().CodeStats(...)`) and stops opening the DB file — the proof the
  carrot is sufficient.
- The proto is a public contract: DTO fields are additive-only thereafter;
  removing or renaming a field is a breaking change requiring a new api_version.
- A **new read-only per-session LOC aggregation must be added** at the service/
  repository layer. `internal/analytics/repository/sqlite/stats.go` aggregates git
  stats at workspace and per-repository granularity only; nothing computes
  per-session committed LOC + peak pending-diff, and the peak-pending snapshot
  aggregation (`MAX` over per-snapshot `SUM` of `json_each(files).additions`) does
  not exist in-tree. This is the one net-new query the decision requires.
- Enables the security "stick": once the sanctioned path exists, we can scrub
  `KANDEV_*` env from the plugin subprocess, stop making the DB path derivable,
  and pursue sandboxing without breaking a real plugin. Tracked as the isolation
  follow-ups referenced from the plugin-system buildout.
- Write RPCs inherit kandev's event-publishing invariant for free by routing
  through service methods; no plugin can mutate a task without `task.*` firing.

## Alternatives considered

- **Raw DB access (status quo).** What the agent-stats plugin does today.
  Rejected: schema coupling, no write events, breaks on Postgres, no auth/scoping,
  and only possible because the subprocess is unsandboxed. This is the failure the
  ADR exists to close.
- **REST passthrough with the plugin's own HTTP client.** Let plugins call
  kandev's HTTP API. Rejected: reintroduces credential/auth machinery the gRPC
  contract deliberately removed (`GRPC-CONTRACT.md` §1), couples plugins to the
  backend's network topology and base URL, and duplicates the transport the
  Host channel already provides securely (spawn relationship + AutoMTLS).
- **Expose internal structs via `structpb`.** Marshal domain models as generic
  structs over the existing Struct-typed fields. Rejected: recreates the exact
  schema coupling of raw DB access, just one layer up — plugins would depend on
  internal field names and shapes with no stable contract boundary.
</content>
</invoke>
