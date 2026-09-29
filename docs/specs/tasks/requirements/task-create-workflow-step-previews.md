---
status: active
system: tasks
created: 2026-09-29
owners:
  - kandev
---

# Task creation workflow step previews

## Overview

A person creating a task can compare the steps of every available workflow
before selecting one. This contract belongs to tasks because tasks owns
workflow definitions and task creation.

The option preview lists the workflow's steps. It differs from the selected
workflow's [launch destination](task-create-launch-preview.md).

## Requirements

### REQ-TASKS-CREATE-WORKFLOW-STEPS-001: Complete workflow option previews

**Intent:** Workflow previews do not depend on previously visited boards or tasks.

#### Acceptance criteria

- **AC-TASKS-CREATE-WORKFLOW-STEPS-001.1:** When the selector opens, every
  available workflow shall load and display its ordered steps without requiring
  a previous visit to that workflow. This includes unselected workflows.
- **AC-TASKS-CREATE-WORKFLOW-STEPS-001.2:** While a workflow's steps load, its
  option shall show a loading state. A successful empty result shall show an
  empty state. Neither state shall masquerade as a failed request.
- **AC-TASKS-CREATE-WORKFLOW-STEPS-001.3:** If one workflow fails to load, its
  option shall show an error and a retry action. Successful options shall retain
  their previews. Retrying shall not select a workflow or clear the task draft.
- **AC-TASKS-CREATE-WORKFLOW-STEPS-001.4:** After workspace changes, selector
  dismissal, or workflow removal, late results shall not populate the current
  options. Reopening shall load current step definitions, including deletions.
- **AC-TASKS-CREATE-WORKFLOW-STEPS-001.5:** Each loaded preview shall retain step
  names, order, colors, configured-start indicators, and available agent badges.
  Loading previews shall not change workflow selection, launch routing, or
  remembered defaults. Hidden and locked workflows retain their existing rules.
- **AC-TASKS-CREATE-WORKFLOW-STEPS-001.6:** Desktop and phone users shall see the
  same preview states and can select or retry with keyboard or touch controls.
  Long step lists shall remain readable without document horizontal overflow.
  Phone and coarse-pointer actions shall have hit areas of at least 44 CSS pixels.

## Out of scope

- Changing workflow definitions, task submission gates, or launch precedence.
- Changing the selected launch destination or prompt preview contract.
- Adding persistent caches, backend endpoints, or workflow-step editing.
- Changing the automation workflow selector's data-loading contract.

## Implementation plans

- [Workflow step preview repair](../../../plans/task-create-workflow-step-previews/plan.md)
