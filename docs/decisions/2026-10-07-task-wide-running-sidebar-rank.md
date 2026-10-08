# ADR-2026-10-07-task-wide-running-sidebar-rank: Task-wide running rank in the sidebar

**Status:** accepted
**Date:** 2026-10-07
**Area:** frontend, backend, protocol

## Context

Running first currently checks only the primary session. Task row activity
already aggregates sessions. A secondary session can therefore show activity
while its task ranks beside idle tasks. Color then puts an idle orange task
above it. The live Projects task reproduces this case with no primary session.

Sidebar ranking must agree between covered local data and globally paged server
queries. Foreground activity does not represent every `RUNNING` session.
It also represents settled background work that does not meet this predicate.

## Decision

Running rank uses an existential predicate over all sessions of one task:
at least one session has strict state `RUNNING`. Included descendants contribute
to ancestor rank through the existing tree aggregation.

Tasks publishes optional `has_running_session` in its bounded summary.
True and false require authoritative session observations. Absence represents
legacy or incomplete evidence. The field has revision-owned semantic meaning.
UI consumes the field without selecting or changing the primary session.

Legacy server ranking uses a task-scoped session `EXISTS` in the same query
snapshot. Covered local ranking requires the new boolean or uses the server.
Requested-task reconciliation repairs old summary rows through the existing CAS
path. No startup-wide backfill or new session subscription is required.

## Consequences

Existing Running rules now include secondary sessions without a settings rewrite.
The change adds one scalar to summary transport and persistence.
It preserves primary state for row presentation and Status sorting.
Live publication must recompute the scalar on start, stop, and removal.
Upgrade tests must cover old rows before a new runtime event arrives.

Pending-input and background icons retain their presentation rules.
A spinner does not become a runtime source. Running rank does not elect a
primary session or repair session ownership.

## Alternatives Considered

- Keep primary-only rank: preserves the previous rule but retains the reported mismatch.
- Elect a primary on read: changes session ownership as a side effect of sorting.
- Use foreground activity: misses runtime states without activity and includes settled background work.
- Send all sessions to every row: increases transport and breaks the bounded summary boundary.
- Query sessions only on the server: fixes paging but leaves complete local ranking without equivalent evidence.
- Add a startup backfill: increases startup work and does not solve ongoing live publication.

## References

- [Requirements](../specs/ui/requirements/sidebar-running-first-activity-sort.md)
- [System design](../specs/ui/system-design/sidebar-running-first-activity-sort.md#task-wide-running-projection)
- [Fix package](../plans/sidebar-task-wide-running-rank/plan.md)
- [Existing sort-chain decision](2026-10-06-composable-sidebar-sort-rules.md)
