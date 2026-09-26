---
status: draft
system: office
requirements:
  - REQ-OFFICE-SESSION-IDENTITY-001
  - REQ-OFFICE-SESSION-IDENTITY-002
  - REQ-OFFICE-SESSION-IDENTITY-003
  - REQ-OFFICE-SESSION-IDENTITY-004
---

# Office Task Session Identity System Design

## Purpose and boundaries

This design places enforcement of "one live office session per pair" on the
office creation path and makes the office lookup deterministic. It changes two
functions in the task repository and one recovery branch in the executor, and
adds no column, no index, and no migration.

The boundary that matters: `task_sessions` has six insert entry points, all
funnelling into one private `createTaskSession`. Exactly one,
`CreateOfficeTaskSession`, is the office path, and its sole production caller is
the executor's `persistOfficeSession`; the other five carry kanban relaunch,
workflow replacement, and spawned sessions. Enforcement attaches to the office
entry point, above the shared private insert, which makes
`AC-OFFICE-SESSION-IDENTITY-004.1` through `.4` true by construction rather than
by predicate tuning.

## Baseline

This design is stated against commit `a96f0d51b`, which is this branch's `HEAD`
and also its merge base with `origin/main`. Every "currently", "already", and
"no change to" below describes that tree. `origin/main` has since moved ahead of
it; the merge base, not the remote tip, is the reference. This pin is exact, and
it is the only part of this section that does not age.

The working tree is **not** that tree. It carries uncommitted work from card
`6def9e4c`'s abandoned attempt, and `6def9e4c` kept acting on it after this
design was written, so no snapshot here describes it. **Build shall re-derive
the tree state at the start of implementation** with `git status --porcelain`
and decide with "The go/no-go test" below rather than by matching a list. Expect
files on this design's own "Files expected to change" list to already carry
another card's edits; expect `base_schema.go` not to, and stop if it does.

### The go/no-go test

The question is not which files are dirty - dirty files here are expected, and
this design modifies three of them itself. It is whether the tree contains
anything violating `AC-OFFICE-SESSION-IDENTITY-004.1`. The two stop clauses are
independent and **either one alone stops the card**, so a partial match is a
decision, not an ambiguity; proceeding requires that neither has matched. Where
Build cannot determine whether an artifact is reachable from `initSchema`, it
counts as present: the default on ambiguity is to stop.

- **Neither stop clause matches: proceed**, however many files are dirty and
  even if none are. This is the expected state. Build implements on top of the
  existing edits, reconciling them to this design wherever they conflict.
- **Any schema artifact present: stop and raise a blocker against `6def9e4c`.**
  Concretely: a modification to `base_schema.go`, an index-creating or
  row-healing function reachable from `initSchema`, or a migration file. Any one
  violates `004.1` on arrival and would create the index on the next boot; one is
  enough, and the absence of the others is not mitigation.
- **A tripwire test present in non-baseline form: stop and raise a blocker.**
  Specifically
  `TestCreateTaskSessionWithInitialRuntimeSeedConsumesOnceAcrossConcurrentAndReplacementSessions`
  asserting one conflict and one row rather than two successful creates and two
  rows. This design requires it in baseline form (see "What must be proven"); a
  rewritten one is restored by the card that rewrote it.

No clause here licenses reverting another card's uncommitted work unasked.
Raising the blocker is the action; editing their tree is not.

### The residual `office_task_session_uniqueness*` files

`office_task_session_uniqueness.go` and its test are staged in the index and
carry the previous attempt's dual-dialect UNIQUE-violation classifier. They are
**not** a schema artifact, so on their own they neither trip the go/no-go test
nor block this card. This design does not need them: with an in-transaction
guard the sentinel is returned directly and no driver error string is parsed.

**The classifier is not inert, though.** `isOfficeTaskSessionUniqueViolation`
has two live call sites in `session.go`: the INSERT branch in
`createTaskSession` (`:949`), and an UPDATE branch in
`updateTaskSessionWithStateGuard` (`:1448`) that `6def9e4c` added and the
baseline does not have, reachable from Change 3's own recovery through
`rebindOfficeSessionExecutionProfile`. Both are dead only because no index
exists, and both carry a comment asserting "no index enforces this pair yet"
pointing at the doc comment this design requires Build to rewrite to say the
opposite. **Build shall delete both branches**, leaving plain `return err` and
`return false, err`; `session.go` is already on the change list. Keeping them
would give `ErrOfficeSessionRaceConflict` three producers, one an UPDATE inside
the recovery path, so `AC-OFFICE-SESSION-IDENTITY-003.7` would hold only by the
accident that no index exists - and would leave an enforcement branch in the
shared private insert `## Change 1` requires to stay enforcement-free.

