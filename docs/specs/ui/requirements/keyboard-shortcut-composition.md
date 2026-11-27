---
status: active
system: ui
created: 2026-10-13
owners:
  - kandev
---

# Keyboard Shortcut Composition Requirements

## Overview

Text composition confirmation must remain available while application shortcuts
are enabled. UI owns this independent reusable interaction contract; Tasks
retains creation, validation, and persistence ownership. Task Create is the
qualified consumer motivating this requirement.

The existing [task keyboard actions](../../tasks/requirements/task-keyboard-actions.md)
criterion `AC-TASKS-KEYBOARD-ACTIONS-001.2` covers moves, not creation or the
reusable shortcut hook. [Integration page shortcuts](integration-page-shortcuts.md)
and [plugin shortcut settings](../../plugins/requirements/plugin-shortcut-settings.md)
retain their configuration and dispatch precedence contracts.

## Terminology

- **Composition event:** A keyboard event reporting `isComposing=true`, or the
  existing legacy IME signal `keyCode=229`, independently of `isComposing`.
- **Ordinary chord:** A subsequent keyboard event with neither composition signal
  and the configured key/modifiers.

## Requirements

### REQ-UI-KEYBOARD-SHORTCUT-COMPOSITION-001: Composition-safe reusable shortcuts

**Intent:** Let users confirm composed text without triggering an application
action, while retaining deliberate keyboard activation afterward.

#### Acceptance criteria

- **AC-UI-KEYBOARD-SHORTCUT-COMPOSITION-001.1:** For every composition event
  delivered to the reusable shortcut hooks, whether through a native listener
  or a React handler, the shortcut callback shall not run, even when key and
  modifiers match. Each composition signal shall independently cause rejection.
- **AC-UI-KEYBOARD-SHORTCUT-COMPOSITION-001.2:** Rejection shall not prevent the
  event's default behavior or stop its propagation. An initially unconsumed event
  shall remain unconsumed by this boundary, regardless of consumption options.
- **AC-UI-KEYBOARD-SHORTCUT-COMPOSITION-001.3:** A later ordinary matching Ctrl
  or Meta chord shall activate once under existing eligibility and consumption
  options. Disabled behavior, target/capture routing, modifier matching, and each
  hook's existing handling of already-prevented ordinary events shall remain
  unchanged. Plain Enter shall remain a nonmatch for the submit shortcut.
- **AC-UI-KEYBOARD-SHORTCUT-COMPOSITION-001.4:** In Create Task, composition
  confirmation through the shortcut-backed input shall not dispatch submission
  or task creation, close the dialog, or discard its entered text. Afterward,
  explicit ordinary shortcut submission shall still reach the existing create
  boundary once when eligible; existing compatibility blocks shall still apply.
- **AC-UI-KEYBOARD-SHORTCUT-COMPOSITION-001.5:** The same result shall hold on
  desktop and the existing phone Create Task surface with an attached keyboard.
  Visible touch submission and the existing surface composition shall remain
  available without new controls or copy.

## Out of scope

OS IME rendering or timing, editor engine behavior, unused RichTextInput, direct
event consumers already guarding composition, global shortcut/platform redesign,
repeat-key policy, task save/preservation races, lifecycle changes, backend
transport/schema, new dependencies, and a broad audit of all Enter handlers.

## Implementation Plans

- [Keyboard shortcut composition](../../../plans/keyboard-shortcut-composition/plan.md)
