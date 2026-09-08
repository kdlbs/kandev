---
status: current
system: tasks
requirements:
  - REQ-TASKS-PLAN-WRITE-CONSISTENCY-001
  - REQ-TASKS-PLAN-WRITE-CONSISTENCY-002
  - REQ-TASKS-PLAN-WRITE-CONSISTENCY-003
  - REQ-TASKS-PLAN-WRITE-CONSISTENCY-004
  - REQ-TASKS-PLAN-WRITE-CONSISTENCY-005
created: 2026-08-31
owners:
  - kandev
---

# Task plan write consistency System Design

## Purpose and boundaries

This design closes the window between a plan read and its commit. It does not
change the truncation thresholds, the response
shape of a non-truncating write, or the missing-task boundary
owned by `docs/specs/tasks/system-design/plan-write-lifecycle.md`.

## Requirement mapping

Ids below abbreviate `REQ-TASKS-PLAN-WRITE-CONSISTENCY-00N`.

| Requirement | Design section |
| --- | --- |
| `001` | Chosen mechanism; Where the decision moves |
| `002` | Chosen mechanism; Retiring lock keys; Scope of the critical section; Existing behavior that must change |
| `003` | Scope of the critical section |
| `004` | Response contract |
| `005` | Scope of the critical section; Retiring lock keys; Failure and recovery |

## Current state

- Agent path: `internal/mcp/handlers.handleCreateTaskPlan` and
  `handleUpdateTaskPlan` call `evaluatePlanWriteGuard`, then
  `PlanService.CreatePlan` / `UpdatePlan` with `ForceNewRevision` from the guard.
- Browser path: `internal/task/handlers.wsCreateTaskPlan` and `wsUpdateTaskPlan`
  call the same methods and never set `ForceNewRevision`.

Before one agent update commits, plan state is read at least four times outside any
transaction: `evaluatePlanWriteGuard` reads `GetPlan` and the revision history,
`UpdatePlan` reads `GetTaskPlan` for its existence check, and `upsertPlan` reads
`GetTaskPlan` and `GetLatestTaskPlanRevision` again. All use the read-only pool,
opened by `internal/db.OpenSQLiteReader` with four connections, while the commit
happens later in `WritePlanRevision` on the single-connection writer pool.
`SetMaxOpenConns(1)` serializes commits against each other but not a reader against a
commit, so read(A), read(B), commit(B), commit(A) is unprevented.

`WritePlanRevision` already computes `max(revision_number) + 1` in its transaction.
The coalesce target and truncation decision are not safe.

## Chosen mechanism

Serialize the whole read, decide, commit sequence per task inside `PlanService`,
and evaluate truncation inside that critical section.

`PlanService` gains a per-task lock keyed by task ID, held across the state reads,
the truncation decision, the coalesce decision, the `WritePlanRevision` call and the
following re-read. Because every plan writer already funnels through
`PlanService`, taking the lock in the four public entry points, `CreatePlan`,
`UpdatePlan`, `RevertPlan` and `DeletePlan`, covers every path that writes a revision
or reads plan state to decide one; "Scope of the critical section" gives the boundary
and explains why the shared `upsertPlan` helper is not the acquisition point. Keying
by task ID satisfies `AC-TASKS-PLAN-WRITE-CONSISTENCY-002.6`: a write for one task
never waits on another's lock.

`PlanService.MarkImplementationStarted` is the one exported method that mutates a
`task_plans` row and is deliberately left outside the lock rather than overlooked: a
single atomic statement that reads no plan state and writes no revision, so it can
neither observe nor invalidate a decision made inside the section. It shares one
column with `upsertPlanHead`'s `ON CONFLICT` clause, `task_plans.updated_at`, and
writes it only on the first mark. The overlap is immaterial: no criterion constrains
the HEAD row's `updated_at`, and `canCoalesce` reads the *revision's* `UpdatedAt` on
`task_plan_revisions`, a different table.

### Retiring lock keys

Task identifiers are unbounded over a process lifetime, so entries must be
removed. Neither keyed-mutex precedent removes them: `parentMutex` in
`internal/task/service/handoff_service.go` and `keyedMutex` in
`internal/plugins/service.go`, the latter because its plugin id keyspace is "small
and long-lived". Task ids are neither, so neither may be copied without retirement.