The two **files** stay untouched: whether they are kept, reworked, or removed is
`6def9e4c`'s disposition, as `## Out of scope` in the requirements records.
Deleting the call sites leaves the function referenced only by its own test,
which is intended; if a linter objects to that, or if the files are still
present at PR time, Build says so in the PR description rather than restoring a
call site.

## Root cause

The office find-or-create reads and writes through different connection pools.
`GetTaskSessionByTaskAndAgent` executes against `r.ro`, the read-only pool,
opened with four connections. `CreateOfficeTaskSession` executes against `r.db`,
the writer, opened with `SetMaxOpenConns(1)`. `EnsureSessionForAgentWithCreation`
therefore reads on a reader connection, decides, then inserts on the writer, and
nothing holds across the two: four concurrent callers each read "no session for
this pair", each decide to create, and each insert succeeds, because the writer
transaction checks only `SELECT COUNT(*) FROM task_sessions WHERE task_id = ?`
for the initial-origin marker and never checks the pair. The pool is four wide
and the observed incident produced four rows in five seconds.

The fix is to move the pair check inside the writer transaction, where mutual
exclusion already exists.

## Why the transaction already suffices

`CreateOfficeTaskSession` opens a transaction and, on PostgreSQL, takes
`SELECT id FROM tasks WHERE id = ? FOR UPDATE` first, to keep the initial-origin
marker correct. That lock gives exactly the property the guard needs.

- **PostgreSQL.** Two concurrent calls for one task serialize on the task row
  lock. The second blocks until the first commits, then, under the default
  READ COMMITTED isolation, its guard query observes the committed row.
- **SQLite.** The writer pool is one connection, held by a transaction for its
  whole lifetime, so a second office insert cannot begin until the first commits
  or rolls back. The driver's deferred-transaction subtlety does not apply:
  exclusion comes from the pool, not the transaction's lock mode.

`AC-OFFICE-SESSION-IDENTITY-001.1` is therefore satisfied by one query inside an
existing transaction, with no new locking primitive. The limit of the argument is
in the requirements' "Out of scope": it holds within one process.

## Change 1 - the creation guard

Inside `CreateOfficeTaskSession`'s transaction, after the task lock and before
the insert, query whether the pair already holds a live row, and refuse if so.

The predicate is written as the complement of the terminal set, satisfying
`AC-OFFICE-SESSION-IDENTITY-001.4`:

```sql
SELECT COUNT(*) FROM task_sessions
 WHERE task_id = ?
   AND agent_profile_id = ?
   AND state NOT IN ('COMPLETED', 'FAILED', 'CANCELLED')
```

A non-zero count returns `ErrOfficeSessionRaceConflict` and rolls the
transaction back, so nothing is written and no existing row is touched
(`AC-OFFICE-SESSION-IDENTITY-001.2`, `.6`). A zero count proceeds to the insert,
keeping the terminal-retry path working (`.3`). An empty `agent_profile_id` is
stored as SQL `NULL` by `createTaskSession`; `NULL = ?` never matches and such a
row names no pair, so the guard is skipped for it outright rather than relied
upon to match nothing (`AC-OFFICE-SESSION-IDENTITY-001.5`).

The guard belongs in `CreateOfficeTaskSession`, not `createTaskSession`: the
private helper is shared with the five non-office entry points, and putting the
guard there would reproduce exactly the regression this card exists to avoid.

The executor's `persistOfficeSessionFallback`, used when the repository does not
implement the office creator interface, already serializes on a per-task mutex
and already lists the task's sessions via `ListTaskSessions(ctx, taskID)`. It
applies the same live-pair rule under that lock to satisfy
`AC-OFFICE-SESSION-IDENTITY-001.7`. That list is task-scoped, not pair-scoped,
so the rule is applied in Go, and the order matters: restrict to rows whose
`agent_profile_id` equals the one being inserted, then test those rows for
terminality. Testing state first would refuse a legitimate insert whenever any
other agent on the same task happened to be live.

