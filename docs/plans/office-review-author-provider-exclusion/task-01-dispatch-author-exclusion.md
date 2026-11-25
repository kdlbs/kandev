---
id: "01-dispatch-author-exclusion"
title: "Exclude the author provider from seat-run routing"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-REVIEW-AUTHOR-EXCL-001
  - REQ-OFFICE-REVIEW-AUTHOR-EXCL-002
  - REQ-OFFICE-REVIEW-AUTHOR-EXCL-003
  - REQ-OFFICE-REVIEW-AUTHOR-EXCL-004
acceptance_criteria:
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-001.1
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-001.2
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-001.3
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-001.4
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-001.5
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-002.1
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-002.2
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-002.3
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-002.4
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-002.5
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-003.1
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-003.3
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-003.4
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-004.1
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-004.2
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-004.3
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-004.4
system_design:
  - ../../specs/office/system-design/review-author-provider-exclusion.md
---

# Task 01: Exclude the author provider from seat-run routing

## Summary

Backend half of the system design, §1 to §5: parse the flag, read the policy
and the author provider, skip the author in the resolver with a record, and
wire it into `DispatchWithRouting` with run events.

## Scope

1. `engine.EnsureParticipantSeatAction.ExcludeAuthorProvider` and the parser.
   Also `wfmodels.AuthorProviderExclusionRoles(StepEvents)`.
2. Repository: `GetWorkflowStepEvents` and `GetTaskAuthorProvider`
   (dialect-safe JSON extraction).
3. `routing.ResolveOptions.AuthorProvider`, `SkipReasonAuthorProvider`, and the
   enabled-loop skip.
4. `scheduler/dispatch_author_exclusion.go`. Wire it into `DispatchWithRouting`.
   Add the `skipOutcome` mapping and the `RouteAttemptOutcomeSkippedAuthorProvider`
   constant. Append a `routing.author_exclusion` run event for each applicable
   resolution.
5. Loader and export round-trip tests for the flag. Add a builtin assertion
   that `office-default` does not set it.

## Exclusions

Web UI (Task 02), workflow-editor controls, metrics, schema changes, and
casting changes.

## Implementation acceptance

- With order `[A, B]` and the last work-stage run on A, a seat run at a
  declaring step launches on B. With B degraded (auto-retry), it parks as
  `waiting_for_provider_capacity` with a `skipped_author_provider` attempt for
  A. With order `[A]`, it parks as `blocked_provider_action_required`.
- An undeclared step, a non-seat agent, or a run with no step in its payload
  dispatches exactly as before: no extra route attempts or events, and the
  existing dispatch tests pass unchanged.
- A work-stage revisit on B makes the next seat-run resolution exclude B. No
  author recorded gives a launch plus an `applied:false, reason:no_author_provider`
  event. A storage error returns a dispatch error and does not launch.

## Verification

```bash
cd apps/backend
go test -count=1 ./internal/office/routing/... ./internal/office/scheduler/... \
  ./internal/office/repository/sqlite/... ./internal/workflow/engine/... \
  ./internal/workflow/models/... ./config/workflows/...
cd ../..
bash scripts/lint-go-changed <changed-go-files>
```

## Likely files

- `apps/backend/internal/workflow/engine/types.go` (+ `ensure_participant_seat_test.go`)
- `apps/backend/internal/workflow/models/phase2.go` or a new `author_exclusion.go` (+ test)
- `apps/backend/internal/office/repository/sqlite/participants.go` or a new
  `author_provider.go` (+ test)
- `apps/backend/internal/office/routing/resolver.go`, `types.go` (+ `resolver_test.go`)
- `apps/backend/internal/office/scheduler/dispatch_routing.go`, new
  `dispatch_author_exclusion.go` (+ test)
- `apps/backend/config/workflows/loader_test.go`, `builtin_engine_test.go`
- `apps/backend/internal/workflow/models/export_test.go` (or the existing export round-trip test)

## Risks

- The scheduler's repo interface may need the two new methods added. Check the
  scheduler test fakes.
- `dispatch_routing.go` is near the Go function-length limit. Keep the new
  logic in its own file.

## Results

Implemented with TDD. The policy lives in
`scheduler/dispatch_author_exclusion.go`. The resolver skip is
`routing.ResolveOptions.AuthorProvider` plus `SkipReasonAuthorProvider`. The
repository reads are in `office/repository/sqlite/author_provider.go`. The
config helpers are in `workflow/models/author_exclusion.go`.

- Dispatch tests cover: launch on the non-author provider; a degraded
  alternative parks as waiting for capacity; author-only parks as requiring
  action; fallback never reaches the author; a retry re-applies the exclusion
  and records one event per resolution; no author recorded; routing disabled;
  three not-applicable shapes dispatch unchanged; corrupt step events fail the
  dispatch. A mutation check (dropping `opts.AuthorProvider`) fails three
  tests.
- The repository queries pass on SQLite, and the PostgreSQL twin
  `TestPostgresAuthorProviderQueries` passes against a disposable local
  PostgreSQL server.
- An E2E run found that Office reactivity wakes (assignment, comment) queue
  task runs without a workflow step, so a Work run started by assignment
  recorded no author. Fix (system design §2a):
  `runs/service.SetTaskStepResolver` stamps `queued_workflow_step_id` with the
  task's step at queue time, and the author lookup reads it after
  `workflow_step_id`. It deliberately does not reuse `workflow_step_id`,
  because that key opts a run into step-change staleness cancellation. It is
  covered by `TestQueueRun_StampsTaskWorkflowStepWhenMissing` and a repository
  case.
- The loader accepts the flag, the YAML export/import round trip preserves
  it, and no built-in template sets it.
- The verification command packages pass, and `scripts/lint-go-changed`
  reports 0 issues.
