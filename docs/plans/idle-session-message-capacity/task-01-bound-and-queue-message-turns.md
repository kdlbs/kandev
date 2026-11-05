---
id: "01-bound-and-queue-message-turns"
title: "Bound and queue message turns"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-SESSION-CEILING-001
  - REQ-AGENTS-SESSION-CEILING-003
  - REQ-TASKS-RESUME-PROMPT-QUEUE-003
acceptance_criteria:
  - AC-AGENTS-SESSION-CEILING-001.1
  - AC-AGENTS-SESSION-CEILING-001.2
  - AC-AGENTS-SESSION-CEILING-001.3
  - AC-AGENTS-SESSION-CEILING-001.6
  - AC-AGENTS-SESSION-CEILING-001.8
  - AC-AGENTS-SESSION-CEILING-001.9
  - AC-AGENTS-SESSION-CEILING-003.1
  - AC-AGENTS-SESSION-CEILING-003.2
  - AC-AGENTS-SESSION-CEILING-003.3
  - AC-AGENTS-SESSION-CEILING-003.4
  - AC-AGENTS-SESSION-CEILING-003.5
  - AC-AGENTS-SESSION-CEILING-003.6
  - AC-AGENTS-SESSION-CEILING-003.7
  - AC-AGENTS-SESSION-CEILING-003.8
  - AC-TASKS-RESUME-PROMPT-QUEUE-003.1
  - AC-TASKS-RESUME-PROMPT-QUEUE-003.2
  - AC-TASKS-RESUME-PROMPT-QUEUE-003.3
system_design:
  - ../../specs/agents/system-design/session-concurrency-ceiling.md
  - ../../specs/tasks/system-design/resume-prompt-queue.md
---

# Task 01: Bound and queue message turns

## Summary

Implement the complete capacity boundary for new message turns.
Direct requests must reserve capacity or persist pending input before success.
Automatic queue delivery must respect the same boundary for warm and cold runtimes.

## In scope

- Add typed bounded/override capacity intent without changing caller authority or provenance.
- Extend the shared controller with attempt-owned turn tickets and union population accounting.
- Keep tickets across cold startup and protect successor ownership from stale callbacks.
- Guard structured prompts, created-message launches, completed follow-ups, and orchestrated passthrough input.
- Integrate direct request acceptance with the existing durable queue receipt.
- Keep the agent-message queued response and entry ID, including full sender metadata.
- Preserve payload, attachment claims, prompt context, delivery IDs, and processed workflow triggers.
- Restore refused queue dispatches and suppress immediate refusal-driven tail drains.
- Register identity-bound message waits and retry through the existing capacity worker.
- Preserve Auto-run, WIP, cancellation, clarification, recovery, and terminal-session rules.
- Keep restart passive and support later authorized activation.

## Out of scope

- New settings, schema, task-level queue banners, or provider-specific schedulers.
- Raw terminal input and changes to parent-child interruption authority.
- Settings help, translations, public docs, and rendered E2E, owned by Task 02.

## Acceptance

1. Warm and cold bounded requests cannot start another session at full capacity.
   Concurrent messages and launches share one controller, including startup-to-turn reservations.
2. Accepted messages have one durable recipient-bound queue or dispatch owner.
   Retry preserves payload and order without duplicate transcript, workflow, or provider effects.
3. Capacity changes and release progress eligible waits. Paused, stale, terminal,
   or restart-only candidates remain blocked. Explicit overrides retain their audit behavior.

## TDD sequence

1. Add `TestMessageTurnCapacity_WarmIdleRefusesAtLimit` using the temporary reproduction's fixture described in the plan.
2. Run that test before production changes. Require failure because the mock provider receives input while population is full.
3. Add real-controller request tests named `TestMessageCapacityAdmission_*` and `TestMessageTaskCapacity_*`.
4. Add attempt-ownership and wait-registry tests named `TestMessageTurnCapacity_*`.
5. Implement the smallest complete path through the existing controller and queue.
6. Run the targeted checks below. Record each actual result.

