---
status: draft
system: office
created: 2026-09-02
owners:
  - kandev
---

# Office Participant Seat Provenance Requirements

## Overview

Two independent writers create participant seats for the same task and role,
and until this contract exists neither knows the other happened: the
step-entry seat-ensuring action of `review-participant-seats.md`, which
seats someone when a task enters a gated step so the fan-out has an
addressee, and the operator-facing per-task registration.

When both write, the store holds two seats naming two different agents: the
natural key is `(step, task, role, agent profile)`, so they do not collide
and the duplicate is admitted silently. The quorum guard then demands two
decisions where the operator named one reviewer, and the gate waits without
saying why.

Multi-seat roles are not the defect: two operators each naming a distinct
reviewer is supported and collapsing it would regress. The defect is
narrower. A seat the system cast so the gate would not deadlock must not
compete with the operator's own later choice; a fallback is a placeholder, a
preference replaces it. Telling them apart needs to know **which writer
wrote a seat**.

This is a distinct contract from `review-participant-seats.md`, which owns
how an agent is chosen at step entry. This one owns what a seat remembers
about its origin and what a second writer may do to it. Office owns the
registration surface and the fallback casting; the task system owns the
store.

## Terminology

- **Seat**, **gated step**, **per-task seat** and **template seat** are used
  as `review-participant-seats.md` defines them.
- **Seat identity** - the tuple (step, task, role, agent profile): the
  store's unique natural key.
- **Role slate** - every seat sharing one (step, task, participant role),
  any agent profile. A slate may hold more than one seat, which is why seat
  identity alone cannot prevent this defect.
- **Seat provenance** - a value on every seat naming its writer. Two values:
  **auto**, written by the step-entry seat-ensuring action of
  `review-participant-seats.md`; **manual**, by any other writer.
- **Automatic casting** - the step-entry writer. **Manual registration** -
  the operator-facing per-task registration writer, reachable for the
  `reviewer` and `approver` roles.
- **Claim** - a manual registration taking over an existing `auto` seat in
  place, reassigning its agent profile and setting its provenance to
  `manual`, rather than writing a second seat into that slate.
- **Unclaimed seat** - an `auto` seat with no decision against it. **Decided
  seat** - one with a decision.
- **Supported dialect** - either backend the seat store runs on, embedded or
  server.

## Prior art

**What we already knew.** Searched the vault at
`/Users/henry/Documents/henry/wiki` through QMD collection `wiki`, three
queries: lexical for provenance/origin/source columns, semantic for telling
auto-created rows from manual ones, hypothetical-document for the
supersede-an-auto-row design. Nothing on seat provenance or slates: that leg
returned nothing useful.

One adjacent page is load-bearing and followed here.
`concepts/optimistic-vs-pessimistic-concurrency` records the Agrawal, Carey
and Livny (ACM TODS 12(4), 1987) finding that optimistic restart wins only
where resources are effectively unbounded, blocking staying preferred under
contention. REQ-OFFICE-SEAT-PROVENANCE-004 follows it: the writers serialize
on a shared exclusion rather than repair a duplicate afterwards. Casting
keeps its natural-key retry as the backstop for what an exclusion cannot
cover, two writers naming the same agent.

**What others shipped.** Searched `saas-kb` with `search_fsm_docs`,
`category: "ai_sdlc"`, two queries: auto versus manual reviewer assignment,
and a default reviewer replaced when a user chooses one. Top relevance
0.0139 (Augment, OpenHands, Multica), none describing a
placeholder-versus-preference relation. That leg returned nothing useful; no
vendor behaviour is copied here.

**What we are doing differently.** Those alternatives treat automatic
assignment as configuration: a trigger fires or not, and manual assignment
is a separate act on a separate list. This contract makes the automatic seat
*provisional* and lets the first operator preference consume it
(REQ-OFFICE-SEAT-PROVENANCE-002).

## Requirements

### REQ-OFFICE-SEAT-PROVENANCE-001: Every seat records which writer wrote it

