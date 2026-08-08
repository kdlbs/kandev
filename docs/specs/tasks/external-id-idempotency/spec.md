---
status: draft
created: 2026-08-07
updated: 2026-08-08
owner: nova28
---

# Task external ID (create idempotency)

## Why

An external system that creates Kandev tasks over the API has no way to ask
"did I already create this one?". Every create is unconditionally a new task, so
a webhook redelivery, a network timeout the caller retried, or a crash between
"task created" and "I recorded the task ID" produces a duplicate task — with a
duplicate worktree and, when `start_agent` is true, a duplicate agent burning
tokens on work already in flight. Callers today compensate with fragile
heuristics (scanning task titles for a prefix, treating an existing branch name
as a witness that the task exists), which break the moment a title is edited or
a branch is renamed.

## What this feature is, and is not

**It is durable duplicate suppression.** A create carrying an external ID never
produces a second task for that identity, and always returns the task that holds
it — including after the caller crashed before recording the task ID. That is
the unblock: the caller gets its task ID back and stops guessing.

**It is not automated crash repair.** Kandev does not detect, adopt, resume, or
clean up a partially-created task, and callers MUST NOT automate doing so
either. The reason is not implementation cost — it is that the information does
not exist. A task whose creation has not finished is indistinguishable from one
whose creation is *still running*: both are simply "not settled yet". Without a
liveness signal — a lease, a heartbeat, an owner token — no observer can tell a
crashed create from a slow one.

An earlier draft claimed a retry would never observe a half-created task, which
required exactly that machinery: fencing tokens, compare-and-swap on every
transition, a two-phase coordinator, and isolation of unsettled rows. It was
reviewed and rejected as unbuildable. This spec does not reintroduce it, and
therefore does not make the guarantee it would have bought.

What callers get instead is an honest flag.

**`creation_complete` has exactly one meaning, in every outcome and on every
surface: the creation of the returned task completed its required synchronous
work.** Nothing more. It says nothing about whether an agent is running, and
nothing about whether any process is currently alive.

The diagnostic case is the specific tuple **`deduplicated: true` +
`creation_complete: false`** — another create claimed this identity and had not
finished when we looked; it may still be in progress. That combination is
diagnostic, not actionable. The safe response is to proceed with the returned
task ID, or to escalate to a human — never to release the identity and create
again. See *The one unsafe thing a caller can do*.

No other tuple carries the "may still be running" caveat.

## What

- A task MAY carry an **external ID**: a caller-supplied string that identifies
  the entity in the caller's own system (a Jira key, a webhook delivery ID, a
  UUID the caller minted before its first attempt).
- An external ID SHALL be held by at most one task per workspace.
- A create carrying an external ID SHALL have exactly one of four outcomes, and
  the response SHALL make which one unambiguous:
  1. **Created** — no task held that identity; a new task was created and holds
     it.
  2. **Found, settled** — a task already holds it and its creation finished.
  3. **Found, unsettled** — a task already holds it and its creation had not
     finished at observation time. It may still be running.
  4. **Created, identity lost** — this request created the task and finished its
     work, but another actor released the identity in the interim, so the task
     survives holding no external ID. Rare; see *Settlement*.
- Both **Found** outcomes SHALL have **no side effects**: no new task row, no
  new agent session, no agent launch, no repository attachment, no attachment
  claim, no workspace-policy write, no branch creation, and no `task.created`
  event.
  - **One bounded exception, on any concurrent-loser path.** When a Found
    outcome is resolved by the step-3 lookup — which is every ordinary retry —
    nothing whatsoever is consumed. When it is instead resolved *after* a
    step-3 miss, the loser may already have allocated an office task-identifier
    sequence number, and that number is not returned to the pool. This applies
    to **both** late-resolution paths, because identifier allocation precedes
    both of them: the unique-index backstop, and the pre-insert re-read that
    catches an admission or capacity failure. This is the single permitted side
    effect; it leaves a gap in the office identifier sequence, which is not
    required to be contiguous, and no other durable change.
- The system SHALL NOT delete, repair, resume, or reclaim an unsettled task, and
  SHALL NOT expire one. There is no duration after which behavior changes.
- **REST** callers SHALL have a side-effect-free way to ask what holds an
  identity without risking a create: the lookup route. **MCP callers do not get
  one in this iteration.** What MCP gets is an idempotent *create-if-absent*
  operation — safe to repeat, but it creates a task when no holder exists, so it
  is not a probe. This asymmetry is deliberate and is stated rather than papered
  over; see *The probe, and what MCP has instead*.
