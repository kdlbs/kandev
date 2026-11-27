---
status: active
system: ui
created: 2026-09-30
owners:
  - Kandev
---

# Task List Workflow Step Grouping Requirements

## Overview

List users shall organize tasks by their configured workflow position using a
**Workflow step** grouping option. The current **State** option describes runtime
status and can place tasks from different workflow steps in the same section.

UI owns the list presentation and portable grouping preference. The
[task system](../../tasks/README.md) continues to own task status, workflow
definitions, and step transitions. This capability is separate from
[listing display preferences](task-listing-display-preferences.md), whose
current scope excludes changes to grouping behavior.

## Terminology

- **Runtime state:** A task's execution status, such as Waiting for input or
  Completed. It need not match its workflow position.
- **Workflow step:** A configured process position, such as Backlog, Build, or
  Review, belonging to one workflow.
- **Root task:** A task displayed at the top of a list hierarchy. A child whose
  parent is absent from the current result page is also displayed as a root.

## Requirements

### REQ-UI-LIST-STEP-GROUPING-001: Workflow step sections

**Intent:** Make List grouping reflect the user's configured process.

#### Acceptance criteria

- **AC-UI-LIST-STEP-GROUPING-001.1:** Desktop and phone List grouping controls
  shall offer **Workflow step** in place of **State**, and shall use workflow
  step grouping by default. Labels and explanatory text shall be localized.
- **AC-UI-LIST-STEP-GROUPING-001.2:** Root tasks in the same workflow step shall
  share a section despite different runtime states. Roots in different steps
  shall occupy separate sections despite identical runtime states. Distinct
  steps shall not merge merely because their names match.
- **AC-UI-LIST-STEP-GROUPING-001.3:** A section shall display its configured step
  name. When multiple workflows appear, headings shall also identify the
  workflow. Sections shall follow workflow order and then configured step
  order, with deterministic ordering for ties.
- **AC-UI-LIST-STEP-GROUPING-001.4:** Tasks without a workflow step shall appear
  in a **No workflow step** section after known steps. Unavailable step metadata
  shall preserve the task's step identity and rows, show a neutral loading or
  unavailable label, and recover when metadata becomes available. Metadata
  from another workspace shall never name a section.
- **AC-UI-LIST-STEP-GROUPING-001.5:** Saved grouping choices and grouping links
  shall survive reloads. Existing **State** preferences and links shall resolve
  to **Workflow step**; Workflow, Repository, and None shall retain their
  existing meanings. Invalid or missing grouping choices shall select the new
  default.
- **AC-UI-LIST-STEP-GROUPING-001.6:** Grouping shall retain list sorting,
  filtering, pagination, row actions, and parent/child indentation. Children
  remain under a displayed parent, including when their own workflow step
  differs. Changing grouping shall not move tasks or change runtime state.
- **AC-UI-LIST-STEP-GROUPING-001.7:** Phone users shall choose the grouping from
  the existing List view-options drawer using the shared portable preference.
  The label and options shall remain readable and operable without horizontal
  document overflow, using the existing touch targets and dismissal behavior.

## Out of scope

- Changing sidebar, Threads, Office, or Kanban grouping.
- Adding a separate runtime-state grouping option.
- Workflow transitions, task lifecycle changes, or task-status icon changes.
- Changing the list's page-local grouping or cross-page hierarchy behavior.
- New feature flags, settings pages, or observability infrastructure.

## Implementation plans

- [Workflow step grouping](../../../plans/task-list-workflow-step-grouping/plan.md).
