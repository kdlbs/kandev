---
status: draft
system: office
created: 2026-09-07
owners:
  - kandev
---

# Office Run Deduplication — Generation Identity

## Overview

Every autonomous wake in Office is enqueued through the shared runs queue with an
`idempotency_key`. The key exists so that a *redelivery* of one occurrence (a
replayed event, a retried handler, two producers reacting to the same fact) does
not launch the same agent twice.

Several producers instead mint a key that is **permanently unique per (reason,
task, agent)**. Such a key cannot distinguish a redelivery from a genuine repeat
of the same kind of work, so the second legitimate occurrence is suppressed. The
suppression is total and leaves no trace: no run row, no inbox item, no wire
event, and a `Debug`-level log line.

The canonical case is assignment. `task_assigned:<task>:<agent>` is stable for
the life of that pair, so both of these are permanent, invisible no-ops:

- Reassigning a task back to an agent that previously held it (A -> B -> A),
  suppressed by the 24-hour lookup.
- Reassigning the same task to the same agent more than 24 hours later,
  suppressed by the unbounded `UNIQUE(idempotency_key)` index after the lookup
  has already passed.

The second case is the more damaging of the two: the fast lookup reports no
duplicate, the producer proceeds, and the durable index rejects the insert. The
queue reports that rejection as a successful no-op.

Two producers already avoid the trap by varying their key with something that
changes per occurrence: the workflow auto-start key carries the step-transition
row id, and the step-entry key carries the entry id and position. Re-running an
epic by moving it through a step therefore works, while re-running it by
re-assignment does not. This capability generalises what those two do into a
contract that every producer must satisfy, and makes the queue's dedup decisions
observable.

This capability owns the *contract on the key*, and the *observability of the
outcome*. It does not change the 24-hour lookup, the durable unique index, the
coalescing window, or the run lifecycle.

## Terminology

- **Occurrence:** one real-world fact that justifies waking an agent — this
  assignment, this comment, this decision, this cron fire. Two occurrences of
  the same kind on the same (task, agent) are distinct work. Two occurrences are
  **distinguishable** when some durable row the producer can read differs between
  them; where nothing does, they collapse to one, and every case that collapses is
  named under `## Out of scope`.
- **Redelivery:** a second attempt to enqueue a wake for an occurrence that has
  already been enqueued — an event replayed, a handler retried, or a second
  producer reacting to the same fact.
- **Dedup key:** the value persisted in `runs.idempotency_key` or
  `agent_wakeup_requests.idempotency_key`.
- **Generation component:** the part of a dedup key that changes between two
  occurrences and stays equal across redeliveries of one occurrence.
- **Windowed dedup hit:** the queue's recent-duplicate lookup matched an
  existing row inside its lookback window.
- **Durable dedup hit:** the recent-duplicate lookup found nothing, and the
  unbounded unique index then rejected the insert. After this capability ships
  this means one of three things: a genuine race between two producers; a producer
  that minted a colliding key; or a **legitimate late redelivery** — one occurrence
  re-enqueued more than the lookback window after the original, which a
  level-triggered reconciler that re-derives the same key on every tick produces
  as normal operation rather than as a fault. Only the first two are anomalies.

## Prior art

### Our own recorded reasoning (wiki)

**Searched:** resolved a configured personal wiki vault path and collection
name from the local `obsidian-wiki` config. The leg then **did not run**:
neither `obsidian-wiki` nor `qmd` is on this executor's PATH, no qmd MCP server
is exposed to this session, and the vault directory itself is unreadable here
(a macOS privacy restriction on this process, not a permission-gate denial).
This is a skipped step, not an empty result: the vault is configured and may
well hold relevant prior positions that this specification therefore did not
consult.

### What other products shipped (saas-kb)

**Searched:** nothing. The `saas-kb` MCP server and its `search_fsm_docs` tool
are not exposed to this session; the only MCP servers available are `codex` and
`kandev`. Skipped step, not an empty result.

### Prior art inside this repository

This leg did run, and it is the one that shaped the design.

- **Two producers already solve this problem**, which is why the defect is
  visible as an asymmetry rather than a total outage.
  `officeAutoStartIdempotencyKey` carries the step-transition row id, and the
  step-entry key carries the entry id and position. Re-running an epic by
  moving it through a step works; re-running it by re-assignment does not. This
  capability generalises their shape rather than inventing one.
