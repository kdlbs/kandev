---
status: draft
system: tasks
requirements:
  - REQ-TASKS-EXTERNAL-ID-001
  - REQ-TASKS-EXTERNAL-ID-SCENARIOS-001
  - REQ-TASKS-EXTERNAL-ID-BOUNDARIES-001
---


# External task ID idempotency System Design



## Purpose and boundaries



The task system owns external identity uniqueness, creation settlement, and the
create response contract. [Task Creation Protocol](task-creation-protocol.md)
owns aggregate persistence and `task.created`; this document owns external-ID
outcomes layered on it. Integrations own caller identity/retry.



## Requirement mapping

| Requirement | Design source |
| --- | --- |
| `REQ-TASKS-EXTERNAL-ID-001` | Extracted from the legacy design sections below. |
| `REQ-TASKS-EXTERNAL-ID-SCENARIOS-001` | Extracted from the legacy design sections below. |
| `REQ-TASKS-EXTERNAL-ID-BOUNDARIES-001` | Extracted from the legacy design sections below. |


## Data model

Two columns on the existing `tasks` table. **There is no separate claim table.**

```
tasks
  ...existing columns...
  external_id             TEXT       nullable  caller-supplied identity;
                                               NULL when the task has none
  external_id_settled_at  TIMESTAMP  nullable  when creation finished;
                                               NULL means not settled
```

The retained `task_creation_operations` and `task_creation_steps` rows are
progress, not external-ID claims. They carry the opaque handle, plan, step
state/evidence/compensation, and completion/release state defined by the boundary
contract. Retained `task_external_id_release_operations` keys
`(workspace_id, operation_id)` and stores a canonical hash of method, workspace,
normalized external ID, and authenticated actor binding; holder/revision when
found; result/status/event ID; and timestamps. It is replay evidence only, never
owns/reserves an ID, survives workspace deletion, and is not pruned.
Release-operation state is closed:
`preparing|completed|aborted|workspace_deleted`, with claim
generation/owner/lease, holder/task binding, and immutable result/disposition.
`preparing` is claimed under the workspace/release-operation/external-ID lock;
completion or safe Abort stores the terminal result. Workspace deletion changes
an owned `preparing` row to `workspace_deleted` under the deletion generation
before clearing the task row. Replay is holder-independent and returns that
stored result; stale generation/lease has zero effect.

- **Uniqueness:** partial unique index `uniq_tasks_external_id` on
  `(workspace_id, external_id)` `WHERE external_id IS NOT NULL`. Supported by
  both SQLite and PostgreSQL. Naming it matters — violation handling must be
  attributable to this constraint specifically, never to "some unique index".
- **Settledness:** `external_id_settled_at IS NULL` means the create that
  claimed this identity had not finished its required synchronous work. It does
  **not** mean that create is dead.
- Empty or whitespace-only input normalizes to `NULL` (see *Validation*), so
  tasks without an external ID never collide with each other.

### Why the identity lives on the task row

The task row is the claim: aggregate insert/delete/release keeps identity and
task lifecycle atomic, leaving no orphan, split-brain, reset, or synchronization
state. Retained release replay never owns an identity.

### Validation and normalization

Applied to the **raw** caller-supplied value, in this exact order. The order is
normative because it decides which error a malformed value produces.

1. **Reject ASCII control characters** (U+0000–U+001F, U+007F) anywhere in the
   raw value — before any trimming. Newline and tab are control characters and
   are rejected here. Doing this before the trim is what makes a
   trailing-newline value an error rather than something the trim silently
   erases.
2. **Trim** leading and trailing Unicode whitespace (any code point with the
   Unicode `White_Space` property).
3. **Empty after trim** → treat as absent (`NULL`). Not an error.
4. **Length** of the trimmed value in **UTF-8 bytes** MUST be ≤ 255.

Everything surviving those rules is accepted verbatim: `jira:PROJ-1234`,
`gh-issue/kdlbs/kandev#2325`, a bare UUID, and non-ASCII letters are all valid.

### Comparison semantics

Matching is **byte-exact and case-sensitive** after normalization. `ext-1` and
`EXT-1` are different identities. No Unicode normalization is performed.

