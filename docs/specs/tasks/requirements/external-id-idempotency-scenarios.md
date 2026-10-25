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
- **GIVEN** settled or unsettled `T` holds `(W, ext-1)`, **WHEN** REST or MCP
  retries with any invalid or different non-identity payload, **THEN** `T`
  returns before non-identity validation and the retry has no side effects.
- **GIVEN** a task `T` holding `(W, ext-release)` whose creation is in
  `preparing-release`, **WHEN** REST or MCP looks up or retries that identity,
  **THEN** it returns FoundUnsettled with `creation_complete: false`; lookup and
  release serialize on the external-ID key, and no handle is exposed.
- **GIVEN** task `T` holds `(W, ext-parent)` and its parent `P` has been deleted,
  **WHEN** MCP retries with `parent_id: P`, no `workspace_id`, and any invalid
  non-identity payload, **THEN** retained parent-to-workspace identity resolves
  `W` before parent validation and Found returns with no side effects.

### Unsettled outcome and the unsafe-recovery guard

- **GIVEN** a task `T` holding `(W, ext-1)` with `external_id_settled_at IS NULL`,
  **WHEN** a client POSTs a create with `external_id: ext-1`, **THEN** the
  response is `200` with `T.id`, `deduplicated: true`, and
  `creation_complete: false`; `T` is unmodified; no new task is created.
- **GIVEN** the same unsettled task `T`, **WHEN** callers repeat create, **THEN**
  each observation returns FoundUnsettled unless internal Runtime has completed
  or aborted it; the retry itself never receives a handle, mutates, resumes,
  deletes, expires, or duplicates `T`.
- **GIVEN** the same unsettled task `T`, **WHEN** a client GETs the lookup for
  `ext-1`, **THEN** the response is `200` with `creation_complete: false`.
- **GIVEN** `launch_intent` is armed, or all required non-launch steps are
  succeeded in `preparing`, **WHEN** Runtime recovers after a crash before
  Complete phase A, **THEN** it claims `complete_preparing` under the fence and
  completes both phases exactly once.
- **GIVEN** the Created owner disappears while creation is preparing, **WHEN**
  its lease expires, **THEN** only mandatory Runtime may claim the retained
  manifest and either resume it or Abort a definitive pre-Complete failure.
- **GIVEN** a required step is `running` when its owner disappears, **WHEN** its
  lease expires, **THEN** it becomes `unknown`; neither Complete nor Abort runs
  until exact provider/marker proof resolves succeeded, safe retry, or no-effect
  permanent failure.
- **GIVEN** one required step permanently fails after another produced an effect,
  **WHEN** recovery runs, **THEN** the effect is fenced and compensated or
  transferred to a durable cleanup job before handle-bound Abort removes live
  state; no task lifecycle event is emitted.
- **GIVEN** a request is cancelled while a step is retrying or unknown, **WHEN**
  the handler returns, **THEN** it performs no direct delete/settle; the retained
  plan/step lease is recovered by Runtime with the same idempotency key.
- **GIVEN** any planned required step is not `succeeded` with matching evidence,
  **WHEN** Complete is attempted, **THEN** it returns conflict and does not enter
  `completing`.
- **GIVEN** Complete has durably entered `completing`, **WHEN** its response or
  process is lost, **THEN** the same handle or Runtime recovery retries Complete
  to one stored result/event and Abort is rejected.
- **GIVEN** a Found caller or mismatched operation/token/actor/manifest handle,
  **WHEN** Complete or Abort is attempted, **THEN** no state/event changes.
- **GIVEN** a create for `ext-1` still running its required synchronous work,
  **WHEN** a second caller releases `ext-1` and creates again, **THEN** two tasks
  exist for `ext-1`'s entity. This is the documented unsafe path; the test
  exists to pin the consequence, and the MCP tool description and REST docs MUST
  warn against automating it.
- **GIVEN** unsettled `T` holds `(W, ext-1)`, **WHEN** an operator releases with
  a fresh operation ID and a client creates again, **THEN** release is `204`, T
  remains with NULL identity, and the create produces a new task. This unsafe
  manual sequence is never initiated by retry logic.

### Completion and identity-release ordering

- **GIVEN** a Created handle whose manifest is complete, **WHEN** Complete runs,
  **THEN** it first durably enters non-abortable `completing`, then commits
  revision one plus exactly one `task.created` and dispatches committed intent.
