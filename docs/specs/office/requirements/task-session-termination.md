---
status: draft
system: office
created: 2026-09-08
owners:
  - kandev
---

# Office Task Session Termination Requirements

## Overview

An office session is keyed on a `(task_id, agent_profile_id)` pair.
REQ-OFFICE-SESSION-IDENTITY-001 makes that pair name exactly one live durable
conversation, and `AC-OFFICE-SESSION-IDENTITY-001.8` states the consequence
explicitly: the office side treats the pair as one conversation *regardless of
which path created the row*. Nothing in that contract says when the
conversation ends.

Three paths end it today, and all three end it on the loss of a single
capacity: a participant role is removed by hand, a seat is taken from its
occupant by a manual claim, or the task is reassigned. Each flips the one
shared session row to `COMPLETED` without asking whether the agent still holds
another capacity on the same task.

That question is not hypothetical. `AC-OFFICE-REVIEW-SEATS-002.4`, `-002.5`
and `-002.9` each seat the task's runner, or an agent already seated in another
role, as a reviewer. An agent holding two capacities at once is a normal,
fallback-default outcome of the shipped casting rules, and when such an agent
loses one of them the current behavior ends the session it is still using for
the other.

In the configuration every profile ships (`features.officeSessionIdentity`
disabled), the defect is sharper than "sometimes over-eager". With that flag
off, a task that has a runner routes *every* office run through the session
owned by the runner, so no session is keyed on a non-runner participant at all.
A termination aimed at a displaced or removed non-runner therefore finds
nothing and does nothing, and a termination aimed at an agent that *is* the
runner finds the task's only live session. On the role-removal and seat-claim
paths, the operation is either a silent no-op or the exact case this document
forbids. Enabling the flag gives each agent its own row and removes the no-op
half, but not the defect: the self-review agent still holds one row for both
capacities.

Office owns this contract because office owns the rule that a pair names one
conversation, and owns all three paths that end one. The task system owns the
session row's storage and its non-office lifecycle, which this document leaves
untouched.

## Terminology

- **Capacity** - a distinct standing an agent holds on a task that causes the
  system to address it there. Two exist: the runner capacity and a seat
  capacity.
- **Runner capacity** - the agent is the task's effective runner, as that is
  resolved for every other reader of the task's runner.
- **Seat capacity** - the agent holds a seat in the task's effective
  participant slate, in any role.
- **Effective participant slate** - the seats a reader of the task observes at
  the task's current step, after per-task rows take precedence over
  template-level rows. It is the same slate the task's own readers see; this
  document does not define a second one.
- **Guarded path** - one of the three paths this document constrains: manual
  removal of a participant role, displacement of a seat's occupant by a claim,
  and reassignment of the task away from its runner.
- **Cascade path** - a path that ends every live session belonging to an agent
  across every task, because the agent instance itself is going away.
- **Live** and **terminal** - as defined by
  REQ-OFFICE-SESSION-IDENTITY. This document adds no state and redefines
  neither.

## Prior art

**Our own prior reasoning (wiki):** the leg did not run, and no result is
reported for it. Receipt: the `wiki-query` skill is not installed on this
runner (absent from `~/.claude/skills/` and from the task-local
`.claude/skills/`), `OBSIDIAN_VAULT_PATH` is unset, `~/.obsidian-wiki/` does
not exist, no vault directory exists under `~`, and `qmd` is not on `PATH`, so
neither the QMD path nor the documented `grep` fallback had a vault to resolve.
Recorded as unavailable rather than as an empty result.

**What others shipped (saas-kb):** the leg did not run. Receipt: no `saas-kb`
MCP server is exposed to this session - the only MCP tools present are the
`kandev` server's - so `search_fsm_docs` could not be called with or without
`category: "ai_sdlc"`. Recorded as unavailable, not as a search that found
nothing.

**In-repo prior decisions**, which did apply and which carried the weight the
two unavailable legs would have:

- **REQ-OFFICE-SESSION-IDENTITY-001 and -004** forecloses the obvious fix.
  Making session identity role-scoped would add a role dimension to the pair
  that -001 has just finished making singular, and
  `AC-OFFICE-SESSION-IDENTITY-004.5` forbids the new column such a dimension
  would need. The card that raised this defect offered role-scoped identity or
  a retained-capacity guard as alternatives; that contract decides between
  them, and this document takes the guard.
