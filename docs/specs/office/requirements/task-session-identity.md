---
status: draft
system: office
created: 2026-08-31
owners:
  - kandev
---

# Office Task Session Identity Requirements

## Overview

`AC-OFFICE-TASKS-001.2` already states the intended invariant: an office task
has "one [session] per `(task_id, agent_instance_id)` pair". Nothing enforces
it. Concurrent office wakeups for one `(task, agent)` pair create three or four
live `task_sessions` rows within a second, and the guard written to catch that
(`ErrOfficeSessionRaceConflict`) has never fired: the condition it tests for
cannot occur.

This capability makes the invariant real and scopes it correctly. It is true of
the **office session-reuse path only**, not of the `task_sessions` table: a
previous attempt to enforce it table-wide with a unique index broke three
shipped flows that deliberately hold two live rows for one pair (see
REQ-OFFICE-SESSION-IDENTITY-004).

Two defects produce the observed behaviour, and both must be fixed together or
the fix is worse than the bug:

1. **A read-then-write window across two connection pools.** The office
   find-or-create reads through the read-only pool (four connections) and
   inserts through the single writer. Concurrent callers all read "no session",
   then all insert. The reader pool's width and the observed duplicate count
   match.
2. **A non-deterministic lookup that can prefer a dead row.** The lookup
   returns the newest row by `started_at` regardless of state. When the newest
   row for a pair is terminal and an older row is still live, the caller reads
   terminal, decides to create, and any guard on creation then refuses. The
   caller re-reads, gets the same terminal row, and fails permanently. This is
   not hypothetical: one pair in the shipped database is already in that shape,
   so enforcement without a lookup fix converts a duplicate-row bug into a task
   that can never start again.

Office owns this contract because office owns the notion that a `(task, agent)`
pair names one durable conversation. The task repository owns the storage
primitive and the non-office creation paths, which this contract leaves
untouched.

## Terminology

