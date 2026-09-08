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



The task system owns external identity uniqueness, creation settlement, and the create response contract. Integrations own the caller's identity and retry policy.



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

Putting the claim on the task itself, rather than in a side table, removes
whole classes of failure **by construction rather than by specification**:

| Hazard | Why it cannot occur here |
|---|---|
| Claim and task disagreeing | They are the same row. Nothing to keep in sync. |
| Orphan claim after a task is deleted | The columns are deleted with the row, through *any* deletion path — including office handoff cascades that call the repository directly and bypass the service. |
| Non-atomic "mark deleted" transition | There is no transition. Deleting the row releases the identity. |
| E2E reset leaving stale claims | Reset already deletes tasks; the columns go with them. No reset change is needed. |
| A task with no claim, or a claim with no task | One row, one insert. Impossible. |

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
`EXT-1` are different identities.

To make that true on both dialects, `tasks.external_id` and its index MUST use a
binary/deterministic collation (SQLite `COLLATE BINARY`, the default;
PostgreSQL `COLLATE "C"` or another explicitly deterministic collation). An
unqualified `TEXT` column on PostgreSQL follows the database collation, which
may be case-insensitive or nondeterministic and would silently violate this
contract. No Unicode normalization is performed.

### Lifecycle

- **Set once at create.** No API changes an external ID on an existing task;
  `PATCH /tasks/:id` and the update MCP/WS surfaces do not accept the field.
- **Settled once at create**, then never cleared or re-stamped except by release.
- **Archiving changes nothing.** An archived task still holds its identity.
- **Deleting frees the identity.** The row is gone.
- **Releasing frees the identity without deleting the task**, setting both
  columns to `NULL`.

### Idempotency is scoped to the task's lifetime

Once the task holding an identity is deleted or released, that identity is free,
and a later create makes a new task. This is a real limitation and it is stated
without excuse:

**A consumer that redelivers indefinitely must suppress redelivery for entities
it has itself deleted.** Kandev retains no record of a consumed identity, so it
cannot distinguish "never seen" from "seen and since deleted".

An earlier draft argued that a caller could detect this because the returned
task ID differs from the one it recorded. That rationale is withdrawn: the
motivating scenario is a caller that crashed *before* recording any task ID, and
such a caller has nothing to compare against.

The alternative — a durable tombstone of consumed identities — was rejected
because it requires the identity to outlive the task row, which reintroduces the
side table and every synchronization hazard the table above removes. If a real
consumer cannot suppress its own post-deletion redeliveries, that is the trigger
to revisit this, and it should be treated as a design change rather than a
tweak.

**This limitation does not affect the motivating crash-recovery case**, where no
deletion occurs.

## State machine

Two observable states for a task holding an external ID:

| State | Representation | Meaning |
|---|---|---|
| **unsettled** | `external_id` set, `external_id_settled_at IS NULL` | The create that claimed this identity had not finished its required synchronous work at observation time. It may still be running. |
| **settled** | both set | That work finished. |

Transitions:

| From | To | Trigger |
|---|---|---|
| (none) | unsettled | A create wins the unique index and commits the task row |
| unsettled | settled | That same create completes its required synchronous work |
| any | (none) | The task is deleted, or the identity is released |

There is no reclaim, no lease, no timeout, and no ownership token, because
nothing in this design ever mutates another caller's task.

### Create sequence (normative)

```
1. authorize workspace                       ← already first in CreateTask
2. validate + normalize external_id
3. LOOKUP by (workspace_id, external_id)     ← before ANY write or allocation
     └─ found → return Found outcome, stop. Nothing else runs.
4. required create-time validation
5. identifier allocation, WIP admission, task-row insert (incl. external_id)
     └─ unique violation on uniq_tasks_external_id → roll back, re-read, Found
6. required synchronous post-create work
7. SETTLE (conditional UPDATE, see below)
8. asynchronous dispatch (agent launch, PR association)
```

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
explicitly.** WIP admission runs *before* the insert and can return a capacity
error without ever attempting it. So a request that missed at step 3, then lost
the race, can surface a capacity failure instead of the Found outcome the
contract promises.

