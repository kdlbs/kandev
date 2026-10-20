---
status: current
system: ui
requirements:
  - REQ-UI-EDIT-FOCUS-001
---

# Inline edit focus continuity

## Boundary and mapping

`REQ-UI-EDIT-FOCUS-001` maps to the focus classification, selection, identity,
and consumer sections below. Domain callbacks retain persistence ownership.
The correction requires no backend, storage, or API changes. This local
interaction correction needs no ADR beyond the rationale recorded here.

## Failure evidence

- `components/task/task-top-bar-title.tsx` binds input blur directly to
  `handleCancel`, which exits editing. The next activation seeds `draft`
  from `taskTitle` and loses the unfinished value.
- `components/task/simple/components/task-title-editable.tsx` binds blur to
  `closeEditor`, which releases the Office field guard and exits editing.
- Both title inputs and `components/task/task-rename-dialog.tsx` select all
  text on every focus event. A returning focus event can therefore replace a
  retained selection before the next keystroke.
- `components/task/tab-rename-input.tsx`,
  `components/quick-chat/quick-chat-tab-item.tsx`,
  `components/task/file-context-menu.tsx`, and
  `components/task/inline-file-input.tsx` complete edits on blur.
  Quick Chat already excludes focus moves within its edit container.

Component regressions reproduced cancellation, premature completion, and repeated
selection. Native Chromium tab switching also reproduced a separate Rename
dialog failure: Radix restores the input from the sidebar trigger with
`select: true`, replacing the retained caret with a full selection.

## Focus classification

The frontend hook in `hooks/use-inline-edit-focus.ts` serves the listed
consumers. It accepts the active edit identity, an input/container
reference, and the consumer's existing internal-blur callback. It owns focus
classification and cleanup only. It does not own draft values or saving.

Use the element's owner document. A related target within the edit container
does not finish an edit. A related target elsewhere in the same document
indicates internal focus movement. Do not treat a null related target as proof
of cancellation or completion.

For null targets, defer classification until document focus and visibility
settle. A hidden document or a document without focus preserves the edit.
Record a same-document outside pointer interaction so a click on a
nonfocusable element still invokes the existing internal-blur behavior.
Keep an unresolved external blur nonterminal until a later internal action.
On return, an outside click must still finish the old edit even if the input
did not regain focus first. Handle focus entering an iframe in the document
as an internal move, without inspecting cross-origin frame contents.

Do not use visibility alone: another application or browser chrome can take
focus while the document remains visible. Do not remove all blur behavior:
that would change existing click-away and keyboard navigation contracts.
Install listeners only during editing. Ignore outside pointer and focus events
until the input first receives focus. This preserves editors that focus after
a frame or passive effect. Keep that guard armed through external focus loss.
Cancel queued work on explicit
completion, identity change, and unmount. Use an edit-generation check to
prevent stale callbacks from resolving a later edit. Preserve each consumer's
at-most-once completion guard and add missing guards where event ordering
otherwise creates duplicate completion.

## Selection

Title inputs select all only at edit entry. Native selection survives external
focus changes in the inline consumers. The Rename dialog additionally uses
`useExternalFocusSelection`: remember the input range on external input blur or window blur,
then defer restoration until focus events settle after the input regains focus.
This runs after
Radix selects the restored input. Bind the range to the element and value;
retain it through repeated focus restoration, then discard it on pointer input,
text input, keydown, or unmount. Do not focus the input
from a window-focus or visibility listener.

Keep `useTaskTitleSelectionRestore` and its code-point clamp behavior intact.
That hook repairs selection after value clamping, not edit entry or browser
activation. Verify both paths together at the title length limit.

## Draft and identity ownership

Each component continues to own its local draft. Same-target prop refreshes
must not seed it again during editing. Bind the top-bar edit to `taskId` so a
route change cannot apply an old draft to the next task. Office must retain its
field guard and frozen baseline during external focus changes, then release
them on a real terminal action or task change.

The Rename dialog already seeds on form mount. Preserve that lifetime and
explicit dismissal behavior. Its mobile consumer uses a captured rename
target. No global draft registry or persistent storage is introduced.

## Consumers and responsive behavior

Task top-bar and Office titles use the focus hook. The Rename dialog uses
the selection hook. Session/terminal `TabRenameInput`, Quick Chat rename,
file rename `TreeNodeName`, and `InlineFileInput` use the focus hook.

Desktop keeps its inline title input. Phone keeps the existing task-actions
Rename dialog, reached through `session-task-switcher-sheet-dialogs.tsx`.
Quick Chat keeps its visible touch action menu and existing input sizing.
These short name edits retain their current surfaces, scroll owners, safe-area
handling, and controls. No composition change is proposed.

The shared task create/edit reset effects only reset on an open/context change.
Office description editing has no blur dismissal. Neither needs speculative
draft storage changes. Canvas rename selects on opening, rather than every
focus. Integration/GitHub/GitLab saved-query dialogs have repeated select-all
handlers and remain audit candidates outside this bounded rename repair.

## Evidence boundary

Unit tests control focus/visibility observations and callback counts.
Component tests exercise exact draft text, selection, target replacement,
explicit keyboard actions, and same-target rerenders. Browser tests must
actually move to a second page and back, assert that native focus/visibility
events occurred, type at the retained caret, and check persisted outcomes.
Synthetic blur alone cannot establish browser-tab correctness.

Use the existing desktop Chromium and `mobile-chrome` projects with the full
`chromium` channel. The headless shell does not emit this native transition.
`visitOtherBrowserTab` disables Playwright focus emulation through CDP on both
pages, switches foreground, and checks actual window events and document
focus before and after the switch. Restore driver emulation during cleanup.
Real mobile operating system app suspension remains unverified on a device.
