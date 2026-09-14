---
status: draft
system: office
created: 2026-09-08
owners:
  - kandev
---

# Office Seat Claim Decision Guard Requirements

## Overview

`AC-OFFICE-SEAT-PROVENANCE-002.5` requires that a decision already recorded
against an agent is never reattributed to a different agent by a later
registration. Two mechanisms defend it in the shipped code, and they defend
different windows.

The first is a filter applied while a registration is choosing which seat to
claim: a seat that already carries a decision is not selected. It runs inside
the registration's transaction and covers every decision that was committed
before the registration looked.

The second is a condition on the update that actually reassigns the seat,
requiring that no decision exists against it at the moment of the write. It
covers only the window *between* those two points - a decision that commits
after the registration chose the seat and before it reassigned it.

That window is now closed for every decision the shipped product records.
Decision recording serializes on the same per-`(task, role)` exclusion the
registration holds for its whole transaction, so a decision carrying a role
cannot commit inside another writer's window. The exclusion is acquired only
when the decision carries a role, and the decision store deliberately supports
recording one without: the only validation on that path requires a task, a
step, a participant and a verdict. A decision with no role skips the exclusion
entirely, and for that decision the condition on the update is the only thing
standing between a recorded verdict and its silent reattribution to whoever
registers next.

No shipped caller produces a decision without a role - both entry points
resolve one, or refuse - so this is a latent path, not a live defect. It is
worth stating because the guard defending it is currently documented as
something else. The concurrency test covering this behavior asserts that the
two writers lock on non-contending namespaces and that the condition on the
update is therefore load-bearing. The first half stopped being true when the
shared exclusion was introduced, and the second half is now true only on the
path the test does not exercise. A test that cannot fail is worse than no test,
because it is counted.

Office owns this contract: it owns the registration path, the decision store's
office-facing entry points, and the exclusion the two share.

## Terminology

- **Registration** - a request to seat a named agent profile in a participant
  role on a task, which may claim an existing automatically-cast seat or write
  a new one.
- **Claimable seat** - the sole automatically-cast, undecided seat in a role's
  slate, which a registration may reassign to the registering agent.
- **Seat exclusion** - the per-`(task, role)` mutual exclusion that automatic
  casting, registration and role-carrying decision recording all acquire, held
  for the duration of the acquiring transaction.
- **Roleless decision** - a decision recorded without a participant role. The
  decision store accepts one; it supersedes prior decisions by participant
  rather than by decider and role, and does not acquire the seat exclusion.
- **Reattribution** - a seat carrying a recorded decision coming to name an
  agent other than the one whose decision is on file, so that the verdict reads
  as having been given by an agent that did not give it.

## Requirements

### REQ-OFFICE-SEAT-GUARD-001: A recorded decision survives a concurrent claim

**Intent:** `AC-OFFICE-SEAT-PROVENANCE-002.5` must hold on every path that can
record a decision, including the one that does not participate in the seat
exclusion. The shipped code already satisfies this requirement; it is written
down because nothing states it, nothing tests it, and the code comment
explaining the mechanism describes a different one.

#### Acceptance criteria

- **AC-OFFICE-SEAT-GUARD-001.1:** When a decision exists against a seat, a
  registration shall not reassign that seat to another agent, whether or not
  that decision carries a participant role.
- **AC-OFFICE-SEAT-GUARD-001.2:** When a roleless decision against a seat
  commits after a registration has selected that seat as claimable and before
  that registration reassigns it, the registration shall reassign no seat and
  shall write a new seat for the registering agent instead.
- **AC-OFFICE-SEAT-GUARD-001.3:** The behavior required by
  `AC-OFFICE-SEAT-GUARD-001.2` shall not depend on the decision writer and the
  registration serializing on the seat exclusion, because the roleless decision
  path does not acquire it. An implementation that removes the condition on the
  reassigning update and relies on the exclusion alone does not satisfy this
  requirement.
- **AC-OFFICE-SEAT-GUARD-001.4:** When a registration declines to reassign a
  seat under `AC-OFFICE-SEAT-GUARD-001.2`, it shall leave that seat's agent
  profile, provenance, decision-required flag, position and creation time
  unchanged, and shall leave the decision itself unchanged.
- **AC-OFFICE-SEAT-GUARD-001.5:** A registration that declines to reassign
  under `AC-OFFICE-SEAT-GUARD-001.2` shall report the same outcome, and produce
  the same effects, as a registration that found no claimable seat at all: a
  new seat is written, and none of the effects a claim earns - the claim
  activity record, the displaced agent's session termination, the displaced
  agent's queued run cancellation - occurs. Nothing was displaced, so nothing
  shall be reported as displaced.
- **AC-OFFICE-SEAT-GUARD-001.6:** A registration shall not be failed by the
  condition in `AC-OFFICE-SEAT-GUARD-001.2`. Finding the seat decided is a
  normal outcome and shall be reported as success.
