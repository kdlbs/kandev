# Specification

> Solution-space document for GitHub issue
> [#1597](https://github.com/kdlbs/kandev/issues/1597) — "Executor-row
> desync persists on v0.73.0". Derived from
> [REQUIREMENTS.md](./REQUIREMENTS.md). Each section cites the
> `§req:` it satisfies.
>
> **One theme, two layers.** The unifying defect is *the session lies
> about being busy*. It manifests in the durable `executors_running`
> table (rows claim a process that is gone) and in the live prompt
> path (a session that looks busy against a turn/process that is
> actually finished or dead). ADR 0025 promoted `executors_running` to
> the authoritative runtime inventory without giving its liveness
> columns a trustworthy producer; this spec makes the table truthful
> rather than reverting that decision.
>
> **Scope note (2026-07-06).** The live-path half was root-caused and
> fixed upstream while this work was in review: #1600 (Claude Monitor
> callbacks emitted content outside a prompt RPC, so the turn never
> completed and the session stayed "busy" forever) and #1602 (resume
> reused executions whose ACP prompt path was dead even though the
> process looked alive). This spec therefore owns the durable-table
> half plus pause→resume consistency. A finer-grained busy signal
> (accepting input while the foreground turn is idle on background
> work, and mid-turn steering for agents that support it) is a
> follow-up product feature tracked separately; a working prototype of
> the background-idle half lives on branch
> `archive/1597-full-six-batches`.

## Pause then resume §spec:pause-resume-recovery

*Status: done — pausing a running turn settles the session to
WAITING_FOR_INPUT and `Service.CancelAgent` now drains the message queue
directly after reconciling, so a queued message is delivered on resume
even on the escalated / dead-process cancel path where no `agent.ready`
event fires (idempotent with the event-driven drain). The resume path
distinguishes a genuinely-generating foreground turn (reject) from a row
that looks live but whose process is gone (clean + relaunch via
`resume_token`), reusing the runtime-aware liveness from
§spec:truthful-executor-rows. Reachable from the pause button via the
`agent.cancel` WS action. Corrected contracts:
`TestCancelAgent_DeliversQueuedMessageOnResume` (replaces the wedge test
`TestCancelAgent_LeavesQueuedMessageForManualDrain`) and
`TestResumeSession_StaleExecutionCleansUpAndRetries` (now asserts the
relaunch reuses the row's `resume_token`); Playwright coverage in
`pause-resume-recovery.spec.ts`.*

**Problem.** On v0.73.0 an operator who pauses a running agent turn
cannot resume it: the next message is dropped or rejected because the
session still looks "busy"/running, and the only recovery is to cancel
and restart the whole headless service — which every other session
depends on.

**Behavior.** Pausing is a first-class, reversible step in the
workflow:

- When the operator stops/pauses a running agent turn, the session
  settles into a state that accepts input (`WAITING_FOR_INPUT`) and its
  `executors_running` row is left resumable — the process is not
  orphaned and the row is not deleted while the session is still open.
- When the operator then sends a new message, the system resumes the
  same session with its context intact, using the row's `resume_token`,
  without a service restart.
- When a paused session's agent process is actually gone (crash, kill,
  escalated cancel), the resume path detects the dead process, cleans
  the stale execution, and relaunches — rather than reporting "already
  running" against a process that no longer exists.
- The queued-message contract is corrected: a message sent to a paused
  session is delivered on resume rather than being stranded for
  indefinite manual drain.

**Corrected test contracts.** Two existing tests currently codify the
wedge as expected behavior and are rewritten to the corrected
contract, red before green:

- `TestResumeSession_LiveAgentReturnsAlreadyRunning` — a session that
  merely *looks* live must not permanently reject a resume; the
  contract distinguishes "a foreground turn is genuinely generating"
  (reject/queue) from "the row looks live but the process is gone"
  (clean and relaunch). It no longer requires a live-*looking* session
  to reject a new message unconditionally.
- `TestCancelAgent_LeavesQueuedMessageForManualDrain` — a paused
  session's queued message is resumable, not stranded; the test
  asserts the message is delivered on resume.

**Why.** The pause→resume wedge is the operator's headline pain
(§req:problem-statement). The single-scalar `RUNNING` gate in
`checkSessionPromptable` rejects input for any session whose DB state
reads `RUNNING`, even after its process has died, and cancel does not
reconcile the row's liveness — so a routine pause silently wedges the
session. The corrected contract makes "busy" mean "a foreground turn is
actually generating," which is the precondition every other section in
this spec establishes.

**Alternatives rejected.** Re-landing #1594 (re-adding the `ready`
promotion) — rejected: the `ready` status already shipped in v0.73.0
via #1587 and is ineffective for the headless/standalone case, so it is
not the fix (§req:problem-statement note). Requiring the operator to
cancel-then-restart — rejected: it is the very symptom being removed.

