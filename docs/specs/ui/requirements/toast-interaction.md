---
status: active
system: ui
created: 2026-11-26
owners:
  - kandev
---

# Toast interaction requirements

## Overview

Users can dismiss a notification while they work, even when the notification
has an action or reports a pending operation. Open dialogs keep their controls
visible while notifications appear.

UI owns this shared interaction. Release and agent runtime updates use it, as
does feedback from in-app operations. [Issue #5533](https://github.com/kdlbs/kandev/issues/5533)
reports the overlap during question image review.

## Requirements

### REQ-UI-TOAST-INTERACTION-001: Dismissible toasts and accessible modal actions

**Intent:** Give users control over in-app toasts while they use dialogs and
drawers.

#### Acceptance criteria

- **AC-UI-TOAST-INTERACTION-001.1:** Each shared in-app toast shall show a
  dismiss button with a localized accessible name and a visible keyboard
  focus indicator. Keyboard, pointer, and touch activation shall dismiss that
  toast alone. The target shall measure at least 44 by 44 CSS pixels on phones
  and on devices with a coarse pointer. This includes loading toasts and
  toasts with actions.
- **AC-UI-TOAST-INTERACTION-001.2:** A toast with an action shall present the
  action alongside Dismiss. Selecting a callback action shall run it once
  and dismiss its toast on completion. A link action shall navigate to its
  target. Dismiss shall remove only the toast and leave the dialog open.
- **AC-UI-TOAST-INTERACTION-001.3:** When a shared modal dialog or drawer is
  open, the toast region shall keep its primary actions and close controls
  visible and operable. In question image review, this includes Submit and
  Close with two simultaneous update notices. Phone users shall also retain
  access to the review controls and answer form within the viewport.
- **AC-UI-TOAST-INTERACTION-001.4:** The topmost active modal shall expose the
  toast controls in its normal focus order. Toast arrival shall leave focus
  where it is and announce messages politely in arrival order, including when
  consecutive messages have identical text, regardless of visual placement.
  Modal transitions shall preserve a single visible instance of each toast
  with its content, action, and remaining lifetime.
- **AC-UI-TOAST-INTERACTION-001.5:** Without a modal, each toast shall use its
  normal top or bottom placement. Timed toasts shall expire at their configured
  duration. Toasts created in the loading state shall stay until dismissed or
  updated to a timed state.

## Related contracts

- [Toast theme](toast-theme.md) defines color behavior for both in-app toast renderers.
- [Question image review](../../tasks/requirements/question-image-review.md)
  defines the review controls, the answer form, and focus return.
- [Platform notifications](../../platform/requirements/notifications.md)
  defines notification delivery beyond this shared in-app surface.
