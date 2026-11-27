---
created: 2026-09-27
status: done
requirements:
  - REQ-UI-SAME-TASK-AGENT-ATTRIBUTION-001
system_design:
  - ../../specs/ui/system-design/same-task-agent-message-attribution.md
legacy_specs: []
---

# Implementation Plan: Same-Task Agent Message Chips

## Overview

Show the sending session's readable label on peer-message chips inside the
same task. One vertical work order shares tab label resolution with the chip,
updates transcript and queued rows, localizes the contextual text, and checks
desktop and phone behavior.

## Scope

### In scope

- Same-task sender labels in transcript and queued message chips.
- Stable attribution when sender session data is unavailable.
- Full sender/task context and localized fallback copy.
- Focused desktop and phone browser proof.

### Out of scope

- Changes to message routing, sender metadata persistence, or session naming.
- New cross-task lookup, navigation, or conversation controls.

## Technical approach

- Reuse the tab's `resolveSessionTabTitle` precedence through a shared store
  selector used by `SessionTab` and `SenderTaskBadge`. Only accept a resolved
  sender session whose `task_id` matches the destination message task.
- Pass destination `task_id` from `ChatMessage` and `QueuedGhostMessage` into
  the shared badge. Branch only when it equals `sender_task_id` and a non-empty
  sender session ID exists; treat an empty ID as absent. Keep the current
  cross-task branch and source-task link.
- When a live session is absent, use `sender_session_name` if present, else a
  localized agent label with a stable short session ID. Keep the task-title
  branch when the sender session ID is absent.
- Add `task` namespace strings in English, Portuguese, Simplified Chinese,
  Traditional Chinese, Japanese, and pseudo-locale catalogs. Use the existing
  Traditional Chinese generation command.

## ASCII UI preview

`UI-01: Same-task transcript`, task conversation after two sibling agents send
messages. This is a change to the chip text; the message bubble remains in its
current position.

```text
Before                                   After
[robot Review Contributor PR #3143]     [robot Luna]
  Can you inspect the PR?                  Can you inspect the PR?
[robot Review Contributor PR #3143]     [robot Astra]
  Here is the finding.                    Here is the finding.

After, full context on keyboard activation or tap:
  From Luna in task "Review Contributor PR #3143"
```

`UI-02: Same-task queue and phone`, queued row or phone transcript. The sender
label is visible without hover; long labels end in an ellipsis inside the
existing surface.

```text
Phone transcript                        Queue row
| [robot Luna]                         | [robot Astra]             |
| Can you inspect the PR?              | Here is the finding.      |
|_____________________________________|___________________________|
```

The chip may truncate, but its semantic button keeps the full context in its
accessible name and opens that context on Enter, Space, click, or tap. Existing
transcript and queue regions own scrolling. Cross-task badges retain the
existing sender-task link. In narrow queue rows, the chip truncates before the
message actions and never covers them. These are structural requirements;
spacing and glyphs are illustrative.

## Tests

| Criteria | Evidence |
| --- | --- |
| `AC-UI-SAME-TASK-AGENT-ATTRIBUTION-001.1`, `.2` | `sender-task-badge.test.tsx` and `chat-message.test.tsx` cover tab-label precedence, updates, and transcript/queue parity. |
| `AC-UI-SAME-TASK-AGENT-ATTRIBUTION-001.3`, `.4` | Badge tests cover full context, truncation, custom-name and ID fallback, and legacy metadata. |
| `AC-UI-SAME-TASK-AGENT-ATTRIBUTION-001.5` | Existing cross-task badge tests plus a focused regression for the unchanged link and title. |

## E2E tests

- `apps/web/e2e/tests/chat/agent-message-attribution.spec.ts` (`chromium`):
  send through `message_task_kandev` with an explicit sibling `session_id`,
  verify persisted queued sender metadata and the rendered chip (`.1`, `.2`,
  `.5`), then open its full context through keyboard navigation (`.3`, `.4`).
- `apps/web/e2e/tests/chat/mobile-agent-message-attribution.spec.ts`
  (`mobile-chrome`): send through the same real sibling-session message path,
  tap the long-label chip to read its full context, and check the 44px hitbox,
  popover viewport containment, no overlap with queue actions at 320px width,
  and zero document horizontal overflow (`.1`, `.3`, `.4`).

## Work orders

- [x] [Task 01: Show sender session on peer chips](task-01-show-sender-session-on-peer-chips.md) (done)

## Verification results

The work order records the post-review unit, typecheck, i18n, lint, spec
validation, and desktop/phone browser results. Both browser scenarios send
through `message_task_kandev` to an explicit sibling session and verify its
persisted queue metadata before rendering the chip.

## Risks

- The session-tab label can depend on live model state; copying only the
  custom session name would leave unnamed Luna/Astra sessions ambiguous.
- Threads and previews can show a task other than the global active task;
  attribution must compare against each message's destination task ID.
- Old messages may have no sender session ID and therefore retain the task
  title chip.
