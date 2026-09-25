---
status: active
system: tasks
created: 2026-09-19
owners:
  - kandev
---

# Task workflow agent overrides

## Overview

A user can replace fixed workflow agent profiles when they create a task.
The task system owns these choices because they control that task's execution.
The agent system continues to own the profiles themselves.

The Feature workflow provides the reference scenario. Analysis and Review target
the Initial Agent. Implement selects a fixed profile. PR targets the Implement
session. One replacement changes Implement and the conversation that PR reuses.

## Terminology

- **Source profile:** A fixed profile selected by one or more workflow steps.
- **Replacement:** The profile that this task uses instead of a source profile.
- **Initial Agent:** The existing initial-session recipient, controlled by the main agent selector.

## Requirements

### REQ-TASKS-WORKFLOW-AGENT-OVERRIDES-001: Task-specific profile replacement

**Intent:** Select a different implementation agent without editing a shared workflow.

#### Acceptance criteria

- **AC-TASKS-WORKFLOW-AGENT-OVERRIDES-001.1:** Advanced settings shall list each distinct fixed step profile once, with its name and affected step names. Initial Agent shall retain its existing selector.
- **AC-TASKS-WORKFLOW-AGENT-OVERRIDES-001.2:** Each row shall default to the workflow profile and permit an eligible replacement or reset. Multiple steps with the same source shall share one replacement.
- **AC-TASKS-WORKFLOW-AGENT-OVERRIDES-001.3:** The task shall retain replacements across creation, deferred launch, reload, and server restart. Other tasks, workflows, and profile definitions shall remain unchanged. Each other task shall continue using its own override, when present, or the workflow default. Concurrent execution shall not share overrides between tasks.
- **AC-TASKS-WORKFLOW-AGENT-OVERRIDES-001.4:** Fixed-profile steps shall use the replacement. Initial-session and earlier-step targets shall preserve their session bindings and lifecycle policies. PR shall reuse the replaced Implement session in the reference scenario.
- **AC-TASKS-WORKFLOW-AGENT-OVERRIDES-001.5:** Creation shall reject unknown source profiles and missing, disabled, unauthorized, or incompatible replacements before task persistence or launch. Runtime failure shall report an error without silently using the original profile.
- **AC-TASKS-WORKFLOW-AGENT-OVERRIDES-001.6:** Collapsing Advanced settings shall preserve selections. Changing workflows or workspaces shall clear replacements. A new task form shall start without previous replacements.
- **AC-TASKS-WORKFLOW-AGENT-OVERRIDES-001.7:** Desktop and phone users shall have the same choices. Controls shall support keyboard and touch input, visible labels, focus return, and contained scrolling. Phone controls shall have at least 44-pixel touch targets.
- **AC-TASKS-WORKFLOW-AGENT-OVERRIDES-001.8:** Workflows without fixed step profiles shall show no replacement rows. Loading or failed workflow data shall not appear as an empty workflow. A failed load shall offer retry.
- **AC-TASKS-WORKFLOW-AGENT-OVERRIDES-001.9:** Tasks without overrides shall retain existing routing. A replacement shall apply once to the matching fixed-profile steps identified at task creation, without recursive substitution. Overrides shall apply only to their original workflow.
- **AC-TASKS-WORKFLOW-AGENT-OVERRIDES-001.10:** Selection errors shall preserve the form and identify the affected row. Workflow summaries shall show effective selections. Create-only and create-and-start shall retain identical overrides.

- **AC-TASKS-WORKFLOW-AGENT-OVERRIDES-001.11:** Workflow-step previews in the task topbar and above the chat input shall show this task's effective recipient profile and model. Both surfaces shall honor task overrides and existing session bindings. Reused sessions shall show their effective model, including applicable session settings. Before an earlier-step session exists, previews shall show its expected model as planned when determinable. Once the session exists, previews shall show its actual effective model. Indeterminate models shall remain explicitly unknown. Other tasks shall show their own selections. Phone tap disclosures shall provide the same information.

- **AC-TASKS-WORKFLOW-AGENT-OVERRIDES-001.12:** When a shared workflow changes an affected step's fixed profile, the task shall retain its explicit replacement for that step. Tasks without that override shall use their normal defaults. New steps shall not inherit an existing task override automatically.

## Out of scope

- Editing overrides after task creation, per-step exceptions, and new saved workflow templates.
- Replacing Initial Agent through Advanced settings or changing workflow-level agent defaults.
- Replacing review-action profiles, Office participants, quorum seats, or dynamic profile candidates.
- Changing session start/end policies, explicit recipient bindings, or conditional session configuration.
- Automatic inheritance into child tasks, cloning tasks, and new MCP tool parameters.

## Implementation plans

- [Delivery package](../../../plans/task-workflow-agent-overrides/plan.md)
- [Change workflow extension](change-workflow.md) defines a proposed opt-in
  replacement of the map during a workflow change. General editing of overrides
  within the current workflow remains outside this creation contract.