- **`childrenCompletedIdempotencyKey`** already argues, in a written comment,
  the exact principle this specification makes general: a key that is
  "permanently unique per (parent, agent)" would wake a parent only for its
  first delegation wave, so the key digests the child set instead. It also
  already made the judgment that the digest covers *which* children exist
  rather than each child's state, to avoid spurious wakes. That reasoning is
  adopted here, not re-derived.
- **`task_sessions.route_generation`** is a monotonic counter already in the
  tree, bumped on a routing decision and compared for staleness. The assignment
  generation this capability needs is the same pattern, so it follows that
  precedent rather than introducing a new one.

**What we are doing differently:** the existing solutions are per-producer, each
one discovered after its own bug report. This capability makes the generation
component a property of the key contract itself (AC-OFFICE-RUN-DEDUP-001.1),
enforced by a test over every producer, so the next producer added cannot
reintroduce a permanent key silently. It also inverts the failure direction:
where `officeAutoStartIdempotencyKey` falls back to a time-derived legacy key
when its generation is missing, this contract falls back to *no key at all*,
because a duplicate wake is recoverable and a suppressed one is not.

## Requirements

### REQ-OFFICE-RUN-DEDUP-001: Generation identity in every persisted dedup key

**Intent:** A dedup key must suppress a redelivery and must not suppress a
repeat. A key built only from durable identifiers that never change (reason,
task, agent) cannot do both, and today it silently chooses the wrong one.

**User story:** As an Office operator, I want re-assigning a task to an agent
that already held it to actually wake that agent, so that a re-run is not a
silent no-op.

#### Acceptance criteria

- **AC-OFFICE-RUN-DEDUP-001.1:** When a producer enqueues a wake with a
  non-empty dedup key, that key shall contain a generation component that is
  equal for two enqueue attempts describing the same occurrence, and different
  for two enqueue attempts describing occurrences that differ in some durable row
  the producer can read. Occurrences that no durable row distinguishes collapse
  to one key; each such case shall be named under `## Out of scope`.
- **AC-OFFICE-RUN-DEDUP-001.2:** When a task is assigned to an agent, then to a
  different agent, then back to the first agent, the system shall wake the first
  agent for the third assignment — by inserting a run row, or, when that
  assignment lands within the coalescing window of a still-queued run for the
  same agent and task, by merging into that row. It shall not report a
  deduplicated outcome, which is the only outcome that produces no run at all.
- **AC-OFFICE-RUN-DEDUP-001.3:** When a task is assigned to the same agent a
  second time and the earlier assignment's run is older than the queue's
  recent-duplicate lookback window, the system shall insert a run row. That
  earlier run is necessarily outside the far shorter coalescing window, so the
  outcome here is queued and never coalesced.
- **AC-OFFICE-RUN-DEDUP-001.4:** When the same assignment occurrence is
  delivered to a producer more than once and the first delivery persisted a run
  row bearing its key, the system shall queue exactly one run for it. A delivery
  that coalesced persisted no such row, so a redelivery after the coalescing
  window queues a second run; see `## Out of scope`.
- **AC-OFFICE-RUN-DEDUP-001.5:** When a wake is triggered by an occurrence that a
  durable row identifies, its generation component shall be derived from that row
  — its primary key, or a persisted column whose value is fixed for that
  occurrence — and shall not be read from the clock at the moment the wake is
  produced. A persisted scheduled time is a stored column, not a clock read, and
  satisfies this criterion; the processing time at which a producer happens to run
  does not.
- **AC-OFFICE-RUN-DEDUP-001.6:** When a wake is triggered by a clock tick that no
  durable row identifies, its generation component can be derived from the tick's
  scheduled time. A tick whose due time is persisted and claimed on a durable row
  before dispatch is identified by that row, and is governed by
  AC-OFFICE-RUN-DEDUP-001.5 instead of by this criterion.
- **AC-OFFICE-RUN-DEDUP-001.9:** When a durable row identifies an occurrence but
  the producer would have to re-read that row after the occurrence has advanced,
  the identifying value shall be captured at the point the occurrence is committed
  or claimed and carried to the producer. A producer shall not recover a
  generation component by re-reading a row that a later occurrence has since
  moved.
- **AC-OFFICE-RUN-DEDUP-001.7:** When a dedup key is supplied by an agent
  through a runtime action, the system shall scope that key to the supplying
  run so a value the agent reuses across two of its own runs does not suppress
  the second.
- **AC-OFFICE-RUN-DEDUP-001.8:** When a run row persisted before this capability
  shipped carries a key in the previous format, the system shall leave that row
  unchanged and shall not treat it as a duplicate of a key in the new format.

### REQ-OFFICE-RUN-DEDUP-002: One occurrence yields one key across producers

