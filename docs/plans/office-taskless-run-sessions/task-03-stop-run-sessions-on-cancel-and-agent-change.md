---
id: "03-stop-run-sessions-on-cancel-and-agent-change"
title: "Stop live run sessions on run cancel and agent disable/removal"
status: withdrawn
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-TASKLESS-001
acceptance_criteria: []
system_design:
  - ../../specs/office/system-design/taskless-run-sessions.md
---

# Task 03: Stop live run sessions on run cancel and agent disable/removal

> **WITHDRAWN — 2026-09-19.** This work order is not to be built. `AC-OFFICE-TASKLESS-001.5`
> and `.6` were cut from the requirement after five rounds of spec review, and this order
> existed only to satisfy them. It gave run cancellation, agent disable and agent removal the run-session stop seam `AC-OFFICE-TASKLESS-001.5` required. That criterion no longer exists, and the three controls not reaching run sessions is now a recorded, accepted gap.
>
> The file is kept, unedited below this banner, because a follow-up that revives the
> deferred flows should start from it rather than re-derive it. The deferral, the accepted
> gaps and the two problems a follow-up must resolve first are recorded under
> [Deferred: stop controls and restart recovery](../../specs/office/requirements/taskless-run-sessions.md#deferred-stop-controls-and-restart-recovery).
> References below to `taskless-run-recovery.md` point at a design file retired in the same
> change; its content is summarized in that section.

## Summary

`AC-OFFICE-TASKLESS-001.5` names four controls that must prevent new taskless
launches and stop live taskless executions: workspace pause, explicit run
cancellation, agent disable/removal, and workspace removal. Two of them do.

- Workspace pause reaches run sessions through
  `office/pause/sweep.go:cancelRunSessions`, which lists live sessions with
  `ListLiveRunSessionsForWorkspace`, calls `RequestRunSessionCancellation` and
  stops by execution ID.
- Workspace deletion does the same in
  `office/service/workspace_deletion.go`.
- **Explicit run cancellation** is a status write on `runs`
  (`CancelRunsWhere` and its callers). Nothing consults
  `office_run_sessions`, so the process keeps running after the run is
  cancelled.
- **Agent disable** (`office/agents/service.go:UpdateAgentStatus`) writes the
  status row only.
- **Agent removal** (`office/agents/service.go:DeleteAgentInstance`) calls
  `SessionTerminator.TerminateAllForAgent`, which is
  `orchestrator/office_session_terminator.go`:
  `ListNonTerminalSessionsByAgentInstance` plus `updateTaskSessionState` keyed
  by `sess.TaskID`. That is the **task**-session store. A run-owned execution is
  never listed and never stopped.

This is production work, not coverage. The criterion is unchanged; the code owes
it the seam.

## In scope

Give explicit run cancellation, agent disable and agent removal the same
run-session stop seam workspace pause already has:

- Persist cancellation intent on the affected live run sessions
  (`RequestRunSessionCancellation`) **before** asking the runtime to stop, so a
  launch racing the control is rejected at the registration admission gate
  rather than starting behind it.
- Stop each live execution by its recorded `execution_id`.
- Count partial failures. A stop that fails leaves the session live with its
  execution ID intact so the next sweep re-lists it, and is **not** recorded as
  terminal — the rule the system design states under cancellation and recovery.
  Report the failure alongside the executions that stopped successfully.
- Reuse the existing sweep helper rather than writing a third copy of the
  list-intent-stop-count loop. If the current helper is too workspace-shaped to
  reuse, extract the inner loop and keep one implementation.

Assert the **retryable** half of `AC-OFFICE-TASKLESS-001.5`, which nothing
covers today. `TestPauseStopsMixedRunSessionInventory` runs a single sweep and
asserts counts, so an implementation that reports a partial failure once and
then loses the session passes it. Add, for each of the three controls and for
workspace pause:

- after a stop that fails, the session is still in a live state and still
  carries its `execution_id` — assert the row, not only the returned count;
- a **second** sweep over the same workspace re-lists that session and attempts
  the stop again. This is the design's own definition of retryable, and one
  sweep cannot demonstrate it.

Selection differs per control and must be exact:

- Run cancellation: the live sessions of that run (`ListRunSessions`, filtered
  to non-terminal, or a targeted query).
- Agent disable/removal: the live sessions whose `agent_profile_id` is that
  agent, across the workspace.

## Out of scope

- Any change to `AC-OFFICE-TASKLESS-001.5` itself. The criterion is correct.
- Task-session termination behavior. `TerminateAllForAgent` keeps doing what it
  does for task sessions; this adds a run-owned path beside it, and does not
  reroute task sessions through Office.
- Workspace pause and workspace deletion, which already satisfy the criterion.
- Retention of terminal run-session rows (named out of scope in the
  requirement).

## Tests

- Run cancel with a live run session: intent stamped, runtime stop called with
  the recorded execution ID, session not left live-and-unstopped.
- Agent disable and agent removal, each with one live run session and one live
  **task** session for the same agent: both are stopped, and the task path keeps
  its existing behavior.
- Partial failure: two live run sessions, one stop fails. Assert the failure is
  counted and surfaced, the failed session is still live with its execution ID,
  and the successful one is terminal. This is the case the criterion calls out
  by name ("including when another execution stopped successfully").
- An agent with no live run session is a no-op, not an error.

## Validation

```text
cd apps/backend
go test ./internal/office/... -run 'Cancel|AgentRemoval|AgentDisable|RunSession' -count=1
go test ./internal/orchestrator/ -count=1
make lint
```

## Reference

- `apps/backend/internal/office/pause/sweep.go` — the seam to reuse
- `apps/backend/internal/office/repository/sqlite/run_sessions.go` —
  `RequestRunSessionCancellation`, `ListLiveRunSessionsForWorkspace`,
  `ListRunSessions`
- `apps/backend/internal/orchestrator/office_session_terminator.go` —
  the task-only cascade this work order sits beside
- `apps/backend/internal/office/service/workspace_deletion.go` — the other
  control that already does it

## Risks

- **Stopping too much.** Agent-scoped selection must not reach another agent's
  sessions in the same workspace, and run-scoped selection must not reach
  another attempt of another run. Assert the negative in each test.
- **Ordering.** Intent before stop is not stylistic: the reverse order leaves a
  window where a preparing attempt registers behind the stop.
