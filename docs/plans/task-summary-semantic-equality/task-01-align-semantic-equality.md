---
id: "01-align-semantic-equality"
title: "Align semantic equality and prove persistence behavior"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001
acceptance_criteria:
  - AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.2
  - AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.3
system_design:
  - ../../specs/platform/system-design/bounded-task-status-delivery.md
---

# Task 01: Align semantic equality and prove persistence behavior

## Summary

Make valid summary equality agree with persisted canonical JSON. Prove that a
PR detail change with an unchanged aggregate and a retained session error is
a successful no-op, while a later real change still persists and publishes.

## In scope

- Add model and real-SQLite regression tests before production edits.
- Replace representation-sensitive equality with successful canonical-payload
  comparison, preserving the existing validation/error path.
- Match the test store's rejection semantics to the production repository.
- Record red/green command results and update the plan/work-order status.

## Out of scope

All exclusions in [the plan](plan.md#scope), especially repository SQL changes,
global normalization changes, UI work, log-level suppression, and retry tuning.

## Acceptance

1. Every tested valid summary compares equal to its canonical JSON round trip,
   including nil/empty error actions and causes. Metadata-only differences
   are equal; real semantic differences are unequal. Failed encodings are
   never accepted as equality.
2. Against real SQLite, both live and recreated projectors process an
   aggregate-preserving PR change with no handler error, exhaustion warning,
   revision change, or extra summary publication. Preserve unrelated fields.
3. A subsequent true aggregate change advances exactly one revision and
   publishes exactly one complete replacement with task/workspace identity.
   Existing store-error and contention behavior stays observable and bounded.

## TDD sequence and regression construction

1. Mark this work order `in_progress`. Add
   `TestTaskStatusSummaryProjectorPRNoopWithRetainedError` using
   `newTaskStatusSummaryTestRepo` and `seedTaskForStatusSummary`.
2. Configure `LoadSessionObservations` with one primary waiting session,
   `ErrorsObserved: true`, and an error with `Scope: "session"`, matching
   `SessionID`, fixed UTC `OccurredAt`, stamp, and preview. Leave optional
   collections absent so production normalization creates the relevant shape.
3. Call `Projector.HandleEvent` with `GitHubTaskPRUpdated`, fixed task/workspace
   and repository identity, one open PR, `checks_state: "failure"`, and
   `pending_review_count: 1`. Load the first summary, then send count 2.
   The unchanged `failure` aggregate must succeed; current code instead returns
   `exhausted CAS retries ... after 3 attempts`. Assert payload and revision
   as well as event counts. Capture the projector logger for the warning check.
4. Include a recreated-projector case using the same temporary repository and
   authoritative session/PR loaders. Keep loader keys aligned with event keys.
   Register bus/logger/database cleanup immediately. No sleeps are needed.
5. Add model cases named in the plan. Independently exercise empty actions and
   empty causes in both `ActiveError` and `TaskError`, plus valid populated
   changes, removal, queue count, Git/PR state, and last activity changes.
6. Implement canonical equality and faithful fake-store semantics. Never
   return equality when either `SemanticJSON` call fails. Do not swallow CAS
   exhaustion or increase its attempt count.
7. In `TestTaskStatusSummaryProjectorPRNoopWithRetainedError`, after the
   no-op cases, change aggregate status to a distinct state, assert one
   write/event, then replay and assert no extra event. Verify the retained
   error and complete replacement survive. Run the targeted packages and
   record results.

## Verification

Run from the repository root. The first command is the red test before the
production edit and must be rerun green afterward. The second covers every
changed suite and the existing contention regressions.

```bash
(cd apps/backend && go test ./internal/task/repository/sqlite -run '^TestTaskStatusSummaryProjectorPRNoopWithRetainedError$' -count=1 -v)
(cd apps/backend && go test -race ./internal/task/statussummary ./internal/task/repository/sqlite -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/task-summary-semantic-equality
```

Do not claim PostgreSQL was exercised by the SQLite suite. There is no SQL or
dialect change in this scope; if one becomes necessary, reconcile the package
and add the required real PostgreSQL check before implementing it.

## Files likely touched

- `apps/backend/internal/task/statussummary/model.go`
- `apps/backend/internal/task/statussummary/model_semantic_equality_test.go` (new)
- `apps/backend/internal/task/statussummary/projector_test.go` (test-store helper)
- `apps/backend/internal/task/repository/sqlite/task_status_summary_projector_noop_test.go` (new)
- This work order and `plan.md` (status and results).

Read-only integration points: `projector.go`, `projector_events.go`,
`projector_derive.go`, `rebuild.go`, and `repository/sqlite/task_status_summary.go`.

## Dependencies

None.

## Risks

Invalid data must reach existing validation rather than being hidden by failed
serialization. The revised fake store can reveal tests that previously assumed
every higher revision was accepted. Diagnose those assumptions rather than
weakening the persistence contract.

## Parallelism

`sequential`

## Inputs

- [Plan and reproduction evidence](plan.md).
- [Requirements](../../specs/platform/requirements/bounded-task-status-delivery.md), AC .2 and .3.
- [Design](../../specs/platform/system-design/bounded-task-status-delivery.md), derivation and persistence guarantees.
- Existing `model_test.go`, `projector_pending_authority_test.go`,
  `projector_queued_test.go`, and `task_status_summary_test.go` patterns.
- `.agents/skills/tdd/references/backend-tests.md`.

## Results

Implemented on 2026-09-17.

- `TaskStatusSummary.SemanticEqual` compares successful canonical semantic JSON
  encodings and rejects invalid summaries as equal.
- The projector test store now models both production rejection conditions:
  stale revisions and unchanged workspace/semantic payloads.
- Added model coverage for nil/empty omitted collections, metadata-only changes,
  real semantic changes, JSON round trips, and invalid values.
- Added a real SQLite projector regression covering live and recreated no-op PR
  updates and one true aggregate change.

Verification passed:

```text
(cd apps/backend && go test ./internal/task/statussummary ./internal/task/repository/sqlite \
  -run 'TestTaskStatusSummarySemanticEquality|TestTaskStatusSummaryProjectorPRNoopWithRetainedError' -count=1)
  2 packages passed.

(cd apps/backend && go test -race ./internal/task/statussummary ./internal/task/repository/sqlite -count=1)
  2 packages passed.

python3 scripts/list-docs.py validate
  Validated 285 decisions and 986 specifications.

python3 scripts/lint-spec-files.py --all
  All specification files passed.

git diff --check
  passed.
```