The predicate is the executor package's existing `isStopTerminalSessionState`
(`executor_execute.go`), which already tests exactly `COMPLETED`, `FAILED` and
`CANCELLED`, and which lives in the same package as the fallback. It is the
single Go implementation `AC-OFFICE-SESSION-IDENTITY-001.10` requires; no second
one is written for this path. `models.IsResumableSessionState` is not usable
here - it excludes `CREATED`, which this contract classifies as live, and its own
doc comment says it answers a different question. The abandoned attempt's
enumerated live list is not usable either: `AC-OFFICE-SESSION-IDENTITY-001.4`
forbids an allow-list.

## Change 2 - the lookup ordering

`GetTaskSessionByTaskAndAgent` currently orders `started_at DESC LIMIT 1` with
no state preference. Change 1 alone would therefore permanently fail any pair
whose newest row is terminal while an older row is live: the caller reads
terminal, decides to create, the guard refuses, the caller re-reads, gets the
same terminal row, and fails again on every wakeup forever. That state exists in
the shipped database today - task `62201cdb`, agent profile `3628b4c9-3`, has a
`COMPLETED` newest row above three live rows - so Change 1 without Change 2 would
make that task permanently unstartable.

The ordering becomes live-first, then recency, then a total tiebreak:

```sql
ORDER BY CASE WHEN ts.state IN ('COMPLETED', 'FAILED', 'CANCELLED')
              THEN 1 ELSE 0 END,
         ts.started_at DESC,
         ts.id DESC
LIMIT 1
```

The `CASE` expression, `DESC` and `LIMIT` are standard on both dialects,
satisfying `AC-OFFICE-SESSION-IDENTITY-002.5`. The `id` tiebreak makes selection
total and reproducible and carries no recency (`002.3`). No tied group exists in
current data.

The three other callers - the office session terminator, the orchestrator's
session-ensure path, and the office test harness - all want "the current session
for this pair", and preferring a live row over a dead one is strictly more
correct for each, so no per-caller variant is needed. A pair carrying legacy
duplicate live rows now resolves to one of them and is usable, which is how
`AC-OFFICE-SESSION-IDENTITY-003.6` is met without a backfill: the extra rows stay
in place and simply stop being selected.

## Change 3 - bounded recovery

The executor's existing conflict branch re-reads the pair and reuses what it
finds. Three gaps remain against REQ-OFFICE-SESSION-IDENTITY-003.

If the re-read yields nothing usable - possible only in the narrow window where
the blocking row goes terminal between the guard and the re-read - the branch
currently falls through and returns a nil session with the conflict error. That
becomes a bounded retry of the create-and-recover sequence, at most one
additional attempt, then the conflict error (`AC-OFFICE-SESSION-IDENTITY-003.3`).
It must also never return a nil session with a nil error (`003.4`), and reuse
must continue not to publish a second `CREATED` lifecycle event, which the
existing "was created by this call" return value already carries (`003.5`).

The third gap is that the branch discards an error. It reads
`raced, lookupErr := e.repo.GetTaskSessionByTaskAndAgent(...)` then tests
`if lookupErr == nil && raced != nil`, so a genuine lookup failure is
indistinguishable from "no row for the pair" and the call returns the conflict
sentinel for what may be a disk, permission, or transport fault.
`AC-OFFICE-SESSION-IDENTITY-003.7` closes this: only a refusal under `001.2` is
a conflict and only a conflict drives the bounded retry. Any other create
failure, and any re-read failure, is returned as itself, wrapped so `errors.Is`
still reaches the original. The bound is therefore two create attempts per call
at most, and a non-conflict failure ends the call at the attempt that produced it
rather than consuming the retry.

With Change 2 in place the common recovery is deterministic: the guard fires only
when a live row exists and the lookup now prefers live rows, so the re-read finds
the row that caused the conflict unless it has since gone terminal.

### Divergent execution profiles during recovery

`AC-OFFICE-SESSION-IDENTITY-001.9` scopes its convergence claim to recoveries
carrying the same execution profile and names last-writer-wins for the divergent
case. The mechanism, recorded here so the criterion need not carry it: the pair
is keyed on the stable agent identity (`agentInstanceID`, from
`dbTask.AssigneeAgentProfileID`) while the execution profile is a *separate*
per-call parameter `createStartSession` takes from the launch request, so two
wakeups can legitimately resolve different profiles for one pair.
`rebindOfficeSessionExecutionProfile` (`executor_office.go:100-165`) then guards
its update on the row's **state**, via
`UpdateTaskSessionIfCurrentStateRemovingMetadataKeys`, not on its execution
profile - so two concurrent rebinds sharing an expected state both succeed, the
last wins, and each caller returns the in-memory session it built rather than a
re-read.

