---
status: active
system: ui
created: 2026-10-28
owners:
  - kandev
---

# Queued Message Timestamps Requirements

## Overview

Users need to see when pending messages entered the queue. A merged message
must show the newest time among its contributing messages.

UI owns this presentation contract alongside the existing queue controls.
Queue persistence, admission, and delivery retain their existing owners.
This capability extends the [message-time preference](message-time-display.md)
to pending queue rows.

## Requirements

### REQ-UI-QUEUED-MESSAGE-TIMESTAMPS-001: Pending message time

**Intent:** Make the age of pending content visible before delivery.

#### Acceptance criteria

- **AC-UI-QUEUED-MESSAGE-TIMESTAMPS-001.1:** Each pending message with a valid queued time shall display that time in its queue row. This includes attachment-only messages and every sender kind, in task chat and Quick Chat.
- **AC-UI-QUEUED-MESSAGE-TIMESTAMPS-001.2:** After manual or automatic merging, the surviving row shall display the latest queued time among all contributing messages. Repeated merges, reordered rows, and reloads shall preserve this result. The displayed time shall not be the time of the merge action.
- **AC-UI-QUEUED-MESSAGE-TIMESTAMPS-001.3:** The timestamp shall follow the saved message-time preference, interface locale, regional formatting, and browser time zone. It shall expose the same counterpart form as transcript timestamps.
- **AC-UI-QUEUED-MESSAGE-TIMESTAMPS-001.4:** Missing or invalid queued times shall produce no timestamp or details control. The message and its actions shall remain usable.
- **AC-UI-QUEUED-MESSAGE-TIMESTAMPS-001.5:** Desktop and phone rows shall show the timestamp below the preview. Phone users shall access its counterpart through a details drawer. Touch targets shall measure at least 44 pixels in both dimensions. Long localized dates shall remain contained without overlapping row actions or causing horizontal page overflow. Accessible names shall include the label and counterpart. Escape shall close the details drawer while keeping the queue panel open.

## Out of scope

New time settings, periodic time refresh, merge eligibility changes, historical
timestamp reconstruction, and timestamps in the collapsed queue-count chip.
The inline queue editor retains its current layout.

## Implementation plans

- [Queue timestamps](../../../plans/queued-message-timestamps/plan.md)