**Intent:** Several occurrences are observed by more than one producer. Assignment
is observed by both the task-event subscriber and the reactivity pipeline, and
those two producers converge on one key today. That convergence is what makes a
reassignment wake an agent once rather than twice. Adding a generation component
independently in each producer would break it.

#### Acceptance criteria

- **AC-OFFICE-RUN-DEDUP-002.1:** When two producers that each derive a dedup key
  enqueue a wake for the same occurrence with the same reason, task, and agent,
  they shall derive an identical key. A producer enqueueing that occurrence with
  no key is governed by REQ-OFFICE-RUN-DEDUP-003 and does not violate this
  criterion.
- **AC-OFFICE-RUN-DEDUP-002.2:** When an assignment is observed by both the
  task-event subscriber and the reactivity pipeline, the system shall queue
  exactly one run.
- **AC-OFFICE-RUN-DEDUP-002.3:** When two producers attempt to enqueue the same
  key concurrently, each shall either persist a run or observe a suppression
  outcome — deduplicated, or coalesced when a coalescible run was queued for that
  agent, reason and task at its coalesce check — without returning an error to
  its caller. At most one may persist, and where such a run was queued for both,
  neither does. No outcome adds a second row bearing the key; none is a failure.
- **AC-OFFICE-RUN-DEDUP-002.4:** When two producers attempt to enqueue the same
  key concurrently, at most one row shall ever bear that key — never two — and
  neither producer shall abort. Whether it is one row or none depends on whether
  a coalescible run was queued at each producer's coalesce check, not on which
  insert committed first; nothing may branch on which outcome a producer saw.

### REQ-OFFICE-RUN-DEDUP-003: Never suppress silently when the generation is unknown

**Intent:** A generation component can be unresolvable — a legacy event without
the field, an occurrence row not yet committed, a read that failed. Falling back
to a permanently-unique key in that case reintroduces exactly the defect this
capability removes, and does so in the paths that are hardest to observe. A
duplicate wake costs one redundant agent turn; a suppressed wake costs the work.

#### Acceptance criteria

- **AC-OFFICE-RUN-DEDUP-003.1:** When a producer cannot resolve a generation
  component for an occurrence, it shall not enqueue the wake under a key whose
  only components are the reason, task, and agent.
- **AC-OFFICE-RUN-DEDUP-003.2:** When a producer cannot resolve a generation
  component, it shall enqueue the wake with no dedup key, accepting a possible
  duplicate run rather than a possible suppression.
- **AC-OFFICE-RUN-DEDUP-003.3:** When a producer enqueues a wake with no dedup
  key, the system shall record that event in telemetry, attributed to the run
  reason and discriminated by whether the key was omitted because a generation
  was unresolvable or because that reason is keyless by design, so that a
  resolution failure is countable on its own.
- **AC-OFFICE-RUN-DEDUP-003.4:** When a wake is enqueued with an empty dedup
  key, the system shall not apply idempotency suppression to it.

### REQ-OFFICE-RUN-DEDUP-004: Deduplication is observable

**Intent:** A dedup is a decision not to do requested work. Today it is reported
at `Debug`, is not counted, and is reported to the office producer path as an
undifferentiated success. An operator asking "why did nothing happen when I
reassigned this" has nothing to read.

**User story:** As an Office operator, I want to see that a wake was suppressed
and why, so that a no-op is diagnosable without attaching a debugger.

#### Acceptance criteria

- **AC-OFFICE-RUN-DEDUP-004.1:** When the queue suppresses a wake, it shall
  increment a counter exposed through the process metrics endpoint, labelled by
  run reason and by whether the suppression was a windowed or a durable dedup
  hit.
- **AC-OFFICE-RUN-DEDUP-004.2:** When the queue suppresses a wake through the
  durable unique index after the recent-duplicate lookup reported no duplicate,
  it shall emit a structured log record at a level an operator sees by default,
  carrying the dedup key, the run reason, and the resolved agent.
- **AC-OFFICE-RUN-DEDUP-004.3:** When the queue suppresses a wake through the
  recent-duplicate lookup, it shall emit a structured log record carrying the
  dedup key and the run reason.
- **AC-OFFICE-RUN-DEDUP-004.4:** When a caller enqueues a wake through the
  **run queue**, the run queue shall report which of queued, deduplicated, or
  coalesced occurred, and shall report it through every run-queue enqueue
  interface Office producers use. This criterion does not reach the
  wakeup-request table, which has neither a windowed lookup nor coalescing and so
  has no third outcome to report; AC-OFFICE-RUN-DEDUP-004.6 covers it in full.
