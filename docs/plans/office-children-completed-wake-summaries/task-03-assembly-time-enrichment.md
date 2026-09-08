---
id: "03-assembly-time-enrichment"
title: "Derive the child list at prompt assembly time"
status: pending
wave: 2
depends_on: ["01-child-summary-query", "02-child-summary-line"]
plan: "plan.md"
requirements:
  - REQ-OFFICE-WAKE-CHILD-SUMMARIES-001
  - REQ-OFFICE-WAKE-CHILD-SUMMARIES-002
  - REQ-OFFICE-WAKE-CHILD-SUMMARIES-004
acceptance_criteria:
  - AC-OFFICE-WAKE-CHILD-SUMMARIES-001.1
  - AC-OFFICE-WAKE-CHILD-SUMMARIES-002.1
  - AC-OFFICE-WAKE-CHILD-SUMMARIES-002.2
  - AC-OFFICE-WAKE-CHILD-SUMMARIES-002.3
  - AC-OFFICE-WAKE-CHILD-SUMMARIES-002.6
  - AC-OFFICE-WAKE-CHILD-SUMMARIES-002.7
  - AC-OFFICE-WAKE-CHILD-SUMMARIES-002.8
  - AC-OFFICE-WAKE-CHILD-SUMMARIES-003.8
  - AC-OFFICE-WAKE-CHILD-SUMMARIES-004.1
  - AC-OFFICE-WAKE-CHILD-SUMMARIES-004.2
  - AC-OFFICE-WAKE-CHILD-SUMMARIES-004.3
  - AC-OFFICE-WAKE-CHILD-SUMMARIES-004.3a
  - AC-OFFICE-WAKE-CHILD-SUMMARIES-004.4
  - AC-OFFICE-WAKE-CHILD-SUMMARIES-004.5
  - AC-OFFICE-WAKE-CHILD-SUMMARIES-004.6
  - AC-OFFICE-WAKE-CHILD-SUMMARIES-004.7
  - AC-OFFICE-WAKE-CHILD-SUMMARIES-004.8
system_design:
  - ../../specs/office/system-design/children-completed-wake-summaries.md
---

# Task 03: Derive the child list at prompt assembly time

## Summary

Rewire `enrichChildrenContext` to take a context and the parent task id, read the
parent's live direct children and their pull-request links, and populate
`PromptContext`. It stops parsing the run payload entirely, which is what closes
the defect and what makes the rendered section identical whichever producer
queued the run.

## In scope

- Change `enrichChildrenContext(pc, payload)` to `enrichChildrenContext(ctx, pc,
  parentTaskID)`, matching the shape of `enrichHandoffContext` and
  `enrichCommentContext`.
- Read via `si.svc.repo.GetChildSummaries` and `si.svc.lookupChildPRLinks`;
  populate `ChildSummaries` (including `PRLinks`) and `ChildSummariesTruncated`.
- Delete the payload `children` / `truncated` unmarshal and its anonymous struct.
- Gate the `buildPromptContext` call site on a non-empty `task_id` as well as the
  run reason, keeping both `task_children_completed` and the legacy
  `children_completed`.
- Log a warn carrying the parent task id (and no run id, which is not in scope at
  the failure site) when the child read fails; leave the section empty.
- Add a `*ForTest` export in `test_helpers.go` so `service_test` can drive the
  real assembly path, following `LoadContinuationSummaryForTest`.

## Out of scope

- Producer-side reads, which Task 04 removes.
- Any change to `truncateComment`, the caps, or the heading, which Task 02 owns.
- Any change to the run payload's shape, to `CoalesceRun`, or to run admission.
- Serialising the two reads against child writes. The section is a point-in-time
  reading with no cross-read atomicity, as the system design states.

## Acceptance

- Four run payloads that differ only in the way each producer would have written
  them, assembled against one parent, yield byte-identical child list sections;
  a payload carrying a `children` array and `truncated: true` that contradict the
  database changes nothing.
- A child read error, an unwired `TaskPRLister`, a `TaskPRLister` returning an
  error, a run with no `task_id`, and a parent task that no longer exists each
  produce a prompt with the lead-in and closing instruction and no child list
  section, and none of them fails the run.
- The child section costs two reads regardless of child count: a parent with 3
  live children and one with 30 issue the same number of child-section queries,
  and a run of any other reason issues none.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/office/service/ -run 'TestEnrichChildrenContext|TestChildSection|TestChildRead|TestPayloadChildrenKey|TestRunWithoutTaskID|TestPRLookupFailure|TestSectionReflectsAssemblyTime|TestNonChildrenReason|TestChildStateIsReportedAsStored' -count=1
```

```bash
cd apps/backend && go test -tags fts5 ./internal/office/... -count=1
```

```bash
cd apps/backend && golangci-lint run ./internal/office/...
```

## Files likely touched

- `apps/backend/internal/office/service/scheduler_integration.go`
- `apps/backend/internal/office/service/test_helpers.go`
- new: `apps/backend/internal/office/service/scheduler_integration_children_test.go`

## Risks

- The four-producer equivalence criterion is the one that cannot be proven by
  reading the diff. It holds because the payload is no longer consulted, so the
  test must assert byte equality across producer-shaped payloads rather than
  assert that the code ignores them.
- `buildPromptContext` already parses `task_id` for `enrichTaskContext`; reusing
  that parsed value is right, but the children branch currently sits outside that
  `if` block. Moving the call inside it without preserving the reason gate would
  silently give every task-bearing run a child read, breaking
  AC-OFFICE-WAKE-CHILD-SUMMARIES-002.7.
- The bounded-read criterion is about growth, not an absolute number:
  `buildPromptContext` performs unrelated reads before the child enricher, so a
  test asserting a total query count for the whole assembly tests the wrong thing.
- Warn, not debug. An empty section is indistinguishable from a childless parent,
  which is precisely the failure this capability exists to make visible.

## Parallelism

`sequential`

## Inputs

- System design, [Where the data comes from](../../specs/office/system-design/children-completed-wake-summaries.md#where-the-data-comes-from), [Control flow](../../specs/office/system-design/children-completed-wake-summaries.md#control-flow), and [Failure and recovery](../../specs/office/system-design/children-completed-wake-summaries.md#failure-and-recovery).
- `apps/backend/internal/office/service/scheduler_integration.go:929-973` and `:1046-1069`.
- Sibling enricher pattern: `enrichHandoffContext`, `enrichCommentContext`.
- Test-export pattern: `apps/backend/internal/office/service/test_helpers.go:29-36`.

## Results

Pending.
