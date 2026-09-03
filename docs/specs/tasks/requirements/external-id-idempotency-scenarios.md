---
status: draft
system: tasks
created: 2026-08-07
owners:
  - nova28
---


# External task ID idempotency scenarios Requirements



## Overview



Creation idempotency is defined by observable outcomes for first creates, retries, concurrent callers, and identity settlement races.



## Scenarios

### Golden path

- **GIVEN** workspace `W` has no task holding `ext-1`, **WHEN** a client POSTs
  `/api/v1/tasks` with `workspace_id: W` and `external_id: ext-1`, **THEN** a
  task is created, the response carries `external_id: "ext-1"`,
  `deduplicated: false`, `creation_complete: true`, a `task.created` event is
  published, and the row has a non-NULL `external_id_settled_at`.
- **GIVEN** a settled task `T` holding `(W, ext-1)`, **WHEN** a client POSTs the
  same create again, **THEN** the response is `200` with `id` equal to `T.id`,
  `deduplicated: true`, `creation_complete: true`, `W` still has exactly one
  task holding `ext-1`, and no `task.created` event is published.
- **GIVEN** a settled task `T` holding `(W, ext-1)`, **WHEN** a client GETs the
  lookup route, **THEN** the response is `200` with `T`'s DTO and no task is
  created.
- **GIVEN** no task holds `(W, ext-nope)`, **WHEN** a client GETs the lookup,
  **THEN** the response is `404`.

### Lookup-before-write ordering

- **GIVEN** an office workspace `W` whose `task_sequence` is `N`, and a settled
  task holding `(W, ext-1)`, **WHEN** a client POSTs a create with
  `external_id: ext-1` and office-task parameters, **THEN** the existing task is
  returned and `W.task_sequence` is **still `N`** — the step-3 lookup resolved
  it before any allocation.
- **GIVEN** an office workspace `W` whose `task_sequence` is `N` and **no** task
  holding `(W, ext-4)`, **WHEN** two office creates with `external_id: ext-4`
  race such that both miss the step-3 lookup, **THEN** exactly one task is
  created, the loser returns a Found outcome, and `W.task_sequence` may be
  `N+2` — the loser's allocated identifier is not reclaimed. This is the single
  permitted side effect of a Found outcome, and the sequence is not required to
  be contiguous.
- **GIVEN** a workflow step at its WIP limit, and a settled task holding
  `(W, ext-1)` on that step, **WHEN** a client POSTs a create with
  `external_id: ext-1` targeting that step, **THEN** the existing task is
  returned with `deduplicated: true` — **not** a WIP-capacity error. The step-3
  lookup resolves it before admission is consulted.
- **GIVEN** a workflow step with **exactly one remaining WIP slot** and **no**
  task yet holding `(W, ext-2)`, **WHEN** two creates with `external_id: ext-2`
  targeting that step race such that both miss the step-3 lookup, the winner
  consumes the last slot and inserts, and the loser is then rejected by WIP
  admission **before** reaching the insert, **THEN** the loser re-reads by
  `(workspace_id, external_id)`, finds the winner's task, and returns it as a
  Found outcome — **not** the capacity error. This is the admission-preemption
  guard; without it the contract's "a retry always returns the existing task"
  promise fails exactly when the step has just saturated.
  - The one-remaining-slot precondition is load-bearing: if the step were
    already full before either request, neither could insert, so there would be
    no winner for the loser to find and the scenario would be unsatisfiable.
- **GIVEN** the same saturated step and **no** task holding `(W, ext-3)`,
  **WHEN** a single create with `external_id: ext-3` is rejected by WIP
  admission and the re-read still finds nothing, **THEN** the original capacity
  error surfaces unchanged — the guard must not swallow genuine failures.
- **GIVEN** a settled task holding `(W, ext-1)`, **WHEN** a client POSTs a create
  with `external_id: ext-1` and a `repositories` entry naming a repository that
  no longer exists, **THEN** the outcome depends on where that validation lives:
  if the handler rejects it pre-service the request fails (documented
  limitation); if it reaches the service, the existing task is returned. The
  implementation MUST state which, and a test MUST pin the actual behavior.

### Unsettled outcome and the unsafe-recovery guard