Therefore: **after a step-3 miss, any pre-insert failure — capacity, admission,
or otherwise — MUST trigger a re-read by `(workspace_id, external_id)` before
that failure is returned.** If a task now holds the identity, the request
returns the Found outcome instead of the failure. Only if the re-read still
finds nothing does the original error surface.

This is narrow but load-bearing: without it, "a retry always returns the
existing task" is false precisely when the destination step is at its WIP limit,
which is exactly when a task is most likely to already exist there.

### Settlement (normative)

Settlement is a single conditional UPDATE. The predicate MUST include
`external_id`:

```sql
UPDATE tasks
   SET external_id_settled_at = ?
 WHERE id = ?
   AND external_id = ?
   AND external_id_settled_at IS NULL
```

`external_id` in the predicate is **required, not defensive**. Release sets both
`external_id` and `external_id_settled_at` to `NULL`; a predicate guarding only
on `id` and `external_id_settled_at IS NULL` would still match a released row
and stamp a settlement onto a task that no longer holds any identity.

**The affected-row count MUST be checked.**

- **One row** — settlement succeeded. Proceed to asynchronous dispatch.
- **Zero rows** — the task was deleted or its identity released while this
  create was running. The implementation MUST re-read current state and MUST NOT
  dispatch asynchronous work. It then splits by what it finds:
  - **The task exists but no longer holds the identity** (it was released) →
    return outcome `CreatedIdentityLost`: `200`, `deduplicated: false`,
    `creation_complete: true`, and `external_id` absent from the body. The work
    finished; only the stamp had nowhere to land.
  - **The task no longer exists** (it was deleted) → return the surface's
    existing not-found error. There is no task to describe.

  It MUST NOT report outcome `Created`, and MUST NOT include `external_id` in
  the body — either would claim an identity the task no longer holds. Note that
  `creation_complete: true` **is** correct here and is not what distinguishes
  this outcome: the create's synchronous work did finish. The absent
  `external_id` is the distinguishing signal.

Zero rows is a legitimate outcome of a benign race, not an error to log and
ignore.

### Settlement call site (normative, per surface)

The service cannot settle, because required synchronous work continues in the
handlers after `CreateTask` returns. Settlement is therefore the **handler's**
responsibility, and the covered steps differ per surface:

| Surface | Required synchronous steps that must succeed before settling | Settle after |
|---|---|---|
| REST | attachment claim, workspace-policy attach, fresh-branch commit | fresh-branch commit, and **before** any session dispatch |
| MCP | remote-contribution association, workspace-policy attach | policy attach, and **before** auto-start dispatch |

**REST's session helper must be split.** It currently performs synchronous
preparation and asynchronous dispatch in one call, swallowing preparation
errors before dispatching the start goroutine. Settling after that helper would
place settlement after async dispatch has already begun, violating step 7 → 8
ordering. The helper must expose preparation and dispatch separately so
settlement can sit between them.

**Best-effort steps do not block settlement.** Task-create last-used recording
and PR association are best-effort today; their failure MUST NOT prevent
settlement and MUST NOT trigger compensation. Only the steps listed in the table
above gate it.

### What "settled" does and does not mean

`external_id_settled_at` means **the required synchronous setup finished**. It
does *not* mean an agent is running, and it does not mean the create's process
is alive or dead.

Callers needing to know whether an agent is running read session state, which
already exists and is already authoritative.

## API surface

### Service contract

`Service.CreateTask` currently returns `(*models.Task, error)` and carries no
outcome signal. It SHALL return:

```
CreateTaskResult {
  Task     *models.Task
  Outcome  enum { Created, FoundSettled, FoundUnsettled, CreatedIdentityLost }
}
```

