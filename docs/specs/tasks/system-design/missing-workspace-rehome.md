---
status: draft
system: tasks
requirements:
  - REQ-TASKS-MISSING-WORKSPACE-REHOME-001
  - REQ-TASKS-MISSING-WORKSPACE-REHOME-002
created: 2026-09-03
owners:
  - kandev
---

# Missing Workspace Rehome System Design

## Purpose and boundaries

The task system owns the canonical task-to-environment binding, session launch
intent, workflow position, durable artifacts, and user-visible launch failure.
The lifecycle manager supplies a typed classification for a physically missing
workspace. Executor implementations materialize the replacement but do not
decide whether repository work can be abandoned.

This design extends, rather than weakens, the fail-closed attach contract in
[Additional Session Workspace Reuse](additional-session-workspace-reuse.md).

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-MISSING-WORKSPACE-REHOME-001` | Recovery classification, rehome operation and launch flow, failure projection |
| `REQ-TASKS-MISSING-WORKSPACE-REHOME-002` | Loss assessment and authorization, responsive surface |

## Recovery classification

`models.ErrWorkspaceReuseUnsafe` remains the broad attach-only failure. The
lifecycle boundary adds a typed reason whose stable code is
`missing_task_workspace`; SSH emits it only when its canonical remote task
directory probe fails because the directory is absent. Other origin mismatch,
inventory, branch, permission, or transport failures remain reuse-unsafe but
are not automatically rehomeable.

Step-entry auto-start, `StartCreatedSession`, and `ResumeTaskSessionWithOptions`
route launch through one task-owned recovery coordinator. The coordinator sees
the typed cause after the ordinary launch attempt, without parsing error text.

## Loss assessment and authorization

The latest environment-owned Git snapshot is the recovery evidence. A loss
assessment records one of:

- `recoverable`: the tree was clean and every local commit was known reachable
  from a configured remote ref at the assessment generation;
- `unique_work`: tracked, untracked, or local-only commit evidence existed;
- `unknown`: no current assessment exists or the evidence is incomplete.

Workflow completion persists an assessment before the transition is eligible
to launch its next step. Server-side conversation, task documents, and plan
artifacts do not depend on the executor directory. A `recoverable` assessment
therefore permits automatic rehome after planning/specification completion.
`unique_work` and `unknown` block automatic rehome.

The existing task launch-error projection gains category
`workspace_rehome_required`. Its bounded detail carries the original cause,
recovery cause when present, loss state, and an opaque error stamp. When loss
is not proven recoverable it offers `rehome_fresh`. The action uses
the existing task launch recovery authorization boundary and rejects foreign
task/session/binding identities or stale stamps.

## Rehome operation and persistence

The repository exposes a single transactional claim operation. It takes the
task cleanup barrier, applies the loss gate, and compare-and-swaps the matching
ready or stopped environment to creating. The winner assigns the current
session as materialization owner, clears stale workspace/provider handles, and
clears old repository inventory. Task, environment, session, workflow step,
repository configuration, intended profiles, conversation, and plan records
retain their identities. No schema migration is required.

## Launch flow and bounded retry

The database state transition is the concurrency boundary. A caller that loses
the claim does not allocate or launch another workspace or session. Backend
restart observes the durable creating or terminal state without allocating a
duplicate.

The original launch is attempt zero. Only a successful rehome claim can consume
attempt one. Attempt one always disables automatic rehome, even if it returns
another missing-workspace error. A normal successful launch creates no
operation. Successful replacement materialization marks the new environment
ready before prompt admission and then completes the operation.

## Failure projection

If replacement preparation or prompt admission fails, normal launch-failure
bookkeeping marks the existing session `FAILED`, marks the environment failed
when appropriate, and persists both safe errors in the launch-error projection.
No failure path leaves an unexplained `CREATED` session.

Success clears the matching launch error by stamp only after the environment is
ready and the intended prompt or resume launch is admitted. Stale failures from
the old execution cannot terminalize the replacement.

## Responsive surface

The existing task Chat error card renders the new category. Desktop keeps its
inline recovery action. Mobile uses the same card and existing inset recovery
drawer/picker pattern, with one vertical scroll owner, safe-area clearance, and
44px touch targets. Shared projection and action handlers own authorization and
state; viewport-specific code owns composition only.

## Observability

Structured logs include task ID, source and replacement environment/session
IDs, operation ID, loss state, attempt ordinal, and terminal outcome, without
workspace paths. Counters cover claims, joined claims, automatic and authorized
rehomes, successes, blocked-loss outcomes, and failures by typed stage.

## Related decisions

- [ADR-2026-09-03-task-environment-rehome-generations](../../../decisions/2026-09-03-task-environment-rehome-generations.md)
- [ADR-2026-08-08-task-owned-worktree-lifetime](../../../decisions/2026-08-08-task-owned-worktree-lifetime.md)
