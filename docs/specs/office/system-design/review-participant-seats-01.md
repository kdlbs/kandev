---
status: draft
system: office
requirements:
  - REQ-OFFICE-REVIEW-SEATS-001
  - REQ-OFFICE-REVIEW-SEATS-002
  - REQ-OFFICE-REVIEW-SEATS-003
  - REQ-OFFICE-REVIEW-SEATS-004
  - REQ-OFFICE-REVIEW-SEATS-005
---

# Office Review Participant Seats System Design

## Purpose and boundaries

Office owns the casting decision: which of its agents review and approve Office
work. This design defines when that casting is computed, where the seats are
written, and what happens when the casting resolves to nobody.

It adds one step action that runs before the fan-out and leaves the slate the
fan-out reads non-empty. Around that action it makes four changes that are
smaller in code and larger in consequence, none of them invisible:

- It **does** change the quorum guard, narrowly: a seat whose agent profile no
  longer resolves must be skipped with a warning rather than failing or
  silently counting, which `AC-OFFICE-REVIEW-SEATS-004.3` requires and the
  guard does not do today. Nothing else about the guard - roles, thresholds,
  decision-required filter - moves.
- It **depends on** a change it does not own: the participant-shaped entry
  actions are compiled and never dispatched, so the `review` step's declared
  sequence does not run. That is `REQ-OFFICE-STEP-ENTRY-001`, designed in the
  sibling document and summarized under *Dependency* below.
- It **does** add one participant writer, because no existing one has both the
  workflow-and-role scope and the single-transaction check-then-insert this
  design depends on. See *Components*.
- It **does** add one column to the participant store, a creation timestamp,
  because the runner resolver's final tier has no dialect-portable way to mean
  "most recent" without one. See *Casting resolution*.

It does **not** change the fan-out's contract, and beyond that column it does
not change the participant store's row shape.

## Requirement mapping

| Requirement | Where it is realized |
| --- | --- |
| REQ-OFFICE-REVIEW-SEATS-001 | Seat-ensuring step action; natural-key uniqueness |
| REQ-OFFICE-REVIEW-SEATS-002 | Office seat caster |
| REQ-OFFICE-REVIEW-SEATS-003 | Read alignment across fan-out and quorum guard |
| REQ-OFFICE-REVIEW-SEATS-004 | Warning records and counters at both readers |
| REQ-OFFICE-REVIEW-SEATS-005 | Office workflow template plus configuration reconciliation |

`REQ-OFFICE-STEP-ENTRY-001` is a dependency, not a claim of this design; it is
mapped by [`step-entry-sequence-execution.md`](step-entry-sequence-execution.md).

## Dependency: the entry sequence must execute, and one entry must be nameable

Nothing here is observable unless the `review` step's declared `on_enter`
sequence runs on a real arrival, and today it does not: the compiled
participant actions are never dispatched in production, so today's
`clear_decisions` and reviewer fan-out are already dead on a running install.
The reported defect therefore has two independent causes, and fixing either
alone changes nothing observable: nothing writes a reviewer seat (this design's
subject), and nothing executes the step's entry sequence, so even a correct
seat would not be woken.

The second is `REQ-OFFICE-STEP-ENTRY-001`, designed in
[`step-entry-sequence-execution.md`](step-entry-sequence-execution.md). Three
of its positions are load-bearing here and are used below without restating:

- **Dispatch is driven from the committed arrival**, the `task_step_transitions`
  row, not the session-carrying entry handler. A task moved into `review` by
  hand with no live session never reaches that handler, and `review` and
  `approval` both allow manual moves, so a seat written only on the handler
  path would be missing on exactly the route this capability exists to serve.
- **Exactly one component dispatches, synchronously, and a dispatch lost to a
  crash is not recovered.** That is a stated contract there
  (`AC-OFFICE-STEP-ENTRY-001.8`, `.9`), not an assumption made here.
