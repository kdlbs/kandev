---
status: active
system: platform
created: 2026-10-02
owners:
  - kandev
---

# Prompt completion ownership requirements

## Overview

A completed foreground prompt must not prevent later autonomous work from completing.
Platform owns this shared runtime contract for all structured agent providers.
Task workflows consume completion events but retain their own admission and transition rules.

## Terminology

- **Dispatch-only prompt:** A prompt whose caller returns after dispatch acknowledgement, before the agent completes.
- **Numbered completion:** A terminal event that identifies a lifecycle prompt generation.
- **Unnumbered completion:** A terminal event for autonomous work outside a lifecycle prompt.

## Requirements

### REQ-PLATFORM-PROMPT-COMPLETION-OWNERSHIP-001: Completion releases its prompt

**Intent:** Preserve prompt ownership without leaving completed work as an obstacle to later input.

#### Acceptance criteria

- **AC-PLATFORM-PROMPT-COMPLETION-OWNERSHIP-001.1:** After a dispatch-only prompt completes, the system shall accept a later autonomous completion when no newer foreground prompt owns the execution.
- **AC-PLATFORM-PROMPT-COMPLETION-OWNERSHIP-001.2:** After that autonomous work completes, the next eligible prompt shall reach the same execution without a completion-readiness timeout or forced restart.
- **AC-PLATFORM-PROMPT-COMPLETION-OWNERSHIP-001.3:** If completion precedes dispatch acknowledgement, that acknowledgement shall not make the completed prompt pending again.
- **AC-PLATFORM-PROMPT-COMPLETION-OWNERSHIP-001.4:** An unnumbered, stale, or duplicate completion shall not release a pending newer foreground prompt or change its transcript.
- **AC-PLATFORM-PROMPT-COMPLETION-OWNERSHIP-001.5:** The system shall preserve completion delivery and finish predecessor transcript processing before a successor prompt resets shared transcript state.
- **AC-PLATFORM-PROMPT-COMPLETION-OWNERSHIP-001.6:** When a matching numbered completion reports an error, the execution shall reach FAILED or, during shutdown, STOPPED before its dispatch barrier is released.
- **AC-PLATFORM-PROMPT-COMPLETION-OWNERSHIP-001.7:** A terminal event for a numbered error shall retain the prompt generation, turn ID, attempt ID, and failure evidence captured for that completion.
- **AC-PLATFORM-PROMPT-COMPLETION-OWNERSHIP-001.8:** When a synchronous terminal-event subscriber waits for dispatch acknowledgement, delivery shall not prevent the acknowledgement callback from running.
- **AC-PLATFORM-PROMPT-COMPLETION-OWNERSHIP-001.9:** When a numbered completion moves an execution to FAILED or STOPPED, a later prompt request shall be rejected without creating a new prompt generation.
- **AC-PLATFORM-PROMPT-COMPLETION-OWNERSHIP-001.10:** When a dispatch-only prompt receives no completion, a waiting successor shall return the existing bounded timeout without clearing the prompt's pending barrier.
- **AC-PLATFORM-PROMPT-COMPLETION-OWNERSHIP-001.11:** When cancellation of a dispatch-only prompt exhausts its completion wait, recovery shall preserve the original transport or completion error when reporting escalation.

## Out of scope

- Provider-specific notification classification or suppression.
- Changes to workflow transitions, task-state projections, or prompt queue policy.
- Changes to cancellation budgets, provider restart policy, or persisted data.
- Changes to frontend markup, copy, or interaction patterns.

## Related contracts

- [ADR 0035: Prompt generation identity](../../../decisions/0035-version-agent-ready-events-by-prompt-generation.md)
- [Workflow reset quiescence](../../tasks/requirements/workflow-step-agent-start-ownership.md)
- [Background work liveness](background-work-liveness.md)

## Implementation plans

- [Dispatch completion and wakeup](../../../plans/dispatch-completion-wakeup/plan.md)