- **AC-OFFICE-RUN-DEDUP-004.5:** When the **run queue** reports a deduplicated or
  coalesced outcome, it shall not return an error, and the caller shall not treat
  the outcome as a failure. The wakeup-request table is exempt from the first
  clause only: it reports its one suppression as a sentinel error, and a caller
  receiving that sentinel shall still not treat it as a failure.
- **AC-OFFICE-RUN-DEDUP-004.6:** When a wakeup request is rejected by the wakeup
  table's unique index, the system shall increment the same counter family with
  the wakeup queue identified as the source.

## Out of scope

- **Rewriting persisted keys on existing rows.** A run row is a historical
  record of a wake that happened. Backfilling it to the new key format would
  restate history and could itself collide with a key a live producer is about
  to mint. AC-OFFICE-RUN-DEDUP-001.8 makes the old rows inert instead.
- **Changing the 24-hour recent-duplicate lookback window.** The window is a
  performance affordance on the fast path; the unbounded unique index is what
  actually enforces identity. Neither is the defect, and both are unchanged.
- **Changing the 5-second coalescing window or which reasons coalesce.**
  Coalescing merges wakes that are already agreed to be redundant within seconds;
  it is a separate mechanism from identity-based dedup and is out of scope here.
  It is deliberately *not* a suppression: a coalesced wake merges into a run row
  that is still queued for that agent and task, carrying the newer payload, so the
  agent is woken. Only a deduplicated outcome produces no run at all. That is why
  AC-OFFICE-RUN-DEDUP-001.2 accepts a coalesced outcome and forbids only a
  deduplicated one, and why the coalescing predicate is left exactly as it stands.
  Two distinct assignment generations landing inside the same five seconds
  therefore yield one wake, not two: the operator asked twice in five seconds and
  gets one launch carrying the second request's payload. A wake that coalesces
  also leaves no row bearing its own key, so a redelivery of that occurrence
  after the window queues a second run. AC-OFFICE-RUN-DEDUP-003.2 elects a
  duplicate over a suppression, so that is the accepted direction; closing it
  would mean keying the merged row, which changes the coalescing mechanism.

- **Distinguishing a blocker set that resolves, un-resolves and re-resolves
  identically.** `task_blockers_resolved` identifies its occurrence by digesting
  the blocker id set, and blocker edges survive resolution — the readiness check
  reads each blocker task's state, not the edge's existence. So if every blocker
  resolves, one is reopened, and it then completes again with no blocker added or
  removed, the second wave digests to the same key as the first and is suppressed
  once past the lookback window. This is the one case
  AC-OFFICE-RUN-DEDUP-001.1 requires be named here: no durable row distinguishes
  the two waves. Both candidates were rejected — the resolving task's id is
  identical in both waves, because the readiness check fires only on the last
  blocker to complete; and a blocker task's `updated_at` is exactly the
  time-derived shape AC-OFFICE-RUN-DEDUP-001.5 disallows, besides re-introducing
  the spurious wakes `childrenCompletedIdempotencyKey` digests the child *set* to
  avoid. A durable per-wave row would close this; creating one is separate work.
- **A user-facing surface for a suppressed wake.** Deduplication is a
  high-frequency, expected outcome on the engine redelivery paths; an inbox
  item or a task-timeline entry per occurrence would be noise that trains
  operators to ignore the surface. REQ-OFFICE-RUN-DEDUP-004 places the
  observability floor at operator telemetry and logs. If a user-visible surface
  is wanted later, the counter added here is the evidence needed to size it.
- **Retiring the duplicate `RunReason*` constant blocks** in
  `internal/office/scheduler` and `internal/office/service`. The audit crosses
  them, but consolidating them is an unrelated refactor.
- **The order in which several wakes from one mutation are enqueued.** A single
  task mutation can fan out to an assignee and several reviewers. This
  capability changes each wake's key, not the sequence the reactivity pipeline
  emits them in, and defines no ordering or tiebreak over that fan-out.
- **Changing the `heartbeat` wake source's key.** `fireHeartbeat` still mints a
  live engine operation id (`heartbeat:<task>:<step>:<unix seconds>`) that the
  engine expands into a persisted key, so heartbeat *is* a producer and is
  covered by the audit. It is already generational on its tick time, which
  AC-OFFICE-RUN-DEDUP-001.6 permits, so no change is required to it. Retiring
  the source itself, and the duplicate `heartbeat` constants left behind, are
  separate work.