**Tradeoffs.** Accepting input against a session that is technically
still finishing an escalated cancel requires the resume path to probe
real liveness (see §spec:truthful-executor-rows), adding a liveness
check on the hot resume path in exchange for correctness.

Satisfies §req:success-criteria #1, #4, #7, #8; §req:user-stories
(operator who paused to redirect); §req:priorities must-have #1, #2.

## Truthful executor rows §spec:truthful-executor-rows

*Status: done — local/standalone rows carry a real host-local liveness handle
(`executors_running.local_pid`), populated event-driven on every lifecycle
transition (launch, boot-ready, turn-complete, cancel, process-exit/crash via
MarkCompleted), with `status`, `agentctl_port`, and an observed `last_seen_at`.
Delivered by the foundation batch (#1597).*

**Problem.** For local/standalone (`local_pc`) runtimes every
`executors_running` row reports `pid = 0` and a `last_seen_at` that
only reflects the last lifecycle write, never a real liveness check.
Rows sit stuck at `starting`/`prepared` with no endpoint. Because the
row cannot distinguish a live session from a dead one, everything that
reads it — cancel cleanup, reconciliation, self-heal — makes wrong
decisions.

**Behavior.** A row describes the real process it mirrors, for the
local/standalone runtime and not only for SSH:

- When a session has finished launching and is between turns, its row
  reads `ready` (not stuck `starting`/`prepared`) and carries a real,
  currently-alive local liveness handle and endpoint — a live
  OS-process reference for the process Kandev spawned, plus
  `agentctl_port`, `agentctl_url`, and a `last_seen_at` that reflects an
  actual liveness observation — that an operator can cross-check against
  the host.
- Every lifecycle transition leaves the row correct: launch,
  boot-ready, turn-complete, cancel (including escalated cancel),
  process-exit / crash, and stop each update the row's `status`,
  endpoint, and liveness handle. Event hooks are the primary producer;
  no lifecycle transition leaves the row claiming a process that has
  exited.
- A dead row is distinguishable from a live one from the row alone
  (status plus a liveness handle whose process can be probed), so
  automated repair/prune can decide safely without external context.

**Why.** ADR 0025 made this table load-bearing but v0.66 never gave its
liveness columns a producer for local runtimes, so pre-existing drift
gained consequences (§req:problem-statement). The remedy is
event-driven correctness first: the row is made truthful by hooking
every transition, because polling can never be as timely or as cheap as
the event that already fires (§req:quality-attributes, event-driven
correctness first). The liveness handle is a real, currently-alive
process reference sufficient to detect dead rows — the process Kandev
already spawns is acceptable; it is not a new per-session runtime handle
(§req:constraints, PID scope).

**Alternatives rejected.** Reusing the SSH `pid` column for a local
process id — rejected: `executors_running.pid` holds the agentctl PID
*on the remote host* for SSH rows, and overloading it would silently
change that column's meaning and invite local-process checks against
remote rows (§req:constraints; see §spec:runtime-aware-liveness). A
periodic poll as the primary liveness producer — rejected: events are
primary, polling is a redundant backstop only (§spec:reconciliation-backstop).

**Tradeoffs.** Recording and probing a local process handle adds a
column and a liveness call at each hooked transition; accepted because
untrustworthy rows are the root cause of every downstream wrong
decision.

Satisfies §req:success-criteria #2, #3; §req:user-stories (operator
running headless for days); §req:priorities must-have #3.

## Runtime-aware liveness §spec:runtime-aware-liveness

