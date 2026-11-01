---
id: "04-settle-on-process-end"
title: "Settle workloads when the process ends"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-BACKGROUND-WORK-010
acceptance_criteria:
  - AC-AGENTS-BACKGROUND-WORK-010.1
  - AC-AGENTS-BACKGROUND-WORK-010.5
system_design:
  - ../../specs/agents/system-design/background-work.md
---

# Task 04: Settle workloads when the process ends

## Summary

Every path that ends the owning agent process settles open workloads, so rows cannot stay `running` after replacement, idle reclaim, or restart.

## In scope

- Settle on the owned-teardown branch of `handleAgentStoppedLocked`, bound to the stopped execution ID. When a lifecycle owner holds the fence, it settles in place before relaunch; otherwise the event handler defers to a bounded service-owned worker that rechecks liveness and execution identity.
- Settle after `reclaimIdleSession` cleanup succeeds.
- Write a test for startup settlement with a dead process. If it fails because `RowLiveness` is unknown before recovery, move settlement after recovery. If it passes, correct the design text.

## Out of scope

- Live completion evidence (Task 05).
- Changing the liveness probe contract.

## Acceptance

- The three paths settle both rows of a fixture matching this session, with an unknown outcome.
- A live or failed probe and a retained successor leave work open.
- The job count clears in the browser on desktop and phone after replacement.

## Verification

```bash
(cd apps/backend && env -u KANDEV_DEBUG_AGENT_MESSAGES go test -trimpath -tags fts5 ./internal/orchestrator -run 'BackgroundWork|IdleReclaim' -count=1)
(cd apps/backend && make build-mock-agent)
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/task/background-work-process-end.spec.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/task/background-work-process-end.spec.ts
```

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/reconcile_liveness.go`
- `apps/backend/internal/orchestrator/event_handlers_background_work.go`
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/background_work_settlement_test.go`
- `apps/web/e2e/tests/task/background-work-process-end.spec.ts`

## Dependencies

None

## Risks

- Settling during replacement could end work the successor owns. Settlement takes the stopped execution ID and holds the lifecycle fence.

## Parallelism

`sequential`

## Inputs

- [Background work design](../../specs/agents/system-design/background-work.md)
- `background_work_settlement_test.go`

## Results

Done, with a corrected startup conclusion.

- `handleAgentStoppedLocked` no longer waits for the session lifecycle lock. Its
  owned-teardown branch makes one opportunistic attempt and otherwise defers to
  a bounded service-owned worker. The worker's lock wait is cancellable, and
  `Service.Stop` cancels and drains it. A deferred attempt that observes a live
  successor leaves work open for a later verified process end.
- The two prompt-readiness replacement call sites hold that lock across the
  stop. `reapPromptUnreadyExecution` now settles in place after the stop
  succeeds and before the caller relaunches, covering both
  `ensureSessionRunningWithAttemptAndBinding` and `recoverAlreadyRunningResume`.
- The coordinator retry stop settles after its synchronous `StopExecution`.
  Workflow profile-switch parking attempts settlement after `StopAgent`, and
  the matching terminal stop-intent event also attempts settlement after
  asynchronous teardown completes. These paths retain the exact execution ID.
- **Startup correction:** the earlier conclusion that startup already settled a
  dead local row was wrong. It used a stubbed `RowLiveness`. The real
  `Manager.RowLiveness` returns Unknown until lifecycle recovery completes, and
  `RowProcessLiveness` returns Unknown once `LocalPID` is cleared. Startup
  reconciliation terminalizes a row whose runtime recovery found no owner and
  clears `LocalPID` in the same write, so the incident's row reached settlement
  as `status=failed, local_pid=0` and settled nothing. The fix treats an ended
  row status as persisted stop evidence only after the agent probe confirms no
  process is running. The startup test now uses the real `RowProcessLiveness` classifier
  and the post-reconciliation row shape (execution-less lookup, failed status,
  cleared PID); it fails without the fix. Idle reclaim remains the backstop for
  a runtime released without a restart.
- `background_work_settlement_paths_test.go` covers owned teardown, a live
  successor that keeps work open, a failed liveness probe, idle reclaim, and
  startup settlement of a reconciled dead row. A new regression test holds the
  lifecycle lock across a synchronously published `agent.stopped` for an owned
  execution, asserts no hang, and captures settlement before the lock owner
  releases the fence. Another test verifies service shutdown cancels a deferred
  worker. The held-lock regression also fails if the in-place reap settlement
  is removed.
- The browser flows for chromium and mobile-chrome pass: the active job count
  clears after the owning process is stopped and resumed, and history stays
  reachable.
- Review reruns passed the race-enabled settlement/lifecycle tests and both
  managed E2E process-end cases (chromium and mobile-chrome).
