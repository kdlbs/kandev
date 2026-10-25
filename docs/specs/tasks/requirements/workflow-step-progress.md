---
status: draft
system: tasks
created: 2026-09-12
owners:
  - kandev
---

# Workflow step progress

## Overview

Users need feedback while a manual step move prepares and starts an agent.
The task system owns this capability because the feedback describes task transitions and session lifecycle.
The existing UI disclosure remains the presentation surface.

## Requirements

### REQ-TASKS-WORKFLOW-STEP-PROGRESS-001: Workflow step progress feedback

The step marker indicates pending work without changing the stepper layout.
Users inspect details in the existing step disclosure.

#### Acceptance criteria

- **AC-TASKS-WORKFLOW-STEP-PROGRESS-001.1:** During a submitted move, the destination marker shall show a spinner. During preparation or startup, the current step marker shall show it.
- **AC-TASKS-WORKFLOW-STEP-PROGRESS-001.2:** The spinner shall preserve the existing marker's layout footprint, visual bounds, alignment, and surrounding spacing. Step labels and connectors shall not move.
- **AC-TASKS-WORKFLOW-STEP-PROGRESS-001.3:** The closed stepper shall show no additional status text, agent name, row, badge, or tooltip.
- **AC-TASKS-WORKFLOW-STEP-PROGRESS-001.4:** The existing step disclosure shall show the known lifecycle status and available agent identity. Existing move controls, options, and capability icons shall remain available under their existing eligibility rules.
- **AC-TASKS-WORKFLOW-STEP-PROGRESS-001.5:** When startup finishes, the normal marker shall return. The disclosure shall show running, waiting, or terminal status from the current owning session.
- **AC-TASKS-WORKFLOW-STEP-PROGRESS-001.6:** A move that does not start an agent shall finish its pending indication when the move settles. An idle created session alone shall not imply startup.
- **AC-TASKS-WORKFLOW-STEP-PROGRESS-001.7:** Rejected moves shall clear request progress and retain existing error reporting. Cancellation, terminal state, or a superseding move shall clear obsolete startup progress.
- **AC-TASKS-WORKFLOW-STEP-PROGRESS-001.8:** Feedback shall belong to the displayed task and relevant step. Historical sessions and late responses from another presentation shall not change it.
- **AC-TASKS-WORKFLOW-STEP-PROGRESS-001.9:** On reload or reconnect, feedback shall use available current lifecycle state. Missing information shall not imply successful startup or trigger a new launch.
- **AC-TASKS-WORKFLOW-STEP-PROGRESS-001.10:** Keyboard users shall access the existing disclosure and status. Touch users shall access equivalent details through existing workflow drawers, without a new phone top-bar control.
- **AC-TASKS-WORKFLOW-STEP-PROGRESS-001.11:** Reduced-motion users shall receive a static pending marker with an accessible status. All new copy shall use the supported translations.

## Out of scope

- Reducing startup latency or changing workflow, profile, prompt, or session policies.
- Continuous spinner animation during an ordinary running turn.
- New runtime phases, backend fields, polling loops, timers, or persistent progress records.
- New retry controls, notifications, or a separate status overlay.

## Related contracts

- [Runtime publication order](runtime-state-publication-order.md)
- [Compact disclosure](../../ui/requirements/compact-workflow-step-navigation.md)
- [Implementation plan](../../../plans/workflow-step-progress/plan.md)
