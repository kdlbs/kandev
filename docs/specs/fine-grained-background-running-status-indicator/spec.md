# Fine-grained background-running status indicator

The system already tracks, per session, a fine-grained substate that
distinguishes an actively-generating foreground turn from a foreground turn that
has gone idle and is only held open by spawned background work — a subagent, a
backgrounded shell, or an active monitor (the busy signal recorded in
ADR-0047). This spec makes the operator-facing status indicators as
fine-grained as that signal: everywhere a task or session status appears, the
"still working in the background" condition reads as its own state, distinct
from both "actively generating" and "done".

Scope note: this effort covers the kanban / task status surfaces the
requirements enumerate. The Office area (autonomous-agent management) keeps its
own status vocabulary and is explicitly out of scope here; extending the
distinction into Office is a possible follow-up, not part of this work.

## Three-state status vocabulary §spec:three-state-vocabulary

*Status: complete*

<!-- The shared vocabulary is established and adopted on every in-scope surface:
getSessionStateIcon / getTaskStateIcon (apps/web/lib/ui/state-icons.tsx) are the
single source of the background-running affordance, distinct from generating and
done by shape+motion (not hue alone). Both the session-level surfaces
(§spec:session-level-indicator) and every task-level surface
(§spec:task-level-indicator) now render it, so the three-state reading is
consistent cross-surface. Live propagation / fresh-load correctness is tracked
separately in §spec:live-propagation-fallback. -->


Every status surface in scope communicates three mutually distinguishable
conditions for an in-flight unit of work:

- **generating** — the foreground agent is actively producing output. This is
  the established "it's running" affordance, unchanged.
- **background-running** — the foreground turn is idle but recognized spawned
  work (subagent / backgrounded shell / active monitor) is still running.
- **done** — no work is in progress.

The background-running affordance is visually separable from *both* the
generating affordance *and* the done affordance, and the separation does not
rely on hue alone: an operator distinguishes it by shape or motion, so it
remains legible for color-vision-deficient operators and in a dense scan.

**Decision and the constraint that drove it.** The three states are defined as a
shared *meaning*, but each surface expresses that meaning within its own
existing visual language rather than every surface being forced onto one
identical icon set. This is driven by a hard tension in the requirements: the
same underlying state must carry the same meaning everywhere, yet the
established generating affordance must not be restyled — and today there is not
one established generating look but several (a spinner, a ring-of-dots, a static
dot, depending on surface). Those two constraints cannot both be fully honored
if "same meaning" is read as "pixel-identical". The resolution is that each
surface keeps its current generating and done affordances untouched and *adds* a
background-running affordance that is distinct on that surface. Consistency is
guaranteed at the level that matters to the operator — the same underlying
condition always produces a not-done, not-generating "still working" reading on
every surface — without a disruptive restyle of indicators operators already
recognize.

**Additive, not a re-signal.** The background-running affordance communicates
only "work is still in progress." It deliberately does not signal "you may now
type" — composer input-gating is a separate concern (ADR-0047) and is not
represented by this indicator.

**Alternatives considered.**

- *Converge every surface onto one canonical three-state icon set.* Rejected:
  it delivers the strongest cross-surface identity but visibly restyles several
  current running indicators (including the session-tab ring-of-dots and the
  task-level spinner), which both enlarges the change and violates the
  "generating affordance unchanged" constraint for whichever surfaces get
  replaced. The consistency gain over "same meaning, per-surface expression" is
  not worth that blast radius for a change whose goal is truthfulness, not a
  visual redesign.
- *Distinguish background only by color* (e.g. a different-hue spinner).
  Rejected: fails the not-color-alone quality attribute; two spinners that
  differ only in hue are not reliably distinguishable in a dense scan or for
  color-vision-deficient operators.

**Tradeoffs.** The background-running icon is not guaranteed to be identical
pixel-for-pixel across surfaces; the guarantee is semantic (never done, never
mistaken for generating), not pictorial. This is an accepted cost of leaving the
established affordances in place.

