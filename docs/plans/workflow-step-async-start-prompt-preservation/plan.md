---
created: 2026-09-17
status: complete
requirements:
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005
system_design:
  - ../../specs/tasks/system-design/workflow-step-agent-start-ownership.md
legacy_specs: []
---

# Implementation Plan: Preserve the Step Prompt Across an Asynchronous Start Failure

## Overview

A workflow step whose `on_enter` runs `reset_agent_context` then
`auto_start_agent` records its prompt in chat history and hands it to the
executor as the execution description. The subprocess is started
asynchronously, so a start that fails after the launch call returned drops the
prompt: the session sits idle until a human retypes it.

This plan adds a single-use pending step-prompt handle that the asynchronous
failure path can read, and consumes it in exactly one of three places: a
synchronous failure, a successful start, or an asynchronous failure.

## Confirmed root cause

- `executor.startAgentOnExistingWorkspaceWithRequest`
  (`apps/backend/internal/orchestrator/executor/executor_execute.go:2454`) calls
  `startAgentProcessAsync` and returns a non-nil execution with a nil error.
- The prompt exists only as the execution description
  (`SetExecutionDescription`, same file, line 2407) and in
  `autoStartStepPrompt`'s local variables. Both die with the failed execution.
- `Executor.handleAgentProcessStartFailure`
  (`executor_execute.go:212`) and `Service.handleAgentStartFailed`
  (`apps/backend/internal/orchestrator/event_handlers_agent.go:3389`) take
  `(ctx, taskID, sessionID, agentExecutionID, err, fromResume)` and carry no
  prompt.
- Every call site of `queueAutoStartPrompt` is inside `autoStartStepPrompt`'s
  synchronous scope
  (`event_handlers_workflow.go:5591`, `:5873`, `:5888`, `:6138`), so nothing in
  the asynchronous failure path can preserve the prompt.
- The repository's own requirements document recorded this as a deliberate
  known gap left open by PR #2263, which fixed only the synchronous path.

## Scope

### In scope

- A single-use, per-session pending step-prompt handle owned by the
  orchestrator service.
- Arming the handle in the `CREATED` branch of `autoStartStepPrompt`.
- Consuming it on synchronous failure, on successful process start, and on
  asynchronous start failure.
- Queue-only preservation on asynchronous failure, with no agent start and no
  scheduled auto-resume.
- Backend tests for the asynchronous failure, the success discard, the
  synchronous path regression from PR #2263, and the superseded/stale/cancelled
  non-consumption cases.

### Out of scope

- Durable (cross-restart) storage of the pending prompt. A backend restart
  between launch and start outcome is owned by session recovery.
- Changing the start-failure projection, `FAILED` semantics, or the recovery
  surfaces touched by PRs #3315, #3610 and #3669.
- Issue #3336 (a step with no `auto_start_agent` configured) — a different
  trigger, explicitly not folded in.
- Frontend work. PR #3420 edits the same requirements document for an editor
  change and is independent of this fix.
- Passthrough sessions and the non-`CREATED` `promptTask` retry loop, which
  already own their queueing.

## Work orders

| Work order | Summary |
| --- | --- |
| [task-01-preserve-async-start-prompt.md](task-01-preserve-async-start-prompt.md) | Add the pending step-prompt handle and consume it across the three start outcomes. |

## Validation

- `make -C apps/backend test`
- `make -C apps/backend lint`
