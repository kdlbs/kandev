---
status: draft
system: office
requirements:
  - REQ-OFFICE-SEAT-GUARD-001
  - REQ-OFFICE-SEAT-GUARD-002
---

# Office Seat Claim Decision Guard System Design

## Purpose and boundaries

Office owns the registration path that claims a seat, and the office-facing
entry points that record a decision. This design changes no production
behavior: it records which of two overlapping defenses covers which window, and
makes the one that is currently untested testable.

Contracts used but not owned: the workflow repository's decision store and the
per-`(task, role)` seat exclusion it shares with registration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-SEAT-GUARD-001` | [Two defenses, two windows](#two-defenses-two-windows) |
| `REQ-OFFICE-SEAT-GUARD-002` | [Making the window reachable](#making-the-window-reachable), [Verification obligations](#verification-obligations) |

## Two defenses, two windows

A registration that finds a claimable seat runs two statements inside one
transaction: a select that chooses the seat, and an update that reassigns it.

The **select** rejects a seat that already carries a decision. It is a plain
read inside the registration's transaction and it sees everything committed
before it ran.

The **update** carries a `NOT EXISTS` condition over decisions against the same
seat, and re-checks the seat's provenance. It can only add something the select
did not already provide when a decision commits *between* the two statements.

Whether anything can commit there is entirely a question about the seat
exclusion:

- **A decision carrying a role** acquires the exclusion, which the registration
  holds for its whole transaction. It cannot commit in the window. The update's
  condition is unreachable through this path, and a test that races this path
  cannot distinguish the condition being present from its being absent.
- **A decision carrying no role** does not acquire the exclusion. The decision
  writer's own code branches on the role being empty - it supersedes by
  participant instead of by decider-and-role, and skips the lock with the
  reasoning that there is nothing to serialize against. That reasoning is
  incomplete: there is something, and it is this window. For this path the
  update's condition is the whole defense.

Neither defense filters on `superseded_at`, and `AC-OFFICE-SEAT-GUARD-001.8`
requires that they keep agreeing. A seat that has ever been decided stays
unclaimable: the superseded rows are the audit trail of a reworked review, and
reassigning a seat that carries one would leave a verdict attributed to an agent
that did not give it. Adding a `superseded_at IS NULL` filter to either
statement would make the two disagree, and would make a reworked seat claimable
by whoever registers next.

Because the condition matches on the seat identity rather than on the
decision's role, it covers the roleless decision without modification. Nothing
in the production code needs to change for
`REQ-OFFICE-SEAT-GUARD-001` to hold - which is why that requirement is written
as a statement of existing behavior with the reachability named, not as a
change order.

No shipped caller produces a roleless decision: both office entry points
resolve a role first and refuse the request when they cannot. The path is
supported by the store and unused by the product, which is the precise sense in
which this is latent.

## Making the window reachable

`AC-OFFICE-SEAT-GUARD-002.2` requires the test to enter the window
deterministically. Two approaches were considered.

**Racing real goroutines** on the roleless path would genuinely reach the
window, because that path takes no exclusion. It is rejected: nothing forces
the interleaving, so the test would pass most of the time by never entering the
window at all - which is the same defect being fixed here, reintroduced in a
different test. A sibling follow-up already records this failure mode against a
neighbouring concurrency test.

**A test-only interleaving hook** is the design. An unexported package-level
function value in the office repository's sqlite package, called between the
select and the update, nil in every build that does not set it. An in-package
test sets it to commit a roleless decision against the chosen seat from a
second connection, then lets the update run.

Three properties make this acceptable rather than a production concession, and
they map to `AC-OFFICE-SEAT-GUARD-002.3`:

- The variable is unexported, so only this package's own tests can set it, and
  it needs no build tag to be unreachable from anywhere else.
- Unset, the call site is a nil check on a value that is never written, so the
  production path is byte-for-byte the behavior that ships today.
- It carries no behavior of its own. It is a yield point, not a policy hook,
  and nothing production reads it.

The hook is placed in the registration's claim attempt, between the statement
that chooses the seat and the one that reassigns it. That is the only window
the requirement is about.

If the sibling follow-up's claim-target-removed test wants the same yield
point, it is the same hook at the same site. That is a welcome economy and not
a dependency in either direction - neither card's tests need the other's.

## Data and contracts

No schema change, no interface change, no new exported symbol. The condition on
the update, the select's filter and the seat exclusion all stay exactly as they
are.

## Control flow

Unchanged in production. Under test, one nil-valued call site between two
existing statements of an existing transaction.

## Failure and recovery

Unchanged. A registration that finds the seat decided at update time affects
zero rows, falls through to inserting a new seat, and reports success - the
existing fallthrough, which `AC-OFFICE-SEAT-GUARD-001.5` and `-001.6` describe
rather than modify. Nothing is retried and nothing is failed.

## Persistence

No schema change, no migration, no backfill. The registration's transaction
boundaries and the exclusion it acquires are untouched.

## Security

No new surface. The hook is unexported, unset in production, and carries no
input from any request.

## Observability

Nothing new. The declined-claim outcome is already indistinguishable from "no
claimable seat existed" by design (`AC-OFFICE-SEAT-GUARD-001.5`), and this
design does not make it distinguishable - a new signal there would report a
path no shipped caller reaches.

## Verification obligations

Three changes to the suite, and one of them is a deletion of a claim rather
than of a test:

1. **The new guard test** (`-002.1`, `-002.2`). Drives a registration with the
   hook set, commits a roleless decision against the chosen seat from a second
   connection inside the window, and asserts the seat is not reassigned and a
   new seat is written for the registering agent instead. Removing the `NOT
   EXISTS` condition must make this test fail; that is the acceptance check for
   the test itself, and should be performed once by hand when it is written.
2. **The existing concurrency test's documentation** (`-002.4`, `-002.5`). Its
   stated premise - that decision recording and registration lock on
   non-contending namespaces - is false since the shared exclusion landed. It
   is re-documented as establishing what it can: that the two writers serialize
   on the shared exclusion, complete without deadlock in either acquisition
   order, and leave no reattributed decision. The test's assertions already
   support that reading; only its docstring asserts more than it shows.
3. **No test is deleted or weakened.** The existing concurrency test keeps its
   iterations and its multi-connection pool - serialization under real
   concurrency is worth covering, and the deadlock question is real, since the
   decision path acquires two exclusions in a fixed order and the registration
   path acquires one of them.

## Related decisions

- [ADR 0005 - agent model unification](../../../decisions/0005-agent-model-unification.md)
