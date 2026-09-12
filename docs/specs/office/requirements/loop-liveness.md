---
status: draft
system: office
created: 2026-09-07
owners:
  - kandev
---

# Office Loop Liveness Requirements

## Overview

Office's unattended loop is a chain: a cron tick evaluates due triggers, claims
one, creates a routine run, enqueues a wake, the wake becomes a `runs` row, the
scheduler claims it, an agent launches, and the run reaches a terminal state.
Every link is individually logged. **No surface answers whether the chain ran.**

On the instance this was written against it stopped on 2026-08-05 and nothing
reported it: `last_run_at` was never written, 322 consecutive fires read as
correct coalescing, one surviving `runs.outcome = 'no_agent_launched'` names a
value no longer in the code, and every `runs` row carried `session_id = ''`.
Every surface read as healthy.

Office owns this contract: every input is an Office primitive. It makes the loop
**legible**, not working — nothing here fires or re-arms a trigger, requeues a
run, or launches an agent.

## Source obligations

Inherited from `Forge/docs/kandev/office/BETA-REQUIREMENTS.md` (gaps 21, 31, §7a),
outside this repository. Restated in full; this spec, not the citation, is built:

- **NFR-1 — no unverifiable success.** A run reporting that work was done must
  carry persisted evidence an agent ran. A success claim without it is a defect to
  be surfaced, not a success.
- **NFR-2 — one-identifier traceability.** From any identifier at any hop, an
  operator must reach every other hop by keyed lookup on a persisted column, with
  no log correlation and no free-text search.

## Terminology

- **Wake origin:** the first durable row a wake creates — `office_routine_runs` for
  a routine fire, `agent_wakeup_requests` otherwise.
- **Causation id:** an opaque identifier minted once per wake origin and copied
  forward unchanged onto every row descending from it.
- **Eligible trigger:** a row in `office_routine_triggers` with `kind = 'cron'`
  and `enabled = 1`, nothing further: an *armed-ness* test, not a *due* test.
  `GetDueTriggers`' extra `next_run_at` clauses shall not be reused here: they
  drop every stranded trigger.
- **Overdue trigger:** an eligible trigger whose `next_run_at` is non-`NULL` and
  older than `now - overdue grace`.
- **Claim instant:** `COALESCE(last_fired_at, created_at)`. Only the claim writes
  `last_fired_at`, in the statement clearing `next_run_at`; `created_at` covers a
  trigger that never fired, making the age total. `updated_at` shall not be used:
  its other writers would re-date a dead trigger.
- **Stranded trigger:** an eligible trigger whose `next_run_at` is `NULL` and
  whose claim instant is older than `now - stranded grace`.
- **Claiming trigger:** the same, but whose claim instant is *within* the grace.
  Mid-claim, not dead; neither overdue nor stranded.
- **Claimable run:** a `queued` run the scheduler's claim filter would take:
  `routing_blocked_status` is `NULL` and no other run for the same
  `agent_profile_id` is `claimed`. Failing either, the queue is holding it back
  on purpose.
- **Queue-eligible instant:** `scheduled_retry_at` when non-`NULL`, else
  `requested_at`, so a backoff ages from when the run may next be claimed.
- **Stuck run:** a claimable run whose `current_route_attempt_seq` is `0` and
  whose queue-eligible instant is older than `now - queued grace`; or a `claimed`
  run whose `claimed_at` is `NULL` or older than `now - claimed grace`. A non-zero
  sequence means the run was relaunched; see **Out of scope**.
- **Stuck instant:** the column a stuck run is aged by — `claimed_at` when
  `claimed`, the queue-eligible instant when `queued`. It is the stuck list's
  ordering key (`AC-OFFICE-LOOP-LIVENESS-004.9`).
- **Terminal shape:** the derived classification of a terminal run
  (`REQ-OFFICE-LOOP-LIVENESS-005`).
- **Silent success:** a terminal shape asserting work happened on a run for which
  no session was ever recorded.
