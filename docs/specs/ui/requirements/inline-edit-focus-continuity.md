---
status: active
system: ui
created: 2026-10-18
owners:
  - kandev
---

# Inline edit focus continuity

## Overview

Users can consult another browser tab or application while editing a name.
Returning to Kandev must preserve their unfinished edit. UI owns this reusable
focus interaction. Tasks, Office, and Workspaces retain validation, mutation,
permissions, and confirmed data ownership.

The initial consumers are task titles, session and terminal tab names,
Quick Chat tab names, and file-tree rename/create inputs. The task Rename
dialog also shares the selection behavior.

## Terms

- **External focus change:** The user switches browser tabs, browser windows,
  applications, or browser chrome while the same Kandev page remains mounted.
- **Internal focus change:** The user moves focus to another control or clicks
  outside the editor within the Kandev document.
- **Edit lifetime:** One open edit of one target, ending with an explicit
  completion, cancellation, target change, or unmount.

## Requirements

### REQ-UI-EDIT-FOCUS-001: Preserve an edit across external focus changes

**Intent:** External focus changes must not discard or submit unfinished input.

#### Acceptance criteria

- **AC-UI-EDIT-FOCUS-001.1:** An external focus change shall keep the editor
  open and retain its exact draft, including whitespace and an empty value.
  The focus change shall not submit, cancel, or report a successful save.
- **AC-UI-EDIT-FOCUS-001.2:** Returning to the editor shall preserve the user's
  selection or caret. Initial select-all behavior shall occur only when an
  edit starts. The next keystroke shall edit at the retained selection.
- **AC-UI-EDIT-FOCUS-001.3:** Internal focus changes shall retain each editor's
  existing completion behavior. Task inline titles cancel. Tab renames and
  file-tree inputs commit according to their existing validation rules.
  Focus moves within a Quick Chat edit container shall remain nonterminal.
- **AC-UI-EDIT-FOCUS-001.4:** Explicit save, Enter, Escape, and Cancel shall
  retain their existing validation and composition rules. A completed edit
  shall produce at most one terminal action. Deferred focus work shall not
  submit or cancel a later edit or a different target.
  In shared session and terminal tab renames, Enter or Escape during IME
  composition shall leave the rename open without submitting or cancelling
  it, and shall preserve the key's default composition behavior. A separate
  Enter after composition finishes shall commit the completed draft once.
- **AC-UI-EDIT-FOCUS-001.5:** Updates to confirmed data for the same target
  shall not replace an open draft. Changing the edit target or unmounting
  shall release the old edit without applying its draft to another target.
- **AC-UI-EDIT-FOCUS-001.6:** Desktop and phone entry points shall preserve
  unfinished input across external focus changes. Returning shall not force
  focus or reopen the phone keyboard over another control.
- **AC-UI-EDIT-FOCUS-001.7:** When an earlier task Rename save succeeds or
  fails after a later edit opens, its completion shall preserve the later
  dialog and exact draft, including a reopened edit of the same task.
  The submitted save shall retain its original target and normalized title,
  immediate dialog dismissal, and existing failure feedback. The later edit
  shall remain available for explicit Save or Cancel.

## Compatibility

This contract distinguishes external focus changes from internal blur.
It narrows the meaning of blur in the Office title contract's AC-5.
The [Office content contract](../../office-task-content-editing/spec.md)
continues to own its field guards, baseline, and explicit completion behavior.

## Exclusions

- Persistence across reload, restart, explicit dismissal, or route unmount.
- Changes to save failures, optimistic rollback, authorization, or APIs.
- A general rewrite of settings autosave or all focus handlers.
- New animations, controls, labels, or responsive layout changes.

## Implementation plan

- [Preserve inline edit drafts](../../../plans/inline-edit-focus-continuity/plan.md).
