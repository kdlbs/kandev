---
created: 2026-09-17
status: complete
requirements:
  - REQ-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001
system_design:
  - ../../specs/platform/system-design/bounded-task-status-delivery.md
legacy_specs: []
---

# Implementation Plan: Task-summary semantic equality

## Overview

Stop false compare-and-set exhaustion errors when a source event changes a
detail but leaves the bounded task summary unchanged. Align the projector's
in-memory equality with its existing persisted semantic JSON, then prove the
complete event-to-storage-to-publication path with a real SQLite repository.
One sequential work order owns the correction and its regression coverage.

## Evidence and confirmed root cause

Investigation baseline: checkout `f32eb3737`, 2026-09-17. Today's retained
backend files contained 31 `exhausted CAS retries refreshing pending task
status` warnings paired with `github.task_pr.updated` handler errors between
11:19 and 15:16 Lisbon time. At 15:16:47, the source changed review counts.
Read-only inspection of three affected task summaries found retained session
errors with omitted `recovery_actions` and `causes` fields. These database
reads describe inspection-time state, not a historical snapshot.

The deterministic cause is independent of concurrent writers:

1. `restoreSessionObservations` / `normalizeRebuildError` rebuild error
   observations. `NormalizeRecoveryActionsForCategory` and
   `NormalizeAgentErrorCauses` can produce non-nil zero-length slices.
2. `deriveActiveError` retains those slices. `cloneSummary` and database JSON
   round trips omit them through `omitempty` and restore nil slices.
3. `TaskStatusSummary.SemanticEqual` uses `reflect.DeepEqual`, which treats
   those in-memory representations as different.
4. A PR detail change, such as pending review count 1 to 2 while checks remain
   failing, changes the keyed observation but not the aggregate summary.
5. The projector attempts a write. The repository correctly rejects it because
   `SemanticJSON` is identical. Reload/rebase restores the same mismatch, so
   all three retries fail and the handler returns an error.

A temporary test named `TestReproSummaryPRNoopWithRetainedError` drove
`Projector.HandleEvent` with a real SQLite repository and synthetic data. It
used one waiting primary session with an explicitly session-scoped retained
error and two PR events with checks `failure`, changing pending review count
from 1 to 2. The second event returned the exact exhaustion error. The before
and after stored JSON were identical and both revisions were 1. No competing
writer, running agent, external provider, or browser was involved.

The temporary reproduction was removed after investigation. Its permanent
success assertion belongs to Task 01's red phase. An initial fixture omitted
the error's session scope and therefore introduced a separate summary change;
the final fixture explicitly sets scope and session identity.

This establishes false errors for aggregate-preserving events, not a confirmed
loss of a meaningful PR status change. Existing projector tests missed it
because `projectorTestStore` rejects old revisions but accepts identical
semantic payloads with newer revisions, unlike the production repository.

## Requirement conformance and assumptions

Platform owns the bounded projection contract; Tasks and Integrations retain
the authoritative source records. This is an implementation defect under
[AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.2 and .3](../../specs/platform/requirements/bounded-task-status-delivery.md)
and the design's existing semantic-no-op and persistence guarantees. Reuse the
active requirement without adding a repair-specific requirement or new IDs.

The intended behavior is settled: unchanged projections succeed silently;
real changes preserve monotonic revisions and workspace-scoped publication.
No new product behavior, schema, ownership boundary, or ADR is needed. The
design clarifies the shared equality rule. The accepted
[summary boundary ADR](../../decisions/2026-08-01-separate-task-summary-session-stream-traffic.md)
and [activity ADR](../../decisions/2026-08-17-separate-task-activity-from-summary-freshness.md)
remain authoritative.

## Scope

### In scope

- Consistent equality for every valid bounded summary and its persisted form.
- Nil/empty error collections and JSON round-trip equivalence.
- Real SQLite projector regression, publication assertions, and faithful test
  store no-op behavior.
- Preservation of genuine changes, validation failures, and contention errors.

### Out of scope

- Workflow import validation/input repair, task-deletion races, diff requests
  during teardown, authentication settings, and agent-provider errors.
- Changes to error normalization throughout the model domain or error text.
- Retry-count increases, suppressing all CAS warnings, database rewrites,
  schema migrations, new flags, and PR polling changes.
- Frontend layout, copy, interaction, and status precedence changes.

## Technical approach

In `apps/backend/internal/task/statussummary/model.go`, implement valid-value
equality using the canonical payload already provided by `SemanticJSON`.
Compare successful encodings with `bytes.Equal`; if either encoding fails,
return false so the projector continues into its existing validation/error
path. Do not equate two encoding failures by comparing nil byte slices.
This reuses the bounded persistence contract and automatically includes future
semantic fields. Do not change the JSON shape or broaden timestamp equivalence
beyond the existing serialized representation.

