---
id: "01-lifecycle-service"
title: "Sleep-inhibition lifecycle service"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/task-sleep-inhibition.md"
---

# Task 01: Sleep-inhibition lifecycle service

## Acceptance

- The typed install-wide setting defaults to disabled and round-trips through the shared System settings store.
- One owned reconciliation loop acquires exactly one injected lease only while enabled and at least one authoritative session satisfies `sessionstate.IsWorking`.
- Disable, final settle, unexpected lease exit, retry, startup recovery, repository errors, and graceful shutdown follow the spec without blocking task execution or leaking goroutines.

## Verification

```bash
cd apps/backend && go test ./internal/system/sleepinhibition -run 'Test(Store|Service)'
```

## Files likely touched

- `apps/backend/internal/system/sleepinhibition/types.go`
- `apps/backend/internal/system/sleepinhibition/store.go`
- `apps/backend/internal/system/sleepinhibition/service.go`
- `apps/backend/internal/system/sleepinhibition/store_test.go`
- `apps/backend/internal/system/sleepinhibition/service_test.go`

## Dependencies

None.

## Parallelism

Sequential; this task establishes the shared contracts consumed by every later task.

## Inputs

- Spec sections: What, Data model, State machine, Failure modes, Persistence guarantees.
- Plan sections: Sleep-inhibition lifecycle, Tests.
- Existing patterns: `apps/backend/internal/system/queuesettings/`, `apps/backend/internal/system/settings/store.go`, `apps/backend/internal/orchestrator/sessionstate/sessionstate.go`, and the goroutine ownership rules in `apps/backend/AGENTS.md`.

## Risks

- Event callbacks must only signal reconciliation; synchronous repository/native work in the memory bus publish path would delay task state transitions.
- Reconcile retries must be bounded and cancellation-aware.

## Output contract

Report the implemented contracts, files changed, exact test results, blockers/risks, and synchronized task/plan status in the primary conversation.

## Results

- Implemented typed settings persistence, event-coalesced reconciliation, retry,
  lease lifecycle, shutdown release, and repository-authoritative session
  filtering in `internal/system/sleepinhibition/`.
- Focused tests passed:
  `cd apps/backend && go test ./internal/system/sleepinhibition -count=1`.
- Race-focused service tests passed:
  `cd apps/backend && go test ./internal/system/sleepinhibition -race -run 'TestService' -count=1`.