Exact per-dialect declarations, collation attestation, and the rejection of
any alternative collation are in *Dialect migration* below.

### Dialect migration and schema attestation

The canonical unique index is `uniq_tasks_external_id` on
`(workspace_id, external_id)` with the column's exact binary collation and
predicate `external_id IS NOT NULL`. SQLite declares the column/index
comparison `COLLATE BINARY`; PostgreSQL declares both column and index
comparison `COLLATE "C"`. Lookup and release use that same predicate and
comparison. Migration/startup compare exact column type/nullability/collation
plus index name, keys, collation, uniqueness, and predicate; a superficially
equivalent or incompatible index is rejected.

Migration is versioned and nullable-first for both columns. On an absent schema,
SQLite `v_external_id_columns` adds nullable
`external_id TEXT COLLATE BINARY, external_id_settled_at TIMESTAMP` in one write
transaction; PostgreSQL locks `tasks` and adds
`external_id text COLLATE "C" NULL, external_id_settled_at timestamp NULL`.
`v_external_id_index` then rejects duplicate IDs, creates the canonical partial
index, and attests both columns plus index. If either column is missing or
incompatible, SQLite transactionally rebuilds `tasks`, copies both values and
every other column byte-for-byte, verifies hashes/foreign keys, and swaps only
after attestation; PostgreSQL adds/alters only the missing/incompatible column,
drops only the incompatible external-ID index, and attests
`pg_attribute`/`pg_index`/`pg_collation`. A populated `external_id` without
settledness evidence blocks readiness with a retained diagnostic; it is never
silently treated as unsettled or settled and creates no recovery operation.
Failpoints cover detect/add/rebuild/copy/index/attest/commit-unknown; failure
rolls back byte-for-byte and retry is idempotent. All external-ID reads/writes
remain disabled until both columns and the canonical index attest.

### Lifecycle

- **Set once at create.** No API changes an external ID on an existing task;
  `PATCH /tasks/:id` and the update MCP/WS surfaces do not accept the field.
- **Settled once at create**, then never cleared or re-stamped except by release.
- **Archiving changes nothing.** An archived task still holds its identity.
- **Deleting frees the identity.** The row is gone.
- **Releasing frees the identity without deleting the task**, setting both
  columns to `NULL`.

### Idempotency is scoped to the task's lifetime

Once the holding task is deleted or released, the identity is free. Kandev
keeps no consumed-identity tombstone, so callers must suppress redelivery for
entities they deleted; a later create may reclaim the freed identity. Tombstones
would change the side-table boundary and need a new design decision; this
limitation does not affect crash recovery without deletion.
## State machine

Observable states for a task holding an external ID:

| State | Representation | Meaning |
|---|---|---|
| **unsettled** | `external_id` set, settlement NULL; creation `preparing|preparing-release|completing` | Required synchronous creation has not committed; release-in-progress remains observable as unsettled. |
| **settled** | both set; creation complete | Required synchronous creation committed. |

Transitions:

| From | To | Trigger/event |
|---|---|---|
| none | unsettled | Begin commits preparing revision zero; no event |
| unsettled | settled | Complete commits revision one plus `task.created` |
| unsettled `preparing|preparing-release` | none | operator Release records release in creation operation; no event until Complete |
| unsettled `completing` | unchanged | Release returns typed `creation_completing` conflict; Complete/recovery owns phase B |
| settled | none | aggregate Release clears ID, advances revision, enqueues one `task.updated` |
| any | none | typed aggregate task deletion |

External-ID ownership has no caller reclaim/lease. A CreationHandle authorizes
only the Created owner; the mandatory Runtime alone may obtain an expired
creation recovery lease. Found outcomes expose neither handle.
Lookup locks the external-ID identity and any `preparing-release` operation row;
release and lookup serialize on that key. `preparing-release` returns the same
FoundUnsettled outcome as other unsettled states and never exposes a handle.
Workspace deletion locks any `preparing` release operation before the task key,
increments its generation, and commits `workspace_deleted` with the deletion
generation and stable result/disposition. The task deletion then clears the
identity; release replay returns that retained terminal result and cannot write
or inspect a later holder. An expired release claim is recovered only under the
same workspace/release-operation fence.

