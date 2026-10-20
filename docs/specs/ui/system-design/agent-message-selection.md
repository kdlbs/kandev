---
status: current
system: ui
requirements:
  - REQ-UI-AGENT-MESSAGE-COMMENTS-001
---

# Agent-message selection

## Purpose and boundaries

This design covers selection capture for the existing
[agent-message comment requirement](../requirements/agent-message-comments.md).
Task Chat and Quick Chat share `MessageCommentSurface`. Comment persistence,
delivery, and message eligibility retain their existing contracts.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-AGENT-MESSAGE-COMMENTS-001` | [Selection capture](#selection-capture), [Presentation and validation](#presentation-and-validation) |

## Selection capture

An eligible mounted message subscribes to its owner document's `mousedown` and
`mouseup` events while its comment editor is closed. Capture runs only when the
mouse interaction starts inside that message. Unrelated controls cannot restore
a dismissed trigger from a preserved DOM selection. Releasing in surrounding
padding must still capture a selection whose boundaries belong entirely to that
message.
`getMessageSelection` validates both boundaries and rejects empty selections.
Each subscription is removed when eligibility changes, the editor opens, or the
message unmounts. Mouse activity inside an open editor cannot replace its quote.

The message-local `touchend` handler and existing keyboard shortcut use the same
selection validation. Streaming replies, raw views, and unsupported message
types retain their current restrictions.

## Presentation and validation

The existing trigger opens a desktop popover or a touch drawer. No layout,
labels, touch targets, or storage format change. The drawer remains the phone
surface for editing selected feedback, with the existing Add and Run actions.

Component tests cover releases outside the body, invalid selections, the open
editor, and dismissal when an unrelated control preserves the DOM selection.
Browser tests use real mouse drags in both chat surfaces. Existing mobile tests
cover drawer interaction and comment delivery.
