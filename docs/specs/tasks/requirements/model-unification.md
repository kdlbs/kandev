---
status: draft
system: tasks
created: 2026-05-05
owners:
  - cfl
---
# Task model unification Requirements

## Overview

Kanban and Office execution use one workflow-driven task model. Workflow style changes default
presentation and configured triggers, not the task lifecycle owner.

## Requirements

### REQ-TASKS-MODEL-UNIFICATION-001: Task model unification

**Intent:** Unify task execution around workflow steps while keeping existing Kanban behavior
unchanged and making event-driven, multi-agent workflows observable through the same task surfaces.

#### Acceptance criteria

- **AC-TASKS-MODEL-UNIFICATION-001.1:** When a task exists, the system shall associate it with a workflow and workflow step, and shall use the workflow's style only as a presentation hint while preserving the existing Kanban Default lifecycle.
- **AC-TASKS-MODEL-UNIFICATION-001.2:** When a step configures comment, blocker, child-completion, approval, heartbeat, budget, or exhausted-agent-error triggers, the system shall dispatch the corresponding trigger through the workflow engine with its event context; steps without those triggers shall retain current behavior.
- **AC-TASKS-MODEL-UNIFICATION-001.3:** When a workflow step has participants, the system shall support role-targeted runs, participant fan-out, decision-required participants, and guarded quorum transitions; re-entering a decision step shall begin a fresh decision round.
- **AC-TASKS-MODEL-UNIFICATION-001.4:** When the workflow engine emits an automated run, the system shall place it in the shared runs queue with coalescing, idempotency, per-agent serialization, cooldown, and one-agent-per-task checkout guarantees; an explicit user start shall remain direct.
- **AC-TASKS-MODEL-UNIFICATION-001.5:** When a run exhausts its retry policy, the system shall fire the error trigger on the failing task with failure context and preserve an inbox signal when no coordination agent can be queued.
- **AC-TASKS-MODEL-UNIFICATION-001.6:** When users view or delegate tasks across workflow styles, the system shall render shared task content, preserve per-workflow Kanban swimlanes, and create child tasks with the selected workflow without losing parent identity or comments.