Retirement has one silent failure mode. Delete an entry whenever its mutex is
released and a goroutine holding the old pointer locks that instance while a later
arrival locks a fresh one for the same task: two writers, no error, the race back.

The protocol that avoids it: one outer mutex guards the map, and each entry holds its
per-task mutex plus a count of goroutines holding or waiting for it. Acquire takes
the outer mutex, finds or creates the entry, increments the count, releases it, then
locks the entry mutex. Release unlocks the entry mutex, takes the outer mutex,
decrements the count, deletes the entry only at zero, and releases the outer mutex. Because the count is incremented before the outer mutex is released, no entry
can be deleted while a goroutine is still on its way to it.

Any implementation keeping both properties is acceptable: the count is mutated only
under the map guard, and deletion happens only at zero under it.

Within the section the reads see the state the write will commit against: no other
in-process writer can commit while the lock is held.

That rests on a second property the whole mechanism depends on: a read issued on the
read-only pool inside the lock must observe a commit made moments earlier on the
writer pool. It holds because the database runs in WAL mode, where a read transaction
takes its snapshot when it begins and sees everything committed before it started.
`_cache=shared` does not provide it; it only governs page-cache sharing.

This is easy to lose: every plan and MCP test helper passes one handle as both writer
and reader and cannot observe it, so `AC-TASKS-PLAN-WRITE-CONSISTENCY-002.10` requires
it over genuinely split pools. See "Test strategy".

Sound only within one process, which the requirements state as an exclusion.

### Where the decision moves

Truncation is decided in `internal/mcp/handlers` today, above the service, so a lock
inside the service cannot cover it. The decision moves into the write path and the
caller selects it rather than performing it:

- The write request carries a flag meaning "evaluate truncation for this write".
  The agent path sets it. The browser path does not, which is
  `AC-TASKS-PLAN-WRITE-CONSISTENCY-003.2` and `003.4`.
- The write result carries what the caller needs to render a warning;
  `PlanWriteResult` below names the fields.
- The MCP handler keeps ownership of the warning string. `planTruncationWarning`
  stays in `internal/mcp/handlers` with its "revision 0 means unknown" convention.
  An unknown number does not prove that history contains the replaced content.
  Therefore, that warning does not claim preservation or recovery. Its signature
  changes because it measures the code
  points itself today, but the handler no longer holds the replaced content, which
  exists only inside the critical section. It takes the counts instead,
  `(replacedRunes, newRunes, priorRevisionNumber int)`, and the measurement moves into
  the service, onto the content the write replaced.
- The detector moves with the decision. `planTruncationDetected` and its thresholds
  `planTruncationMinPriorChars` and `planTruncationMaxRetainRatio` are in
  `internal/mcp/handlers` today, and `internal/task/service` cannot import them:
  `mcp/handlers` imports `task/service` and no reverse import exists, so reusing them
  where the replaced content lives would be a cycle. They move to
  `internal/task/service`, keeping the single source of truth the thresholds
  exclusion assumes; no copy stays behind. `evaluatePlanWriteGuard` and
  `planWriteGuardResult` become dead and are deleted rather than kept as a thin
  flag-setter: their two callers set the request flag and read the result.
- The handler performs no plan or revision read after the service call. Rendering
  from a re-read would put the race back into the one path the warning exists to make
  trustworthy; returning the full prior content would copy a 40k character plan on
  every truncating write.
- The renderer assumes `newRunes < replacedRunes` and `replacedRunes > 0`, today
  pinned on its only caller invoking it after `planTruncationDetected` returned true.
  That carries: the handler renders only when the result's truncation flag is set,
  and the service sets it only where the comparison already holds.

`ForceNewRevision` remains for callers that force an append for their own reasons;
the in-section truncation decision also forces one
(`AC-TASKS-PLAN-WRITE-CONSISTENCY-001.5`).

Concretely: `CreatePlanRequest` and `UpdatePlanRequest` each gain one bool beside
`ForceNewRevision`, and `CreatePlan` and `UpdatePlan` return a result struct rather
than `*models.TaskPlan`.

