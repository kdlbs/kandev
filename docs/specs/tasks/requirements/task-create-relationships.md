---
status: active
system: tasks
created: 2026-11-05
owners:
  - kandev
---

# Task-create relationship establishment Requirements

## Overview

Today a new task card can be created with no link to any other card. The only
in-dialog relationship control is the "Depends on" selector inside the
create-dialog's advanced settings; parent assignment exists only through the
separate New Subtask dialog, and nothing lets a creator mark two tasks as
associated without ordering one behind the other. The result is a task
relationship graph ([`task-relationship-graph.md`](task-relationship-graph.md))
that fills with disconnected nodes whenever a creator skips the dependency
picker or has no dependency to declare, and subtasks that are never
rediscovered as part of a larger unit of work.

This requirement record defines an optional, low-friction way to establish a
task's relationships to other tasks at the moment of creation, for every
creation path: the main New Task dialog, the New Subtask dialog, and
agent-driven creation through `create_task_kandev`. It adds a new
non-blocking "related to" relationship alongside the existing parent and
depends-on relationships, and it defines a shared suggestion contract so a
creator sees likely candidates before falling back to manual search.

## Requirements

### REQ-TASKS-TASK-CREATE-RELATIONSHIPS-001: Parent selection in the main create dialog

**Intent:** Let a creator attach a new task to a parent without leaving the
main create flow or switching to the separate New Subtask dialog.

#### Acceptance criteria

- **AC-TASKS-TASK-CREATE-RELATIONSHIPS-001.1:** When a creator opens the main
  New Task dialog's advanced settings in create mode with no parent already
  supplied by the caller, a Parent selector shall be available alongside the
  existing Depends-on selector, defaulting to no parent.
- **AC-TASKS-TASK-CREATE-RELATIONSHIPS-001.2:** When a creator opens the
  Parent selector, it shall offer non-archived, non-descendant candidate
  tasks ordered by the suggestion contract in
  REQ-TASKS-TASK-CREATE-RELATIONSHIPS-003, and shall exclude the task's own
  prospective identity and any task that is not eligible to be a parent.
- **AC-TASKS-TASK-CREATE-RELATIONSHIPS-001.3:** When a creator selects a
  parent candidate and submits the form, the created task shall have that
  task as its parent, matching the existing parent-assignment contract used
  by the New Subtask dialog.
- **AC-TASKS-TASK-CREATE-RELATIONSHIPS-001.4:** When the main create dialog
  already receives an externally supplied parent (for example, launched from
  an existing New Subtask entry point), the Parent selector shall not be
  shown and the externally supplied parent shall take effect unchanged.
- **AC-TASKS-TASK-CREATE-RELATIONSHIPS-001.5:** When a creator leaves the
  Parent selector at its default, the task shall be created with no parent,
  with no warning or confirmation step.

### REQ-TASKS-TASK-CREATE-RELATIONSHIPS-002: Related-to link at creation

**Intent:** Let a creator associate a new task with one or more existing tasks
that are neither an ordering dependency nor a parent, so related but
unordered work stays discoverable.

#### Acceptance criteria

- **AC-TASKS-TASK-CREATE-RELATIONSHIPS-002.1:** When a creator opens the main
  New Task dialog's advanced settings, a Related-to selector shall be
  available alongside the Depends-on and Parent selectors, supporting
  multiple selected tasks, defaulting to none selected.
- **AC-TASKS-TASK-CREATE-RELATIONSHIPS-002.2:** When a creator selects one or
  more related tasks and submits the form, the system shall record a
  non-blocking, non-hierarchical relationship between the created task and
  each selected task. This relationship shall not affect auto-start,
  blocking, or dependency-resolution behavior for either task.
- **AC-TASKS-TASK-CREATE-RELATIONSHIPS-002.3:** A related-to relationship
  shall be symmetric: once established, each of the two tasks shall report
  the other as related to it.
- **AC-TASKS-TASK-CREATE-RELATIONSHIPS-002.4:** When a creator leaves the
  Related-to selector at its default, the task shall be created with no
  related-to links, with no warning or confirmation step.
- **AC-TASKS-TASK-CREATE-RELATIONSHIPS-002.5:** The Related-to selector shall
  exclude the task's own prospective identity, any task already selected as
  a dependency, and any task already selected as the parent, so the same
  pair of tasks is not linked through two relationship kinds at once.

### REQ-TASKS-TASK-CREATE-RELATIONSHIPS-003: Smart candidate suggestions

**Intent:** Reduce the effort of linking a new task by surfacing likely
candidates before a creator needs to search.

