---
status: draft
system: office
created: 2026-09-02
owners:
  - kandev
---

# Step Entry Run Coalescing Requirements

## Overview

`shouldCoalesceRun` excludes only task-comment keys from the 5-second
coalescing window. A run enqueued by a step-entry action therefore merges into
a still-queued run for the same `(agent_profile_id, reason)`, and a fast
reject-then-reenter round produces no runnable run at all: the task stalls with
a queue that looks satisfied.

This requirement is separated from
[`step-entry-dispatch-convergence.md`](step-entry-dispatch-convergence.md),
which discovered it, for the reason
[`step-entry-recovery-scan.md`](step-entry-recovery-scan.md) is separate: the
contract is not specific to convergence. It governs every run an entry sequence
enqueues, from either dispatcher, under any workflow, and it changes the runs
queue rather than the dispatchers. Its acceptance criteria keep the
`AC-OFFICE-STEP-ENTRY-DISPATCH-005.*` identifiers they were authored under, so
existing citations continue to resolve.

## Terminology

Terms defined in
[`step-entry-sequence-execution.md`](step-entry-sequence-execution.md) -
**arrival**, **step entry**, **entry identity** - carry the same meaning here.

- **Coalescing window:** the 5-second interval within which the runs queue
  merges a new request into a still-queued run sharing its
  `(agent_profile_id, reason)`.
- **Entry-triggered run:** a run enqueued by an action in a step's declared
  entry sequence, as opposed to one enqueued by a comment, an event trigger, or
  an operator action.

## Requirements

### REQ-OFFICE-STEP-ENTRY-DISPATCH-005: Entry-triggered runs bypass coalescing

**Intent:** An entry sequence's run reaches the agent as its own run, so a
round that rejects and re-enters is not silently collapsed into the round
before it.

#### Acceptance criteria

- **AC-OFFICE-STEP-ENTRY-DISPATCH-005.1:** A run enqueued by any step-entry
  action shall be excluded from the coalescing window, for both
  `queue_run_for_each_participant` and `queue_run`.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-005.2:** The exclusion shall be carried by a
  discrete field on the queue request, read by both the service-side coalescing
  predicate and the repository-side SQL exclusion. It shall not be encoded in
  the idempotency key, and shall not reuse the task-comment prefix. Because the
  repository-side exclusion tests an already-queued row rather than the incoming
  request, the field shall be persisted on the run row and read back from it; an
  in-memory request field alone shall not satisfy this. A row written before the
  field existed shall read as not-excluded.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-005.3:** The request type is declared in both
  `internal/runs/service` and `internal/workflow/engine` and the two must
  match; the field shall be added to both.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-005.4:** Idempotency suppression shall be
  unaffected by this exclusion and shall continue to apply.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-005.5:** A test shall enqueue two runs for
  the same agent profile and reason, from two entries, inside the coalescing
  window, and assert two runnable rows exist.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-005.6:** Before any statement naming the
  persisted exclusion column is issued, the system shall probe that the column
  exists. When it does not, it shall log one ERROR naming the table and the
  column and fall back to the coalescing behaviour that preceded this
  requirement, leaving the exclusion inactive; it shall not issue a statement
  against a missing column, which would fail every run enqueue and not only the
  exclusion. This is required because the migration helper records a failed
  `ALTER` at WARNING and continues, so a column can be absent at runtime without
  startup failing. A test shall assert the fallback against a store lacking the
  column.

## Out of scope

- **The 5-second coalescing window's value.** It keeps its current value for
  every run that is not entry-triggered, and does not become configurable.
- **Coalescing for other run sources.** Comment-triggered, event-triggered, and
  operator-triggered runs keep today's behaviour; only entry-triggered runs are
  excluded.
- **Idempotency suppression.** Per AC-OFFICE-STEP-ENTRY-DISPATCH-005.4 it is
  unchanged; the 24-hour idempotency window is untouched by this requirement.
- **Backfilling the persisted column.** Rows written before the column existed
  read as not-excluded (005.2); no migration rewrites historical runs.

## Prior art

Carried from
[`step-entry-dispatch-convergence.md`](step-entry-dispatch-convergence.md),
whose Prior art section records the wiki and saas-kb legs in full. The
load-bearing position is `concepts/agent-replay-non-idempotence.md` (0.91,
`lifecycle: draft`): re-running an *author* is not idempotent. Coalescing is the
inverse failure - not a duplicated agent pass but a dropped one - and the same
reasoning applies: the runs queue may suppress a run only when it can show the
work already ran, which idempotency can establish (005.4) and a time window
cannot.
