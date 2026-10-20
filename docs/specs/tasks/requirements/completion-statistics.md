---
status: active
system: tasks
created: 2026-10-18
owners:
  - kandev
---

# Completion Statistics Requirements

## Overview

Tasks owns committed completion state and its domain projections. Statistics
consumes that state to report completed work. This projection contract extends
the [task completion requirements](task-completion.md), especially
`AC-TASKS-COMPLETION-001.2`, `.3`, and `.11`; it does not redefine task entry,
reopening, or completion eligibility.

## Requirements

### REQ-TASKS-COMPLETION-STATS-001: Committed completion statistics

**Intent:** Users see completion totals consistent with persisted task outcomes,
including tasks whose current workflow position no longer reflects completion.

#### Acceptance criteria

- **AC-TASKS-COMPLETION-STATS-001.1:** For eligible unarchived tasks, global and
  repository completion totals shall count a task if and only if its committed
  state is `COMPLETED`. `TODO`, `IN_PROGRESS`, `FAILED`, and `CANCELLED` shall not
  count through workflow position, name, or current completion setting.
- **AC-TASKS-COMPLETION-STATS-001.2:** Renaming, reordering, or changing the
  completion setting of a resident task's step shall not change its completion
  contribution unless its committed task state changes. A legitimate committed
  completion outside the current final step, or without a resolvable step,
  shall remain counted.
- **AC-TASKS-COMPLETION-STATS-001.3:** Completed activity shall apply the same
  eligibility as completion totals, counting each task once in its available
  completion-date bucket. A completed session alone shall not qualify an
  unarchived task whose committed task state is not `COMPLETED`.
- **AC-TASKS-COMPLETION-STATS-001.4:** Archived tasks shall retain their existing
  Statistics completion contribution regardless of task state, subject to the
  existing eligibility filters and activity timestamp availability. This is
  compatibility of the Statistics metric; archiving shall not acquire task
  lifecycle authority or declare successful task completion.
- **AC-TASKS-COMPLETION-STATS-001.5:** Completion statistics shall preserve
  ephemeral-task and automation-run exclusions, workspace selection, repository
  membership attribution, soft-deleted repository exclusion, and distinct-task
  counting within each repository. A task linked to multiple repositories shall
  retain its contribution to each eligible repository.
- **AC-TASKS-COMPLETION-STATS-001.6:** Global and repository task totals shall
  retain task-creation range boundaries. Session and commit aggregates shall
  retain their own range clocks. Completed activity shall retain its existing
  horizon, ascending date buckets, zero-filled dates, and latest non-null
  session-completion timestamp with archive-time fallback. Without either
  timestamp, a task shall contribute no activity bucket even when counted in
  completion totals. Task creation/update time shall not become a fallback.
- **AC-TASKS-COMPLETION-STATS-001.7:** Native Statistics shall display the
  corrected completed counts, derived completion rates, repository completion
  ratios, and completed-activity values on desktop and phone through the
  existing surfaces. Other aggregates, ordering, limits, response shapes,
  authorization, and loading/error/retry behavior shall remain compatible.
- **AC-TASKS-COMPLETION-STATS-001.8:** Reading completion statistics shall not
  change task state, workflow position or settings, session state, provider
  data, metadata, or completion history.

## Compatibility

Former false positives from unarchived non-completed tasks in the last column
shall disappear. Former false negatives from committed completions outside that
column shall become counted. Historical values may therefore change on the next
read without rewriting historical records. Timestamp-less completions may still
appear in totals without appearing in activity, as before.

## Out of scope

- Task lifecycle changes, completion-setting migrations, or archive semantics.
- New APIs, metrics, registries, caches, or reporting performance work.
- Correcting the existing 365-day activity horizon for the all-time selection.
- Frontend layout/copy, freshness mechanisms, or additional preservation work.

## Implementation plans

- [Completion statistics](../../../plans/completion-statistics/plan.md)