- **`AC-OFFICE-SEAT-PROVENANCE-002.12`** already scopes the claim path's
  termination to an agent "holding a live session for that task **in that
  role**". The implementation is broader than its own frozen criterion, because
  the session model cannot express "in that role". This document does not amend
  that criterion; it makes the behavior conform to what the criterion already
  says.
- **`AC-OFFICE-REVIEW-SEATS-002.3`** already treats an agent holding two
  capacities as undesirable-but-permitted, preferring a candidate that is
  "neither the task's runner nor already seated in another participant role"
  and falling back when none exists. The fallback is what makes this defect
  reachable, and it is deliberate, so the fix belongs on the termination side
  rather than in casting.
- **ADR 0005 (agent model unification)** made `agent_profile_id` shared between
  kanban and office, which is why the capacity question must be answered on the
  office code path rather than by a constraint on the shared row.

**What we are doing differently:** the shape this codebase reaches for when a
lifecycle event is over-broad is a narrower key. That is rejected here for the
first reason above, and the alternative is unusual enough to name: the key stays
as wide as it is and the *decision to act on it* acquires a precondition.
Ending the row becomes conditional on "this agent's one conversation about this
task" having stopped being true, rather than on one reason to suspect it might
have.

## Requirements

### REQ-OFFICE-SESSION-TERM-001: A session ends only with the last capacity

**Intent:** A guarded path removes one capacity. It must end the agent's
session only when that was the agent's last one, so an agent that is still
addressed on the task keeps the conversation it is still using.

**User story:** As an operator claiming a review seat from the agent that is
also writing the code, I want that agent to keep working, so that taking over
the review does not silently stop the task.

#### Acceptance criteria

- **AC-OFFICE-SESSION-TERM-001.1:** When a guarded path would end an agent's
  office session for a task, the system shall determine whether that agent
  retains a capacity on that task, and shall end the session only when it
  retains none.
- **AC-OFFICE-SESSION-TERM-001.2:** The determination shall observe state in
  which the mutation that removed the capacity has already committed, so the
  capacity being removed is never itself observed as retained. A determination
  made before that commit would suppress every termination and satisfies
  nothing in this document.
- **AC-OFFICE-SESSION-TERM-001.3:** When the agent retains at least one
  capacity, the system shall leave the session row entirely unwritten - its
  state, its reason and every other field unchanged - and shall report success
  to the guarded path's caller. A suppressed termination is not an error and
  shall not be reported as one.
- **AC-OFFICE-SESSION-TERM-001.4:** When the agent retains no capacity, the
  system shall end the session exactly as it does today: the row becomes
  `COMPLETED` carrying the reason that path already supplies, and no other
  observable behavior of that path changes.
- **AC-OFFICE-SESSION-TERM-001.5:** The determination shall have exactly one
  implementation in application code, reachable from all three guarded paths,
  and the system shall not introduce a second. Three copies that agree today
  are three copies that can disagree later, which is the failure this criterion
  exists to prevent.
- **AC-OFFICE-SESSION-TERM-001.6:** The guarded paths shall be exactly three:
  removal of a participant role from a task, displacement of a seat's occupant
  by a claim on that seat, and reassignment of a task away from its runner. The
  system shall not extend the determination to a path not named here without
  amending this criterion.
- **AC-OFFICE-SESSION-TERM-001.7:** If the determination cannot be completed
  because the state it reads is unavailable, then the system shall not end the
  session, shall record the failure under REQ-OFFICE-SESSION-TERM-004, and
  shall report success to the guarded path's caller. The determination fails
  closed: ending the session of an agent that is still working is the defect
  being removed, and leaving a session row live is recoverable by the next
  guarded path that runs, whereas ending one is not.
- **AC-OFFICE-SESSION-TERM-001.8:** When either the task identifier or the
  agent profile identifier is empty, the system shall neither evaluate the
  determination nor end a session, and shall report success. No pair is named,
  so there is nothing to decide.
- **AC-OFFICE-SESSION-TERM-001.9:** When no live session exists for the pair,
  the observable outcome shall be identical whether or not the determination
  was evaluated. The system may skip it, and a test shall not depend on whether
  it did.
- **AC-OFFICE-SESSION-TERM-001.10:** When two guarded paths run concurrently
  for the same agent and task, each removing a different capacity, the system
  shall end the session. This follows from
  `AC-OFFICE-SESSION-TERM-001.2` without further coordination: each path reads
  strictly after its own commit, so whichever commits second observes both
  removals and finds no retained capacity. The system shall not add a lock, a
  retry or a re-read to obtain this outcome, and shall not rely on which path
  reads first.