- **Entry identity is that ledger row's identifier.** It is what
  `AC-OFFICE-REVIEW-SEATS-003.2` deduplicates a redelivery against, and it is
  why a rejected-then-reworked task re-entering `review` gets a fresh fan-out
  rather than being discarded as a duplicate
  (`AC-OFFICE-REVIEW-SEATS-003.6`): re-entry writes a new row. It reaches only
  what the dispatch runs; the quorum guard is not on that path, which is why
  `AC-OFFICE-REVIEW-SEATS-004.7` is scoped to dispatch-time conditions and the
  guard's own two get their own rule in *Failure and recovery*. Deduplication
  rests on durable state keyed by that identity rather than process memory
  (`AC-OFFICE-STEP-ENTRY-001.10`), so it survives a restart.

## Why seats are written at step entry

Two placements were considered.

**Template seats on the step** (`task_id = ''`) would be written once when the
workflow is materialized and would apply to every task. Rejected: the casting
depends on the task's runner (`AC-OFFICE-REVIEW-SEATS-002.3`), which is not
known at materialization time; the store's template-seat writer has no
production caller and no coverage; and a template row is invisible in the
per-task participant surfaces an operator would look at.

**Per-task seats written at task creation** would be written for whatever step
the task happens to start on. Rejected, and this is the specific mistake the
overview warns about: it seats the creation-time step, not the gated step the
task will later reach, which is how the original slate deadlock was produced.

**Per-task seats written at step entry** is what this design adopts. Entry is
the first and only moment at which the task, its runner, and the step that
needs a reviewer are all known.

### Seats need no reconciliation; the step configuration does

These two are separate questions and the round-one design conflated them.

**Seats** need no reconciliation trigger. A task that has already entered a
gated step has already run that step's entry sequence; writing a seat for it
afterwards wakes nobody, because the wake happened at entry. Such a task is
recovered by re-entering the step (`AC-OFFICE-REVIEW-SEATS-005.5`). There is no
sweep over tasks.

**The step configuration does.** Office's workflow is materialized from an
embedded template, the ensure path returns early when the workflow exists, and
a step's action list is a serialized blob written only on the create path. So
adding the action to the template gives it to new workspaces and nobody else:
every existing install keeps its old rows and `AC-OFFICE-REVIEW-SEATS-005.1`
fails there while every test passes - the shape of the defect this card
reports. The repository has the precedent: a startup routine reconciles a step
flag on system-workflow rows materialized before the template set it, matching
steps by name within their template identifier and scoped so user-created and
user-customised workflows are never touched. This design adds a sibling routine
for step actions, under *Configuration reconciliation* below.

## Components

**Seat-ensuring step action (workflow engine).** A new entry-sequence action
kind naming a participant role. The engine compiles it and, at dispatch, calls
a registered callback. The engine holds no Office knowledge.

If the action's role is missing, empty, or not a recognized participant role,
the engine treats the declaration as unusable: it writes no seat, reports a
distinct malformed-configuration outcome to the callback's observability path,
does not fail the entry, and continues with the rest of the sequence
(`AC-OFFICE-REVIEW-SEATS-001.11`). Configuration is operator-editable and
survives template changes, so an unusable declaration is a runtime condition,
not a build-time impossibility. A second declaration of the same role in one
sequence is absorbed by the same-role write check, which is what makes
`AC-OFFICE-REVIEW-SEATS-001.12` hold without a separate guard.

**Office seat caster (Office).** Registered against that action kind. Given
workspace, task, step and role, it resolves an agent profile and returns it
with its provenance. Office is the only component that knows the CEO rule.

**Participant writer (task system).** A **new** writer, not a reuse. Calling it
"the existing per-task upsert, unchanged in shape" is wrong on two counts,
either of which ships a race-prone first cut that passes single-threaded tests.
That upsert checks on the **read** handle and inserts on the **write** handle -
two connections, so its check-then-insert structurally cannot be one
transaction, and the role-level bound in *Failure and recovery* depends on it
being one. And its natural-key lookup is scoped to **step and agent profile**,
where the check needed here is scoped to **workflow and role** (control flow
step 3), which no existing method performs; the step-level upsert is worse on
both counts.

The new writer's contract: in one transaction on the write handle, check
whether any seat in the given role exists for the given task at any step of the
given workflow; if none does, insert one seat at the given step,
decision-required, at position zero; return the seat and whether it was
inserted. On losing a race to the unique index, the caller retries the whole
transaction so it observes the winner's committed row. This is a sibling of a
writer already in the same file - an established pattern here, but still new
code the scope estimate must carry.

