---
id: "03-hold-signal-advance-for-entry-prompt"
title: "Hold a signal-gated advance until the step's entry prompt is delivered"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-STAGE-MOVES-003
acceptance_criteria:
  - AC-TASKS-WORKFLOW-STAGE-MOVES-003.1
  - AC-TASKS-WORKFLOW-STAGE-MOVES-003.2
  - AC-TASKS-WORKFLOW-STAGE-MOVES-003.3
  - AC-TASKS-WORKFLOW-STAGE-MOVES-003.4
  - AC-TASKS-WORKFLOW-STAGE-MOVES-003.5
system_design:
  - ../../specs/tasks/system-design/workflow-stage-moves.md
---

# Work Order 03: Hold a signal-gated advance until the step's entry prompt is delivered

## Outcome

A capacity-deferred step entry prompt bound to the current entry is kept, not
discarded, while its destination session is running. A signal-gated step does
not advance on a completion signal while that prompt is still queued and
valid.

## In scope

- `apps/backend/internal/orchestrator/ceiling_entry.go`:
  `validateCeilingDestination` returns `ceilingEntryUnavailable` for a bound
  record on a `STARTING` or `RUNNING` destination. An unbound record keeps
  `ceilingEntrySuperseded`.
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`:
  `allowEngineSignalCompletion` takes the task and holds the advance while a
  bound `prompt_ensure` or `workflow_step_ensure` record for the current step
  is valid, unavailable, or cannot be validated.

## Out of scope

- The completion-signal response contract and `advances` field.
- Ceiling admission order and replay retry policy.

## Acceptance conditions

1. A bound entry prompt on a running destination is `ceilingEntryUnavailable`.
   An unbound one stays `ceilingEntrySuperseded`.
2. A signal-gated step with a valid queued entry prompt does not advance and
   leaves the session `WAITING_FOR_INPUT`. The same step advances when no
   entry prompt is pending.
3. A superseded or delivered entry prompt, and a user-cancelled turn, keep the
   existing behavior.

## Verification

Add focused tests in a new test file beside `ceiling_entry.go`. Run them before
the fix (expected to fail) and after it:

```bash
cd apps/backend
go test ./internal/orchestrator/ -run 'ValidateCeilingEntry|PrepareEngineTurnCompletion|SignalCompletion' -count=1
```

## Likely files

See In scope. Dependencies: none.

## Results

- Tests in `workflow_entry_before_advance_test.go`. Before the fix,
  `TestValidateCeilingEntry_BoundEntryPromptOnRunningDestinationIsRetried` got
  `ceilingEntrySuperseded` and
  `TestPrepareEngineTurnCompletion_HoldsAdvanceWhileBoundEntryPromptPending`
  advanced past the queued prompt. Both pass after the fix. The unbound,
  superseded-destination, and release-event tests pass before and after, which
  confirms the preserved paths.
- The hold has no age limit. See "How a hold ends" in the system design.
- `TestCeilingDispatchAdmissionSerializesRouteMutation` fails intermittently
  on Windows at its claim-expiry time comparison, with and without this
  change. It is excluded from this order's evidence.

- Review remediation adds `TestPrepareEngineTurnCompletion_HoldsAdvanceWhenEntryBindingUnreadable`:
  incomplete and malformed bindings remain pending until ceiling recovery settles
  their record. The exact-source contained gate proof failed both assertions
  before remediation. Genuine repository and orchestrator verification remains
  owned by hosted Backend CI; the contained proof fakes storage and validation.