This is shipped behaviour and Change 3 must not alter it. Closing the window
would mean guarding the rebind on the observed execution profile as well as the
state, or serializing recovery per pair; both change shipped reuse semantics and
no criterion here depends on either, which is why the requirements name it out of
scope rather than leaving it unstated. Change 3 changes only frequency: a
conflict-driven second attempt re-enters the same recovery, so the two writes
`AC-OFFICE-SESSION-IDENTITY-001.9` permits can occur twice. It adds no new class
of write, and the two-attempt cap bounds the repetition.

## Files expected to change

| File | Why |
|---|---|
| `internal/task/repository/sqlite/session.go` | the guard in `CreateOfficeTaskSession`; the ordering in `GetTaskSessionByTaskAndAgent`, plus that function's comment, which describes a `uniq_office_task_session` index that does not exist; deleting the two residual classifier branches named under "Baseline" |
| `internal/orchestrator/executor/executor_office.go` | bounded recovery in `EnsureSessionForAgentWithCreation`; the live-pair rule in `persistOfficeSessionFallback` |
| `internal/task/repository/sqlite/errors.go` | the sentinel's doc comment, which must describe an in-transaction guard rather than an index |

No change to `base_schema.go`, `base_migrations.go`, or any rebuild block;
`AC-OFFICE-SESSION-IDENTITY-004.5` forbids it. That describes what this card
adds to the baseline above, not the working tree as found.

## What must be proven

**Test doubles: what is permitted.** Several bullets below need an interleaving
or a failure that data setup alone cannot produce, and there is nothing to
inherit: no backend test references `EnsureSessionForAgentWithCreation`, and no
double in `internal/orchestrator/` implements `CreateOfficeTaskSession`.
Build **may** author a double that wraps the real repository and controls only
*timing and failure* - blocking a call until another goroutine has passed a
chosen point, counting calls, or returning a chosen error from one named method
on a chosen attempt. It **may not** replace the behaviour under test: guard
decision, ordering, terminal classification and recovery all run as production
runs them, and the double implements `officeTaskSessionCreator` wherever a
bullet says the transactional path is proven. Holding a loser until the winner
has committed is permitted; returning the sentinel without consulting the guard
is not, and neither is stubbing `GetTaskSessionByTaskAndAgent` to return a row
it did not read. "Must not be satisfied by an injected conflict" below bans
faking the *refusal*, not this seam.

- A real concurrent office create for one pair yields exactly one row, with the
  sentinel raised unfaked (`AC-OFFICE-SESSION-IDENTITY-003.1`, `.2`). **This test
  must be authored; no existing test has its shape.** N goroutines, N >= 2, call
  `CreateOfficeTaskSession` on one task with an *identical* `agent_profile_id`;
  exactly one returns nil, every other returns an error satisfying
  `errors.Is(err, ErrOfficeSessionRaceConflict)`, and a subsequent list shows
  exactly one row for the pair. It must not be satisfied by an injected conflict.

  Do **not** model it on
  `TestCreateOfficeTaskSessionMarksOnlyTheFirstConcurrentSessionAsOrigin`, the
  only existing office concurrency test: it races `agent-1` against `agent-2` -
  two *different* pairs on one task - and asserts **both** creates succeed, so
  copying its shape produces a test that never reaches the guard. That is also
  why the guard must key on the pair and never on the task alone. It must pass
  unmodified, and so must its PostgreSQL twin
  `TestPostgresCreateOfficeTaskSessionMarksOnlyTheFirstConcurrentSessionAsOrigin`.
