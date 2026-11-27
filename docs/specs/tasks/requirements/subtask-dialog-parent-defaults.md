---
status: active
system: tasks
created: 2026-10-14
owners:
  - kandev
---

# Subtask Dialog Parent Defaults

## Overview

Creating a subtask from a task row must use that selected parent's context,
including when another task is open and parent data arrives after the dialog
opens. Tasks owns this creation contract; desktop and phone share its outcome.

## Requirements

### REQ-TASKS-SUBTASK-DIALOG-PARENT-DEFAULTS-001: Selected Parent Initial Defaults

**Intent:** Initialize the subtask form from its specified parent without
replacing work the user has already entered.

#### Acceptance criteria

- **AC-TASKS-SUBTASK-DIALOG-PARENT-DEFAULTS-001.1:** When a user opens New Subtask for a specified parent, the form shall use that parent's context even when the parent is absent from the current kanban view and another task is active. The active task or its selected session shall not supply replacement defaults.
- **AC-TASKS-SUBTASK-DIALOG-PARENT-DEFAULTS-001.2:** When initial parent context becomes available, untouched fields shall receive its defaults once. The lowest-position parent repository and its base branch shall take precedence over primary-session repository/base fallbacks. The parent's explicitly identified primary session, or otherwise its session marked primary, shall supply the profile and worktree context; a secondary session shall not substitute. A parent worktree branch shall default workspace mode to inheritance; otherwise the initial settled context shall default to a new workspace.
- **AC-TASKS-SUBTASK-DIALOG-PARENT-DEFAULTS-001.3:** Late initial data and later refreshes shall not replace entered title/prompt or an explicit repository, profile, or workspace-mode choice. An intentional removal or cleared choice shall remain a user choice; later refreshes shall not reapply initial defaults.
- **AC-TASKS-SUBTASK-DIALOG-PARENT-DEFAULTS-001.4:** When the parent has no sessions, is unavailable, or its session read fails, the form shall remain usable through existing controls and validation without indefinite loading or substitution of another task's context. Available parent repository context shall remain usable without a session.
- **AC-TASKS-SUBTASK-DIALOG-PARENT-DEFAULTS-001.5:** Desktop and phone shall use the same selected-parent/defaulting behavior through their existing New Subtask controls, preserving current layout, translated labels, and submission behavior.

## Exclusions

Task/session loading policy, agent eligibility, APIs, persistence, workspace
materialization, task activation, generic form initialization, and changes to
title generation or prompt-copy actions are outside this contract.

## Design and delivery

- [System design](../system-design/subtask-dialog-parent-defaults.md)
- [Required-CI correction work order](../../../plans/shortcut-conflict-dispatch/task-01-match-accepted-chords.md#required-ci-correction-addendum)
