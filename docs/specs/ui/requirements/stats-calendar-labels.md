---
status: active
system: ui
created: 2026-10-19
owners:
  - kandev
---

# Statistics Calendar Labels Requirements

## Overview

Statistics presents date-only activity buckets as calendar labels. UI owns this
presentation contract shared by completed-task charts and the activity heatmap:
it remains meaningful for any supplied bucket values, independently of task
completion eligibility or activity collection. The [completion projection](../../tasks/requirements/completion-statistics.md)
continues to own which completed tasks contribute, and
[interactive reads](../../platform/requirements/interactive-read-availability.md)
continues to own section availability and recovery. Neither contract changes.

## Terminology

- **Calendar key:** A date-only `YYYY-MM-DD` bucket value, not an instant to
  convert to the browser timezone.
- **Completion bucket:** An existing day, Monday-start week, or month containing
  the supplied calendar keys.

## Requirements

### REQ-UI-STATS-CALENDAR-LABELS-001: Stable calendar attribution

**Intent:** Users can attribute plotted activity to the correct calendar date
regardless of their browser timezone.

**User story:** As a Statistics reader, I want dates beside activity counts to
identify their supplied calendar buckets on both desktop and phone.

#### Acceptance criteria

- **AC-UI-STATS-CALENDAR-LABELS-001.1:** Every completed-task day label shall identify its calendar key;
  every week label shall identify the Monday starting its bucket; every month
  label shall identify its bucket month and year, independently of browser
  timezone, including month and year crossings.
- **AC-UI-STATS-CALENDAR-LABELS-001.2:** Changing Day, Week, or Month shall retain existing bucket membership,
  counts, ascending order, and bar cardinality. Visible endpoint labels and
  disclosed bar tooltip labels shall agree for the same bucket.
- **AC-UI-STATS-CALENDAR-LABELS-001.3:** Each disclosed activity heatmap date shall identify its supplied
  calendar key without timezone conversion. Activity values and grid placement
  shall remain compatible.
- **AC-UI-STATS-CALENDAR-LABELS-001.4:** Desktop and phone shall present the same calendar attribution
  through their existing Statistics surfaces and bucket controls. Current
  regional date formatting and translated controls shall remain compatible.
- **AC-UI-STATS-CALENDAR-LABELS-001.5:** Reading or regrouping labels shall preserve APIs, stored values,
  completion eligibility, loading, errors, retries, and existing layout and
  interaction behavior.

## Out of scope

Generic timestamp displays, a timezone preference, new bucket definitions,
backend analytics changes, completion semantics, layout or touch redesign,
and new user-facing copy.

## Implementation plans

- [Statistics calendar labels](../../../plans/stats-calendar-labels/plan.md)