Keep `persistAndPublishLocked`, its reload/rebase behavior, and the repository
SQL contract intact unless the regression demonstrates a necessary local
adjustment. Correct `projectorTestStore.CompareAndUpdateTaskStatusSummary` in
`projector_test.go` to reject an unchanged workspace and semantic payload as
well as stale revisions. Do not relax unrelated test assertions to compensate.

Add focused model tests in `model_semantic_equality_test.go` and an integration
test in `internal/task/repository/sqlite/task_status_summary_projector_noop_test.go`.
Use the existing temporary-database helpers, synthetic identities, explicit
session error scope, fixed UTC times, and synchronous event handling.
The fixture must not open the developer's database or copy private log data.

## Tests

| Evidence | Acceptance mapping |
| --- | --- |
| `TestTaskStatusSummarySemanticEqualityMatchesPersistence`: nil/empty collections independently for active/task errors, JSON round trips, timestamp monotonic metadata, ignored revision/update time, and real semantic differences | AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.2 |
| `TestTaskStatusSummarySemanticEqualityRejectsInvalidValues`: invalid summaries cannot be classified as successful no-ops | AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.2 |
| `TestTaskStatusSummaryProjectorPRNoopWithRetainedError`: live and recreated projectors, real SQLite, unchanged payload/revision, zero extra publications, no exhaustion warning | AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.2, AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.3 |
| The real-change portion of `TestTaskStatusSummaryProjectorPRNoopWithRetainedError`: later aggregate change persists once and publishes the complete next revision while preserving error and unrelated fields | AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.2, AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.3 |
| Existing pending-authority and queue contention tests with the corrected test store | AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.2 |

## E2E evidence

The real-repository integration tests cover source event, normalization,
projection, SQL persistence/reload, and complete workspace-summary publication.
They exercise the shared payload consumed by desktop and mobile. No rendered
UI changes or new browser test are needed for this internal no-op correction.

## Work orders

- [x] [Task 01: Align semantic equality and prove persistence behavior](task-01-align-semantic-equality.md)

Execution is sequential. No subagents are authorized or required.

## Related implementation records

The linked packages `bounded-task-status-delivery` (implemented),
`session-stream-overload-isolation` (complete), `backend-runtime-state-ownership`
(completed), and `deleted-session-error-summary` (done) were inventoried.
This repair does not reopen their work orders or alter their E2E matrices;
their historical verification results remain unchanged.

## Verification results

Investigation: the final temporary regression failed as expected with three
CAS retries, identical persisted JSON, and unchanged revision 1:

```bash
(cd apps/backend && go test ./internal/task/repository/sqlite -run '^TestReproSummaryPRNoopWithRetainedError$' -count=1 -v)
```

Implementation and permanent regression verification passed on 2026-09-17:

```text
(cd apps/backend && go test ./internal/task/statussummary ./internal/task/repository/sqlite \
  -run 'TestTaskStatusSummarySemanticEquality|TestTaskStatusSummaryProjectorPRNoopWithRetainedError' -count=1)
  2 packages passed.

(cd apps/backend && go test -race ./internal/task/statussummary ./internal/task/repository/sqlite -count=1)
  2 packages passed.
```

The production equality path now uses the same canonical semantic JSON as the
repository. The SQLite projector regression proves no-op events keep the same
revision and publication count, recreated projectors converge, and a real PR
aggregate change advances once while preserving the retained error.
Design-package checks passed on 2026-09-17:

- `python3 scripts/list-docs.py validate`: 285 decisions and 986 specifications.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `git diff --check`: passed.
- `git status --short -- docs/plans/task-summary-semantic-equality`: confirmed
  the new package is present and untracked; it contains this plan and Task 01.

The implementation is complete in Task 01. Commit and PR delivery evidence is
recorded outside this plan.

## Risks

- Comparing canonical JSON allocates bounded payloads; keep it on the existing
  semantic-event path and do not introduce transcript or polling work.
- A faithful test store may expose pre-existing tests that rely on artificial
  revision bumps. Preserve their behavioral intent and diagnose each failure.
- A no-op fix must not hide real field changes, invalid values, store failures,
  or genuine contention exhaustion.
- The SQLite reproduction establishes this cause but does not prove every
  historical CAS error had the same cause. No SQL changes are planned; if that
  scope changes, add the repository-required PostgreSQL behavioral coverage.
