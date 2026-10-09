---
id: "02-taskless-lifecycle"
title: "Taskless scheduler lifecycle"
status: done
wave: 2
depends_on: ["01-run-session-foundation"]
plan: "plan.md"
requirements:
  - REQ-OFFICE-TASKLESS-001
acceptance_criteria:
  - AC-OFFICE-TASKLESS-001.1
  - AC-OFFICE-TASKLESS-001.2
  - AC-OFFICE-TASKLESS-001.3
  - AC-OFFICE-TASKLESS-001.4
  - AC-OFFICE-TASKLESS-001.5
  - AC-OFFICE-TASKLESS-001.6
  - AC-OFFICE-TASKLESS-001.7
  - AC-OFFICE-TASKLESS-001.8
system_design:
  - ../../specs/office/system-design/taskless-run-sessions.md
---

# Task 02: Taskless scheduler lifecycle

## Summary

Wire genuine taskless sessions through both scheduler launch paths, then carry them through success, usage, failure, cancellation and restart. This task activates the capability only when the full lifecycle works.

## In scope

Own Office RunSessionLauncher wiring, routed candidates, session-bound JWT/context,
actual invocation identity, lifecycle/usage dedup, continuation, startup reconciliation,
stop inventory and deletion cleanup. Audit runtime event consumers for task assumptions.
Test every configured executor through the common launch request contract and run
an actual local mock-agent lifecycle, rather than accepting a fake launcher count.
Update docs/public/office-provider-routing.md for taskless identity and the relevant
scoped guidance. Keep the existing coordinator routine/defaults and historical failures.

## Out of scope

Other work orders, unrelated refactors, live-instance changes and publication.

## Acceptance

- Concrete and routed lightweight cron/manual runs execute a real prompt, finish and create no task rows; subsequent fire/retry has a fresh session and correct bounded continuation.
- Exact attempt events, usage and runtime outcomes are idempotent; failures, pause races and mixed live/failed stop inventories preserve routing and agent state.
- Restart and deletion cleanup stop or reconcile predecessors before replacement; no permanent claimed run or orphan process remains. Task-bound and idle/coalescing behavior stays covered.

## Regression evidence

Add TestTasklessRoutineCronToSessionCompletion in backendapp, TestTasklessRoutedFallback,
TestTasklessLateCompletionDoesNotFinishSuccessor, TestTasklessUsageDuplicate,
TestTasklessPauseDuringRegistration, TestTasklessPauseMixedStopResults and
TestTasklessRestartUnknownPredecessorBlocksReplacement. Existing wakeup/routine_e2e_test.go
stops before scheduler integration and is not sufficient evidence. Replace only the
unsupported-launch assertions in scheduler_taskless_launch_test.go; preserve terminal-CAS,
failure counters, event workspace identity and inbox flood protection where applicable.

## Verification

Run from the repository root; every command is independently rooted. New test files
named below are outputs of this work order. Record red/green evidence.

```bash
(cd apps/backend && go test ./internal/office/service ./internal/office/scheduler ./internal/office/pause ./internal/office/runtime ./internal/office/wakeup ./internal/office/costs -count=1)
(cd apps/backend && go test ./internal/backendapp -run 'Taskless|Routine.*Session|Office.*Scope' -count=1)
(cd apps/backend && go test ./internal/office/service ./internal/office/pause -run 'Taskless|RunSession' -race -count=1)
make build-backend
make build-web
(cd apps/web && pnpm e2e:run --project=chromium e2e/tests/office/taskless-routine-session.spec.ts)
node scripts/validate-public-docs.mjs
node --test scripts/validate-public-docs.test.mjs
git diff --check
```

## Files likely touched

- `apps/backend/internal/office/service/service.go`
- `apps/backend/internal/office/service/scheduler_integration.go`
- `apps/backend/internal/office/service/scheduler_taskless_launch_test.go`
- `apps/backend/internal/office/service/event_subscribers.go`
- `apps/backend/internal/office/scheduler/dispatch_routing.go`
- `apps/backend/internal/office/pause/sweep.go`
- `apps/backend/internal/office/pause/service.go`
- `apps/backend/internal/office/runtime/context_builder.go`
- `apps/backend/internal/backendapp/adapters_office.go`
- `apps/backend/internal/backendapp/office_taskless_run_session_test.go (new)`
- `apps/backend/internal/agent/runtime/lifecycle/`
- `apps/backend/internal/orchestrator/ (owner guards in event subscribers only)`
- `apps/backend/internal/office/agents/`
- `apps/backend/internal/office/workspaces/`
- `apps/backend/internal/office/costs/`
- `apps/web/e2e/tests/office/taskless-routine-session.spec.ts (new)`
- `docs/public/office-provider-routing.md`

## Dependencies

01-run-session-foundation

## Risks

Runtime.Start must actually start the process and deliver the prompt; Runtime.Launch alone cannot do this today. Pause inventory must include sessions for already cancelled runs. Never fall back to agent-only attribution for new events.

## Parallelism

`sequential`

## Inputs

Read linked requirements/designs in full, the plan evidence, scoped AGENTS.md,
TDD guidance and the adjacent existing tests before changing code. UI tasks also
read mobile-parity and E2E fixture guidance.

## Results

- Wired concrete and routed taskless scheduler launches through run-owned runtime sessions, including admission, first prompt, lifecycle identity, usage deduplication, continuation, pause cancellation and startup reconciliation.
- Added mixed stop/restart/deletion cleanup coverage and corrected task-owner admission so existing task-bound launches retain their contract while run-owned launches use run admission.
- Updated provider-routing documentation and scoped backend guidance.
- Backend Office suites, backendapp taskless coverage, race checks, public-doc validation and builds passed. The configured Postgres test remains unavailable because `KANDEV_TEST_POSTGRES_DSN` is unset.
