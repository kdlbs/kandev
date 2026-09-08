---
status: draft
system: office
created: 2026-09-07
owners:
  - kandev
---

# Office Agent Recovery Requirements

## Overview

An Office agent in `paused` or `stopped` does no work: the scheduler refuses
its queued runs and refuses to wake it. Returning it to `idle` is already a
supported product operation — `PATCH /api/v1/office/agents/:id/status` accepts
the transition and the agent state machine lists `paused -> idle` ("user clicks
Resume") and `stopped -> idle` ("user reactivates") as user-driven transitions.

No user interface calls that endpoint. The nearest thing is the inbox "Mark
fixed" action, and it does not close this gap for two separate reasons. It
reaches exactly one population — an agent auto-paused after consecutive
failures, whose inbox entry still exists and has not been dismissed — so every
other way an agent reaches `paused` or `stopped` (a budget guard, a direct API
call, an operator who already dismissed the inbox entry) it never sees. And for
the population it does reach, it writes the agent's existing status back
unchanged: it clears the pause reason, resets the failure counter and dismisses
the entries, but leaves the agent `paused`. It also attempts to re-queue the
affected runs, and every one of those attempts fails, because a run cannot be
queued for an agent that is `paused`.

So today there is no path at all, for any agent, from `paused` or `stopped` back
to `idle` that does not involve hand-writing an HTTP request against the
backend.

This capability adds that affordance: an operator-facing control on the Office
agent detail surface that returns a `paused` or `stopped` agent to `idle`.

The Office system owns this contract because it owns Office agent identity, the
agent status field, and the scheduler behavior that status gates. Per the
vertical-ownership rule, user visibility does not move the contract to the UI
system: there is no reusable presentation contract here, only the visible form
of an Office agent lifecycle transition.

## Prior art

**Wiki leg — DID NOT RUN, tool absent.** Searched for the `wiki-query` skill and
for a vault to point it at: `/Users/neo/.claude/skills/` contains no `wiki*`
entry, `/Users/neo/.obsidian-wiki` does not exist (so there is no `config`
symlink to resolve), no `OBSIDIAN_VAULT_PATH` or equivalent is set in the
environment, and no `qmd` MCP server is registered in this session. There is
therefore no resolved vault path or QMD collection to report, and no degraded
`grep` fallback either. This is an unavailable tool, not an empty result.

**saas-kb leg — DID NOT RUN, tool absent.** The `saas-kb` MCP server is not
registered in this session; the only MCP server exposed is `kandev`, so
`search_fsm_docs` (and its `category: "ai_sdlc"` filter) could not be called. No
competitive scan was performed.

**In-repo prior reasoning — ran, and it was decisive.** Searched
`docs/specs/office/**` and `docs/decisions/**` for the capability's nouns
(paused, stopped, unpause, resume, recover). Kandev has already taken a position
here: the Office agents system design records the agent state machine with
`paused -> idle` triggered by "user clicks Resume" and `stopped -> idle` by
"user reactivates", both with the actor recorded as **user**
([agents-01](../system-design/agents-01.md), "State machine"). The transitions
were built and the operator-facing half was not. This capability is not a new
product position; it is the missing half of one already taken, which is why the
spec adds no transition and changes no rule.

Two adjacent contracts were checked for overlap and found to be narrower rather
than duplicative. [Runtime](runtime.md) states that re-runs of a failed agent
happen only via explicit user action — "Resume session" in chat, "Mark fixed" on
an inbox entry, or reassignment — and [inbox](inbox.md) scopes "Mark fixed" to a
live `agent_paused_after_failures` entry. Neither covers an agent that reached
`paused` or `stopped` by any other route, which is the gap this capability
closes.

**What we are doing differently.** The one adjacent flow bundles three effects
together — counter reset, inbox dismissal, an attempted run re-queue — and does
not change status at all. This capability is its exact complement: it changes
status and does nothing else. Bundling the two would mean an operator
reactivating a manually stopped agent silently re-queues failed work they never
asked to retry. Separating "put this agent back in service" from "treat these
failures as resolved" is the substantive departure, and the "Consequences of
the named exclusions" section below states what the operator gives up by using
the narrower action, and in what order the two are best used.

## Terminology

- **Recoverable status:** an agent status from which this capability offers a
  return to `idle`. Exactly `paused` and `stopped`.
- **Recovery control:** the operator-facing control this capability adds. It is
  one affordance covering both recoverable statuses, carrying one name for both
  (`AC-OFFICE-AGENT-RECOVERY-001.9`), not a pair of status-specific controls.
- **Pause reason:** the free-text `pause_reason` field carried on the agent,
  written by whatever moved the agent out of service.
- **Acting client:** the browser session whose operator activated the recovery
  control.
- **Auto-pause recovery:** the existing inbox "Mark fixed" action on an
  `agent_paused_after_failures` entry. It clears the pause reason, resets the
  consecutive-failure counter, dismisses inbox entries, and attempts to re-queue
  runs. It does not change the agent's status, and because it does not, those
  re-queue attempts are refused. Its name notwithstanding, it is a
  failure-bookkeeping action, not a recovery of the agent's ability to work.

## Requirements

### REQ-OFFICE-AGENT-RECOVERY-001: Operator recovery of a paused or stopped agent

**Intent:** Give an operator a first-class way to put a `paused` or `stopped`
Office agent back into service, so that recovering an out-of-service agent does
not require a hand-written backend request.

**User story:** As an operator, I want to return a paused or stopped Office
agent to `idle` from the agent's own page, so that it resumes accepting
scheduled work without my leaving the product.

#### Acceptance criteria

- **AC-OFFICE-AGENT-RECOVERY-001.1:** When an operator views an Office agent
  whose status is `paused` or `stopped`, the system shall present a recovery
  control on the agent detail surface, on every one of that agent's detail
  sub-routes.
- **AC-OFFICE-AGENT-RECOVERY-001.2:** When an operator views an Office agent
  whose status is any value other than `paused` or `stopped` — including `idle`,
  `working`, `pending_approval`, an empty status, and a value the client does
  not recognise — the system shall not present the recovery control.
- **AC-OFFICE-AGENT-RECOVERY-001.3:** When an operator activates the recovery
  control, the system shall request the target status `idle` for that agent and
  shall request no other field change.
- **AC-OFFICE-AGENT-RECOVERY-001.4:** When the recovery request succeeds, the
  system shall render the status the server returned for that agent, and shall
  not render a status the acting client predicted before the response arrived.
- **AC-OFFICE-AGENT-RECOVERY-001.5:** When the recovery request succeeds and the
  server returns `idle`, the acting client shall stop presenting the recovery
  control for that agent without requiring a page reload or a manual refresh.
- **AC-OFFICE-AGENT-RECOVERY-001.6:** When an agent's status is `paused` or
  `stopped` and its `pause_reason` is a non-empty string, the system shall
  display that text to the operator on the agent detail surface. When the
  agent's status is any other value, or `pause_reason` is empty, the system
  shall display no pause-reason text. The pause reason is shown under the same
  status gate as the recovery control so that the explanation and the remedy
  appear and disappear together; a `pause_reason` left on an agent in another
  status is stale bookkeeping, not something the operator can act on here.
- **AC-OFFICE-AGENT-RECOVERY-001.7:** When a recovery request succeeds, the
  agent's `pause_reason` shall be empty, and the acting client shall stop
  displaying the previous pause-reason text.
- **AC-OFFICE-AGENT-RECOVERY-001.8:** When an operator recovers an agent, the
  system shall not change that agent's consecutive-failure count, shall not
  dismiss any inbox entry, and shall not queue any run.
- **AC-OFFICE-AGENT-RECOVERY-001.9:** The recovery control shall carry an
  accessible name that identifies the action, and shall be reachable and
  operable by keyboard. That name shall be a **single string, identical for both
  `paused` and `stopped`** — the control shall not vary its label by source
  status. The name shall not be "Resume" or "Reactivate" alone: the agent state
  machine binds each of those verbs to exactly one of the two source statuses
  ("user clicks Resume" for `paused`, "user reactivates" for `stopped`), so
  either one, used for both, mislabels half the cases. Wording built on the
  return-to-service sense this requirement is written in satisfies this; the
  exact copy is an implementation choice within that constraint.

### REQ-OFFICE-AGENT-RECOVERY-002: Recovery request outcomes

**Intent:** An operator recovering an agent is usually reacting to something
already broken. The control must never leave them unable to tell whether the
agent came back, and must never report a recovery that did not happen.

#### Acceptance criteria

- **AC-OFFICE-AGENT-RECOVERY-002.1:** While a recovery request for an agent is
  in flight, the system shall not accept a further activation of that agent's
  recovery control, and shall indicate that the request is in progress.
- **AC-OFFICE-AGENT-RECOVERY-002.2:** When a recovery request fails, the system
  shall surface an operator-visible error, shall leave the displayed status and
  pause reason as they were before the request, and shall accept a further
  activation of the recovery control.
- **AC-OFFICE-AGENT-RECOVERY-002.3:** When a recovery request is repeated for an
  agent that is already `idle`, the system shall report success and shall leave
  the agent in `idle`.
- **AC-OFFICE-AGENT-RECOVERY-002.4:** When two operators recover the same agent
  concurrently, the system shall leave that agent in `idle`, and shall report
  success to both.
- **AC-OFFICE-AGENT-RECOVERY-002.5:** When an agent's status changes between the
  moment the acting client rendered it and the moment the operator activates the
  recovery control, the system shall apply the operator's request against the
  agent's current server-side status and shall render the resulting status,
  rather than the status the client had rendered.
- **AC-OFFICE-AGENT-RECOVERY-002.6:** When the requested transition is refused,
  or the target agent cannot be resolved, the system shall treat the request as
  failed under AC-OFFICE-AGENT-RECOVERY-002.2 and shall not report success.

### REQ-OFFICE-AGENT-RECOVERY-003: Recovery reach

**Intent:** A recovery affordance that an operator cannot navigate to is not an
affordance. An out-of-service agent must remain findable.

#### Acceptance criteria

- **AC-OFFICE-AGENT-RECOVERY-003.1:** The Office agent list shall include agents
  whose status is `paused` or `stopped`, and shall offer navigation from each to
  that agent's detail surface.
- **AC-OFFICE-AGENT-RECOVERY-003.2:** When an operator recovers an agent, no
  Office permission beyond the one already required to view that agent's detail
  surface shall be required.

## Ordering, concurrency, and determinism

This capability acts on one agent row at a time and presents no list of its own,
so it defines no sort order and no tiebreak. The only ordering it constrains is
between the request and what the operator sees: the displayed status follows the
server's response and never precedes it
(AC-OFFICE-AGENT-RECOVERY-001.4).

The requested target is the constant `idle` rather than a value derived from the
status the client last rendered. A repeat request is therefore a no-op rather
than a conflict (AC-OFFICE-AGENT-RECOVERY-002.3), two concurrent operators
converge on the same result (AC-OFFICE-AGENT-RECOVERY-002.4), and a request
built against a stale render still expresses the operator's intent
(AC-OFFICE-AGENT-RECOVERY-002.5).

One other writer of this field is not a recovery and does not converge with one.
Auto-pause recovery reads the agent's status and writes that same value back, so
a "Mark fixed" that began before a recovery landed can complete after it and
restore the earlier `paused`. The agent is then `paused` with an empty pause
reason, a state this capability's own request never produces. No compare-and-set
exists on the status endpoint and this capability adds none, and the acting
client is not told of the later write, so it keeps showing `idle` and keeps the
control hidden per `AC-OFFICE-AGENT-RECOVERY-001.5`. The control returns
whenever that agent is next read from the server, and activating it again is the
operator's remedy. Removing the stale write belongs to the flow that performs it
(see "Out of scope").

## Consequences of the named exclusions

Three follow from `AC-OFFICE-AGENT-RECOVERY-001.3`,
`AC-OFFICE-AGENT-RECOVERY-001.7` and `AC-OFFICE-AGENT-RECOVERY-001.8`. They are
intended, not oversights. An implementation must not suppress any of them: the
actions this control may take are fixed by 001.3 (request `idle` and no other
field change) and 001.8 (no counter write, no dismissal, no queued run), and
each consequence below is downstream of those two.

- **The failure count survives.** An agent recovered by this control keeps its
  consecutive-failure count. If that count is already at or above the failure
  threshold, the agent's next failure re-pauses it immediately. Recovering an
  agent whose underlying fault is unfixed buys one attempt, not a reset.

- **An auto-pause inbox entry does not survive, and zero or more per-run
  entries come back.** Office selects the consolidated
  `agent_paused_after_failures` entry by matching the agent's `pause_reason`
  against the auto-pause prefix, and it suppresses the individual
  `agent_run_failed` entries for exactly the agents that match. Because a
  recovery clears `pause_reason` (`AC-OFFICE-AGENT-RECOVERY-001.7`), recovering
  an auto-paused agent removes the consolidated entry and lifts that
  suppression. How many per-run entries then appear is **not fixed at one per
  previously failed run**: both inbox queries independently exclude entries that
  already carry a dismissal record, so a run whose entry was dismissed earlier —
  by the operator directly, or automatically by a prior "Mark fixed" on the
  consolidated entry — stays hidden. The count is therefore zero or more, and it
  is zero whenever every per-run entry was already dismissed. No acceptance
  criterion fixes that count, so nothing here can be tested against one or worded
  as if it were. This is a side effect of clearing the pause reason, not a
  dismissal — the control writes no dismissal row, which is what
  `AC-OFFICE-AGENT-RECOVERY-001.8` requires. This matches
  [inbox](inbox.md), which already records that these rows leave the inbox when
  the agent is un-paused.

- **"Mark fixed" on the consolidated entry stops applying; the per-run "Mark
  fixed" does not.** Auto-pause recovery identifies its target by the same
  `pause_reason` prefix, so once this control has cleared it, the consolidated
  action finds nothing to act on and returns without resetting the counter and
  without attempting a re-queue — and the entry that offered it is gone from the
  inbox anyway. The per-run `agent_run_failed` entries are a **separate**
  action: each carries its own "Mark fixed", which dismisses that entry and, when
  the run still exists and its payload names a task, re-queues that run. A run
  that has since vanished, or one whose payload names no task, is dismissed with
  nothing queued. Recovery does not disable this action, and where it does
  re-queue, the run is now accepted rather than refused, because the agent is
  `idle`.

**Recovery first. The other order destroys the work it appears to retry.**
For an agent auto-paused after consecutive failures, where the operator wants
both the failures treated as resolved and the agent working again, this control
should be used **first**. Used first, it returns the agent to `idle` and
un-suppresses whichever per-run `agent_run_failed` entries survive, each of
which still carries its own "Mark fixed"; where that action re-queues at all
(above), the run is now accepted, because the agent is `idle`. What
recovery-first gives up is the **one-click counter reset**, and the counter is
not stuck by that: it also clears on the agent's next successful turn.

In the other order, the consolidated "Mark fixed" dismisses every per-run entry
permanently — a dismissal is never undone — while leaving the agent `paused`,
so each re-queue it then attempts is refused and nothing is queued. That leaves
the operator with a reset counter, an empty inbox, no queued work, and no
per-run entry left to retry from. This is a defect in that flow, which this
capability neither causes nor repairs; it is recorded here because it is the
reason the ordering guidance reads the way it does.

For every other route into `paused` or `stopped` — a budget guard, a direct API
call, an operator who already dismissed the inbox entry — "Mark fixed" never
applied in the first place and this control is the only action needed.

## Out of scope

- **Pausing or stopping an agent from the UI.** This capability covers the
  return to service only. The opposite direction is a separate decision about
  what an operator may take out of service and with what confirmation, and it is
  not needed to close the recovery gap.
- **`pending_approval -> idle`.** That transition means "the hire was approved"
  and is owned by the Office approvals flow, which already has its own operator
  surface. Presenting it as a recovery would let an operator approve a hire
  through a control that does not describe what it is doing.
- **Resetting the consecutive-failure counter, dismissing inbox entries, and
  re-queueing runs.** Owned by the auto-pause recovery flow described in
  [inbox](inbox.md) and [runtime](runtime.md). See "Consequences of the named
  exclusions" above for what this means in practice.
- **Repairing the auto-pause recovery flow.** Two defects in it are described
  above: it writes back a status it read earlier, which can revert a concurrent
  recovery, and it dismisses per-run entries permanently while its own unchanged
  `paused` status causes every re-queue it attempts to be refused. Both live in
  that flow, not in this control, and either fix changes behavior this
  capability does not own.
- **Live propagation of the recovered status to other clients.** The status
  endpoint publishes no event, so another operator's already-open view keeps the
  stale status until it refetches for some other reason. This capability
  constrains the acting client only
  (AC-OFFICE-AGENT-RECOVERY-001.5). Making the endpoint broadcast is a backend
  contract change and is deliberately not bundled with adding the control.
- **Widening or narrowing the agent status transition rules.** This capability
  consumes the transitions the Office agent contract already allows and does not
  change which transitions are legal.
- **Recovering an agent's in-flight sessions, runs, or task assignments.**
  Returning an agent to `idle` makes it eligible for future work. It does not
  resurrect work that already failed.
- **A bulk or multi-agent recovery action.** One agent at a time.