A seat shall carry, as durable typed data, which writer created it. Without
it a fallback and a preference are the same row.

#### Acceptance criteria

- **AC-OFFICE-SEAT-PROVENANCE-001.1:** Every seat written by automatic
  casting shall carry provenance `auto`.
- **AC-OFFICE-SEAT-PROVENANCE-001.2:** Every seat written by manual
  registration shall carry provenance `manual`, including one written when
  no claim was available.
- **AC-OFFICE-SEAT-PROVENANCE-001.3:** A seat written by any writer that
  states no provenance shall carry `manual`. The default is contracted, not
  incidental: the runner-assignment writer states none, and reads of its
  seats shall yield `manual`.
- **AC-OFFICE-SEAT-PROVENANCE-001.4:** A seat that existed before provenance
  was recorded shall read as `manual` after upgrade. No pre-existing seat
  shall become claimable by the upgrade alone.
- **AC-OFFICE-SEAT-PROVENANCE-001.5:** Provenance shall be readable as a
  typed value wherever a seat is read. No reader shall infer it from a
  seat's position, identifier, creation time, or its agent profile's
  relationship to the task. This means every in-process reader of the seat
  store; no participant API response carries provenance.
- **AC-OFFICE-SEAT-PROVENANCE-001.6:** Adding provenance to a store that
  predates it shall preserve every existing seat with its agent profile,
  decision-required flag and position, and shall be safe to apply more than
  once.
- **AC-OFFICE-SEAT-PROVENANCE-001.7:** Only a claim, or -002.3's promotion
  of a seat its own holder already occupies, shall change a seat's
  provenance, and either shall only ever change it from `auto` to `manual`.
  No operation shall change a seat's provenance from `manual` to `auto`.

### REQ-OFFICE-SEAT-PROVENANCE-002: A manual registration claims an unclaimed automatic seat

An operator naming a reviewer for a role the system already filled on their
behalf replaces that fallback, not joins it. A genuine second preference is
still a second seat.

#### Acceptance criteria

- **AC-OFFICE-SEAT-PROVENANCE-002.1:** When a manual registration names a
  role whose slate at the task's current step holds exactly one `auto` seat
  and that seat is unclaimed, the registration shall reassign that seat to
  the named agent profile and set its provenance to `manual`, and the number
  of seats in that role slate shall be unchanged.
- **AC-OFFICE-SEAT-PROVENANCE-002.2:** A claim shall leave the claimed
  seat's decision-required flag, position, creation time and identifier
  unchanged. Downstream readers shall observe a reassigned seat, not a
  replaced one.
- **AC-OFFICE-SEAT-PROVENANCE-002.3:** When the named agent profile already
  holds a seat at that seat identity, the registration shall write no new
  seat and shall leave the slate's seat count and every field of that seat
  but provenance unchanged. When that seat is the slate's sole `auto` seat
  and undecided, its provenance shall become `manual`, so naming the agent
  already cast stops that seat being claimable by a later registration;
  otherwise nothing changes at all. Either way the registration raises no
  error, writes no activity entry and publishes no task-changed
  notification, and is not a claim: -002.9, -002.12 and -002.13 do not reach
  it.
- **AC-OFFICE-SEAT-PROVENANCE-002.4:** The check of
  AC-OFFICE-SEAT-PROVENANCE-002.3 shall be evaluated before any claim is
  attempted, so that re-registering an agent that already holds a seat never
  consumes a *different* seat another agent could still claim.
- **AC-OFFICE-SEAT-PROVENANCE-002.5:** When the slate's sole `auto` seat is
  decided, the registration shall not claim it, shall write a new `manual`
  seat, and shall leave that seat's agent profile and provenance unchanged.
  A decision already recorded against an agent shall not be reattributed to
  a different agent by a later registration.
