---
created: 2026-09-08
status: draft
requirements:
  - REQ-OFFICE-WAKE-CHILD-SUMMARIES-001
  - REQ-OFFICE-WAKE-CHILD-SUMMARIES-002
  - REQ-OFFICE-WAKE-CHILD-SUMMARIES-003
  - REQ-OFFICE-WAKE-CHILD-SUMMARIES-004
system_design:
  - ../../specs/office/system-design/children-completed-wake-summaries.md
legacy_specs: []
---

# Implementation Plan: Children-Completed Wake Summaries

## Overview

Make the `task_children_completed` wake name the children it is about. Today the
prompt's child list section is unreachable: `enrichChildrenContext` populates it
from a `children` key in the run payload, and none of the four producers writes
that key, so a parent woken to review delegated work is told only that the work
finished.

The section moves off the run payload entirely and is derived at prompt assembly
time from the parent's current children. That is what makes the wake identical
whichever of the four producers won its race — the producers stop being asked for
the content at all — and it is why the payload route was rejected (see the system
design's "Why not carry the summaries on the run payload").

Delivery order follows the data path. Wave 1 fixes the two ends that have
independent test oracles and no dependency on each other: the repository query
that will supply the rows (Task 01) and the renderer that will format them
(Task 02). Wave 2 connects them by rewiring the enricher to read at assembly time
(Task 03), which is the work order that closes the defect. Wave 3 removes the
producer-side reads that now feed nothing (Task 04); it lands last so the wake is
never in a state where the reads have been deleted but the assembly-time read is
not yet in place.

## Scope

### In scope

- `Repository.GetChildSummaries` becomes a single statement with a live-child
  predicate, a parent-existence guard, a deterministic order, a deterministic
  last-comment tiebreak, and a comment read that makes truncation detectable.
- The rendered child summary line gains pull-request URLs, rune-counted caps on
  every unbounded field, control-character sanitization, and a heading that does
  not assert completion.
- `enrichChildrenContext` takes a context and the parent task id, reads at prompt
  assembly time, and stops parsing the run payload.
- `queueChildrenCompletedRun` and `ParentWakeReconciler.buildPayload` stop
  performing child-summary and pull-request reads whose results are discarded.

### Out of scope

- Removing `ChildSummaries` from `engine.OnChildrenCompletedPayload`, or changing
  `engine.ChildSummary`. Deleting a field from a shared workflow-engine type is a
  separate contract change with a different owner.
- `orchestrator.childCompletionPayload`. It builds summaries from rows already in
  memory for the readiness check, so it costs no extra read and
  AC-OFFICE-WAKE-CHILD-SUMMARIES-002.5 does not reach it.
- `SchedulerService.cascadeChildrenCompleted`. It queues a bare
  `scheduler.RunContext` and never assembled summaries to discard.
- Wave identity, run deduplication, and backstop admission, owned by the sibling
  `parent-wake-wave-identity` capability. It may not have merged yet, so it is
  named rather than linked; the system design restates its producer-equivalence
  constraint in full and nothing here needs that document resolved.
- Readiness: when the wake fires and which children make a parent ready.
- Any user interface, control, view, or setting.

## Technical approach

### Repository (Task 01)

`apps/backend/internal/office/repository/sqlite/blockers.go`,
`GetChildSummaries`. The un-transacted `COUNT(*)` and the separate `SELECT`
collapse into one statement, with the count as a scalar subquery over the same
predicate so the count and the rows share a snapshot — two statements can leave a
stale total above the cap while the capped select already covers every live child,
which is the state AC-OFFICE-WAKE-CHILD-SUMMARIES-003.6 forbids. A plain scalar
subquery rather than `COUNT(*) OVER ()` keeps window-function support out of the
requirement. `archived_at IS NULL` enters both the count and the row predicate;
no state predicate is added, because AC-OFFICE-WAKE-CHILD-SUMMARIES-003.1a
requires a restarted child to appear. `EXISTS (SELECT 1 FROM tasks p WHERE
p.id = ?)` guards on the parent existing, because `tasks.parent_id` is
`TEXT DEFAULT ''` with no foreign key and no cascade, so orphaned child rows
would otherwise list under a deleted parent. `ORDER BY t.created_at ASC, t.id ASC`
makes the 20-row cap deterministic; the last-comment subquery gains
`ORDER BY c.created_at DESC, c.id DESC` and reads `SUBSTR(c.body, 1,
maxCommentChars+1)` so the renderer can tell a cut body from a complete one.
Every construct used is common to SQLite and PostgreSQL, so no dialect branch is
introduced.

### Renderer (Task 02)

`apps/backend/internal/office/service/prompt_builder.go`. `ChildSummaryPrompt`
gains `PRLinks []string`. `writeChildSummaryLine` renders four shapes — the two
optional segments, comment then pull requests, are each self-delimiting and carry
their own leading ` — `. Every interpolated field has each rune rejected by
`strconv.IsPrint` replaced with a single space, before any cap and before `%q`
quotes the comment; that is what makes one child exactly one line, and it leaves
`%q` nothing to expand except `"` and `\`. Caps are counted in Unicode code
points, never bytes, because SQL `SUBSTR` counts code points on both backends
while Go's `len` and slicing count bytes: comment 500, title 200, each URL 200,
each rendered as a leading slice plus a 12-code-point ` [truncated]` marker that
is included in the cap. At most ten URLs render, sorted ascending by URL string
before capping, with ` (+N more)` where N is the count not rendered, clamped to
` (+999+ more)` so the numeral is the one remaining unbounded quantity no longer.
The heading changes from `"\nCompleted children:\n"` to `"\nChild tasks:\n"`; the
lead-in, closing instruction, and truncation notice are unchanged.

### Enricher (Task 03)

`apps/backend/internal/office/service/scheduler_integration.go`.
`enrichChildrenContext` takes the same shape as its sibling enrichers: `ctx` plus
the parent task id, a read, and a populated `PromptContext`. It calls
`GetChildSummaries` and `si.svc.lookupChildPRLinks`, sets `ChildSummaries` and
`ChildSummariesTruncated`, and no longer unmarshals the payload. The call site in
`buildPromptContext` is gated on a non-empty `task_id` in addition to the run
reason. A child-read failure logs at warn with the parent task id — warn rather
than debug because an empty section is exactly the defect being closed and is
otherwise indistinguishable from a parent with no children — and leaves the
section empty. `test_helpers.go` gains a `*ForTest` export so an external test
package can drive the real assembly path.

### Producers (Task 04)

`queueChildrenCompletedRun` (`event_subscribers.go`) and
`ParentWakeReconciler.buildPayload` (`scheduler_wake_reconciler.go`) stop calling
`GetChildSummaries` and `lookupChildPRLinks` and dispatch with no child
summaries. `buildPayload` becomes infallible, so its error arm and its caller's
error branch disappear. `GetChildSetKey`, its revalidation, and the transactional
receipt are untouched — the guards that decide *whether* to dispatch are not the
ones being removed. One deliberate behaviour change follows: the reconciler will
now dispatch where a `GetChildSummaries` failure previously aborted the tick,
which strictly increases delivery of a wake already judged due
(AC-OFFICE-WAKE-CHILD-SUMMARIES-002.9).

## Tests

| Acceptance criterion | Evidence |
| --- | --- |
| 001.1, 001.2 | `prompt_builder_test.go` `TestBuildPrompt_ChildrenCompleted_WithSummaries`; `scheduler_integration_children_test.go` `TestEnrichChildrenContext_ListsLiveChildren` |
| 001.3, 001.4 | `prompt_builder_test.go` `TestWriteChildSummaryLine_CommentSegment` (no comment and empty comment render identically) |
| 001.5 | `prompt_builder_test.go` `TestTruncateComment_CountsRunes` (501 runes marked, 500 not, multibyte body stays valid UTF-8); `blockers_test.go` `TestGetChildSummaries_ReadsOneCharPastCommentCap` |
| 001.6 | `prompt_builder_test.go` `TestWriteChildSummaryLine_IsSingleLine` (CR, LF, U+2028, tab in title and comment) |
| 001.7, 001.8, 003.7 | `prompt_builder_test.go` `TestWriteChildSummaryLine_PRSegment` (sorted, ten rendered, `(+N more)` is the unrendered count, clamped at `(+999+ more)`, per-URL cap) |
| 001.9 | `prompt_builder_test.go` `TestWriteChildSummaryLine_MissingIdentifier` |
| 001.10, 001.11, 001.12 | `prompt_builder_test.go` `TestBuildPrompt_ChildrenCompleted_NoSummaries`, `TestBuildPrompt_ChildrenCompleted_HeadingIsNeutral` |
| 002.1, 003.8 | `scheduler_integration_children_test.go` `TestChildSectionIsByteIdenticalAcrossProducers` (four payload shapes, one parent, byte-equal sections) |
| 002.2, 002.3 | `scheduler_integration_children_test.go` `TestPayloadChildrenKeyIsIgnored` (payload carries `children` and `truncated` contradicting the database) |
| 002.4, 002.5, 002.9 | `event_subscribers_children_completed_test.go` `TestQueueChildrenCompletedRun_PerformsNoChildSummaryRead`; `scheduler_wake_reconciler_test.go` `TestReconcileOne_DispatchesWithoutChildSummaryRead` |
| 002.6 | `scheduler_integration_children_test.go` `TestChildSectionRendersForLegacyReason` |
| 002.7 | `prompt_reason_contract_test.go` stays green; `scheduler_integration_children_test.go` `TestNonChildrenReasonPerformsNoChildRead` |
| 002.8 | `scheduler_integration_children_test.go` `TestChildStateIsReportedAsStored` (child in a terminal workflow step but non-terminal task state renders its task state) |
| 003.1, 003.1a, 003.6 | `blockers_test.go` `TestGetChildSummaries_ExcludesArchived`, `TestGetChildSummaries_IncludesNonTerminalChild`, `TestGetChildSummaries_ArchivedChildCannotTriggerTruncation` |
| 003.2, 003.3, 003.4, 003.5 | `blockers_test.go` `TestGetChildSummaries_OrdersByCreatedAtThenID`, `TestGetChildSummaries_LastCommentTiebreak`, `TestGetChildSummaries_CapsAtTwenty` |
| 003.9 | `child_summaries_postgres_test.go` `TestPostgresGetChildSummaries` (membership, order, cap, truncation on PostgreSQL) |
| 004.1, 004.4 | `scheduler_integration_children_test.go` `TestChildReadFailureStillLaunches` |
| 004.2 | `scheduler_integration_children_test.go` `TestPRLookupFailureStillRendersLine` (lister unwired, and lister erroring) |
| 004.3 | `blockers_test.go` `TestGetChildSummaries_DeletedParentYieldsNoRows` (orphaned child rows present) |
| 004.3a | `scheduler_integration_children_test.go` `TestRunWithoutTaskIDSkipsChildRead` |
| 004.5, 004.6 | `scheduler_integration_children_test.go` `TestSectionReflectsAssemblyTimeValues` (child mutated between queue and assembly; two assemblies of one run) |
| 004.7 | Covered by construction, not by a race test: each read reports one snapshot and neither can fail assembly. Asserted as the pairing rule in `TestSectionReflectsAssemblyTimeValues`. |
| 004.8 | `scheduler_integration_children_test.go` `TestChildReadCountDoesNotGrowWithChildren` (3 children and 30 children yield an equal child-section query count) |

## E2E tests

None. The capability adds no control, view, or setting; its output reaches two
existing surfaces unchanged — the agent session's first message and the Office
run detail prompt panel, both rendering `runs.assembled_prompt` verbatim. A
browser test would assert that an existing panel still displays a string the
backend test already pins byte-for-byte, so the end-to-end evidence for this work
package is Task 03's assembly-path test rather than a Playwright spec.

## Work orders

Wave 1:

- [ ] [Task 01: Make the child-summary query deterministic and live-only](task-01-child-summary-query.md)
- [ ] [Task 02: Render the child summary line](task-02-child-summary-line.md)

Wave 2:

- [ ] [Task 03: Derive the child list at prompt assembly time](task-03-assembly-time-enrichment.md)

Wave 3:

- [ ] [Task 04: Stop producers paying for discarded child summaries](task-04-drop-producer-reads.md)

## Verification results

Pending.

## Risks

- **The 500 boundary is shared across two packages in Wave 1.** Task 01's SQL
  reads `maxCommentChars + 1` code points and Task 02's renderer marks a body
  longer than 500 code points. The two constants live in different packages and
  the files never touch, but the tasks are only correct together: if either side
  moves its number alone, a complete comment is reported as truncated or a cut one
  is presented as whole. Both work orders state the contract; Task 03's
  end-to-end test is where a mismatch actually fails.
- **Mixing byte and code-point units is the failure mode with the worst
  symptom.** Go's `len(s)` and `s[:n]` count bytes while SQL `SUBSTR` counts code
  points, so a byte-unit slip both mislabels non-ASCII comments and can split a
  UTF-8 sequence into a live agent prompt. The product ships four non-English
  locales and agent comments routinely contain non-ASCII, so a test on ASCII-only
  fixtures would not catch it. Every cap test uses a multibyte body.
- **The heading literal is inside a byte-identity contract.**
  AC-OFFICE-WAKE-CHILD-SUMMARIES-003.8 makes the whole section byte-comparable,
  and `prompt_builder_test.go:240` currently asserts on the old
  `"Completed children:"` string. Task 02 must update that assertion rather than
  leave a test that passes for the wrong reason.
- **Task 04 leaves `lookupChildPRLinks` with exactly one caller.** That caller is
  Task 03's enricher, which is why Wave 3 follows Wave 2. Landing Task 04 first
  would leave the helper callerless and reachable by `make -C apps/backend
  deadcode`.
- **The reconciler gets strictly more permissive.** Removing its
  `GetChildSummaries` error arm means it dispatches on ticks where it previously
  bailed. This is intended and required by
  AC-OFFICE-WAKE-CHILD-SUMMARIES-002.9, but it changes reconciler behaviour, so
  the test must pin that `GetChildSetKey`, its revalidation, and the receipt still
  gate dispatch.