### Create sequence (normative)

```
1. authorize workspace
2. validate + normalize external_id
3. LOOKUP by (workspace_id, external_id)     ← before ANY write or allocation
     └─ found → return Found outcome, stop. Nothing else runs.
4. required create-time validation and complete manifest construction
5. BEGIN aggregate: admission + preparing task/required DB rows +
     retained creation operation/revision 0; no lifecycle event
     └─ unique violation → roll back, re-read, Found
6. required synchronous physical/surface work recorded against the operation
7. COMPLETE aggregate: verify manifest + settle + revision 1 + task.created
     ├─ required-step failure before Complete → ABORT; retained identity, no event
     └─ ambiguous Complete result → retry same token; never abort blindly
8. asynchronous dispatch (agent launch, PR association)
```
REST, MCP, Office runtime, and agentctl creators implement the lookup preflight
and Found short-circuit under *Request ordering* below, parsing only transport
syntax, effective workspace/auth, and the raw external ID before the lookup.
REST uses a small raw envelope before the create DTO; the MCP schema permits
`external_id` without `title`, while post-miss validation still requires a
nonempty title.


**Step 3 is load-bearing and its position is normative.** It must precede
identifier allocation, WIP admission, and every other write. Two concrete
consequences of getting this wrong, both of which violate the no-side-effects
requirement:

- `assignIdentifier` runs before persistence and calls `IncrementTaskSequence`,
  an unconditional `UPDATE workspaces SET task_sequence = task_sequence + 1`.
  A found outcome resolved after that point permanently burns a sequence number.
- WIP admission can reject a create before the insert is ever attempted, so a
  retry for an already-held identity would fail with a capacity error instead of
  returning the existing task.

Step 5's unique-violation path remains as the **TOCTOU backstop** for the narrow
race between the step-3 lookup and the insert. It is not the primary mechanism.

**The backstop is not reachable from every failure, and that gap must be closed
explicitly.** WIP admission runs *before* the insert, so a request that missed
at step 3, then lost the race, can surface a capacity failure instead of the
Found outcome the contract promises.

Therefore: **after a step-3 miss, any pre-insert failure — capacity, admission,
or otherwise — MUST trigger a re-read by `(workspace_id, external_id)` before
that failure is returned.** If a task now holds the identity, the request
returns the Found outcome instead of the failure. Only if the re-read still
finds nothing does the original error surface.

This is narrow but load-bearing: without it, "a retry always returns the
existing task" is false precisely when the destination step is at its WIP limit,
which is exactly when a task is most likely to already exist there.

### Settlement (normative)

