---
status: active
system: tasks
created: 2026-10-27
owners:
  - kandev
---

# Daily Activity Statistics Requirements

## Overview

Tasks owns the persisted task, session, turn, and message activity projected by
workspace Statistics. The separate [calendar label contract](../../ui/requirements/stats-calendar-labels.md)
owns presentation of date keys; [completion statistics](completion-statistics.md)
owns completed-task counts.

## Requirements

### REQ-TASKS-DAILY-ACTIVITY-001: Daily activity attribution

**Intent:** Users see persisted activity on the UTC day it happened, even when
an agent turn continues across midnight.

#### Acceptance criteria

- **AC-TASKS-DAILY-ACTIVITY-001.1:** For every persisted message belonging to an
  eligible task session in the selected workspace, the daily activity response
  shall count it once in the UTC date of its creation when that date is within
  the requested horizon. A same-day turn, or any turn starting within that
  horizon, shall not be required for the message to count.
- **AC-TASKS-DAILY-ACTIVITY-001.2:** The response shall retain the existing
  exclusion of ephemeral tasks and automation-run tasks and shall not include
  messages from another workspace. It shall retain the existing message-type
  eligibility, including user and agent messages and tool activity.
- **AC-TASKS-DAILY-ACTIVITY-001.3:** Turn counts and distinct task counts shall
  retain their existing attribution to UTC turn-start days. Multiple turns or
  messages in a session shall not multiply any count. Message-only days shall
  not create turn or task counts.
- **AC-TASKS-DAILY-ACTIVITY-001.4:** The existing range selection, inclusive
  UTC date horizon, ascending order, zero-filled days, response fields, limits,
  authorization and read availability/error behavior shall remain compatible.
  Activity outside the returned dates shall not enter a bucket.
- **AC-TASKS-DAILY-ACTIVITY-001.5:** Native Statistics on desktop and phone
  shall consume the corrected daily message counts through its existing activity
  heatmap, including its activity intensity and disclosed message count.
  Existing layout, interaction, copy, loading and error behavior shall remain
  unchanged.
- **AC-TASKS-DAILY-ACTIVITY-001.6:** Reading activity statistics shall not
  modify tasks, sessions, turns, messages, or other persisted state.

## Out of scope

Calendar labels and timezone presentation, completion statistics, data repair,
frontend redesign, session lifecycle changes, and unrelated Statistics metrics.

## Implementation plans

- [Overnight activity statistics](../../../plans/overnight-activity-statistics/plan.md)
