---
status: active
system: tasks
created: 2026-09-14
owners:
  - kandev
---

# Task keyboard actions requirements

## Overview

Operators can complete task moves with supplemental instructions and invoke
sidebar task actions without a pointer. Tasks owns this capability because task
identity, action eligibility, and workflow movement define its behavior.
This extends [task menu grouping](task-menu-grouping.md) and uses the existing
[move overrides contract](../../workflow-step-move-overrides/spec.md).

## Terminology

The target task is the currently open task, matching existing task commands.
A highlighted task search result or an unrelated sidebar selection is not the
target of these commands. Mod means Command on macOS and Control elsewhere.

## Requirements

### REQ-TASKS-KEYBOARD-ACTIONS-001: Submit move options by keyboard

#### Acceptance criteria

- **AC-TASKS-KEYBOARD-ACTIONS-001.1:** With focus in an open move-options surface,
  Mod+Enter shall perform the same move as its enabled Move action, carrying the
  current instructions, reset-context, and skip-step-prompt choices. This applies
  to dialogs, drawers, proceed options, and inline step options.
- **AC-TASKS-KEYBOARD-ACTIONS-001.2:** Plain Enter and Shift+Enter in instructions
  shall remain text editing. Repeated keydown, IME composition, already consumed
  events, and Alt/Shift-modified submit chords shall not initiate a move.
- **AC-TASKS-KEYBOARD-ACTIONS-001.3:** Disabled or pending moves shall not submit;
  overlapping click and shortcut activation shall issue at most one request.
  A failed submission shall keep its draft for correction and retry. Dismissal
  shall not submit, and a successful submission shall use existing completion behavior.
- **AC-TASKS-KEYBOARD-ACTIONS-001.4:** The Move action shall expose a localized,
  platform-correct shortcut hint on keyboard-oriented surfaces. Touch users shall
  retain a visible Move action; attached keyboards shall support the same chord.

### REQ-TASKS-KEYBOARD-ACTIONS-002: Task action parity in the command palette

#### Acceptance criteria

- **AC-TASKS-KEYBOARD-ACTIONS-002.1:** Mod+K shall expose task commands for the
  current task without requiring a live agent session. The palette shall identify
  that task. With no resolvable current task, task commands shall be unavailable.
  Changing task or workspace shall clear any in-progress action choices rather
  than retargeting a draft or confirmation.
- **AC-TASKS-KEYBOARD-ACTIONS-002.2:** Move to shall open searchable eligible steps
  in the task's workflow. Selecting a step shall open its options with instructions
  focused; Mod+Enter shall submit. With no options selected, Move shall use normal
  defaults. Current and ineligible steps shall not be actionable. Cross-workflow
  movement shall remain separately available with the sidebar's target rules.
  Both destination lists shall show step colors and a trailing arrow. Enter opens
  options; Mod+Enter on a selected destination moves immediately with defaults.
  Color choices shall display their color. Archive shall close the palette and
  show a standalone confirmation, including on phones.
- **AC-TASKS-KEYBOARD-ACTIONS-002.3:** For the same single task, the palette shall
  expose each sidebar action with equivalent visibility, enabled state, outcome,
  and confirmation: Pin/Unpin, Color, Priority, Edit, Rename, Duplicate, Create
  subtask, nesting, links, detach, step/workflow moves, primary plugin actions,
  Archive, and Delete. Duplicate shall remain disabled. Card-only plugin Edit
  actions shall not be added. Plugin registration changes shall update the list.
- **AC-TASKS-KEYBOARD-ACTIONS-002.4:** Search, arrow navigation, Enter, and a visible
  Back action shall support nested choices. Escape shall leave the nested choice
  before dismissing the root palette. Closing shall restore focus to the opener;
  invoking an action shall transfer focus to its form or confirmation. No action
  shall drag or navigate an unrelated sidebar row. Archive/Delete shall retain
  their existing confirmation and post-removal navigation behavior.
- **AC-TASKS-KEYBOARD-ACTIONS-002.5:** Phone users shall reach the same applicable
  actions through existing task overflow and, with an attached keyboard, the command palette.
  Nested choices shall use one surface with Back, internal scrolling, safe-area
  clearance, and at least 44px touch targets without document horizontal overflow.
  All new labels, search keywords, hints, and errors shall be localized.

## Out of scope

Bulk-selection commands, a keyboard-shortcut settings system, implementing
Duplicate, new plugin registration contracts, and changes to backend move,
authorization, prompt delivery, persistence, or workflow policy.

## Implementation Plans

- [Task keyboard actions](../../../plans/task-keyboard-actions/plan.md)
