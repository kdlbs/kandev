---
id: "01-retry-legacy-post-start-failures"
title: "Retry legacy post-start transient failures"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-RUNTIME-001
acceptance_criteria:
  - AC-OFFICE-RUNTIME-001.11
system_design:
  - ../../specs/office/system-design/runtime-01.md
---

# Task 01: Retry legacy post-start transient failures

## Summary

Give the legacy (non-routing) post-start failure path,
`HandleAgentFailure`, the same bounded classified-transient retry the
routing tier and pre-launch tier already have, so one transient blip no
longer counts as a full strike toward `consecutive_failures` and auto-pause.

## In scope

- `routingerr.(*Error).ShouldShortRetry()` as the single shared transient
  predicate, reused by both the scheduler's `officeShortRetryAllowed` and the
  new legacy-path check.
- `tryLegacyTransientRetry` in `HandleAgentFailure`, tried before
  `MarkRunFailed`.
- `ScheduleRetryIfClaimed` (claimed-only guarded write) so the retry can never
  resurrect a run the claim path already cancelled for staleness.
- Agent id / `providerError.ProviderID` substitution into the classifier.
- Lifecycle run, execution, and prompt evidence must prove a current,
  effect-safe invocation before the retry is scheduled. Unknown or mismatched
  evidence falls through to terminal accounting.
- Regression tests for eligibility, the scheduled-arrival gate, and the
  `claimed -> failed -> queued` race.

## Out of scope

Hole 2 (auto-unpause, blocked on card `497b1f63`), Hole 3 (already shipped as
`recoverStaleClaimedRuns`), the pre-launch retry tier, the routing-tier retry
policy, the `blocked_provider_action_required` park, and `apps/web`.

## Acceptance

1. A classified-transient failure on the legacy post-start path (no
   dispatcher / no `ResolvedProviderID`, so `tryPostStartFallback` did not
   handle it) is retried up to 2 times at 5s then 10s before
   `consecutive_failures` increments, observed by `consecutive_failures`
   staying unchanged on the first classified-transient failure
   (AC-OFFICE-RUNTIME-001.11).
2. The retry shares its attempt counter with the pre-launch tier (no new
   column) and is abandoned under the same 24h staleness rule, so a run that
   already exhausted pre-launch retries gets no additional post-start retry.
3. The retry is refused — falling through to terminal accounting instead of
   being scheduled — when the run's scheduled arrival (age plus the delay
   about to be scheduled) already exceeds the 2h `staleRunThreshold` the claim
   path uses to cancel stale runs.
4. `ScheduleRetryIfClaimed` only mutates a run whose status is still
   `claimed`; a concurrent cancellation (zero affected rows) falls through to
   the existing `MarkRunFailed` guard instead of resurrecting a cancelled run.

## Verification

Run from `apps/backend`:

```bash
go test -tags fts5 -count=1 ./internal/office/... ./internal/runs/... ./internal/agent/runtime/routingerr/...
gofmt -l $(git diff --name-only origin/main...HEAD -- '*.go')
make lint
make typecheck
```

Repository-root document checks:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
python3 scripts/lint-architecture.py --all
```

## Files touched

- `apps/backend/internal/agent/runtime/routingerr/routingerr.go`,
  `short_retry_test.go`
- `apps/backend/internal/office/scheduler/routing_lifecycle.go`
- `apps/backend/internal/office/service/event_subscribers.go`,
  `event_subscribers_session_attribution_test.go`, `failure.go`,
  `failure_test.go`, `failure_transient_retry_test.go`,
  `scheduler_integration.go`
- `apps/backend/internal/runs/repository/sqlite/runs.go`,
  `runs_queue_test.go`
- `docs/specs/office/requirements/runtime.md`,
  `docs/specs/office/system-design/runtime-01.md`
- This work order and `plan.md`.

## Dependencies

None. Read `apps/backend/internal/office/AGENTS.md` before touching this
path again.

## Risks

The retry re-drives the whole run from the top only after the lifecycle event
identifies the exact run and proves that no output or effect was observed for
the current invocation. Events with missing or stale run identity, unknown,
output-producing, or effectful evidence fall through to terminal accounting.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/office/requirements/runtime.md#req-office-runtime-001-office-agent-runtime--error-handling-contract).
- [Design](../../specs/office/system-design/runtime-01.md#legacy-post-start-transient-retry).
- The existing routing-tier retry (`office/scheduler/routing_lifecycle.go`)
  and pre-launch retry (`office/service/retry.go`) as the working reference
  implementations.

## Results

Completed 2026-09-16. PR [#3725](https://github.com/kdlbs/kandev/pull/3725),
branch `feature/office-recovery-post-d86`, head `ce8f0da06`.

- `go test -tags fts5 -count=1 ./internal/office/... ./internal/runs/...
  ./internal/agent/runtime/routingerr/...`: all packages this diff touches
  pass, including the 16 `failure_transient_retry_test.go` cases, the 2
  `TestScheduleRetryIfClaimed*` cases, and the 5-case `ShouldShortRetry`
  table.
- `go build ./...`: clean. `gofmt -l` over changed `.go` files: empty.
- `make -C apps/backend lint`: "0 issues." `make typecheck`: exit 0.
  `make lint-format`: clean.
- `python3 scripts/list-docs.py validate`: "Validated 278 decisions and 952
  specifications." `python3 scripts/lint-spec-files.py --all`: all passed.
- `python3 scripts/lint-architecture.py --all`: clean.
- Full `make -C apps/backend test`: 15 pre-existing FAIL packages, zero in any
  package this diff touches (standing KB clusters, unrelated).
- Two additional pre-existing failures independently reproduced against a
  scratch worktree at bare `origin/main`
  (`TestMigrate_PriorityIdempotent`, `scripts/pr-await.test.sh`'s
  strict-deadline case) and excluded as not caused by this change.

No live instance, deployment, or database migration was touched. Production
code matches the rewritten system design section exactly; no subagents were
used for the implementation.
