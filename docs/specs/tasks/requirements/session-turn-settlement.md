---
status: draft
system: tasks
created: 2026-10-02
owners:
  - kandev
---

# Session turn settlement

## Overview

The task system owns session execution lifecycle and follow-up prompt admission.
A completed turn must release its session for subsequent work, including turns
that an agent starts without a Kandev prompt.

This contract makes the synthetic-turn behavior from
[ADR 0035](../../../decisions/0035-version-agent-ready-events-by-prompt-generation.md)
explicit. It does not change prompt ownership or completion eligibility.

## Requirements

### REQ-TASKS-SESSION-TURN-SETTLEMENT-001: Completion progress during resume

**Intent:** A valid turn completion and a concurrent session resume must both
finish without a backend restart.

#### Acceptance criteria

- **AC-TASKS-SESSION-TURN-SETTLEMENT-001.1:** When an eligible synthetic turn
  completes successfully during a concurrent resume request, the session shall
  become ready and the resume request shall proceed. Neither operation shall
  wait indefinitely for the other.
- **AC-TASKS-SESSION-TURN-SETTLEMENT-001.2:** Completion notifications shall
  retain their originating execution and attempt identity. A concurrent resume
  shall not attribute the completed turn to its successor attempt.
- **AC-TASKS-SESSION-TURN-SETTLEMENT-001.3:** An eligible synthetic completion
  shall settle both a turn with content and a turn without content. Existing
  completion notification order and follow-up admission shall remain intact.
- **AC-TASKS-SESSION-TURN-SETTLEMENT-001.4:** A superseded startup callback or
  prompt completion shall not settle its successor. An unnumbered completion
  shall remain ineligible while a dispatched prompt is pending.

### REQ-TASKS-SESSION-TURN-SETTLEMENT-002: Agent-started turns reach completion

**Intent:** A turn the agent starts on its own, such as the follow-up to a
finished background job, must not hold the session busy after the agent stops.

#### Acceptance criteria

- **AC-TASKS-SESSION-TURN-SETTLEMENT-002.1:** When an agent-started turn has
  gone quiet for the idle window and has no tool with a reported active status,
  the session shall complete the turn and become ready.
- **AC-TASKS-SESSION-TURN-SETTLEMENT-002.2:** An update that carries no status
  shall not make a tool active. This includes an update for a tool that has
  already finished, and it shall not delay or prevent turn completion.
- **AC-TASKS-SESSION-TURN-SETTLEMENT-002.3:** When the provider explicitly
  reports the end of an agent-started turn, the session shall complete that turn
  even if a tool never reported a terminal status.
- **AC-TASKS-SESSION-TURN-SETTLEMENT-002.4:** A foreground tool with a reported
  active status shall keep the turn open during silence when the provider has
  not reported the end of the turn.
- **AC-TASKS-SESSION-TURN-SETTLEMENT-002.5:** When an agent-started turn
  completes, auto-run queued messages for that session shall be delivered without
  another user action.

## Exclusions

- Changing dispatch-only prompt completion eligibility.
- Changing cancellation, workflow movement, or idle-session reclamation policy.
- Recovering a mutex deadlock that already exists in a running backend.
- Changing desktop or phone controls, copy, or presentation.

## Implementation plans

- [Completion callback lease repair](../../../plans/completion-callback-lease/plan.md).
- [Stuck turn and queue delivery repair](../../../plans/stuck-turn-queue-delivery/plan.md).
