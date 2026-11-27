---
status: current
system: tasks
requirements:
  - REQ-TASKS-DAILY-ACTIVITY-001
---

# Daily Activity Statistics System Design

## Purpose and boundaries

Tasks owns the activity projection of its persisted sessions and messages.
[Interactive read availability](../../platform/system-design/interactive-read-availability.md)
continues to own analytics admission and read failure behavior; the
[calendar label design](../../ui/system-design/stats-calendar-labels.md) owns
presentation of the resulting dates. This correction introduces no new
persistence or cross-system ownership boundary.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-DAILY-ACTIVITY-001` | Attribution and compatibility; Delivery and verification |

## Attribution and compatibility

`GetDailyActivity` in `apps/backend/internal/analytics/repository/sqlite/stats.go`
currently derives `message_days` from in-range `turn_scope` and requires a
matching session and day. A persisted message after UTC midnight is omitted
until a turn starts in that session on that day; a turn predating the horizon
also cannot seed that day.

Keep the recursive date spine and the independent `turn_activity` aggregate.
Build `message_activity` from `task_session_messages` joined through its
session to its task and the date spine using the dialect-normalized message
`created_at` date. Apply the same selected workspace, `is_ephemeral = 0`, and
`andNotAutomationOriginT` filters as the turn scope. Count each message row
once independently of turn existence, turn date, author, or message type.
Do not join raw turns to raw messages: their fanout multiplies aggregates.
Preserve the turn-start and distinct-task aggregates exactly, including zero
turns and tasks on a message-only day. Preserve current UTC date helpers,
rebinding, date horizon and ordering on SQLite and PostgreSQL.

The read-only query retains its existing analytics operation admission,
cancellation, error normalization, and reader connection. No migration, write,
or change to other statistics is required. Existing workspace authorization
and error response behavior remain at the handler boundary.

## Delivery and verification

`StatsHandlers` registers the existing daily-activity route and serializes its
unchanged DTO. `ActivityHeatmap` in `apps/web/app/stats/stats-charts.tsx`
computes intensity from turn plus message count and displays the message count
in its tooltip. Desktop and phone consume this same data; no frontend code,
layout, interaction, localization, or calendar-label change is needed. This
query-only correction does not change responsive or touch behavior, so mobile
parity design and a new browser suite are not required. The existing desktop calendar-label test covers the unchanged heatmap tooltip;
the adjacent phone calendar-label test covers the completed chart, not this
heatmap. A real registered HTTP route with persisted messages proves the
corrected data path; the shared rendering code supports the phone outcome but
does not itself constitute a live phone rendering proof.

An independent real-schema SQLite regression uses task repository writers and
the registered handler, with numeric before/after midnight and older-than-range
cases and controls. The existing aggregate parity fixture runs SQLite and,
when `KANDEV_TEST_POSTGRES_DSN` is already available, isolated PostgreSQL with
a non-UTC session timezone. Update only the two legacy omission expectations;
retain its other numeric assertions and the bounded high-fanout test. A skipped
PostgreSQL run is not live dialect evidence and must be reported as such.

## Implementation plans

- [Overnight activity statistics](../../../plans/overnight-activity-statistics/plan.md)
