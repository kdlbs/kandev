---
status: active
system: ui
created: 2026-07-31
owners:
  - Kandev frontend
---
# Task Review Shortcut Switcher Requirements

## Overview

Keyboard users opening one of several reviews linked to a task must leave the shortcut chord and use arrow keys before choosing a review. A hold-and-repeat switcher keeps selection inside the original shortcut flow.

## Requirements

### REQ-UI-TASK-REVIEW-SHORTCUT-001: Task Review Shortcut Switcher

**Intent:** Keyboard users opening one of several reviews linked to a task must leave the shortcut chord and use arrow keys before choosing a review. A hold-and-repeat switcher keeps selection inside the original shortcut flow.

#### Acceptance criteria

- **AC-UI-TASK-REVIEW-SHORTCUT-001.1:** The configurable **Open Task Pull Request** shortcut keeps its existing no-review toast and single-review direct-open behavior.
- **AC-UI-TASK-REVIEW-SHORTCUT-001.2:** When a task has multiple linked reviews, the first shortcut press opens the review picker with its first displayed row selected.
- **AC-UI-TASK-REVIEW-SHORTCUT-001.3:** While the shortcut's primary modifier remains held, each additional deliberate press of the configured shortcut key advances selection by one displayed row and wraps from the last row to the first. Secondary modifiers such as Shift may be released after the first chord. Operating-system key repeat does not advance selection.
- **AC-UI-TASK-REVIEW-SHORTCUT-001.4:** Cycling includes every row already shown by the picker: GitHub pull requests followed by GitLab merge requests, preserving their displayed order.
- **AC-UI-TASK-REVIEW-SHORTCUT-001.5:** Releasing the shortcut's primary hold modifier opens the selected review at its provider and closes the picker. For the default binding, this is Command on macOS and Control on Windows or Linux; releasing Shift alone does not open the review.
- **AC-UI-TASK-REVIEW-SHORTCUT-001.6:** Custom shortcut bindings use their configured key and primary hold modifier. A modifierless binding can open and cycle the picker, but requires Enter or a click to activate the selection because it has no modifier-release signal.
- **AC-UI-TASK-REVIEW-SHORTCUT-001.7:** ArrowUp, ArrowDown, Enter, click, and Escape remain supported. Escape, application-window blur, or document hiding cancels the switcher, and a later modifier release does not open a review.
- **AC-UI-TASK-REVIEW-SHORTCUT-001.8:** The selected row remains visibly and programmatically identifiable while the picker is open.

## Migrated source detail

## Why

Keyboard users opening one of several reviews linked to a task must leave the
shortcut chord and use arrow keys before choosing a review. A hold-and-repeat
switcher keeps selection inside the original shortcut flow.

## What

- The configurable **Open Task Pull Request** shortcut keeps its existing
  no-review toast and single-review direct-open behavior.
- When a task has multiple linked reviews, the first shortcut press opens the
  review picker with its first displayed row selected.
- While the shortcut's primary modifier remains held, each additional deliberate
  press of the configured shortcut key advances selection by one displayed row
  and wraps from the last row to the first. Secondary modifiers such as Shift
  may be released after the first chord. Operating-system key repeat does not
  advance selection.
- Cycling includes every row already shown by the picker: GitHub pull requests
  followed by GitLab merge requests, preserving their displayed order.
- Releasing the shortcut's primary hold modifier opens the selected review at
  its provider and closes the picker. For the default binding, this is Command
  on macOS and Control on Windows or Linux; releasing Shift alone does not open
  the review.
- Custom shortcut bindings use their configured key and primary hold modifier.
  A modifierless binding can open and cycle the picker, but requires Enter or a
  click to activate the selection because it has no modifier-release signal.
- ArrowUp, ArrowDown, Enter, click, and Escape remain supported. Escape,
  application-window blur, or document hiding cancels the switcher, and a later
  modifier release does not open a review.
- The selected row remains visibly and programmatically identifiable while the
  picker is open.
- The existing touch and coarse-pointer review entry points and picker layout do
  not change.

## Scenarios

- **GIVEN** a task has two linked reviews, **WHEN** the user holds the default
  primary modifier and Shift and presses G once, **THEN** the picker opens with
  the first displayed review selected.
- **GIVEN** the held shortcut picker is open on the first of three linked
  reviews, **WHEN** the user presses G three more times while keeping the primary
  modifier held, **THEN** selection advances through the second and third rows
  and wraps to the first row.
- **GIVEN** the held shortcut picker has a GitLab merge-request row selected,
  **WHEN** the user releases the primary modifier, **THEN** that merge request
  opens at GitLab and the picker closes.
- **GIVEN** the default shortcut picker is open, **WHEN** the user releases Shift
  while continuing to hold the primary modifier, **THEN** the picker stays open
  and no review opens.
- **GIVEN** the held shortcut picker is open, **WHEN** the user presses Escape
  and later releases the primary modifier, **THEN** the picker closes without
  opening a review.
- **GIVEN** the shortcut is rebound to a modified key chord, **WHEN** the user
  repeats that configured key and releases its primary modifier, **THEN** the
  configured chord cycles and opens the selected review exactly like the
  default chord.
- **GIVEN** the shortcut is rebound without a modifier, **WHEN** the user repeats
  its key, **THEN** the picker cycles and stays open until the user presses Enter,
  clicks a row, or cancels it.
- **GIVEN** a task has exactly one linked review, **WHEN** the user invokes the
  shortcut, **THEN** that review opens directly without showing the picker.
- **GIVEN** a task has no linked reviews, **WHEN** the user invokes the shortcut,
  **THEN** Kandev shows the existing no-linked-review message.

## Out of scope

- Changing review ordering, linked-review discovery, or provider data.
- Adding reverse cycling or another shortcut.
- Changing the in-app Review dialog's separate PR selector.
- Changing touch, mobile, or coarse-pointer presentation.

## Implementation plan

[Task Review Shortcut Switcher implementation plan](../../../plans/task-review-shortcut-switcher/plan.md)
