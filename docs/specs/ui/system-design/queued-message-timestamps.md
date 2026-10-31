---
status: current
system: ui
requirements:
  - REQ-UI-QUEUED-MESSAGE-TIMESTAMPS-001
---

# Queued Message Timestamps System Design

## Purpose and boundaries

Expose the existing queue timestamp through the reusable chat timestamp UI.
This is a presentation extension. It needs no backend change, migration,
new preference, or new API field.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-UI-QUEUED-MESSAGE-TIMESTAMPS-001 | Data source, Shared presentation, Responsive behavior, Verification |

## Data source

`QueuedMessage.queued_at` in `apps/web/lib/state/slices/session/types.ts`
already arrives through queue status responses and store reconciliation.
Render this server value without deriving a timestamp from browser receipt time.

In `internal/orchestrator/messagequeue/auto_merge.go`, `latestQueuedAt` selects
the maximum time. `buildAutoMergedEntry` uses it for automatic merges.
Manual merging in `repository_sqlite.go` persists and returns the same maximum.
`useEntryMutations` in `use-queue.ts` refetches after manual merging.
Repeated merges therefore retain the maximum across all source messages,
independent of queue position. Existing persistence tests cover this behavior.

The manual-merge requirement records the same maximum-time behavior.
The active automatic-merge contract already specifies the maximum in
AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.9.

## Shared presentation

`MessageTimestamp` lives in `messages/message-timestamp.tsx` and serves both
`messages/message-actions.tsx` and the queue. Transcript behavior and selectors
remain stable. `QueuedGhostMessage` renders it in `DisplayView`, after
the message preview and attachments inside the existing flexible content column.
Render it independently of text presence so attachment-only rows receive it.

Keep `formatMessageTime` as the formatting and strict timestamp-validation
boundary. Read `userSettings.messageTimeDisplay` and the active locale through
the existing hooks. Reuse `task:messageTime` and
`task:messageTimestampAriaLabel`. No new translation keys are expected.

The component accepts an optional `wrap` property for queue containment.
Use existing muted text, small monospace type, and tabular numbers.
Allow long absolute labels to wrap within the content column. Do not introduce
a fixed time column, new icon, animation, or additional scroll region.

## Responsive behavior

The entry point remains the queue chip and expanded queue panel.
The nearest mobile exemplar is the existing transcript `MessageTimestamp`:
`useTouchDrawer` selects a details drawer for phone and coarse-pointer input.
Reuse that interaction, focus return, and 44-pixel trigger minimum.
The timestamp drawer marks its Escape boundary with `data-queue-escape-boundary`.
`useQueuePanelEscape` defers to that boundary while the drawer has
`data-state="open"`, including the interval before focus moves into it.
Closing the drawer therefore leaves the queue panel open.
The queue panel remains the scroll owner. The timestamp stays outside the
drag handle and action controls. Both desktop and phone use the same content
order, with more wrapping permitted on phones.

## Failure and recovery

Invalid times render nothing through the formatter's existing null result.
Queue refetch and event reconciliation remain authoritative after merges.
No client timestamp fallback may replace missing server evidence.
Existing rows merged by an older backend can only show their retained value.
Deleted source rows cannot supply historical timestamps.

## Verification

Component tests cover queue timestamps, all display modes, invalid input,
attachment-only rows, sender kinds, updated entry props, and the touch drawer.
Keep transcript timestamp tests as extraction regression coverage.
Desktop and mobile browser tests exercise manual and automatic merging,
reload, newest-time selection, and contained long dates.
Use server status values as expected timestamps and causal event waits.

## Implementation plans

- [Queue timestamps](../../../plans/queued-message-timestamps/plan.md)