- The *convergence* half of `AC-OFFICE-SESSION-IDENTITY-003.1` is proven at the
  layer that delivers it. The bullet above races `CreateOfficeTaskSession`,
  where every loser receives an error; that proves "exactly one row" but cannot
  observe "and shall return that same row to all N callers", which lives in the
  executor's recovery (`executor_office.go:84-97`). **This test must also be
  authored.** N goroutines, N >= 4 so the race is at least as wide as the reader
  pool that produced the incident, call `EnsureSessionForAgentWithCreation` for
  one pair on a task with no session for it; every call returns a nil error,
  every returned session carries the same `ID`, exactly one reports
  `created == true`, and a subsequent list shows exactly one row for the pair.

  Those four assertions are **not sufficient alone and must not ship alone.**
  They all hold on an interleaving in which every goroutine after the first finds
  the row on its *initial* lookup and returns through ordinary reuse, so the
  recovery branch never runs and the test is green whether or not Change 3 works
  - the one thing it exists to distinguish. The test therefore uses the seam
  above to force at least one caller past its initial lookup into
  `CreateOfficeTaskSession`, and asserts through the seam's call counting that at
  least one such call was refused under `001.2` and that its caller still
  received a session. A run in which none reaches the recovery must **fail**.

  The nil-error assertion is the load-bearing one of the four:
  REQ-OFFICE-SESSION-IDENTITY-003's Intent is that a caller which loses the race
  must not receive an error, while the repository-level bullet above
  deliberately asserts the opposite at its own layer. It must not be satisfied by
  an injected conflict, and it must run against a repository implementing
  `officeTaskSessionCreator` - not incidental, since `persistOfficeSession`
  branches on that, so a double that does not implement it exercises the
  in-process fallback instead of the transactional guard and would pass while
  proving nothing about the path production takes.
- A terminal-only pair still accepts a new row (`AC-OFFICE-SESSION-IDENTITY-001.3`).
- A pair shaped like `62201cdb` - terminal newest, live older - resolves to the
  live row and does not fail (`AC-OFFICE-SESSION-IDENTITY-002.1`, `003.6`). This
  is the regression test for the livelock Change 2 exists to prevent.
- A refusal writes nothing: the pair's rows are byte-identical before and after
  a refused create, and no row is added (`AC-OFFICE-SESSION-IDENTITY-001.6`).
  Separately, the recovery that follows a refusal may still write to the row it
  reuses - an `IDLE` row is observed to reach `RUNNING`, and a rebind clears that
  row's `metadata.acp_session_id` - while every other row for the pair is untouched
  (`AC-OFFICE-SESSION-IDENTITY-001.9`). Proving only the first would lock in a
  suppression of shipped behaviour. The second half is not reachable by seeding -
  a pre-seeded `IDLE` row is found by the first lookup and reused - so force the
  refusal with the seam above.
- A non-conflict failure is not laundered into the sentinel: when the create
  fails for a reason other than the guard, or when the recovery re-read itself
  fails, the caller receives that error and `errors.Is(err,
  ErrOfficeSessionRaceConflict)` is false, and no retry is spent
  (`AC-OFFICE-SESSION-IDENTITY-003.7`). Neither failure is reachable by data
  setup; both come from the seam above.
