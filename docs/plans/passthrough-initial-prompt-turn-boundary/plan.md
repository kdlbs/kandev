---
created: 2026-09-01
status: done
requirements:
  - REQ-TASKS-PASSTHROUGH-INITIAL-TURN-001
system_design:
  - ../../specs/tasks/system-design/passthrough-initial-prompt-turn-boundary.md
legacy_specs:
  - ../../specs/cli/requirements/cli-mode-parity.md
---

# Implementation Plan: Passthrough Initial Prompt Turn Boundary

## Overview

Add a process-scoped pending-initial-prompt marker to passthrough lifecycle,
then use it to reject completion callbacks until fresh prompt injection ends.
One work order keeps marker ownership, launch and fallback integration, cleanup,
and regression coverage atomic.

## Scope

### In scope

- Mark fresh passthrough processes that still require initial stdin prompt
  injection.
- Suppress all completion callback sources while the matching process owns the
  marker.
- Clear the marker safely after successful or aborted injection.
- Preserve first-completion behavior for prompt-flag, promptless, and resumed
  processes.
- Cover process replacement and fresh-start fallback behavior.

### Out of scope

- Replacing the idle detector or fixing mid-turn idle false positives.
- Changing prompt text, submit chunk planning, or idle timeout values.
- Changing `agent.ready`, `agent.boot_ready`, or workflow event semantics.
- Adding frontend behavior or browser automation for a backend lifecycle gate.

## Technical approach

Extend `AgentExecution` in
`apps/backend/internal/agent/runtime/lifecycle/types.go` with a runtime-only
pending initial-prompt process identity protected by
`passthroughLifecycleMu`. Add narrow helpers that install, query, and clear the
marker by process ID.

In `manager_passthrough.go`, centralize the predicate for stdin initial-prompt
delivery. Install the marker alongside `PassthroughProcessID` for the original
fresh start and the fresh-start resume fallback. Do not install it for
`PromptFlag`, empty-description, or ordinary resume paths.

Have `handlePassthroughTurnComplete` snapshot the active process and marker
under the lifecycle lock. Keep the existing stale-process rejection, then
return with a debug diagnostic when the active process still owns the pending
marker. In `autoInjectInitialPromptWith`, defer identity-checked cleanup after
capturing the process ID so success, timeout, shutdown, and write failure all
release only their own marker.

## Tests

| Acceptance criterion | Evidence |
| --- | --- |
| `AC-TASKS-PASSTHROUGH-INITIAL-TURN-001.1` | A lifecycle regression marks a running PTY as pending and fires turn completion. It asserts that no `agent.ready` event or Ready status occurs. |
| `AC-TASKS-PASSTHROUGH-INITIAL-TURN-001.2` | An auto-injection test submits all chunks, verifies marker cleanup, then fires completion and asserts exactly one ordinary `agent.ready` event. |
| `AC-TASKS-PASSTHROUGH-INITIAL-TURN-001.3` | Existing and extended tests prove unmarked active processes, prompt-flag launches, promptless launches, and resume paths retain ordinary behavior. |
| `AC-TASKS-PASSTHROUGH-INITIAL-TURN-001.4` | The fresh-fallback launch test asserts that the new process receives the marker before its injection goroutine runs. |
| `AC-TASKS-PASSTHROUGH-INITIAL-TURN-001.5` | Timeout/write-failure and replacement tests prove identity-checked cleanup and completion eligibility for the active replacement. |

## Work orders

- [x] [Task 01: Guard the passthrough initial turn](task-01-guard-passthrough-initial-turn.md)

## Verification results

- The focused lifecycle command passed 16 tests.
- The race-detector command passed the same 16 tests.
- The full lifecycle package command passed 2,100 tests.

## Risks

- A marker without process identity could suppress a replacement process.
- Cleanup that runs only on successful writes could leave manual follow-up work
  permanently non-completing after an injection failure.
- Installing the marker for prompt-flag or resume paths would hide a genuine
  first turn completion.