Settlement is `Store.CompleteTaskCreation(handle)`, not repository UPDATE. The
opaque handle binds operation/task/workspace, token, actor, and CreationPlan
manifest. Retained steps use the state machine and APIs in
[Task Creation Protocol](task-creation-protocol.md#creationplan-and-required-step-ledger).
Complete requires every planned step `succeeded` with matching proof, commits
`preparing -> completing`, and prohibits Abort. It then atomically:

1. stamps settlement if the task still holds the normalized external ID;
2. marks task/operation complete;
3. changes retained revision zero to one; and
4. inserts immutable revision-one `task.created`.

Same-handle replay returns stored outcome/event. Unknown commit retries Complete;
caller code never switches to Abort. If Release cleared the ID, completion emits
creation without it and returns `CreatedIdentityLost`. Aborted operations return
not-found/no dispatch.

Normal Abort requires a permanent failed step and proof that every possible
effect is absent, compensated, or durably transferred to cleanup. Expired
running/ambiguous steps become unknown and block both outcomes until Runtime
reconciles them. Runtime may reconstruct only a fenced recovery handle; Found
callers receive none.

### Completion call site (normative, per surface)

Handlers build the complete plan before Begin and synchronously drive
`CreationCoordinator.Execute(handle)`; they do not perform untracked steps:

The route rows below are a completion-surface projection of the canonical
creation inventory in `detached-workspace-continuity.md`; they may classify
steps differently but must not add or omit a public creator. Every listed owner
uses the same lookup-first external-ID protocol and `CreationCoordinator`.
For every creator, `CreationPlan.start_policy` is `none|immediate|deferred`,
resolved from `start_agent` and one canonical dependency admission: the create
request's `blocked_by` is empty. Non-empty `blocked_by` always selects
`deferred`, regardless of predecessor state; false `start_agent` selects
`none`; true selects `immediate` iff `blocked_by` is empty, else `deferred`.
Immediate has exactly one `session_prepare` and `launch_intent`; none/deferred
have neither. Deferred stores one start-when-unblocked intent and promotes only
from the last predecessor's `dependencies_resolved` transition; WIP gates
launches at the chokepoint, never the created policy. Other counts are zero/one attachment by
policy, one `attachment_claim` per nonempty set, one fresh-branch step per
repository, and one remote-contribution step per requested contribution. Found
plans no steps.


| Surface owner/symbol | Policy/default source | Planned required steps | No-policy / identity-found branch and test owner |
|---|---|---|---|
| `task/handlers.TaskHandlers.httpCreateTask` | HTTP body via `resolveWorkspacePolicy` | `workspace_attachment` when selected, one `attachment_claim` per nonempty set, repository/blocker, fresh branch per selected repo, remote contribution when requested, and `session_prepare`/`launch_intent` iff `start_policy=immediate` | no attachment when no binding; Found bypass; HTTP handler tests |
| `task/handlers.TaskHandlers.wsCreateTask` | WS body and parent | same cardinality; WS external-ID is deferred, but no-ID creation uses the shared plan | no attachment when no binding; WS handler tests |
| `mcp/handlers.Handlers.handleCreateTask` | MCP body/source task | same cardinality, including remote contribution when requested and session/launch iff `start_policy=immediate` | no attachment when no binding; Found bypass; MCP ledger tests |
| `backendapp.pluginsTaskWriterAdapter.CreateTask` | plugin input and service defaults | shared plan cardinality; plugin external-ID is deferred | no attachment when no binding; plugin writer tests |
| `task/service.Service.CreateChildTask` | parent group/member/environment binding | workspace attachment when selected; inherited repository/blocker steps plus session/launch iff `start_policy=immediate` | no attachment when no binding; Found bypass; child-service tests |
| `office/engine_adapters.TaskCreatorAdapter.CreateChildTask` | parent lookup plus child spec | delegates the same selected step set to task service | no independent step; adapter tests |
| `backendapp.childTaskCreatorAdapter.CreateChildTask` | Office parent plus typed spec | delegates the same selected step set to task service | no independent step; Office adapter tests |
| `backendapp.taskCreatorAdapter.CreateOfficeTask` / `CreateOfficeTaskAsAgent` | workspace office-workflow default | selected workspace attachment plus repository steps and session/launch iff `start_policy=immediate` | no attachment when no binding; Office adapter tests |
| `backendapp.taskCreatorAdapter.CreateOfficeTaskInWorkflow` | explicit workflow and workspace default | same selected step set | no attachment when no binding; routine creator tests |
| `backendapp.taskCreatorAdapter.CreateOfficeSubtask` | parent ID and inherited policy | same selected step set | no attachment when no binding; Office subtask tests |
| `backendapp.reviewTaskCreatorAdapter.CreateReviewTask` / `issueTaskCreatorAdapter.CreateIssueTask` | watcher workspace/repository request | selected workspace attachment plus repository steps and session/launch iff `start_policy=immediate` | no attachment when no binding; Found bypass; watcher tests |
| `office/runtime.RegisterRoutes -> Handler.createTask` and `Handler.createSubtask` | authenticated Office runtime request | delegates to `Actions.CreateTask`/`CreateSubtask`, which use the same external-ID lookup, plan, and coordinator | Office runtime route tests |
| `office/runtime.Actions.CreateTask` and `CreateSubtask` | Office `RunContext`, parent, and typed request | canonical service path; Found settled/unsettled bypasses all non-identity decoding and side effects | Office action tests |
| `cmd/agentctl.taskCreate` | authenticated CLI run context and raw external ID | same lookup-first envelope and coordinator; no independent settlement path | authenticated CLI task-create test |
| `workflow/engine.CreateChildTaskCallback.Execute` | trigger task and engine spec | delegates through `TaskCreatorAdapter` to the same plan resolver | never writes rows; workflow callback tests |

The MCP `remote-contribution association` is the required
`task_remote_contribution_associations` claim and gates `creation_complete`.
Provider PR/MR association in `github_task_prs`/`gitlab_task_mrs` is the distinct
optional `provider_pr_association` projection after `task.created`; its failure
cannot Abort or change creation outcome.

Best-effort feeder pull, last-used recording, and PR association are after
Complete and never trigger Abort. All creators use this protocol even without an
external ID, so `task.created` means required setup completed. Request failure
invokes only handle-bound Abort; an ineligible Abort leaves durable recovery.


## API surface

### Service contract

The internal service split is explicit:

```
BeginTaskCreationResult {
  Task     *models.Task
  Outcome  enum { Created, FoundSettled, FoundUnsettled }
  Handle   *CreationHandle // non-nil iff Created
}
CompleteTaskCreationResult {
  Task     *models.Task
  Outcome  enum { Created, CreatedIdentityLost }
  EventID  string
}
```

REST/MCP compose those into the existing four public outcomes. The handle is
an in-process capability, never DTO/MCP output, log data, or client input:
Created passes the exact handle through attachment/remote/session steps to
Complete or pre-Complete Abort; Found branches have none and return
immediately.

`CreatedIdentityLost` exists only as Complete's result: this request finished
the task after an operator release, so the task survives with no external ID.
It is distinct from FoundUnsettled, carries `creation_complete:true`, and exposes
the absent ID. If typed task deletion won, Complete returns not-found rather
than inventing a task response.

Callers MUST skip their post-create work on both `Found*` outcomes:

| Caller | In scope? | Post-create work to skip |
|---|---|---|
| REST `httpCreateTask` | **Yes** | attachment claim, fresh-branch commit, session prepare/start, task-create last-used recording, PR association |
| MCP `handleCreateTask` | **Yes** | remote-contribution association, auto-start launch |
| WS `wsCreateTask` | Deferred | agent launch, last-used recording |
| Plugin host `Tasks().Create` | Deferred | `StartAgent` best-effort start |

The deferred rows cannot misbehave in this iteration because their surfaces do
not accept an external ID, so the outcome is always `Created` for them. They are
listed so enabling one later starts from a complete inventory.

The no-side-effect contract is verified as a complete matrix:
`ExternalIDFoundNoSideEffectsMatrix` runs FoundSettled and FoundUnsettled
through REST `httpCreateTask` and MCP `handleCreateTask` and, per cell, asserts
zero effects in every enumerated category: attachment claim, fresh-branch
commit, session prepare/start, last-used recording, PR and remote-contribution
association, auto-start launch, revision/outbox row, and task-row mutation,
with the Found response returned unchanged (`deduplicated:true`;
`creation_complete:true` for FoundSettled only). A single-case test is not a
substitute for the matrix.


### REST — create

`POST /api/v1/tasks` gains one optional request field, `external_id`.

Response additions, both **always present** on every create response:

```jsonc
{
  "id": "…",
  "external_id": "jira:PROJ-1234",  // omitted when the task holds none
  "deduplicated": false,            // true for both Found outcomes
  "creation_complete": true,        // false ONLY for Found, unsettled
  // …all other existing task DTO fields…
}
```

`deduplicated` and `creation_complete` are required booleans, not presence-only
markers. A presence-only field makes a serialization bug indistinguishable from
a genuine fresh create.

The field is `creation_complete`, not `complete`, because it names one narrow
thing — required synchronous setup finished — never "the task is done" or "the
agent is running". Every schema and tool description MUST carry that meaning.

The four success outcomes map to exhaustive, mutually exclusive tuples:

| Outcome | Status | `deduplicated` | `creation_complete` | `external_id` in body |
|---|---|---|---|---|
| Created | `200` | `false` | `true` | present |
| Found, settled | `200` | `true` | `true` | present |
| Found, unsettled | `200` | `true` | `false` | present |
| Created, identity lost | `200` | `false` | `true` | **absent** |
| `external_id` fails validation | `400` | — | — | — |
| Caller not authorized for `workspace_id` | `404` `{"error": "task not created"}` | — | — | — |

Reading the tuples:

- **`creation_complete: false` occurs in exactly one outcome**, `Found,
  unsettled`, and only ever alongside `deduplicated: true`. That is the one
  tuple meaning "another create may still be in progress."
- **`Created, identity lost` carries `creation_complete: true`**, because this
  create's required synchronous work genuinely did finish — only the settlement
  stamp had nowhere to land, since the identity was released. Reporting `false`
  would contradict the single definition of the field and would falsely imply
  in-progress work.
- It is distinguished from `Created` by the **absent `external_id`**, the
  literal truth: the task no longer holds one. The caller has a valid task ID
  and should treat the external identity as unclaimed.

All four success outcomes are `200` with the task body. There is no `409` or
`410`: an unsettled task is a fact to report, not a conflict to raise, and a
freed identity is indistinguishable from one never used. Fresh creates stay
`200`, avoiding a breaking change for existing clients.

### The probe, and what MCP has instead

**REST callers get a true probe:** the lookup route below. It reads and returns;
it never creates.

**MCP callers do not.** A create carrying an external ID is *idempotent* —
repeatable with no duplicates and no side effects when a holder exists — but not
side-effect-free: with no holder it creates a task. Calling that a probe would
mislead an agent into "checking" an identity and creating something.

The MCP tool description MUST therefore say plainly: this creates the task if
nothing holds the identity yet.

No MCP lookup tool is added in this iteration because no in-scope MCP flow needs
to ask without being willing to create — an agent reaching for an identity is
reaching for the task. Add a read-only tool when a flow genuinely needs to ask
first; it is a small, additive change.

### REST — lookup

```
GET /api/v1/workspaces/:id/tasks/by-external-id?external_id=<value>
```

| Situation | Status | Body |
|---|---|---|
| A task holds it (including archived, including unsettled) | `200` | task DTO with `creation_complete` |
| No task holds it | `404` | `{"error": "task not found"}` |
| `external_id` missing or fails validation | `400` | `{"error": "<reason>"}` |
| Caller not authorized for the workspace | `404` | `{"error": "task not found"}` |

Read-only; no side effects.

### REST — release

```
DELETE /api/v1/workspaces/:id/tasks/by-external-id?external_id=<value>
Idempotency-Key: <caller-generated UUID>
```

Release requires the stable operation key; missing/malformed is `400`. Handler
builds `ReleaseTaskExternalID{WorkspaceID, NormalizedExternalID,
ReleaseOperationID, UserActor}`. Store first locks the release-operation key. An
existing same request hash returns stored status/event without looking at the
current holder; different hash is `409`. A new operation then locks external-ID
key, holder task, creation operation, retained revision, and outbox:
- completed: clear both columns, advance revision, insert immutable
  `task.updated`, and store holder/revision/event plus `204`;
- preparing/preparing-release: clear columns, record release in creation
  operation with no event, and store `204`;
- completing: return typed `creation_completing` conflict (`409`), leaving
  phase-B ownership unchanged;
- no holder: store/return `404`.

Thus response-loss replay cannot release a later reuse holder. Release before
Complete yields CreatedIdentityLost; Complete before release yields created then
updated; typed delete before a new release returns/stores 404; release before
delete orders updated before deleted for completed tasks. Repository settle/
release and direct publisher methods are removed. Unauthorized remains `404`;
malformed external ID is `400`. Release is manual, never automated from Found.

### Request ordering and error precedence

Ordering is normative:

1. Parse transport syntax and effective workspace/external-ID envelope.
2. Authorize workspace without exposing identity existence.
3. Validate/normalize external ID; malformed is `400`.
4. Lookup and return Found immediately.
5. Only a miss/no ID decodes and validates the remaining create request.
REST, MCP, Office runtime, and authenticated agentctl handlers are restructured
around this preflight; a late lookup in `Service.CreateTask` is insufficient.
Found wins over missing/invalid title,
deleted repository/workflow/step/parent, disabled profile, invalid attachment or
contribution, changed policy, and stale launch metadata. It also ignores payload
differences by design. Invalid JSON that prevents envelope parsing, unauthorized
workspace, and malformed external ID precede lookup. Tests cover every listed
drift/error for both Found outcomes with no sequence, claim, policy, branch,
session, event, or launch side effect.

### MCP

`create_task_kandev` gains one optional string parameter:

- `external_id` — "A stable identifier from your own system (issue key, webhook
  delivery ID, a UUID you generated). Creating a task twice with the same
  `external_id` in the same workspace returns the first task instead of making a
  duplicate — use it when a retry or restart could re-run this call. Replay the
  same arguments you sent the first time."