**Step-entry dispatch (orchestrator).** The change above, without which none of
the others is reachable.

## Data and contracts

A seat is `(step_id, task_id, role, agent_profile_id, decision_required,
position)`. The natural key is the first four columns. There is no unique index
on them today, which is what would let two concurrent entries both insert
(`AC-OFFICE-REVIEW-SEATS-001.4`); this design adds one.

**Position and ordering.** `position` is a non-negative integer; seats written
here use zero (`AC-OFFICE-REVIEW-SEATS-001.6`). Zero is also the zero value an
operator-written seat may carry, so ordering by position alone is not total,
and the surrogate row identifier is no stable tiebreak across dialects.
Order is `position` ascending then `agent_profile_id` ascending
(`AC-OFFICE-REVIEW-SEATS-001.10`). No position range is reserved for anyone;
positions are advisory ordering only and no code may infer provenance from the
value.

**The participant table is created in more places than one** - two production
schema owners and six test fixtures that hand-create it, eight sites. The
unique index and the new timestamp column must reach the two production sites;
the six fixtures are the reason a schema change here passes its own package's
tests while failing at startup, so they are updated in the same change and a
test asserts fixture and production shapes agree rather than trusting they do.

**Four readers read participants, and they do not agree**, which matters
because `REQ-OFFICE-REVIEW-SEATS-003` claims two of them do: the fan-out reads
seats in a role for a task at a step **without** the decision-required filter,
to decide whom to wake; the quorum guard reads the same **with** it, to decide
whom to count; the slate gatherer merges template and per-task seats in Go; and
the read-side listing serves API and UI. The first two disagree by design. This
design keeps the divergence and closes its consequence by writing every seat
decision-required (`AC-OFFICE-REVIEW-SEATS-001.2`), so the two sets coincide
for seats it writes.

The ordering requirement lands on the **gatherer**, not on either query: SQL
order does not survive a Go-side merge of two result sets, so ordering asserted
only in the queries is not the order the fan-out iterates.
`AC-OFFICE-REVIEW-SEATS-003.3` is satisfied by sorting after the merge, and its
test asserts the sequence the fan-out iterates rather than the one a query
returns.

## Control flow

A task enters a gated step:

1. The step change commits, and its ledger row is written in the same
   transaction. That row **is** the arrival, and its identifier is the entry
   identity.
2. The session-independent half of the step's compiled entry sequence is
   dispatched with that identity and evaluated in declared order with the
   record-and-continue loop. The session-shaped half is handled exactly as it
   is today, on the paths that have a session. In the shipped Office template
   the dispatched sequence is `clear_decisions`, then the seat-ensuring action,
   then the fan-out: the seat action sits **immediately before the first
   fan-out for its role** (`AC-OFFICE-REVIEW-SEATS-001.1`,
   `AC-OFFICE-REVIEW-SEATS-005.8`), which also keeps `clear_decisions` ahead of
   the wake (`AC-OFFICE-REVIEW-SEATS-003.4`).
3. The seat-ensuring callback checks whether the task already holds a seat in
   this role. **The check is workflow-scoped, not step-scoped**: it asks
   whether any seat in this role exists for this task anywhere in the task's
   workflow. If one does, it writes nothing and leaves that seat untouched
   (`AC-OFFICE-REVIEW-SEATS-001.5`, `AC-OFFICE-REVIEW-SEATS-003.5`). A
   step-scoped check would still duplicate an operator's seat placed at another
   step of the same workflow, the case `-003.5` names explicitly.
4. Otherwise it calls the Office caster, and on a resolved agent writes one
   seat, decision-required, at position zero, keyed on the natural key so a
   concurrent duplicate is absorbed by the unique index rather than by a
   read-then-write race.
5. The fan-out reads the slate and queues one run per seat in its role,
   carrying the entry identity so a redelivery of this arrival does not queue a
   second run (`AC-OFFICE-REVIEW-SEATS-003.2`).
6. Later, on turn completion, the quorum guard counts decisions from
   decision-required seats in the role.