- **GIVEN** the completion transaction outcome is unknown, **WHEN** the caller
  retries the same handle or Runtime takes over, **THEN** one stored task/event
  result is returned and Abort never runs.
- **GIVEN** operator release wins before Complete, **WHEN** Complete runs,
  **THEN** it emits one `task.created` whose immutable payload has no external
  ID, dispatches committed intent, and returns `CreatedIdentityLost`: `200`,
  `deduplicated:false`, `creation_complete:true`, external ID absent.
- **GIVEN** Complete wins before release, **WHEN** release runs, **THEN**
  `task.created` revision one precedes one next-revision `task.updated` clearing
  the identity.
- **GIVEN** every success outcome, **WHEN** tuples are enumerated, **THEN**
  `creation_complete:false` appears only with FoundUnsettled and
  `deduplicated:true`.
- **GIVEN** typed DeleteTask targets a preparing/completing task, **WHEN** it
  locks the operation, **THEN** it returns conflict with no mutation; only
  handle-bound pre-Complete Abort may remove unfinished live state.

### No side effects on a found outcome

- **GIVEN** a settled task `T` holding `(W, ext-1)` with exactly one agent
  session, **WHEN** a client POSTs a create with `external_id: ext-1` and
  `start_agent: true`, **THEN** `T` still has exactly one session and no new
  agent execution is started.
- **GIVEN** an **unsettled** task `T` holding `(W, ext-1)` with no session,
  **WHEN** a client POSTs a create with `external_id: ext-1` and
  `start_agent: true`, **THEN** the retry creates no session or launch and gets
  no handle; only separately scheduled internal Runtime recovery may progress T.
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
- **GIVEN** Office deletes settled `T` through the aggregate with typed workspace
  reason, **WHEN** deletion commits, **THEN** identity is freed with its ordered
  `task.deleted` and a later create may claim it; no repository bypass exists.
- **GIVEN** settled `T` and a fresh release operation ID, **WHEN** an authorized
  client DELETEs the release route, **THEN** response is `204`, T retains one
  next-revision `task.updated` clearing both ID columns, and lookup is `404`.
- **GIVEN** that release response is lost, **WHEN** the same operation ID is
  retried after the identity is free or reused, **THEN** stored `204` returns
  without inspecting or mutating the current holder.
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

- **GIVEN** a pre-feature database whose `tasks` table has neither ID column,
  **WHEN** the backend starts, **THEN** versioned nullable-column migration
  adds both columns with SQLite `BINARY`/PostgreSQL `"C"` ID collation before
  the canonical partial index; values NULL and readiness closed until attestation.
  Failure rolls back byte-for-byte.
- **GIVEN** the same database, **WHEN** the backend starts a second time,
  **THEN** schema attestation and migration replay succeed without changing rows.
- **GIVEN** `ext-1` already exists, **WHEN** `EXT-1` is created in the same
  workspace on either dialect, **THEN** both rows coexist and exact lookup
  returns only the byte-matching row.
- **GIVEN** incompatible column/index collation or a migration failpoint, **WHEN**
  startup runs, **THEN** migration either commits the fully attested canonical
  schema or rolls back byte-for-byte; readiness and external-ID reads/writes stay
  disabled on failure.

## Requirements



### REQ-TASKS-EXTERNAL-ID-SCENARIOS-001: External task ID idempotency scenarios



**Intent:** Creation idempotency is defined by observable outcomes for first creates, retries, concurrent callers, and identity settlement races.



#### Acceptance criteria



- **AC-TASKS-EXTERNAL-ID-SCENARIOS-001.1:** When a creation scenario occurs, the system shall return the outcome, task identity, and side-effect behavior described by that scenario.
- **AC-TASKS-EXTERNAL-ID-SCENARIOS-001.2:** Found outcomes shall win over every
  non-identity payload/server-state drift scenario while transport,
  authorization, workspace, and external-ID errors retain stated precedence.
- **AC-TASKS-EXTERNAL-ID-SCENARIOS-001.3:** Creation handle/step/lease/evidence/
  unknown/compensation, completing, ambiguous-result, and Runtime scenarios shall
  produce the stated single event or eligible Abort without caller adoption.
- **AC-TASKS-EXTERNAL-ID-SCENARIOS-001.4:** Preparing/completed release races and
  same-operation replay shall preserve the stated task, revision, event, and
  current-holder outcomes.



## Out of scope

- Boundary exclusions are recorded in [External task ID idempotency boundaries](external-id-idempotency-boundaries.md).
