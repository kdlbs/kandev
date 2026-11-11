---
status: current
system: tasks
created: 2026-11-10
requirements:
  - REQ-TASKS-COMPLETION-OUTCOMES-001
  - REQ-TASKS-COMPLETION-OUTCOMES-002
owners:
  - kandev
---

# Workflow completion outcomes

## Boundary and requirement mapping

The tasks system owns signal identity and proof of its transition.
The [entry guard](workflow-explicit-completion-signal.md#entry-bound-completion) remains authoritative for step ownership.
The [carry mechanism](workflow-completion-signal-payload-delivery.md) remains authoritative for prompt delivery.
The [profile routing contract](workflow-step-fixed-profile-routing.md) selects the destination session.

| Requirement | Sections |
| --- | --- |
| REQ-TASKS-COMPLETION-OUTCOMES-001 | Source identity, claim, commit guard, reconciliation |
| REQ-TASKS-COMPLETION-OUTCOMES-002 | Receipt, result projection, compatibility |

## Current implementation

`mcp/handlers.stepCompletionLaunchStep` returns the latest turn's ID with its immutable step and entry stamps.
`PendingStepCompletionSignal` carries `signal_id` and `source_turn_id`.
`sqlite.Repository.ClaimStepCompletionSignal` deduplicates by source turn and workflow entry and replaces an unconsumed earlier source with the current persisted turn.
`handleDuplicateStepComplete` returns the retained receipt, including after bag cleanup.

`prepareOnTurnStartSession` clears the bag for a new Kandev user prompt.
Canonical native output can create another turn through `ProjectCanonicalAgentDeliveryEvent` without that prompt path.
The later turn's completion therefore replaces the earlier pending source instead of inheriting its handoff.

`writeTaskReviewState` projects idle work into generic task state `REVIEW`.
It does not select the workflow step named Review.
Credential preflight, absent move actions, and other admission guards can legitimately leave an idle task in Build.

## Source identity and claim

Extend `stepCompletionLaunchStep` to return the selected turn ID with its immutable step and entry stamps.
Capture this tuple once at authenticated backend admission.
Do not add an agent-supplied identity argument or replace captured identity after a wait.

Add `signal_id` and `source_turn_id` to `PendingStepCompletionSignal`.
Mint the signal ID only for a winning claim, or discard a losing candidate ID.
The idempotency key is `(task_id, session_id, source_turn_id, workflow_entry_id)`.
Payload equality and `SignaledAt` do not determine identity or order.

Extend `ClaimStepCompletionSignal` within its existing task-write transaction:

1. Validate ownership and the expected step and entry.
2. Recheck the captured source against the session's current persisted turn.
3. Return the stored signal and receipt for the same source key.
4. Replace an unconsumed earlier-turn signal when the captured source is still current.
5. Write the signal and its pending receipt together.

A later native verification turn alone does not replace the pending signal.
Only another accepted completion or the existing user-message reopening path changes its ownership.
Consumption therefore checks the stored signal ID, rather than requiring its source to remain the latest output turn.

Keep the existing lock order.
Task-first claim and move paths must not acquire the session-turn advisory lock after the task lock.
Use the task boundary already shared with stamped turn creation, and test concurrent creation on both databases.
If a path cannot establish consistent turn evidence, reject it rather than adding a reverse lock order.

## Bounded receipt

Add one `last_step_completion_receipt` key to source-session metadata.
Use a typed model with a version, signal ID, source tuple, accepted timestamp, outcome, and optional committed transition.
Keep no array of receipts, payload copies, or chat transcript.
A newer accepted source replaces this slot.
An older request without a matching retained receipt receives a stale or unproven result, never another transition.

Outcomes are `pending`, `blocked`, `not_configured`, `superseded`, `transition_applied`, and `destination_queued`.
Safe reason codes describe blocked and superseded results.
The receipt's committed transition contains the immutable ledger ID and destination workflow and step IDs.

The slot survives pending-bag consumption and ordinary turn-start cleanup.
Existing session deletion and metadata retention own its lifetime.
No SQL table, column, migration, public argument, or session-turn metadata write is required.

## Commit guard and proof

Extend `WithWorkflowStepCompletionGuard` to carry the source session and exact signal ID.
In `taskStepTransitionGuardMatches`, check the persisted bag under the existing task transaction.
A removed or replaced signal cannot authorize the prepared transition.
Preserve the independent task-entry fence.

Both legacy `executeStepTransition` and engine `applyEngineTransitionWithCommitResultMode` use this guard.
They must capture carry text and audit context from the same guarded signal.
`workflowStore.carryStepHandoffForTransition` must not recapture a different signal after evaluation.

In the transaction that records `task_step_transitions`, update the matching receipt with that returned transition ID.
Include WIP admission in the outcome.
Do not derive proof from the later `session_step_history` write, which remains best-effort telemetry.
If essential receipt persistence fails, roll back the move and leave the signal retryable.
This proof write is separate from the carry contract's best-effort auxiliary operations.

Only the winning commit can publish destination lifecycle work.
Bag cleanup remains conditional on the exact observed JSON value.
Blocked-outcome writes also compare signal ID, so a late error cannot mark a replacement as blocked.

## Reconciliation

`onStepCompletionSignaled`, turn-end processing, and the watchdog continue to read durable state.
Bus events are wakeups and carry no handoff authority.
After a superseded commit attempt releases its guards, give the current pending signal another reconciliation opportunity.
Use the existing event or watchdog machinery, without an unbounded retry loop.

A duplicate pending signal can republish after a notification error.
A duplicate committed signal returns its receipt and publishes no transition or prompt.
A transient commit or essential proof error releases the turn-completion generation consumed by that exact attempt, so the same durable signal remains retryable without a newer session snapshot.
If an exact receipt exists, allow this read-only result before the terminal-session new-signal rejection.
Authorization still runs first.

A credential or executor refusal leaves the source route intact and records a safe blocked reason.
It never selects the Build session as the Review runner to make progress.
Destination queueing reports the committed destination and scheduling state without claiming reviewer dispatch.

## Result projection

Retain the existing top-level fields.
On duplicates, add `step_id` and `signaled_at` from the stored signal identity.
Add these fields to accepted and duplicate responses:

| Field | Content |
| --- | --- |
| `signal_id` | Stable ID of the stored signal |
| `source` | Session, turn, source step, and source entry |
| `outcome` | Receipt status and optional safe reason code |
| `transition` | Exact committed ledger ID and destination, or absent |
| `current` | State, workflow, step, entry, primary session, and queue placement |

Read `current` from one statement-consistent repository snapshot after publication.
It covers state, workflow, step, latest entry, queue placement, and primary session together.
It describes observation time and can differ from the receipt's historical destination.
Do not splice a pre-claim task row into newer entry or session reads.
Do not infer transition success from `REVIEW`, `advances:true`, or a primary-session change.
`transition_applied` proves the database move only.
It does not prove that a reviewer received a prompt.

`mcp/server.stepCompleteHandler` already forwards the result map.
Keep that behavior and add serialization coverage.
Update tool and system-prompt guidance to explain the receipt and asynchronous outcome.
Keep the final-action rule, question barriers, and stale-turn recovery instruction.

## Compatibility and security

The no-turn compatibility path keeps first-signal-wins for one entry.
It reports an absent source turn explicitly and does not invent history.
A fresh attributed turn can replace a legacy pending signal that has no proven turn identity.
Historical records without a receipt cannot acquire fabricated transition proof.
Keep the lossless entry parsing and A-to-B-to-A rejection from the previous repair.

Same-turn duplicate text remains ignored, including changed or oversized handoff and blockers.
Duplicate results report no truncation of discarded text.
The response extension does not broaden task access, MCP mode visibility, or executor permissions.
Identifiers belong in structured logs, never metric labels.
Count a new accepted source once and count no duplicate as another acceptance.

## Validation and related records

The [work package](../../../plans/step-completion-outcomes/plan.md) maps criteria to named regressions and exact commands.
Service-level acceptance tests observe a distinct configured Review recipient and its actual prompt.
They cover the latest native correction, Review request-changes through Iterate and a second Review entry, queued promotion after repository reload, and a real credential refusal that keeps the source route.

- [Source-specific completion decision](../../../decisions/2026-11-10-source-specific-step-completion.md)
- [ADR 0015](../../../decisions/0015-explicit-completion-signal-for-auto-advance.md)
- [Entry-bound delivery turns](../../../decisions/2026-10-22-entry-bound-delivery-turns.md)
