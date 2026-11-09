---
id: "02-reproduce-correction-handoff"
title: "Reproduce same-entry correction and handoff ordering"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-001
  - REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-003
  - REQ-TASKS-SIGNAL-PAYLOAD-DELIVERY-001
  - REQ-TASKS-SIGNAL-PAYLOAD-DELIVERY-002
acceptance_criteria:
  - AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-001.1
  - AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-003.3
  - AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-003.6
  - AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-003.7
  - AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-001.10
  - AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-001.13
  - AC-TASKS-SIGNAL-PAYLOAD-DELIVERY-002.1
system_design:
  - ../../specs/tasks/system-design/workflow-explicit-completion-signal.md
  - ../../specs/tasks/system-design/workflow-completion-signal-payload-delivery.md
---

# Task 02: Reproduce same-entry correction and handoff ordering

## Summary

This is a bounded investigation prerequisite for #5173, not a corrective patch.
Reconstruct the admitted input, turn, workflow entry, signal, transition, and
handoff identities before selecting a repair. It does not delay #5178.

## In scope

- Recheck issue ownership and linked PRs. Keep #5173 unassigned until its
  independent cause and fix reach the required certainty.
- Use a fresh SQLite fixture and disposable Go overlays. Trace both engine and
  legacy completion paths, the waiting-session subscriber, and repository CAS.
- Record each event's task/session, user or peer operation, turn ID, immutable
  step/entry stamp, signal time/payload, committed transition, destination
  recipient, and handoff claim. Keep runtime state separate from workflow step.
- Start with the exact three-turn report shape: first correction, later
  additive correction, then read-only verification. Vary only one input route
  or event ordering at a time. Compare direct message, queued message, and
  admitted peer input. do not assume each route runs the same reset.
- Cross a completion claim with new input before transition commit. Cover
  duplicate/reordered turn-end and signal events, cancellation, stale entry
  stamps, A -> B -> A, failed metadata clear, and superseded turns.
- Inspect whether the configured Build step actually has a runnable automatic
  move. Include a control with no move: `REVIEW` runtime state plus Build
  workflow step is then a valid result.
- For a genuine destination transition, use a different Review profile/session.
  Compare committed step/entry, primary recipient, claimed carry token,
  persisted prompt and dispatched handoff. Preserve independent Review isolation.
- Limit the initial pass to those routes and two deterministic race barriers:
  claim versus new input, and transition commit versus destination dispatch.
  Stop after that matrix if no failing invariant emerges.

## Out of scope

- Disabling idempotency, overwriting every pending signal, inventing turn
  stamps, forcing a task move, or amending a consumed destination handoff.
- Production changes, permanent regression tests, response-schema changes,
  automatic repair of historical tasks, and live user records in this prerequisite.
- Assuming `already_signaled` proves a committed transition, or assuming that
  three normally completed native turns prove three new server-admitted inputs.

## Acceptance

1. A deterministic trace identifies either the failing invariant or the exact
   missing evidence. It distinguishes same-entry duplicate retry from a later
   admitted correction and distinguishes runtime `REVIEW` from Review entry.
2. Duplicate controls preserve the first payload and permit at most one
   transition. Stale entry evidence is rejected. A successful turn-start reset
   permits a later correction. A delayed source event cannot alter Review.
3. The result adds evidence and a decision to this package. A proven regression
   gets a separate proposed repair work order and precise test. A new response
   or supersession contract requires an amended design before implementation.
   No failure means no invented patch or issue assignment.

## Verification

Existing controls:

```bash
(cd apps/backend && go test -trimpath -race ./internal/mcp/handlers -run '^TestHandleStepComplete_(FirstCallAccepted|DuplicateCallDoesNotOverwriteHandoffOrBlockers|DedupWaitingRepublishes|ConcurrentCallsClaimOneSignal|StaleTurnRecoveryGuidance|FreshTurnAfterStepChange)$' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/task/repository/sqlite -run '^TestStepCompletionEntry' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/orchestrator -run '^(TestStepCompletionEntryHandoff|TestProcessOnTurnComplete_ConcurrentDuplicateCalls_OnlyOneTransitionApplies|TestOnStepCompletionSignaled|TestDeliveryFallbackStepCompletionStaleEntry)$' -count=1)
```

Planned disposable reproduction, from the repository root:

```bash
(cd apps/backend && go test -trimpath -race -overlay /tmp/workflow-correction-probe/overlay.json ./internal/orchestrator ./internal/mcp/handlers -run '^TestCorrectionHandoffProbe' -count=1 -v)
git diff --check -- docs/plans/workflow-prompt-handoff-integrity
```

Create the temporary overlay and its fixtures as part of the investigation.
The overlay path is not a checked-in tool. Remove it after recording the
trace. If evidence implicates dialect-sensitive claims, the subsequent repair
must also name and run the existing PostgreSQL variants with an isolated DSN.
Their absence is a limit, not a pass.

## Files likely touched

Only `evidence.md`, `plan.md`, and this work order for the result.
Disposable overlays target these source/test boundaries:

- `apps/backend/internal/mcp/handlers/handlers.go` and `step_completion_entry.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`,
  `event_handlers_step_completion.go`, and `step_completion_entry.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming.go`,
  `event_handlers_agent.go`, and `peer_message_initial_launch.go`
- `apps/backend/internal/task/handlers/message_handlers.go`
- `apps/backend/internal/task/repository/sqlite/step_completion_entry.go`,
  `step_entry_dispatch.go`, and `task_queue_promotion_claim.go`

## Dependencies

None. Run independently of Tasks 01 and 03, sequentially in this session.
The issue's original workflow definition and admitted-input chronology are
evidence gaps, not permission to read a live deployment.

## Risks

- The current claim uses step/entry identity, not a turn identity. The normal
  reset enables later input, but a clear failure or bypass can leave old data.
- A source correction after a committed transition belongs to a different
  admission decision. It cannot silently replace a Review prompt already sent.
- The payload specification remains `draft` although current source implements
  carry tokens. Its first-signal rule cannot be silently rewritten.
- The requested structured destination response is not currently implemented
  or sufficiently designed. Record its need without guessing field semantics.

## Parallelism

`sequential`

## Inputs

- [Evidence and remaining hypotheses](evidence.md#issue-5173-unconfirmed).
- [Entry-bound completion](../../specs/tasks/system-design/workflow-explicit-completion-signal.md#entry-bound-completion).
- [Payload delivery design](../../specs/tasks/system-design/workflow-completion-signal-payload-delivery.md).
- [ADR 0015](../../decisions/0015-explicit-completion-signal-for-auto-advance.md)
  and [entry-bound attribution](../../decisions/2026-10-22-entry-bound-delivery-turns.md).

## Results

Started on 2026-11-08 as a bounded investigation, not a patch. The planned
three-turn route matrix and both deterministic race barriers remain unrun, so
this work order is incomplete rather than done.

The existing controls ran with the race detector:

```bash
(cd apps/backend && go test -trimpath -race ./internal/mcp/handlers -run '^TestHandleStepComplete_(FirstCallAccepted|DuplicateCallDoesNotOverwriteHandoffOrBlockers|DedupWaitingRepublishes|ConcurrentCallsClaimOneSignal|StaleTurnRecoveryGuidance|FreshTurnAfterStepChange)$' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/task/repository/sqlite -run '^TestStepCompletionEntry' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/orchestrator -run '^(TestStepCompletionEntryHandoff|TestProcessOnTurnComplete_ConcurrentDuplicateCalls_OnlyOneTransitionApplies|TestOnStepCompletionSignaled|TestDeliveryFallbackStepCompletionStaleEntry)$' -count=1)
```

All passed. Together with the planning probe recorded in
[the evidence](evidence.md#issue-5173-unconfirmed), the controls establish:

- A duplicate claim at the same entry retains the first handoff.
- A normal turn-start reset clears the pending bag and allows a later
  same-entry claim with the latest correction.
- Stale turn/entry evidence is rejected, and concurrent duplicate turn
  completions apply at most one transition.

No failing invariant emerged. The remaining facts are still missing: the
reported workflow move configuration, exact turn and entry stamps, admitted
operation IDs, signal timestamps, transition claim outcome, carry token, and
destination dispatch identity. Treating `already_signaled` as proof of a
committed Review transition remains unsupported. Runtime state `REVIEW` with a
Build workflow step also remains a valid mapping when no runnable automatic move
exists.

Decision: do not invent a same-entry supersession or structured-response
contract, do not patch completion behavior, and keep #5173 unassigned. A later
repair needs the missing chronology or a new deterministic reproduction. No
production or permanent test change was made by this work order.
