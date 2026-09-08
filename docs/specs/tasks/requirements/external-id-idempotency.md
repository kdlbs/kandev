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

## Requirements



### REQ-TASKS-EXTERNAL-ID-001: External task ID idempotency



**Intent:** A caller can retry task creation with an external identity and receive the one task that owns that identity without creating duplicate work.



#### Acceptance criteria



- **AC-TASKS-EXTERNAL-ID-001.1:** When a create request carries an external ID, the system shall return the single task associated with that identity and shall not create a duplicate task.
- **AC-TASKS-EXTERNAL-ID-001.2:** When the returned task's creation is unsettled, the response shall identify that diagnostic state without authorizing identity release or duplicate creation.
- **AC-TASKS-EXTERNAL-ID-001.3:** When a request finds an existing identity, the system shall preserve the no-side-effect contract defined by the requirement.



## Out of scope



Crash repair, adoption, resume, cleanup, and caller-side identity release are excluded from this capability.