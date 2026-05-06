---
status: shipped
created: 2026-05-03
owner: cfl
---

# Office task session lifecycle

## Why

Today every office wakeup creates a fresh `task_sessions` row. The
ACP conversation can resume across launches via a stored
`acp_session_id`, but the DB row is brand new each time. After a few
wakeups on a task you end up with N session rows, each separately
COMPLETED, each with its own duration and command count, and a vestigial
`is_primary` flag pointing at the *first* one (because `PrepareSession`
only marks the first row primary). Advanced-mode resume then has to
guess which row "is" the conversation, the inline timeline shows N
entries for what is conceptually one ongoing thread, and the topbar
spinner depends on which row's state is currently RUNNING.

This is wrong on two axes:

1. **Sessions should be persistent identities, not per-launch records.**
   An ACP session = a conversation thread = an agent's working memory
   on a task. Spawning a new agent process is what we do per turn, but
   the conversation should outlive any single process.

2. **Sessions are per-(task, agent), not per-task.** A task can have
   multiple agents working on it — the assignee, listed reviewers,
   listed approvers, agents pulled in via @mentions. Each maintains
   their own context. CEO's notes shouldn't appear in the QA reviewer's
   working buffer, and when the QA reviewer is woken later they should
   pick up exactly where their previous turn left off — not start over,
   and not see the CEO's intermediate work.

The shipped specs already prescribe this direction:

- `office-overview/spec.md:97` — "one-shot heartbeat runs from the
  scheduler + interactive sessions from advanced mode"
- `office-scheduler/spec.md:80` — "One-shot session model — Each
  wakeup produces a single agent session that runs to completion and
  exits"
- `office-advanced-mode/spec.md:11` (status: shipped) — "the agent
  runs, completes, and the execution is torn down"

What we have is the per-launch part right and the per-(task, agent)
identity wrong. This spec fixes both.

## What

### A. Schema — sessions become per-(task, agent_instance)

- New column on `task_sessions`: `agent_instance_id TEXT` (nullable).
- Partial unique index:
  ```sql
  CREATE UNIQUE INDEX uniq_office_task_session
    ON task_sessions(task_id, agent_instance_id)
    WHERE agent_instance_id IS NOT NULL;
  ```
  Office sessions get one row per (task, agent). Kanban / quick-chat
  sessions leave the column NULL; the unique index doesn't apply. They
  keep their per-launch + `is_primary` model intact.

`is_primary` stays as-is. Office code never reads it; kanban
advanced-mode resume keeps using it.

### B. State machine — RUNNING ↔ IDLE for office sessions

A new session state `IDLE`:

- An office session is `IDLE` when no agent process is running and the
  full executor backend is torn down — but the conversation is
  preserved (`acp_session_id` stored on the row, ready for `session/load`
  on the next wakeup).
- Transition graph: `CREATED → STARTING → RUNNING → IDLE → RUNNING →
  IDLE → ...` repeating across an arbitrary number of turns. The same
  IDLE↔RUNNING cycle applies uniformly to **every** agent role on a
  task: assignee, reviewers, approvers, @-mentioned agents.
- A session goes `COMPLETED` (terminal) only when the agent leaves the
  participants list — reassignment for the assignee, removal from the
  reviewers/approvers picker for the others. Until that moment, the
  session stays IDLE between turns.
- The current `WAITING_FOR_INPUT` state stays for kanban/quick-chat
  ("agent is up, parked between turns"). Office sessions skip
  WAITING_FOR_INPUT entirely — their pause state is IDLE, with the
  agent process *gone* and the executor backend *torn down*.

This is the fire-and-forget model:

> Wakeup → spin everything up (executor + agent + ACP) → run one turn
> → tear everything down → IDLE.

The next wakeup spins it back up. There is no warm executor between
turns, no idle-timeout cleanup needed, no resource accumulation.

