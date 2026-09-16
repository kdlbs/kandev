---
created: 2026-09-16
status: implemented
requirements:
  - REQ-OFFICE-RUNTIME-001
system_design:
  - ../../specs/office/system-design/runtime-01.md
legacy_specs: []
---

# Implementation Plan: Retry classified-transient legacy post-start failures

## Overview

Close "Hole 1" of the Beta unattended-recovery review: the legacy
(non-routing) post-start failure path, `HandleAgentFailure`, had no retry at
all — one transient blip counted as a full strike toward auto-pause. One
sequential work order adds a bounded, classified-transient retry to that path
before it can count toward `consecutive_failures`.

## Evidence and root cause

`handleAgentFailed` tries the routing tier (`tryPostStartFallback`) first and
falls through to `HandleAgentFailure` when there is no dispatcher or no
`ResolvedProviderID`. `HandleAgentFailure` (`office/service/failure.go`)
incremented `consecutive_failures` immediately on every failure, with no
classification step, so a single transient upstream blip on that fallthrough
path auto-paused the agent at the same rate as a genuine terminal failure.
The working routing-tier retry (`office/scheduler/routing_lifecycle.go`) and
pre-launch retry (`office/service/retry.go`) already handle their own paths
correctly and are unchanged.

## Scope

### In scope

- A bounded classified-transient retry on the legacy post-start path only,
  reusing the existing routing classifier's `ClassTransient && AutoRetryable
  && FallbackAllowed` predicate (no second classifier).
- A claim-guarded write so the retry can never resurrect a run the claim path
  has already cancelled for staleness.
- Regression coverage for the new eligibility gate, the scheduled-arrival
  staleness check, agent id / `providerError.ProviderID` substitution, and the
  `claimed -> failed -> queued` race this closes.

### Out of scope

- Hole 2 (automatic unpause) — blocked on card `497b1f63` (broken manual
  unpause); a product decision, not part of this change.
- Hole 3 (reaper for runs stranded in `claimed`) — already shipped on `main`
  as `recoverStaleClaimedRuns`.
- The pre-launch retry tier, the routing-tier retry policy, and the
  `blocked_provider_action_required` park — all unchanged by design.
- `apps/web` — no frontend surface for this change.

## Technical approach

Follow the [system design](../../specs/office/system-design/runtime-01.md#legacy-post-start-transient-retry).
Add `routingerr.(*Error).ShouldShortRetry()` (nil-safe) as the single shared
transient predicate; `officeShortRetryAllowed` in the scheduler delegates to
it so there is exactly one definition for both callers (`office/scheduler`
already imports `office/service`, so the shared predicate lives in
`routingerr`). `HandleAgentFailure` calls a new `tryLegacyTransientRetry`
first; `MarkRunFailed` only runs when that returns false.

`tryLegacyTransientRetry` eligibility: `RetryCount < 2`, the existing 24h
`retryMaxAge`, and a scheduled-arrival gate that refuses the retry (falling
through to terminal accounting) when `age + delay` already exceeds the 2h
`staleRunThreshold` the claim path uses to cancel stale runs — otherwise the
run could age into the cancellation window during its own backoff and the
failure would be silently lost instead of counted. The provider error's
`ProviderID` (or the caller's `AgentID`) is threaded into the classifier
before the predicate runs.

The write path is a new `ScheduleRetryIfClaimed` (`WHERE id=? AND
status='claimed'`, returns affected-row count) instead of the unguarded
`ScheduleRetry`. On success the run goes `claimed -> queued` directly and
never visits `failed`. Zero affected rows or a DB error falls through to the
existing `MarkRunFailed`, whose identical guard no-ops the same way — this is
what closes the `claimed -> failed -> queued` race a plain
claim-then-schedule ordering would leave open.

Backoff is 5s then 10s, capped at 2 retries, sharing the existing
`retry_count` column with the pre-launch tier (`MaxRetryCount = 4`), so the
combined budget can never be exceeded and no new column is needed.

## Tests

- `failure_transient_retry_test.go`: eligibility (retry-count cap, 24h
  staleness, scheduled-arrival gate), agent id / `providerError.ProviderID`
  substitution, and the classifier delegation (16 tests).
- `runs_queue_test.go`: `TestScheduleRetryIfClaimed*` covering the
  claimed-only guard and its zero-rows fallthrough (2 tests).
- `routingerr/short_retry_test.go`: table test for `ShouldShortRetry` across
  the classifier's boolean combinations, nil-safe (1 table, 5 cases).

## End-to-end evidence

No `apps/web` files change (`git diff --name-only origin/main...HEAD` — 13
files, 0 under `apps/web/`), so no Playwright coverage is required. Backend
package tests exercise the real SQLite-backed run repository, not mocks.

## Work orders

- [x] [Task 01: Retry legacy post-start transient failures](task-01-retry-legacy-post-start-failures.md)

## Verification results

Implementation completed 2026-09-16, PR
[#3725](https://github.com/kdlbs/kandev/pull/3725).

- `go build ./...`: clean. `gofmt -l` over changed `.go` files: empty.
- `make -C apps/backend lint`: "0 issues."
- `make typecheck`: exit 0. `make lint-format`: clean.
- `go test -tags fts5 -count=1 ./internal/office/... ./internal/runs/...
  ./internal/agent/runtime/routingerr/...`: every touched package passes.
- `python3 scripts/list-docs.py validate`: "Validated 278 decisions and 952
  specifications." `lint-spec-files.py --all`: all passed.
- `python3 scripts/lint-architecture.py --all`: clean.
- Full `make -C apps/backend test`: 15 pre-existing FAIL packages, none in a
  package this diff touches (standing KB clusters, unrelated to this change).

See the [work-order results](task-01-retry-legacy-post-start-failures.md#results)
for the full command list and receipts.

## Risks

The retry re-drives the whole run from the top and may repeat any side
effects the failed attempt already committed — no effect-safety gate applies
to it, matching the routing tier's own post-start requeue. This is a known,
accepted residual, not a defect: closing it is a larger effect-idempotency
change outside this card's scope.

## Documentation impact

`docs/specs/office/requirements/runtime.md` gained AC-OFFICE-RUNTIME-001.9;
`docs/specs/office/system-design/runtime-01.md` § Legacy post-start transient
retry was rewritten to match the implementation. No public-docs or operator-
facing surface changed.
