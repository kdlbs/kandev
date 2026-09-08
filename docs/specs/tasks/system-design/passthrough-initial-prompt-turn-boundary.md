---
status: current
system: tasks
requirements:
  - REQ-TASKS-PASSTHROUGH-INITIAL-TURN-001
---

# Passthrough Initial Prompt Turn Boundary System Design

## Purpose and boundaries

The task system owns whether a raw passthrough completion signal represents a
completed turn. The agentctl interactive runner continues to own PTY output,
prompt-pattern detection, status detection, and idle detection. The lifecycle
manager owns the delivery context needed to decide whether those raw signals
are eligible.

The boundary applies only to fresh passthrough processes whose non-empty task
prompt must be injected through stdin. It does not change prompt-flag launches,
promptless tools, or resumed conversations.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-PASSTHROUGH-INITIAL-TURN-001` | [Runtime contract](#runtime-contract), [Control flow](#control-flow), [Failure and recovery](#failure-and-recovery) |

## Components and responsibilities

- `process.InteractiveRunner` reports raw prompt-pattern, status, and idle
  completion signals and exposes the first-idle wait used by prompt injection.
- `lifecycle.AgentExecution` owns an in-memory, process-scoped initial-prompt
  marker protected with the passthrough process lifecycle state.
- `lifecycle.Manager.startPassthroughSession` and the fresh-start resume
  fallback bind that marker to the newly active PTY process when stdin prompt
  delivery is required.
- `lifecycle.Manager.autoInjectInitialPromptWith` waits for readiness, submits
  all planned stdin chunks, and clears the matching marker on every exit path.
- `lifecycle.Manager.handlePassthroughTurnComplete` validates process identity
  and suppresses completion while the active process owns the marker. Eligible
  signals continue through `MarkReady` and the existing orchestrator path.

## Runtime contract

The pending marker contains the active passthrough process identity. It is
runtime-only state and is not persisted. A boolean without process identity is
insufficient. A delayed callback or cleanup from an old process must not affect
the replacement process's boundary.

The marker exists only when all of these conditions hold:

- the launch creates a fresh conversation,
- the agent has no `PromptFlag` delivery path, and
- the execution contains a non-empty task description for stdin injection.

The marker does not emit `agent.boot_ready`, `agent.ready`, or
`agent.running`. The execution is already running while Kandev waits for the
startup idle and injects the prompt.

## Control flow

1. Lifecycle determines whether the fresh process needs stdin prompt delivery.
2. After the interactive runner returns the new process identity, lifecycle
   installs that identity as the active process. Lifecycle installs the pending
   initial-prompt owner under the same lock.
3. A first-idle, prompt-pattern, or status callback can wake prompt injection.
   The completion handler observes the matching pending marker and returns
   without calling `MarkReady`.
4. Prompt injection marks the execution running through the existing
   idempotent path and writes every planned prompt and submit chunk.
5. Prompt injection clears the marker for that process after the final write.
6. Agent output resets the existing turn detector. The next eligible
   completion callback finds no pending marker and follows the ordinary
   `MarkReady` path, including workflow and queue consumers.

Prompt-flag launches, promptless processes, and ordinary resumes never install
the marker, so their first completion signal remains eligible.

## Failure and recovery

- Prompt injection clears the matching marker when readiness times out,
  shutdown interrupts delivery, or a PTY write fails. This avoids suppressing
  later manual work indefinitely.
- Marker cleanup compares process identity. Cleanup from an old process cannot
  clear a boundary installed for its replacement.
- A delayed completion callback from a replaced process continues to fail the
  existing active-process identity check.
- A fresh-start resume fallback installs a new marker for its new process. A
  successful resume does not install one because it does not reinject history.
- Mid-turn idle false positives remain a separate detector-accuracy concern.

## Observability

Lifecycle emits a debug diagnostic when it ignores a completion signal for a
process with a pending initial prompt. Existing timeout and write-failure
warnings continue to expose prompt injection failures. No persisted state or
new metric is required.

Focused lifecycle tests cover startup suppression, ordinary completion after
submission, prompt-flag and promptless behavior, abort cleanup, fresh fallback,
and process replacement.

## Related decisions

- [Keep passthrough initial prompt state in lifecycle](../../../decisions/2026-09-01-passthrough-initial-prompt-turn-boundary.md)
- [Defer passthrough running publication until guard release](../../../decisions/2026-08-31-passthrough-running-publication.md)