`CreatedIdentityLost` is the fourth outcome, and it exists solely to represent
the settlement zero-row race: this request created the task and finished its
required synchronous work, but by the time settlement ran the identity had been
released by another actor, so the task survives holding no external ID. Without
it the contract has a reachable state with no representation — the handler would
have to claim an identity the task no longer holds, or invent an undocumented
response. See *Settlement*.

It is deliberately distinct from `FoundUnsettled`: that one means *someone
else's* create is unfinished, while this one means *this* create finished but
lost its identity. Callers act differently — the first says "wait or escalate",
the second says "your task exists, but the identity you asked for is now free
and someone may claim it".

**It carries `creation_complete: true`.** The work finished; only the settlement
stamp had nowhere to land. Reporting `false` would break the field's single
definition and falsely suggest in-progress work. The absent `external_id` is
what identifies this outcome.

If the task was **deleted** rather than released during that window, there is no
task to return and the request surfaces the appropriate not-found error; that is
an error path, not this outcome.

Callers MUST skip their post-create work on both `Found*` outcomes:

| Caller | In scope? | Post-create work to skip |
|---|---|---|
| REST `httpCreateTask` | **Yes** | attachment claim, workspace-policy attach, fresh-branch commit, session prepare/start, task-create last-used recording, PR association |
| MCP `handleCreateTask` | **Yes** | remote-contribution association, workspace-policy attach, auto-start launch |
| WS `wsCreateTask` | Deferred | agent launch, last-used recording |
| Plugin host `Tasks().Create` | Deferred | `StartAgent` best-effort start |

The deferred rows cannot misbehave in this iteration because their surfaces do
not accept an external ID, so the outcome is always `Created` for them. They are
listed so enabling one later starts from a complete inventory.

### ⚠️ The MCP skip is a data-loss guard

Running MCP's post-create steps on a found outcome can **delete the existing
task**. The handler resolves remote contributions from the *request*
(`handlers.go:658`), then indexes them against the *returned task's*
repositories (`:715-719`). A found outcome returns a task whose repository list
need not match the retry payload, and the index guard at `:719` calls
`DeleteTask` at `:720`. Two further rollback paths do the same at `:729` and
`:741`. The sequence:

> retry a create with an external ID → found outcome returns the existing task →
> post-create steps wrongly run → repository index mismatch → rollback
> **deletes the task the caller was trying to recover**

That turns crash recovery into data loss. Skipping these steps on a found
outcome is a **correctness requirement**, and its scenario is required coverage.

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

The field is named `creation_complete`, not `complete`, because it describes one
narrow thing — that the create's required synchronous setup finished — and a
broader name invites callers to read it as "the task is done" or "the agent is
running". Every schema and tool description MUST carry that narrow meaning.

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
- It is distinguished from `Created` by the **absent `external_id`**, which is
  the literal truth: the task no longer holds one. A caller seeing this tuple
  has a valid task ID and should treat the external identity as unclaimed.

All four success outcomes are `200` with the task body. There is no `409` and
no `410`: an unsettled task is a fact to report, not a conflict to raise, and a
freed identity is indistinguishable from one never used. Keeping fresh creates
at `200` also avoids a breaking change for existing clients.

### The probe, and what MCP has instead

**REST callers get a true probe:** the lookup route below. It reads and returns;
it never creates.

**MCP callers do not.** A create carrying an external ID is *idempotent* — safe
to repeat, no duplicates, no side effects when a holder exists — but it is not
side-effect-free, because when no holder exists it creates a task. Calling that
a probe would be wrong, and an MCP agent told it was one could use it to "check"
an identity and be surprised to find it had made something.

The MCP tool description MUST therefore say plainly: this creates the task if
nothing holds the identity yet.

