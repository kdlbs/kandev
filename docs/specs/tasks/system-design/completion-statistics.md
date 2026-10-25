---
status: current
system: tasks
requirements:
  - REQ-TASKS-COMPLETION-STATS-001
---

# Completion Statistics System Design

## Purpose and boundaries

Tasks owns this projection because it owns the committed outcome being counted.
The [task completion design](task-completion.md#entry-and-notification) retains
all transition authority. Analytics reads persisted `tasks.state`; it does not
reevaluate `IsTerminalStep`, `complete_task_on_enter`, names, or current ordering.

[Interactive read availability](../../platform/system-design/interactive-read-availability.md)
owns capacity and historical optimization compatibility, not completion meaning.
This local conformance correction needs no new system directory or ADR.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-COMPLETION-STATS-001` | Completion selection; Compatibility boundaries; Native Statistics; Verification |

## Completion selection

In `apps/backend/internal/analytics/repository/sqlite/stats.go`, replace the
final-position branch of the three completion predicates with a comparison to
the committed `COMPLETED` task state:

- `GetGlobalStats`: `t.archived_at IS NOT NULL OR t.state = 'COMPLETED'`.
- `buildRepositoryStatsQuery`: the equivalent expression using `scope`.
- `GetCompletedTaskActivity`: the equivalent expression using `t`, retaining
  its separate timestamp-availability condition.

Remove the now-unused `workflow_steps` joins in these queries and the unused
`workflow_step_id` projection in `task_repository_scope` if no remaining
expression consumes it. Preserve independent task/session/turn/message/git
aggregations. No new shared query abstraction is necessary for three simple
predicates. No new workflow-setting join replaces the removed position lookup.

The archived branch remains an OR condition. A task satisfying both branches
still counts once. Empty or missing workflow-step references cannot hide an
otherwise eligible committed completion. Non-completed unarchived task states
cannot gain eligibility through a completed session.

## Compatibility boundaries

Keep current query scopes and clocks intact:

- Global task aggregates filter workspace and `tasks.created_at` through
  `rangeStartPredicate`; session-derived aggregates filter session start time.
- Repository task aggregates use distinct task/repository membership and task
  creation time. Session aggregates retain that task-membership attribution;
  Git aggregates retain session repository identity and commit time. Preserve
  eligible-repository selection and existing behavior for cross-workspace links.
- Keep `is_ephemeral = 0` and `andNotAutomationOriginT` in their current scopes.
  Preserve repository ordering and zero-valued rows for eligible empty repos.
- Completed activity retains `MAX(task_sessions.completed_at)` per task,
  followed by `COALESCE(ts.completed_at, t.archived_at)`. Session time takes
  precedence even when archive time is later. Retain `COUNT(DISTINCT t.id)`,
  dialect date normalization, the recursive dates spine, horizon and ordering.
  Out-of-horizon timestamps and absent timestamps produce no visible bucket.
- Preserve other totals, averages, duration rules, limits, DTOs, authorization,
  analytics admission/release, cancellation and error normalization.

The intended numerical change is limited to former final-position false
positives and committed-state false negatives. No data repair, schema migration,
task/session/provider/metadata write, or completion event occurs during reads.

## Native Statistics

`StatsHandlers` in `internal/analytics/handlers/stats_handlers.go` already passes
the repository results through existing DTOs for global, repository, and
completed-activity routes. `apps/web/app/stats/stats-sections.tsx` derives the
global completion rate and repository ratios from those fields.
`apps/web/app/stats/stats-page-client.tsx` renders completed activity through
`CompletedTasksChart` in `stats-charts.tsx`. Both desktop and phone consume
these shared values.

Keep those handlers and frontend surfaces unchanged. No rendered structure,
interaction, copy, localization, or responsive behavior changes; the mobile
parity review therefore requires no new mobile composition or browser suite for
this query correction. Existing empty/loading/error/retry behavior remains in
place. Repository integration coverage is the executable projection boundary.

## Verification

Keep permanent coverage in `internal/analytics/repository/sqlite`, following
`newRealSchemaTestDB` in `commit_capture_activation_integration_test.go` for
real SQLite writers and consumers. Initialize task storage, then workflow
storage, then analytics as in `internal/backendapp/storage.go`. A local fixture
can adapt the existing helper pattern to respect that order without changing
unrelated activation tests. Finish constructors before creating a dedicated
workflow and its steps. Bootstrap can populate an empty workflow; inspect
repository `ListStepsByWorkflow` and service `GetNextStepByPosition` rather
than assuming a numeric position is final.

Use real task/workflow repository writers and persisted task readback, not a
parallel miniature schema or a copied ROOT probe. Establish actual unchecked
final, checked final, checked non-final, and reordered-step observations.
Create explicit committed state fixtures through task writers; assert state
stays unchanged after workflow edits and analytics reads. The supplied ROOT
receipt separately proves real `TaskService.MoveTask` behavior; do not add a
task-service test solely to duplicate that evidence or run its broad suite.

New tests must execute all three actual analytics consumers with a small numeric
oracle and positive controls: `COMPLETED`, `TODO`, `IN_PROGRESS`, `FAILED`,
`CANCELLED`, archived compatibility, exclusion/attribution/range mixtures, and
timestamp precedence/absence/horizon behavior. In particular,
`GetCompletedTaskActivity` requires its own failing-before/passing-after query
evidence; the receipt only inspected its old predicate.

Correct old final-position fixtures to persist their intended `COMPLETED`
state while retaining meaningful numeric assertions, notably
`TestGetCompletedTaskActivity_ExcludesEphemeralTasks`. Keep timestamp-less
completed controls distinct from non-completed controls.

Run every new focused case, positive control, and generated SQL dialect fixture
before complete owning-package race validation. Shared SQL retains existing
SQLite/pgx dialect helpers and rebinding. Without separate authorization for a
disposable PostgreSQL database, unset `KANDEV_TEST_POSTGRES_DSN`, run SQLite and
generated pgx SQL checks of the existing `buildRepositoryStatsQuery` builder,
and report live PostgreSQL tests as skipped. Do not extract new production query
builders solely to support SQL-text assertions.

## Related decisions

- [Separate task completion from conversation availability](../../../decisions/2026-09-09-task-completion-conversation-availability.md)

## Implementation plans

- [Completion statistics](../../../plans/completion-statistics/plan.md)
