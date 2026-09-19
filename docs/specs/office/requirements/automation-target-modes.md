---
status: active
system: office
created: 2026-08-23
owners:
  - Kandev
---

# Automation Target Modes Requirements

## Overview

An automation can run as hidden coordinator work or create an ordinary task
for each firing. Hidden work is useful for scheduled reports and background
coordination. Visible work is useful when a firing should enter a selected
workflow and be handled like a normal task by a person. Both modes can use a
repository-backed execution or a task-owned scratch workspace when no
repository is attached.

## Terminology

- **Task mode:** The persisted firing target. `automation_run` is the hidden
  default. `normal_task` creates a visible task.
- **Repository selection:** An ordered list of repository and base-branch
  pairs. An empty list means that no repository is attached.
- **Scratch workspace:** A task-owned local directory created when a task has
  no attached repository. It is not a repository selection.
- **Visible normal task:** A task with the normal task origin and workflow
  behavior. It appears in the Kanban and sidebar and does not receive the
  coordinator-only MCP surface.

## Requirements

### REQ-OFFICE-AUTOMATION-TARGETS-001: Choose an automation target

**Intent:** Let a workspace owner choose whether an automation firing is
background coordination or ordinary workflow work.

#### Acceptance criteria

- **AC-OFFICE-AUTOMATION-TARGETS-001.1:** When an automation is created or
  edited, the form shall show a visible target choice with hidden automation
  run and normal task options.
- **AC-OFFICE-AUTOMATION-TARGETS-001.2:** When the target is omitted by a new
  client or an existing stored automation has no target value, the system shall
  use `automation_run` so existing automations retain hidden-run behavior.
- **AC-OFFICE-AUTOMATION-TARGETS-001.3:** When the hidden target is selected,
  workflow and repository selection shall each be optional. A firing with no
  workflow or repository shall be admitted and launched in a task-owned scratch
  workspace.
- **AC-OFFICE-AUTOMATION-TARGETS-001.4:** When no repository is attached, the
  selected executor profile shall run in a task-owned scratch workspace.
  Worktree shall remain a valid profile choice and shall not require a Git
  worktree when the repository list is empty.
- **AC-OFFICE-AUTOMATION-TARGETS-001.5:** When the normal task target is
  selected, a workflow shall be required. A missing step shall use the
  workflow's configured starting step. Repository selection shall remain
  explicit, so an empty list never silently falls back to the workspace's
  first repository.
- **AC-OFFICE-AUTOMATION-TARGETS-001.6:** Portable automation export shall
  include the target and repository modes and shall exclude runtime task,
  session, turn, and cleanup pointers.
- **AC-OFFICE-AUTOMATION-TARGETS-001.7:** An invalid target, workflow,
  repository, or executor combination shall fail validation before an
  `AutomationRun` is admitted, and shall return an actionable error to the
  editor or trigger caller.
- **AC-OFFICE-AUTOMATION-TARGETS-001.8:** A trigger shall not cause the editor
  or backend to attach the workspace's first repository. A repository-free
  automation remains repository-free unless the trigger's established event
  contract supplies its own exact repository context.
- **AC-OFFICE-AUTOMATION-TARGETS-001.9:** When a person configures repository
  access, the editor shall let them add one or more explicit repository and
  base-branch pairs or remove every pair. It shall not offer or apply a first
  workspace repository fallback.
- **AC-OFFICE-AUTOMATION-TARGETS-001.10:** When a firing uses selected
  repositories, the system shall create the task environment from the saved
  base branch for each repository and preserve pair order.
- **AC-OFFICE-AUTOMATION-TARGETS-001.11:** When a workflow is selected, the
  editor shall show its ordered step preview and the system shall place a new
  task in that workflow's configured start step. The editor shall not ask the
  person to select a workflow step.

### REQ-OFFICE-AUTOMATION-TARGETS-002: Create and continue visible tasks

**Intent:** Make a selected normal-task automation participate in ordinary
task work without losing exact automation-run accounting.

#### Acceptance criteria

- **AC-OFFICE-AUTOMATION-TARGETS-002.1:** When `normal_task` and `new_task`
  are selected, each firing shall create a distinct visible task in the
  selected workflow, and the task shall appear in the Kanban and sidebar.
- **AC-OFFICE-AUTOMATION-TARGETS-002.2:** When `normal_task` and
  `reuse_thread` are selected, later firings shall continue the same visible
  task and primary session when it is live and compatible. Only one scheduled
  run shall be open at a time.
- **AC-OFFICE-AUTOMATION-TARGETS-002.3:** Every visible firing shall remain a
  distinct `AutomationRun` with exact task, session, and turn identity. A
  visible task shall use the normal agent profile and task lifecycle, not
  `SurfaceAutomation` or hidden-run self-target restrictions.
- **AC-OFFICE-AUTOMATION-TARGETS-002.4:** Completion, failure, and exact-run
  stop shall terminalize only the matching visible run and release its
  automation concurrency slot.
- **AC-OFFICE-AUTOMATION-TARGETS-002.5:** Deleting or disabling an automation
  shall not delete a visible normal task. Hidden automation tasks remain owned
  by the hidden-run cleanup lifecycle.
- **AC-OFFICE-AUTOMATION-TARGETS-002.6:** A visible task with repository mode
  `none` shall retain its normal Kanban/sidebar identity while its agent runs
  in a task-owned scratch workspace.

### REQ-OFFICE-AUTOMATION-TARGETS-003: Explain target choices on every viewport

**Intent:** Make the difference between background work and normal task work
clear before a person saves an automation.

#### Acceptance criteria

- **AC-OFFICE-AUTOMATION-TARGETS-003.1:** The editor shall state that hidden
  per-run tasks do not appear in the Kanban or sidebar, and shall state that
  normal-task firings do appear there.
- **AC-OFFICE-AUTOMATION-TARGETS-003.2:** Target, repository, workflow, and
  executor descriptions shall remain visible without hover or focus and shall
  be associated with their headings or controls for assistive technology.
- **AC-OFFICE-AUTOMATION-TARGETS-003.3:** Target and continuity controls shall
  remain usable with touch targets of at least 44 pixels on mobile. The editor
  shall keep one primary vertical scroll owner and document horizontal overflow
  at zero.
- **AC-OFFICE-AUTOMATION-TARGETS-003.4:** The Export and New Automation actions
  on the automation settings toolbar shall have the same rendered height on
  desktop and mobile.
- **AC-OFFICE-AUTOMATION-TARGETS-003.5:** Repository, workflow, agent-profile,
  and executor-profile selection shall use the same searchable controls,
  logos, repository/branch chips, and workflow step previews as task creation.

## Out of scope

- Concurrent turns or additional sessions on a reusable target.
- Per-automation MCP capability editing or arbitrary executor capability
  allowlists.
- Automatic repository creation for a visible normal task.
- Converting an existing hidden task into a visible task in place. A target
  change takes effect on the next firing and may replace the continuation.
- Deleting visible tasks as part of automation deletion.
