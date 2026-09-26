---
status: draft
system: tasks
created: 2026-09-23
owners:
  - kandev
---

# Change workflow

## Overview

A user can move an existing task into another workflow and choose its destination
step and fixed agents. This corrects a task created in the wrong workflow without
creating a second task. The task system owns workflow membership and task-specific
agent choices. Agent profiles and workspace resources keep their existing owners.

The reference case moves a Kanban task to Feature, at Analysis. Implement can use
a replacement profile. Steps that reuse Implement continue to reuse its selected
conversation. Actual workflow configuration determines each step's recipient.

## Terminology

- **Workflow agent:** A fixed profile referenced by destination workflow steps.
- **Replacement:** A task-specific alternative to a workflow agent.
- **Entry preview:** A prediction of the selected step's session and settings.

## Requirements

### REQ-TASKS-CHANGE-WORKFLOW-001: Destination and agent choices

**Intent:** Change one task's workflow with explicit destination and agent choices.

#### Acceptance criteria

- **AC-TASKS-CHANGE-WORKFLOW-001.1:** Single-task entry points shall display
  `Change workflow...` instead of `Send to workflow`. Card, sidebar, detail,
  preview, command-palette, and phone task actions shall open the same logical form.
- **AC-TASKS-CHANGE-WORKFLOW-001.2:** The form shall offer accessible, visible
  destination workflows in the task's workspace, excluding its current workflow.
  It shall list destination steps in workflow order, including steps hidden by a
  personal board preference. The user shall explicitly select a step before submission.
  A workflow with no steps shall show an empty state and prevent submission.
- **AC-TASKS-CHANGE-WORKFLOW-001.3:** Each distinct fixed destination profile
  shall have one replacement selector, with its affected steps. The default shall
  use that workflow profile. Reset shall restore that default. Replacements shall
  apply only to this task and shall use the existing profile eligibility rules.
- **AC-TASKS-CHANGE-WORKFLOW-001.4:** Initial-session and earlier-step recipients
  shall appear as conversation relationships, without replacement selectors.
  Steps that reference a replaced fixed-profile step shall show that relationship.
  Existing explicit session bindings shall retain their authority.
- **AC-TASKS-CHANGE-WORKFLOW-001.5:** Changing the destination workflow shall
  clear its step and draft replacements. Loading and failed workflow/profile data
  shall have distinct states. Failed loads shall offer retry. Invalid selections
  shall remain visible with a reason until the user repairs them.
- **AC-TASKS-CHANGE-WORKFLOW-001.6:** Before submission, the form shall show the
  selected step's entry preview with the draft replacements. It shall distinguish
  session reuse, creation, source-session retirement, prompt dispatch, and unknown
  outcomes. Preview failure alone shall not block a valid change. Retry shall be
  available, and stale results shall not appear current.
- **AC-TASKS-CHANGE-WORKFLOW-001.7:** Phone users shall reach the form through a
  visible action. The form shall provide the same choices in a full-height surface,
  with one scrolling body, a reachable primary action, and safe-area clearance.
  Touch targets shall measure at least 44 CSS pixels. Keyboard navigation, labels,
  focus return, and all six supported locales shall cover the complete flow.
- **AC-TASKS-CHANGE-WORKFLOW-001.8:** Cancel shall leave the task unchanged.
  Submission shall prevent duplicate requests. Errors shall preserve valid draft
  choices. Success shall update workflow membership and step indicators without
  navigating away from an open task or changing the board's selected workflow.

### REQ-TASKS-CHANGE-WORKFLOW-002: Consistent task transition

**Intent:** Save the destination and agent choices as one task transition.

#### Acceptance criteria

- **AC-TASKS-CHANGE-WORKFLOW-002.1:** A successful change shall retain the task
  identity, title, description, plans, messages, attachments, relationships,
  linked change requests, repository branches, and workspace resources. It shall
  not rewrite historical session profiles or fabricate completed destination steps.
- **AC-TASKS-CHANGE-WORKFLOW-002.2:** The destination workflow, selected step,
  and replacements shall become effective together. Invalid destinations,
  unauthorized choices, invalid replacements, and persistence failures shall
  leave the original assignment and overrides unchanged.
- **AC-TASKS-CHANGE-WORKFLOW-002.3:** Explicit changes through this form shall
  replace the previous workflow's override map, including when the user keeps all
  defaults. The form shall disclose this before submission. Reload and restart
  shall retain the new choices. Other tasks and shared workflows shall not change.
- **AC-TASKS-CHANGE-WORKFLOW-002.4:** Entry and future transitions shall honor
  the new choices under existing session-target, new/reuse, source-retirement,
  WIP-admission, dependency, and completion rules. A missing earlier-step recipient
  shall follow the existing routing error behavior, without inventing a binding.
  Replacements that become unavailable shall never silently fall back.
- **AC-TASKS-CHANGE-WORKFLOW-002.5:** The change shall retain existing manual-move
  session protections, including the primary-session exception and rejection of
  other starting/running sessions. A stale task snapshot shall fail without
  overwriting a newer task edit or move. Refresh shall preserve still-valid draft
  choices and require another submission. An uncertain response shall trigger a
  task refresh before any retry, without automatic transition replay.
- **AC-TASKS-CHANGE-WORKFLOW-002.6:** Existing moves without explicit mapping
  shall retain their override, API, lifecycle, and bulk behavior. Same-workflow
  `Move to` shall remain separate. The bulk action shall have a distinct plural
  label and shall not imply that it offers per-task agent mapping.

## Scope and related contracts

This capability extends the creation-only scope of
[workflow agent overrides](workflow-agent-overrides.md). It supersedes the
single-task cross-workflow submenu presentation in
[task actions](task-actions-menu.md), [menu grouping](task-menu-grouping.md),
and [action outcomes](task-actions-menu-outcomes.md) when implemented.
Those documents continue to own unrelated actions and bulk behavior.

## Out of scope

- Cross-workspace migration, Office task conversion, or ephemeral-task conversion.
- Bulk mapping, changing workflow definitions, editing agent profiles, or executor changes.
- Replacing Initial Agent, review-action profiles, quorum participants, or dynamic candidates.
- A new session-lifecycle policy, automatic stop-all behavior, or automatic undo.
- MCP/plugin schema expansion and general editing of overrides in the current workflow.

## Delivery

- [System design](../system-design/change-workflow.md)
- [Implementation package](../../../plans/change-workflow/plan.md)
