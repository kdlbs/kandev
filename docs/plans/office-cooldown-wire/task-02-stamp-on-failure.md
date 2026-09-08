---
id: "02-stamp-on-failure"
title: "Stamp last_run_finished_at on agent failure"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/office/runtime.md"
---

# Task 02: Stamp `last_run_finished_at` on agent failure

## Root cause / resolved decision

`HandleAgentFailure` (`apps/backend/internal/office/service/failure.go:29-66`) marks the run failed
(`MarkRunFailed`) and releases the task checkout (`releaseTaskCheckoutForRun`) but never calls
`stampRunFinished`. Flagged by Greptile on PR #2902
(https://github.com/kdlbs/kandev/pull/2902#discussion_r3832467867) and confirmed real by reading the
code. PR #2902 deliberately scoped its own fix to `AgentCompleted`/`AgentStopped` (and their taskless
variants) only.

Resolved via the spec amendment in this plan (`docs/specs/office/runtime.md` "Retry policy"):
cooldown paces every terminal run outcome uniformly, including failure. Rationale: `cooldown_sec`
is a run-pacing mechanism keyed on "time since this agent last ran," not a success tracker — the
column name (`last_run_finished_at`, not `last_success_finished_at`) and the pre-existing spec
language ("cooldown between agent runs") already point this way. Once Task 01 wires the gate
correctly, an unstamped failure path would leave a below-threshold failing agent completely exempt
from pacing (unlike success/stop), which is the opposite of the safe default: a flapping provider or
a misconfigured agent could re-fire immediately after every failure with zero pacing, burning
cost/tokens on repeated attempts before the separate consecutive-failure/auto-pause threshold (a
different, coarser mechanism — unaffected by this change) eventually kicks in.

Note `HandleAgentFailure` is only reached when `tryPostStartFallback` (the routing-dispatcher retry
path, gated on `run.ResolvedProviderID` via `markShortProviderCooldown`) returns `false` — i.e. only
for a genuinely terminal failure, not a provider-routing retry. Stamping here does not double up with
or interfere with that separate, already-cooldown-aware retry path.

## Acceptance

- `HandleAgentFailure` calls `s.stampRunFinished(ctx, run)`, placed after
  `s.releaseTaskCheckoutForRun(ctx, run)` and before `IncrementAgentConsecutiveFailures`, mirroring
  the completed/stopped paths' ordering (terminal-mark -> release checkout -> stamp).
- Behavior is identical to the completed/stopped paths: no-op safely if `run == nil` or
  `run.AgentProfileID == ""` (already handled inside `stampRunFinished` — no new nil checks needed
  here).
- No change to the consecutive-failure counter, auto-pause threshold, or auto-pause behavior — this
  task only adds the cooldown stamp.

## Regression test

Extend the existing `TestSchedulerTick_AgentFailedReleasesTaskCheckout` in
`apps/backend/internal/office/service/scheduler_checkout_release_test.go` (does not currently assert
the runtime stamp — only checkout release) rather than duplicating its full setup in a new function.
Mirror the assertion already used by `TestSchedulerTick_AgentCompletedReleasesTaskCheckout` in the
same file:

1. Capture `beforePublish := time.Now().UTC()` before publishing the `events.AgentFailed` event
   (the test currently doesn't capture this — add it).
2. After the existing checkout-reacquisition assertion, add:
   ```go
   runtime, err := svc.GetAgentRuntimeForTest(ctx, agent.ID)
   if err != nil {
       t.Fatalf("get agent runtime: %v", err)
   }
   if runtime == nil || runtime.LastRunFinishedAt == nil {
       t.Fatal("expected last_run_finished_at to be stamped after AgentFailed, but it is still unset")
   }
   if runtime.LastRunFinishedAt.Before(beforePublish) {
       t.Errorf("last_run_finished_at = %v, want at/after %v", runtime.LastRunFinishedAt, beforePublish)
   }
   ```
3. Update the function's doc comment (currently states it is "the failure-path counterpart" only for
   checkout release) to also note it now covers the cooldown-stamp parity.

This assertion must fail before the code change (no stamp call exists yet -> `runtime` is nil or
`LastRunFinishedAt` is nil) and pass after.

## Verification

```bash
cd apps/backend && go test ./internal/office/service/... -run TestSchedulerTick_AgentFailed -v
cd apps/backend && go test ./internal/office/service/...
cd apps/backend && golangci-lint run ./internal/office/service/... --new-from-rev=HEAD~1 --timeout=5m
```

## Files likely touched

- `apps/backend/internal/office/service/failure.go`
- `apps/backend/internal/office/service/scheduler_checkout_release_test.go`

## Dependencies

None.

## Parallelism

Sequential. Marked parallel-safe with Task 01 at the plan level (disjoint files/tests, no shared
schema) but this repo's convention keeps `parallelism: sequential` as the per-task default; delegate
only on explicit user authorization.

## Inputs

- Spec: `docs/specs/office/runtime.md` "Retry policy" section and the new persistence-guarantee
  bullet / scenario (amended by this plan).
- Existing pattern: `TestSchedulerTick_AgentCompletedReleasesTaskCheckout` and
  `TestSchedulerTick_TasklessAgentCompletedStampsRuntime` in the same test file — both already
  assert the identical `GetAgentRuntimeForTest` / `LastRunFinishedAt` shape for the success paths.

## Risks

- None expected — `stampRunFinished` is already shared, already covered by its own nil/empty-ID
  guard, and already exercised by three other call sites. This is additive; no existing behavior
  changes.

## Output contract

Report the exact before/after stamping behavior, files changed, exact commands and results,
blockers/risks, then mark this task `done` and update its checkbox in `plan.md`.

## Completion report

- **Before**: `HandleAgentFailure` marked the run failed and released the task checkout but never
  called `stampRunFinished` — `office_agent_runtime.last_run_finished_at` stayed unset after a
  failure, exempting a below-threshold failing agent from cooldown pacing entirely. Reproduced by
  extending `TestSchedulerTick_AgentFailedReleasesTaskCheckout`, which failed pre-fix with
  `expected last_run_finished_at to be stamped after AgentFailed, but it is still unset`.
- **After**: `HandleAgentFailure` calls `s.stampRunFinished(ctx, run)` immediately after
  `s.releaseTaskCheckoutForRun(ctx, run)`, before the consecutive-failure counter logic — mirroring
  the completed/stopped paths' ordering. No change to the counter, threshold, or auto-pause behavior.
- Files changed: `apps/backend/internal/office/service/failure.go` (one-line call + doc comment),
  `apps/backend/internal/office/service/scheduler_checkout_release_test.go` (extended existing test
  with `beforePublish` capture + runtime-stamp assertions, updated doc comment).
- Commands and results:
  - `go test ./internal/office/service/... -run TestSchedulerTick_AgentFailed -v` — fails before the
    fix (stamp assertion), passes after (all subtests green).
  - `go test ./internal/office/service/...` — `ok`.
  - `golangci-lint run ./internal/office/service/... --new-from-rev=HEAD --timeout=5m` — `0 issues`.
  - Plan-level validation (`go test ./internal/backendapp/... ./internal/office/service/...
    ./internal/office/repository/sqlite/...`) — all `ok`.
- Blockers/risks: none. Behavior is additive and uses the already-shared, already-nil-safe
  `stampRunFinished` helper exactly as planned.