- **AC-OFFICE-SEAT-GUARD-001.7:** This requirement adds no acceptance criterion
  to the participant-seat-provenance contract and amends none of its criteria.
  It names the path on which `AC-OFFICE-SEAT-PROVENANCE-002.5` is defended by
  the condition on the update alone.
- **AC-OFFICE-SEAT-GUARD-001.8:** Any decision against the seat shall block the
  reassignment, whether or not it has been superseded, and whether or not more
  than one exists. Both defenses shall agree on this: a seat the selection
  rejects as decided shall not be one the update would accept, and the reverse.
  Superseded decisions are retained as the audit trail of a reworked review, and
  a seat that has ever been decided has an attribution history that reassigning
  it would falsify.
- **AC-OFFICE-SEAT-GUARD-001.9:** A registration that declined to reassign under
  `AC-OFFICE-SEAT-GUARD-001.2` shall leave the registering agent seated by the
  new seat it wrote, so a repeat of the same registration is governed by the
  existing already-seated rule and writes nothing further. This document adds no
  retry and no second attempt at the claim.

### REQ-OFFICE-SEAT-GUARD-002: The guard is verified by a test that fails without it

**Intent:** A guard whose test passes when the guard is deleted is not covered.
The suite must contain at least one test that distinguishes the two, and the
tests that cannot must stop claiming they do.

#### Acceptance criteria

- **AC-OFFICE-SEAT-GUARD-002.1:** The test suite shall contain a test that
  fails when the condition required by `AC-OFFICE-SEAT-GUARD-001.3` is removed
  from the reassigning update, and that passes when it is present.
- **AC-OFFICE-SEAT-GUARD-002.2:** That test shall reach the window described in
  `AC-OFFICE-SEAT-GUARD-001.2` deterministically. Its result shall not depend
  on thread or transaction scheduling, and it shall not be capable of passing
  by never entering the window it targets.
- **AC-OFFICE-SEAT-GUARD-002.3:** Any mechanism introduced to make
  `AC-OFFICE-SEAT-GUARD-002.2` deterministic shall have no effect in a
  production build: with the mechanism unset, the code path shall behave
  exactly as it does today.
- **AC-OFFICE-SEAT-GUARD-002.4:** A test that establishes that the two writers
  serialize on the seat exclusion shall say that, and shall assert an outcome
  that serialization can produce. It shall not document itself as establishing
  that the condition on the update is load-bearing, which serialization
  prevents it from showing.
- **AC-OFFICE-SEAT-GUARD-002.5:** No test documentation in the suite shall
  assert that role-carrying decision recording and registration lock on
  non-contending exclusions. That premise is false and is the reason
  `AC-OFFICE-SEAT-GUARD-002.4` is needed.

## Out of scope

- **Removing the condition on the reassigning update.** Named because the
  premise that it is unreachable is the tempting conclusion from the shared
  exclusion, and it is wrong: `AC-OFFICE-SEAT-GUARD-001.3` requires it to stay,
  and the roleless decision path is why.
- **Making the roleless decision path acquire the seat exclusion.** This is the
  other way to close the window, and it is deliberately not taken here. The
  path has no role to key the exclusion on and would have to derive one from
  the seat it references, turning a write into a read-then-lock and widening
  decision recording's contention for a path no shipped caller uses. The
  condition on the update already delivers the outcome
  `AC-OFFICE-SEAT-GUARD-001.2` requires. Recorded so the trade is on the record
  rather than an omission.
- **Rejecting roleless decisions.** Whether the decision store should require a
  role at all is a separate contract question about the decision model. This
  document takes the store's accepted input shapes as given and requires the
  seat behavior to be correct for all of them.
- **The concurrency test covering a claim target removed mid-transaction.** A
  sibling follow-up owns that test and its own determinism gap. If a shared
  mechanism satisfies `AC-OFFICE-SEAT-GUARD-002.3` for both, that is a welcome
  outcome and not a requirement of this document, which constrains only the
  decision guard's own coverage.
- **Any user-visible surface.** No behavior change ships to a user from this
  document: `AC-OFFICE-SEAT-GUARD-001` records what the code already does. No
  copy, no i18n, no UI work.

## Prior art

The prior-art legs for this card are recorded once, in
[Office Task Session Termination Requirements](task-session-termination.md#prior-art),
with their receipts. Both external legs were unavailable on this runner; the
in-repo prior decisions that applied to this document are
`AC-OFFICE-SEAT-PROVENANCE-002.5`, which this requirement defends without
amending, and the shared seat exclusion introduced to close the role-carrying
decision race, whose own documentation records that the two writers previously
never contended.

## System design

The technical design is
[part 1](../system-design/seat-claim-decision-guard-01.md).