- **GIVEN** a task `T` holding `(W, ext-1)` with `external_id_settled_at IS NULL`,
  **WHEN** a client POSTs a create with `external_id: ext-1`, **THEN** the
  response is `200` with `T.id`, `deduplicated: true`, and
  `creation_complete: false`; `T` is unmodified; no new task is created.
- **GIVEN** the same unsettled task `T`, **WHEN** a client POSTs that create ten
  more times over any span of time, **THEN** every response is identical and `T`
  is never deleted, repaired, or duplicated — there is no timeout after which
  behavior changes.
- **GIVEN** the same unsettled task `T`, **WHEN** a client GETs the lookup for
  `ext-1`, **THEN** the response is `200` with `creation_complete: false`.
- **GIVEN** a create for `ext-1` still running its required synchronous work,
  **WHEN** a second caller releases `ext-1` and creates again, **THEN** two tasks
  exist for `ext-1`'s entity. This is the documented unsafe path; the test
  exists to pin the consequence, and the MCP tool description and REST docs MUST
  warn against automating it.
- **GIVEN** an unsettled task `T` holding `(W, ext-1)`, **WHEN** an operator
  DELETEs the release route and then a client POSTs the create again, **THEN**
  the release returns `204`, `T` still exists with NULL `external_id`, and the
  create produces a **new** task with `deduplicated: false`.

### Settlement predicate and zero-row handling

- **GIVEN** a create for `ext-1` that has committed its task row and finished its
  required synchronous work, **WHEN** settlement runs, **THEN** exactly one row
  is updated and asynchronous dispatch proceeds.
- **GIVEN** a create for `ext-1` whose identity is **released** by another actor
  after the row commit but before settlement, **WHEN** settlement runs, **THEN**
  it affects **zero** rows — the `external_id` predicate no longer matches — no
  asynchronous work is dispatched, and the response is `200` with
  `deduplicated: false`, `creation_complete: true`, and **no** `external_id`
  field, i.e. outcome `CreatedIdentityLost`.
- **GIVEN** the same released-before-settlement race, **WHEN** the caller
  inspects the response, **THEN** it distinguishes this from `Created` by the
  absent `external_id`, and from `Found, unsettled` by `deduplicated: false`;
  **AND** `creation_complete` is `true`, because this create's synchronous work
  did finish.
- **GIVEN** every success outcome across the whole contract, **WHEN** their
  response tuples are enumerated, **THEN** `creation_complete: false` appears in
  exactly one — `Found, unsettled` — and always alongside `deduplicated: true`.
- **GIVEN** a create for `ext-1` whose **task is deleted** by another actor
  before settlement, **WHEN** settlement runs, **THEN** it affects zero rows, no
  asynchronous work is dispatched, and the response is the surface's existing
  not-found error rather than a fabricated success.
- **GIVEN** any settlement that affects zero rows, **WHEN** the handler
  continues, **THEN** no agent is launched and no PR association is started.

### No side effects on a found outcome

- **GIVEN** a settled task `T` holding `(W, ext-1)` with exactly one agent
  session, **WHEN** a client POSTs a create with `external_id: ext-1` and
  `start_agent: true`, **THEN** `T` still has exactly one session and no new
  agent execution is started.
- **GIVEN** an **unsettled** task `T` holding `(W, ext-1)` with no session,
  **WHEN** a client POSTs a create with `external_id: ext-1` and
  `start_agent: true`, **THEN** `T` still has no session and no agent is
  launched — an unsettled task is reported, never resumed.
- **GIVEN** a settled task `T` holding `(W, ext-1)`, **WHEN** a client POSTs a
  create with `external_id: ext-1`, `start_agent: true`, one attachment, and a
  `repositories` entry naming a different repository with `fresh_branch: true`
  (the flag is per-repository, not top-level), **THEN** `T` still has exactly its
  original repositories, no new worktree or branch exists, the attachment is not
  claimed, and no last-used task-create settings are recorded.
- **GIVEN** a settled task `T` holding `(W, ext-1)` and title `"Original"`,
  **WHEN** a client POSTs a create with `external_id: ext-1` and title
  `"Changed"`, **THEN** `T`'s stored title is still `"Original"`.

### MCP

- **GIVEN** a settled task `T` holding `(W, ext-1)`, **WHEN** an agent calls
  `create_task_kandev` with `external_id: ext-1` resolving to `W` and
  `start_agent: true`, **THEN** the tool result JSON carries `T.id`,
  `deduplicated: true`, `creation_complete: true`, and no new task or session is
  created.
