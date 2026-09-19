---
status: draft
system: office
created: 2026-09-17
owners:
  - kandev
---

# Office: Assignment Wake Rate Limit Requirements

## Overview

An Office agent holding `can_assign_tasks` — the default `worker` role
permission — can call the dashboard's assignee mutator repeatedly with itself as
the target. Each call commits a new `tasks.assignment_generation`, so each
produces a genuinely new wake occurrence with a distinct dedup key. Spaced past
the coalescing window, the agent mints an unbounded stream of never-suppressed
`task_assigned` runs against its own task.

Two mechanisms look like they should bound this and do not.

**Deduplication does not**, by design. `AC-OFFICE-RUN-DEDUP-001.3` *requires* a
second assignment to the same agent to insert a run: a key must suppress a
redelivery and must not suppress a repeat. Every wake in this stream is a
repeat.

**The spend ceiling does not**, and this is the load-bearing finding.
`ClassifyRunProvenance` (`AC-OFFICE-BUDGET-007.1`) is a total function of
`reason` alone, and `task_assigned` sits on its attended allowlist because a
*person* assigning a task is plausibly waiting on it. Attended runs are exempt
from operator policies and from the default ceiling, and a reason-only
classifier cannot tell an agent actor from a human one, so these runs never
reach it. The spend is unbounded, not loosely bounded.

This document owns the bound: a per-task allowance on agent-initiated
assignment wakes, a separate guard rather than a change to coalescing.

## Terminology

- **Agent-initiated assignment wake:** a `task_assigned` wake for a mutation
  whose actor type is `agent`; defined normatively by
  `AC-OFFICE-ASSIGN-RATE-001.1`.
- **Allowance:** the admitted agent-initiated assignment wakes permitted for one
  task in one rolling window.
- **Admitted:** a wake that inserted a `runs` row. One suppressed by dedup or
  merged by coalescing is neither admitted nor refused; an earlier gate decided
  it.
- **Refused:** a wake this capability declined, inserting no row and merging into
  none.

## Prior art

### Our own recorded reasoning (wiki)

**Searched:** QMD collection `wiki` (564 documents, re-indexed 2026-09-17),
semantic query with reranking off. A semantic result, not a degraded keyword
scan.

[[agent-budget-governance]] states the position this capability departs from: an
operator-initiated agent is bounded by the operator's attention, a cron- or
webhook-triggered one by nothing except a budget. Its companion
[[event-triggered-agent-activation]] frames the axis as *who starts it*.

Our case is a third kind neither page enumerates: **agent-initiated** activation
that the classifier deciding whether a budget applies cannot tell from
operator-initiated. It has an actor, so Kandev reads it as attended and bounded
by an attention nobody is paying. That is why the budget is not the bound here.

Two of its anti-patterns are adopted: an org cap without a per-principal cap
(Kandev's default ceiling is exactly an org cap, this adds a narrower per-task
one, and the missing aggregate is named under `## Out of scope`), and treating a
cap as cost control when it is also the blast radius of an agent's autonomy
(queue rows, scheduler churn and session launches are blast radius no spend
ceiling touches). We depart from its window shape, where the corpus converges on
`calendar_month_utc`, because a calendar window lets an attacker spend a full
allowance either side of the boundary.

### What other products shipped (saas-kb)

Recorded in the system design's `## Prior art considered`, beside the choices it
informs: a count cap rather than a cooldown, and why the near-universal
per-principal concurrency cap is not this axis.

### Prior art inside this repository

Three precedents shape the criteria below: Office automations'
`max_concurrent_runs` cap, the workspace pause gate's refused admission, and
`checkIdleSkip`'s open failure, which `AC-OFFICE-ASSIGN-RATE-002.2` follows and
the pause gate deliberately does not. Call sites and the contrast are in the
system design.

## Requirements

### REQ-OFFICE-ASSIGN-RATE-001: A per-task allowance bounds agent-initiated assignment wakes