`updateTaskSessionState`'s terminal-state guard relaxes for office:
COMPLETED/FAILED/CANCELLED stay terminal, but IDLE → RUNNING is
allowed.

### C. Wakeup path — `EnsureSessionForAgent`

The office scheduler's wakeup processing replaces `PrepareSession` on
the launch path with `EnsureSessionForAgent(task, agent_instance)`:

1. `SELECT * FROM task_sessions WHERE task_id = ? AND agent_instance_id = ?`
   - If found and state is `IDLE`: flip state to RUNNING, return.
   - If found and state is RUNNING/STARTING: return as-is (idempotent).
   - If found and state is terminal (COMPLETED / FAILED / CANCELLED):
     create a new row (existing pair was retired, e.g. agent was
     removed and re-added).
   - If not found: insert with state `CREATED`, agent_instance_id set.
2. Hand off to the existing `LaunchPreparedSession` path, which spawns
   the agent process and runs the ACP handshake.
3. ACP init: if the session has a stored `acp_session_id`, call
   `session/load` with it. On error (session expired, agent CLI version
   mismatch), fall back to `session/new` and overwrite the stored token.

### D. Turn complete — full teardown for office

Today `handleCompleteStreamEvent` parks office sessions in
`WAITING_FOR_INPUT` and leaves the agent process + executor backend
warm. After this spec, every office turn-complete fires the
fire-and-forget shutdown sequence:

1. State flip on the session row: `RUNNING → IDLE`. State transition
   first so the workflow handler's terminal-state guard short-circuits
   (mirrors the existing `completeAndStopSession` pattern — without
   this ordering, `handleAgentCompleted` re-runs workflow on the new
   step and ping-pongs the task).
2. `agentManager.StopBySessionID(sessionID, false)` — tears down the
   agent subprocess + the executor backend (Docker container,
   standalone process, sprites instance, etc.) + agentctl connection.
   The session's `acp_session_id` stays on the row for the next
   `session/load`.
3. Lifecycle manager cleans up: in-memory execution entry removed,
   `AgentStopped` event published, resources released.

The result: nothing is "warm" between turns. The next wakeup recreates
the executor from scratch and reloads the conversation.

For kanban / quick-chat sessions: unchanged. They keep
WAITING_FOR_INPUT semantics + warm executor between turns.

### E. Participation removal — terminate that agent's session

A session goes `COMPLETED` (terminal) only when its agent stops being
a participant on the task. Three triggers:

1. **Reassignment.** Task assignee changes from CEO to Eng Lead. CEO's
   session goes COMPLETED. The reactivity pipeline's existing hard-cancel
   of the active session keeps working
   (`runReactivityForAssigneeChange` in `dashboard/service.go`); under
   this spec it also writes the terminal state.
2. **Reviewer / approver removal** via the picker. Removing QA from the
   reviewers list takes their session to COMPLETED. The participants
   CRUD endpoint (`DELETE /tasks/:id/reviewers/:agentId`) gains a
   side-effect: terminate any matching session.
3. **Agent instance deletion.** Deleting the agent at the workspace
   level cascades — all of its sessions across all tasks go COMPLETED.

Plain decision recording (approve / request-changes) does NOT terminate
the reviewer's session. They might be summoned again — e.g.
request-changes triggers assignee work, which on completion fires
`task_review_requested` again on the same reviewer. Their session goes
RUNNING → IDLE → RUNNING, same row, conversation preserved across
review cycles.

If the same agent is later re-added (e.g. removed and re-listed as a
reviewer), `EnsureSessionForAgent` sees the prior COMPLETED row and
creates a new one — it explicitly does *not* resume terminated rows.
This preserves the historical separation: the second tour-of-duty has
its own conversation thread.

### F. Advanced-mode resume

For office tasks, advanced-mode entry resolves the session via:

1. If we know the current viewer is an agent (via authenticated request
   context), use that agent's session for the task.
2. Otherwise (the singleton human user), use the task's current
   assignee's session.
