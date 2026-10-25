---
id: "10-fanout-reporting"
title: "Participant fan-out reporting"
status: pending
wave: 10
depends_on: ["04-skipped-marker-state", "09-discard-diagnostics"]
plan: "plan.md"
requirements:
  - REQ-OFFICE-STEP-ENTRY-DISPATCH-007
acceptance_criteria:
  - AC-OFFICE-STEP-ENTRY-DISPATCH-007.1
  - AC-OFFICE-STEP-ENTRY-DISPATCH-007.2
  - AC-OFFICE-STEP-ENTRY-DISPATCH-007.3
  - AC-OFFICE-STEP-ENTRY-DISPATCH-007.4
  - AC-OFFICE-STEP-ENTRY-DISPATCH-007.5
  - AC-OFFICE-STEP-ENTRY-DISPATCH-007.6
  - AC-OFFICE-STEP-ENTRY-DISPATCH-007.7
  - AC-OFFICE-STEP-ENTRY-DISPATCH-007.8
  - AC-OFFICE-STEP-ENTRY-DISPATCH-007.9
  - AC-OFFICE-STEP-ENTRY-DISPATCH-007.10
  - AC-OFFICE-STEP-ENTRY-DISPATCH-007.11
system_design:
  - ../../specs/office/system-design/step-entry-dispatch-convergence.md
---

# Task 10: Participant fan-out reporting

## Summary

Replace `queue_run_for_each_participant`'s single joined error with a logger
that emits one ERROR per failed participant plus one INFO summary, and give the
three declaration faults a stated outcome and a terminal marker so a
configuration typo cannot leave a position permanently unrunnable.

## In scope

- The per-participant ERROR records and the one INFO summary, produced inside
  the callback rather than by its caller.
- Behavior for an empty role, a matching participant with no agent profile, and
  a role matching nobody, each with its marker outcome.
- The fan-out's marker outcome when it ends with its marker unset.
- Keeping `roleSeatsForFanOut`'s `Position ASC, AgentProfileID ASC` order
  unchanged, and a fixture whose agent-profile-id and row-creation orders
  disagree.

## Out of scope

- Changing the enumeration order. `review-participant-seats.md`
  AC-OFFICE-REVIEW-SEATS-003.3 binds this fan-out to ascending position then
  ascending agent profile identifier, and nothing here amends it.
- Re-introducing the `role ASC` leading key, since role is constant after
  filtering.

## Acceptance

- A reader can tell which participant failed, how many matched, and whether the
  fan-out matched nobody.
- Each of the three declaration faults leaves the position terminal rather than
  permanently unrunnable.
- The order test fails if the tiebreak is moved to the participant row id.

## Verification

```bash
cd apps/backend && go test ./internal/workflow/engine/... ./internal/orchestrator/...
```

## Files likely touched

- `apps/backend/internal/workflow/engine/phase2_callbacks.go`
- `apps/backend/internal/workflow/engine/phase2_callbacks_test.go`

## Dependencies

Tasks 04 and 09.

## Risks

- The existing fixture is lexically pre-sorted, so it passes under either
  tiebreak and would not catch a change to the enumeration order.
- Participant de-duplication on `(role, agent_profile_id)` and
  collect-and-continue failure isolation are already implemented; re-implementing
  either would regress verified behavior.

## Parallelism

`sequential`

## Inputs

- System design, "Diagnostics" and "Enumeration order".
- `review-participant-seats.md` AC-OFFICE-REVIEW-SEATS-003.3.

## Results

Pending.
