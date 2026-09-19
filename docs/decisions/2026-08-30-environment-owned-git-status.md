# ADR-2026-08-30-environment-owned-git-status: Persist Current Git Status with the Task Environment

**Status:** accepted
**Date:** 2026-08-31
**Area:** backend, frontend, protocol

## Context

Multiple task sessions can share one `TaskEnvironment` and one physical
workspace. Shared, inherited, and passthrough sessions use this model.

Git snapshots used `session_id` for persistence, ranking, and delivery. The
frontend stored the results under one environment key.

This mismatch created competing authorities. An old completion from a sibling
session can outrank a newer live observation of the same workspace.

Delivery also used the session ID as the status identity. A late sibling
message can replace the correct environment state after hydration.

## Decision

Persist current Git status with `task_environment_id` as the scoping key. Keep
`session_id` only as nullable capture provenance.

Select one current observation for each task environment and repository pair.
Observation time has priority over snapshot type.

The newest observation defines the current state. An older detailed snapshot
cannot supply files after a newer sparse observation exists.

An agent-completion capture removes earlier live and completion observations
for the same environment and repository. A live upsert keeps one live row for
that pair.

Every status payload carries `task_environment_id` and repository identity.
The frontend uses these delivered fields as its storage key.

If any execution in the environment is live, agentctl is authoritative. If no
execution is live, the latest persisted environment observation is
authoritative.

## Consequences

- Session deletion does not remove the current status of a retained
  environment.
- Sibling sessions cannot create independent current-status authorities.
- A newer clean or sparse observation clears stale file details.
- The backend can retain a session ID for audit and historical session reads.
- The migration must rebuild the table because the session foreign key changes
  from cascade removal to nullable provenance.
- SQLite and Postgres need transactional cutover and replay coverage.
- Direct analytics and delivery queries must distinguish environment scope from
  session provenance.
- The WebSocket route can remain session-based. The status identity in the
  payload is environment-based.

## Alternatives Considered

1. **Keep session-scoped persistence and select across sibling sessions.** This
   keeps multiple authorities for one workspace. It also fails after session
   deletion removes the selected row.
2. **Use frontend timestamp ordering only.** This can hide message reordering.
   It cannot correct backend ranking or session-cascade removal.
3. **Prefer all detailed snapshots over sparse snapshots.** This restores old
   file lists after a newer live observation. The detail is not current.
4. **Store current status on `task_environments`.** This removes snapshot
   history and capture provenance. The existing snapshot table can represent
   the environment boundary without this second model.
5. **Remove sibling subscriptions.** This changes presentation behavior but
   leaves persistence and summary reads session-scoped.
