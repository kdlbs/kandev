---
id: "01-fanout-skip-decided-and-author"
title: "Participant fan-out skips decided seats and the comment author"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-GATE-COMMENT-001
  - REQ-OFFICE-GATE-COMMENT-002
  - REQ-OFFICE-GATE-COMMENT-005
acceptance_criteria:
  - AC-OFFICE-GATE-COMMENT-001.1
  - AC-OFFICE-GATE-COMMENT-001.2
  - AC-OFFICE-GATE-COMMENT-001.3
  - AC-OFFICE-GATE-COMMENT-001.4
  - AC-OFFICE-GATE-COMMENT-001.5
  - AC-OFFICE-GATE-COMMENT-001.6
  - AC-OFFICE-GATE-COMMENT-001.7
  - AC-OFFICE-GATE-COMMENT-001.9
  - AC-OFFICE-GATE-COMMENT-001.10
  - AC-OFFICE-GATE-COMMENT-001.13
  - AC-OFFICE-GATE-COMMENT-001.14
  - AC-OFFICE-GATE-COMMENT-001.15
  - AC-OFFICE-GATE-COMMENT-001.16
  - AC-OFFICE-GATE-COMMENT-002.1
  - AC-OFFICE-GATE-COMMENT-002.2
  - AC-OFFICE-GATE-COMMENT-002.3
  - AC-OFFICE-GATE-COMMENT-002.4
  - AC-OFFICE-GATE-COMMENT-002.5
  - AC-OFFICE-GATE-COMMENT-002.7
  - AC-OFFICE-GATE-COMMENT-005.2
  - AC-OFFICE-GATE-COMMENT-005.3
system_design:
  - ../../specs/office/system-design/gate-comment-wake-01.md
---

# Task 01: Participant Fan-out Skips Decided Seats And The Comment Author

## Summary

Teach `queue_run_for_each_participant` two filters over its existing population:
drop the comment author's seat whenever the trigger is a comment, and, when the
new `skip_decided` config key is boolean `true`, drop every seat holding a
non-superseded decision at the step. Decision-read failure fails open with a
warning. A comment fan-out that fails returns an error wrapping a new
`ErrCommentFanOutIncomplete` sentinel, which the dashboard uses to suppress the
legacy runner wake while leaving the comment for the subscriber's one
redispatch. No template change here.

## In scope

- `SkipDecided` on `QueueRunForEachParticipantAction`, read in
  `readQueueRunForEachParticipantConfig` (boolean `true` only), and included in
  `queueActionDigest`.
- `Decisions DecisionStore` and `Logger` on `QueueRunForEachParticipantCallback`,
  set at both construction sites (`buildWorkflowCallbacks` and
  `engineOnEnterCallback`).
- The two filters in `Execute`, in the design's order, reusing `commentPayload`
  and `mapDecisionsToSeats`, preserving population order.
- Fail-open on decision read error or nil store with a warning carrying task,
  step, role and error.
- `ErrCommentFanOutIncomplete` in the engine package, wrapped with `%w` around
  every error `Execute` returns when `in.Trigger == TriggerOnComment`: missing
  `Adapter` or `Participants` (keeping `ErrActionNotYetWired` via multi-`%w`),
  missing role, the population-read error and the joined per-seat errors. The
  wrapped text names the task id, `in.Step.ID` and the role as `role %q`, so a
  missing role renders as `role ""` beside the kept inner text
  `queue_run_for_each_participant missing role` (AC-001.13).
- `DashboardService.dispatchCommentEngineTrigger` returns `handled` and
  `suppressAssigneeWake`; on the sentinel it warns (task, step, comment,
  error), publishes without `engine_dispatched` and passes
  `SkipAssigneeCommentWake = true`. Every other error keeps today's path
  (AC-001.14, .15): at a gated step that is every failure the fan-out did not
  report, namely the `IsOperationApplied` read, state or step load, an
  unregistered action kind, an `on_comment` action declared before the fan-out
  failing, and marking the operation applied.
- A test proving a comment-keyed request is not coalesced
  (`shouldCoalesceRun` returns false for a `commentkeys.TaskComment` key).

## Out of scope

- `office-default.yml`, the reconciler, and prompts (Tasks 02 and 03).
- Any change to `roleSeatsForFanOut`, the quorum guard, seat casting or the run
  service.
- Adding `AuthorType` to `OnCommentPayload`.
- Any change to the subscriber path (`handleCommentCreated`,
  `queueCommentRun`) or the channel-inbound producers (AC-001.16 is pinned by a
  test, not by code).
- Narrowing the assignee short-circuit in `isSelfComment` or `queueCommentRun`
  (kept by AC-OFFICE-GATE-COMMENT-002.7).

## Acceptance

- With `skip_decided: true`, a population of one decided and one undecided seat
  queues exactly one run, for the undecided seat, with `comment_id`,
  `author_id` and the authored `stage_type` in its payload; a seat whose only
  decision is superseded is woken.