The tool result is the task as JSON carrying `external_id`, `deduplicated`, and
`creation_complete`. The tool description MUST state all three of these:

1. **This creates the task when nothing holds the identity yet.** It is
   create-if-absent, not a lookup. An agent must not call it merely to check.
2. **`deduplicated: true`** means the task already existed and was not created,
   so the agent must not report having created something new.
3. **`deduplicated: true` together with `creation_complete: false`** means
   another create claimed this identity and had not finished when observed, and
   **may still be in progress** — so the agent must not assume the task is
   ready, and must not release the identity and create again.

Point 3 is scoped to that tuple deliberately. `creation_complete` appears as
`false` in no other outcome, so an agent that keys off the pair cannot
misinterpret it.

The workspace is resolved before the identity is used: explicit `workspace_id`,
else a retained parent-to-workspace mapping from live task identity or
`task_lifecycle_revisions`, keyed by `parent_id`, without loading or validating
the live parent, else auto-resolution when exactly one workspace exists. The
identity resolves against that **effective** workspace. If no retained mapping
exists for a parent and no explicit workspace is supplied, the request must
provide `workspace_id`; it may not validate the parent merely to discover the
workspace.

The MCP input schema accepts an authorized `external_id` envelope without title
so preflight can return Found. After a miss, handler validation requires the
same title and create fields as today; no-ID calls are unchanged.