- **GIVEN** an **unsettled** task `T` holding `(W, ext-1)`, **WHEN** an agent
  calls `create_task_kandev` with `external_id: ext-1`, **THEN** the result
  carries `T.id` with `creation_complete: false` and no task is created or
  started.
- **GIVEN** a settled task `T` holding `(W, ext-1)` whose repository list differs
  from the retry payload, **WHEN** an agent calls `create_task_kandev` with
  `external_id: ext-1` and a `repository_url` that would resolve to a remote
  contribution, **THEN** `T` is returned, no remote contribution is associated,
  **and `T` still exists**. This is the data-loss guard; required coverage.
- **GIVEN** a settled task `T` holding `(W, ext-1)`, **WHEN** an agent calls
  `create_task_kandev` with `external_id: ext-1` and a `workspace_mode` needing
  a workspace-policy attach, **THEN** no policy is attached and `T` still exists.
- **GIVEN** a settled task holding `(W, ext-1)` and `W` is the only workspace,
  **WHEN** an agent calls `create_task_kandev` with `external_id: ext-1` and no
  `workspace_id`, **THEN** that task is returned with `deduplicated: true`.
- **GIVEN** a settled task holding `(W, ext-1)`, **WHEN** an agent calls
  `create_task_kandev` with `parent_id` naming a task in a different workspace
  `W2` and `external_id: ext-1`, **THEN** a new subtask is created in `W2`
  holding `ext-1`, because the effective workspace is `W2`.
- **GIVEN** an agent whose session belongs to a task in workspace `W`, and a task
  holding `(W2, ext-1)` in a workspace the agent's task does not belong to,
  **WHEN** the agent calls `create_task_kandev` targeting `W2` with
  `external_id: ext-1`, **THEN** the call is denied by MCP identity scoping and
  reveals nothing about `W2`.

### Uniqueness and concurrency

- **GIVEN** a task holding `(W1, ext-1)` and nothing in `W2`, **WHEN** a client
  POSTs a create for `workspace_id: W2` with `external_id: ext-1`, **THEN** a new
  task is created in `W2` with `deduplicated: false`.
- **GIVEN** two tasks in `W` with no external IDs, **WHEN** a third is created
  without one, **THEN** the create succeeds — absent identities never collide.
- **GIVEN** a task `P` in `W` holding `ext-parent`, **WHEN** a client creates a
  subtask of `P` with no `external_id`, **THEN** the subtask holds no identity.
- **GIVEN** no task holds `(W, ext-race)`, **WHEN** two creates with
  `external_id: ext-race` are issued concurrently and both pass the step-3
  lookup, **THEN** exactly one task in `W` holds `ext-race`, both requests return
  `200`, the outcomes are (`deduplicated: false`, `deduplicated: true`), and
  **no orphan task row exists from the loser**.
- **GIVEN** the same race against **PostgreSQL**, **THEN** the same assertions
  hold — an environment-gated PostgreSQL test, since the SQLite path passing is
  not evidence for it.
- **GIVEN** a create whose task-row insert collides on the `tasks` primary key
  rather than on `uniq_tasks_external_id`, **THEN** the response is an error and
  **not** a Found outcome.

### Validation

- **GIVEN** any workspace, **WHEN** a client POSTs a create with an
  `external_id` whose trimmed UTF-8 length is 256 bytes, **THEN** the response
  is `400`, no task is created, and no agent is launched.
- **GIVEN** any workspace, **WHEN** a client POSTs a create with `external_id`
  `"ext-1\n"`, **THEN** the response is `400` — the control character is
  rejected before trimming.
- **GIVEN** any workspace, **WHEN** a client POSTs a create with `external_id`
  `"\t"`, **THEN** the response is `400`, not "absent".
- **GIVEN** any workspace, **WHEN** a client POSTs a create with `external_id`
  `"   "`, **THEN** the task is created with a NULL `external_id`, and the
  response omits `external_id`.
- **GIVEN** a task holding `(W, ext-1)`, **WHEN** a client POSTs a create with
  `external_id: "  ext-1  "`, **THEN** that task is returned with
  `deduplicated: true` — trimming precedes the lookup.
- **GIVEN** a task holding `(W, ext-1)`, **WHEN** a client POSTs a create with
  `external_id: "EXT-1"`, **THEN** a new task is created — case-sensitive.