#### Acceptance criteria

- **AC-TASKS-TASK-CREATE-RELATIONSHIPS-003.1:** When a creator opens the
  Parent or Related-to selector (and the existing Depends-on selector) from a
  known board context, candidates from the same swimlane or column as that
  context shall be ranked above candidates from other swimlanes or columns.
- **AC-TASKS-TASK-CREATE-RELATIONSHIPS-003.2:** Tasks the creator has
  recently viewed or created in the active session shall be ranked above
  candidates with no such recent interaction, after the same-swimlane-or-column
  group in AC-003.1.
- **AC-TASKS-TASK-CREATE-RELATIONSHIPS-003.3:** For the Depends-on and
  Related-to selectors, once a creator has typed search text, a candidate
  whose title textually matches the search text shall be ranked above a
  candidate that matches only by id or change-request number.
- **AC-TASKS-TASK-CREATE-RELATIONSHIPS-003.4:** When none of the ranking
  signals in AC-003.1 through AC-003.3 apply or distinguish two candidates,
  the existing most-recently-updated, then most-recently-created, then title
  ordering shall apply as the fallback.
- **AC-TASKS-TASK-CREATE-RELATIONSHIPS-003.5:** The ranking in this
  requirement shall apply to the Parent and Related-to selectors introduced
  by this capability and to the existing Depends-on selector, so all three
  selectors offer consistent suggestions.

### REQ-TASKS-TASK-CREATE-RELATIONSHIPS-004: Relationship establishment for agent-created tasks

**Intent:** Give an agent creating a task through the MCP tool the same
ability to establish parent, depends-on, and related-to relationships as a
human using the create dialog, so agent-created tasks are not systematically
less connected.

#### Acceptance criteria

- **AC-TASKS-TASK-CREATE-RELATIONSHIPS-004.1:** `create_task_kandev` shall
  accept an optional list of related-task identifiers, in addition to its
  existing `parent_id` and `blocked_by` parameters.
- **AC-TASKS-TASK-CREATE-RELATIONSHIPS-004.2:** When the related-task list
  contains a valid, non-archived task identifier, the created task shall have
  a related-to relationship with that task, matching the symmetric contract
  in AC-TASKS-TASK-CREATE-RELATIONSHIPS-002.3.
- **AC-TASKS-TASK-CREATE-RELATIONSHIPS-004.3:** When the related-task list
  contains an identifier that does not resolve to an eligible task, task
  creation shall fail with a validation error identifying the invalid
  identifier, matching the existing `blocked_by` validation contract, rather
  than silently dropping the reference.
- **AC-TASKS-TASK-CREATE-RELATIONSHIPS-004.4:** Omitting the related-task
  parameter shall create the task with no related-to links and shall not
  change any existing `create_task_kandev` behavior.

### REQ-TASKS-TASK-CREATE-RELATIONSHIPS-005: Related-to visibility after creation

**Intent:** Let anyone viewing a task discover its related-to links the same
way they already discover its dependencies.

#### Acceptance criteria

- **AC-TASKS-TASK-CREATE-RELATIONSHIPS-005.1:** The task dependency chip shall
  show a third, separately labeled section listing related-to tasks when at
  least one exists, alongside its existing "Waiting on" and "Blocks"
  sections.
- **AC-TASKS-TASK-CREATE-RELATIONSHIPS-005.2:** A task with only related-to
  links and no dependency edges or parent shall make its dependency chip and
  task-graph entry control visible, matching the visibility contract in
  [`task-relationship-graph.md`](task-relationship-graph.md) REQ-001 as
  amended by this capability.
- **AC-TASKS-TASK-CREATE-RELATIONSHIPS-005.3:** Removing a related-to link is
  out of scope for this requirement; the Edit task dialog remains the
  reference point for any future editing capability, matching the existing
  dependency-editing ownership split.

## Out of scope

- Editing or removing an established parent, dependency, or related-to link
  from the task graph. The graph remains read-only.
- Changing the existing Depends-on selector's AND-blocking semantics, or
  making a related-to link participate in auto-start or blocking behavior.
- A forced or warned relationship step. Every selector introduced by this
  capability is optional and skippable with no confirmation step.
- Bulk relationship editing, or establishing relationships for tasks other
  than the one being created.
- Changing the New Subtask dialog's existing parent-defaulting behavior
  ([`subtask-dialog-parent-defaults.md`](subtask-dialog-parent-defaults.md)).
  This capability adds the Related-to selector to that dialog but does not
  change how its parent is supplied.
