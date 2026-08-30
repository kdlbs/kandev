---
status: draft
system: tasks
requirements:
  - REQ-TASKS-EXTERNAL-ID-001
  - REQ-TASKS-EXTERNAL-ID-SCENARIOS-001
  - REQ-TASKS-EXTERNAL-ID-BOUNDARIES-001
---


# External task ID idempotency operations System Design



## Purpose and boundaries



This design records the operational, failure, persistence, and current-state evidence needed to preserve the idempotency contract.



## Requirement mapping

| Requirement | Design source |
| --- | --- |
| `REQ-TASKS-EXTERNAL-ID-001` | Extracted from the legacy design sections below. |
| `REQ-TASKS-EXTERNAL-ID-SCENARIOS-001` | Extracted from the legacy design sections below. |
| `REQ-TASKS-EXTERNAL-ID-BOUNDARIES-001` | Extracted from the legacy design sections below. |


## Permissions

- The external ID is workspace-scoped data and inherits the workspace's existing
  authorization rules with no additions.
- Authorization precedes all external-ID work. An unauthorized caller receives
  `404 {"error": "task not created"}` for a create and
  `404 {"error": "task not found"}` for a lookup or release, and cannot learn
  whether an identity exists in that workspace or whether its task is settled.
- Because uniqueness is workspace-scoped and workspaces are per-owner when auth
  is enabled, two users cannot collide on each other's external IDs or probe
  each other's namespaces.
- An in-session agent calling `create_task_kandev` is already scoped to its
  task's workspace owner by the MCP identity scoper.
- Release is a workspace-scoped mutation requiring the same authorization as any
  other write to that workspace.

## Failure modes

| Condition | Behavior |
|---|---|
| Two concurrent creates race past the step-3 lookup | The partial unique index admits exactly one. The loser's insert transaction rolls back — including its task row and runner participant — then it re-reads by `(workspace_id, external_id)` and returns that task as a Found outcome. Neither sees a `5xx`, and no orphan row survives. A loser that already allocated an office identifier has burned a sequence number; see the row below. |
| A concurrent loser burned a task-identifier sequence number | Accepted. The step-3 lookup makes this rare (it requires two creates to interleave within the window between lookup and insert), and the consequence is a gap in the office identifier sequence, not a correctness failure. Identifier sequences are not required to be contiguous. |
| Unique-violation classification across dialects | The violation MUST be attributed to `uniq_tasks_external_id` specifically — by constraint name on PostgreSQL via typed `pgconn.PgError` inspection, and by the constraint-bearing message on SQLite. A bare `23505` check is insufficient: it matches any unique violation, so a primary-key failure would be misread as a Found outcome. |
| Crash between the task insert and settlement | The task exists unsettled. A retry returns **Found, unsettled**. Nothing is deleted or repaired. |
| Crash after settlement, before the response reaches the caller | A retry returns **Found, settled**. |
| The task is deleted while its create is still running | Settlement affects zero rows. Async dispatch is suppressed, state is re-read, and the response reflects reality rather than claiming a create that no longer exists. |
| The identity is released while its create is still running | Settlement affects zero rows (the `external_id` predicate no longer matches). Same handling as above; the task survives without an identity. |
| Found outcome where `start_agent: true` was requested | No agent launched, no session created. This is the central safety property. |
| Found outcome on an archived task | The archived task is returned with `archived_at` set. Not auto-unarchived. |
| Found outcome where the request carried different repositories, attachments, parent, or policy | All ignored; the existing task is returned unmodified. |
| A retry fails handler validation because server state changed | The request fails with that handler's existing error. Documented, not worked around. Use the lookup route for drift-robust recovery. |
| Workspace deleted | Its tasks are deleted; their identities go with them. |
| A caller supplies an external ID on a subtask create | Accepted and applied to the subtask, scoped to the effective workspace. Never inherited from the parent. |

## Persistence guarantees

- Both columns live on `tasks` and survive restart with the row. Nothing is
  cached, and there is no TTL.
- **There is no idempotency window.** A held identity resolves against the
  current table however old the task is.
- An unsettled task persists indefinitely. Kandev never sweeps, expires, or
  repairs one. It is visible and mutable like any other task.
- A released or deleted identity is gone irreversibly; no history is retained.

## Appendix: verified current state

Claims checked against the code at `09fa82144` (a fast-forward of `origin/main`
at `cba3838c8`).

**In-scope entry points.** REST `POST /api/v1/tasks` (`task_handlers.go:140` →
`httpCreateTask` at `task_http_handlers.go:826`, body struct at `:730`) and MCP
`create_task_kandev` (tool schema `internal/mcp/server/server.go:805`, client
handler `internal/mcp/server/handlers.go:142`, backend handler at
`internal/mcp/handlers/handlers.go:559`). Both converge on
`taskservice.Service.CreateTask` (`internal/task/service/service_tasks.go:124`),
which authorizes first at `:125`.

