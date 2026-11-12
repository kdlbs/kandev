# ADR-2026-11-05-task-related-to-storage: Store the related-to link in a new `task_relations` table

**Status:** accepted
**Date:** 2026-11-05
**Area:** backend

## Context

[Task-create relationship establishment](../specs/tasks/requirements/task-create-relationships.md)
adds a non-blocking, symmetric "related to" link between two tasks, alongside
the existing directed, blocking `task_blockers` edge
(`internal/office/repository/sqlite/base.go:766-771`,
`internal/office/models/models.go:634-637`). The
[system design](../specs/tasks/system-design/task-create-relationships.md) and
the [task relationship graph](../specs/tasks/system-design/task-relationship-graph.md)
both need a stable way to read and distinguish the two edge kinds.

`task_blockers` is directed (`task_id`, `blocker_task_id`) and gates auto-start
and launch through `BuildDependencyViews`
(`internal/task/service/service_dependencies.go`). A related-to link has
neither property: it does not block, has no predecessor/dependent direction,
and never participates in dependency resolution.

## Decision

- Add a new `task_relations` table with a normalized, order-independent
  primary key (`task_id_low`, `task_id_high`, with `CHECK (task_id_low <
  task_id_high)`), rather than a nullable `kind`/`blocking` flag column on
  `task_blockers`.
- Give it its own repository interface (`RelationRepository`) and service
  wiring, separate from `BlockerRepository`, mirroring the existing pattern
  rather than branching `BlockerRepository` methods on an edge kind.
- Expose both kinds to the frontend and the task-graph read as one `kind`
  field (`dependency` | `related`) on the existing edge response shape,
  keeping one wire contract even though storage is split.

## Consequences

- `task_blockers`' invariants (directed, `CHECK(task_id != blocker_task_id)`,
  AND-gate dependency resolution) are untouched; no existing dependency query,
  cycle check, or launch-gate code needs a "is this a real dependency" filter.
- Cycle detection, which only matters for a directed, blocking edge, never
  needs to consider `task_relations` rows.
- A pair of tasks can in principle hold both a `task_blockers` row and a
  `task_relations` row at the same time (for example, a dependency added later
  through the Edit dialog after a related-to link was established at
  creation). This is accepted: the two tables model independent facts, and the
  create-time exclusion in
  [`task-create-relationships.md`](../specs/tasks/requirements/task-create-relationships.md)
  (AC-TASKS-TASK-CREATE-RELATIONSHIPS-002.5) only prevents redundant selection
  in the create dialog, not a future divergence.
- Two small repository files and one new table to maintain, instead of one
  wider `task_blockers` table with conditional semantics per row.
- Task deletion's cleanup pass gains one more table to clear
  (`RemoveRelationsForTask`), alongside its existing `task_blockers` cleanup.

## Alternatives considered

- **A `blocking BOOLEAN` flag on `task_blockers`:** reuses one table, but
  forces every dependency-resolution query to filter `WHERE blocking = true`
  defensively, and keeps a directed `(task_id, blocker_task_id)` shape for a
  relationship that is not directed, which would allow two rows
  (`a,b`) and `(b,a)` to represent the same related-to link unless a separate
  normalization or uniqueness rule is added on top — at which point the table
  no longer shares `task_blockers`' actual key shape anyway.
- **Encode related-to as a zero-weight dependency:** rejected outright; it
  would make a related-to link visible to auto-start and launch-gate logic
  unless every call site added an exclusion, which is the kind of silent
  coupling REQ-TASKS-TASK-CREATE-RELATIONSHIPS-002.2 explicitly rules out.
