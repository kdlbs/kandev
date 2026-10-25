---
status: draft
system: office
requirements:
  - REQ-OFFICE-SEAT-ASSURANCE-001
  - REQ-OFFICE-SEAT-ASSURANCE-002
---

# Office Participant Seat Provenance Assurance System Design

## Purpose and boundaries

`participant-seat-provenance-assurance.md` asks for two things: a guard the
seat store applies to its own argument, and a set of behaviours the suite
must be able to fail on. The first is four lines of code and needs a design
only for where it sits and what it returns. The second is where the real
design work is, and it is not optional detail: a criterion that says "the
suite shall cover X" and stops there hands the builder the hardest question
— *how do you make X happen on purpose* — with no answer.

Three of those questions have already been answered wrongly once in this
area, in the shipped code's own comments: a window that "cannot be forced
deterministically", a race left to the scheduler across fifteen iterations,
and an end-to-end assertion that watched the wrong field. So this document
names a mechanism for each, and says what to do when the mechanism does not
hold.

It owns no seating behaviour. `participant-seat-provenance.md` and
`review-participant-seats.md` own that, both frozen here.

## Requirement mapping

| Requirement | Sections |
| --- | --- |
| `REQ-OFFICE-SEAT-ASSURANCE-001` | *The store-boundary guard*, *Failure and recovery*, *Security* |
| `REQ-OFFICE-SEAT-ASSURANCE-002` | *Making each window reachable*, *Where the cases live*, *Observability* |

## The store-boundary guard

### Placement

The check is the **first statement** of the seat store's add-participant
entry point — `Repository.AddTaskParticipant` in
`internal/office/repository/sqlite/participants.go` — before the transaction
is begun and therefore before the
shared exclusion of `AC-OFFICE-SEAT-PROVENANCE-004.2` is acquired. That
ordering is the whole of `AC-OFFICE-SEAT-ASSURANCE-001.3`: a rejected call
that has already taken the exclusion would make a malformed argument a
source of contention for correct writers, and a rejected call that has
already opened a transaction would need a rollback to be correct rather
than being trivially correct.

It is deliberately *not* placed in the service layer beside the existing
role lookup. The service layer is a caller; the point of the guard is that
the store stops depending on which caller is in front of it. Placing it
there would reproduce the gap one layer down.

The existing surface check stays exactly where it is. Two layers is the
intent, not a duplication to collapse later: the surface returns a client
error to an operator, the store refuses to write a row. Neither replaces
the other, and `AC-OFFICE-SEAT-ASSURANCE-001.6` is what keeps the surface's
behaviour identical.

### What it returns

An exported sentinel error value — `ErrEmptyAgentProfileID`, named here so
two callers cannot end up comparing against two different identities —
returned with the zero result. Sentinel
rather than a formatted string because `AC-OFFICE-SEAT-ASSURANCE-001.2`
requires a caller to distinguish this failure from the store's *unchanged*
outcome without matching on a message, and the store's other refusals
(no step, no such task) are reported as *unchanged* with a nil error, which
is exactly the confusion the criterion forbids.

Nothing maps it to an HTTP status. The one surface that could is already
unreachable by construction, and inventing a status for a caller that does
not exist is the speculative configuration this repository's engineering
principles reject. It falls through the existing generic error mapping,
which is correct for a caller that should not have called.

The guard consults no store state, so `AC-OFFICE-SEAT-ASSURANCE-001.4` and
`-001.7` are structural: the decision is a comparison against the empty
string on an argument, giving the same answer on every call and on both
dialects, with no query to differ between them.

Only the empty string. No trimming, no normalisation
(`AC-OFFICE-SEAT-ASSURANCE-001.5`). The surface's own check tests the empty
string too, and a guard that trimmed would accept a value the surface
rejects, or reject one it accepts, and the two layers would then disagree
about what they are guarding. A whitespace identifier names no agent
profile, which `AC-OFFICE-SEAT-PROVENANCE-005.8` already handles by
refusing it the claim search.

## Making each window reachable

One subsection per criterion that needs a mechanism. The others —
`002.1`, `002.2`, `002.8`, `002.9` — are ordinary sequential cases against
a seeded slate and need no design beyond naming the assertions, which their
criteria already do.

### The step must be resolved on the writing handle (`002.3`)

The store resolves the task's current step twice over in this codebase:
once on the transaction handle, inside its exclusion, and once on the
read-only pool for the listing paths. Swapping one for the other is a
one-word edit that changes nothing any current test observes, because in
every test both handles point at the same database.

**Mechanism: give them different databases.** Build the office repository
with its writer on one store and its reader on a second, where the same
task row names a *different* current step. Register a participant, then
assert the seat landed at the step the **writer's** store names.