**Re-entering the same step.** Step 1's entry identity changes, because the
arrival writes a new ledger row. Step 3 finds the existing seat and writes
nothing. Step 5's fan-out sees a new identity and queues a fresh run: for a
task rejected at Approval and reworked, same seat, new review round.

## Casting resolution

Given workspace, task, step and role:

1. List the workspace's Office agents whose role is `ceo` and whose status is
   neither `stopped` nor `pending_approval`, ordered by `created_at` then
   identifier.
2. If the list is empty, seat the task's runner, record the fallback
   provenance, and record self-review if the runner is the seated agent
   (`AC-OFFICE-REVIEW-SEATS-002.5`). If the runner does not resolve, write no
   seat and emit the unfillable signal (`AC-OFFICE-REVIEW-SEATS-002.6`).
3. If the list holds one agent and it is the task's runner, seat it and record
   self-review (`AC-OFFICE-REVIEW-SEATS-002.4`).
4. If the list holds more than one agent and the first is the task's runner,
   seat the second (`AC-OFFICE-REVIEW-SEATS-002.3`).
5. Otherwise seat the first, recording the provenance as *eligible pool* rather
   than *fallback* (`AC-OFFICE-REVIEW-SEATS-002.9`). This is the ordinary case,
   and naming it is what makes the provenance counter's values exhaustive
   rather than exceptional.

**The runner resolver does not work on Postgres, and is repaired here.** Step 2
is the only path that seats anyone in a CEO-less workspace, and in this card's
scenario it is the path that runs: the task stands at `review`, its `runner`
seat was written at `work`, and `review` declares no primary agent profile, so
the resolver's first two tiers miss and the third decides. That third tier
orders by `rowid`, which does not exist on Postgres, in a file that branches on
the dialect elsewhere - so the call errors instead of returning an agent,
routing to `AC-OFFICE-REVIEW-SEATS-002.6`: no seat, unfillable signal, task
parked. The defect this capability exists to remove would survive on Postgres
while every SQLite test passed.

Swapping `rowid` for the table's `id` does not fix it: that column is a text
identifier with no ordering meaning, so ordering by it is a random pick wearing
a determinism costume, which `AC-OFFICE-REVIEW-SEATS-002.10` forbids. The table
carries no timestamp, so **no existing column can answer "most recently
written"**.

**Design position: the repair is in scope, and it is a column.** The migration
*Persistence* already requires also adds a creation timestamp, and the third
tier orders by it descending with `agent_profile_id` ascending as tiebreak - a
named column and a named tiebreak, identical on both engines. Pre-existing rows
backfill to one constant, so they are mutually tied and resolved by the
tiebreak: still deterministic. Scoping the repair out was considered and
rejected; it leaves this capability's only CEO-less path broken on one of two
supported engines.

**Where the status exclusion is applied.** `AC-OFFICE-REVIEW-SEATS-002.2`
requires excluding `stopped` and `pending_approval` without changing any other
caller of the agent listing. The exclusion is therefore applied in the caster,
over the listing's result, not by adding a filter to the shared listing method
or by changing its default. A shared listing whose behavior changes for
everyone is how an unrelated Office surface silently loses agents.

## Failure and recovery

**Two entries race for the same task and step.** Both call the caster, and may
resolve *different* agents if the workspace's agent set changed between them,
so the unique index alone does not bound the outcome to one seat: it bounds it
to one seat *per resolved agent*, which is two rows.
`AC-OFFICE-REVIEW-SEATS-002.7` requires one seat per role per entry, so the
bound has to be at the role level. Step 3's write check provides it: it is a
read of "any seat in this role for this task, anywhere in this workflow",
performed inside the same transaction as the insert and on the same connection,
so the second entry observes the first entry's committed row and writes
nothing. That transaction is why the writer is new rather than reused (see
*Components*): the existing upsert splits read and write across two connections
and cannot provide this bound. The unique index remains the backstop for the
identical-agent case and for any caller that skips the check.