### Write surfaces NOT changed

The WebSocket `task.create` action and the plugin host `Tasks().Create` do not
accept `external_id`. A caller supplying it there has the field ignored, exactly
as any other unknown field is today; no error, no claim.

No dedupe event suppression is needed: Found outcomes return before Begin, while
Begin emits nothing. Only idempotent Complete inserts `task.created`, and replay
returns its stored event identity without a second outbox row.
Release event semantics are state-scoped: preparing/preparing-release release
is folded into the eventual created payload (or Abort with no event); completing
release is rejected; completed release is one next-revision `task.updated`. The
outbox payload is immutable, so later reuse cannot change historical events.

### Task representations

`external_id` is added to exactly these. This table **is** the requirement.

| Representation | Location | Carries it |
|---|---|---|
| `dto.TaskDTO` | `internal/task/dto/dto.go` | Yes — `omitempty`, like the sibling `identifier` field. Covers REST reads/lists and the MCP task tools, which project this DTO. |
| WS task lifecycle events | aggregate `task.created` outbox payload and event-bus adapter | Yes, copied from the completed immutable payload rather than reread |
| Plugin `Task` + SDK type + mapper | `plugin.proto`, `pkg/pluginsdk/data_types.go`, `internal/plugins/host_data.go` | **No.** Deferred with the plugin surface. Adding it later is additive on the proto (ADR 0043) and not breaking. |
| `v1.Task` | `pkg/api/v1/task.go` | **No.** Separate legacy projection. |
| Task-context references | `pkg/api/v1/task_context.go` | **No.** Lightweight ref. |

`creation_complete` is a **create-response and lookup-response field only**. It
is not added to the shared task DTO, because it is meaningful only in the
context of an idempotent create; putting it on every task read would leak a
create-time condition into unrelated surfaces.

## Verification ownership

Work order 01 owns lookup/MCP schema, CreationPlan/handle/step Store APIs and
coordinator, recovery/compensation, and retained aggregate release. Work order 04
proves both dialects and production REST/MCP composition. Matrices cover Found/
error precedence; handle/actor/plan/token mismatch; each step state, idempotency
key, lease loss, evidence, unknown proof and compensation; completing ambiguity;
workspace deletion of both unsettled states; release against all creation/delete
states; response-loss replay after later reuse; exact events; and no repository
bypass.