**Intent:** A repeat assignment must still wake the agent, so the bound cannot
suppress the class. It bounds how often the class may fire, scoped so a human
operator is never throttled.

**User story:** As an Office operator, I want an agent that can assign tasks to
be unable to mint an endless stream of runs against a task, so that an
unattended workspace cannot spend without limit while nobody is watching.

#### Acceptance criteria

- **AC-OFFICE-ASSIGN-RATE-001.1:** An *agent-initiated assignment wake* is a
  `task_assigned` wake produced for a task mutation whose actor type is exactly
  `agent`. A wake carrying any other actor type — `user`, an unrecognised value,
  or no actor type at all — is not one, and no criterion here applies to it: it
  shall be admitted without consuming an allowance, never refused, and shall
  move none of the counters in `REQ-OFFICE-ASSIGN-RATE-003`. The internal and administrative caller that
  supplies an empty acting agent identifier is classified `user`, so it is
  never limited. An absent actor type is a normal classification outcome, not a
  degraded one: the out-of-scope producers `AC-OFFICE-ASSIGN-RATE-001.12` names
  carry none.
- **AC-OFFICE-ASSIGN-RATE-001.2:** The system shall admit at most `N`
  agent-initiated assignment wakes for one task within any rolling window of
  duration `W` in serial evaluation. Concurrent evaluation may exceed `N`;
  see AC-OFFICE-ASSIGN-RATE-002.1.
- **AC-OFFICE-ASSIGN-RATE-001.3:** The allowance shall be scoped to the task
  alone. Wakes for one task count against one allowance whichever agent is being
  assigned, so alternating assignment between two agents shall not yield two
  allowances.
- **AC-OFFICE-ASSIGN-RATE-001.4:** Only an admitted wake shall consume the
  allowance. A refused wake shall not consume it: each admitted wake frees its
  share of the allowance one window after its own request instant, however many
  refusals fell in between, subject to `## Accepted consequence`'s coalescing
  relabel.
- **AC-OFFICE-ASSIGN-RATE-001.5:** The allowance shall be evaluated after the
  recent-duplicate lookup and after the coalescing attempt, and before the run
  row is inserted. A wake that an earlier gate already suppressed shall be
  neither refused by this one nor counted against the allowance, subject to
  `## Accepted consequence`'s coalescing relabel.
- **AC-OFFICE-ASSIGN-RATE-001.6:** When the allowance is exhausted, the system
  shall refuse the wake: insert no run row, merge into no existing run, and
  report neither a queued nor a coalesced nor a deduplicated outcome for it.
- **AC-OFFICE-ASSIGN-RATE-001.7:** A refusal shall not abort or roll back the
  mutation. The assignee change stays persisted, the assignment generation stays
  bumped, and the mutation's other effects — the previous assignee's session
  interrupt, that assignee's office session row transition, and the task-updated
  publication — shall still occur. This mutation produces no
  comment wake and no mention wake, so none is required here and a regression
  test for this criterion shall not assert one.
- **AC-OFFICE-ASSIGN-RATE-001.8:** Within the producer scope fixed by
  `AC-OFFICE-ASSIGN-RATE-001.12`, the allowance shall apply whether or not the
  wake carries a dedup key. A wake enqueued keyless because its generation could
  not be resolved, per `REQ-OFFICE-RUN-DEDUP-003`, is still bounded, so the
  keyless path is not an unbounded bypass of this allowance.
- **AC-OFFICE-ASSIGN-RATE-001.9:** An unassignment — a mutation whose new
  assignee identifier is empty — produces no assignment wake, and shall
  therefore be neither refused nor counted against the allowance.