- Callers SHALL be able to release an external ID from a task, freeing it for
  reuse without deleting the task. This is an operator action, not a recovery
  step — see *The one unsafe thing a caller can do*.
- Omitting the external ID SHALL leave task creation behaving exactly as it does
  today.
- The external ID SHALL be accepted on the **REST create endpoint** and the
  **`create_task_kandev` MCP tool**. The WebSocket `task.create` action and the
  plugin host task-create API do not accept it in this iteration.
- The external ID SHALL appear on exactly the task representations listed in
  *Task representations*. That table is the complete requirement; there is no
  broader "everywhere tasks are read" obligation.
- The external ID identifies the **external entity**, not the request body. A
  second create for a held external ID SHALL return the existing task unchanged
  even when the rest of the payload differs; it SHALL NOT patch the existing
  task.
- An external ID SHALL NOT be inherited by subtasks, copied on any task-cloning
  path, or auto-generated by the system.

## The one unsafe thing a caller can do

**Do not automatically release an identity because a create reported
`creation_complete: false`, and then create again.** This is called out
explicitly because it is the intuitive "recovery" move and it is unsafe.

The failure:

1. Create A commits its task row for identity `E` and continues its remaining
   work normally.
2. Retry B observes `E` held by an unsettled task and returns
   `creation_complete: false`.
3. B releases `E` and creates again, producing task T2.
4. A finishes. Two tasks now exist for one external entity, and if both
   requested an agent, two agents are running.

The unique index prevents two *simultaneous holders*; it cannot prevent
duplicates across a release. Releasing an identity that another create is
actively using re-opens exactly the duplicate this feature exists to prevent.

**Safe responses to `creation_complete: false`:**

- Proceed with the returned task ID. It is a real task.
- Poll the lookup; if it settles, the original create finished on its own.
- Escalate to a human, who can inspect the task and decide.

**Release is for a human or operator who has determined the task is abandoned**
— not for an automated retry loop. Client libraries and agent tooling SHOULD NOT
expose release as an automatic recovery action.

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

## Out of scope

### Deferred write surfaces (designed, not built)

Neither has a named consumer today, and they are **not equally cheap** to
enable. Each is additive.

| Surface | Cost to enable later |
|---|---|
| WebSocket `task.create` | **Low.** One request-struct field, one line in the service-call literal, and skip the launch and last-used recording on a found outcome. One file. Deferred because it is the UI's own path: nothing retries on the UI's behalf. |
| Plugin host | **Meaningfully higher.** `plugin.pb.go` and `plugin_grpc.pb.go` are generated and committed, so a proto edit requires regenerating them through the Makefile proto target (installs `protoc-gen-go`, needs network). It also needs a new SDK method — **do not change the existing `TaskReader.Create` signature**, which is source-breaking for shipped plugins — a mapper change, and a release in each separate plugin repo. |

### Other non-goals

- **Detecting whether an unsettled create is still alive.** No lease, heartbeat,
  or owner token. This is the deliberate boundary of the feature.
- **Repairing, resuming, adopting, or garbage-collecting unsettled tasks.**
- **Any timeout or expiry on an unsettled task.**
- **A tombstone for deleted identities.** See *Idempotency is scoped to the
  task's lifetime*.
- **An MCP lookup tool.** MCP gets an idempotent create-if-absent, not a probe;
  no in-scope MCP flow needs to ask without being willing to create. Adding a
  read-only tool later is small and additive.
- **Changing an external ID in place.** Write-once; re-key via release + create.
- **System-generated external IDs.**
- **Idempotency for anything other than task creation.** `spawn_session_kandev`
  is not covered.
- **Restructuring the create handlers** so identity resolution precedes
  handler-level validation. The consequence — payload and server-state drift can
  fail a retry before dedupe — is documented, not engineered around.
- **Retiring the existing integration dedupe tables.**
- **Replacing the office `runs.idempotency_key` mechanism.**
- **The office runtime `create_task` HTTP action.**
- **Request-payload fingerprinting.**
- **Cross-workspace uniqueness or a global namespace.**
- **A UI surface** for entering, displaying, or releasing external IDs.
- **Bulk lookup or bulk release.**
- **Backfilling external IDs onto existing tasks.**

## Open questions

None. Every decision is settled; downstream steps implement as written and route
back here if a change is needed.

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