Use a mixed population with running, starting, waiting, and terminal sessions.
Use two independent warm contenders for the final free slot.
Block startup confirmation while a turn ticket remains held.
Release an older ticket after its successor exists.
Exercise two queued sibling sessions, not only two tasks.

Cover release-before-registration, Auto-run OFF, queue-full and storage errors,
failed cold resume, and wrapped refusal.
Cover cancellation, queue removal, transfer/reset, and accepted or uncertain durable submissions.
Use synchronization barriers.

## Technical inputs

- Shared design: [Message-turn admission](../../specs/agents/system-design/session-concurrency-ceiling.md#message-turn-admission).
- Preserved contract: [Backend restart](../../specs/tasks/system-design/resume-prompt-queue.md#backend-restart).
- `promptTask` and `sessionAlreadyPromptReady` in `task_operations.go`.
- `sessionCeilingController`, `sessionKeyedCeilingReservation`, and `ceiling_release.go`.
- `executeQueuedMessageWithReservation` and `handleQueuedMessageExecutionError` in `event_handlers_agent.go`.
- `drainQueuedMessageForPromptableSessionLockedWithTaskAdmissionAndIdentity` in `event_handlers_workflow.go`.
- `ceilingSweepTick` and `SetSessionCapacity` for shared wake-up.
- `MessageHandlers.wsAddMessage`, `CreateQueuedMessageIdempotent`, and `AtomicQueuedPromptCoordinator` for durable request acceptance.
- `dispatchTaskMessage`, `promptPreparedTaskMessage`, and `resumeCompletedTaskMessageSession` for agent messages.
- `PromptTaskWithDeliverySubmissionID` and `dispatchPreparedPrompt` for acceptance identity.
- `queue.priorityRank` for priority reuse. Export the internal-package helper rather than copying its rank table.

Existing manual-over-ceiling prompt tests encode the behavior this draft changes.
Update those expectations in this work order. Add separate explicit-control cases
so the change does not remove the Start/Resume/Send Now override.
Do not change queue provenance to request a capacity exemption.

## Verification

Run these commands from the repository root:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator -run '^TestMessageTurnCapacity_WarmIdleRefusesAtLimit$' -count=1 -v)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator ./internal/orchestrator/messagequeue ./internal/orchestrator/queue ./internal/task/handlers ./internal/mcp/handlers -count=1)
(cd apps/backend && GOMAXPROCS=2 go test -trimpath -tags fts5 -race ./internal/orchestrator ./internal/task/handlers ./internal/mcp/handlers -run '^Test(MessageTurnCapacity|MessageCapacityAdmission|MessageTaskCapacity|CIAutomationIdlePrompt)' -count=1)
bash scripts/lint-go-changed $(git status --porcelain | awk '{print $2}' | grep '\.go$')
git diff --check
```

If extraction changes the owned file list, update the lint command to include
every changed Go file. Include changed existing tests before marking this task done.
No SQL changes are planned. If implementation needs dialect-sensitive SQL,
update this work order and add real PostgreSQL behavior coverage before proceeding.

## Files likely touched

All Go paths below are relative to `apps/backend/`.

- `internal/orchestrator/message_turn_capacity.go` (new)
- `internal/orchestrator/message_turn_capacity_wait.go` (new)
- `internal/orchestrator/message_turn_capacity_test.go` (new)
- `internal/orchestrator/message_turn_capacity_wait_test.go` (new)
- `internal/orchestrator/service.go`
- `internal/orchestrator/session_ceiling.go`
- `internal/orchestrator/ceiling_release.go`
- `internal/orchestrator/ceiling_sweep.go`
- `internal/orchestrator/task_operations.go`
- `internal/orchestrator/agent_delivery_submission.go`
- `internal/orchestrator/event_handlers_agent.go`
- `internal/orchestrator/event_handlers_workflow.go`
- `internal/orchestrator/queue_send_now.go`
- `internal/orchestrator/session_launch.go`
- `internal/orchestrator/peer_message_initial_launch.go`
- `internal/orchestrator/queued_dispatch.go`
- `internal/orchestrator/ci_automation_dispatch.go`
- `internal/orchestrator/dynamic_launch.go`
- `internal/orchestrator/ceiling_replay.go`
- `internal/orchestrator/event_handlers_automation.go`
- `internal/orchestrator/event_handlers_clarification.go`
- `internal/orchestrator/event_handlers_transient.go`
- `internal/orchestrator/event_handlers_transient_retained_runtime.go`
- `internal/orchestrator/provider_interruption_dispatch.go`
- `internal/orchestrator/initial_submission_recovery.go`
- `internal/backendapp/adapters.go`
- `internal/orchestrator/queue/queue.go` and `queue_test.go`
- `internal/orchestrator/ceiling_seam3_integration_test.go`
- `internal/task/handlers/message_handlers.go`
- `internal/task/handlers/message_capacity_admission_test.go` (new)
- `internal/mcp/handlers/handlers.go`
- `internal/mcp/handlers/message_task_capacity_test.go` (new)

Extract focused helpers instead of expanding the large existing orchestration files.
The primary session owns this whole work order and its shared plan status.

## Dependencies

None. Implement this vertical backend outcome before Task 02.

## Risks

- Reusing launch reservation release for turn tickets creates a startup admission gap.
- A late failure can corrupt successor accounting without an attempt token.
- Success before durable queue admission can lose input after a queue write error.
- Capacity refusal must not requeue a prompt already accepted by agentctl.
- Completed intent and explicit overrides must remain trusted internal decisions.

## Parallelism

`sequential`

## Results

Implemented in the primary session.

- Added typed bounded/override intent and attempt-owned turn tickets in `message_turn_capacity.go`, with union population accounting in `session_ceiling.go`.
- Guarded `promptTask` before warm readiness and cold resume; explicit Send Now, clarification, interruption, and accepted-continuation paths keep the override.
- Added the process-local capacity wait registry and sweep pass in `message_turn_capacity_wait.go`, including restored-head identity, sibling independence, priority/enqueue ordering, and completed-follow-up intent.
- Extended `message.add`, the agent-message handler, queued ordinary/lifecycle dispatch, and queued passthrough admission.
- Added regression tests for warm and cold refusal, concurrent final-slot admission, released/expired ticket rejection, queue restoration and release, handler queue acceptance, steer-fallback queueing, agent-message queued responses, pinned completed follow-up intent, prompt_ensure replay deferral, bounded CI auto-fix queueing, CREATED/completed Auto-run gating, a three-contender startup race, and transient identity/snapshot/session reads.

Red evidence came from the investigation's temporary reproduction, which failed at the intended provider-call assertion for both the manual and automatic warm paths. The permanent regression replaced that temporary test.

Verification actually run:

```text
go test -trimpath -tags fts5 ./internal/orchestrator ./internal/task/handlers ./internal/mcp/handlers ./internal/backendapp -count=1
ok  github.com/kandev/kandev/internal/orchestrator
ok  github.com/kandev/kandev/internal/task/handlers
ok  github.com/kandev/kandev/internal/mcp/handlers
ok  github.com/kandev/kandev/internal/backendapp

go test -trimpath -tags fts5 ./internal/orchestrator/messagequeue ./internal/orchestrator/queue -count=1
ok  github.com/kandev/kandev/internal/orchestrator/messagequeue
ok  github.com/kandev/kandev/internal/orchestrator/queue

GOMAXPROCS=2 go test -trimpath -tags fts5 -race ./internal/orchestrator ./internal/task/handlers ./internal/mcp/handlers -run '^Test(MessageTurnCapacity|MessageCapacityAdmission|MessageTaskCapacity|CIAutomationIdlePrompt)' -count=1
ok  github.com/kandev/kandev/internal/orchestrator
ok  github.com/kandev/kandev/internal/task/handlers
ok  github.com/kandev/kandev/internal/mcp/handlers

bash scripts/lint-go-changed $(git status --porcelain | awk '{print $2}' | grep '\.go$')
0 issues.

git diff --check
clean
```

The desktop and phone rendered scenarios are recorded in Task 02.