- **Activation instant:** the timestamp published in `kandev_meta` once a boot has
  positively probed that this capability's columns exist. Older rows cannot be
  classified and must not be reported as defects.

## Requirements

### REQ-OFFICE-LOOP-LIVENESS-001: Persist routine fire recency

**Intent:** `office_routines.last_run_at` is the field an operator reaches for to
ask "is this routine running", and nothing writes it, so a dead schedule reads
like one that never had a schedule. It records that the **schedule fired**, not
that the fire produced work: a coalesced or skipped fire still advances it.

#### Acceptance criteria

- **AC-OFFICE-LOOP-LIVENESS-001.1:** When an `office_routine_runs` row is created
  for a routine, the system shall persist that row's dispatch instant to the
  routine's `last_run_at`, whatever disposition the run later reaches.
- **AC-OFFICE-LOOP-LIVENESS-001.2:** The persisted instant shall be the routine
  run's `started_at`, never the instant the write executed.
- **AC-OFFICE-LOOP-LIVENESS-001.3:** The write shall be monotonic: when the stored
  value is already at or after the incoming instant, it shall not change.
- **AC-OFFICE-LOOP-LIVENESS-001.4:** When two dispatches for the same routine
  commit concurrently, the stored value shall equal the greater of the two
  instants, whichever order they commit in.
- **AC-OFFICE-LOOP-LIVENESS-001.5:** The write shall touch only `last_run_at` and
  `updated_at`, and read-modify-write no other routine column.
- **AC-OFFICE-LOOP-LIVENESS-001.6:** When the write matches zero rows, the system
  shall treat that as success and shall not retry.
- **AC-OFFICE-LOOP-LIVENESS-001.7:** When the write returns an error, the system
  shall complete the dispatch anyway and record the failure as a counted, logged
  event.
- **AC-OFFICE-LOOP-LIVENESS-001.8:** No other routine write shall change
  `last_run_at`. The existing multi-column routine update that carries the field
  shall stop writing it, so a caller holding a stale routine snapshot cannot move
  the value backwards or clear it: monotonicity is a property of the column, not
  of one statement.

### REQ-OFFICE-LOOP-LIVENESS-002: Correlate a wake to its run and its session

