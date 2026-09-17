---
id: "02-queue-prompt-on-failed-launch"
title: "Queue the auto-start prompt when a CREATED launch fails"
status: done
wave: 2
parallelism: sequential
depends_on: ["01-reset-skips-created"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/workflow-step-agent-start-ownership.md"
---

# Task 02: Queue the auto-start prompt when a CREATED launch fails

> **Scope corrected during implementation.** The incident's failure is
> *asynchronous* — `startAgentOnExistingWorkspace` calls
> `startAgentProcessAsync` and returns `nil`, so `StartCreatedSession` returned
> no error and this branch was never reached in production. The synchronous
> branch is still reachable and still drops prompts, but only when there is no
> in-memory execution (the post-restart shape) and the full `LaunchAgent` path
> runs. This task closes that case; the async gap is recorded under the spec's
> **Known gap** and needs its own cycle.

## Acceptance

- When `autoStartStepPrompt`'s `CREATED` branch gets a busy or already-running
  error from `StartCreatedSession`, the already-recorded prompt is queued for
  the session via `queueAutoStartPrompt`, passing `userMsgRecorded` so the
  drain does not duplicate the chat row.
- Permanent rejections (Office scheduler guard, missing agent profile) are not
  queued — nothing would drain them.
- The existing `requeueTaken()` restoration of a taken handoff message is
  preserved.
- The original error is still returned, so session `FAILED` transition, error
  surfacing, and the caller's behavior are unchanged.
- A failure to queue is logged and does not mask the original launch error.
- The queued prompt is recoverable by the existing `handleAgentBootReady` drain
  — no new drain path is introduced.

## Regression test

Add to `apps/backend/internal/orchestrator/event_handlers_duplicate_autostart_test.go`,
alongside `TestAutoStartTransientError_BootReadyDrainsOrphanedQueue`:

- **Red first.** A `CREATED` session whose `StartCreatedSession` fails; assert a
  queued message exists for the session afterward. Before the fix the queue is
  empty and the prompt is only a chat row.
- Assert the returned error is the launch error, unchanged.
- Extend through the existing boot-ready drain to show the prompt is delivered
  once the session becomes promptable, mirroring the sibling test's end-to-end
  shape.

## Verification

```bash
(cd apps/backend && go test ./internal/orchestrator/ -race -run 'TestAutoStart|TestProcessOnEnterResetAgentContext')
```

```bash
(cd apps/backend && go test ./internal/orchestrator/... -race)
```

```bash
(cd apps/backend && golangci-lint run ./... --new-from-rev=origin/main --timeout=5m)
```

## Files Likely Touched

- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/event_handlers_duplicate_autostart_test.go`

## Dependencies

Task 01 (same file; edit ordering only).

## Inputs

- Spec scenario 5 and the matching failure mode.
- The `CREATED` branch at `event_handlers_workflow.go:1680`.
- The precedent in the same function's `PromptTask` retry loop: the
  `isAgentAlreadyRunningError` branch calls `queueAutoStartPrompt` with a
  comment describing this exact situation.
- `handleAgentBootReady`'s drain, pinned by
  `TestAutoStartTransientError_BootReadyDrainsOrphanedQueue`.

## Output Contract

The `CREATED` branch queues before returning its error. No signature change, no
new queue mechanism.