3. If no session exists yet (e.g. user opens advanced mode before the
   first wakeup runs), create one for the assignee on demand, then
   resume the executor via the existing `tryEnsureExecution` path.

Kanban tasks keep using `is_primary` for resume.

### G. Frontend impact

- `selectLiveSessionForTask` still works — picks the most recent
  RUNNING/STARTING session across all (task, agent) rows. The topbar
  Working spinner drops to zero the moment the agent's turn ends and
  the session goes IDLE.
- `selectActiveSessionsForAgent` — already filters by
  `agent_instance_id` (now populated correctly post-this-spec). Sidebar
  per-agent live badge lights up only while that agent's session is
  RUNNING.
- Inline session timeline entries — one collapsible entry per
  (task, agent) pair, ordered by most-recent activity (`updated_at`).
  A task with assignee + 2 reviewers shows up to 3 entries. Each entry
  collapses when its agent's session is IDLE; expanded by default while
  RUNNING. The "ran N commands" header derives from messages on that
  session, accumulated across launches.
- Properties panel decision chips (reviewers / approvers) keep their
  per-agent decision icon. No change.

### Out of scope

- Cross-task session sharing. CEO's session on TES-1 is independent of
  CEO's session on TES-2; nothing changes here.
- ACP session expiry / GC. We rely on the agent CLI's own session
  retention; if `session/load` fails we cold-start. A "GC stale IDLE
  sessions older than N days" sweep is a future spec.
- Conversation export / import. Sessions stay in SQLite; portability is
  separate work.
- Kanban / quick-chat behaviour. This spec is strictly office-scoped.

## Acceptance

1. **First wakeup on a task creates a session.** `agent_instance_id`
   on the row matches the task's assignee. `acp_session_id` is empty
   until the ACP handshake fills it.
2. **Second wakeup for the same agent reuses the row.** No new
   `task_sessions` row is created. `state` cycles RUNNING → IDLE →
   RUNNING. The agent process gets `session/load` with the stored
   `acp_session_id` and resumes the conversation — visible in the chat
   timeline as continuous history.
3. **A reviewer being woken creates a separate session.** `(TES-1, CEO)`
   and `(TES-1, QA-reviewer)` are distinct rows with distinct
   `acp_session_id`s. CEO's working notes don't appear in the QA
   reviewer's chat embed.
4. **Turn complete drops the topbar spinner immediately.** The session
   transitions to IDLE, the agent process exits, the spinner disappears
   without a refresh.
5. **Reviewer's decision does NOT terminate their session.** Reviewer
   approves → state goes RUNNING → IDLE (not COMPLETED). The row keeps
   its `acp_session_id`. If the assignee later requests another review
   cycle (request-changes loop), the reviewer's next wakeup resumes
   the same conversation.
6. **Reassignment terminates the prior assignee's session.** Prev
   assignee's row goes COMPLETED — they're not coming back to this
   task unless explicitly re-added. The reactivity pipeline's existing
   hard-cancel of the active session keeps working.
7. **Removing a reviewer/approver via the picker terminates their
   session.** The participants DELETE endpoint flips that agent's
   session to COMPLETED.
8. **Advanced-mode entry on an office task resumes the assignee's
   session.** No new row is created. The executor re-launches via
   `tryEnsureExecution`, `session/load` restores the conversation, and
   the agent's chat is interactive.
9. **Full teardown is verifiable.** After turn complete, the executor
   backend (container/process), agent subprocess, and agentctl
   connection are gone — `lifecycle.Manager` reports zero in-memory
   executions for the (task, agent) pair until the next wakeup.
10. **`session/load` failure falls back to `session/new`.** The stored
    `acp_session_id` is overwritten and the conversation is treated as
    fresh — but the row identity (and history at the kandev level)
    persists.
11. **Kanban / quick-chat tasks behave exactly as before.** No
    regressions on per-launch sessions, `is_primary`, WAITING_FOR_INPUT,
    warm executors.