- Under `on_comment` the author's seat is never queued, with or without
  `skip_decided`; a `"user"` author and an empty author exclude nobody; the same
  action under `on_enter` excludes nobody and skips nobody.
- A `ListStepDecisions` error, or a nil `Decisions` with `skip_decided`, queues
  every non-author seat and logs one warning; existing fan-out tests pass
  unchanged.
- Under `on_comment`, a population-read failure, a one-seat enqueue failure, a
  missing role and a nil `Adapter` each return an error satisfying
  `errors.Is(err, ErrCommentFanOutIncomplete)` whose text names the step id and
  the role (for the missing role: `role ""` and `missing role`); the
  nil-`Adapter` error also satisfies `ErrActionNotYetWired`. At a step whose
  `on_comment` list is a failing `queue_run` followed by the fan-out, and on an
  `IsOperationApplied` store error, the engine returns an error that does not
  satisfy the sentinel and the fan-out queues no run (AC-001.15). The same
  failures under `on_enter` do not wrap the sentinel.
- A dispatcher returning the sentinel: the dashboard queues no legacy assignee
  wake, still wakes an @-mention, publishes the event without
  `engine_dispatched` and logs one warning. Any other dispatcher error (for
  example a step-load or operation-marker failure at a gated step) keeps the
  legacy wake and publishes without `engine_dispatched` (AC-001.15).
- A comment published without `engine_dispatched` whose subscriber dispatch
  returns the sentinel is logged once and dispatched no second time
  (AC-001.16).
- With `GetTaskExecutionFields` failing, a runner-authored comment at a gated
  step reaches the fan-out and the runner's own seat is excluded as author
  (AC-002.7).

## Verification

```bash
# From the repository root:
cd apps/backend && go test ./internal/workflow/engine/... -race -count=1
cd apps/backend && go test ./internal/orchestrator/... -run 'QueueRunForEachParticipant|StepEntry' -race -count=1
cd apps/backend && go test ./internal/runs/service/... -run Coalesce -race -count=1
cd apps/backend && go test ./internal/office/dashboard/... -run Comment -race -count=1
cd apps/backend && go test ./internal/office/service/... -run Comment -race -count=1
make -C apps/backend lint
git diff --cached --name-only | grep '\.go$' | xargs -r gofmt -l
git diff --check
```

## Files likely touched

- `apps/backend/internal/workflow/engine/types.go`
- `apps/backend/internal/workflow/engine/phase2_callbacks.go`
- `apps/backend/internal/workflow/engine/queue_run_test.go` (or a new
  `queue_run_gate_comment_test.go`)
- `apps/backend/internal/orchestrator/workflow_callbacks.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/runs/service/service_test.go` (coalescing pin)
- `apps/backend/internal/office/dashboard/service_tasks.go`
- `apps/backend/internal/office/dashboard/service_tasks_test.go`
- `apps/backend/internal/office/service/event_subscribers_test.go` (or a new
  test file; AC-001.16 pin)

## Dependencies

None.

## Risks

- `Execute` is near the Go function-length limit; extract the filtering into a
  helper rather than growing it.
- `mapDecisionsToSeats` maps agent decisions by `(role, decider_id)`; tests must
  include a decision naming the seat by `participant_id` and one naming it only
  by decider identity.
- Changing `queueActionDigest` changes idempotency keys only for actions that set
  the new key; assert the digest of an action without it is unchanged.

## Parallelism

`parallel-safe` with Task 02.

## Inputs

- `docs/specs/office/system-design/gate-comment-wake-01.md`, sections "Fan-out
  selection", "Idempotency and concurrency", "Failure and recovery".
- `apps/backend/internal/workflow/engine/phase2_callbacks.go` (`roleSeatsForFanOut`,
  `queueRunPayload`, `idempotencyKey`, `QueueRunForEachParticipantCallback`).
- `apps/backend/internal/workflow/engine/quorum.go` (`mapDecisionsToSeats`).

## Results

Shipped. `SkipDecided`, `readQueueRunForEachParticipantConfig`, and the digest
salt landed in `internal/workflow/engine/types.go`. `wrapCommentFanOut`,
`filterFanOutSeats`, `excludeCommentAuthor`, and `excludeDecidedSeats` landed in
`internal/workflow/engine/phase2_callbacks.go`, wired with `Decisions`/`Logger`
at both construction sites (`orchestrator/workflow_callbacks.go`,
`orchestrator/event_handlers_workflow.go`). `DashboardService.CreateComment`'s
sentinel classification landed in `office/dashboard/service_tasks.go`. The
comment-coalescing pin landed in
`internal/runs/service/service_comment_coalesce_test.go`. All listed
Acceptance items verified against a real engine/dashboard/coalescing test run
in the Review step (see plan.md's REVIEW PHASE section); `go build ./...` and
`golangci-lint run ... --new-from-rev` both clean on this file set.
