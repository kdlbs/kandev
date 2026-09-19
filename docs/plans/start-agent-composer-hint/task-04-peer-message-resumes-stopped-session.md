---
id: "04-peer-message-resumes-stopped-session"
title: "Backend pin test: peer message resumes a stopped session"
status: done
wave: 1
parallelism: sequential
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/prevent-agent-autostart-on-open.md"
---

# Task 04: Backend pin test — peer message resumes a stopped session

## Acceptance

- A new Go test in `apps/backend/internal/orchestrator/`
  (`task_operations_test.go`, next to the other
  `TestQueueAndInterruptForPeerMessage_*` tests) pins the requirement that a
  message received from another task (sub-task via `message_task_kandev`,
  delivered as `QueueAndInterruptForPeerMessage` with `QueuedByAgent`)
  auto-starts the agent when the receiving session is stopped-but-resumable
  (the post-restart / recovered-idle shape the prevent-auto-start preference
  leaves the session in).
- Test shape: seed a task + session in `WAITING_FOR_INPUT` with a resume
  token (an `executors_running` row carrying the token, like
  `TestResumeTaskSession_*` / `TestEnsureSessionRunning_*` fixtures), an
  agent manager reporting `isAgentRunning: false` (so `ensureSessionRunning`
  takes the resume path) with a recording `LaunchAgent`, and the executor
  wired like the existing resume-path tests. Call
  `QueueAndInterruptForPeerMessage(ctx, taskID, sessionID, prompt, metadata)`
  and assert the agent was launched (resume) and the queued message was
  dispatched to the agent (PromptAgent reached), mirroring the assertions of
  `TestQueueAndInterruptForPeerMessage_DeliversQueuedMessageWithoutUserCancelSideEffects`
  but with the stopped-agent resume path instead of
  `isAgentRunning: true`.
- The test must PASS against the current backend (the behavior already
  exists: `takeIfPromptableLocked` → `executeQueuedMessage` → `PromptTask` →
  `ensureSessionRunning` resumes). It is a regression pin, not a fix; if it
  fails, stop and report — that would mean the requirement is actually
  broken and needs a fix design.

## Verification

```bash
(cd apps/backend && go test ./internal/orchestrator/ -run TestQueueAndInterruptForPeerMessage_StoppedSessionResumesAgent -race)
```

## Files Likely Touched

- `apps/backend/internal/orchestrator/task_operations_test.go` (new test)
- Possibly `apps/backend/internal/orchestrator/task_operations.go` ONLY if the
  test proves the behavior is broken (then stop and report; do not fix
  silently)

## Dependencies

None. Backend-only; independent of the frontend hint tasks.

## Inputs

- Spec: "What" bullet "A message auto-starts the agent covers every message…"
  and the scenario "GIVEN a recovered-idle session whose composer hint is
  visible, WHEN a message is received from another task…".
- `QueueAndInterruptForPeerMessage` at
  `task_operations.go:5363-5423` (queues with `QueuedByAgent`, dispatches via
  `takeIfPromptableLocked` when promptable).
- Resume-path fixture examples: `TestResumeTaskSession_*` at
  `task_operations_test.go:5161-5204` (sets `AgentProfileID`, wires the
  executor with the mock agent manager, asserts the resumed state and prompt
  dispatch); `TestQueueAndInterruptForPeerMessage_*` at
  `task_operations_test.go:1905-2920` (queue + dispatch assertions).
- `ensureSessionRunning` at `task_operations.go:1891` (resumes
  WAITING_FOR_INPUT sessions with a resume token).

## Output Contract

The backend test suite proves: a peer (sub-task) message delivered to a
stopped-but-resumable session starts the agent and the message reaches the
agent. Combined with the frontend E2E (task-03), the hint's promise —
"a message auto-starts the agent" — is verified for both the user's composer
sends and received sub-task messages.
