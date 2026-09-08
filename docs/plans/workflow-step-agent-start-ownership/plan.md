---
spec: docs/specs/tasks/requirements/workflow-step-agent-start-ownership.md
created: 2026-08-05
status: complete
---

# Implementation Plan: Workflow Step Agent Start Ownership

## Overview

Two small, ordered changes in `internal/orchestrator/event_handlers_workflow.go`.
Task 01 removes the double start at its source by making context reset a no-op
for sessions that have never been prompted. Task 02 closes the prompt-loss hole
the same failure exposed, by queueing the recorded auto-start prompt when the
`CREATED` launch fails — reusing the queue-and-drain machinery that already
exists for the sibling `PromptTask` branch.

Both tasks edit the same file, so they are sequential. No schema, contract, API,
or persistence change.

## Confirmed root cause

On entering a step whose `on_enter` has both `reset_agent_context` and
`auto_start_agent`:

1. `resetAgentContext` (`event_handlers_workflow.go:2137`) early-returns only
   when there is no agent execution ID. A `CREATED` session with a
   workspace-only execution therefore reaches `ResetAgentContext`, whose
   reset-unsupported fallback **starts** the subprocess and mints a fresh ACP
   session.
2. `markIdleAfterReset` (`:2117`) flips only `RUNNING`/`STARTING`, so the
   session stays `CREATED`. Its comment asserts the opposite invariant.
3. `autoStartStepPrompt` (`:1680`) reads `CREATED`, records the prompt, and
   calls `StartCreatedSession` → `StartAgentProcess` on the running execution.
4. `Manager.Configure`
   (`internal/agentctl/server/process/manager.go:1388`) rejects with
   `cannot configure while agent is running`.
5. `handleAgentProcessStartFailure`
   (`internal/orchestrator/executor/executor_execute.go`) marks the session
   `FAILED` and force-stops the agent. The `CREATED` branch's only recovery is
   `requeueTaken()`, which restores a taken handoff message and not the prompt
   recorded by `recordAutoStartMessage`, so the prompt is dropped.
6. Recovery's `session/load` for that ACP session fails `-32002` (it died with
   the killed process); the fallback `session/new` boots an idle, unprompted
   agent and the session parks in `WAITING_FOR_INPUT`.

Evidence: backend logs from a local run, spanning ~13 seconds from the step
move to the idle boot, plus the orphaned prompt row left in
`task_session_messages` with no matching delivery.

Confirming detail: the execution was created workspace-only hours earlier and
has no `StartAgentProcess` entry before the step move — the reset path is what
started it.

## Backend

### Task 01 — reset is a no-op for never-prompted sessions

`resetAgentContext` gains a `session.State == CREATED` early-return, before the
execution lookup, returning `true` (success, nothing to do). This restores the
invariant `markIdleAfterReset` already documents and leaves `auto_start_agent`
as the single starter.

Safety: a `CREATED` session has no agent conversation to clear, and the process
auto-start launches begins on a fresh ACP session regardless. Nothing is lost by
skipping.

Existing coverage is unaffected: `seedSession`
(`event_handlers_test.go:637`) seeds sessions as `RUNNING`, so
`TestProcessOnEnterResetAgentContext`'s restart assertions keep exercising the
unchanged path. The regression test sets `CREATED` explicitly.

### Task 02 — a recorded auto-start prompt is never dropped

**Scope corrected during implementation.** The plan assumed
`StartCreatedSession` surfaces the incident's failure synchronously. It does
not: `startAgentOnExistingWorkspace` ends with `startAgentProcessAsync` and
returns `nil`, and its log line "agent starting on existing workspace" is the
one immediately preceding the async failure in the production trace. So the
`CREATED` branch's error clause was never reached for the reported bug.

The synchronous branch is still reachable — and still drops the prompt — when
there is no in-memory execution (the post-restart shape), where
`startAgentOnExistingWorkspace` returns `ErrStaleExecution` and the full
`LaunchAgent` path runs. Task 02 closes that case: a busy or already-running
error queues the prompt via `queueAutoStartPrompt(..., userMsgRecorded, ...)`,
mirroring the selectivity of the `PromptTask` retry loop below it. Permanent
rejections are not queued. The queued prompt is drained by the existing
`handleAgentBootReady` drain that
`TestAutoStartTransientError_BootReadyDrainsOrphanedQueue` already pins, and
the error is still returned so `FAILED` state and surfacing are unchanged.

The async path remains uncovered — see the spec's **Known gap**. Task 01
removes the only known trigger for it on a first-turn launch, so the incident
cannot recur through it.

## Frontend

No frontend changes. The symptom is entirely backend prompt dispatch; the UI
renders whatever session state the backend reports.

## Waves

| Wave | Task | Parallel-safe |
|---|---|---|
| 1 | 01 — reset skips CREATED sessions | no (shared file) |
| 2 | 02 — queue prompt on failed CREATED launch | no (shared file) |

`parallelism: sequential` on both. Task 02 depends on Task 01 only for edit
ordering in the same file, not for behavior.

## Validation

```bash
(cd apps/backend && go test ./internal/orchestrator/... -race)
```

```bash
(cd apps/backend && golangci-lint run ./... --new-from-rev=origin/main --timeout=5m)
```

## Risks

- `resetAgentContext` is also reachable from the session-scoped
  `Service.ResetAgentContext` entry point (`session_scope_matrix_test.go:61`)
  covering a different, user-invoked reset. Task 01 changes only the
  workflow-step helper, not that public method — verify the distinction holds
  before editing.
- Skipping the reset means a `CREATED` session no longer has `acp_session_id`
  cleared on step entry. That value is already empty for a never-prompted
  session; the regression test asserts it stays empty rather than assuming it.

## Out of scope

Carried from the spec: the agentctl `Configure` guard stays as-is, native ACP
`session/reset` is unchanged, the `-32002` resume failure is a consequence not a
defect, and task `FAILED` / recovery semantics are untouched.