**Why the lookup must precede allocation and admission.** `assignIdentifier`
runs at `service_tasks.go:160`, before `createTaskWithCapacity` at `:166`, and
calls `IncrementTaskSequence` — an unconditional
`UPDATE workspaces SET task_sequence = task_sequence + 1`
(`repository/sqlite/task.go:2328`). Admission checks precede the insert
(`task.go:215`) and can return before it (`:257`). Both were confirmed by
adversarial review.

**Why settlement needs the `external_id` predicate.** Release sets both columns
to `NULL`, so a predicate of `id` plus `external_id_settled_at IS NULL` still
matches a released row.

**Why settlement lives in the handlers.** The service publishes `task.created`
and returns before caller-specific work (`service_tasks.go:171`, `:190`). REST
then claims attachments (`task_http_handlers.go:912`), attaches policy (`:933`),
commits a fresh branch (`:947`), and prepares/starts a session (`:954`); MCP
associates remote contributions (`handlers.go:715`), attaches policy (`:737`),
and launches (`:749`).

**Why REST's session helper must be split.** Preparation errors are swallowed at
`task_http_handlers.go:1237` while the start goroutine is dispatched at `:1289`,
inside the same helper. PR association is background work at `:1201`. MCP
auto-start is a goroutine at `handlers.go:1303`.

**The MCP data-loss path.** `resolveMCPRemoteContributions` derives contributions
from the request at `handlers.go:658`; the handler indexes them against the
returned task's repositories at `:715-719`; the guard at `:719` calls
`DeleteTask` at `:720`. Further rollback-delete paths at `:729` and `:741`.
Independently confirmed.

**Schema placement.** The `tasks` DDL is
`internal/task/repository/sqlite/base_schema.go:295`. Per
`apps/backend/AGENTS.md`, a column added to an existing table must come from an
idempotent `ADD COLUMN` in `runMigrations()`, with anything referencing it
following there, not in schema init. Precedents at `base_migrations.go:165` and
`:1086`. The insert to extend is `insertTaskTx` (`repository/sqlite/task.go:316`),
whose task insert and runner participant are genuinely transactional — a unique
loser leaves no row and does not double-count WIP.

**Response shape.** `createTaskResponse` (`task_http_handlers.go:761`) embeds
`dto.TaskDTO` (`internal/task/dto/dto.go:143`); the current success path returns
`http.StatusOK`, not `201` (`:960`). `TaskDTO` already carries the analogous
optional `Identifier` at `dto.go:195`. Unauthorized/not-found maps to `404` via
`handleNotFound` (`internal/task/handlers/errors.go:20`).

**Representations are not all TaskDTO.** WS task lifecycle events are a
hand-built map in `publishTaskEventNow` (`service_events.go:367`). Plugins have
an independent proto message, SDK type, and mapper (`plugin.proto:216`,
`pkg/pluginsdk/data_types.go:92`, `internal/plugins/host_data.go:878`);
`CreateTaskResponse` holds only `Task task = 1` (`plugin.proto:456`) and SDK
`TaskReader.Create` returns `(*Task, error)` (`pkg/pluginsdk/host.go:138`).

**Dialect handling.** Partial unique indexes work on both dialects. A typed
`pgconn.PgError` precedent exists at `repository/sqlite/task.go:2238`. Every
existing *unique*-violation helper matches SQLite text only
(`internal/office/repository/sqlite/wakeup_requests.go:246`,
`internal/github/store.go:1433`, `internal/prompts/service/service.go:64`,
`internal/sentry/store.go:948`), and per `apps/backend/AGENTS.md` this package
also runs against PostgreSQL.

**Deletion shadow paths.** Office handoff cascades delete tasks through the
repository directly (`handoff_cascade.go:332`). This design is immune: the
identity is a column on the deleted row.

**Design provenance.** Three independent adversarial reviews (OpenAI Codex,
read-only). Pass 1 (unique index): 10 P1s, including a crash between the row
commit and post-create work leaving a half-created task returned as complete.
Pass 2 (reservation table + lease + reclaim): NOT READY, 7 P1s — no fencing
token, destructive release of live claims, non-atomic tombstoning across
deletion shadow paths, an unimplementable completion protocol given async work,
unsettled rows visible to ordinary reads, unsatisfiable handler ordering.
Pass 3 (identity on the task row, honest reporting): NOT READY, 7 P1s — the
decisive one being that `complete: false` cannot distinguish a live create from
an abandoned one, so the release-and-recreate "recovery" it implied could
produce duplicate tasks and agents.

This version responds by **removing the recovery claim rather than the
information**. The flag remains, explicitly diagnostic; the unsafe automated
action is named and forbidden; the settlement predicate, its zero-row handling,
its call site, and the lookup-before-write ordering are now specified concretely.
All factual findings from every pass were independently re-verified against
source before acceptance.