**User-level test path.** Trigger a session into the background-running
condition (agent replies, then a monitor or backgrounded shell keeps running).
On each in-scope surface, confirm the indicator differs from that surface's
generating indicator and from its done indicator, and that the difference
survives a grayscale/desaturated view.

§req:success-criteria §req:quality-attributes §req:constraints

## Task-level indicator reflects live background work §spec:task-level-indicator

*Status: complete*

<!-- Backend task-record aggregate (most-active-wins) + live emission on
generating↔background flips drive all four at-a-glance task surfaces: the
board/kanban card, the task-list rows, the graph/swimlane nodes, and the
open-task header now read the aggregate-aware getTaskStateIcon (icon) and, for
the open-task header, a matching text badge — so none can read done while a
session still has background work outstanding. -->


On every at-a-glance task surface — the board / kanban card, the task list rows,
the graph / swimlane nodes, and the open-task header — a task whose foreground
turns are idle while spawned background work is still running shows the
background-running affordance, not a done affordance and not the generating
affordance. No task-level surface shows "done" while any session the task owns
still has recognized background work outstanding.

For a task running more than one session at once, its single task-level
indicator follows **most-active-wins**: it shows generating if any session is
generating; it shows background-running only when no session is generating but at
least one has background work running; it falls through to today's behavior
(done / waiting / failed, per existing rules) only when neither is true. The
background-running tier is inserted between generating and done; it does not
redefine how the other task states render.

