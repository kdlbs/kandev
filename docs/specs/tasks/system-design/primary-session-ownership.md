---
status: draft
system: tasks
requirements:
  - REQ-TASKS-PRIMARY-SESSION-OWNERSHIP-001
---

# Primary Session Ownership System Design

## Purpose and boundaries

The task repository stores `task_sessions.is_primary`, and the orchestrator
replaces a primary session that stops or is deleted. This design keeps the
flag single-valued and makes both automatic promotions use the same choice.
[Workflow session lifecycle](workflow-profile-session-lifecycle.md) owns
recipient selection for workflow entries and parking. The message queue keeps
its session-bound targets.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-PRIMARY-SESSION-OWNERSHIP-001` | [Data and contracts](#data-and-contracts), [Control flow](#control-flow) |

## Components and responsibilities

- `SetSessionPrimary`, `SetSessionPrimaryIfNonterminal`, and the workflow route
  variant in `task/repository/sqlite/session.go` demote every session of the
  task and promote one inside one transaction. They are the only writers that
  can set `is_primary` after creation; the full-row session update only
  releases it.
- `updateTaskSessionWithSnapshotGuard` backs `UpdateTaskSession` and its state
  and revision guarded variants. It writes the session's other columns.
- `bestPrimarySessionCandidate` in `orchestrator/primary_session_candidate.go`
  chooses the session for an automatic promotion.
- `maybePromotePrimary` runs when a session reaches a terminal state, and
  `promoteNextPrimaryAfterRemoval` runs after a session is deleted.

## Data and contracts

No unique index backs `is_primary`, so the promotion transaction is the only
guard. The full-row `UPDATE` also wrote `is_primary` from the caller's copy. A
copy read while a session was primary therefore made it primary again after a
later promotion, without an error, and `GetPrimarySessionByTaskID` could return
either row.

The full-row `UPDATE` never promotes `is_primary`: it keeps the stored flag,
and only a snapshot-guarded write whose session copy says non-primary releases
the owned flag. Promotion and removal still go through `SetSessionPrimary`.
Creation keeps its own `INSERT` (`001.1`). Test fixtures that set the flag
through the full-row write use `SetSessionPrimary`.

`bestPrimarySessionCandidate(ctx, taskID, sessions, excludeSessionID, activeOnly)`
skips the excluded and terminal sessions and, with `activeOnly`, sessions that
are not active. It ranks the rest:

1. A session whose `agent_profile_id` equals the effective profile of the
   task's current step: the step's own profile, a task override, or the
   workflow default. A task without a step, an unreadable step, or no
   resolvable profile treats every session as matching.
2. Any other session.
3. A session with a valid `workflow_parking` marker, last.

Inside a rank it keeps the existing order: `RUNNING`, then other active states,
then idle (`001.2`). It returns an empty ID only when no session is eligible,
so a task with only unmatched sessions still gets a primary (`001.3`).

## Control flow

`maybePromotePrimary` passes `activeOnly=true`, which keeps its rule that only
an active session can inherit the role. `promoteNextPrimaryAfterRemoval`
passes `false`; it now excludes terminal sessions, which it accepted before.
Both promote through `SetPrimarySession`, which neither moves queued messages
nor changes an explicit session selection (`001.4`).

## Failure and recovery

A failed task or step read during selection treats every session as matching,
so a promotion still happens. A failed promotion write logs a warning and
leaves the previous state, as before.

## Persistence

No schema or migration change. Existing rows with two primaries are not
repaired by this change; the next promotion of the task demotes both.

## Security

No permission or trust boundary changes.

## Observability

The existing `auto-promoted primary session` log keeps the old and new IDs.
