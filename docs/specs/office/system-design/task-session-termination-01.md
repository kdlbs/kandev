---
status: draft
system: office
requirements:
  - REQ-OFFICE-SESSION-TERM-001
  - REQ-OFFICE-SESSION-TERM-002
  - REQ-OFFICE-SESSION-TERM-003
  - REQ-OFFICE-SESSION-TERM-004
---

# Office Task Session Termination System Design

## Purpose and boundaries

Office owns the rule that a `(task, agent)` pair names one durable conversation
and owns the three paths that end one. This design adds a precondition to those
three paths and changes nothing else.

Contracts this design uses but does not own: the session row's storage and
state machine (task system); the participant seat store and the exclusion that
protects it (workflow system); the effective-runner resolution shared by every
reader of a task's runner (office task projection). None of the three is
modified.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-SESSION-TERM-001` | [Where the precondition lives](#where-the-precondition-lives), [Control flow](#control-flow) |
| `REQ-OFFICE-SESSION-TERM-002` | [The capacity question](#the-capacity-question) |
| `REQ-OFFICE-SESSION-TERM-003` | [What stays where it is](#what-stays-where-it-is) |
| `REQ-OFFICE-SESSION-TERM-004` | [Observability](#observability) |

## Components and responsibilities

- **The office dashboard service** owns the three guarded call sites
  (`internal/office/dashboard/service_tasks.go`) and gains one new private
  helper that answers the capacity question. It already holds the office
  repository, which is the only component that can answer it.
- **The orchestrator's session terminator**
  (`internal/orchestrator/office_session_terminator.go`) is unchanged. It stays
  the dumb executor of "flip this pair's row to `COMPLETED`, idempotently", and
  keeps both its methods and its two interfaces exactly as they are.
- **The office repository** supplies the two reads the question needs. Both
  already exist: the task's execution fields, whose assignee field is the
  shared effective-runner projection, and the task's effective participant
  slate at its current step.

## Where the precondition lives

The guard goes in the dashboard service, immediately before each
`TerminateOfficeSession` call, not inside the terminator.

The terminator is in `internal/orchestrator` and reaches the session repository
only. The capacity question needs participant seats and the runner projection,
both owned by `internal/office/repository/sqlite`, which the orchestrator does
not and should not import - the `SessionTerminator` interfaces exist precisely
so office does not import orchestrator and orchestrator does not import office.
Pushing the question down would either invert that dependency or widen the
interface with a callback, for no gain.

Putting it in the service also keeps the two cascade paths correct by
construction. `TerminateAllForAgent` is a different method on the same type
with different callers (agent deletion, config-sync reconciliation), and
`AC-OFFICE-SESSION-TERM-003.1` requires it to stay unguarded. A guard inside
the shared terminator would have to be argued out of that method; a guard in
the dashboard service never reaches it.

`AC-OFFICE-SESSION-TERM-001.5` requires one implementation. The three call
sites are three lines in one file calling one unexported helper on the same
receiver; there is no second copy to drift.

## The capacity question

One helper, one signature: given a task and an agent profile, does the agent
still hold a capacity on that task?

**The runner capacity** is read from the task's execution fields, whose
assignee value is produced by the shared runner projection
(`RunnerProjection`). `AC-OFFICE-SESSION-TERM-002.2` requires this rather than
a direct read of runner seats, and the reason is specific: that projection has
three precedence tiers, and its last tier deliberately reads runner rows across
*all* steps, ordered by creation time. A hand-written "is there a runner seat
naming this agent" query would therefore disagree with the projection exactly
when a task has moved between steps - which is the case the guard is most
likely to meet. Reusing the projection makes disagreement impossible instead of
unlikely.

**The seat capacity** is read from the task's effective slate at its current
step (`ListAllTaskParticipants`), which already applies per-task precedence
over template-level rows and already scopes to the current step. That scoping
is what `AC-OFFICE-SESSION-TERM-002.3` requires and it is load-bearing: seats
are keyed `(step, task, role, agent)` and are not cleaned up when a task
leaves a step, so an unscoped read would find a seat naming a previous occupant
and suppress a correct termination permanently. The slate read is also what
`AC-OFFICE-SESSION-TERM-002.4` and `-002.5` fall out of: it already merges
template rows in, and it already returns every role.

Neither read is ordered-dependent (`-002.7`): the helper asks whether any
returned row names the agent, and stops. Both are plain reads on the read pool,
taking no lock and joining no transaction - the guard is not trying to be
atomic with anything, and `AC-OFFICE-SESSION-TERM-001.10` explains why it does
not need to be.

**Failure is closed** (`-001.7`). Either read failing means "do not terminate",
recorded and swallowed. This inverts the usual best-effort default in these
call sites, deliberately: the surrounding code swallows a *termination*
failure because a session left live is recoverable, and the same reasoning says
a session wrongly ended is not.

## Control flow

Each guarded path already commits its mutation before it reaches the
termination step, which is what `AC-OFFICE-SESSION-TERM-001.2` needs and why no
reordering is required:

1. **Role removal.** The participant delete commits, then the service asks the
   question and terminates only on "no capacity". The removed role is already
   absent from the slate.
2. **Seat claim.** The claim transaction commits with the seat's agent profile
   already reassigned to the claiming agent, then the post-commit effects run
   on a cancellation-detached context. The displaced agent no longer holds that
   seat when the question is asked. The question is inserted before the
   existing termination call and leaves the displaced-run cancellation beside
   it untouched (`AC-OFFICE-SESSION-TERM-003.4`).
3. **Reassignment.** The assignee update commits - which also rewrites the
   runner seat at the task's current step - then the reactivity pipeline runs,
   then the termination step. By the time the question is asked, the projection
   already returns the new runner, so the previous runner is retained only if
   it holds a seat in another role.

The third is where `AC-OFFICE-SESSION-TERM-003.2` matters. The reactivity
pipeline's hard cancel of the previous runner's running execution happens
*before* the termination step and is not guarded. An agent reassigned away from
the runner role but still seated as a reviewer stops the coding turn it no
longer owns and keeps the row it still reviews through. Those are two different
objects, and the design keeps them decoupled on purpose.

### Concurrency

`AC-OFFICE-SESSION-TERM-001.10` is satisfied without coordination, and the
argument is worth recording because the obvious reading is that two concurrent
removals could both suppress.

Each path reads strictly after its own commit. Take two paths removing
different capacities from the same agent: whichever commits second issues its
read after both commits are visible, observes neither capacity, and terminates.
The first may well suppress - it can still see the second's capacity - but the
outcome is the pair ending with a terminated session either way. The window
therefore closes itself, and adding a lock or a re-read would buy nothing.

This depends only on each read being a fresh statement issued after that
caller's own commit, which holds on both engines under their default isolation.
It does not depend on which caller reads first.

## Failure and recovery

- A failed capacity read suppresses the termination and is recorded
  (`-001.7`, `-004.2`). The session row stays live and is re-evaluated by the
  next guarded path that runs on the pair.
- A suppressed termination is not an error to its caller (`-001.3`). All three
  call sites already treat termination as best-effort and log rather than
  propagate; suppression is quieter than that and returns success.
- A session left live because a capacity was retained needs no recovery: it is
  the conversation the agent is still using, and the existing reuse rules will
  hand it back on the agent's next wakeup.
- A session left live by a *mistaken* retained-capacity answer is the residual
  risk this design accepts, and `-004.1`'s record with the retained capacity
  named is how it is diagnosed.

## Persistence

No schema change. No new column, index, migration or backfill
(`AC-OFFICE-SESSION-TERM-003.3`). The guard adds two reads on the read-only
pool per guarded termination and removes some writes; it opens no transaction
and takes no lock.

Sessions already ended by the pre-existing behavior are left as they are - the
requirement's "Out of scope" says so explicitly, and no repair is authorized.

## Security

No new surface. All three guarded paths already enforce their own
authorization before reaching the termination step - the participant mutators
behind the approve-permission gate, reassignment behind the assign-tasks gate -
and the guard runs after those checks, never instead of one. The question reads
only state the caller has already been authorized to mutate.

## Observability

Suppression is logged at the existing call sites' logger with the task, agent
profile, guarded reason and which capacity was retained (`-004.1`), and a read
failure is logged distinguishably (`-004.2`). Both follow the structured-zap
idiom the surrounding code already uses.

A counter under the office namespace counts suppressions, labelled only by
bounded dimensions - the guarded reason and which capacity was retained
(`-004.3`). Neither is unbounded: the reasons are a fixed set of three and the
capacity is one of two. No task, agent, step or session identifier is a label,
matching `AC-OFFICE-REVIEW-SEATS-004.6`'s rule for the same reason.

Terminations that proceed keep their existing reason values and log line
(`-004.4`), so the two outcomes remain comparable in one place.

## Verification obligations

The capacity question's own tests are ordinary. Two are worth naming because
they are the ones that fail silently if written casually:

- The **step-scoping** criterion (`-002.3`) needs a task that has *moved*: a
  seat written at a step the task has left, naming the agent, must not suppress
  a termination. A test that never moves the task passes whether or not the
  scope is bounded.
- The **decoupling** criterion (`-003.2`) needs a reassignment of an agent that
  keeps a reviewer seat, asserting both halves: the execution is cancelled
  *and* the session row is untouched. Asserting only the second passes for an
  implementation that wrongly suppresses both.

The shipped-configuration reachability described in the requirement's overview
also gives a direct regression test that needs no flag: with
`features.officeSessionIdentity` off, an agent that is both runner and
auto-cast reviewer holds the task's only session, and displacing it from the
reviewer seat must leave that session live.

## Related decisions

- [ADR 0005 - agent model unification](../../../decisions/0005-agent-model-unification.md)