This is decisive in both directions. The shipped implementation reads
through the transaction and lands at the writer's step. An implementation
that reads through the read-only pool lands at the reader's step — or, if
the reader's row is absent instead of divergent, writes nothing and reports
*unchanged*. Either way the case goes red, and it does so without any
concurrency, on the embedded dialect, in milliseconds.

Two constraints the builder will otherwise hit. Schema initialisation runs
against the writer, so the reader store must be given its own `tasks` table
and row explicitly. And every assertion afterwards must read through the
writer handle directly, never through the repository's reader accessor,
which by construction now points at the wrong database.

Prefer a divergent step id over an absent row: divergence proves the seat
landed at the writer's step, where absence only proves the reader was not
consulted.

### The claim target must vanish mid-write, on purpose (`002.10`)

This is the window `AC-OFFICE-SEAT-PROVENANCE-004.8` protects: a seat
selected as claimable, removed before the conditional write applies, and
the registration falling through to inserting its own seat rather than
completing having written nothing. The shipped comment on that path states
it "cannot be forced deterministically", and the Postgres case that stands
in for it runs fifteen concurrent iterations whose split it logs but does
not assert — so a run in which the window was never entered passes
identically to one in which it was.

The premise that it cannot be forced is wrong, and the reason is visible in
the code. The claim search is a plain read; the claim itself is a
conditional `UPDATE` of one row by identifier. A plain read is not blocked
by a row lock, and an `UPDATE` is. So an outside session can stand exactly
between them.

**Mechanism, on the server dialect, three connections:**

1. A holding session opens a transaction and takes a row lock on the seat
   (`SELECT ... FOR UPDATE`). Nothing else is locked.
2. The registration runs in a goroutine. Its advisory lock is uncontended.
   Its identity probe misses. Its claim search reads the seat *through the
   row lock* and selects it as the candidate. Its conditional write then
   blocks.
3. The test waits until the registration's backend is observably waiting on
   a lock, through the server's own activity view. **This wait is the
   proof**: reaching it establishes the candidate was selected and the
   write not yet applied, which is the window itself. It replaces the
   existing case's unasserted iteration split.
4. The holding session deletes the seat and commits.
5. The blocked write re-evaluates, matches no row, and reports zero rows
   affected. The registration falls through and inserts.

Assert the outcome is *inserted*, that exactly one seat exists, that it
names the registering agent with provenance `manual`, and that its
identifier differs from the seat that was removed. An implementation that
reported a claim on a zero-row write would leave the slate empty and the
outcome *claimed*, failing on both counts.

No production code changes for this. That is the point of choosing an
external handle over the failpoint idiom this repository also uses: the
window is entered on the shipped path, not on a path that exists only under
test.

**If the wait probe proves unreliable** — a server build where the wait is
not visible, or a probe that flakes — fall back to the failpoint idiom
already established here for destructive cutover migrations: a hook between
the claim search and the conditional write, nil in production, that the
test uses to perform the removal. Take that only if the probe fails in
practice, and record why in the case's comment. Do not take it first.

The existing fifteen-iteration case is not deleted. It exercises genuine
cross-connection concurrency, which the deterministic case does not; the
two answer different questions and both stay.

### A cancellation that fails (`002.7`)

`AC-OFFICE-SEAT-PROVENANCE-006.6`'s second sentence is the only place the
contract accepts a residual: the claim commits, the displaced agent's
queued run cannot be cancelled, the registration still succeeds, and one
run stays runnable for an agent no longer seated. Nothing tests it.

**Mechanism: take the runs store away for the duration of the call, then
give it back.** Seed the fan-out run for the displaced agent, rename the
runs table aside, drive the registration, then restore it. Renaming rather
than dropping is what makes the fourth assertion possible — after the
restore, the run is still there and still runnable, which is the residual
the criterion describes, and a dropped table would take the evidence with
it.

Four assertions, one per clause of the sentence: the request succeeds; the
claimed slate is exactly as a successful claim leaves it; the failure is
recorded; the displaced run is still runnable.

The third needs a logger the test can read. The service takes one at
construction, and the logger package can be built from a zap logger, so an
observer core at warning level supplies it. The existing shared test
harness hands the service a default logger, so this needs a
logger-accepting variant of that harness rather than a change to every
caller of it. Assert on the observed entry's fields — the task, the step,
the displaced agent profile — not on its message text.

### The guard requires exactly one decision (`002.6`)

The end-to-end quorum case already walks the scenario the whole contract
exists for: a step entry casts a seat, an operator then registers a
different agent, and the slate must end at one seat. It asserts the guard's
role and the participant listing. It does not assert the number of
decisions the guard requires, which is the operator-visible consequence —
a regression to two seats leaves the role correct and the listing arguably
explicable, and shows up only when the gate silently never fires.

