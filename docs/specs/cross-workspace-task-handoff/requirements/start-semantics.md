---
status: active
system: cross-workspace-task-handoff
created: 2026-09-02
updated: 2026-09-15
owners:
  - nova28
---

# Start semantics and the response contract Requirements

## Overview

Whether the delivery agent actually started is reported, never assumed. The
default is not to launch, because a cross-workspace card starting immediately
in a workspace the caller does not run in is the riskiest available default,
and a launch is only ever reported as attempted when it was genuinely
dispatched and its result observed.

### REQ-CROSS-WORKSPACE-TASK-HANDOFF-START-001: Reported start, and the response object

#### Acceptance criteria

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-START-001.1:** `start_agent` shall
  default to false, diverging from same-workspace task creation's default of
  true; the flag's help text and the CEO instructions shall state the
  divergence and its reason.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-START-001.2:** The response shall report
  `started: true` if, and only if, all three hold: `start_agent` was true;
  the create outcome was "created" (not found, and not identity-lost); and
  the launch dispatched for the delivery task returned without error. That
  return is the only linearization point — `started` shall never be derived
  from the destination step's own configuration or inferred from anything
  the action did not itself observe. `started: true` asserts only that the
  launch call returned no error; it does not assert that the executor is up,
  a session is ready, or a first prompt was accepted.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-START-001.3:** The launch shall be
  dispatched synchronously, bounded by the platform's existing agent-launch
  timeout, and its result observed before the response is composed —
  fire-and-forget dispatch is not sufficient, because it would make this
  criterion trivially true and a launch failure unreachable. When the launch
  is attempted and fails, including a nil launcher or the timeout elapsing,
  `started` shall be false and the response shall carry a `start_error`
  naming the failure; the delivery task shall not be deleted and the call
  shall not become an error response.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-START-001.4:** On either found outcome,
  and on the identity-lost outcome, `started` shall be false and no launch
  shall be attempted; neither case shall carry a `start_error`, because
  nothing was attempted.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-START-001.5:** Whether the source-side
  reverse-link write succeeded is not a fourth condition on `started`. A
  created call with `start_agent: true` whose reverse-link write failed
  shall still dispatch the launch and report `started` on the three
  conditions in criterion .2 alone: `reverse_link_recorded: false` together
  with `started: true` is a correct and expected combination, not a
  contradiction, because the delivery task's forward provenance is already
  durable at creation and the reverse link is a repairable source-side index
  (see the reverse-link-integrity requirements), not a precondition for the
  card to run.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-START-001.6:** `start_agent: false` shall
  be honoured at create time even when the resolved destination step carries
  an auto-start `on_enter` action: this action shall not stamp the marker
  that would make such a step launch on creation. A delivery task may still
  be launched later by the target workflow — for example when someone moves
  the card onto an auto-start step — but that happens after this call has
  returned, is the target workflow's own behaviour, and `started` shall never
  attempt to predict or report it.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-START-001.7:** The response shall be a
  single object carrying at least: the delivery task id; the target
  workspace id; the workflow id and step id the task is associated with; the
  create-idempotency outcome and `creation_complete` signal; `started`;
  `reverse_link_recorded`; and the handoff timestamp. On a found outcome,
  the workflow id, step id, and handoff timestamp describe the task that
  already exists — read at response time — not the arguments the replaying
  call supplied, because per-workspace `external_id` uniqueness is not keyed
  on workflow and a replay's step or workflow arguments may no longer match
  where the found task actually is. A divergence between what the caller
  asked for and what is reported on a found outcome shall not itself be a
  refusal.

  `reverse_link_error` shall be present only when `reverse_link_recorded` is
  false. `start_error` shall be present only when `started` is false and a
  launch was actually attempted; `started: false` with no `start_error` is
  the correct shape whenever no launch was attempted at all — every found
  outcome, the identity-lost outcome, and any call made with
  `start_agent: false`. No separate boolean duplicating `outcome` shall be
  added to this object.

## Out of scope

Predicting or reporting a launch performed later by the target workflow, once
a delivery card has been moved onto an auto-start step after this call has
already returned.