*Status: done — `RowProcessLiveness` judges a row by its `runtime`: local rows
are probed by `local_pid`; SSH/remote and docker rows return Unknown so a
host-local process check never runs against a remote pid. The SSH remote-pid
stop path (`kill -0` over SSH) is unchanged. Delivered by the foundation batch
(#1597).*

**Problem.** Liveness semantics differ by runtime: an SSH row's process
id lives on a remote host, while a local row's process lives on the
Kandev host. A single local-process liveness check applied blindly
would either corrupt SSH rows or regress the SSH remote-pid stop path.

**Behavior.** Liveness and prune logic branch on the row's `runtime`:

- Local/standalone rows are judged by the local liveness handle
  (§spec:truthful-executor-rows); local-process existence checks never
  run against SSH (remote) rows.
- SSH rows retain their existing meaning: `pid` is the agentctl PID on
  the remote host, and the remote-pid stop path continues to stop the
  remote process over SSH unchanged.
- Reconciliation and pruning evaluate each row against the correct
  host for its runtime, so a remote row is never pruned because a local
  process check failed and vice versa.

**Why.** The operator explicitly flagged, and the code confirms, that
`pid` is SSH-only remote semantics; local liveness must not overload it
(§req:constraints, do not overload SSH pid). No-regression of the SSH
executor's remote-pid stop path is a stated quality attribute
(§req:quality-attributes).

**Alternatives rejected.** A single runtime-agnostic liveness predicate
— rejected because it cannot be correct for both a local process and a
remote pid; runtime-awareness is inherent to the problem.

**Tradeoffs.** Per-runtime branching in the liveness/prune paths adds
conditional complexity; accepted as the minimum required to avoid
cross-runtime corruption.

Satisfies §req:success-criteria #2, #3; §req:quality-attributes
(runtime-aware liveness, no regression); §req:constraints (SSH pid).

## Resume-safety invariant §spec:resume-safety-invariant

*Status: done (#1597 Batch 4)*

**Problem.** Losing the ability to resume a session is unacceptable,
yet multiple cleanup paths (cancel cleanup, startup reconciliation,
on-demand stale cleanup, task teardown) delete `executors_running`
rows, and any of them could delete a row that still backs an open,
resumable session.

**Behavior.** One ironclad rule governs deletion, enforced across every
cleanup path:

- A row backing a non-terminal session, or holding a `resume_token`, is
  repaired in place and never deleted. Only rows confirmed
  terminal/dead may be pruned.
- A session waiting for input before a restart is still resumable with
  full context afterward; no cleanup — on cancel, on startup, or
  otherwise — removes a row it still needs to resume.
- `resume_token` remains a single source of truth in
  `executors_running`; it is not duplicated into a second table.

**Why.** Data safety with a single source of truth is a hard
requirement (§req:quality-attributes). Duplicating `resume_token` into
a second table would reintroduce exactly the divergence risk this effort
removes, so the guarantee is expressed as a deletion invariant rather
than as redundant storage (§req:constraints, do not duplicate
resume_token).

**Alternatives rejected.** Copying `resume_token` into `task_sessions`
so a deleted runtime row is survivable — rejected: two writers of the
same fact is the divergence pattern being eliminated. Deleting rows
eagerly and relying on relaunch — rejected: it can destroy the only
handle to a resumable conversation.

**Tradeoffs.** Cleanup paths must classify a row as terminal/dead
before deleting, adding a check to each path; accepted because an
erroneous delete costs the operator a conversation.

Satisfies §req:success-criteria #7; §req:quality-attributes (data
safety); §req:constraints (do not duplicate resume_token);
§req:user-stories (operator with sessions waiting for input).

## Startup reconciliation §spec:reconciliation-backstop

*Status: done — startup reconciliation repairs live-looking rows whose
local process is confirmed dead (status=stopped, `local_pid` cleared,
`resume_token`/worktree preserved) and prunes only rows that fail
`RowMustBePreserved`. Scoped deliberately to startup: events (the
lifecycle hooks of §spec:truthful-executor-rows) are the primary
producer, and every in-app transition — including process exit/crash,
which the lifecycle manager observes — fires one. A periodic polling
backstop was built, then removed before merge: it defended against
failure modes (OOM kill, dropped event) that have not been observed,
and the live-wedge causes it would have self-healed were fixed at their
root upstream (#1600 completes idle async ACP turns; #1602 reaps
prompt-dead executions on resume).*

**Problem.** Nothing reconciled rows against reality at startup:
the pass preserved a row's `status` without re-checking the process,
and stale rows were pruned only on-demand per session. So after heavy
headless use the backlog of dead rows grew across restarts (~50 → 77,
measured in #1597).

**Behavior.** The startup pass makes rows true:

- On startup the system verifies rows against reality using
  runtime-aware liveness: rows whose local process is confirmed dead
  are repaired so they no longer claim a live process, and only rows
  whose session is confirmed terminal/dead are pruned, subject to the
  §spec:resume-safety-invariant.
- Reconciliation never deletes a row backing a non-terminal session or
  holding a `resume_token`.
- After a clean restart following heavy use, the count of stale rows
  (rows whose process is dead) trends toward zero rather than
  accumulating across restarts.

**Why.** Events are the primary producer; startup reconciliation exists
because a backend restart is exactly the moment events could not have
fired for whatever the previous process was doing when it died
(§req:quality-attributes, event-driven correctness first). ADR 0025
stays: the table remains the authoritative inventory and is made
trustworthy rather than reverted (§req:constraints, ADR 0025 stays).

**Alternatives rejected.** Polling as the primary means of updating the
table — rejected: events are primary and must be hooked wherever they
exist; polling never updates a row the moment an event could
(§req:quality-attributes). A periodic in-run backstop pass — built and
then removed: insurance for undemonstrated failure modes; can be
restored from branch `archive/1597-full-six-batches` if a stale row is
ever observed to appear *between* restarts. Reverting ADR 0025 to drive
cleanup off `task_sessions` again — rejected: it reintroduces multiple
paths deciding liveness, which ADR 0025 consolidated for good reason.

Satisfies §req:success-criteria #5, #6; §req:quality-attributes
(bounded growth, truthfulness/durability); §req:priorities must-have
#5; §req:user-stories (operator on a long-lived server).