**The seated agent is deleted after the seat is written.** The row now
references a profile that does not resolve: the fan-out cannot wake it and the
guard would otherwise wait forever on a decision that can never arrive. The
guard skips such a seat, emits the warning record and counter of
`AC-OFFICE-REVIEW-SEATS-004.3`, and continues with the remaining seats. This is
the one change to the guard's behavior, and why the boundaries section does not
claim the guard is untouched.

**The caster errors.** No seat is written, the unfillable signal is emitted
with an error outcome distinct from the empty-result outcome
(`AC-OFFICE-REVIEW-SEATS-004.9`), and the entry does not fail
(`AC-OFFICE-REVIEW-SEATS-002.8`). A step entry that fails on a casting error
would strand the task more thoroughly than a missing seat does.

**The guard warns more than once for one entry, and that is correct.** Entry
identity bounds records per entry, but it reaches only what the entry dispatch
runs, and two of the four conditions do not fire there.
`AC-OFFICE-REVIEW-SEATS-004.2` (no decision-required seat in the role) and
`-004.3` (a seat whose agent profile no longer resolves) are detected when the
quorum guard is evaluated, and the guard is re-evaluated each time a decision
is recorded - so a role with two decision-required seats drives two evaluations
inside one entry.

**Design position.** Those two are bounded per **guard evaluation**
(`AC-OFFICE-REVIEW-SEATS-004.10`), and `-004.7` is scoped to the dispatch-time
conditions it can actually bound. The alternative - the guard re-reading the
ledger for the current entry identity on every evaluation and keeping a durable
marker per (entry, role, condition) - was rejected: it buys de-duplication of a
*log line* with a read per decision plus the only persisted state in this
design whose sole purpose is suppressing diagnostics. The condition is a
standing fact about a stalled gate, not an event, so re-reporting it each time
the gate is re-examined is honest, and a counter rising faster than entries is
itself diagnostic.

**One entry action fails.** The rest are still attempted and the failure is
recorded (`AC-OFFICE-STEP-ENTRY-001.4`): the seat-ensuring action failing must
not prevent `clear_decisions`, and the fan-out failing must not prevent a later
action. That behavior is the dependency's, not this design's - the step-entry
dispatch evaluates its own list with a record-and-continue loop while the
engine's shared loop keeps aborting on first error for every other trigger
(`AC-OFFICE-STEP-ENTRY-001.6`).

## Configuration reconciliation for existing workspaces

`AC-OFFICE-REVIEW-SEATS-005.6` and `AC-OFFICE-REVIEW-SEATS-005.7` are satisfied
by a startup routine modelled on the existing step-flag reconciler.

For each embedded workflow template, for each of its steps, for each
system-owned workflow row materialized from that template, the routine reads
the step's stored action list, and **inserts the seat-ensuring action if and
only if no action of that kind for that role is already present**. It writes
the list back only when it changed.

**Where it inserts decides whether any of this works.** Insert-if-absent with
no stated position is not enough: appending satisfies "the action is present"
while leaving it *after* the fan-out, where it writes a seat nobody reads until
the next entry. Fresh installs would pass every test and upgraded installs stay
broken - the defect this routine exists to fix, one layer down. So position is
part of the contract (`AC-OFFICE-REVIEW-SEATS-005.8`):

> The seat-ensuring action is inserted **immediately before the first fan-out
> action declaring the same participant role**, and at the **head** of the
> entry sequence when the step declares no such fan-out.

The embedded template declares the same order, so for an otherwise unmodified
step the reconciled list and a freshly materialized list are identical in
content and order. That equality is the assertion to write: it catches template
and routine drifting apart, which no test of either alone can see. For the
shipped `review` and `approval` steps the result is `clear_decisions`, then the
seat-ensuring action, then the fan-out.

**When someone else is writing the same row.** This is a read-modify-write over
a serialized blob, so a concurrent edit to an unrelated action on the same step
would be silently lost - failing `AC-OFFICE-REVIEW-SEATS-005.7` in the case
that criterion exists for. Read and write therefore occur in one transaction
that writes back only if the stored list still matches what was read; on a
mismatch the routine re-reads and retries a bounded number of times
(`AC-OFFICE-REVIEW-SEATS-005.9`). Exhausting them leaves that step unchanged,
records a warning naming workflow and step, and does **not** prevent startup:
the step is picked up next start, and refusing to boot over one un-reconciled
step is a worse failure than the one being fixed.