- **AC-OFFICE-ASSIGN-RATE-001.10:** The admission decision shall be a count of
  the wakes falling in the window and shall not depend on the order in which
  they fall within it. No ordering rule and no tiebreak between two wakes
  sharing one persisted request instant is part of this contract; window
  membership is fixed by `AC-OFFICE-ASSIGN-RATE-001.11` and the limit by
  `AC-OFFICE-ASSIGN-RATE-001.2`. A test that must identify *which* wake was the
  `N`th shall arrange distinct persisted request instants rather than rely on a
  tiebreak.
- **AC-OFFICE-ASSIGN-RATE-001.11:** The window shall be the half-open interval
  from the evaluation instant less `W`, exclusive, to the evaluation instant,
  inclusive. A wake whose request instant is exactly one window old falls
  outside the window and shall not count.
- **AC-OFFICE-ASSIGN-RATE-001.12:** The boundary is payload shape, not a count
  of producers: the gate binds exactly those `task_assigned` wakes that reach it
  carrying actor type `agent`, and every criterion here is scoped to that shape.
  One producer does so today, the task-mutation reactivity path, the only one
  carrying the mutation's actor. No other producer of a
  `task_assigned` wake shall set actor type `agent`; that invariant shall be
  pinned by a test enumerating the producers and asserting the actor type each
  emits. Producers carrying no actor are out of scope per `## Out of scope`.
- **AC-OFFICE-ASSIGN-RATE-001.13:** A wake produced by the in-scope producer for
  a mutation whose actor is an agent shall carry the actor type `agent` on the
  wake itself, so the gate classifies it without re-reading the mutation.
  Losing that field would take every such wake out of scope under
  `AC-OFFICE-ASSIGN-RATE-001.1` and silently stop the allowance binding.

### REQ-OFFICE-ASSIGN-RATE-002: The allowance degrades safely and predictably

**Intent:** A cost guard that turns a transient database error into dropped
legitimate wakes trades a bounded cost problem for an unbounded liveness one.
Failure direction, race behaviour and constants must be stated, not discovered.

**User story:** As a Kandev maintainer, I want the allowance's failure and race
behaviour fixed in the contract, so that an implementation cannot pick the
convenient answer.

#### Acceptance criteria

- **AC-OFFICE-ASSIGN-RATE-002.1:** When two agent-initiated assignment wakes
  for one task are evaluated concurrently, each may observe an allowance with
  room and both may be admitted. The system shall permit over-admission bounded
  by the number of concurrent evaluations, and shall never refuse a wake before
  `N` have been admitted in the window.
- **AC-OFFICE-ASSIGN-RATE-002.2:** When the read deriving the current count
  fails, the system shall admit the wake and shall count and log that degraded
  admission under its own distinct reason. It shall not refuse the wake. The
  degraded admission shall be logged at warning level carrying the task
  identifier, the assignee agent profile identifier, the acting agent
  identifier, the allowance, and the underlying read failure; it shall not carry
  an observed count, which the failed read did not produce. This is
  deliberately the opposite of the workspace pause gate, whose closed failure is
  correct because a kill switch failing open is not a kill switch. The precedent
  is `checkIdleSkip`.
- **AC-OFFICE-ASSIGN-RATE-002.3:** When a wake's task cannot be determined, the
  system shall admit it and count it under a distinct unattributed reason, so a
  payload-shape change cannot silently disable the gate by making every wake
  unattributable. This case arises only for a wake already in scope
  under `AC-OFFICE-ASSIGN-RATE-001.1`: a wake out of scope by actor type is
  admitted there and never reaches this criterion, so a wake carrying neither an
  actor type nor a task is not counted here. A task cannot be determined when
  the identifier is absent, null, empty, or present but not a string; those
  shapes are treated identically.
  That admission shall be logged at warning level carrying the reason, the
  allowance, and the acting agent identifier when the wake carries one; it shall
  not carry a task identifier, which is by definition the value that could not
  be determined.
- **AC-OFFICE-ASSIGN-RATE-002.4:** The gate shall hold no state of its own: the
  count shall be derived from already-persisted run rows. A restart, a crash
  between evaluation and insert, or a deleted run row therefore changes the
  observed count without leaving the allowance inconsistent; no reconciliation
  step is required.
