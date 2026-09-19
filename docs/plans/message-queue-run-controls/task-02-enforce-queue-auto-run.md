---
id: "02-enforce-queue-auto-run"
title: "Enforce queue Auto-run"
status: done
wave: 2
depends_on: ["01-persist-queue-auto-run"]
plan: "plan.md"
spec: "../../specs/ui/requirements/message-queue-run.md"
---

# Task 02: Enforce queue Auto-run

## Intent

Make every automatic FIFO path honor the durable policy, expose the authorized
WebSocket mutation, and align Send Now, legacy drain, and explicit Cancel with
the confirmed resume/pause contract.

## Acceptance

1. `message.queue.auto_run.set` validates an explicit boolean, authorizes the
   session, persists OFF without cancelling, and persists ON while attempting
   an immediate head only when promptable. The response and status event carry
   exact `auto_run` and `dispatched` values.
2. ACP ready/boot-ready, passthrough, workflow, lifecycle, CI automation, and
   every other automatic FIFO reservation use the Task 01 policy-aware
   operation. OFF holds user, agent, workflow, and server entries alike.
3. Successful legacy drain resumes ON. Accepted entry/all Send Now claims
   project ON. Pre-claim failure preserves OFF, while asynchronous accepted
   restoration keeps ON.
4. Explicit user Cancel that leaves pending entries persists OFF before its
   guard is released. Internal and Send Now cancellation do not pause. Existing
   cancellation and workflow-completion guarantees remain intact.
5. Policy deferral is a successful queued outcome for lifecycle and CI
   producers, not a delivery error. Clarification and workflow guards remain
   authoritative even when ON.

## Verification

```bash
cd apps/backend
go test ./internal/orchestrator/handlers -run 'Test.*(AutoRun|Drain|SendNow|QueueStatus)' -count=1
go test ./internal/orchestrator -run 'Test.*(AutoRun|CancelAgent.*Queued|HandleAgentReady|HandleAgentBootReady|CIAutomation|Lifecycle)' -count=1
go test -race ./internal/orchestrator -run 'Test.*AutoRun.*Race|TestCancelAgent_RacesHandleAgentReady' -count=1
go test ./internal/orchestrator ./internal/orchestrator/handlers -count=1
```

## Files likely touched

- `apps/backend/pkg/websocket/actions.go`
- `apps/backend/internal/orchestrator/handlers/queue_handlers.go`
- `apps/backend/internal/orchestrator/handlers/queue_handlers_test.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/task_operations_test.go`
- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/event_handlers_test.go`
- `apps/backend/internal/orchestrator/event_handlers_queue_general_test.go`
- `apps/backend/internal/orchestrator/event_handlers_lifecycle_dispatch.go`
- `apps/backend/internal/orchestrator/ci_automation_dispatch.go`
- `apps/backend/internal/orchestrator/queue_send_now.go`
- `apps/backend/internal/orchestrator/queue_send_now_test.go`
- relevant workflow, lifecycle, CI, and passthrough test files discovered by
  the production-call-site audit

## Dependencies

Task 01. This task consumes its repository result distinctions and additive
status field.

## Parallelism

Sequential. It owns the protocol and orchestration behavior consumed by the
frontend.

## Inputs

- Spec: `API Surface`, `State and Concurrency`, `Failure Modes`, and scenarios.
- Existing guarded paths in `DrainQueuedMessage`, `handleAgentReady`,
  `drainQueuedMessageForPromptableSessionLocked`, and `queue_send_now.go`.
- Existing Cancel parking and race tests in `task_operations_test.go`.

## Risks

- Audit raw `ReserveQueued` calls, not only the shared drain helper.
- Do not turn a policy pause into a CI round failure or retry storm.
- OFF must not use agent cancellation or trigger workflow completion.
- Preserve the exact transaction boundary established in Task 01 rather than
  adding an orchestrator-level check-then-take race.

## Output contract

Report all audited automatic call sites, protocol behavior, Cancel/Send Now
results, files changed, exact commands and test counts, blockers, and residual
risks. Set this task to `done`, record results below, and synchronize `plan.md`
before Task 03.

## Results

Implemented `message.queue.auto_run.set`, guarded orchestration control, legacy
drain resume, and explicit-Cancel conditional pause. Automatic reservation is
policy-aware for ready, boot-ready, passthrough, workflow, lifecycle, and CI
paths. CI replacement distinguishes a policy pause from a failed delivery;
lifecycle dispatch already treats a no-op drain as accepted queued work.
Targeted peer takes remain explicit, while their FIFO fallback uses the shared
policy-aware helper. Accepted Send Now claims resume atomically; rejected
selections and internal/silent cancellation preserve prior policy.

Coverage added for ON/OFF, active turns, clarification, ACP/passthrough ready,
CI/lifecycle deferral, Cancel versus silent cancel, legacy drain, Send Now
restore/pre-claim failure, authorization, boolean validation, action
registration, unavailable service, and status responses.

Verification:

- `go test ./internal/orchestrator/handlers -run 'Test.*(AutoRun|Drain|SendNow|QueueStatus)' -count=1` passed.
- Focused Auto-run, Cancel, ready, CI, lifecycle, and Send Now orchestrator tests passed.
- `go test -race ./internal/orchestrator -run 'Test.*AutoRun.*Race|TestCancelAgent_RacesHandleAgentReady' -count=1` passed.
- `go test ./internal/orchestrator ./internal/orchestrator/handlers -count=1` passed (`413.234s`, `0.077s`).