```go
type PlanWriteResult struct {
	Plan                *models.TaskPlan
	TruncationDetected  bool
	ReplacedRunes       int
	NewRunes            int
	PriorRevisionNumber int // 0 means not established
}
```

The two browser call sites read `.Plan` and are otherwise unchanged. This keeps
`AC-TASKS-PLAN-WRITE-CONSISTENCY-004.1` and `003.2` true.

### Scope of the critical section

The lock is acquired in the four public entry points, `CreatePlan`, `UpdatePlan`,
`RevertPlan` and `DeletePlan`, in each case after identifier validation and
authorization and before that method's own first plan-state read.

For `RevertPlan` that first read is the target-revision fetch, not the HEAD read
after it. `mergeRevisionInTx` rewrites a revision row's title and content in place
when it is the coalesce target, so the latest revision is mutable, not immutable
history: a revert fetching that target outside the lock can commit content a
concurrent coalescing write already replaced. `upsertPlan`
does not acquire it; it becomes a helper whose documented contract is that its caller
already holds the task's plan write lock. That is what lets `UpdatePlan` put its
existence check in the same section as the write it guards.

The acquisition point is named because the obvious alternative fails. `UpdatePlan`
reaches `upsertPlan`, so locking in both takes a non-reentrant per-task mutex twice
on one path: every update self-deadlocks, permanently for that task, stranding the
refcount. `AC-TASKS-PLAN-WRITE-CONSISTENCY-005.5` states the invariant: the section
is entered once per call path, so no method that acquires may call another that
acquires.

Authorization runs before acquisition, next to identifier validation. It is a task
lookup wired to `AuthorizeTaskAccess`, so it performs I/O and there is no reason to
hold a task's write lock across it; this is also today's order.

The section ends after the post-write re-read, not at `WritePlanRevision`'s return.
`upsertPlan` and `RevertPlan` both re-read HEAD once the write has committed, and in
`upsertPlan` that re-read, not the in-memory value the write was assembled from, is
what the method returns. Released at the commit, that read could observe a queued
write's commit instead and hand the caller another write's content as the outcome of
its own, which `AC-TASKS-PLAN-WRITE-CONSISTENCY-002.5` forbids in the same response
`004.2` pairs with the truncation counts. It is kept rather than replaced by the
in-memory value because it also carries the persisted HEAD identity when the
pre-write read failed. See "Existing behavior that must change".

`DeletePlan` is in that list deliberately. Moving `UpdatePlan`'s existence check
inside the section is only worth doing if the deleter participates: an unlocked
delete can still commit between an in-lock check and the write it guards, so moving
the check alone changes nothing. `DeletePlan` writes no revision, so it is not a plan
write under the requirements' Terminology, but it mutates state every plan write
reads.
`AC-TASKS-PLAN-WRITE-CONSISTENCY-002.12` states the ordering this buys.

With the deleter inside, `UpdatePlan`'s existence check can move in too. It currently
reads `GetTaskPlan` before delegating to `upsertPlan`, which reads it again; the
redundant second read should collapse into one taken under the lock. Collapsing it
changes what a failed read must do, which "Existing behavior that must change"
settles.

The two reads it collapses serve opposite contracts and the surviving read keeps
both. `CreatePlan` tolerates an absent HEAD and appends
(`AC-TASKS-PLAN-WRITE-CONSISTENCY-001.8`, `002.11`); `UpdatePlan` must fail on one
(`002.12`) rather than recreate a plan a concurrent delete removed. The requirement
is therefore caller-supplied, not inferred from the read: `upsertPlan` takes a
`requireExistingHead` argument, set by `UpdatePlan` and clear on the `CreatePlan`
path, and returns `ErrTaskPlanNotFound` when it is set and the read returned
`absent`. `unknown` is not `absent` and never triggers it, per `001.6`. The same flag
marks the update path, so the HEAD-row title and author fallbacks stay gated to it
and `CreatePlan`'s defaults are unchanged (`004.1`). The `001.9` preserve flags are
not gated to it: they are computed on both paths, because a create over an existing
plan can omit a title too. Any equivalent shape is acceptable, including hoisting the
read into the entry points and passing the tri-state down; a single tolerant read
with no branch is not.