- **AC-OFFICE-ASSIGN-RATE-002.5:** Re-delivering one assignment occurrence that
  carries a dedup key shall not consume the allowance twice: redelivery is
  decided by the dedup identity, which `AC-OFFICE-ASSIGN-RATE-001.5` places
  before this gate. A keyless wake has no such identity, so re-delivering one
  does consume the allowance again — intended, because
  `AC-OFFICE-ASSIGN-RATE-001.8` bounds that path precisely where nothing
  upstream suppresses it. A test for this criterion shall use a keyed wake.
- **AC-OFFICE-ASSIGN-RATE-002.6:** `N` shall be 5 and `W` shall be 10 minutes,
  fixed values of this capability rather than operator-configurable, stated in
  one place the implementation and its tests share. `N` shall be at least 1; 0,
  which would refuse every agent-initiated assignment wake, is not permitted.

### REQ-OFFICE-ASSIGN-RATE-003: A refusal is never silent

**Intent:** A wake that vanishes without trace is indistinguishable from one
never produced, and an operator debugging a stalled task cannot tell. The
sibling deduplication capability set the floor at operator telemetry.

**User story:** As an Office operator, I want a refused assignment wake to be
visible in telemetry and logs, so that an agent not waking up is diagnosable
instead of mysterious.

#### Acceptance criteria

- **AC-OFFICE-ASSIGN-RATE-003.1:** Every refusal and every degraded admission
  shall increment a counter labelled by reason, drawn from a closed set of
  three values: allowance exhausted, count read failed, and task
  unattributed.
- **AC-OFFICE-ASSIGN-RATE-003.2:** No task, agent, workspace, session or run
  identifier shall ever be a counter label, so cardinality cannot grow with
  traffic.
- **AC-OFFICE-ASSIGN-RATE-003.3:** A refusal shall be logged at warning level
  carrying the task identifier, the assignee agent profile identifier, the
  acting agent identifier, the observed count and the allowance.
- **AC-OFFICE-ASSIGN-RATE-003.4:** An admitted wake this capability did not
  alter shall move none of these counters; ordinary admitted volume is already
  visible on the runs queue.
- **AC-OFFICE-ASSIGN-RATE-003.5:** The outcome reported to the producer for a
  refused wake shall be distinguishable from queued, coalesced and deduplicated
  outcomes, and shall not be the outcome meaning no enqueue was attempted. A
  caller shall not infer a rate-limit refusal from an absence.
- **AC-OFFICE-ASSIGN-RATE-003.6:** The counter shall be published on the same
  operator telemetry surface as the run-deduplication counters.

## Out of scope

- **The five-second coalescing window, and which reasons coalesce.** Held
  unchanged by repeated human decision on the run-dedup-generation card. This
  capability is a separate guard placed after it; a coalesced wake neither
  consumes the allowance nor can be refused by it.
- **The 24-hour recent-duplicate lookback and the durable unique index.**
  Unchanged. Both decide *identity*, which this capability never touches.
- **What makes a wake occurrence distinct.** `REQ-OFFICE-RUN-DEDUP-001` through
  `-004` own dedup key identity. This capability refuses wakes whose identity is
  genuinely new; it never redefines identity.
- **A per-acting-agent allowance.** [[agent-budget-governance]] and the vendor
  corpus both argue for a per-principal cap. Not adopted: it would throttle a
  coordinator legitimately assigning many *different* tasks in a burst, the
  behaviour Office's coordinator role exists to perform. Sizing one needs a
  legitimate-coordinator rate measured from real workspace data, which does not
  exist yet.
- **A workspace-aggregate launch budget, and a causation depth cap.** Card
  `7dbc0b94` owns work-in-progress limits, depth caps and self-trigger
  suppression. This is the per-task half only: across `T` tasks an agent retains
  `5T` admitted wakes per window, and only an aggregate budget bounds that. The
  two compose; neither substitutes for the other.
