---
id: "01-implementation"
title: "feat(capacity): add opt-in total and execution-grade limits"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-SESSION-CEILING-003
acceptance_criteria:
  - AC-AGENTS-SESSION-CEILING-003.1
  - AC-AGENTS-SESSION-CEILING-003.2
  - AC-AGENTS-SESSION-CEILING-003.3
  - AC-AGENTS-SESSION-CEILING-003.4
  - AC-AGENTS-SESSION-CEILING-003.5
system_design:
  - ../../specs/agents/system-design/session-concurrency-ceiling.md
---

# feat(capacity): add opt-in total and execution-grade limits

## Outcome and scope

Opt into all-session and explicit execution-grade capacity with existing reservation, FIFO replay and queue presentation contracts.

Out of scope: Changing disabled defaults, assigning grades from profile names, or changing dynamic candidate order.

## Implementation acceptance

1. The affected native entrypoint follows the linked requirement and design while preserving unrelated behavior and history.
2. Rejected, stale and failed operations retain their existing refusal or rollback semantics.
3. The focused implementation checks and linked documentation coverage have honest recorded results; unexecuted follow-up checks remain pending.

## Files and dependencies

The source files are the existing PR #4715 diff. This work order adds its delivery record and links existing requirements and system design; it introduces no runtime gate or permanent test.

- [System design](../../specs/agents/system-design/session-concurrency-ceiling.md)
- [Plan](plan.md)

## Verification and results

Validation:
- Existing related backend and frontend checks passed; 22 existing frontend tests and full TypeScript/changed-file lint passed.
- Original grade/admission cases were reused through temporary Go overlays across 11 packages, plus a focused manual-deferral check. A temporary mounted settings check verified all-policy grade-zero save and disabling without replacing the saved grade map.
- SQL guard, affected SQLite race conformance and the actual previous-stable upgrade passed. The upgrade needed a temporary two-path adaptation of an existing Unix-path fixture on Windows; product code and assertions were unchanged.
- All normal commit hooks passed. The large Go check used a task-local cache and the script's process-local concurrency option after shared export-cache failures/time limits; the lint rules, memory target and timeout stayed intact.
- Four existing capacity tests and the existing upgrade manifest were adapted for the additive fields; no permanent test cases or CI jobs were added.

Documentation-only correction: git diff --check and the existing .github/scripts/pr-docs.cjs validateCoverage entrypoint with the candidate changed paths and the linked candidate documents supplied on stdin. No GitHub status is synthesized locally.

The already prepared review follow-up is still in progress. Its remaining narrow checks and normal commit hooks must pass before that follow-up is published. The earlier published implementation checks do not establish the pending result.

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