Serializing the delete does not close the loss path behind it. `DeletePlan` removes
only the HEAD row and leaves the revisions. A create that follows finds no HEAD,
emits no warning (`AC-TASKS-PLAN-WRITE-CONSISTENCY-001.8`), and under today's
coalesce rules can still merge into a surviving revision, overwriting pre-delete
content sequentially rather than by a race. `002.11` closes it by requiring an append
whenever the task has no HEAD at commit time.

The event publishes at the end of `upsertPlan` and `RevertPlan` should not hold the
lock: they call the event bus, which widens the section for no correctness benefit.

That is a restructuring, not a comment. The post-write re-read and both publishes sit
at the end of `upsertPlan` while the lock is owned by `CreatePlan` and `UpdatePlan`,
so the release point is unreachable from the acquisition point. A `defer release()`
in the entry point, which the panic rule in "Failure and recovery" pushes toward,
holds the lock across both publishes; releasing inside `upsertPlan` instead lets that
defer run a second time, decrementing the refcount twice and deleting an entry
another goroutine waits on, the failure "Retiring lock keys" exists to prevent.

`upsertPlan` stops publishing: it returns the saved plan plus the events its caller
is to emit, and the entry point acquires, calls it, releases, then publishes them in
today's order, plan event before revision event. Release is idempotent, so a deferred
release can coexist with an explicit one. Any implementation is acceptable that holds
three properties: one refcount decrement per acquisition, release on every exit path
including a panic, and no event published under the lock.

`DeletePlan` has no post-write re-read, so that boundary does not locate its section
end: it ends when the delete commits, and its `TaskPlanDeleted` publish moves outside
under the same rule.

### Existing behavior that must change

Five things in `PlanService` are not merely relocated by this design. Each is wrong,
or becomes wrong, against the contract, and none is discoverable above.

`resolveCoalesceWindow` maps a negative configured window to `defaultCoalesceWindow`,
five minutes, so `canCoalesce`'s `s.coalesceWindow <= 0` guard never sees a negative
value and a negative window coalesces normally today, while
`002.8` requires zero or negative to append every
write. The negative branch must stop substituting the default: preserve the value or
clamp it to zero, either of which reaches the guard. Only an absent configuration
falls back to five minutes. Without this an implementer believes `002.8` holds.

`upsertPlan` must append when the task has no HEAD at commit time, per
`002.11`. Today the coalesce decision consults only
the latest revision, so revisions surviving a delete remain eligible merge targets.

The collapsed reads must stop aborting the write. This is the largest of the five and
becomes visible only once the collapse happens. Two reads become one, twice over:
`upsertPlan`'s `GetTaskPlan` with the guard's `GetPlan`, and `upsertPlan`'s
`GetLatestTaskPlanRevision` with the guard's revision lookup. Each pair returns the
same state. But their halves have opposite failure contracts: the guard's reads are
tolerant, a failure forces an append and the write proceeds, which is what makes
`001.6` and `001.7` true, while `upsertPlan`'s return
the error and the write dies.

The merged reads take the tolerant contract:

- A failed HEAD read forces an append, emits no warning, and completes the write
  (`001.6`). Truncation cannot be evaluated without the replaced content, and
  appending stops a later write coalescing into an unknown prior revision.
- A failed latest-revision read forces an append and completes the write; when
  truncation was detected the warning is still emitted without a revision number
  (`001.7`). The rule is deliberately unconditioned on the truncation flag: after the
  collapse one call serves both paths, and appending is always safe, so `001.7`
  becomes an instance of the rule rather than an exception.

Both rules outrank the coalesce rules for that write, and
`005.1` states the precedence: its merge clause is
conditioned on the task having a HEAD and on the write having read it successfully,
so a failed read falls to `001.6` and an absent HEAD to `002.11`, both appending.
Coalesce behavior is unchanged for every write whose reads succeeded.