No MCP lookup tool is added in this iteration because no in-scope MCP flow needs
to ask without being willing to create — an agent reaching for an identity is
reaching for the task. If a flow appears that genuinely needs to ask first, add
a read-only tool then; it is a small, additive change.

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
```

Sets `external_id` and `external_id_settled_at` to `NULL` on the task that holds
it. Does **not** delete or otherwise modify the task. Returns `204` on success,
`404` when no task holds it, `400` on validation failure, `404` when
unauthorized.

Release is an operator action for an identity a human has determined is
abandoned. See *The one unsafe thing a caller can do* for why it must not be
automated in response to `creation_complete: false`.

### Request ordering and error precedence

Two orderings are normative:

1. **Authorization precedes any external-ID work.** An unauthorized caller
   receives the existing unauthorized response and SHALL NOT learn whether an
   external ID exists there, whether it is held, or whether its task is settled.
   `Service.CreateTask` already authorizes as its first statement.
2. **Validation of the external ID precedes the lookup**, so a malformed value
   is a `400` rather than an accidental lookup.

The step-3 lookup then precedes all remaining create-time work, per
*Create sequence*.

**Honest limitation — a retry can still fail before dedupe.** Both in-scope
handlers perform validation *before* reaching the service: REST validates
attachments, launch-profile requirements, and repositories; MCP resolves
repositories, workflow, workspace, contributions, and launch metadata. This spec
does not require restructuring them. Two distinct causes follow:

- **Payload drift** — the retry sends something different from the original.
  Mitigated by replaying the original payload.
- **Server-state drift** — the retry is byte-identical, but the world changed:
  a repository was deleted, a workflow or step was removed, an agent profile was
  disabled, a parent task was archived, a credential expired. **Replaying the
  original payload does not help here.**

Server-state drift means the "a caller that crashed and came back a week later
still finds its task" property holds for the **lookup route**, which touches
none of that validation, but **not necessarily for the create path**. A caller
that wants recovery robust against long absences should use the lookup route
first and fall back to create only when the lookup returns `404`.

This is a real weakening relative to an idealized idempotent create, and it is
stated rather than implied. Engineering around it would require a create
coordinator that resolves the identity before any handler validation, which was
judged not worth its cost.

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
else the parent's when `parent_id` is set, else auto-resolution when exactly one
workspace exists. The identity resolves against that **effective** workspace.

The pre-existing MCP-layer `title` check runs before any backend call, so an MCP
replay carrying an external ID but no title gets the MCP validation error rather
than a found outcome. This is existing behavior and is consistent with the
replay guidance above.

### Write surfaces NOT changed

The WebSocket `task.create` action and the plugin host `Tasks().Create` do not
accept `external_id`. A caller supplying it there has the field ignored, exactly
as any other unknown field is today; no error, no claim.

No dedupe event suppression is needed on the WebSocket: the found outcomes
return early inside the service, so WS subscribers never see an event for a
create that did not happen.

### Task representations

`external_id` is added to exactly these. This table **is** the requirement.

| Representation | Location | Carries it |
|---|---|---|
| `dto.TaskDTO` | `internal/task/dto/dto.go` | Yes — `omitempty`, like the sibling `identifier` field. Covers REST reads/lists and the MCP task tools, which project this DTO. |
| WS task lifecycle events | `publishTaskEventNow`'s hand-built map, `service_events.go` | Yes — added explicitly. **Not** built from `TaskDTO`, so it needs its own change. |
| Plugin `Task` + SDK type + mapper | `plugin.proto`, `pkg/pluginsdk/data_types.go`, `internal/plugins/host_data.go` | **No.** Deferred with the plugin surface. Adding it later is additive on the proto (ADR 0043) and not breaking. |
| `v1.Task` | `pkg/api/v1/task.go` | **No.** Separate legacy projection. |
| Task-context references | `pkg/api/v1/task_context.go` | **No.** Lightweight ref. |

`creation_complete` is a **create-response and lookup-response field only**. It
is not added to the shared task DTO, because it is meaningful only in the
context of an idempotent create; putting it on every task read would leak a
create-time condition into unrelated surfaces.
