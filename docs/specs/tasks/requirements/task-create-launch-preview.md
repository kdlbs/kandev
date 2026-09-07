---
status: draft
system: tasks
created: 2026-09-05
owners:
  - kandev
---

# Task Create Launch Preview Requirements

## Overview

The task creation dialog shows the workflow step that the applicable agent-start
action will enter. With an empty description, the primary action is **Start Plan
Mode**, which enters the first positional step. Once a description is present,
the immediate agent-start actions use the workflow auto-start precedence. The
dialog can also preview the step prompt after applying the current task prompt.
This preview lets a person review workflow instructions before starting an agent.

The `tasks` system owns this capability because it owns task creation, launch
routing, and workflow-step prompt behavior. The web dialog presents that
task-owned launch decision.

## Terminology

- **Launch destination:** The workflow step that the applicable immediate
  agent-start action will enter. An empty description selects the first
  positional step for **Start Plan Mode**; a nonempty description follows the
  auto-start, configured-start, and positional fallback order.
- **Step prompt:** The prompt template stored on the launch destination.
- **Composed preview:** The step prompt after the dialog applies substitutions
  that are available before task creation.

## Requirements

### REQ-TASKS-TASK-CREATE-LAUNCH-PREVIEW-001: Launch destination disclosure

**Intent:** A person can see where an immediate task launch will start before
submitting the task.

**User story:** As a person creating a task, I want to see its launch step, so
that the workflow destination is not hidden.

#### Acceptance criteria

- **AC-TASKS-TASK-CREATE-LAUNCH-PREVIEW-001.1:** When the workflow selector
  shows a selected workflow, the system shall show the immediate launch
  destination outside the selector, immediately to its right, with an
  adjacent localized information control followed by the muted label
  **Start step: {{step}}**. The information control shall provide help that
  explains the displayed destination and its action-sensitive precedence.
- **AC-TASKS-TASK-CREATE-LAUNCH-PREVIEW-001.2:** When the description is empty,
  the displayed launch destination shall be the first positional step because
  **Start Plan Mode** uses the plan-mode launch path. When the description is
  nonempty, the displayed destination shall be the first positional step with
  an `auto_start_agent` entry action. If no such step exists, it shall use the
  configured start step, then the first positional step.
- **AC-TASKS-TASK-CREATE-LAUNCH-PREVIEW-001.3:** When the selected workflow
  changes, the displayed destination shall update without using steps from the
  previous workflow.
- **AC-TASKS-TASK-CREATE-LAUNCH-PREVIEW-001.4:** When the destination is not
  available, the system shall omit the destination text. The workflow selector
  shall remain usable.
- **AC-TASKS-TASK-CREATE-LAUNCH-PREVIEW-001.5:** When the description changes
  between empty and nonempty, the displayed destination shall update to match
  the applicable launch path.

### REQ-TASKS-TASK-CREATE-LAUNCH-PREVIEW-002: Step prompt preview

**Intent:** A person can inspect how the launch step changes the task prompt
before starting the agent.

**User story:** As a person creating a task, I want to preview the launch prompt,
so that a step template cannot replace my task prompt without notice.

#### Acceptance criteria

- **AC-TASKS-TASK-CREATE-LAUNCH-PREVIEW-002.1:** When the launch destination has
  a nonempty step prompt, the task prompt toolbar shall show a preview button
  immediately after **Enhance prompt with AI**.
- **AC-TASKS-TASK-CREATE-LAUNCH-PREVIEW-002.2:** When the person selects the
  preview button, the editor shall show the composed preview instead of the
  editable task prompt. Selecting the button again shall restore the unchanged
  task prompt and its editing controls.
- **AC-TASKS-TASK-CREATE-LAUNCH-PREVIEW-002.3:** The composed preview shall
  replace the first `{{task_prompt}}` token with the current task prompt. When
  the token is absent, the step prompt shall be the complete preview.
- **AC-TASKS-TASK-CREATE-LAUNCH-PREVIEW-002.4:** The composed preview shall leave
  task IDs, saved-prompt references, and other server-owned values unresolved.
  The system shall not invent values that do not exist before task creation.
- **AC-TASKS-TASK-CREATE-LAUNCH-PREVIEW-002.5:** When the launch destination has
  no step prompt, the toolbar shall not show the preview button. If a workflow
  change removes the available preview, the editor shall return to edit mode.
- **AC-TASKS-TASK-CREATE-LAUNCH-PREVIEW-002.6:** The preview button shall have a
  localized accessible name that identifies the workflow step prompt and
  resolved step, plus a pressed state. On coarse pointers, its active hit area
  shall be at least 44 CSS pixels.
- **AC-TASKS-TASK-CREATE-LAUNCH-PREVIEW-002.7:** On phone layouts, the prompt
  preview shall remain inside the dialog and shall not cause document-level
  horizontal overflow.

## Out of scope

- Selecting a different initial workflow step.
- Changing backend launch routing or prompt composition.
- Projecting the **Create without starting agent** destination; that action uses
  the configured start step and is not an immediate agent start.
- Showing workflow-level instructions that are separate from the step prompt.
- Expanding saved-prompt references or generating a task ID before creation.
- Adding the preview to task chat, session creation, or subtask creation.