- **AC-OFFICE-SEAT-PROVENANCE-002.6:** When the slate holds more than one
  `auto` seat, the registration shall claim none and shall write a new
  `manual` seat. No ordering or tiebreak is required or permitted: the count
  alone decides, so the outcome cannot depend on the order the store returns
  them in.
- **AC-OFFICE-SEAT-PROVENANCE-002.7:** When the role slate holds no `auto`
  seat, the registration shall write a new `manual` seat.
- **AC-OFFICE-SEAT-PROVENANCE-002.8:** No registration shall claim a
  `manual` seat, however many the slate holds or whether they are decided.
  Two operators each registering a distinct agent in one role shall yield
  two seats, and a third registration naming a third agent shall yield
  three.
- **AC-OFFICE-SEAT-PROVENANCE-002.9:** A claim shall be observable as such:
  the system shall record the takeover, naming the task, step, role,
  displaced agent profile and claiming agent profile. That record shall be a
  task activity entry distinct from the one a plain registration writes; an
  ops-only log line does not satisfy it.
- **AC-OFFICE-SEAT-PROVENANCE-002.10:** When a claim has displaced an agent
  profile and that profile is later registered again in the same role at the
  same step, a second `manual` seat shall be written for it.
- **AC-OFFICE-SEAT-PROVENANCE-002.11:** A claim shall be subject to the same
  authorization as writing a new seat: a caller who may not register a
  participant shall not displace one, and no path shall reach a claim
  without that check passing.
- **AC-OFFICE-SEAT-PROVENANCE-002.12:** When a claim displaces an agent
  profile holding a live session for that task in that role, that session
  shall be ended as if the agent had been removed from the role, so it
  leaves the task's live participant indicators.
- **AC-OFFICE-SEAT-PROVENANCE-002.13:** A claim shall publish the same
  task-changed notification, naming the same participant field, that writing
  a new seat in that role publishes. A reader refreshing on it shall observe
  the claimed slate.

### REQ-OFFICE-SEAT-PROVENANCE-003: Each seat operation states the scope it searches

Three operations search three different scopes, deliberately. Left unstated
the difference reads as an inconsistency and gets "fixed".

#### Acceptance criteria

- **AC-OFFICE-SEAT-PROVENANCE-003.1:** A claim search shall consider only
  seats at the task's current step. A seat at another step of the same
  workflow shall not be claimable.
- **AC-OFFICE-SEAT-PROVENANCE-003.2:** Automatic casting's existing-seat
  check shall keep considering seats at any step of the task's workflow, as
  AC-OFFICE-REVIEW-SEATS-001.5 requires.
- **AC-OFFICE-SEAT-PROVENANCE-003.3:** When a task holds an `auto` seat in a
  role written at one step and a manual registration for that role arrives
  while the task stands at a different step, the registration shall write a
  new `manual` seat at the current step, leaving the earlier `auto` seat
  unchanged. The two seats sit in different role slates and neither step's
  quorum guard shall count the other's.
- **AC-OFFICE-SEAT-PROVENANCE-003.4:** Removal of a per-task participant
  shall keep removing every seat for that task, agent profile and role at
  every step, whatever its provenance. Removal is deliberately broader in
  scope than registration; this document does not change it.

### REQ-OFFICE-SEAT-PROVENANCE-004: Concurrent writers converge on one seat

The writers shall not interleave into a duplicate. Ordering the calls in a
test proves the sequential paths, not this one.

#### Acceptance criteria

- **AC-OFFICE-SEAT-PROVENANCE-004.1:** When automatic casting and a manual
  registration naming a different agent profile run concurrently for one
  task and role, exactly one seat shall exist in that slate afterwards,
  naming the manually registered agent profile and carrying provenance
  `manual`. This outcome shall not depend on which writer commits first.
- **AC-OFFICE-SEAT-PROVENANCE-004.2:** The two writers shall serialize
  against each other on a shared exclusion covering at least the task and
  the participant role, so that neither evaluates its existence check
  against the other's uncommitted state. An exclusion only one of the two
  writers acquires does not satisfy this criterion. Shared means one
  resource: two identities derived independently, each naming the task and
  the role but never contending, do not satisfy it.