- The three non-office flows still create a second live row for a shared pair.
  Each needs its own named proof; the previous attempt was caught only because
  the first of the three happened to have one. All three ids below are
  `AC-OFFICE-SESSION-IDENTITY-*`.
  - **Kanban relaunch (`004.2`).**
    `TestCreateStartSession_KanbanRunnerCreatesDistinctSession`
    (`internal/orchestrator/task_operations_test.go`) exists, must pass
    unmodified, and is the tripwire that caught the previous attempt.
  - **Workflow replacement (`004.3`).**
    `TestCreateTaskSessionWithInitialRuntimeSeedConsumesOnceAcrossConcurrentAndReplacementSessions`
    (`session_test.go`) is the existing repository-level proof and must pass in
    its **baseline** form. Replacement reaches the shared insert through
    `prepareSession` -> `createPreparedSession` ->
    `CreateTaskSessionWithInitialRuntimeSeed`, and that test races two sessions
    carrying one *identical* `agent_profile_id` on one task through exactly that
    entry point, asserting both succeed and two rows result - a direct tripwire
    for enforcement leaking off the office path. The abandoned attempt rewrote it
    (see "Baseline"): a version asserting one conflict is the regression, not the
    target. `PrepareSessionForExistingEnvironment`'s own two tests are negative -
    they reject an unusable workspace before any session is created - and prove
    nothing here.
  - **Spawn profile inheritance (`004.4`).**
    `TestHandleSpawnSession_SameTask_DefaultsToSenderProfile`
    (`internal/mcp/handlers/spawn_session_test.go`) locks the inheritance
    default against a live `RUNNING` spawner session on the same task and must
    pass unmodified, but it proves only profile *resolution*: it asserts against
    a mock orchestrator's recorded launch calls and writes no `task_sessions`
    row, so it would still pass if the resulting insert were refused. The missing
    half - that the insert is not refused - is **already covered; author nothing
    new.** The entry point was traced, not assumed: `handleSpawnSession` calls
    `LaunchSession` with `IntentStart`
    (`internal/mcp/handlers/spawn_session.go:46-51`), reaching `launchStart` ->
    `startTask` -> `prepareSessionForStart` -> `createStartSession`
    (`internal/orchestrator/task_operations.go:1399`, `:1591`) - the entry point
    kanban relaunch uses.
    `TestCreateStartSession_KanbanRunnerCreatesDistinctSession`, named above for
    `.2`, seeds a live `RUNNING` session for the pair and asserts
    `createStartSession` still returns a *distinct* session id, so it proves for
    spawn exactly what it proves for relaunch. Build cites it for both criteria;
    what must not happen is recording it as proven by the resolution test alone.

    Do **not** author an office-task version. Where `IsFromOffice` is true and
    the task carries an assignee, `createStartSession` routes to
    `EnsureSessionForAgentWithCreation` keyed on the **task assignee** rather
    than the profile the spawn call resolved, so it converges on the one
    persistent office row and reports `created == false`.
    `TestCreateStartSession_OfficeRunnerReusesPersistentSession`
    (`task_operations_test.go:185`) asserts that reuse and must also pass
    unmodified, so a test asserting a second distinct row on an office task would
    contradict one this design requires to pass. That convergence is pre-existing
    routing this capability does not change, and `.4` is satisfied on that shape
    by the session being delivered without error, not by a row being added.
- The guard's predicate is the complement of the terminal set, demonstrated by a
  row in a state outside both sets being treated as live
  (`AC-OFFICE-SESSION-IDENTITY-001.4`). The fallback also restricts to the pair
  before testing state: a task carrying a live session for a *different* agent
  profile still accepts an office create for its own pair (`001.7`).
- **The empty-`agent_profile_id` bypass inserts, on both paths**
  (`AC-OFFICE-SESSION-IDENTITY-001.5`, and the final sentence of
  `AC-OFFICE-SESSION-IDENTITY-001.7`). Two rows carrying an empty
  `agent_profile_id` are created in sequence on one task and **both** succeed;
  the guard is not consulted for either. Prove it separately at the repository
  and at the fallback: the two are safe for different reasons and only one is
  safe by accident. In SQL the branch cannot misfire - `createTaskSession` stores
  an empty string as `NULL`, and `agent_profile_id = ?` never matches `NULL`. The
  fallback works on Go structs, where that row reads back as
  `AgentProfileID == ""`, so the natural expression of
  `AC-OFFICE-SESSION-IDENTITY-001.7`'s pair restriction -
  `s.AgentProfileID == session.AgentProfileID` - makes `""` match `""`,
  classifies a second empty-profile row as a live row for the pair, and
  **refuses a legitimate insert**, exactly what that criterion's final sentence
  forbids. The fallback case is load-bearing and must run against a repository
  that does *not* implement `officeTaskSessionCreator`; the natural wrong
  implementation passes the repository case and fails only here.
- **A live row for the pair created by a non-office path is reused, not
  duplicated** (`AC-OFFICE-SESSION-IDENTITY-001.8`). Two assertions at two layers;
  do not fold them into one test, because neither layer can observe both halves.
  **Repository:** seed a live row for the pair through a non-office insert entry
  point - `CreateTaskSession` is the simplest - then call
  `CreateOfficeTaskSession` for that pair and assert it is refused under `001.2`
  and adds no row. **Executor:** seed the same shape, then call
  `EnsureSessionForAgentWithCreation` and assert a nil error, `created == false`,
  a returned session carrying the seeded row's `ID`, and still one row for the
  pair. Assert the `ID`, not the whole row: `001.9` permits the recovery to write
  to the row it reuses. Do **not** assert a refusal at the executor layer - the
  lookup runs first, Change 2 orders the live seeded row to the top of it, and
  reuse returns before `createOfficeSession` is called, so no create happens
  there to refuse. This makes the guard deliberately origin-blind, the behaviour
  most likely to be read as a bug and "fixed" later.