One consequence of that tolerance is not obvious. A failed HEAD read is not "this
task has no HEAD", and `upsertPlan` cannot tell them apart today: both arrive nil.
They must be distinguished as found, absent or unknown, because that value decides
four things besides truncation: the event type, the HEAD identity, `UpdatePlan`'s
title and author fallbacks, and `UpdatePlan`'s existence gate. Under unknown the
write emits the plan updated event rather than the created one; a spurious create
asserts something false about a task's history. It supplies no HEAD identity, which
is safe as long as the re-read succeeds: `upsertPlanHead` fills an empty `ID` and a
zero `CreatedAt` with a fresh UUID and `now` in Go, unconditionally, on the caller's
own struct, before the statement runs, and its `ON CONFLICT(task_id) DO UPDATE`
clause then writes neither column, so an existing row keeps its real ones and the
caller receives those from the in-section re-read. The fabricated pair survives only
in memory, which is what makes a failed re-read a special case in "Failure and
recovery". An implementer
reusing the nil result for unknown ships a write that succeeds, persists correctly
and emits the wrong event type.

The metadata fallbacks are the least obvious and the most damaging to get wrong.
`UpdatePlan` takes `title` and `created_by` from the HEAD row whenever the request
omits them, and both callers may legitimately omit a title. Under unknown those
fallbacks have nothing to read and today's defaults take over: `upsertPlan`
substitutes the literal `Plan` and the author is re-resolved, so a transient read
failure would silently rename the user's plan and rewrite its author, on a write
`001.6` requires to succeed. The write must instead
leave the stored title and `created_by` as they are: the values it could not read are
the values it must not replace.

Making the `ON CONFLICT` assignments conditional on the value being empty does not
achieve that, because nothing empty reaches the statement. Three substitution sites
exist, two on this path: `upsertPlan` replaces an empty title with `Plan` and
`resolveAuthor` resolves the author before the repository is called, then
`upsertPlanHead` substitutes both again. Emptiness is observable only at the top of
`upsertPlan`, so preservation is captured there and carried down as an explicit
signal: `upsertPlan` computes one preserve flag per column from the HEAD read being
unknown and the request field being empty, before its own substitution runs, and
passes both flags through `WritePlanRevision` to `upsertPlanHead`. Neither
substitution changes; they still supply the insert branch's defaults. The flags bind
into the update branch alone,
`title = CASE WHEN ? THEN task_plans.title ELSE excluded.title END` and likewise for
`created_by`, so a task with no HEAD row still inserts the default while an existing
row keeps its stored value. Only `content` and `updated_at` are written
unconditionally. After the upsert, the same transaction reads the stored title and
author. The repository returns these values in the HEAD object. It also uses the
stored title in the new revision. Thus, a later revert cannot apply a fallback title.

Two consequences. `WritePlanRevision` is on the exported `PlanRepository` interface,
so this changes it and every implementation; "Where the decision moves" counts
them. And all four production entry points default `CreatedBy` before
`PlanService` is entered, `agent` on MCP and `user` on the browser, so only the title
half can fire in production today. `001.9` is a service-boundary contract and its
author half is tested there, not through a handler.

`RevertPlan` must stop aborting on its own HEAD read, taken after the target-revision
fetch and whose error it currently returns. A revert writes a revision, so it is a
plan write under the requirements' Terminology and
`001.6` governs it unchanged: an unreadable HEAD
forces an append, emits no warning, and the revert completes. This read is not half
of either collapsed pair, which is why it survives a fix aimed only at those.

`CreatePlan` must validate its identifier before acquiring. The empty-task-id check
lives inside `upsertPlan` today and `CreatePlan` reaches it only through that helper,
so acquiring in the entry point would lock on the empty string and only then reject
the request, serializing every malformed request against every other, which
`005.4` forbids. The check moves up; the other three
already validate first.

## Alternatives considered

**Move truncation detection and the force-append decision inside
`WritePlanRevision`'s transaction.** Robust across processes, because the decision is
made from state read inside the write transaction. Rejected because it puts an
agent-facing presentation heuristic (the 2000 character floor, the retain ratio, code
point counting) into the SQLite repository, and because the coalesce decision would
have to move there too or a second window stays open. Its `WritePlanRevision` change
no longer separates the two: the chosen mechanism changes that method as well.

**Return the written revision number and derive the preceding revision as
`written - 1`.** Rejected as insufficient rather than wrong. It repairs `001.2` in
the warning path, where the write is always an append so `written - 1` really is the
preceding revision. It does not repair `001.1`, `001.3` or any of `002`, whose
decisions are still made from state read before the commit. It closes one symptom, not the race.