**Intent:** **NFR-2** ([Source obligations](#source-obligations)) requires a wake
→ run → session → process chain traceable by one id. It breaks twice today: no row
carries an id shared with its predecessor, and the launch drops the session id it
is handed.

#### Acceptance criteria

- **AC-OFFICE-LOOP-LIVENESS-002.1:** Every wake origin shall mint exactly one
  causation id and persist it on the row it creates.
- **AC-OFFICE-LOOP-LIVENESS-002.2:** A wakeup request created for a routine fire
  shall carry that fire's causation id.
- **AC-OFFICE-LOOP-LIVENESS-002.3:** A run created from a wakeup request shall
  carry that request's causation id.
- **AC-OFFICE-LOOP-LIVENESS-002.4:** When a wakeup request is coalesced or skipped
  against an in-flight run, the run's causation id shall not change; the request
  keeps its own, and the two remain joinable through the request's run id.
- **AC-OFFICE-LOOP-LIVENESS-002.5:** When a run is requeued, retried, parked or
  lifted, its causation id shall not change.
- **AC-OFFICE-LOOP-LIVENESS-002.6:** An empty causation id shall mean "not
  correlated". No reader shall group, join, or aggregate two rows because both
  carry an empty causation id.
- **AC-OFFICE-LOOP-LIVENESS-002.7:** When a launch creates an agent session for a
  run, the run shall record that session's id before the launch call returns to the
  scheduler, on both the direct and the provider-routed launch path.
- **AC-OFFICE-LOOP-LIVENESS-002.8:** When a launch reports success but yields no
  session id, the run's session id shall be left empty and the condition counted,
  never defaulted to a placeholder.
- **AC-OFFICE-LOOP-LIVENESS-002.9:** A heavy routine fire, which creates a task
  instead of a wakeup request, shall record its causation id and its linked task id
  on the routine run, so the fire reaches the sessions its task produces by the
  keyed path `office_routine_runs.linked_task_id` → `tasks.id` →
  `task_sessions.task_id`.
- **AC-OFFICE-LOOP-LIVENESS-002.10:** When one run is launched more than once — a
  run requeued after a post-start provider fallback and relaunched against the same
  row — the recorded session id shall be the one from the launch holding the
  greatest `runs.claimed_at`, and the launch write shall never replace a recorded
  session id with an empty one. This binds the launch write only: the requeue
  preceding a relaunch already clears the column, which this capability does not
  change.
- **AC-OFFICE-LOOP-LIVENESS-002.11:** When a launch yields a non-empty session id
  but persisting it to the run fails, the system shall not fail the launch and
  shall not retry the write; it shall leave the run's session id at whatever value
  it already held, never writing the unpersisted one, and shall record the failure
  as a counted, logged event carrying the run id and the unstored session id. That
  counter shall be distinct from the one counting launches that yielded no session
  id.

### REQ-OFFICE-LOOP-LIVENESS-003: Count every hop of the loop, outside dev mode

**Intent:** Office's only counters cover provider routing and budget claims, and
`/debug/vars` is mounted only under dev mode or pprof, so none is reachable on a
normal install. Per-hop counts are what show which link stopped.

#### Acceptance criteria

- **AC-OFFICE-LOOP-LIVENESS-003.1:** The system shall count, at minimum: cron
  routine ticks evaluated, eligible triggers claimed, routine runs created, wakeup
  requests created, runs claimed, agent launches, and terminal run transitions.
- **AC-OFFICE-LOOP-LIVENESS-003.2:** Routine runs shall be counted by
  `(source, disposition)` and terminal runs by terminal shape, so a fire that
  coalesced, one skipped and one launched are separately countable.
- **AC-OFFICE-LOOP-LIVENESS-003.3:** Every counter whose event is attributable to
  a workspace shall carry it as a label.
- **AC-OFFICE-LOOP-LIVENESS-003.4:** Counters shall be readable with dev mode and
  pprof both disabled.
- **AC-OFFICE-LOOP-LIVENESS-003.5:** A workspace-scoped reader shall receive only
  that workspace's labelled counter values, and no other workspace's in any form.
- **AC-OFFICE-LOOP-LIVENESS-003.6:** The system shall publish the instant of the
  most recent cron loop tick and the instant the process started.
- **AC-OFFICE-LOOP-LIVENESS-003.7:** Counters shall continue to be published through
  the existing expvar mechanism, so `/debug/vars` keeps working.
- **AC-OFFICE-LOOP-LIVENESS-003.8:** When an event's workspace cannot be
  resolved, the counter shall record it under an explicit unattributed label
  rather than dropping it or guessing. Its value shall not be added into any
  workspace's own values, shall be returned only in the process-scoped part of the
  response, identical for every reader, and shall not be reachable only through a
  dev-mode surface.
- **AC-OFFICE-LOOP-LIVENESS-003.9:** A counter naming a persisted state change
  shall be incremented exactly once per change, at the site that persists it, and
  a change that fails to persist shall not increment it. A counter naming an event
  that persists nothing — a tick, a failed write, a launch that yielded no session,
  an unservable read — shall be incremented exactly once at the site that observes
  it, and shall be separable from the counters of persisted changes.

### REQ-OFFICE-LOOP-LIVENESS-004: One request answers whether the loop is alive

**Intent:** "Was an eligible routine due, claimed, launched and completed inside
its SLO?" today needs manual reconstruction. One workspace-scoped request must
answer it with a verdict, its evidence and its thresholds.

#### Acceptance criteria

- **AC-OFFICE-LOOP-LIVENESS-004.1:** The system shall expose a workspace-scoped
  read returning one verdict from a closed set: `unknown`, `dead`, `degraded`,
  `not_armed`, `healthy`.
- **AC-OFFICE-LOOP-LIVENESS-004.2:** The verdict shall be decided by that exact
  precedence order, first match winning.
- **AC-OFFICE-LOOP-LIVENESS-004.3:** The verdict shall be `unknown` when the
  activation instant has not been published.
- **AC-OFFICE-LOOP-LIVENESS-004.4:** The verdict shall be `dead` when the
  workspace has at least one overdue or stranded trigger.
- **AC-OFFICE-LOOP-LIVENESS-004.5:** The verdict shall be `degraded` when no `dead`
  condition holds and the workspace has at least one stuck run, or at least one
  silent success whose terminal instant falls inside the evaluation window. The
  window binds silent successes only: stuck runs are never terminal, carry no
  terminal instant, and shall be reported however long they have been stuck.
- **AC-OFFICE-LOOP-LIVENESS-004.6:** The verdict shall be `not_armed` when no
  `dead` or `degraded` condition holds and the workspace has no eligible trigger.
- **AC-OFFICE-LOOP-LIVENESS-004.7:** The response shall carry the evidence for its
  verdict: the offending trigger rows, the stuck runs, the silent-success runs,
  and the terminal-shape counts for the window. Overdue and stranded triggers form
  one list, each row naming the condition it matched; so do queued-stuck and
  claimed-stuck runs. Every condition that can decide `dead` or `degraded` shall
  be evidenced by the rows that caused it, never by a count alone.
- **AC-OFFICE-LOOP-LIVENESS-004.8:** Every threshold and the evaluation window
  used to reach the verdict shall be returned alongside it.
- **AC-OFFICE-LOOP-LIVENESS-004.9:** Each returned list shall be ordered
  most-urgent-first by named columns with a named tiebreak, shall be capped, and
  shall report the cap and the untruncated total when it truncates.
- **AC-OFFICE-LOOP-LIVENESS-004.10:** When any input needed for the verdict cannot
  be read, the system shall fail loudly rather than return one, recording which
  input failed.
- **AC-OFFICE-LOOP-LIVENESS-004.11:** The read shall be side-effect free: it shall
  not fire, claim, re-arm, requeue, cancel or launch anything, and shall write no
  table.
- **AC-OFFICE-LOOP-LIVENESS-004.12:** Two reads issued concurrently for the same
  workspace shall each return an internally consistent verdict, and neither shall
  affect the other. Internally consistent means each evidence list, its cap and
  its total come from a single statement, and every threshold is applied to the
  one captured instant. It does not mean the lists share a snapshot: the read
  takes no transaction.
- **AC-OFFICE-LOOP-LIVENESS-004.13:** A workspace with no routines, runs or agents
  shall return `not_armed` with empty evidence lists, never an error or null.
- **AC-OFFICE-LOOP-LIVENESS-004.14:** The read shall accept no caller-supplied
  parameter other than the workspace, so no caller can widen the window, raise the
  cap or soften a threshold; every threshold shall be evaluated against one
  instant captured once per request.
- **AC-OFFICE-LOOP-LIVENESS-004.15:** A claiming trigger shall not be reported as
  stranded or overdue and shall appear in no evidence list — excluded from the
  verdict, not returned under a third label.
- **AC-OFFICE-LOOP-LIVENESS-004.16:** The evaluation window shall be applied to a
  terminal run's finish instant, or its request instant when that is absent, so no
  terminal run escapes the window by recording no finish instant.
- **AC-OFFICE-LOOP-LIVENESS-004.17:** A queued run that is not claimable shall
  appear in no evidence list and shall decide no verdict, and a queued run's age
  shall be measured from its queue-eligible instant. A run the queue is holding
  back, and one whose backoff has not expired, are deferrals rather than stalls.

### REQ-OFFICE-LOOP-LIVENESS-005: Separate the terminal shapes

**Intent:** Five terminal dispositions write `status = 'finished'`, a sixth writes
`status = 'failed'` with a null outcome, and a legacy value no enum contains
survives in production data. Naming each ending apart makes "quiet because there
is no work" and "quiet because it silently fails" different readings.

#### Acceptance criteria

- **AC-OFFICE-LOOP-LIVENESS-005.1:** The system shall classify every terminal run
  into exactly one shape from a closed set.
- **AC-OFFICE-LOOP-LIVENESS-005.2:** The classification shall be total: every
  combination of persisted status, outcome and session id shall map to one shape,
  including any status or outcome value this document does not enumerate.
- **AC-OFFICE-LOOP-LIVENESS-005.3:** A run that ended without an agent having
  launched shall be distinguishable from one that launched.
- **AC-OFFICE-LOOP-LIVENESS-005.4:** A run whose outcome asserts work happened but
  which never recorded a session shall be classified as a silent success and shall
  be separately countable — **NFR-1** made observable.
- **AC-OFFICE-LOOP-LIVENESS-005.5:** A legitimate no-launch skip shall be
  classified distinctly from a launch failure.
- **AC-OFFICE-LOOP-LIVENESS-005.6:** A run predating the activation instant shall
  be classified as pre-activation, never as a silent success.
- **AC-OFFICE-LOOP-LIVENESS-005.7:** Classification shall read only persisted
  columns, never matching free-text messages.
- **AC-OFFICE-LOOP-LIVENESS-005.8:** Classification shall not change `runs.status`
  or `runs.outcome`, nor alter what any existing consumer reports.
- **AC-OFFICE-LOOP-LIVENESS-005.9:** A run that has not reached a terminal status
  shall be given no shape and appear in no terminal-shape total; it is the
  stuck-run detector's input.

## Out of scope

Each exclusion below is a decision, not an omission.

- **Repairing anything this capability detects.** No overdue trigger fired, no
  stranded trigger re-armed, no stuck run requeued or cancelled, no agent launched
  — the separation `REQ-OFFICE-STALL-VISIBILITY-003` also makes. The stranded
  trigger is a real defect; its fix is a scheduling change owned elsewhere.
- **Reconciling `eligible trigger` with routine status.** The cron path selects on
  `enabled` alone and never reads `office_routines.status`, so a paused routine
  with an enabled trigger still fires. Eligibility tracks that behaviour; aligning
  the two is the scheduler's card.
- **Widening `runs.outcome`.** `docs/specs/task-delivery-ledger/spec.md` owns that
  column; this capability derives its vocabulary on top of it.
- **The terminal transition of `office_routine_runs`.** `status` stays
  `task_created` forever and `completed_at` is written on every status change.
  Both belong to card `032fadb8`, which owns `SyncRunStatus`.
- **A single-id chain for the heavy routine path.** `runs` carries no task id, so
  `AC-OFFICE-LOOP-LIVENESS-002.9` delivers a two-key join rather than one id end
  to end. Closing it needs the workflow engine to carry a causation id through
  task creation.
- **Recording a causation id on `task_sessions`.** The run names its session, so
  one id reaches it through the run.
- **Runs whose workspace cannot be resolved.** No foreign key makes
  `runs.agent_profile_id` name a live profile, so such a run reaches no workspace
  and decides no verdict. It counts under `AC-OFFICE-LOOP-LIVENESS-003.8`'s
  unattributed label rather than being dropped.
- **Queue latency for a run already launched once.** A post-start fallback and a
  stale-claim recovery both return a run to `queued` with `claimed_at` cleared,
  and `runs` has no column dating that return. The fallback is excluded by the
  attempt-sequence test; the recovery is not, so such a run is aged from its
  original request and its reported wait overstates. It is genuinely unclaimed,
  so it is reported rather than hidden. Closing this needs a requeue instant on
  `runs`, owned elsewhere.
- **Notifying anybody** (card `5470c372`, which this unblocks); **WIP limits,
  causation depth caps and self-trigger suppression** (card `7dbc0b94`, which
  consumes this id); **retention** (card `9a7aa679`).
- **A global, unauthenticated metrics endpoint.** Every read added here is
  workspace-scoped and goes through the existing Office authorization guard.
- **Any frontend.** No screen renders the verdict, the counters or `last_run_at`.
- **Correcting the historical `no_agent_launched` rows.** They are classified, not
  rewritten; this capability performs no data migration.