- **Office session** - a `task_sessions` row created through the office
  find-or-create path (`EnsureSessionForAgentWithCreation` ->
  `createOfficeSession` -> the repository's office creator). Office sessions are
  the only rows this contract constrains.
- **Terminal state** - `COMPLETED`, `FAILED`, or `CANCELLED`.
- **Live** - any state that is not terminal. Defined as the complement, not an
  enumerated list, so a state added later is treated as live by both the reuse
  rule and the creation guard rather than by only one.
- **Pair** - a `(task_id, agent_profile_id)` tuple with a non-empty
  `agent_profile_id`.

## Prior art

**Our own prior reasoning (wiki):** unavailable - no `OBSIDIAN_VAULT_PATH` and
no vault on this machine, so the leg could not run. Recorded, not faked.

**In-repo prior decisions**, which did apply:

- `AC-OFFICE-TASKS-001.2` and `AC-OFFICE-TASKS-001.3` already assert one
  session per pair and define the terminal set. This capability enforces an
  existing contract; it does not invent one. Where the two could drift, this
  document defers to `tasks.md`.
- `system-design/tasks-01.md`'s DDL block for a unique index on
  `task_sessions(task_id, agent_instance_id)` names a column ADR 0005 renamed,
  and the index was never created. Stale, not a live decision, superseded here.
- **ADR 0008** (idempotent migrations, pre-migration snapshot on upgrade boots)
  and **ADR 0027** (replayable schema migrations) both favour a design needing
  no schema change and no backfill, which this one does not.
- **ADR 0005 (agent model unification)** made `agent_profile_id` shared between
  kanban and office. That is precisely why the pair cannot be treated as
  office-only at the schema level, and why enforcement belongs on the office
  code path instead.

**What others shipped (saas-kb, `category: ai_sdlc`):** nothing useful. Two
queries returned unrelated vendor docs at relevance 0.0094-0.0137; no vendor in
that corpus documents per-agent session identity, so there is nothing to copy
or deliberately diverge from.

**What we are doing differently:** the obvious move, which the stale DDL and the
previous attempt both reached for, is a database unique index. We reject it. The
invariant is a property of one code path, not of the table, and the column a
partial index predicate would need is shared with the paths that must stay
allowed. Enforcement therefore lives in the office creation path's existing
transaction, which already establishes the mutual exclusion required. This
trades a cross-process guarantee for correctness on the paths that exist; see
"Out of scope".

## Requirements

### REQ-OFFICE-SESSION-IDENTITY-001: At most one live office session per pair

**Intent:** Office session creation must refuse to add a second live row for a
pair that already has one, so that `AC-OFFICE-TASKS-001.2` holds at runtime and
`ErrOfficeSessionRaceConflict` becomes reachable.

#### Acceptance criteria

- **AC-OFFICE-SESSION-IDENTITY-001.1:** When office session creation is asked
  to insert a row for a pair, the system shall determine whether a live row
  already exists for that pair and shall perform that determination and the
  insert within a single database transaction.
- **AC-OFFICE-SESSION-IDENTITY-001.2:** If a live row already exists for the
  pair, then the system shall not insert a row and shall return an error
  satisfying `errors.Is(err, ErrOfficeSessionRaceConflict)`.
- **AC-OFFICE-SESSION-IDENTITY-001.3:** While every existing row for the pair
  is terminal, the system shall insert the new row and shall succeed. Terminal
  rows accumulate as retry history and are never an obstacle to creation.
- **AC-OFFICE-SESSION-IDENTITY-001.4:** The system shall classify a row as live
  by testing that its state is not one of `COMPLETED`, `FAILED`, `CANCELLED`.
  It shall not test membership of an enumerated live list, so that a state
  added later is treated identically by this guard and by the reuse rule in
  `AC-OFFICE-TASKS-001.3`.
- **AC-OFFICE-SESSION-IDENTITY-001.5:** If the row being inserted has an empty
  `agent_profile_id`, then the system shall skip the guard entirely and insert.
  Such a row is stored with SQL `NULL` and names no pair.
- **AC-OFFICE-SESSION-IDENTITY-001.6:** When the guard refuses an insert, the
  transaction that refuses shall write nothing. It shall not insert the new row,
  and it shall leave every existing row for the pair unmodified: no row is
  updated, cancelled, merged, or deleted, and no `metadata.acp_session_id` is
  moved between rows. This obligation is on the refusing transaction, which
  rolls back. It does not constrain what the caller does after the refusal has
  returned; that is governed by `AC-OFFICE-SESSION-IDENTITY-001.9` and
  REQ-OFFICE-SESSION-IDENTITY-003.
- **AC-OFFICE-SESSION-IDENTITY-001.7:** Where the executor's repository does not
  implement the office creator interface and the in-process fallback creation
  path is used instead, the system shall apply the same rule as
  `AC-OFFICE-SESSION-IDENTITY-001.2` under that path's existing per-task lock,
  so the fallback is not a silently weaker guarantee - an equivalence that holds
  because exactly one `Executor` exists per process, a second one sharing the
  same database being an unshipped topology this document does not defend. That
  path holds a
  task-scoped list of sessions rather than a SQL result set, so the rule is
  applied in application code and in this order: the system shall first restrict
  the list to rows whose `agent_profile_id` equals the one being inserted, and
  shall then classify those rows using the predicate required by
  `AC-OFFICE-SESSION-IDENTITY-001.10`. Applying the state test before the pair
  restriction would refuse a legitimate insert whenever any other agent on the
  same task holds a live session, so the order is part of the contract.
  `AC-OFFICE-SESSION-IDENTITY-001.5` binds this path identically: where the row
  being inserted has an empty `agent_profile_id`, the fallback shall skip the
  rule and insert, and shall not instead match that empty value against other
  rows that also carry none, since no pair is named.
- **AC-OFFICE-SESSION-IDENTITY-001.8:** The system shall apply the guard to any
  live row for the pair, including one created by a non-office path such as a
  session spawned on an office task that inherited the spawner's profile.
  `task_sessions` carries no column distinguishing which path created a row, so
  the office side treats the pair as naming one conversation regardless of
  origin, and an office wakeup reuses that row rather than adding a second. This
  is a deliberate consequence of having no discriminator, not an oversight; it
  is the same assumption the pair lookup has always made.
- **AC-OFFICE-SESSION-IDENTITY-001.9:** After a refusal has returned, the
  recovery required by `AC-OFFICE-SESSION-IDENTITY-003.2` shall be permitted to
  write to the one row it reuses, and this capability shall not suppress those
  writes. Two are pre-existing behaviour of the reuse path and shall be
  preserved: transitioning a reused row from `IDLE` to `RUNNING`, and rebinding
  that row's execution profile, which includes clearing its
  `metadata.acp_session_id` so that a fresh agent connection is established. The
  permission extends no further: the writes are confined to the single row the
  recovery returns, no other row for the pair is touched, no
  `metadata.acp_session_id` is copied or moved from one row to another, and no
  row is deleted or merged on any path in this document. Their relative order is
  deliberately unconstrained: the two writes touch disjoint fields and leave the
  same observable end state either way. Their convergence under concurrency is
  scoped. Two recoveries reaching the same row with the *same* execution profile
  may each perform both writes without detecting the other, and the row settles
  in the same state. Two carrying *different* execution profiles do not
  converge; that case is reachable, and for it the rule is last-writer-wins on
  the stored row, with each caller's returned in-memory session reflecting its
  own write and not guaranteed to match what is stored. That is pre-existing
  reuse behaviour: this capability shall preserve it unchanged and shall add no
  third write, no new ordering, and no new coordination point. Making the
  divergent case converge is out of scope; the system design records why it is
  reachable and what closing it would cost.
- **AC-OFFICE-SESSION-IDENTITY-001.10:** The terminal classification required by
  `AC-OFFICE-SESSION-IDENTITY-001.4` shall have exactly one implementation in
  application code, reachable from every enforcement site that needs it, and the
  system shall not introduce a second one. It shall not reuse
  `models.IsResumableSessionState` for this purpose: that function answers a
  different question and classifies `CREATED` as non-resumable, whereas this
  contract classifies `CREATED` as live. The guard's SQL predicate expresses the
  same set in SQL, and the two shall be kept in correspondence.

### REQ-OFFICE-SESSION-IDENTITY-002: Deterministic, live-preferring lookup

**Intent:** The office lookup for a pair must return the row a caller can
actually use, and must return the same row for the same data every time.
Without this, REQ-OFFICE-SESSION-IDENTITY-001 converts an existing duplicate
into a permanently unstartable task.

#### Acceptance criteria

- **AC-OFFICE-SESSION-IDENTITY-002.1:** When more than one row exists for a
  pair, the system shall return a live row in preference to a terminal row,
  regardless of `started_at`.
- **AC-OFFICE-SESSION-IDENTITY-002.2:** Among rows of equal liveness, the
  system shall return the row with the greatest `started_at`.
- **AC-OFFICE-SESSION-IDENTITY-002.3:** If two or more candidate rows tie on
  both liveness and `started_at`, then the system shall break the tie by
  `task_sessions.id` descending. This column is named to make selection total
  and reproducible; it carries no meaning and no recency. No current data
  exhibits such a tie (measured: zero tied groups).
- **AC-OFFICE-SESSION-IDENTITY-002.4:** While no row exists for the pair, the
  lookup shall return no session and no error. If either identifier is empty,
  then the lookup shall likewise return no session and no error rather than
  querying, since no pair is named.
- **AC-OFFICE-SESSION-IDENTITY-002.5:** The ordering in
  `AC-OFFICE-SESSION-IDENTITY-002.1` through `.3` shall be expressed so that it
  evaluates identically on SQLite and PostgreSQL.

### REQ-OFFICE-SESSION-IDENTITY-003: Convergent find-or-create under concurrency

**Intent:** Concurrent office wakeups for one pair must converge on one live
session and every caller must receive it. A caller that loses the race must not
receive an error.

#### Acceptance criteria

- **AC-OFFICE-SESSION-IDENTITY-003.1:** When N callers concurrently request an
  office session for the same pair and no live row exists, the system shall
  create exactly one row and shall return that same row to all N callers.
- **AC-OFFICE-SESSION-IDENTITY-003.2:** When a caller's insert is refused under
  `AC-OFFICE-SESSION-IDENTITY-001.2`, the system shall re-read the pair, apply
  the existing reuse rule, and return the resulting session with its
  "was created by this call" indicator false.
- **AC-OFFICE-SESSION-IDENTITY-003.3:** If the re-read in
  `AC-OFFICE-SESSION-IDENTITY-003.2` yields no usable session because the
  blocking row reached a terminal state between the guard and the re-read, then
  the system shall retry the create-and-recover sequence at most once more. If
  that second attempt is itself refused under
  `AC-OFFICE-SESSION-IDENTITY-001.2` and its own re-read again yields no usable
  session, then the system shall return `ErrOfficeSessionRaceConflict`. The
  system shall not retry unboundedly: at most two create attempts are made per
  call, whatever the outcome.
- **AC-OFFICE-SESSION-IDENTITY-003.4:** The system shall never return a nil
  session together with a nil error from the office find-or-create.
- **AC-OFFICE-SESSION-IDENTITY-003.5:** When a caller reuses an existing row
  rather than creating one, the system shall not publish a second `CREATED`
  lifecycle event for that row.
- **AC-OFFICE-SESSION-IDENTITY-003.6:** Given a pair that already holds more
  than one live row from data written before this capability, when an office
  session is requested for that pair, the system shall return one of those rows
  and shall not fail. Convergence on legacy duplicates is achieved by selection
  under REQ-OFFICE-SESSION-IDENTITY-002, not by modifying data.
- **AC-OFFICE-SESSION-IDENTITY-003.7:** Only a refusal under
  `AC-OFFICE-SESSION-IDENTITY-001.2` shall be treated as a conflict. If a create
  attempt fails for any other reason, or if the re-read in
  `AC-OFFICE-SESSION-IDENTITY-003.2` fails, then the system shall return that
  failure to the caller as itself, wrapped so that the originating error remains
  inspectable with `errors.Is`. Such a failure shall not drive the retry in
  `AC-OFFICE-SESSION-IDENTITY-003.3` and shall not be reported as
  `ErrOfficeSessionRaceConflict`. The re-read's error shall not be discarded.
  The sentinel's meaning to a caller is "the pair is already served, re-read and
  reuse"; a database, transport, or transaction fault is not that, and reporting
  one as the other converts an operational failure into an outcome the contract
  tells callers to treat as benign.

### REQ-OFFICE-SESSION-IDENTITY-004: Non-office session creation is unconstrained

**Intent:** Three shipped flows deliberately hold two live rows for one pair.
This requirement exists so that a future implementation cannot satisfy
REQ-OFFICE-SESSION-IDENTITY-001 by a mechanism that also reaches them. It is
the guard against repeating the regression that produced this card.

#### Acceptance criteria

- **AC-OFFICE-SESSION-IDENTITY-004.1:** The system shall apply the constraint in
  REQ-OFFICE-SESSION-IDENTITY-001 only to rows created through the office
  creation path, and shall not apply it through any table-level database
  constraint on `task_sessions`.
- **AC-OFFICE-SESSION-IDENTITY-004.2:** When a kanban task is relaunched for an
  agent profile whose existing session for that task is still live, the system
  shall create a second, distinct live session and shall succeed.
- **AC-OFFICE-SESSION-IDENTITY-004.3:** When a workflow replacement session is
  prepared for an existing environment, the system shall create the replacement
  while the current session for the same pair is still live, and shall succeed.
- **AC-OFFICE-SESSION-IDENTITY-004.4:** When a session is spawned without an
  explicit agent profile and inherits the spawner's profile on the same task,
  the system shall deliver a session and shall succeed, this being the
  documented default of that tool. Where the task is not office-owned the
  delivered session shall be a newly created one, as today. Where it is
  office-owned and carries an assignee, the spawn converges on that task's one
  persistent office session by pre-existing routing, and the criterion is met by
  that session being delivered without error; there this capability shall neither
  refuse the convergence nor add a second live row to make the outcome a
  creation.
- **AC-OFFICE-SESSION-IDENTITY-004.5:** The system shall introduce no new column
  on `task_sessions`, no new index on `task_sessions`, and no backfill or
  repair of existing `task_sessions` rows.

## Out of scope

- **Cross-process concurrency on one database file.** The guarantee in
  REQ-OFFICE-SESSION-IDENTITY-001 rests on serialization within a single
  process: the single-connection SQLite writer pool, and the task row lock
  already taken on PostgreSQL. Two kandev processes writing one SQLite file
  concurrently is not a supported topology and is not defended against here. A
  table-level constraint would cover it; that is rejected for the reasons in
  REQ-OFFICE-SESSION-IDENTITY-004, and the upgrade path, should the topology
  ever be supported, is a dedicated office-slot discriminator column plus a
  partial unique index over it. Named so the trade is on record, not an
  omission.
- **Repairing the existing duplicate pairs.** Measured on the shipped database
  on 2026-08-31: 119 rows across 48 pairs, of which 101 are terminal retry
  history that is legitimate and must be preserved. Exactly one pair holds more
  than one live row; `AC-OFFICE-SESSION-IDENTITY-003.6` makes it usable without
  touching it. The census drifts upward while the system runs, so no acceptance
  criterion depends on the count, and `AC-OFFICE-SESSION-IDENTITY-004.5` forbids
  a backfill at any count. No migration is authorised by this document.
- **Making concurrent recoveries with different execution profiles converge on a
  defined winner.** `AC-OFFICE-SESSION-IDENTITY-001.9` records the semantics that
  hold - last-writer-wins, each caller returning its own write - instead of
  asserting a convergence that does not, so nothing is left to invent. Both ways
  of closing the window change shipped reuse behaviour and no criterion here
  depends on either; the system design records them, and records that the
  bounded retry widens this window in frequency and not in kind. Named so the
  trade is on record and not an omission.
- **The stale DDL in `system-design/tasks-01.md`.** It names a column that does
  not exist and is superseded by this contract. Correcting or deleting that
  block is editorial cleanup for a separate change, not part of this capability.
- **`ErrOfficeSessionRaceConflict`'s driver-error classification.** With
  enforcement expressed as an explicit in-transaction guard, the sentinel is
  returned directly and no `UNIQUE constraint` message need be parsed. Whether
  the parent card's dual-dialect classifier is retained, and whether the
  `strings.Contains` match that could never have matched SQLite's error text is
  removed, is that card's disposition. This contract requires only that the
  sentinel is reachable and testable with `errors.Is`.
- **Any user-visible surface.** No new copy, no i18n work, no UI change.

## System design

The technical design is [part 1](../system-design/task-session-identity-01.md).
