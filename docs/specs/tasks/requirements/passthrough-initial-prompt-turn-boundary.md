---
status: active
system: tasks
created: 2026-09-01
owners:
  - cfl
---

# Passthrough Initial Prompt Turn Boundary Requirements

## Overview

The task system owns session lifecycle state, turn completion, workflow
triggers, and downstream completion effects. A fresh CLI passthrough process
can become idle before Kandev injects its initial task prompt. That startup
readiness signal must not be interpreted as a completed agent turn.

CLI passthrough owns the PTY transport and readiness detector. The task system
owns whether a detector signal is eligible to complete a turn.

## Terminology

- **Pending initial prompt:** A non-empty task prompt that Kandev must submit
  through a fresh passthrough process's PTY because the agent does not receive
  it at process launch.
- **Completion signal:** A passthrough prompt-pattern, status, or idle signal
  that can normally mark an agent turn ready for follow-up work.

## Requirements

### REQ-TASKS-PASSTHROUGH-INITIAL-TURN-001: Initial prompt owns the first turn boundary

**Intent:** A passthrough task shall remain active until its agent has received
and processed the initial prompt.

#### Acceptance criteria

- **AC-TASKS-PASSTHROUGH-INITIAL-TURN-001.1:** When a fresh passthrough process
  has a pending initial prompt, completion signals received before Kandev
  finishes submitting that prompt shall not complete a turn. These signals
  shall not change session or task state, evaluate `on_turn_complete`, or invoke
  other turn-completion consumers.
- **AC-TASKS-PASSTHROUGH-INITIAL-TURN-001.2:** After Kandev finishes submitting
  the initial prompt, the next eligible completion signal shall complete the
  initial turn through the ordinary passthrough completion path.
- **AC-TASKS-PASSTHROUGH-INITIAL-TURN-001.3:** A passthrough process that
  receives its prompt at process launch or has no initial prompt shall retain
  ordinary completion behavior. A resumed conversation without reinjection
  shall use the same behavior.
- **AC-TASKS-PASSTHROUGH-INITIAL-TURN-001.4:** A fresh-start fallback that must
  reinject the initial prompt shall use the same pending boundary as an
  original fresh launch.
- **AC-TASKS-PASSTHROUGH-INITIAL-TURN-001.5:** When initial prompt submission
  aborts, that process's pending boundary shall end. When its process is
  replaced, the old boundary shall not suppress the replacement process or
  later user work.

## Out of scope

- Replacing the passthrough idle heuristic for turns that have already started.
- Changing prompt formatting, submit sequences, or per-agent idle timeouts.
- Changing completion semantics for headless prompt-flag agents.