- **AC-OFFICE-SESSION-TERM-001.11:** The system shall not end a session that
  the same guarded path has already ended, and repeating a guarded path's
  termination step for a pair shall produce no further write. This preserves
  the existing idempotency of the termination step unchanged.

### REQ-OFFICE-SESSION-TERM-002: What counts as a retained capacity

**Intent:** The determination must ask a question the rest of the system
already answers the same way, and must ask it over a scope that cannot pick up
a capacity the agent no longer holds.

#### Acceptance criteria

- **AC-OFFICE-SESSION-TERM-002.1:** The system shall treat an agent as
  retaining a capacity on a task when it is the task's effective runner, or
  when it holds a seat in the task's effective participant slate, and shall
  treat it as retaining none otherwise. The two are alternatives, not an
  ordered test: neither takes precedence and the determination shall not depend
  on which is evaluated first.
- **AC-OFFICE-SESSION-TERM-002.2:** The system shall resolve the task's
  effective runner by the same rule every other reader of that task's runner
  uses, so that the determination and the rest of the system cannot disagree
  about who the runner is. It shall not resolve the runner by reading
  participant seats directly, which would answer a different question: seats
  written for a step the task has since left remain readable, and the runner
  rule already fixes their precedence.
- **AC-OFFICE-SESSION-TERM-002.3:** The seat scope shall be the task's
  effective participant slate at the task's current step. A seat recorded at a
  step the task is not standing on shall not, on its own, count as a retained
  capacity. Without this bound a task that has moved through steps accumulates
  seats naming a previous occupant, and a termination that is correct would be
  suppressed forever.
- **AC-OFFICE-SESSION-TERM-002.4:** A template-level seat that the slate
  projects onto the task shall count exactly as a per-task seat does. The agent
  is addressed on the task either way, which is the only thing the
  determination asks.
- **AC-OFFICE-SESSION-TERM-002.5:** Every participant role shall count,
  including a role that carries no decision obligation. The determination asks
  whether the agent is still addressed on the task, not whether it still owes a
  decision.
- **AC-OFFICE-SESSION-TERM-002.6:** The determination shall depend on the
  presence of a capacity and not on how many the agent holds. An agent holding
  two seats and an agent holding one shall be treated identically.
- **AC-OFFICE-SESSION-TERM-002.7:** The determination shall not depend on the
  order in which seats are returned, and shall produce the same result for any
  ordering of the same slate.
- **AC-OFFICE-SESSION-TERM-002.8:** When the task has no current step, the
  effective slate shall be empty and only the runner capacity shall be capable
  of being retained. When the task does not resolve at all, the determination
  shall be treated as having failed under `AC-OFFICE-SESSION-TERM-001.7` rather
  than as having found no capacity.
- **AC-OFFICE-SESSION-TERM-002.9:** The determination shall compare agent
  profile identifiers and shall not require the named profile to resolve to a
  live agent. A seat naming a profile that no longer resolves still names this
  agent, and an agent instance that is genuinely going away is handled by a
  cascade path instead.
- **AC-OFFICE-SESSION-TERM-002.10:** The determination shall produce the same
  result on every database engine the product supports.
- **AC-OFFICE-SESSION-TERM-002.11:** The determination selects nothing. It
  answers whether any capacity is held, so no ordering, tiebreak or
  deterministic choice among capacities is required, and the system shall not
  introduce one. Which capacity is named in `AC-OFFICE-SESSION-TERM-004.1`'s
  record may be any that was found.
- **AC-OFFICE-SESSION-TERM-002.12:** When a capacity is granted to the agent
  between the guarded path's commit and the determination's read, the system
  shall observe it and suppress the termination. An agent that has just been
  seated again is addressed on the task, and the criterion asks about the
  present, not about the state at the moment the capacity was lost.

### REQ-OFFICE-SESSION-TERM-003: What this contract does not change

**Intent:** This is a narrow precondition on three call sites. It is stated
negatively as well as positively so that an implementation cannot satisfy it by
a mechanism that also reaches the paths that must keep working, and so that the
inconsistency it deliberately leaves in place is not "fixed".

#### Acceptance criteria

- **AC-OFFICE-SESSION-TERM-003.1:** A cascade path shall remain unguarded and
  shall continue to end every live session belonging to the agent across every
  task. An agent instance being deleted or reconciled away retains no capacity
  anywhere by definition, so evaluating the determination there would be a cost
  with no possible effect.
