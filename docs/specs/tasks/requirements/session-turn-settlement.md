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

## Exclusions

- Changing dispatch-only prompt completion eligibility.
- Changing cancellation, workflow movement, or idle-session reclamation policy.
- Recovering a mutex deadlock that already exists in a running backend.
- Changing desktop or phone controls, copy, or presentation.

## Implementation plans

- [Completion callback lease repair](../../../plans/completion-callback-lease/plan.md).