- **The ordering is total, and evaluates identically on both dialects**
  (`AC-OFFICE-SESSION-IDENTITY-002.2`, `.3`, `.5`). One pair is seeded so that
  each comparison is forced in turn: two live rows differing only in
  `started_at` select the greater `started_at`; two live rows sharing one
  `started_at` select the greater `id`; and the assertions run against both
  SQLite and PostgreSQL, since `.5` is a claim about the expression evaluating
  the same way on each and a SQLite-only test cannot observe it. Follow the
  existing dual-dialect convention: this bullet's test takes the
  `TestPostgres`-prefixed twin shape named above. The
  `62201cdb`-shaped bullet proves liveness outranks recency; this one proves the
  two tiebreaks beneath it.
- **An absent pair and an empty identifier each return no session and no error**
  (`AC-OFFICE-SESSION-IDENTITY-002.4`). Three cases, each asserting a nil
  session *and* a nil error: a task with no row for the pair, an empty `task_id`,
  and an empty `agent_profile_id`. This locks shipped behaviour Change 2 must not
  disturb - the ordering rewrite edits this same function, and turning "no rows"
  into an error would break every caller that branches on `session != nil`.
- **The retry is bounded at two create attempts**
  (`AC-OFFICE-SESSION-IDENTITY-003.3`). The count is asserted directly: in the
  worst case, where every create is refused under
  `AC-OFFICE-SESSION-IDENTITY-001.2` and every re-read yields no usable session,
  count the create attempts and assert **exactly two**, never three, and that the
  call then returns `ErrOfficeSessionRaceConflict` rather than looping. That
  worst case - the blocking row live at each guard and terminal before each
  re-read - is doubly nested and has no data-setup shape; build it with the seam
  above, whose call-counting permission is what makes "exactly two" observable at
  all. A bound no test observes is one a later edit removes silently, and the
  failure it prevents - unbounded retry against a pair that stays terminal - is
  this card's own livelock relocated one layer up.
- **Reuse publishes no second `CREATED` event**
  (`AC-OFFICE-SESSION-IDENTITY-003.5`). An office wakeup that reuses an existing
  row emits no `CREATED` lifecycle event, while the wakeup that created it did.
  Assert on the emitted event, not only on `EnsureSessionForAgentWithCreation`'s
  `created` boolean, which is the input to that decision, not an observation.

**Coverage of the remaining criteria.** Every criterion is named in a bullet
above or accounted for here, so one carrying no proof is a recorded decision
rather than an oversight. `001.1` and `.2` are asserted inside the two
concurrency bullets rather than separately - a guard that failed to refuse would
fail those first. `001.3`, `.4`, `.6` and `.9` each have their own bullet. `002.1`
is proven by the `62201cdb`-shaped bullet. `003.1`, `.2`, `.6` and `.7` have
their own bullets; `003.4` (never a nil session with a nil error) already holds
in the shipped recovery and is a standing invariant of both concurrency tests
rather than a case of its own. `004.2`, `.3` and `.4` are the three tripwires.

`004.1`, `004.5` and `001.10` are architectural negatives - no column, no index,
no migration, no schema change, and exactly one terminal-classification predicate
rather than a second copy - proven by the diff rather than by a test. `001.10`
sits here deliberately: no behavioural assertion can distinguish
`persistOfficeSessionFallback` calling `isStopTerminalSessionState` from it
carrying an identical inline copy, so like the other two it is a review check
against the `## Baseline` go/no-go test, not a Build-authored test.

## Risks

- **The guard is process-local.** Stated and accepted in the requirements' "Out
  of scope" - a real reduction against a table constraint, taken because a table
  constraint provably breaks shipped flows.
- **Change 2 alters a shared lookup.** Three other callers are affected. The
  argument that live-first is strictly more correct for all of them is made
  above and should be confirmed per call site during implementation, not assumed.
- **A non-office row can now absorb an office wakeup.** Under
  `AC-OFFICE-SESSION-IDENTITY-001.8`, if a spawned session on an office task
  inherited the profile and is live, the next office wakeup reuses it instead of
  creating an office row; before this change it created a second row. No column
  tells the two apart, so this follows from the design rather than being chosen,
  and is recorded because behaviour changes.
- **The guard adds a query inside the writer transaction.** An indexed lookup on
  `task_id` under a lock the transaction already holds, on a path that runs once
  per office session creation, not per turn.
