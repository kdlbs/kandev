---
status: active
system: tasks
created: 2026-08-19
owners:
  - kandev
---
# Create Task Escape Dismissal Requirements

## Overview

This document is the migrated task-system source for the capability. The source detail below remains authoritative while the system is migrated into separate requirement and design records.

## Requirements

### REQ-TASKS-TASK-CREATE-ESCAPE-DISMISSAL-001: Create Task Escape Dismissal

**Intent:** Preserve the observable task or workflow behavior recorded by the legacy specification.

#### Acceptance criteria

- **AC-TASKS-TASK-CREATE-ESCAPE-DISMISSAL-001.1:** When a consumer uses this capability, the system shall provide the observable behavior and exclusions documented below.

## Migrated source detail

## Why

Users can press Escape to close an autocomplete menu while they write a task prompt.
The same key must not close the Create Task dialog and discard the active task-creation flow.

## What

- The standard Create Task dialog remains open when the user presses Escape.
- This rule applies when an autocomplete menu is open and when no menu is open.
- If an autocomplete menu is open, Escape closes that menu, keeps the dialog open, and leaves
  focus in the prompt textarea so typing can continue.
- Escape does not clear the prompt, the autocomplete query, attachments, or other form values.
- The footer Cancel action and other existing dismissal actions keep their current behavior.
- The rule applies to all standard task-creation entry points and all viewport sizes.

## Scenarios

- **GIVEN** the Create Task dialog is open without an autocomplete menu, **WHEN** the user presses Escape, **THEN** the dialog stays open with its form values unchanged.
- **GIVEN** the Create Task dialog has an open `@` autocomplete menu, **WHEN** the user presses Escape, **THEN** the menu closes, the dialog stays open, and the prompt textarea keeps focus.
- **GIVEN** a task-chat `@` or `#` autocomplete menu is open, **WHEN** the user presses Escape, **THEN** the menu closes and the chat editor keeps focus so typing can continue.
- **GIVEN** the Create Task dialog is open on a phone with a hardware keyboard, **WHEN** the user presses Escape, **THEN** the full-height dialog stays open.
- **GIVEN** the Create Task dialog is open, **WHEN** the user selects Cancel, **THEN** the dialog closes through the existing cancellation path.

## Out of scope

- Changing Escape behavior in Edit Task, New Agent, Quick Chat, or other dialogs.
- Changing autocomplete search, selection, or trigger characters.
- Changing pointer, outside-click, browser-back, or Cancel dismissal behavior.
- Changing the dialog layout, general focus order, scroll owner, or mobile composition beyond
  retaining prompt focus after autocomplete dismissal.

## Implementation plan

- [Create Task Escape Dismissal](../../../plans/task-create-escape-dismissal/plan.md)