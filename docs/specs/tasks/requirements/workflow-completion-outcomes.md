---
status: active
system: tasks
created: 2026-11-10
owners:
  - kandev
---

# Workflow completion outcomes

## Overview

An agent needs to distinguish signal acceptance from a committed workflow transition.
A later correction also needs its own completion identity.
The tasks system owns this contract because it owns workflow entries, session identity, and transition admission.

This capability extends [explicit completion](workflow-explicit-completion-signal.md).
It narrows the duplicate rule in [payload delivery](workflow-completion-signal-payload-delivery.md), rather than creating another handoff mechanism.

## Terminology

- **Source:** The authenticated task session and the turn resolved when the backend admits the completion call.
- **Entry:** One arrival at a workflow step. Leaving and returning creates another entry.
- **Duplicate:** Another completion call from the same source turn and entry, even if its text differs.
- **Receipt:** Durable evidence of an accepted signal and its known transition outcome.
- **Current destination:** The task's observed workflow, step, entry, primary session, and queue placement.

## Requirements

### REQ-TASKS-COMPLETION-OUTCOMES-001: Source-specific completion

**Intent:** Preserve duplicate safety without discarding a later completed correction.

#### Acceptance criteria

- **AC-TASKS-COMPLETION-OUTCOMES-001.1:** A duplicate shall retain the first accepted payload for its source turn. It shall not create another transition or destination prompt.
- **AC-TASKS-COMPLETION-OUTCOMES-001.2:** A later turn in the same current entry shall be eligible to replace an unconsumed earlier signal. This includes turns created from native output without another Kandev user prompt.
- **AC-TASKS-COMPLETION-OUTCOMES-001.3:** A replacement shall carry its own source identity, summary, handoff, and blockers. A delayed earlier request, consumer, or cleanup shall not overwrite or remove that replacement.
- **AC-TASKS-COMPLETION-OUTCOMES-001.4:** At most one transition shall leave a given source entry. A concurrent replacement shall either supply that transition's payload or receive an explicit stale result after another transition commits.
- **AC-TASKS-COMPLETION-OUTCOMES-001.5:** A transition shall use the accepted signal that still owns completion at commit. Its carry token and audit payload shall describe that same signal.
- **AC-TASKS-COMPLETION-OUTCOMES-001.6:** A native verification turn without a new completion call shall not replace the accepted handoff. The existing new-user-message reopening rule shall still cancel pending completion.
- **AC-TASKS-COMPLETION-OUTCOMES-001.7:** A stale turn or entry shall gain no authority over the current destination. A committed handoff shall not change through a late source call.
- **AC-TASKS-COMPLETION-OUTCOMES-001.8:** Completion shall preserve task authorization, executor authorization, credential preflight, clarification barriers, and configured Review isolation. An unavailable destination shall not cause execution through the source session as a substitute.

### REQ-TASKS-COMPLETION-OUTCOMES-002: Observable completion outcome

**Intent:** Make the result useful for recovery without presenting acknowledgement as transition proof.

#### Acceptance criteria

- **AC-TASKS-COMPLETION-OUTCOMES-002.1:** Accepted and duplicate results shall identify the stored signal and source. They shall distinguish pending, blocked, unconfigured, superseded, committed, and destination-queued outcomes.
- **AC-TASKS-COMPLETION-OUTCOMES-002.2:** A committed result shall identify the exact transition and its destination. The response shall separately report the current destination and generic task state from one consistent read.
- **AC-TASKS-COMPLETION-OUTCOMES-002.3:** Signal acceptance and `already_signaled` shall not imply transition commitment or reviewer dispatch. Generic task state `REVIEW` shall not imply entry into a workflow step named Review.
- **AC-TASKS-COMPLETION-OUTCOMES-002.4:** A retry from the recorded source shall retain its receipt after consumption, bag cleanup, and backend restart. It shall create no new transition, including after destination entry begins.
- **AC-TASKS-COMPLETION-OUTCOMES-002.5:** Unknown or failed reads shall not produce invented transition proof. Historical signals without a receipt shall remain explicitly unproven.
- **AC-TASKS-COMPLETION-OUTCOMES-002.6:** Public arguments shall remain unchanged. Existing `accepted`, `reason`, `step_id`, `signaled_at`, `advances`, and truncation meanings shall remain compatible.
- **AC-TASKS-COMPLETION-OUTCOMES-002.7:** Source and outcome data shall remain task-scoped. A caller without task access shall receive no receipt or destination details.

## Exclusions

This capability does not reopen a committed source step or amend an active Review conversation.
It does not make handoff delivery transactional with agent startup.
The existing delivery failure contract still applies after a transition commits.
It adds no rendered UI, configuration, fallback button, or executor permission.

## Implementation plan

- [Completion identity and outcomes](../../../plans/step-completion-outcomes/plan.md)
