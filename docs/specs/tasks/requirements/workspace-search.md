---
status: active
system: tasks
created: 2026-10-14
owners:
  - kandev
---

# Workspace task search requirements

## Overview

Users and consumers locate workspace tasks by text in task titles,
descriptions, and linked repository names or local paths. Tasks owns this
shared discovery contract because its workspace list supplies matching rows
and totals to the command panel, task mentions, and dependency picker.

## Requirements

### REQ-TASKS-WORKSPACE-SEARCH-001: Literal workspace task text matching

**Intent:** Punctuation in task or repository text must not broaden a search
and displace the actual matching task from a bounded result page.

#### Acceptance criteria

- **AC-TASKS-WORKSPACE-SEARCH-001.1:** A nonempty text query shall match a
  literal substring in at least one of the task title, task description,
  linked repository name, or linked repository local path, subject to the
  requested filters. Percent signs, underscores, and backslashes shall be
  ordinary characters, including when adjacent or at the end of a query.
- **AC-TASKS-WORKSPACE-SEARCH-001.2:** Matching shall retain the existing
  case-insensitive behavior and searchable fields. An empty query shall retain
  ordinary workspace listing behavior; search shall not introduce additional
  whitespace normalization or a wildcard mode.
- **AC-TASKS-WORKSPACE-SEARCH-001.3:** Each matching task shall appear at most
  once. The reported total and every result page shall describe the same
  filtered literal match set and retain the requested sort order. With one
  literal match and more than twenty newer tasks that match only if punctuation
  is treated as a wildcard, the first twenty-row page shall contain that
  literal match and report a total of one.
- **AC-TASKS-WORKSPACE-SEARCH-001.4:** Workspace, workflow, repository,
  archive-mode, ephemeral-mode, configuration-task, and automation-origin
  restrictions shall retain their existing behavior for search and empty
  queries. Archive-only precedence and sorting shall remain unchanged.
- **AC-TASKS-WORKSPACE-SEARCH-001.5:** SQLite and PostgreSQL deployments shall
  provide the same literal punctuation matching. Desktop and phone consumers
  of workspace task search shall receive the corrected rows and totals
  through their existing requests and interactions.

### REQ-TASKS-WORKSPACE-SEARCH-002: Idle command-panel task eligibility

**Intent:** Hidden workflow steps must not displace eligible tasks from the
bounded idle command-panel results.

#### Acceptance criteria

- **AC-TASKS-WORKSPACE-SEARCH-002.1:** When the selected workflow and its steps
  are loaded, the empty-query Commands and Tasks scopes shall select unarchived
  workspace tasks in that workflow whose step is shown in the command panel
  before selecting the first twenty by the existing workspace-list order. If
  twenty newer hidden-step tasks precede an eligible task, that task shall be
  available as an actionable result on desktop and phone.
- **AC-TASKS-WORKSPACE-SEARCH-002.2:** The eligible twenty-row window shall
  retain the existing step-position ordering, five-row Commands preview, and
  twenty-row Tasks limit. An all-hidden workflow shall yield no task results;
  a terminal workflow state alone shall not mean archived.
- **AC-TASKS-WORKSPACE-SEARCH-002.3:** Workspace authorization and existing
  archive, ephemeral, configuration-task, and automation-origin restrictions
  shall remain intact. With no steps loaded, the existing unfiltered fallback
  remains in effect. Requests for a different workspace shall never leak rows.
- **AC-TASKS-WORKSPACE-SEARCH-002.4:** Nonempty metadata and conversation
  searches shall retain literal-query behavior, active-before-archived
  grouping, bounds, selection, and cancellation behavior. The correction shall
  use bounded requests and shall not fetch all task pages into the client.

## Out of scope

- Global, sidebar, board, file, message, or PR-number search redesign.
- New searchable fields, search ordering rules, settings, or normalization.
- Search syntax, schema changes, migrations, or new dependencies.
- Frontend layout, navigation, scrolling, touch targets, and localized copy.

## Related contracts

- [System design](../system-design/workspace-search.md)
- [Archive-result presentation and ranking](command-panel-archived-task-results.md)
- [Board repository matching](../../ui/requirements/board-repository-matching.md),
  especially `AC-UI-BOARD-REPOSITORY-MATCHING-001.4`, owns a separate client-side
  board lens and is not the owner of workspace-list matching.