- **AC-OFFICE-SEAT-PROVENANCE-004.3:** When two manual registrations naming
  two different agent profiles run concurrently for a role slate holding one
  unclaimed `auto` seat, exactly one shall claim it and the other shall
  write a new `manual` seat. Two seats shall exist afterwards, both
  `manual`, and no `auto` seat shall remain.
- **AC-OFFICE-SEAT-PROVENANCE-004.4:** When two manual registrations naming
  the same agent profile run concurrently for the same role, exactly one
  seat shall exist afterwards.
- **AC-OFFICE-SEAT-PROVENANCE-004.5:** A writer that loses any race in this
  requirement shall return success to its caller. No caller shall observe a
  contention error, and no caller shall be required to retry. This governs
  contention on that exclusion, not the store's own limits: a wait exceeding
  the store's timeout is a failure under AC-OFFICE-SEAT-PROVENANCE-005.5,
  not a race outcome.
- **AC-OFFICE-SEAT-PROVENANCE-004.6:** Every criterion in this requirement
  shall hold on both supported dialects. A criterion satisfied only where
  one dialect's locking primitive happens to be available is not satisfied.
- **AC-OFFICE-SEAT-PROVENANCE-004.7:** Which stored seat a concurrent claim
  reuses is not observable. For every race here the outcome, the agent
  profiles seated in that role and the provenance of each, shall be
  identical whichever writer wins. The activity entry of
  AC-OFFICE-SEAT-PROVENANCE-002.9 is outside this guarantee: under
  AC-OFFICE-SEAT-PROVENANCE-004.3 which registration claims is not
  contracted.
- **AC-OFFICE-SEAT-PROVENANCE-004.8:** When the seat a registration selected
  to claim is removed before that claim is applied, the registration shall
  write a new `manual` seat for its named agent. It shall not complete
  having written no seat.
- **AC-OFFICE-SEAT-PROVENANCE-004.9:** Two concurrent automatic castings for
  one task and role remain governed by AC-OFFICE-REVIEW-SEATS-001.4. This
  requirement adds the cases where at least one writer is a manual
  registration, which it does not reach.
- **AC-OFFICE-SEAT-PROVENANCE-004.10:** A registration shall resolve the
  task's current step within that exclusion and act at that step. A step
  read before it was acquired shall not be used, so no seat lands at a step
  the task left while the registration waited.

### REQ-OFFICE-SEAT-PROVENANCE-005: Absent, empty and malformed inputs are defined

#### Acceptance criteria

- **AC-OFFICE-SEAT-PROVENANCE-005.1:** A manual registration naming an empty
  agent profile identifier shall be rejected at the registration surface
  before any seat is written or claimed.
- **AC-OFFICE-SEAT-PROVENANCE-005.2:** A manual registration for a task that
  stands at no step shall write no seat, claim no seat, and shall not fail.
- **AC-OFFICE-SEAT-PROVENANCE-005.3:** A seat whose stored provenance is
  neither `auto` nor `manual` shall not be claimable, and reading it shall
  not fail the read.
- **AC-OFFICE-SEAT-PROVENANCE-005.4:** When the decision store cannot be
  read while a claim is being evaluated, the registration shall claim
  nothing, write nothing, and surface the failure. It shall not fall through
  to writing a second seat.
- **AC-OFFICE-SEAT-PROVENANCE-005.5:** A failed registration shall leave the
  role slate exactly as it found it: no partially applied claim, and no seat
  whose provenance changed without its agent profile changing.
- **AC-OFFICE-SEAT-PROVENANCE-005.6:** A manual registration naming a task
  that does not exist shall behave as -005.2 does: no seat written, no seat
  claimed, no failure raised.
- **AC-OFFICE-SEAT-PROVENANCE-005.7:** Only the `reviewer` and `approver`
  roles are reachable through manual registration. A registration naming any
  other role shall be rejected before any seat is written or claimed.
