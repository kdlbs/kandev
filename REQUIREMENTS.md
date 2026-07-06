# Requirements

> Source: GitHub issue [#1597](https://github.com/kdlbs/kandev/issues/1597)
> — "Executor-row desync persists on v0.73.0". Follow-up to #1585
> (partially fixed by #1587); supersedes the closed #1594.
>
> Note: the issue's framing is partly out of date. The `ready` status and its
> promotion path already shipped in v0.73.0 (via #1587), so re-landing #1594 is
> not the fix. The real defect is architectural — see the problem statement.

## Problem statement §req:problem-statement

An operator runs Kandev **headless as a long-lived service** (`kandev --headless`
under systemd, Homebrew linux-x64), driving resumable agent-mode sessions over
many hours and across restarts. The executor is local/standalone (`local_pc`):
process tree `kandev → kandev __backend → agentctl → claude-agent-acp → claude`.

**The symptom that hurts, in the operator's words:** *pause an agent, and you
can't resume it.* When the operator stops/pauses a running agent turn and then
tries to continue by sending a new message, the message does not get through —
the session still looks "busy"/running, and the only way out is to **cancel and
restart the whole Kandev service**. (This was reproduced live during this very
investigation: a session that appeared to be "running" refused a new message
until the service was restarted.) For a headless server this is severe — a
routine pause silently wedges a session, and recovery means bouncing the process
that every other session depends on.

**The mechanism underneath:** Kandev keeps one row per session in the
`executors_running` table, meant to mirror the live processes. It doesn't. On
v0.73.0, after normal use, the rows drift from reality and the bad rows pile up:

- **Every row reports `pid = 0`** for local runtimes — live `agentctl`/`claude`
  processes exist, but no row references them, so nothing can tell a live
  session from a dead one.
- **Rows sit at `starting`/`prepared`** with `pid = 0` and `last_seen_at = NULL`,
  and the accumulating backlog grows across restarts (~50 → 77).
- Because the table can't be trusted, the machinery that reads it — cancel
  cleanup, startup reconciliation, and the wedged-session self-heal — makes
  wrong decisions, which is how a pause can leave a session unresumable.

**Why this started when it did (the architectural root cause):** ADR 0025
("Runtime Cleanup Uses `executors_running`", accepted 2026-06-22, shipped in
#1465 / v0.66.0) **promoted `executors_running` to the authoritative durable
runtime inventory** — the source of truth for "which processes are alive and
need stopping," used by task cleanup *and* startup reconciliation. Before that,
those decisions were driven off `task_sessions`. The rows were *always* somewhat
inaccurate for local runtimes (no real pid, no heartbeat), but **nothing
load-bearing read those columns**, so it didn't matter. v0.66 made the table
load-bearing **without giving its liveness columns a trustworthy producer**, so
the pre-existing drift suddenly had consequences — and it compounds every
restart.

**Why the table drifts by construction:** it is written *only* at lifecycle
events, in lockstep with the in-memory store (launch, ready, complete). There is
**no producer** for a real local liveness handle, no heartbeat on
`last_seen_at`, and restart reconciliation *preserves* a row's `status` without
re-checking reality. So whenever a process dies **outside** a hooked event — a
crash, a kill, a backend restart, an escalated cancel — the row keeps claiming a
process that is gone, and nothing corrects it.

Current solutions fall short because the v0.73.0 fix (#1587) addressed only the
turn/session half (no dangling open turns, no sessions stuck `RUNNING`) and the
`ready` promotion it added is ineffective for the headless/standalone case. The
executor-row half — truthful liveness, correct cancel/resume, bounded backlog —
is still broken.

**Scope of this document (one theme, two layers).** The unifying problem is *the
session lies about being busy*. That manifests at two layers with one root
cause: the durable `executors_running` table (rows claim a process that is gone)
and the in-memory turn/session "busy" signal (too coarse to tell "foreground
generating" from "idle / waiting on background"). This REQUIREMENTS.md owns both;
`/spec` and `/plan` slice them into shippable pieces.

A confirmed concrete trigger of the "session refuses input" symptom (verified
via `acpdbg` against the live `claude-acp` agent) is a Claude session that
launched background work the busy signal failed to recognize — a **Monitor**
watch or a **run_in_background Bash** shell — leaving the operator locked out
while the foreground turn was actually idle. This is evidence for the existing
requirement above, not a new one.

## Success criteria §req:success-criteria

Observable from the app and the live database, after normal multi-hour headless
use and across restarts:

1. **Pause, then resume, works without restarting the service.** After the
   operator stops/pauses a running agent turn, sending a new message resumes the
   same session with its context intact — no wedged "still running" state, no
   service bounce required.
2. **A running/ready session's row is truthful.** When a session has finished
   launching and is between turns, its row reads `ready` (not stuck
   `starting`/`prepared`) and carries a real, currently-alive liveness handle
   and endpoint (`agentctl_port`, `last_seen_at`, and a live process reference)
   that an operator can cross-check against the host — for the local/standalone
   runtime, not only remote SSH.
3. **A dead row is distinguishable from a live one** from the row alone, so
   repair/prune can be automated safely.
4. **A wedged session recovers on its own** — the existing self-heal path
   becomes reachable in practice, without a manual restart or waiting on a UI
   poll to happen to fire.
5. **The backlog stops growing.** After a clean restart following heavy use, the
   count of stale rows (rows whose process is dead) trends toward zero rather
   than accumulating across restarts.
6. **Restart makes rows true, not just sessions.** Reconciliation repairs live
   rows to reflect reality and removes only rows whose session is truly
   finished — it never leaves a row claiming a process that no longer exists.
7. **No resumable session ever loses its resume ability.** No cleanup — on
   cancel, on startup, or otherwise — deletes a row that backs a still-open
   session or holds a `resume_token`; a session waiting for input before a
   restart is still resumable with full context afterward.
8. **The session accepts input whenever the foreground turn is idle.** When the
   foreground agent turn has finished — or is only *waiting on a spawned
   background task* — the session accepts a new message rather than reporting
   "running/busy" and dropping or rejecting it. Kandev distinguishes "foreground
   actively generating" from "idle / waiting on background." *(Same theme as #1:
   the busy signal must be true.)*

**Scope disposition (2026-07-06).** While this work was in review, upstream
fixed the live-path root causes independently: #1600 (Monitor callbacks emitted
content outside a prompt RPC, so the turn never completed and the session
stayed "busy") and #1602 (resume reused executions whose ACP prompt path was
dead even though the process looked alive). Criteria **#1, #4, #8** are
therefore satisfied primarily by those upstream root-cause fixes (plus this
branch's cancel-path queue drain for #1); this branch delivers **#2, #3, #5,
#6, #7** (the durable-table half, which upstream did not touch). The
finer-grained busy signal behind #8's "waiting on a spawned background task"
clause — and mid-turn steering for agents that support it — is a follow-up
product feature tracked separately; a working prototype lives on branch
`archive/1597-full-six-batches`.

**These criteria are the acceptance tests.** Each must be proven by a test that
reproduces the symptom end-to-end, not merely asserted in isolation. Today's
suite exercises the underlying mechanisms on happy paths (reconcile,
cancel-unstick, resume, ready-persist) and in one place codifies the wedge
itself as expected — `TestResumeSession_LiveAgentReturnsAlreadyRunning` requires
a live-*looking* session to reject a new message, and
`TestCancelAgent_LeavesQueuedMessageForManualDrain` requires a paused session to
leave the next message undelivered. So none of the criteria above is currently
guarded by a failing test, and some are contradicted by passing ones. The work
starts by writing red characterization tests for each symptom (backend
integration for the state model; Playwright e2e for the pause→resume and
foreground/background operator flows), then fixing to green.

## User stories §req:user-stories

- **As an operator who paused an agent to redirect it**, I want to type a new
  message and have the agent pick up where it left off, so that pausing is a
  normal part of the workflow and not a way to permanently wedge a session that
  forces me to restart the whole server. *(→ §req:success-criteria #1, #4, #7)*

- **As an operator running Kandev headless for days**, I want the
  `executors_running` table to reflect the processes actually running, so that
  when the app (or I) read it, we get the truth about what is live and what is
  dead. *(→ §req:success-criteria #2, #3)*

- **As an operator on a long-lived server**, I want dead executor rows repaired
  or cleaned up automatically instead of piling up, so that the backlog doesn't
  grow without bound and hide real problems. *(→ §req:success-criteria #5, #6)*

- **As an operator with sessions waiting for input**, I want those sessions to
  survive a restart and stay resumable with history intact, and I want cleanup
  to never delete a row I still need to resume, so that a routine restart or
  automated cleanup can't cost me a conversation. *(→ §req:success-criteria #7)*

- **As an operator whose agent kicked off background work**, I want to keep
  talking to the session while that background task runs, so that a long
  background job doesn't lock me out of the conversation the way a genuinely
  in-flight turn would. *(→ §req:success-criteria #8)*

## Quality attributes §req:quality-attributes

- **Event-driven correctness first; reconciliation is a redundant backstop.**
  The row must be made truthful by hooking every lifecycle transition (launch,
  boot-ready, turn-complete, cancel including escalated cancel, process-exit /
  crash, stop). A health/verify-against-reality pass exists *only* to catch what
  events cannot — backend crash, OOM kill, orphaned process, dropped event — and
  guarantees eventual consistency during those unusual cases. Polling must never
  be the primary means of updating the table when there is an event to hook.
- **Truthfulness / durability.** The executor-state table is a source of truth
  the app relies on (ADR 0025); it must stay consistent with the live processes
  through normal operation and across restarts.
- **Data safety with a single source of truth.** Losing the ability to resume a
  session is unacceptable. The remedy is an ironclad rule about what may be
  deleted — *not* duplicating `resume_token` into a second table, which would
  introduce the same divergence risk this effort is removing.
- **Self-healing under long uptime.** The target is a service that runs for days
  unattended; recovery from wedged/stale state must be automatic.
- **Bounded growth.** No unbounded accumulation of dead rows (and, relatedly, no
  unbounded accumulation of orphaned OS processes — see #1247).
- **Runtime-aware liveness.** Liveness semantics differ by runtime: an SSH row's
  process id lives on a remote host. Liveness/prune logic must respect the
  runtime and never apply local-process checks to a remote row.
- **No regression** of the v0.73.0 turn/session fix (#1587) or of the SSH
  executor's existing remote-pid stop path.

## Constraints §req:constraints

- **Environment:** v0.73.0 baseline; Homebrew linux-x64; headless
  (`kandev --headless`) long-lived systemd service; local/standalone (`local_pc`)
  executor; Claude Agent SDK over ACP with per-session HTTP MCP.
- **Do not overload the SSH `pid` semantics.** `executors_running.pid` is today
  written only by the SSH executor and holds the agentctl pid **on the remote
  host** (used to stop it over SSH). A local liveness handle must not silently
  change what that column means for SSH rows; local-liveness checks must never
  run against remote rows. Prefer an unambiguous local handle over reusing the
  remote-pid field. *(Operator concern; confirmed in code.)*
- **Do not duplicate `resume_token`.** Keep one source of truth. Guarantee
  safety with an invariant instead: a row backing a non-terminal session, or
  holding a `resume_token`, is **repaired in place, never deleted**; only rows
  confirmed terminal/dead may be pruned. *(Operator decision.)*
- **PID scope — real and live, not a per-session redesign.** "Liveness handle
  populated" means a real, currently-alive process reference sufficient to
  detect and clean dead rows (the process Kandev already spawns is acceptable);
  it does **not** require building a new per-session runtime handle. *(Operator
  decision; most-robust-without-overbuilding.)*
- **ADR 0025 stays.** `executors_running` remains the authoritative runtime
  inventory; the fix makes that table trustworthy rather than reverting the
  decision. A superseding/for-record ADR update may be warranted.
- **Backend conventions apply:** SQLite column changes via idempotent
  `ADD COLUMN` migrations; event-publishing and goroutine-ownership rules in
  `apps/backend/CLAUDE.md` hold; every changed behavior needs regression tests.

## Priorities §req:priorities

Ordered by operator impact. Scope confirmed as the **full** fix (complete the
event hooks + populate liveness + reconcile), not a single slice.

**Must have**

1. **Characterization tests first (red before green).** Land failing tests that
   reproduce each symptom — stuck/accumulating rows, `pid=0` for local runtimes,
   pause→resume wedge, and foreground-idle-while-background-running rejecting
   input — before changing behavior. Existing tests that codify the wedge
   (`TestResumeSession_LiveAgentReturnsAlreadyRunning`,
   `TestCancelAgent_LeavesQueuedMessageForManualDrain`) are updated to the
   corrected contract. *(§req:success-criteria — all)*
2. **Pause → resume works without a service restart.** Cancel/pause leaves the
   session and its row in a state a new message can resume; the operator never
   has to bounce the service to recover a paused session.
   *(§req:success-criteria #1, #4, #8)*
3. **Complete the event hooks so the row is truthful in the common case.** Every
   lifecycle transition (launch, boot-ready, turn-complete, cancel + escalated
   cancel, process-exit/crash, stop) leaves the row's `status`, endpoint, and a
   real local liveness handle (`last_seen_at`, live process reference) correct —
   for local/standalone, not only SSH. *(§req:success-criteria #2, #3)*
4. **A truthful, fine-grained busy signal.** The session's "busy" state reflects
   whether the foreground turn is actively generating, so input is accepted when
   it should be. *(§req:success-criteria #8)*
5. **Reconciliation as a redundant backstop.** On startup (and, where events
   can't cover it, periodically) verify rows against reality: repair live rows,
   prune only rows confirmed terminal/dead, and **never** delete a row backing a
   non-terminal session or holding a `resume_token`. The backlog stops growing.
   *(§req:success-criteria #5, #6, #7)*

**Nice to have**

6. Reduce related orphaned-process accumulation (#1247) by using the real
   liveness handle to identify and reap dead agent processes. *(Deferred
   without code: #1247's measured evidence is live idle sessions — the
   existing idle-instance reaper's job — not dead trees under deleted
   worktrees, which have not been observed. A working dead-tree reaper
   lives on branch `archive/1597-full-six-batches` if evidence appears.)*
7. Record the "table is trustworthy / event-primary, reconcile-redundant"
   decision as an ADR update alongside/superseding ADR 0025.
