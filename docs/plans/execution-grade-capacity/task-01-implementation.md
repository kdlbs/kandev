---
id: "01-implementation"
title: "feat(capacity): add opt-in total and execution-grade limits"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-SESSION-CEILING-005
acceptance_criteria:
  - AC-AGENTS-SESSION-CEILING-005.1
  - AC-AGENTS-SESSION-CEILING-005.2
  - AC-AGENTS-SESSION-CEILING-005.3
  - AC-AGENTS-SESSION-CEILING-005.4
  - AC-AGENTS-SESSION-CEILING-005.5
system_design:
  - ../../specs/agents/system-design/session-concurrency-ceiling.md
---

# feat(capacity): add opt-in total and execution-grade limits

## Main integration: message-turn tickets

The newer shared ceiling also counts in-flight message-turn tickets in its
total. A ticket already represented by an admitted row or launch reservation
uses the existing grade once. A separate ticket has no grade snapshot, so the
all-policy grade population is unknown until it is represented by an admitted
row or launch reservation. Existing session ownership and automatic-policy
message-turn behavior remain unchanged.

## Outcome and scope

Opt into all-session and explicit execution-grade capacity with existing reservation, FIFO replay and queue presentation contracts.

Out of scope: Changing disabled defaults, assigning grades from profile names, or changing dynamic candidate order.

## Implementation acceptance

1. The affected native entrypoint follows the linked requirement and design while preserving unrelated behavior and history.
2. Rejected, stale and failed operations retain their existing refusal or rollback semantics.
3. The focused implementation checks and linked documentation coverage have honest recorded results; unexecuted follow-up checks remain pending.

## Files and dependencies

This work order tracks the original PR #4715 slice and the maintainer review
fixes below. It introduces no runtime gate.

- [System design](../../specs/agents/system-design/session-concurrency-ceiling.md)
- [Plan](plan.md)

## Original implementation verification

Validation:
- Existing related backend and frontend checks passed; 22 existing frontend tests and full TypeScript/changed-file lint passed.
- Original grade/admission cases were reused through temporary Go overlays across 11 packages, plus a focused manual-deferral check. A temporary mounted settings check verified all-policy grade-zero save and disabling without replacing the saved grade map.
- SQL guard, affected SQLite race conformance and the actual previous-stable upgrade passed. The upgrade needed a temporary two-path adaptation of an existing Unix-path fixture on Windows; product code and assertions were unchanged.
- All normal commit hooks passed. The large Go check used a task-local cache and the script's process-local concurrency option after shared export-cache failures/time limits; the lint rules, memory target and timeout stayed intact.
- Four existing capacity tests and the existing upgrade manifest were adapted for the additive fields; no permanent test cases or CI jobs were added.

Documentation-only correction: git diff --check and the existing .github/scripts/pr-docs.cjs validateCoverage entrypoint with the candidate changed paths and the linked candidate documents supplied on stdin. No GitHub status is synthesized locally.

The maintainer consolidation below supersedes the earlier pending follow-up.

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

## Maintainer consolidation

The contributor included reservation deduplication before consolidation.
Recovered permanent admission, grade-map validation, and draft Save/Reset tests
from `9dcd1edf347`. The backend and browser draft reject more than 64 grade
limits. Both regressions failed before the bound and passed afterward.

Ported task-grade validation into main's extracted task-update helper.
Permanent tests cover combined priority/grade changes, omitted grades, explicit
clearing, and invalid input. SQLite and PostgreSQL tests also preserve the
admitted grade through ordinary session writes and keep legacy grades unknown.

Reproduced both historical CI failures from the contributor head. The legacy
task rebuild now supplies an empty grade when its source lacks the column and
preserves an existing value. The sidebar HTTP test decodes JSON instead of
requiring one property order.

Focused race tests passed in session capacity, orchestrator, task repository,
task handlers, and task service. The snapshot regression passed on SQLite and
PostgreSQL 18. All 27 focused frontend tests passed.
All three phone settings scenarios passed without retries, including the new
grade-limit Save/Reset/disable/reload flow and 44px input/action targets.
Test cleanup restores the policy and grade map with the total limit.
