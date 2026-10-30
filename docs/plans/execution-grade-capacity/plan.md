---
status: done
created: 2026-10-23
requirements:
  - REQ-AGENTS-SESSION-CEILING-003
system_design:
  - ../../specs/agents/system-design/session-concurrency-ceiling.md
---

# feat(capacity): add opt-in total and execution-grade limits

Opt into all-session and explicit execution-grade capacity with existing reservation, FIFO replay and queue presentation contracts.

## Delivery order and dependencies

The implementation is one existing PR slice. Preserve its native dependencies and existing feature defaults, then verify the narrow affected paths. No dependent PR is imported into this package.

- [Implementation and verification](task-01-implementation.md)

## Scope and risks

Changing disabled defaults, assigning grades from profile names, or changing dynamic candidate order. These remain outside this delivery slice. Preserve existing files, configuration and history; report unavailable integration coverage without inventing a successful result.

## Verification strategy

Reuse the completed focused implementation checks recorded below and the existing documentation-coverage evaluator. Only remaining review fixes require their affected checks to run again.

## ASCII UI preview

UI-01: Existing affected surface, after this change.

```text
Task Behavior > Session capacity
  Policy: automatic | all
  Total limit: [ ]
  Grade             Limit
  control           [ ]
  work              [ ]

The existing section stacks on phone; controls retain their labels and save action.
```

This sketch records the affected structure; spacing is illustrative. It does not add a new control or approval requirement.


## Review follow-up

The actual capacity-setting update reproduced two lost-ownership cases when a
held grade was removed: an existing reservation and an already-counted session
handoff were refused as new admission. A real queue projection also lost its
configuration refusal reason when the latest population was known to be zero.

Existing ownership now receives its slot credit before new-admission grade
configuration checks. A failed or unknown population read remains unknown;
new launches still refuse missing configuration or unknown population. Existing
new-request refusal reason precedence is preserved. Under the all-session
policy the queue keeps its persisted configuration refusal alongside fresh
counts; changing to automatic policy does not carry that warning forward.

The disposable checks cover both ownership cases, held ownership during a
population read failure, unknown-grade refusal precedence, a zero-population
queue, policy change and unknown-capacity omission. They are run through an
explicit Go overlay outside normal collection. Existing selected admission and
latest-observation queue controls are reused; no permanent tests are added.
Final command and results are recorded after execution; normal hooks must pass
before publication.

Final follow-up result: the temporary cases and selected existing admission/latest-observation controls passed with -race (orchestrator 1.135s; statussummary 1.052s). Command: go test -race -tags fts5 -trimpath -p 2 with the explicit temporary overlay, both affected packages, and the TestOneOffGradeReview/TestLaunchQueueSummaryFromTaskWithCapacityUsesLatestObservation/TestAdmitAtCeiling/TestUnknownPopulation/TestHandOff selection, -count=1. git diff --check passed.
