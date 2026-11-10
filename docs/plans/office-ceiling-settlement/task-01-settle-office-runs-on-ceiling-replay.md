---
id: "01-settle-office-runs-on-ceiling-replay"
title: "Settle Office runs across ceiling replay"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-SESSION-CEILING-001
acceptance_criteria:
  - AC-AGENTS-SESSION-CEILING-001.5
  - AC-AGENTS-SESSION-CEILING-001.10
system_design:
  - ../../specs/agents/system-design/session-concurrency-ceiling.md
---

# Task 01: Settle Office runs across ceiling replay

## Summary

Make the parked Office run follow the ceiling replay to a terminal outcome
without becoming a second launcher. The ceiling's stored `start` payload
carries the Office run identity; a successful replay claims and binds the
exact parked row, a terminal drop fails it, and lifecycle events recover a
failed replay write. Terminal events carry the exact run identity, and launch
failures enter Office's normal failure policy.

## In scope

- `causing_run_id` round-trip through the ceiling `start` deferral payload and
  replay options.
- Exact-row settlement on replay success (`queued` +
  `blocked_provider_action_required` + payload task + Office agent).
- Exact-row failure on a terminal ceiling drop.
- `AgentReady` recovery gated by the run's latest durable ceiling-deferral
  marker.
- Preserve a separate terminal run identity after the first ready event
  consumes `causing_run_id`; clear the terminal identity when a new prompt
  starts.
- Completion-before-ready ordering: a parked run is finished atomically when
  its completion arrives first; a ready-recovery claim is retried once through
  the normal completion path.
- Failure-before-ready ordering: settle the exact parked run and pass it to the
  normal Office failure policy. Error cleanup cannot finish it successfully.
- Keep the durable ceiling deferral until the Office caller accepts a drop
  settlement; write the one-shot card note only after the record clears.
- Check the run state before reading its event history for the deferral
  marker.
- Office routing design and public provider-routing documentation for the
  recovery owner and the manual-clear warning.

## Out of scope

- Office-side automatic retry, `waiting_for_session_capacity`, timed lift, or
  boot backfill.
- The taskless/execution-grade capacity policy (`#4715`), the explicit discard
  contract (`#4508`), and unrelated style suggestions on the same PR.

## Acceptance

- `AC-AGENTS-SESSION-CEILING-001.5`: the replay sweep remains the only replay
  owner; the Office run is settled from its stored identity without adding a
  second retry path.
- `AC-AGENTS-SESSION-CEILING-001.10`: an admitted replay binds its session to
  the run, a dropped start fails the run, and no run is left claimed after its
  execution has stopped.
- Terminal settlement uses the exact launch run identity and cannot affect a
  successor run on the same task and agent.
- A pre-ready launch failure follows Office failure policy. A failed drop
  settlement leaves the durable ceiling deferral available for retry.

## Changed paths

- `apps/backend/internal/office/repository/sqlite/run_routing.go`
- `apps/backend/internal/office/service/ceiling_deferred_settlement.go`
- `apps/backend/internal/office/service/event_subscribers.go`
- `apps/backend/internal/agent/runtime/lifecycle/`
- `apps/backend/internal/orchestrator/watcher/`
- `apps/backend/internal/office/service/taskless_completion.go`
- `apps/backend/internal/office/scheduler/dispatch_routing.go`
- `apps/backend/internal/office/service/scheduler_integration.go`
- `apps/backend/internal/orchestrator/ceiling_seams.go`
- `apps/backend/internal/orchestrator/ceiling_replay.go`
- `apps/backend/internal/orchestrator/ceiling_replay_result.go`
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/backendapp/main.go`
- `docs/specs/office/system-design/routing-01.md`
- `docs/public/office-provider-routing.md`

## Validation

- Focused tests passed for terminal lifecycle identity, stale stop and failure
  settlement, repository error guards, event decoding, and retryable drop
  settlement.
- `python3 scripts/list-docs.py validate` and
  `python3 scripts/lint-spec-files.py --all` passed.
- The broader five-package test run reached its 10-minute timeout in the
  unrelated `TestStartupReconciliationPreservesOnlyExactPendingRemoteOwner`
  test while it waited in SQLite.