- **Reclassifying an agent-initiated `task_assigned` run as unattended.** The
  adjacent fix, deliberately not taken here: `AC-OFFICE-BUDGET-007.1` makes
  provenance a total function of `reason` and `-007.2` pins the attended
  allowlist to a literal set including `task_assigned`, so an actor-aware
  classifier changes `budget-run-provenance.md`'s contract and belongs to its
  owner. It is incomplete alone in any case — a daily ceiling permits a full
  day's allowance to burn in minutes and bounds no queue rows, session launches
  or scheduler churn.
- **Rate-limiting the assignment mutation itself.** The assignee mutator keeps
  accepting every call its permission check allows and the assignment generation
  keeps bumping; only the wake is refused. Refusing the write would make the
  board un-editable by agents.
- **A persisted record of a refused wake.** Office automations record a
  `skipped` row, the better surface in the abstract. Not adopted: `runs` has no
  such status and adding one to queue infrastructure shared by every workflow
  style is larger than this guard warrants. `REQ-OFFICE-RUN-DEDUP-004` set the
  floor for a high-frequency suppression outcome at operator telemetry and logs,
  which `REQ-OFFICE-ASSIGN-RATE-003` meets.
- **Notifying anybody.** No inbox item, approval, activity entry, task-timeline
  row or escalation. The counter added here is the evidence needed to size such
  a surface later.
- **Operator tunability of `N` and `W`.** They ship as fixed values, like the
  coalescing window. A settings surface, its API and its frontend are separate
  work.
- **A dedicated runtime feature toggle.** Office is already off in the prod
  profile, so this ships behind the flag gating the surface it guards. A second
  toggle inside a disabled feature buys no rollout safety.
- **Any frontend.** No screen renders the allowance, the counters, or a
  refusal.
- **Every other producer of a `task_assigned` wake.** Besides the in-scope
  mutation path the repository has four: the task-lifecycle event subscriber,
  onboarding, the unstarted-task recovery sweep, and the orchestrator's workflow
  auto-start action. Two reasons, not convenience. None carries
  an actor, so none is agent-initiated under `AC-OFFICE-ASSIGN-RATE-001.1`,
  which `AC-OFFICE-ASSIGN-RATE-001.12` makes a standing invariant rather than an
  observation. And a *per-task* allowance structurally cannot bind them: each
  wakes a task just created, onboarded or swept, where a fresh or long-idle task
  brings a fresh allowance, and the subscriber's update path is a redelivery
  already owned by the dedup identity. Bounding them needs the aggregate scope
  named above.
- **Existing run rows.** No migration, backfill or reinterpretation. Already
  persisted rows count toward a window exactly as new ones do.

## Accepted consequence

When the allowance is exhausted and the refused wake targets a *different* agent
from the current assignee, that agent is left assigned and not woken until a
later occurrence or a user-initiated assignment wakes it. Accepted, not
mitigated: it needs five agent-driven reassignments of one task inside ten
minutes; user-initiated assignment is never limited, so an operator can recover
the task by hand; and the refusal is counted and logged under
`REQ-OFFICE-ASSIGN-RATE-003`. Exempting cross-agent
reassignment would reopen the exploit: alternating assignment between two agents
was already unbounded.

A coalescing merge can also relabel the actor type the count reads, because
`CoalesceRun` does not match on actor type and overwrites the surviving row's
payload. Per mixed-actor merge the observed count may run one high or one low.
Accepted: both available fixes are excluded, actor-aware coalescing by
`## Out of scope` and state of this gate's own by
`AC-OFFICE-ASSIGN-RATE-002.4`. The mechanism is in the design's
`## Counting without new state`.

## System design

[Assignment wake rate limit](../system-design/assignment-wake-rate-limit-01.md).