- **AC-OFFICE-SESSION-TERM-003.2:** On the reassignment path, cancellation of
  the previous runner's *running execution* shall be unchanged, and shall
  continue to happen whether or not the session row's termination is
  suppressed. The two are deliberately decoupled: the agent must stop the work
  it is no longer the runner for, and must keep the conversation it is still
  seated for. An implementation that suppresses the cancellation alongside the
  termination does not satisfy this document.
- **AC-OFFICE-SESSION-TERM-003.3:** Session creation, reuse, lookup and
  identity shall be unchanged. The system shall introduce no new column on the
  session store, no new index on it, and no backfill or repair of existing
  rows.
- **AC-OFFICE-SESSION-TERM-003.4:** A suppressed termination shall change
  exactly one thing relative to today: the session row is not written. It shall
  not withhold, delay, duplicate or reorder any participant seat write,
  decision, queued or cancelled run, activity entry or change notification that
  its guarded path performs, and shall not itself wake, queue or re-queue
  anything.
- **AC-OFFICE-SESSION-TERM-003.5:** The behavior required by
  REQ-OFFICE-SESSION-TERM-001 and -002 shall hold identically whether
  `features.officeSessionIdentity` is enabled or disabled. The defect is
  reachable in both states and the fix shall not be conditioned on either.
- **AC-OFFICE-SESSION-TERM-003.6:** No participant casting, seat provenance, or
  claim behavior shall change. This document adds no acceptance criterion to
  the participant-seat-provenance contract and amends none of its criteria.

### REQ-OFFICE-SESSION-TERM-004: A suppressed termination is observable

**Intent:** The new outcome is a session that stays live where one used to end.
Left unrecorded it is indistinguishable from a termination that failed, from a
path that never ran, and from the leak a mistake in
`AC-OFFICE-SESSION-TERM-002.3` would produce.

#### Acceptance criteria

- **AC-OFFICE-SESSION-TERM-004.1:** When a termination is suppressed because
  the agent retains a capacity, the system shall emit a record identifying the
  task, the agent profile, the guarded path's reason, and which capacity was
  retained.
- **AC-OFFICE-SESSION-TERM-004.2:** When a termination is suppressed because
  the determination failed, the system shall emit a record that is
  distinguishable from `AC-OFFICE-SESSION-TERM-004.1`'s and that identifies the
  failure. A read fault and a retained capacity are different events and shall
  not be reported as one.
- **AC-OFFICE-SESSION-TERM-004.3:** The system shall increment a counter for
  each suppression, labelled only by bounded dimensions. It shall not label a
  counter with a task, agent, step or session identifier.
- **AC-OFFICE-SESSION-TERM-004.4:** A termination that proceeds shall continue
  to record what it records today, with the same reason values. This document
  adds no reason value and retires none.

## Out of scope

- **Role-scoped session identity.** Named because the card that raised this
  defect offered it as the alternative fix, and because it is the shape a
  reader will reach for first. It would add a role dimension to the
  `(task, agent)` pair that REQ-OFFICE-SESSION-IDENTITY-001 exists to make
  singular, and would need a discriminator column
  `AC-OFFICE-SESSION-IDENTITY-004.5` forbids. Rejected here, not deferred: the
  guard in REQ-OFFICE-SESSION-TERM-001 delivers the same observable outcome
  with no change to session identity at all.
- **Repairing sessions already ended by the current behavior.** No migration,
  backfill or repair is authorized by this document. A session wrongly
  completed before this contract ships stays completed; the pair's next office
  wakeup creates a fresh row by the existing rules, which is the recovery that
  already exists.
- **Ending a session when an agent loses its last capacity by a path not named
  in `AC-OFFICE-SESSION-TERM-001.6`.** An agent can stop being addressed on a
  task in ways none of the three guarded paths observes - a step transition
  that changes the slate, for one. Those paths end no session today and this
  document does not make them start: the scope is the over-eager termination
  that exists, not an audit of session lifetime.
- **The `features.officeSessionIdentity` rollout.** Whether that flag is
  promoted, and what its enablement requires, is its own decision.
  `AC-OFFICE-SESSION-TERM-003.5` only requires this contract to be indifferent
  to it.
- **The casting rules that make an agent hold two capacities.**
  `AC-OFFICE-REVIEW-SEATS-002.3` through `-002.9` are deliberate and unchanged.
- **Any user-visible surface.** The observable consequence is that a session
  stays in the live participant indicators the frontend already renders from
  existing session state. No new copy, no new component, no i18n work.

## System design

The technical design is
[part 1](../system-design/task-session-termination-01.md).