Three consequences of that shape: it is a per-row read-modify-write rather than
a column overwrite, because overwriting the blob with the template's list would
discard operator edits to unrelated actions on the same step; it is scoped to
system-owned rows matched by template identifier and step name, as the existing
reconciler is, so user-created and user-customised workflows are untouched
(`AC-OFFICE-REVIEW-SEATS-005.7`); and it is idempotent by construction, because
the presence check is the same every run and the insertion position is derived
from the list being examined, not from a remembered index. It registers after
migrations and after the step-flag reconciler, single-flight per process.

**The acceptance test, and how it drives the task.** Not a unit test of the
routine: it materializes a workflow from the *pre-change* template shape, runs
startup, then drives a task to Review and asserts a reviewer seat and a queued
run - the observation `AC-OFFICE-REVIEW-SEATS-005.6` asks for. A test that only
asserts the stored blob changed would pass with the dispatch gap still open.

*How* it drives the task is not incidental: the two routes into Review exercise
different code and only one has a session. **The product route** is an
agent-created Office subtask reaching Review as the reported defect's task did;
that is the primary assertion. **A manual move with no live session** is
supported (`review` declares `allow_manual_move: true` and no
`auto_start_agent`) and is the route that would silently write no seat if the
dispatch sat in the session-carrying entry handler, so it is asserted
explicitly. Both must produce a seat and a queued run; a suite covering only
the first would pass with `AC-OFFICE-STEP-ENTRY-001.1` broken for half the
arrivals.

## Persistence

One migration does two things to the seat table: it adds a **creation
timestamp column**, and it adds a unique index on the seat natural key
`(step_id, task_id, role, agent_profile_id)`, preceded by a dedupe of any
existing rows that violate it. The column is what *Casting resolution* orders
the runner resolver's third tier by; pre-existing rows backfill to a single
constant, and the dedupe runs after the backfill so its survivor rule is
unaffected by it.

This path runs on **both SQLite and Postgres**, so ADR-0027 applies in full and
is not optional here:

- Replay classification uses the `internal/db` helpers
  (`IsAlreadyExistsError` for the index, `IsDuplicateColumnError` for the
  timestamp column, which this migration does add). No local error-string
  matching.
- The dedupe must be written in dialect-neutral SQL. The obvious SQLite
  formulation keyed on `rowid` does not exist on Postgres; the dedupe therefore
  selects the surviving row by an explicit ordering over real columns.
- Coverage in the same change: fresh schema, then replay against the same
  database, on SQLite **and** on Postgres via the isolated-Postgres helper.
  SQLite passing is not evidence for this path.
- The six test fixtures that hand-create the participant table are updated in
  the same change, per *Data and contracts* above.

Survivor selection is `position` ascending then `agent_profile_id` ascending -
the total order the readers use - so which row survives is defined rather than
incidental.

## Security

Seats reference agent profiles inside one workspace. The caster lists agents
for the task's workspace only, and the seat inherits the step's and task's
existing scoping. This capability adds no authorization surface and no API
route. The reconciliation routine runs at startup with no request identity,
touches only system-owned workflow rows, and reads no user input.

## Observability

Four counters, each labelled only from bounded dimensions
(`AC-OFFICE-REVIEW-SEATS-004.6`):

| Counter | Labels |
| --- | --- |
| `office_review_seat_ensure_total` | `role`, `outcome` (`seated`, `already_seated`, `no_candidate`, `no_runner`, `error`, `malformed_config`) |
| `office_review_seat_provenance_total` | `role`, `provenance` (`eligible_pool`, `runner_fallback`), `self_review` (`true`, `false`) |
| `workflow_quorum_slate_empty_total` | `role` |
| `workflow_participant_agent_unresolved_total` | `role` |

**One label value is reserved**, because one outcome has no bounded role.
`malformed_config` fires exactly when the declared role is missing, empty or a
string an operator typed, so using that string as the `role` label would break
`AC-OFFICE-REVIEW-SEATS-004.6` at the one increment where the label *is* the
unbounded thing. For that outcome `role` takes the fixed value `invalid` and
the operator's string appears only as a typed field on the warning record
(`AC-OFFICE-REVIEW-SEATS-004.11`) - the section's own split applied to its
awkward case rather than around it.