- **AC-OFFICE-SEAT-PROVENANCE-005.8:** A registration naming an agent
  profile that does not exist shall claim no seat and displace no `auto`
  seat. Whether it writes one of its own is unchanged here; what is
  forbidden is an unknown agent taking over a cast seat.

### REQ-OFFICE-SEAT-PROVENANCE-006: The operator sees one decision asked of one agent

The observable point of all the above.

#### Acceptance criteria

- **AC-OFFICE-SEAT-PROVENANCE-006.1:** When a task enters a gated step that
  casts a seat automatically, and an operator then registers a different
  agent in that same role, the step's quorum guard for that role shall
  require exactly one decision.
- **AC-OFFICE-SEAT-PROVENANCE-006.2:** The seat that guard counts shall name
  the registered agent profile, not the automatically cast one.
- **AC-OFFICE-SEAT-PROVENANCE-006.3:** That entry's fan-out wakes the seat
  once, addressed to whoever holds it at that moment. After a claim, the run
  addressed to the displaced agent profile shall be cancelled as
  AC-OFFICE-SEAT-PROVENANCE-006.6 requires.
- **AC-OFFICE-SEAT-PROVENANCE-006.4:** A single decision recorded by the
  registered agent shall satisfy that role's quorum guard and allow the
  step's transition to fire.
- **AC-OFFICE-SEAT-PROVENANCE-006.5:** The task's participant listing for
  that role shall show one entry naming the registered agent.
- **AC-OFFICE-SEAT-PROVENANCE-006.6:** A claim shall cancel the run that
  entry queued for the displaced agent profile while that run is still
  queued or claimed, and shall leave one that already finished unchanged.
  When the cancellation itself fails, the registration shall still succeed
  and the failure shall be recorded; one run may then remain runnable for
  the displaced agent profile.

## Out of scope

- **Changing removal's scope to match registration's.** Registration binds
  to the task's current step while removal spans every step
  (AC-OFFICE-SEAT-PROVENANCE-003.4). Narrowing it would strand seats at
  steps a task left, a separate defect.
- **Collapsing multiple `manual` seats in a role.** Two operators naming two
  reviewers stays supported (AC-OFFICE-SEAT-PROVENANCE-002.8); nothing
  deduplicates them.
- **Reassigning a decided seat.** Once an agent has decided, no registration
  moves that seat (AC-OFFICE-SEAT-PROVENANCE-002.5). Overriding a recorded
  decision is a decision-lifecycle question, not a seating one.
- **A third provenance value.** Only `auto` and `manual` exist. The
  seat-caster's `eligible_pool` and `runner_fallback` values in
  `review-participant-seats-01.md` record *which agent* casting chose, a
  different axis from *which writer* wrote it, noted so neither is read as
  the other.
- **Provenance for `watcher`, `collaborator` and `runner` beyond the
  default.** Only `reviewer` and `approver` reach both writers; the others
  get `manual` by AC-OFFICE-SEAT-PROVENANCE-001.3, with no claim behaviour.
- **Reporting a not-found error for an unknown task or agent profile.**
  AC-OFFICE-SEAT-PROVENANCE-005.6 and -005.8 record that the surface
  silently ignores these. That is shipped behaviour; changing it would
  change every participant mutation's status code. What -005.8 forbids is a
  *claim* on it.
- **Surfacing provenance in the task UI.** The operator sees the outcome
  (REQ-OFFICE-SEAT-PROVENANCE-006), not the mechanism.

## Dependencies

- `review-participant-seats.md` owns automatic casting: which agent is
  chosen, when a seat is written, and the workflow-wide check restated in
  -003.2. It also owns when a seat is woken, including one written or
  reassigned after that entry's fan-out (AC-OFFICE-REVIEW-SEATS-003.5). No
  criterion in it changes here.
- `step-entry-sequence-execution.md` owns whether a step's entry actions run
  at all; if they do not, nothing here is reachable.