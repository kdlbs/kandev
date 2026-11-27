---
id: "03-slow-poll-idle-backoff"
title: "Back off unchanged slow-mode polling"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-LOCAL-REPOSITORIES-004
acceptance_criteria:
  - AC-WORKSPACES-LOCAL-REPOSITORIES-004.6
  - AC-WORKSPACES-LOCAL-REPOSITORIES-004.4
system_design:
  - ../../specs/workspaces/system-design/local-repositories.md
---

# Task 03: Back off unchanged slow-mode polling

## Summary

In slow mode, each polling loop lengthens its interval while its completed
scans observe no change, capped at 5 minutes. A change or any mode push
returns to 30 seconds. Fast and paused modes are unchanged.

This order proposes a cadence change to the documented 30-second unfocused
rate. It trades freshness of an unfocused badge (up to 5 minutes late after a
long quiet period) for fewer background Git processes. It is a separate commit
so it can be adjusted or dropped without the other orders.

## In scope

1. Add tests in `agentctl/server/process` before the change, driving the
   existing tick functions against a real temporary repository:
   - After consecutive unchanged slow-mode monitor scans, the slow file
     interval from `pollIntervals` follows 30, 30, 60, 120, 240, 300, 300
     seconds; a changed scan restores 30 seconds (baseline: always 30).
   - The Git poll loop backs off on its own counter and resets when HEAD moves.
   - A failed scan does not advance the backoff.
   - A same-mode `SetPollMode(slow)` push restores 30 seconds and signals both
     loops when an interval had been lengthened, and stays a no-op otherwise.
     A fast and back to slow transition also restores 30 seconds.
   - Fast and paused intervals ignore the backoff.
   - With both loops running, the same-mode push is received by each loop,
     which re-arms at 30 seconds without running a scan.
2. Add one idle counter per loop on `WorkspaceTracker`. Advance it from the
   completed-scan outcome in `monitorTick` and `gitPollTick`, where
   `checkGitChanges` now reports whether any tracked value moved.
3. Apply the schedule inside `pollIntervals` for slow mode only, so each loop
   keeps one interval source. Reset both counters on every `SetPollMode`
   push, before the same-mode return, and wake the loops when that shortened a
   lengthened slow interval.

## Out of scope

Fast and paused intervals, the startup grace, mode aggregation, focus wakeups,
turn-end final scans, and access-denied pausing.

## Acceptance

- The schedule, failed-scan, and same-mode wake tests fail on the baseline and
  pass after the change; the fast and paused test passes on both.
- Existing poll mode, grace, loop, monitor, and Git poll tests keep their
  baseline results.

## Verification

```bash
(cd apps/backend && go test -trimpath ./internal/agentctl/server/process -run '^TestSlowPoll' -count=1)
(cd apps/backend && go test -trimpath ./internal/agentctl/server/process -run '^Test(PollMode_IsValid|NewWorkspaceTracker_DefaultsToFast|SetPollMode_|PollIntervals|MonitorLoop_|PollModeGrace_|ManagerRescan|CheckGitChanges_|HandleUpstreamOnlyChange_|PollGitChanges_|OverlapGuard_|WorkspaceTracker_StopsWhen|WorkspaceTracker_MonitorExitsWhenNoGitRepo|WorkspaceTrackerAccessDeniedPausesUntilVisibleRetry|HandleGitPollFailure|GitPollSnapshot_)' -count=1)
(cd apps/backend && go build ./... && go vet ./internal/agentctl/server/process/...)
```

## Files likely touched

- `apps/backend/internal/agentctl/server/process/workspace_poll_mode.go`
- `apps/backend/internal/agentctl/server/process/workspace_monitor.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_poll.go`
- `apps/backend/internal/agentctl/server/process/workspace_tracker.go`
- `apps/backend/internal/agentctl/server/process/workspace_poll_backoff_test.go`

## Dependencies

None.

## Risks

An unfocused badge can lag a change by up to 5 minutes after a long quiet
period. The start and cap are local constants if a different bound is preferred.
No CPU reduction was measured for this change on the baseline.

## Results

Implemented on 2026-10-12 against `add376a2af`.

- RED (baseline polling files, new tests): the monitor schedule, Git poll
  schedule, failed-scan, and same-mode wake tests failed (`file interval = 30s,
  want 1m0s`, `git interval = 30s, want 1m0s`, `monitor loop was not woken`).
  `TestSlowPollBackoffLeavesFastAndPausedIntervals` passed on the baseline.
  The running-loop test was added after the RED run because it sets the new
  counters directly.
- GREEN: all six `^TestSlowPoll` tests pass.
- Neighbours: 32 listed tests pass and two skip on Windows.
  `TestMonitorLoop_FastToPausedStopsPolling` fails its fast-mode setup step
  (`expected fast mode to emit notification before pausing`) with the baseline
  polling files as well, so it is an existing Windows failure.
- `go build ./...` and `go vet ./internal/agentctl/server/process/` pass.
- Review follow-up: the running-loop test now observes each loop arming its
  timer through the nil-in-production `pollTimerArmed` hook: both loops arm at
  the lengthened interval, then re-arm at 30 seconds after a same-mode push,
  with no monitor scan and both idle counters still zero. With the same-mode
  wake disabled it fails (`did not arm their timer at 30s`); it passes three
  times under `-race`.

Review assessment on 2026-10-13: retained the explicit freshness trade-off.
Runtime interest selects slow mode only without focused UI interest; focus wakes
both loops, explicit refresh scans directly, and turn-end final refresh precedes
removal of runtime interest. Failed reads leave idle counts unchanged. The
running-loop test observes both timer arms without consuming their wake signals.
The public Git operations guide now documents the five-minute unfocused bound.
Full process-package, Windows, and integration verification runs in hosted CI.