**Decision and the constraint that drove it.** The task-level three-state
condition (including the most-active-wins aggregation across a task's sessions)
is computed on the backend and delivered as part of the task record — alongside
the existing primary-session status already carried there — so that every
task-level surface reads one authoritative value. This is driven by how these
surfaces get their data: the board, list, graph, and header render from the task
record (initial page payload plus task-level update events) and do not hold the
full set of a task's sessions for tasks that are not open. Deriving the
aggregate on the client would therefore be blank or stale for exactly the
off-screen tasks an operator is scanning. A backend-computed, task-record-borne
value is correct on first paint, in a second tab, and for tasks the client has
never expanded — and it keeps the surfaces from disagreeing, because they all
read the same field rather than each re-deriving it.

**Change from today's aggregation.** Task-level surfaces today variously reflect
only the *designated primary* session, or a most-active ranking, or the
most-recent live session — three different derivations feeding different
surfaces. This capability makes the three-state reading uniformly
most-active-wins. A consequence the operator will observe: a task whose primary
session has finished but whose secondary session is still generating or running
background work now reads as working, where before a primary-only surface could
have read done. This is the intended behavior — a task is not "done" while any
of its sessions is still working.

**Alternatives considered.**

- *Derive the task-level aggregate on the client from the session store.*
  Rejected: the store is not guaranteed to hold every visible task's full
  session set, so off-screen task rows and cards — the primary scanning surface
  — would show blank or stale background state and could disagree with the
  open-task view. It also re-introduces the multi-source-of-truth problem this
  work is meant to end.
- *Keep primary-session-only aggregation and just add the background tier to
  it.* Rejected: it would still falsely read "done" for a task whose primary
  session finished while a secondary session runs background work, which
  directly violates the "never done while work continues" success criterion.

**Tradeoffs.** Most-active-wins means a task-level surface can now reflect a
non-primary session's activity, which is a behavior change some operators may
notice; it is accepted because it is precisely what makes the indicator
truthful. Carrying the aggregate on the task record means a task-level update is
emitted when a session's activity flips between generating and background; this
added update traffic is accepted as the cost of keeping scanning surfaces live
and consistent (see §spec:live-propagation-fallback).

**User-level test path.** With a single-session task, drive it
generating → background-running → done and confirm the board card, task list
row, graph/swimlane node, and open-task header each reflect all three in turn
without opening the task. With a two-session task, confirm the task indicator
shows generating while either session generates, background-running when one
session is only running background work and none is generating, and done only
when both are finished.

§req:success-criteria §req:user-stories §req:priorities §req:quality-attributes

## Session-level indicators surface the substate uniformly §spec:session-level-indicator

*Status: complete*

Every surface that shows a per-session status reflects the same three states.
The session switcher already does. The session-reopen menu — which today shows
no indicator at all for a running session and drops the fine-grained substate —
shows the background-running state distinctly from generating and from done. The
session tab / global "something is running" indicator distinguishes a session
that is only running background work from one that is actively generating, and
neither reads as done.

**Decision and the constraint that drove it.** Session-level surfaces read the
per-session substate that the underlying signal already emits and already places
on the session record (initial payload plus the per-session activity event), and
render it through the same three-state vocabulary as every other surface. The
driving constraint is the requirement's core grievance: the signal is already
produced end-to-end and one surface renders it correctly, yet the peer surfaces
ignore or drop it, so the operator cannot trust any single icon. Bringing every
session-level surface onto the substate it is already being handed removes the
surface-to-surface disagreement without new mechanism.

**Alternatives considered.**

- *Leave session-level surfaces as-is and only fix task-level ones.* Rejected:
  the session-reopen menu and session tabs are exactly where an operator lands
  after scanning, so a task that reads "working" at the board level but "done"
  (or blank) in the reopen menu re-creates the contradiction one level down.

**Tradeoffs.** The session tab and reopen menu gain a state they did not
previously render, a small additional visual element in already-dense controls;
this is accepted because their silence today is precisely the defect.

**User-level test path.** With a session in the background-running condition,
open the session-reopen menu and the session tab and confirm each shows the
background-running affordance — distinct from generating and never a done check
— matching what the session switcher shows for the same session.

§req:success-criteria §req:user-stories §req:constraints

## Live propagation, fresh-load correctness, and safe fallback §spec:live-propagation-fallback

*Status: complete*

As a session moves between generating, background-running, and done, every
in-scope surface — session-level and task-level — updates promptly without a
manual refresh. A freshly loaded page and a second tab show the correct state
immediately rather than a stale value that only corrects on the next transition.
When the fine-grained substate is unknown or unavailable, no surface falsely
reads "done" while a turn is still open.

**Decision and the constraint that drove it.** Transitions are pushed live:
per-session substate flips propagate to session-level surfaces, and because the
task-level aggregate is carried on the task record, a session's activity flip
also refreshes the task-level surfaces subscribed to that task. The initial page
payload and the session/task records carry the current substate so first paint
and additional tabs are correct without waiting for a transition. This is driven
by the requirement that a stale "done" that clears only on refresh is itself a
defect: the indicator's whole value is being trusted at a glance, so it must be
correct at the moment of the glance, including immediately after load.

**Safe fallback.** The fine-grained substate is in-memory and best-effort by
design (ADR-0047); after a backend restart the in-memory tracker resets. The
fallback is chosen so that an unknown substate never resolves to "done" while a
turn is open: an in-flight session whose substate is unknown reads as working
(generating), not done, and a task with such a session does not read done.
Correctness of "not falsely done" is never traded for the optimization of the
finer distinction.

**Alternatives considered.**

- *Persist the fine-grained substate so it survives restart.* Rejected
  (consistent with ADR-0047): a persisted copy becomes a second source of truth
  that can drift, can survive a restart as a false reading for a session whose
  turn is already gone, and adds write churn on the hot streaming path. The
  substate only matters for a live in-flight turn; resetting to the safe default
  on turn-close is sufficient and simpler.
- *Refresh task-level surfaces only on coarse state changes, not on
  activity flips.* Rejected: a generating→background flip does not change the
  coarse state, so the scanning surfaces would keep showing the wrong one of the
  three states until an unrelated transition, reproducing the stale-until-
  refresh defect the requirements call out.

**Tradeoffs.** Emitting a task-level refresh on activity flips adds update
traffic proportional to how chatty background work is (e.g. a bursty monitor);
this is accepted as the price of live, trustworthy scanning surfaces, and is
bounded by only re-emitting on an actual change of the aggregated three-state
value.

**User-level test path.** With a task working in the background, confirm a
freshly loaded page and a second browser tab both show background-running
immediately (not done, no manual refresh). Drive a generating→background flip
and confirm the board card and task list update without a coarse state change.
Restart the backend mid-work and confirm no surface flips to "done" while the
session's turn is still open.

§req:success-criteria §req:quality-attributes §req:constraints