## Response contract

No change for a write that emits no warning, satisfying `AC-TASKS-PLAN-WRITE-CONSISTENCY-004.1`.
`planWriteResponse` still embeds `*dto.TaskPlanDTO` and `dto.TaskPlanDTO` is
untouched, so the browser plan editor is unaffected. A truncating agent write still
adds `plan_write_warning` and, when established, `prior_revision_number`.

## Failure and recovery

- Current plan content unreadable, or the preceding revision number unestablished:
  append and complete the write, with no warning in the first case and, in the
  second, a warning without a revision number when truncation was detected
  (`001.6`, `001.7`). Revision numbers start at 1, so
  0 still means unknown. Both rules, and why the append is unconditioned on
  truncation, are in "Existing behavior that must change"; neither is today's
  `upsertPlan` behavior, which aborts.
- Post-write re-read fails, or succeeds reporting no row, after the write has
  committed: report the success (`AC-TASKS-PLAN-WRITE-CONSISTENCY-005.8`). Both
  reach the service as a nil plan, and a task
  deleted after the commit cascades the HEAD row away, so it is real. The
  transaction is durable and cannot be rolled back, so the service
  returns the plan it assembled in memory, from the request's content plus whatever
  HEAD identity it holds, and publishes the events it would otherwise have published.
  Returning the error or `ErrTaskPlanNotFound` instead, which is what `upsertPlan`
  does today, tells the caller a committed write failed and sends an agent into the
  retry-or-rewrite-from-memory behaviour the warning exists to prevent. When the
  pre-write HEAD read was itself unknown the service has no identity to report: the
  struct it handed the repository carries only the fabricated pair "Existing behavior
  that must change" describes. It clears both before returning, so the caller reads
  the identity as unknown rather than as a row that does not exist. The repository
  has already resolved the stored title and author in the write transaction. The
  fallback result and its event can report that metadata as authoritative.
  (`005.8`). This path cannot tell the insert branch apart, so it clears
  unconditionally. `RevertPlan` is governed by the same rule and
  needs no substitute: it returns the revision it wrote, whose id, number and
  timestamps `insertNewRevisionInTx` populated in place before the commit, and
  publishes with the same cleared identity.
- Task deleted between validation and commit: unchanged. The foreign key
  violation classifies to `ErrTaskNotFound` and the transaction rolls back, per
  `plan-write-lifecycle.md`.
- A write that fails must release the lock, including on a panic, so `005.3`
  holds. Release must not depend on the happy
  path and must run the full protocol above: skipping the refcount decrement leaks an
  entry permanently, the bound "Retiring lock keys" exists to hold.
- Identifier validation and authorization run before the lock is taken, so an empty
  identifier is never a lock key, such requests do not serialize against each other
  (`005.4`), and no write lock is held across an
  authorization lookup.
- Only the per-task plan write lock is held inside the section (`005.5`); nothing
  in it may acquire a second.
- A caller whose context is cancelled while queued is not released early
  (`005.7`). A plain `sync.Mutex` is not
  context-aware and stays that way: the section is one set of reads plus one short
  transaction, so the wait is already bounded. The write acquires in its turn, fails
  when it reaches its write transaction, and releases normally, which `005.3`
  covers. Its reads before that transaction run on a context decoupled from the
  caller's, `context.WithoutCancel`, so cancellation cannot divert the write into
  `001.6`'s unreadable-HEAD path and make it succeed. `001.6` fires on a genuine
  read failure only.

## Test strategy

The seam is the repository, not the service: `Handlers.planService` is a concrete
`*service.PlanService`, and `internal/mcp/handlers/task_plan_guard_test.go` already
builds handlers over a wrapped repository
(`newMCPPlanTestHandlersWithFailingRevisionLookup` wraps `sqlite.Repository` and
injects failures through it). The concurrency test required by
`AC-TASKS-PLAN-WRITE-CONSISTENCY-001.3` and `002.1` through `002.3` uses that wrapper
to block a chosen call, so the interleaving is forced rather than raced.

It cannot be written as "block A inside the section, then commit B for the same
task": under this design B cannot commit while A holds the lock, which is the
property being tested. B is observed to queue instead, and the assertions are about
the state each write decided against:

1. Seed a plan large enough to cross the 2000 character floor.
2. Start write A, a truncating agent write, and block it inside the section at a
   wrapped repository call, after it has read plan state.
3. Start write B for the same task from the other path. Observe that B has not
   committed: it is queued on the same per-task lock.
4. Release A and let it commit, then let B proceed to its own commit.
5. Assert that A's counts describe the content A replaced and the revision it names
   holds that content; that B's decisions were made against the state A committed,
   not what B could have read before queuing; and that HEAD equals the latest
   revision.

Step 3 must go through a write path: committing B through the repository directly
would bypass the lock, exercise a writer class the requirements exclude, and pass
unchanged against an implementation with no serialization.

The pool split matters twice. Existing helpers build the repository with
`sqlite.NewWithDB(sqlxDB, sqlxDB, nil)`, one handle as both writer and reader, and
nothing under `internal/task/` or `internal/mcp/` opens `OpenSQLiteReader` today.
That is why the interleaving must be forced rather than left to pool timing, and why
`AC-TASKS-PLAN-WRITE-CONSISTENCY-002.10` needs its own test over one file with a real
`OpenSQLite` writer and a real `OpenSQLiteReader` reader: without it this design's
read-after-write visibility is asserted by prose alone.

One existing test must be rewired rather than left to pass.
`TestMCPPlanTruncationGuard_RevisionLookupFailureOmitsRevisionNumber` is `001.7`'s
only coverage and injects into `ListTaskPlanRevisions`, the call the collapse
removes; left alone it passes while exercising nothing. Its seam moves to
`GetLatestTaskPlanRevision`. Any test asserting a write aborts on a HEAD read failure
has its expectation inverted: rewire it, do not delete or weaken it. Tests calling
`evaluatePlanWriteGuard` directly move to the service result.

Additional coverage: a write for one task commits while a write for another is held
inside the section (`002.6`); boundaries at exactly 2000 replaced characters and
exactly half retained (`001.4`); a case whose byte and code point lengths differ
(`001.1`); browser path emits no warning and keeps its response shape (`003.2`);
revert appends and does not warn (`003.3`); a write failing inside the section is
followed by a successful write to the same task (`005.3`); a same-author repeat
inside the coalesce window adds no revision while one outside it appends (`005.1`);
a negative configured window appends every write (`002.8`); a delete followed by a
same-author create inside the window appends rather than merging into a surviving
revision (`002.11`); a delete forced to interleave with a write is ordered
(`002.12`); a queued write whose context is cancelled fails and does not block the
next write to that task (`005.7`); a failed HEAD read appends, emits no warning,
completes the write and emits the plan updated event rather than the created one
(`001.6`); a failed latest-revision read appends and completes the write where
truncation was not detected; a revert whose own HEAD read fails appends, emits no
warning and completes; a write that omits a title and whose HEAD read fails leaves
the stored title and `created_by` unchanged rather than substituting `Plan`; a
post-write re-read that fails, and one that reports no row, each returning success
with the revision durable and no plan identity rather than the pair `upsertPlanHead`
generated in memory, once through a create or update and once through a revert; an
unknown-HEAD update that verifies the stored title in HEAD, the new revision, the
write result, and a later revert; a divergent HEAD and latest revision that verifies
the warning names no unverified revision; an environment-gated PostgreSQL case for
the metadata-preservation branches; a queued cancellation that observes the waiter
registration before it cancels the caller; an update whose HEAD read returns absent
fails rather than recreating the plan, while
one whose HEAD read is unknown proceeds; and a revert to the latest
revision forced to interleave with a same-author
coalescing write commits the target content, not the content that write merged in.

## Implementation note: starting state

The parent card calls a related change "already implemented and merged". It is not
merged: the work sits only on `feature/plan-truncation-guar-qd5`, which has been
rebuilt and extended since, so identify it by subject rather than by commit id. On
`origin/main` `evaluatePlanWriteGuard` still calls `PlanService.ListRevisions`, there
is no `GetLatestRevision`, and the false `ListRevisions` doc comment remains. It touches `task_plan_guard.go` and
`plan_service.go`, which this work also touches: confirm its merge state and rebase
rather than reimplement.