**Warning records.** Every increment above that represents a problem is
accompanied by a structured warning-level record emitted through the backend's
logger (`AC-OFFICE-REVIEW-SEATS-004.8`), carrying:

- a stable event name identifying the condition, one per condition, so records
  can be selected without matching on message text;
- the identifiers the corresponding acceptance criterion names - workspace,
  task, step, role, and where applicable the unresolved agent - as **separate
  typed fields**, never interpolated into the message string;
- the underlying error, when the condition arose from an error rather than an
  empty result.

The split follows the repository's existing metrics convention: counters carry
bounded labels and answer "how often"; records carry unbounded identifiers and
answer "which one". Neither alone diagnoses a parked task, the failure this
capability exists to make visible.

## Prior art

**Our own wiki returned nothing useful.** The vault path resolves but the host
refuses to read it from this worktree and neither query CLI is present, so no
prior position of ours on reviewer seating could be retrieved. That is a gap,
not an absence of prior thinking: query the wiki from a context that can read
the vault before assuming the reasoning below is novel.

**What other products shipped** (vendor claims, not evidence they work).
*Multica* is the closest analogue and reaches the same conclusion: its **squad**
is a workspace-level group whose members carry role descriptions that "grant no
permissions and never trigger members automatically" - casting is data a
coordinator consults, the separation drawn here between the caster and the
engine that reads its seats. It dedupes triggers so an already-queued member is
not enqueued twice (`AC-OFFICE-REVIEW-SEATS-003.2`), and calls Backlog "a
parking lot" it does not trigger on, matching the Office `backlog` step and why
that step stays a separate contract. *Warp* drives a state machine over issue
labels - casting-free, because a label carries no identity, and so unable to
express "these two agents must both approve", the quorum contract Office
already has. *Augment Code* and *OpenHands* ship a reviewer that runs against a
pull request with no casting layer, because in both the reviewer is a
product-provided capability rather than one of the customer's own agents.
Office cannot borrow that: its reviewers are the customer's agents, so identity
has to come from somewhere.

**What we do differently, and why.** Multica requires a squad to exist before a
reviewer does; an issue with no squad has no reviewer. Office cannot take that
position, because the defect being fixed *is* a task parking forever when
nothing is configured. This design adds what Multica leaves to the operator: a
total resolution that returns a seat whenever the workspace holds any agent,
falling back to the CEO and, in a single-agent workspace, to a recorded
self-review - a weaker review in the unconfigured case, accepted in exchange
for never producing a silently unfillable gate. We also state the
woken-equals-counted invariant (`REQ-OFFICE-REVIEW-SEATS-003`) as observable
behavior, which none of the surveyed products expose, because our two reads
already disagree on one filter and that divergence is invisible until a task
stalls.

## Related decisions

- **ADR-0004, task model unification.** Establishes the phase-2 participant and
  quorum action kinds this design extends, and defines `stage_type` as a
  semantic hint for rendering that the engine does not branch on. That is why
  this design selects gated steps by what their configuration declares rather
  than by stage type: stage type was never designed to be a unique selector,
  and nothing constrains a workflow to one step of a given type.
- **ADR-0005, agent model unification.** Migrated Office's own per-task
  participant table into the shared participant table and keyed it on the agent
  profile identifier, which is why a seat references a profile and why the
  Office adapter can return a profile identifier directly.
- **ADR-0027, replayable schema migrations across SQLite and Postgres.** Binding
  on the column addition, the index migration and its dedupe (*Persistence*),
  and on the runner resolver's dialect portability (*Casting resolution*).
- No new architecture decision record is required: this design adds a step
  action, one column, one index and one startup reconciliation routine to
  existing extension points with precedents in the codebase. The dependency's
  dispatch change completes an execution path ADR-0004 already specified; if
  the build finds that completing it requires changing how triggers are driven
  generally, a decision record becomes necessary and the build should stop and
  say so.

Round one cited "the workflow phase 2 decision record". No such record exists;
the three above are the real authorities.
