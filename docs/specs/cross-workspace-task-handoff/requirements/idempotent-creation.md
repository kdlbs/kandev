---
status: active
system: cross-workspace-task-handoff
created: 2026-09-02
updated: 2026-09-15
owners:
  - nova28
---

# Idempotent creation and settlement Requirements

## Overview

This action reuses the platform's existing external-id create-idempotency
contract unchanged — see
[External task ID idempotency](../../tasks/requirements/external-id-idempotency.md)
and its boundaries document — rather than restating, reinterpreting, or
extending it. What this requirement adds is specific to a handoff: settling
the identity itself on the created path (something the shared create call
cannot do on its own), and reporting partial failure and retry semantics the
caller needs when a resource — the delivery task — has already been created
and must not be lost.

### REQ-CROSS-WORKSPACE-TASK-HANDOFF-IDEMPOTENCY-001: Outcome reporting, settlement, and partial-failure results

#### Acceptance criteria

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-IDEMPOTENCY-001.1:** When `external_id`
  is supplied, the action shall obtain its outcome from the shared
  idempotency mechanism unchanged and shall surface which of that mechanism's
  four outcomes occurred, together with its `creation_complete` signal,
  unchanged in meaning. A single success/failure boolean shall not be used in
  its place: the "created but not yet settled" outcome is diagnostic and
  cannot be collapsed into "already existed" without discarding a
  safety-critical signal.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-IDEMPOTENCY-001.2:** On either outcome
  where the delivery task was found rather than newly created, the action
  shall perform no target-side work: no second task, session, launch,
  repository attachment, workspace-policy write, or task-created event. This
  is exhaustive, not illustrative: the reverse-link repair defined in the
  reverse-link-integrity requirements, and the activity entries defined in
  the provenance requirements — including the target-side one — are
  unaffected by this rule and are still performed, because they record that
  a call happened rather than mutate the returned task's state.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-IDEMPOTENCY-001.3:** On the
  found-but-unsettled outcome, the action shall return the task with
  `creation_complete: false`, shall not release the identity, shall not
  create a second task, and shall not wait, poll, or retry internally for
  settlement. The response message shall state that the delivery task
  exists, that another create may still be finishing it, and that the safe
  responses are to proceed with the returned id or escalate to a human.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-IDEMPOTENCY-001.4:** On the
  identity-lost outcome, the action shall report that outcome explicitly and
  state that the delivery task exists but no longer holds the external id,
  so an identical replay would create a second task; the caller is to record
  the returned id rather than replay.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-IDEMPOTENCY-001.5:** On the newly-created
  path, the action shall settle the external id itself, because the shared
  create call cannot: settlement is decided after that call returns. The
  action shall therefore settle unconditionally on this path — including when
  `external_id` was omitted, in which case settlement is a no-op that reports
  settled — after the create and before any launch dispatch, and shall map
  the result as follows:
  - settled: outcome is "created" and `creation_complete` is true;
  - not settled: outcome is "identity lost", `creation_complete` is true,
    the surviving task is the one the settlement call returns, and no launch
    shall be dispatched;
  - a settlement error: HTTP 500. The delivery task shall not be deleted, and
    the error message shall carry the delivery task's id — the one case
    where a 500 response names a resource — because the task already exists
    and withholding its id would lose it permanently, which this settlement
    step exists to prevent.

  Omitting this settlement step is silent and load-bearing: a task left
  unsettled after its first create makes every later replay report the
  found-but-unsettled outcome forever, which is the one outcome this
  requirement tells the caller to escalate rather than trust.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-IDEMPOTENCY-001.6:** When the delivery
  task exists but its source-side reverse-link write fails — whether
  immediately after creation or during a later repair — the action shall
  return a non-error result carrying the full response object (see the
  start-semantics requirements) with `reverse_link_recorded: false` and a
  `reverse_link_error` message, rather than an error result, and shall not
  delete the delivery task. The delivery task's id is the single most
  important thing the caller must not lose, and a plain error response
  cannot carry it as reliably as a structured field. The error message shall
  instruct an identical replay when `external_id` was supplied — which
  becomes a repair — and, when it was not, shall say so and warn that a
  replay would create a second task.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-IDEMPOTENCY-001.7:** When `external_id`
  is omitted, outcome shall be "created" and `creation_complete` shall be
  true on every successful call; the found and identity-lost outcomes are
  reachable only when `external_id` was supplied.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-IDEMPOTENCY-001.8:** When `external_id`
  is omitted, the call is not idempotent, and the command's help text and the
  CEO role instructions shall say so and shall direct the caller to derive a
  stable `external_id` from the deciding artefact when a retry is possible.
  The action shall not invent an id from `title`, which changes freely
  between attempts, and shall not generate one itself.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-IDEMPOTENCY-001.9:** Two concurrent calls
  carrying the same `external_id` and the same `target_workspace_id` shall
  produce exactly one delivery task, and both callers shall receive that
  task's id. The winner reports "created"; the loser reports whichever found
  outcome it observed and shall still receive the reverse-link repair
  described in the reverse-link-integrity requirements. Neither caller shall
  receive an error on account of losing the race.

## Out of scope

Any change to the shared external-id mechanism: its four outcomes, its
no-side-effect rule for found outcomes, its refusal to detect liveness, and
its per-workspace uniqueness are consumed as-is. This requirement works
around the absence of cross-workspace uniqueness (see
reverse-link-integrity requirements); it does not add cross-workspace or
global uniqueness.
