---
created: 2026-09-17
status: complete
requirements:
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001
system_design:
  - ../../specs/tasks/system-design/workflow-step-agent-start-ownership.md
  - ../../specs/tasks/system-design/task-launch-failure-recovery.md
legacy_specs: []
---

# Implementation Plan: Workflow asynchronous start prompt preservation

## Overview

Preserve an automatic workflow prompt after asynchronous startup failure, then deliver it through existing recovery and queue admission.
One implementation work order owns the complete backend path and its regression evidence.
The backend implementation and regression evidence are complete in this workspace.

## Evidence and root cause

Source: [issue #3753](https://github.com/kdlbs/kandev/issues/3753), with no comments or image attachments at investigation time.
Investigated checkout: `f32eb373781bcec12583c37e4a9d52b1ee12072f`.

The read-only control-flow trace confirms the prompt-loss path:

1. `autoStartStepPrompt` records the workflow prompt and calls `startCreatedSessionWithComposedPrompt` for a `CREATED` session.
2. `startAgentOnExistingWorkspaceWithRequest` starts the prepared execution asynchronously and returns a successful `TaskExecution`.
3. The caller therefore skips `handleCreatedAutoStartLaunchFailure`, which owns synchronous prompt preservation.
4. A later `StartAgentProcess` error reaches `handleAgentProcessStartFailure` and `handleAgentStartFailed` without the workflow prompt envelope.
5. Those handlers classify the error and settle state, but neither persists that workflow input in the queue.
6. `RecoverTaskLaunch` calls `RecoverSession(..., "fresh_start")`, which resumes the same session without reconstructing this missing queue entry.

The original [start-ownership package](../workflow-step-agent-start-ownership/plan.md) explicitly excluded this asynchronous case.
Its completed Task 02 covers synchronous busy errors only.
Current launch-recovery code already persists typed bootstrap errors and guards successor executions.
Thus, the source confirms prompt loss, but does not establish the reporter's exact idle-state sequence on this newer checkout.
No access to the reporter's runtime or private logs was required.

## Scope

### In scope

- Extend the existing workflow owner with `REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005`.
- Capture the exact pending workflow input before asynchronous launch admission.
- Preserve it once after an eligible failure, without starting another launch from that callback.
- Prove explicit recovery, boot-ready drain, metadata preservation, and stale-event rejection.

### Out of scope

- Provider start failures themselves, ACP reset support, and the agentctl running-process guard.
- New UI, automatic retry policy, historical orphan repair, and prompt replay after ambiguous provider acceptance.
- A crash before callback persistence, a new prompt ledger, schema migrations, and changes to Office scheduling.

## Technical approach

The [owning design](../../specs/tasks/system-design/workflow-step-agent-start-ownership.md#asynchronous-launch-prompt-preservation)
defines attempt capture, guarded queue persistence, and recovery.

Add the private envelope helper in `apps/backend/internal/orchestrator/workflow_start_prompt.go`.
Wire capture through `event_handlers_workflow.go` and `task_operations.go`.
Consume it through the accepted path in `event_handlers_agent.go`.
Separate queue persistence from automatic resume scheduling in `event_handlers_workflow.go`.
Preserve existing executor callbacks and dynamic route/ceiling bookkeeping.
Change `executor/executor_execute.go` only if propagation or final-attempt ordering needs correction.

The existing queue owns durable input after preservation. Context holds only the in-flight launch snapshot.
Existing queue ownership and launch-error decisions remain authoritative, so this package introduces no separate architectural decision.

## Tests

New file: `apps/backend/internal/orchestrator/workflow_start_prompt_test.go`.

| Acceptance | Planned test |
| --- | --- |
| 005.1, 005.2 | `TestWorkflowAsyncStartFailure_PreservesPromptThroughRecovery` |
| 005.1, 005.2 | `TestWorkflowAsyncStartFailure_PreservesPlanOnlyInputThroughRecovery`, `TestWorkflowAsyncStartFailure_PreservesConfigOnlyInputThroughRecovery` |
| 005.1, 005.2 | `TestWorkflowAsyncStartFailure_ExcludesTrulyEmptyInput` |
| 005.1, 005.2 | `TestWorkflowAsyncStartFailure_PreservesInputMetadata` |
| 005.3 | `TestWorkflowAsyncStartFailure_RejectsStaleAndDuplicateCallbacks`, including a committed move between the ownership read and queue insertion and leave-and-return to the same step |
| 005.3, 005.4 | `TestWorkflowAsyncStartFailure_DynamicFallbackOwnership` |
| 005.4, 005.5 | `TestWorkflowAsyncStartFailure_QueueFailureKeepsLaunchError` |
| 005.2, 005.5 | `TestWorkflowAsyncStartFailure_PersistedQueueSurvivesRestart` |
| 005.6 | `TestWorkflowAsyncStartFailure_SuccessAndSynchronousRejection` |

Before implementation, the first test failed because the queue remained empty
after asynchronous failure. The completed implementation now preserves the
entry and exercises delivery through recovery.
Existing executor bootstrap tests cover `AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.11` and `.12`.
Retain those checks for safe error projection and successor fencing.

## End-to-end evidence

The first regression uses real orchestrator and queue services with a controllable fake provider.
It drives workflow entry, delayed startup failure, the existing recovery action, boot-ready delivery, and the captured provider prompt.
This backend service flow covers the user outcome without a new browser interface or artificial browser fixture.
It must assert that no manual prompt is necessary after explicit launch recovery.

## Work orders

- [x] [Task 01: Preserve asynchronous workflow launch input](task-01-preserve-workflow-launch-input.md)

Execution is sequential. No subagents are authorized.

## Verification results

Planning evidence: read-only source trace confirmed the missing ownership transfer.
The existing synchronous regression passed:

```bash
(cd apps/backend && go test -tags fts5 ./internal/orchestrator -run '^TestAutoStartCreatedLaunch_QueuesPromptWhenAgentAlreadyRunning$' -count=1)
```

This test covers the earlier synchronous repair, not the proposed asynchronous fix.
Implementation regressions pass, including asynchronous recovery delivery,
effective plan-only and config-only input, true-empty exclusion, metadata
preservation, stale-callback fencing, the atomic move-admission barrier,
same-step re-entry fencing, queue failure handling, dynamic ownership, restart
persistence, and synchronous rejection behavior. Workflow entry admission
captures the launch-time task transition, session incarnation, and lifecycle
generation, then validates them inside the shared task/queue transaction. The
queue retains the raw prompt and mode metadata, so recovery composes each
effective prompt exactly once.

Package checks:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans/workflow-async-start-prompt-preservation
git status --short -- docs/plans/workflow-async-start-prompt-preservation
```

Results on 2026-09-17: catalog validation passed for 285 decisions and 986 specifications.
All 36 linter tests passed. All specification files passed, and the diff whitespace check passed.
The implementation package is included in this pull request.
GitHub confirmed `carlosflorencio` as the issue assignee.

## Risks

- Process-start failure callbacks can precede launch return or arrive after a successor starts.
- Dynamic profile fallback must not queue input while another candidate owns the same launch.
- Calling the existing queue helper unchanged can schedule recovery before failed-process cleanup.
- A full queue or storage error prevents preservation. The existing launch error must remain visible.
- Restart protection begins after queue persistence. This package does not claim crash-safe admission before that boundary.