- **GIVEN** a PostgreSQL install whose database collation is case-insensitive,
  **WHEN** the previous scenario runs, **THEN** it still produces two distinct
  tasks — the column's deterministic collation overrides the database default.

### Lifecycle

- **GIVEN** a settled task `T` holding `(W, ext-1)` that has been archived,
  **WHEN** a client POSTs a create with `external_id: ext-1`, **THEN** `T` is
  returned with `deduplicated: true` and a non-null `archived_at`, and `T`
  remains archived.
- **GIVEN** a settled task `T` holding `(W, ext-1)`, **WHEN** `T` is deleted and
  a client POSTs the create again, **THEN** a new task is created with
  `deduplicated: false` — idempotency is scoped to the task's lifetime.
- **GIVEN** a settled task `T` holding `(W, ext-1)`, **WHEN** `T` is deleted by
  an office handoff cascade that calls the repository directly, **THEN** the
  identity is free and a subsequent create succeeds — no code path can leave a
  stale identity behind.
- **GIVEN** a settled task `T` holding `(W, ext-1)`, **WHEN** a client DELETEs
  the release route, **THEN** the response is `204`, `T` still exists with NULL
  `external_id` and NULL `external_id_settled_at`, and the lookup returns `404`.
- **GIVEN** a task `T` holding `ext-1`, **WHEN** a client PATCHes
  `/api/v1/tasks/T` with an `external_id` field in the body, **THEN** `T`'s
  identity is unchanged.

### Permissions

- **GIVEN** auth is enabled, a settled task holding `(W, ext-1)` owned by user
  `A`, and user `B` who does not own `W`, **WHEN** `B` POSTs a create for
  `workspace_id: W` with `external_id: ext-1`, **THEN** `B` receives
  `404 {"error": "task not created"}`, the response carries no `deduplicated`,
  no `creation_complete`, and no field of `A`'s task, and no task is created.
- **GIVEN** the same setup but an **unsettled** task, **WHEN** `B` POSTs the
  same create, **THEN** `B` receives `404` — settledness is not observable
  across an authorization boundary.
- **GIVEN** the same setup, **WHEN** `B` GETs or DELETEs the by-external-id route
  for `W`, **THEN** the response is `404`.
- **GIVEN** user `B` unauthorized for `W`, **WHEN** `B` POSTs a create for `W`
  with a 300-byte `external_id`, **THEN** the response is `404`, not `400` —
  authorization precedes validation.

### Read surfaces and the deferred write surfaces

- **GIVEN** a task holding `ext-1`, **WHEN** it is returned by a REST read, a
  REST list, an MCP task tool, or a WS task lifecycle event, **THEN** each of
  those four carries `external_id: "ext-1"`.
- **GIVEN** a task holding no identity, **WHEN** any of those four returns it,
  **THEN** the representation omits `external_id` rather than sending `null` or
  `""`.
- **GIVEN** a task holding `ext-1`, **WHEN** it is returned by an ordinary task
  read, **THEN** the representation does **not** carry `creation_complete`.
- **GIVEN** a task holding `(W, ext-1)`, **WHEN** a client sends the
  `task.create` WS action with an `external_id` field, **THEN** a new task is
  created and the field is ignored.
- **GIVEN** a task holding `(W, ext-1)`, **WHEN** a plugin calls
  `Tasks().Create` with `StartAgent: true`, **THEN** a new task is created and
  started normally.

### Migration

- **GIVEN** an existing database whose tasks predate this feature, **WHEN** the
  backend starts, **THEN** both columns exist and are NULL for every
  pre-existing task, `uniq_tasks_external_id` exists, and creating tasks without
  an external ID continues to succeed.
- **GIVEN** the same database, **WHEN** the backend starts a second time,
  **THEN** the migration replays without error.

## Requirements



### REQ-TASKS-EXTERNAL-ID-SCENARIOS-001: External task ID idempotency scenarios



**Intent:** Creation idempotency is defined by observable outcomes for first creates, retries, concurrent callers, and identity settlement races.



#### Acceptance criteria



- **AC-TASKS-EXTERNAL-ID-SCENARIOS-001.1:** When a creation scenario occurs, the system shall return the outcome, task identity, and side-effect behavior described by that scenario.



## Out of scope

- Boundary exclusions are recorded in [External task ID idempotency boundaries](external-id-idempotency-boundaries.md).
