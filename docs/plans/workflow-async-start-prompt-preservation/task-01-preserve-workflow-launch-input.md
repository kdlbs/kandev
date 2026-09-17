---
id: "01-preserve-workflow-launch-input"
title: "Preserve asynchronous workflow launch input"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001
acceptance_criteria:
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005.1
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005.2
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005.3
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005.4
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005.5
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005.6
system_design:
  - ../../specs/tasks/system-design/workflow-step-agent-start-ownership.md
  - ../../specs/tasks/system-design/task-launch-failure-recovery.md
---

# Task 01: Preserve asynchronous workflow launch input

## Summary

Carry immutable workflow input across launch admission and persist it when that asynchronous launch fails before delivery.
Keep existing recovery actions, queue policy, failure projection, and executor cleanup authoritative.

## In scope

- Add the private attempt envelope and retire it on success or synchronous rejection.
- Bind task, session, execution evidence, turn, workflow entry, and final dynamic attempt ownership.
- Persist raw queue-form input once through the accepted failure path, without immediate auto-resume scheduling.
- Retain recorded-message identity, attachment ownership, references, handoff, origin, and plan mode.
- Add all tests named in the plan, including failure-to-recovery service coverage.

## Out of scope

New UI, public APIs, migrations, provider retries, and the existing post-start transient error mechanism.

## Acceptance

1. An asynchronous failure after successful launch return preserves the exact input and recovery delivers it once with one transcript row.
2. Stale, duplicate, terminal, archived, deleted, and superseded-entry callbacks cannot enqueue input or affect successor work.
3. Error projection, paused queues, synchronous failures, dynamic fallback, and success retain their established behavior.

## Regression procedure

1. Seed a `CREATED` session with a prepared execution and a step with reset plus automatic start.
2. Block the fake `StartAgentProcess` call with channels.
3. Drive workflow entry through the real service and prove the launch call returned without error.
4. Release the provider with a typed pre-delivery startup failure.
5. Await failure handling through explicit synchronization, then assert one preserved queue entry.
6. Before implementation, observe the expected red result: zero preserved
   entries.
7. Implement the ownership transfer, then recover through `RecoverTaskLaunch` and the normal boot-ready path.
8. Assert one provider prompt, one user message, and removal of the consumed queue entry.

Add table cases for generic, authentication, and managed-runtime bootstrap errors.
Exercise duplicate callback delivery and a failure before launch return.
Race replacement execution, same-execution successor turn, cancellation, completion, archive, deletion, and a later workflow entry.
Exercise a dynamic fallback candidate followed by success and a final failed candidate.
Cover combined handoff, attachment-only input, entity references, plan mode, and failed original message persistence.
Cover queue-full/write errors, a paused queue, and reopening the queue repository after preservation.
Verify no replay from successful startup or post-start prompt failure.

## Verification

Run these commands from the repository root after the TDD cycle:

```bash
(cd apps/backend && go test -tags fts5 ./internal/orchestrator -race -run 'TestWorkflowAsyncStartFailure|TestAutoStartCreatedLaunch|TestAutoStartTransientError_BootReadyDrainsOrphanedQueue|TestProcessOnEnterResetAgentContext' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/orchestrator/executor -race -run 'TestStartAgentProcessAsync|TestBootstrapFailure|TestBuildBootstrapLastAgentError' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/orchestrator/messagequeue -race -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/orchestrator/workflow_start_prompt.go` (new)
- `apps/backend/internal/orchestrator/workflow_start_prompt_test.go` (new)
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/executor/executor_execute.go` (only if callback propagation requires correction)
- `apps/backend/internal/orchestrator/executor/workflow_start_prompt_test.go` (new, if executor behavior changes)

## Dependencies

None. Existing synchronous preservation and typed bootstrap errors are present in the investigated checkout.

## Risks

The accepted failure path must preserve input before recoverable failure routing can expose a promptable session.
Guard acquisition must not deadlock the launch caller, callback, or queue drain.
Do not infer current ownership from mutable session-only caches.
Do not replace existing dynamic-route or capacity callbacks with the prompt-preservation callback.

## Parallelism

`sequential`

## Inputs

- [Requirement 005](../../specs/tasks/requirements/workflow-step-agent-start-ownership.md)
- [Asynchronous preservation design](../../specs/tasks/system-design/workflow-step-agent-start-ownership.md#asynchronous-launch-prompt-preservation)
- [Launch-error design](../../specs/tasks/system-design/task-launch-failure-recovery.md)
- Existing patterns: `event_handlers_duplicate_autostart_test.go`, `executor/executor_launch_failure_classification_test.go`, and `task_launch_recovery_test.go`.
- Existing queue rules: [server-owned auto-run](../../decisions/2026-08-16-server-owned-queue-auto-run.md).

## Results

Implemented the private immutable workflow-start attempt envelope and bound it
through initial-turn creation, prepared launch, dynamic process-start success,
and accepted asynchronous startup failure callbacks. Preserved input uses the
existing queue metadata and storage path, claims once, leaves auto-run policy
unchanged, and does not schedule a replacement from the failure callback.

Added service regressions for recovery delivery, effective plan-only and
config-only input, true-empty exclusion, duplicate and stale callback fencing,
metadata and failed-transcript preservation, classified bootstrap errors,
queue write and capacity failures, dynamic route ownership, paused queue
restart persistence, successful startup, and synchronous permanent rejection.
The stale-callback suite also commits a task move between the pre-admission
ownership read and queue insertion, and leaves and returns to the same step;
both cases assert that no obsolete queue row or dispatch is produced. The
production queue path captures the launch-time transition, session
incarnation, and lifecycle generation and validates them inside the shared
task/queue transaction. In-memory repositories fail closed for this strict
admission instead of claiming an atomic fence they cannot provide.

The effective-input regressions preserve empty raw queue content with mode
metadata, deliver one provider prompt, and assert one matching transcript
row/system block after recovery. A launch with no raw content, attachments,
references, handoff, plan mode, or session configuration remains excluded.
Admission capture failures now abort a prepared launch before the provider is
started, while legacy tasks without a transition row receive a durable current
entry before capture. Queue drains retain reserved entries when session
metadata reads fail, and recovery uses launch-time effective-input and
config-mode metadata rather than reclassifying a mode-only prompt from the
mutable session projection.

Verification passed:

```text
(cd apps/backend && go test -tags fts5 ./internal/orchestrator -race -run 'TestWorkflowAsyncStartFailure|TestAutoStartCreatedLaunch|TestAutoStartTransientError_BootReadyDrainsOrphanedQueue|TestProcessOnEnterResetAgentContext' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/orchestrator -race -run 'TestCaptureWorkflowStartPromptAdmission|TestDispatchTakenQueuedMessage_RetainsEntryWhenInputReadFails' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/orchestrator/executor -race -run 'TestStartAgentProcessAsync|TestBootstrapFailure|TestBuildBootstrapLastAgentError' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/orchestrator/messagequeue -race -count=1)
make -C apps/backend build
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```