The count is already on the wire; the case's local response type simply
does not name the field. **Mechanism: widen that type to carry the required
count and assert it is one**, alongside the existing role assertion rather
than instead of it. This is the smallest possible change and needs no new
endpoint, fixture or spec file.

Its placement in the existing case matters and is already load-bearing
there: the registration happens *after* the step move, because seats bind
to the task's current step. That ordering is the scenario, not an accident
to tidy.

### The two surface refusals (`002.4`, `002.5`)

The empty-identifier refusal is reachable over HTTP: post a registration
naming an empty identifier, assert a client error and that the slate for
that task and role is still empty.

The non-reviewer-approver refusal is **not** reachable over HTTP, and this
is worth stating rather than discovering. The routes are role-fixed: each
endpoint names its own role, and the service exposes only reviewer and
approver entry points. There is no request that carries a bad role. The
refusal that exists is a lookup miss inside the shared body both entry
points call.

**Mechanism: a white-box case**, in the service's own package rather than
its external test package, calling that shared body with a role outside the
pair and asserting it fails and writes nothing. Go permits both packages in
one directory, so this is a new file, not a change to the existing ones. A
structural assertion over the role map instead would pass against a body
that ignored the map, so it is the weaker choice.

## Where the cases live

Revive's 800-effective-line limit applies to test files here, and the
guidance is explicit that new tests go in a new file rather than being
appended to a large one. Three of the four files this work extends are
already close to it — the Postgres claim file and the dashboard participant
file are within a few dozen lines of the limit, and the two repository
files have moderate room.

So: the deterministic window case and the cancellation-failure case each
begin a new file, and the white-box role case must be a new file regardless
because of its package. Helpers already defined in the existing files of
the same package — notably the multi-connection Postgres pool builder — are
reused, not copied; that helper's own comment explains why it exists in
that package at all, and a second copy would invite the two to drift.

Measure before appending anywhere. A case added to a file that then crosses
the limit fails lint, and moving it afterwards costs a round.

## Failure and recovery

The guard's failure path is the whole of its behaviour: it returns before
acquiring anything, so there is nothing to release, roll back or retry, and
`AC-OFFICE-SEAT-PROVENANCE-005.5`'s "leave the slate as you found it" is
satisfied by never having touched it.

Everything else in this document is test-side. Its failure mode is a case
that cannot be made to pass, and `AC-OFFICE-SEAT-ASSURANCE-002.12` fixes
what that means: it is evidence this document is wrong, and routes back to
the spec step. It is not licence to weaken the case or edit the
implementation under test. A case that turns out to be flaky is likewise a
defect in the case — the mechanisms above are chosen to be deterministic
precisely so that flakiness is a signal rather than a cost of doing
business.

The dialect-gated cases skip when the server dialect is unavailable, using
the same gate the existing Postgres cases use.
`AC-OFFICE-SEAT-ASSURANCE-002.11` is the reason that gate must remain a
visible skip: a case that silently passed without running would report
coverage this document then relies on.

## Persistence

Nothing here changes the schema, a migration, or a stored value. The guard
writes nothing by definition, and the cases assert against columns that
already exist — including the seat's creation time, which is a real column
on the seat table even though the office-side projection does not carry it,
so `AC-OFFICE-SEAT-ASSURANCE-002.9` must read the column directly rather
than the projected struct.

The cancellation-failure case mutates schema *within its own test database*
by renaming a table aside and back. That is a fixture manipulation of an
isolated store, in the same family as the existing case that drops the
decision table to stand in for a storage error, and it reaches no shared
state.

## Security

`AC-OFFICE-SEAT-PROVENANCE-002.11` requires a claim to be subject to the
same authorization as writing a new seat, and that check runs at the
service layer, above the store. The new guard sits below it and changes no
ordering: a caller refused by the permission check never reaches the store
at all, and a caller that passes it is unaffected by a guard that only
rejects an argument no legitimate caller supplies.

The white-box case for `002.5` calls the shared service body directly,
which is beneath the HTTP surface but *above* the permission check that
body performs, so it does not bypass authorization; it exercises the same
refusal an internal caller would meet.

## Observability

No new metric, log line or counter. The cancellation-failure warning that
`AC-OFFICE-SEAT-ASSURANCE-002.7` asserts already exists and is already
emitted; the case reads it rather than adding it, which is what makes the
assertion evidence about shipped behaviour.

The guard emits nothing. A rejection is returned to a caller that should
not have called, and a log line would be written for a condition no
production path can produce.

## Related decisions

- `participant-seat-provenance-01.md` owns the shared exclusion, the
  control flow the guard prefixes, the outcome value the store returns,
  and the displaced run's cancellation. Unchanged here.
- `review-participant-seats-01.md` owns automatic casting's choice of
  agent, which the concurrency cases exercise but do not constrain.
