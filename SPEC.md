# Specification — Task busy indicators reflect all in-flight work

> Solution-space document derived from [REQUIREMENTS.md](./REQUIREMENTS.md).
> Each section cites the `§req:` slug it satisfies.
>
> **Two layers, one truth.** A task's live state has a *producer* (the
> backend signal that knows whether foreground or background work is in
> flight) and a *surface* layer (every place that state is drawn). The
> producer is largely landed — see **Foundation** at the end — and is
> governed by ADR-0046 (fine-grained foreground-idle busy signal) and the
> executor-row / pause-resume effort tracked under GitHub issue
> [#1597](https://github.com/kdlbs/kandev/issues/1597). This document's
> primary scope is the **surface** layer: making every indicator tell the
> truth about *all* in-flight work — foreground and background — the same
> way, everywhere, live, and preventing the destructive misstep a false
> "complete" invites (§req:problem-statement).
>
> **Status honesty.** Much of the surfacing has already landed and is
> marked accordingly; the sections below record both what is true today
> and the gaps this effort closes. The Foundation sections retain their
> original `§req:` citations, which reference the prior requirements
> revision; the surfacing sections cite the current REQUIREMENTS.md.
>
> **Out of scope.** The Office area (autonomous-agent management) keeps
> its own status vocabulary and is not brought into this state model
> (operator decision). Composer input-gating ("you may type") is a
> separate ADR-0046 concern and is not what these indicators signal.

---

## Four distinguishable states, not by color alone §spec:state-vocabulary

*Status: complete — all four states are shipped across the in-scope surfaces
and remain distinguishable without relying on color alone.*

Every status surface communicates four mutually distinguishable
conditions for a task or session:

- **foreground-running (generating)** — the foreground agent is actively
  producing output. This is the established "it's running" affordance
  (the gold/yellow spinner on the sidebar, the running spinner on cards),
  left unchanged.
- **background-running** — the foreground turn is idle but recognized
  spawned work (a subagent session, a `run_in_background` shell, or an
  active Monitor watch) is still running.
- **waiting-for-my-input** — the agent is idle and waiting for the
  operator to reply.
- **finished (done)** — nothing is running and nothing is pending.

Any two of these are told apart without relying on hue alone: the
distinction survives a grayscale/desaturated view, small/compact sizes,
and a quick glance — carried by shape, motion, or an accessible
label/tooltip in addition to color.

**Decision and the constraint that drove it.** The four states are
defined as a shared *meaning*, and each surface expresses that meaning
within its own existing visual language rather than every surface being
forced onto one identical icon set. This is driven by a hard tension in
the requirements: the same underlying state must read the same way
everywhere (§req:success-criteria #5), yet the established
foreground/generating affordance must not be restyled
(§req:success-criteria #9, §req:constraints) — and today there is not one
established generating look but several (a gold spinner on the sidebar, a
blue running spinner on cards, a filled dot in session menus). Those two
constraints cannot both be fully honored if "same meaning" is read as
"pixel-identical." The resolution: each surface keeps its current
generating and done affordances untouched and *adds* a background-running
affordance (and, per §spec:waiting-for-input-parity, a consistent
waiting affordance) that is distinct on that surface. Consistency is
guaranteed at the level that matters to the operator — the same
underlying condition always produces the same not-done, not-generating
reading everywhere — without a disruptive restyle of indicators operators
already recognize.

**Alternatives considered.**

- *Converge every surface onto one canonical icon set.* Rejected: it
  delivers the strongest cross-surface identity but visibly restyles
  several current running indicators, which both enlarges the change and
  violates the "foreground stays as-is" constraint. The consistency gain
  over "same meaning, per-surface expression" is not worth that blast
  radius for a change whose goal is truthfulness, not a visual redesign.
- *Distinguish background by color alone* (a different-hue spinner).
  Rejected: fails §req:quality-attributes (accessibility) — two spinners
  that differ only in hue are not reliably distinguishable in a dense
  scan or for color-vision-deficient operators.

**Tradeoffs.** The background-running (and waiting) affordance is not
guaranteed identical pixel-for-pixel across surfaces; the guarantee is
semantic (never done, never mistaken for generating), not pictorial —
an accepted cost of leaving established affordances in place.

**User-level test path.** Drive a session through all four conditions and,
on each surface, confirm the four indicators are mutually distinct and
that the distinction survives a grayscale view.

§req:success-criteria (#3, #4, #9) · §req:quality-attributes ·
§req:constraints

---

## Task-level indicators reflect all of a task's work §spec:task-level-truth

*Status: complete — every at-a-glance task surface, including both sidebars,
uses the task-level most-active-wins truth and never renders "done" while any
owned session is working.*

On every at-a-glance task surface — the board / kanban card, the task
list rows, the graph / swimlane nodes, the open-task header, and the
desktop and mobile sidebar — a task whose foreground turns are idle while
spawned background work is still running shows the background-running
affordance, not a done affordance and not the generating affordance. No
task-level surface shows "done" while any session the task owns still has
foreground or background work outstanding.

For a task running more than one session at once, its single task-level
indicator follows **most-active-wins**: generating if any session is
generating; background-running when no session is generating but at least
one has background work running; otherwise it falls through to today's
behavior (waiting / done / failed, per existing rules). The
background-running tier is inserted between generating and done; it does
not redefine how the other states render.

**Decision and the constraint that drove it.** The task-level aggregate
(including the most-active-wins reduction across a task's sessions) is
computed on the backend and delivered on the task record — so every
task-level surface reads one authoritative value rather than each
re-deriving it. This is driven by how these surfaces get their data: the
board, list, graph, header, and sidebar render from the task record
(initial payload plus task-level update events) and do not hold the full
session set for tasks that are not open. Deriving the aggregate on the
client would be blank or stale for exactly the off-screen tasks an
operator is scanning, and would let surfaces disagree (§req:success-
criteria #5, §req:quality-attributes: single source of truth). A
backend-computed, task-record-borne value is correct on first paint, in a
second tab, and for tasks never expanded. The sidebar's current
session-substate derivation is the one task-level surface that violates
this — it can disagree with the board for a multi-session task — and is
brought onto the same aggregate.

**Change from prior aggregation.** Task-level surfaces previously
variously reflected only the *designated primary* session, or a
most-active ranking, or the most-recent live session. This capability
makes the reading uniformly most-active-wins. An operator will observe
that a task whose primary session finished but whose secondary session is
still working now reads as working, where a primary-only surface could
have read done — the intended behavior: a task is not "done" while any of
its sessions is still working (§req:success-criteria #1, #2).

**Alternatives considered.**

- *Derive the task-level aggregate on the client from the session store.*
  Rejected: the store is not guaranteed to hold every visible task's full
  session set, so off-screen rows and cards would show blank or stale
  background state and could disagree with the open-task view — re-
  introducing the multi-source-of-truth problem this work ends.
- *Keep primary-session-only aggregation and just add the background
  tier.* Rejected: it would still falsely read "done" for a task whose
  primary session finished while a secondary session works, directly
  violating §req:success-criteria #1.

**Tradeoffs.** Most-active-wins means a task-level surface can now reflect
a non-primary session's activity — a behavior change some operators may
notice — accepted because it is precisely what makes the indicator
truthful. Carrying the aggregate on the task record means a task-level
update is emitted when a session's activity flips between generating and
background; this added traffic is accepted (see §spec:live-and-durable).

**User-level test path.** With a single-session task, drive it
generating → background-running → done and confirm the board card, task
list row, graph/swimlane node, open-task header, and sidebar each reflect
all three without opening the task. With a two-session task whose primary
finished but whose secondary runs background work, confirm every task-
level surface — including the sidebar — reads background-running, not done.

§req:success-criteria (#1, #2, #5) · §req:user-stories · §req:priorities
(must-have #1, #2) · §req:quality-attributes

---

## Session-level indicators surface the substate uniformly §spec:session-level-truth

*Status: complete — desktop and mobile session surfaces all distinguish
background-running from generating and done using the shared vocabulary.*

Every surface that shows a per-session status reflects the same four
states. The session switcher and the session-reopen menu already
distinguish background-running from generating and from done. The mobile
sessions section — brought onto the shared `getSessionStateIcon`
vocabulary — shows it too, so an operator on a compact view sees the same
truth as on desktop (§req:success-criteria #5, §req:quality-attributes:
mobile parity).

**Decision and the constraint that drove it.** Session-level surfaces read
the per-session substate the signal already emits and already places on
the session record (initial payload plus the per-session
`session.activity_changed` event) and render it through the same
vocabulary as every other surface. The driving constraint is the
requirement's core grievance: the signal is produced end-to-end and some
surfaces render it, yet peer surfaces (the mobile sessions section) drop
it, so the operator cannot trust any single icon. Bringing every
session-level surface onto the substate it is already handed removes the
disagreement without new mechanism.

**Alternatives considered.**

- *Leave the mobile sessions section as-is and only fix desktop/task-level
  surfaces.* Rejected: mobile parity is a stated quality attribute, and a
  session that reads "working" on desktop but shows no such state on
  mobile re-creates the contradiction the effort exists to remove
  (§req:success-criteria #5, §req:user-stories: operator on mobile).

**Tradeoffs.** The mobile sessions section gains a state it did not
previously render — a small additional element in a dense control —
accepted because its silence today is precisely the defect.

**User-level test path.** With a session in the background-running
condition, open the session switcher, the reopen menu, and the mobile
sessions section and confirm each shows the same background-running
reading — distinct from generating and never a done check.

§req:success-criteria (#3, #5) · §req:user-stories · §req:quality-
attributes (mobile parity) · §req:constraints

---

## Waiting-for-my-input reads consistently everywhere §spec:waiting-for-input-parity

*Status: complete — the waiting, clarification, and permission readings now
match across every task and session surface and remain distinct from done and
both running states without relying on color alone.*

Waiting-for-my-input is a first-class fourth state, distinguishable from
finished and from both running states, on every surface — not only the
sidebar. Where the sidebar today reads "waiting" richly (an agent that has
finished its turn and is waiting on a reply, a pending clarification
question, or a pending permission prompt), the same "needs me" reading is
carried to the board card, task-list row, graph/swimlane node, open-task
header, and the session menus, so an operator scanning any surface can
tell "needs me" apart from "done" and from "still working."

**Decision and the constraint that drove it.** Waiting-for-input is
promoted to full cross-surface parity (operator decision), driven by
§req:success-criteria #3, which requires all four states distinguishable
*on every surface*. The requirement is internally split — its priorities
list waiting-for-input strengthening as nice-to-have #6 — and the operator
resolved the tension in favor of full parity: "needs me" is as
consequential to the operator's triage as "still working," so a surface
that collapses waiting into the coarse state is as untrustworthy as one
that collapses background into done. The reading derives from the same
source of truth the sidebar already uses (coarse `WAITING_FOR_INPUT`
plus the message-derived pending-clarification / pending-permission
flags), lifted into the shared vocabulary rather than re-invented per
surface (§req:constraints: "make it consistent, not invent it").

**Alternatives considered.**

- *Reconcile only surfaces that actively contradict each other and defer
  richer parity.* Rejected by the operator in favor of full parity: a
  surface that shows the coarse state where the sidebar shows a pending-
  clarification question still leaves the operator unable to trust that
  surface at a glance.
- *Leave waiting-for-input untouched (it "partly works").* Rejected: the
  same not-color-alone and every-surface-agrees bars that apply to
  background apply to "needs me"; leaving it uneven keeps a
  surface-to-surface disagreement the requirements call out.

**Tradeoffs.** Carrying the message-derived pending flags (clarification /
permission) to every task surface widens the inputs each surface consumes
beyond the coarse state, a modest increase in what those surfaces read;
accepted as the cost of a trustworthy "needs me" everywhere.

**User-level test path.** Put a task into each waiting condition (turn
finished awaiting reply; pending clarification question; pending
permission prompt) and confirm the card, list row, graph/swimlane node,
header, and session menus each read "waiting for input" distinctly from
done and from running — matching the sidebar.

§req:success-criteria (#3) · §req:user-stories · §req:priorities
(nice-to-have #6, elevated by operator decision) · §req:constraints

---

## Live, durable, single-source, and safe-by-default §spec:live-and-durable

*Status: complete — live updates, fresh loads, and safe fallback all preserve
the rule that an in-flight task never reads as done.*

As a task moves between the four states, every in-scope surface —
session-level and task-level — updates promptly without a manual refresh
(within a second or two of the change). A freshly loaded page and a
second tab show the correct state immediately rather than a stale value
that only corrects on the next transition. The state stays correct across
page refresh, task reopen, and a backend restart. When the fine-grained
substate is momentarily unknown or unavailable, no surface falsely reads
"done" while a turn is still open.

**Decision and the constraint that drove it.** All surfaces derive from
one shared notion of task/session state — the per-session substate and the
task-record aggregate — pushed live; per-session flips propagate to
session-level surfaces, and because the aggregate rides the task record, a
session's activity flip also refreshes the task-level surfaces subscribed
to that task. The initial payload and the records carry the current
substate so first paint and additional tabs are correct without waiting
for a transition. This is driven by §req:quality-attributes (a stale
"done" that only clears on refresh is itself a defect: the indicator's
whole value is being trusted *at the moment of the glance*) and by the
single-source-of-truth attribute (surfaces cannot disagree because they
read the same field).

**Safe fallback.** The fine-grained substate is in-memory and best-effort
by design (ADR-0046); after a backend restart the tracker resets. The
fallback is chosen so an unknown substate never resolves to "done" while a
turn is open: an in-flight session whose substate is unknown reads as
running (never done), and a task with such a session does not read done.
Correctness of "not falsely done" is never traded for the optimization of
the finer generating-vs-background distinction (§req:quality-attributes:
truthfulness first — false "done" is the dangerous error).

**Alternatives considered.**

- *Persist the fine-grained substate so it survives restart.* Rejected
  (consistent with ADR-0046): a persisted copy becomes a second source of
  truth that can drift, can survive a restart as a false reading for a
  session whose turn is already gone, and adds write churn on the hot
  streaming path. The substate only matters for a live in-flight turn;
  resetting to the safe default on turn-close is sufficient and simpler.
- *Refresh task-level surfaces only on coarse state changes.* Rejected: a
  generating→background flip does not change the coarse state, so scanning
  surfaces would show the wrong one of the states until an unrelated
  transition — reproducing the stale-until-refresh defect.

**Tradeoffs.** Emitting a task-level refresh on activity flips adds update
traffic proportional to how chatty background work is (a bursty Monitor);
accepted as the price of live, trustworthy scanning surfaces, and bounded
by only re-emitting on an actual change of the aggregated value.

**User-level test path.** With a task working in the background, confirm a
freshly loaded page and a second tab both show background-running
immediately (no manual refresh). Drive a generating→background flip and
confirm the board card and list update without a coarse state change.
Restart the backend mid-work and confirm no surface flips to "done" while
the session's turn is still open.

§req:success-criteria (#5, #6, #7) · §req:quality-attributes ·
§req:priorities (must-have #2, #3)

---

## Guard against destroying a running task §spec:destructive-action-guard

*Status: complete — archive and delete confirmations warn when the same
task-level truth shown by the indicators says work is still running. The
operator-approved archive-confirmation bypass remains unchanged; delete is
always guarded.*

When the operator attempts to archive or delete a task that still has work
running — any session generating or running recognized background work,
i.e. the same "in-flight" truth the indicators show — the confirmation the
operator sees carries a clear, prominent warning that the task is still
working, before the destructive action proceeds. The warning is derived
from the same source of truth as the indicators, so a task the board shows
as background-running warns on archive/delete for exactly that reason.

**Decision and the constraint that drove it.** The guard is expressed as a
**warning added to the existing archive and delete confirmation dialogs**,
and it respects the user's existing archive-confirmation preference
(`confirmTaskArchive`): if the operator has turned that dialog off, archive
still proceeds without a prompt (operator decision, q1_opt2). The driving
requirement is §req:success-criteria #8 (a still-running task must not be
discarded by accident) and §req:priorities must-have #5. Delete has no
bypass and always confirms, so it is always guarded; archive can be
silenced, and a user who disabled archive confirmation has explicitly
opted into unconfirmed archiving. Reusing the existing dialogs (rather than
a new blocking modal or a hard block) keeps the guard a *warn-before-
proceed* — matching the requirement's wording ("warns the operator before
it proceeds") and preserving a deliberate "archive this, I don't care"
action.

**Residual risk (recorded, operator-accepted).** Because the guard honors
the `confirmTaskArchive` bypass, an operator who has disabled archive
confirmation can archive a still-running task with no warning — a partial
gap against the letter of §req:success-criteria #8 ("can't be discarded by
accident"). This was chosen with the tradeoff stated: it is the least
invasive option, delete remains always-guarded, and the bypass is an
explicit user opt-in. If the gap proves painful in practice, the escalating
variant (a running-work warning that fires even when the general
confirmation is off) is the natural follow-up.

**Alternatives considered.**

- *Escalate past the bypass — always warn while work runs.* Fully
  satisfies §req:success-criteria #8 but overrides a setting the user
  deliberately turned off; deferred per operator decision as more
  intrusive than warranted now.
- *Hard-block archive/delete until the task is stopped.* Rejected:
  stronger than the requirement's "warn before it proceeds," and it blocks
  a legitimate deliberate archive of a running task.
- *A guard computed independently of the indicators' state.* Rejected:
  it could warn when the board shows done, or fail to warn when the board
  shows working — the guard must agree with what the operator sees
  (§req:quality-attributes: single source of truth).

**Tradeoffs.** Reading the in-flight state into the archive/delete flow
couples those actions to the busy signal; accepted because the guard's
correctness *depends* on agreeing with the visible state.

**User-level test path.** With a task running foreground or background
work, invoke archive and invoke delete: confirm each confirmation dialog
shows the "still working" warning. Stop the task and confirm the warning
is gone. With `confirmTaskArchive` disabled, confirm archive proceeds
without a prompt (documented behavior) while delete still confirms.

§req:success-criteria (#8) · §req:user-stories (operator cleaning up the
board) · §req:priorities (must-have #5) · §req:quality-attributes

---

# Foundation — busy-signal producer (largely landed, separate effort)

> These sections document the producer layer the surfacing above depends
> on: the executor-row / pause-resume effort (GitHub #1597) and the
> in-memory foreground-idle substate (ADR-0046). They are a separate,
> largely-landed effort; their `§req:` citations reference the prior
> requirements revision preserved in git history, not the current
> REQUIREMENTS.md. Retained here so the full producer→surface stack is
> visible in one place.

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
