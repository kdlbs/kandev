---
status: draft
system: tasks
created: 2026-08-07
owners:
  - nova28
---


# External task ID idempotency Requirements



## Overview



A caller can retry task creation with an external identity and receive the one task that owns that identity without creating duplicate work.



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

**It is durable duplicate suppression integrated with recoverable creation.**
A create carrying an external ID never produces a simultaneous second holder
and always resolves an already-held identity before request-specific payload
validation. Kandev persists a preparing creation operation so its own fenced
Runtime can finish or abort work after a process failure.

Callers never adopt, repair, reclaim, or complete another request's preparing
task. An observer cannot distinguish slow work from a crashed owner, so the
observable `FoundUnsettled` outcome remains diagnostic. Only the internal
Runtime may claim expired creation work and act from its durable manifest.

**`creation_complete` has exactly one meaning:** the returned task completed its
required synchronous creation manifest. It says nothing about agent liveness.
The tuple `deduplicated: true` plus `creation_complete: false` means another
create owns the identity and has not completed; callers proceed with that task
ID or escalate, never release and recreate automatically.

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
- Callers SHALL NOT delete, repair, resume, reclaim, or expire an unsettled task.
  Only a fenced internal creation-recovery actor may resume or abort it from the
  retained operation manifest.
- **REST** callers SHALL have a side-effect-free way to ask what holds an
  identity without risking a create: the lookup route. **MCP callers do not get
  one in this iteration.** What MCP gets is an idempotent *create-if-absent*
  operation — safe to repeat, but it creates a task when no holder exists, so it
  is not a probe. This asymmetry is deliberate and is stated rather than papered
  over; see *The probe, and what MCP has instead*.
- Callers SHALL release through a required stable operation ID, freeing the
  identity without deleting the task. Same-operation replay shall never affect a
  later holder. Release is a manual operator action, not recovery.
- Omitting the external ID SHALL preserve the existing public create behavior;
  internal creation durability and event ordering still use the shared protocol.
- The external ID SHALL be accepted on the REST create endpoint,
  `create_task_kandev`, Office runtime root/subtask create actions, and the
  authenticated `agentctl taskCreate` route. The WebSocket `task.create` action
  and plugin host task-create API do not accept it in this iteration.
- The external ID SHALL appear on exactly the task representations listed in
  *Task representations*. That table is the complete requirement; there is no
  broader "everywhere tasks are read" obligation.
- The external ID identifies the **external entity**, not the request body. A
  second create for a held external ID SHALL return the existing task unchanged
  even when the rest of the payload differs; it SHALL NOT patch the existing
  task.
- After workspace authorization and external-ID validation, REST, MCP, Office
  runtime, and authenticated agentctl create paths SHALL resolve a held identity
  before attachment, repository, workflow/step, profile, parent, policy,
  contribution, or launch validation. Server-state or payload drift shall not
  prevent a Found response for an authorized held identity.
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

## Requirements



### REQ-TASKS-EXTERNAL-ID-001: External task ID idempotency



**Intent:** A caller can retry task creation with an external identity and receive the one task that owns that identity without creating duplicate work.



#### Acceptance criteria



- **AC-TASKS-EXTERNAL-ID-001.1:** When a create carries an external ID, the
  system shall return the single task associated with the normalized byte-exact,
  case-sensitive identity and shall not create a duplicate. SQLite `BINARY` and
  PostgreSQL `"C"` column/index/lookup semantics shall be schema-attested before
  external-ID traffic is served.
- **AC-TASKS-EXTERNAL-ID-001.2:** When the returned task's creation is unsettled,
  the response shall identify that diagnostic state without authorizing caller
  identity release, completion, or duplicate creation.
- **AC-TASKS-EXTERNAL-ID-001.3:** When a request finds an existing identity,
  the system shall return the existing task with no task, attachment,
  repository, workflow, session, launch, event, or policy side effects.
- **AC-TASKS-EXTERNAL-ID-001.4:** After workspace authorization and external-ID
  validation, REST, MCP, Office runtime, and authenticated agentctl create paths
  shall perform identity lookup before all request-specific validation, so
  payload or server-state drift cannot replace a Found response with a validation
  error.
- **AC-TASKS-EXTERNAL-ID-001.5:** Only Created exposes a private handle bound to
  actor/token/CreationPlan. Required effects use durable typed step state,
  idempotency, evidence, unknown reconciliation, and compensation; proof gates
  Complete/Abort, and ambiguous Complete retries rather than Abort.
- **AC-TASKS-EXTERNAL-ID-001.6:** A stable release operation ID shall
  atomically clear a completed task identity, advance retained revision, and
  enqueue one immutable `task.updated`; replay returns its stored result without
  inspecting or affecting a later holder. Release of `completing` returns a
  typed conflict without mutating phase B. Workspace deletion fences a
  `preparing-release` row, stores `workspace_deleted` plus deletion generation,
  then clears the identity before live-row removal.
- **AC-TASKS-EXTERNAL-ID-001.7:** Releasing an identity from a preparing task
  shall record the release without publishing before `task.created`; subsequent
  Complete shall emit one creation event without the ID and return
  `CreatedIdentityLost`.
- **AC-TASKS-EXTERNAL-ID-001.8:** A fenced internal Runtime may resume or abort a
  preparing creation from its durable manifest after owner loss, but callers
  shall never adopt, repair, reclaim, complete, or automatically release it.

## Out of scope

External-ID support on WebSocket/plugin create and caller-driven automatic
repair, adoption, reclamation, completion, or release remain excluded.