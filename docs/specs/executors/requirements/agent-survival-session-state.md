---
status: draft
system: executors
created: 2026-09-02
owners:
  - kandev
---

# Survived Session State and Capability Gating Requirements

## Overview

[Agent survival across a backend restart](agent-survival-across-restart.md) states
how a control server and its agent subprocesses outlive their backend and how a
later backend re-tracks them. This document states what must then be true of the
session a user looks at: that its state is honest, that a turn which finished while
nobody was attached is not lost, and that an operator can switch the whole
capability off and get today's behavior back.

The split is by subject, not by size. The other document owns process lifetime and
the re-tracking mechanism; this one owns the observable outcome. Both are consumed
by one design, in three parts, listed under `## Related design` below.

## Terminology

**Control server**, **instance**, **adoption**, **recovery inventory**, **attached**,
**re-tracking** and **reconciliation pass** are used exactly as defined in
[standalone control-server ownership](standalone-control-server-ownership.md#terminology).

## Requirements

### REQ-EXECUTORS-SURVIVAL-003: Session and task state stay truthful after re-tracking

**Intent:** Startup reconciliation exists to make state honest after a restart that
killed every agent. Once agents survive, that same reconciliation reports a live
session as interrupted and idle and abandons a turn still running, so a successful
survival would surface as an abandoned task.

**User story:** As a Kandev user, I want a session that survived a restart to show
as still working, so I can tell survival from interruption without opening it.

#### Acceptance criteria

- **AC-EXECUTORS-SURVIVAL-003.1:** Re-tracking shall reach an outcome for every
  standalone recovery-inventory record before startup state reconciliation begins,
  and that reconciliation shall read those outcomes.
- **AC-EXECUTORS-SURVIVAL-003.7:** Re-tracking shall reach an outcome within a
  single bounded time covering adoption, enumeration and the reconstruction of every
  record together, rather than a bound applied per instance. That bound shall be
  configurable, shall default to thirty seconds, shall be accepted only between one
  second and five minutes inclusive with a value outside that range rejected at startup
  rather than clamped, and shall start when the system first contacts the recorded
  control endpoint. The lower end exists because a bound at or near zero elapses before
  any record can reach an outcome, stopping every surviving instance while reporting the
  capability enabled; the upper end because this bound delays startup state
  reconciliation, the cost AC-EXECUTORS-SURVIVAL-003.1's ordering already accepts, which
  must not grow without limit. When it elapses, every record without an outcome shall be
  treated as not re-tracked, its live instance stopped through the path of
  AC-EXECUTORS-SURVIVAL-002.6, and startup state reconciliation shall proceed rather
  than wait. An adoption operation in flight when the bound elapses shall be allowed to
  finish and its result recorded, never cancelled, because abandoning a credential
  rotation midway would leave the control server holding a replacement no backend will
  confirm; an adoption completing after the bound shall re-track nothing. A stop whose
  retry sequence under AC-EXECUTORS-SURVIVAL-002.15 is in flight and not yet exhausted
  when the bound elapses shall likewise be allowed to finish and its result recorded,
  never cancelled: its session is not re-tracked either way, so letting it finish decides
  only whether a live agent nothing is tracking is left running. A session whose stop is
  still in flight when the bound elapses shall retain its guard until that stop resolves,
  rather than releasing it at the bound as AC-EXECUTORS-SURVIVAL-002.8 otherwise
  requires, because the guard exists to keep a second agent off a session whose live
  instance may still be running and that is exactly what the stop has not yet decided. On
  success the guard is released then; on failure AC-EXECUTORS-SURVIVAL-002.16 governs and
  it is retained. When enumeration itself did not complete, no instance shall be stopped
  and AC-EXECUTORS-SURVIVAL-002.12 governs instead. A re-tracking that completes after
  the bound shall not publish its session as re-tracked and shall not alter any session
  or task state.
- **AC-EXECUTORS-SURVIVAL-003.2:** When a session was re-tracked, the
  system shall not mark its task as interrupted on account of the restart.
- **AC-EXECUTORS-SURVIVAL-003.3:** When a session was re-tracked and its
  turn had not reached a terminal outcome, the system shall leave the session
  in its running state rather than moving it to a waiting-for-input state.
- **AC-EXECUTORS-SURVIVAL-003.4:** When a session was re-tracked, the
  system shall not abandon that session's open prompt turns.
- **AC-EXECUTORS-SURVIVAL-003.5:** When a session was re-tracked and its
  task was in progress, the system shall leave the task in progress rather than
  moving it to review on account of the restart.
- **AC-EXECUTORS-SURVIVAL-003.6:** When the system classifies the process liveness
  of a standalone recovery-inventory record, that classification shall reflect
  whether that record's own instance is present, and shall not report the record as
  live solely because the shared control server is running. When presence cannot be
  determined, the classification shall be unknown, never live and never dead. Within
  one reconciliation pass the presence check shall reuse a single enumeration rather
  than issuing one per record. A classification made outside a pass, for a single
  record, shall issue its own enumeration and shall not read one cached by a pass.
  Any enumeration a classification reads shall have been taken after re-tracking
  reached an outcome for every record, so an instance that re-tracking itself stopped
  under AC-EXECUTORS-SURVIVAL-002.10 or AC-EXECUTORS-SURVIVAL-003.7 is never
  classified live and never escapes repair. A record whose own stop was still in
  flight when that enumeration was taken shall be classified unknown rather than read
  from it, until that stop resolves. AC-EXECUTORS-SURVIVAL-003.7 lets such a stop
  outlive the bound while startup state reconciliation proceeds, so the enumeration
  can legitimately still show that instance, and reading it there would report live
  the one instance recovery is in the act of stopping. A record this backend itself
  created during this process lifetime shall be classified against the enumeration of
  whichever control server this backend is using, whether it adopted that server or
  started it. The adoption-dependent rules below, which name the server presence is
  determined against and say what happens when this backend adopted none, govern only
  records this backend did not create, which are the ones inherited from an earlier
  launch; the timing rule and the disabled-capability rule closing this criterion
  govern every record, whichever backend created it. Without that split, on a start
  with no survivor every record the backend went on to create would fall to the
  process-identifier probe below and be reported live for as long as the shared
  control server ran, which the first sentence of this criterion forbids. For an
  inherited record, presence shall be determined only against a control server this
  backend adopted. When this backend adopted none, the classification shall turn on
  whether a control server answered at the recorded endpoint, and on nothing else.
  When one answered and was left running, which is every path on which adoption did
  not complete against a reachable server, every inherited record shall be unknown: a
  surviving instance runs on the server this backend did not adopt, so its absence
  from a freshly started one is not evidence that it is gone, and classifying it dead
  there would repair or prune a record whose agent is still running, the outcome
  [AC-EXECUTORS-CONTROL-OWNERSHIP-004.7](standalone-control-server-ownership.md)
  exists to prevent, reached through reconciliation rather than adoption. When
  nothing answered at the recorded endpoint, or no endpoint was recorded, no control
  server holds any inherited instance and the classification shall be the same
  process-identifier probe the disabled capability uses, so records that are
  genuinely dead are still repaired. A classification made outside a reconciliation
  pass before re-tracking has reached an outcome for every record shall likewise be
  unknown, and shall neither enumerate nor wait: such a caller runs from ordinary
  request and timer paths at any moment, including while recovery is still in
  progress, so no enumeration satisfying the rule above can exist for it yet, and
  blocking it would stall a user-facing request for the whole of the bound of
  AC-EXECUTORS-SURVIVAL-003.7. When the capability is disabled, this classification
  shall remain the process-identifier probe in force today, as
  AC-EXECUTORS-SURVIVAL-005.1 requires.

### REQ-EXECUTORS-SURVIVAL-004: A turn that ends while unattached is not lost

**Intent:** An agent finishing its turn while no backend is attached has no one to
report to. Stream attachment is asynchronous, so a terminal event can also land
between re-tracking an execution and attaching its streams.

**User story:** As a Kandev user, I want a turn that finished during a restart to
appear finished, so a completed task does not sit forever running.

#### Acceptance criteria

- **AC-EXECUTORS-SURVIVAL-004.1:** When an agent reaches a terminal turn
  outcome while no backend is attached, the system shall retain that outcome,
  together with an identifier for the turn it belongs to that the control server
  itself assigns and reports, until an owning backend acknowledges it by that identifier
  under AC-EXECUTORS-SURVIVAL-004.6. Retrieval alone shall not end the retention. That
  identifier shall be unique across every turn of every instance the control server
  supervises for as long as that control server runs, and shall never be reused within
  that lifetime, so that an identifier names at most one turn. Without that, a retried
  acknowledgement could discard a different turn's outcome and a delivered event could
  deduplicate against the wrong turn, defeating the exactly-once guarantee
  AC-EXECUTORS-SURVIVAL-004.4 and AC-EXECUTORS-SURVIVAL-004.6 exist to provide.
  Identifiers need not be unique across control-server lifetimes: one presented from an
  earlier lifetime is one this server never held, which AC-EXECUTORS-SURVIVAL-004.6
  already accepts as a no-op.
- **AC-EXECUTORS-SURVIVAL-004.2:** When a backend re-tracks an instance, the
  system shall retrieve that instance's turn status and apply any terminal outcome
  reached while unattached before publishing that session's state. Publishing it as
  running is the outcome when no terminal outcome was retained, which is the case
  AC-EXECUTORS-SURVIVAL-004.5 isolates; when a terminal outcome was retrieved and applied,
  the session shall be published in whatever state that outcome produces, the same state
  the turn would have reached had a backend been attached when it ended. Publishing such a
  session as running would report a turn that finished during the outage as still working,
  the failure AC-EXECUTORS-SURVIVAL-003.3 already conditions its running state on avoiding.
- **AC-EXECUTORS-SURVIVAL-004.3:** When a terminal event is produced
  between re-tracking an execution and completing its stream attachment, the
  system shall deliver that event once the streams attach.
- **AC-EXECUTORS-SURVIVAL-004.4:** When a turn's terminal outcome is observed
  through both retained turn status and a delivered event, the system shall apply
  exactly one terminal outcome for that turn, keeping the first applied. Whether
  two observations describe the same turn shall be decided by the turn identifier
  of AC-EXECUTORS-SURVIVAL-004.1, which both observations carry, and not by any
  in-memory counter that a restart resets.
- **AC-EXECUTORS-SURVIVAL-004.5:** The turn-status read of
  AC-EXECUTORS-SURVIVAL-004.2 shall be bounded by the same per-read timeout and
  retry count as AC-EXECUTORS-SURVIVAL-002.13, and its outcomes shall be
  distinguished. When the control server answers that no terminal outcome is
  retained, the session shall be published as running. When the read fails without
  answering and its retries are exhausted, the system shall not publish that session
  as running: it shall place the instance on the not-re-tracked path of
  AC-EXECUTORS-SURVIVAL-003.7 and stop it, because a stopped session is repaired by the
  existing stale-execution path while a falsely-running one is repaired by nothing.

- **AC-EXECUTORS-SURVIVAL-004.6:** Retrieving a retained terminal outcome shall not
  discard it. The retrieval of AC-EXECUTORS-SURVIVAL-004.2 shall be repeatable and shall
  return the same outcome and turn identifier until the system acknowledges that outcome
  by naming its turn identifier, and only that acknowledgement shall discard it. An
  acknowledgement naming a turn identifier the control server no longer holds, or never
  held, shall be accepted and shall change nothing, so a retried acknowledgement is safe.
  The acknowledgement shall be sent only after that outcome has been durably applied to
  session and task state, and never in the same exchange as the retrieval that returned it.
  Discarding on retrieval, or acknowledging in the same exchange as it, both lose the
  outcome permanently when a backend stops before applying it, leaving nothing retained
  for the next backend to find: the loss REQ-EXECUTORS-SURVIVAL-004 exists to prevent,
  reintroduced by the mechanism meant to close it. Applying a terminal outcome to a turn that has already reached that outcome
  in durable session and task state shall change nothing, so an outcome retained because
  its acknowledgement never arrived may be applied again by a later backend without
  effect; no durable record of which turn identifiers were applied is required, and the
  in-memory record of AC-EXECUTORS-SURVIVAL-004.4 remains scoped to one backend process
  lifetime.

### REQ-EXECUTORS-SURVIVAL-005: The capability is opt-in and its disabled behavior is unchanged

**Intent:** Every mechanism here changes process lifetime, so a failure mode is an
orphaned agent or a lost session. The operator needs a switch returning the system to
behavior that has shipped for months.

**User story:** As a Kandev operator, I want a single switch returning agent lifetime
to today's behavior, so I can turn off a misbehaving capability without downgrading.

#### Acceptance criteria

- **AC-EXECUTORS-SURVIVAL-005.1:** When the capability is disabled, the system
  shall terminate the control server with the backend, attempt no adoption, report
  no recovered instances, and reproduce today's observable restart behavior.
- **AC-EXECUTORS-SURVIVAL-005.2:** The capability shall default to
  disabled in every shipped runtime profile.
- **AC-EXECUTORS-SURVIVAL-005.3:** When a session runs in passthrough mode,
  the system shall behave as if the capability were disabled for that session,
  shall never report it as re-tracked, and shall not represent it as having
  survived. Such a session shall not be guarded under
  [AC-EXECUTORS-SURVIVAL-002.8](agent-survival-across-restart.md), and no request to start
  an execution for it shall be refused with that criterion's recovery reason, even though
  the recovery inventory names it exactly as it names any other standalone session. A
  passthrough agent runs on a terminal the backend process itself owns, so it never
  survives a restart and can never be a re-tracking candidate, and refusing its launch for
  the duration of the bound of AC-EXECUTORS-SURVIVAL-003.7 would be a user-visible change
  on the very sessions this criterion promises are unaffected. A session shall be
  identified as passthrough here from the durable session record's own passthrough mode,
  which outlives a restart; the backend's in-memory execution state shall not be
  consulted, because it is empty when AC-EXECUTORS-SURVIVAL-002.8 takes its guards and
  would report every session as not passthrough. When that mode cannot be read for a
  record, the session shall be guarded rather than excluded, because failing to guard a
  re-tracking candidate is the two-agent outcome AC-EXECUTORS-SURVIVAL-002.8 exists to
  prevent, while a wrongly guarded passthrough session costs only one refused launch
  until the bound elapses.
- **AC-EXECUTORS-SURVIVAL-005.4:** When the host platform does not support
  the survivable shutdown this capability requires, the system shall behave as if
  it were disabled and shall report its state as unavailable, distinctly from
  operator-disabled, on the same runtime-capability surface that reports whether
  the capability is enabled, together with the reason it is unavailable. An
  operator shall be able to tell an unsupported platform from a switched-off
  capability without reading a log.
- **AC-EXECUTORS-SURVIVAL-005.5:** When the capability is disabled while a
  detached control server from an earlier launch is still running, the next backend
  shall stop it and its instances rather than leave them running unowned, but only
  when it can prove that server is its own by the same test
  [standalone control-server ownership](standalone-control-server-ownership.md)
  applies to adoption. A server it cannot prove is its own is left untouched. This
  stop shall be issued as the ownership-shutdown operation of
  [AC-EXECUTORS-CONTROL-OWNERSHIP-002.9](standalone-control-server-single-driver.md),
  so that a backend holding only a superseded
  credential can still perform it without adopting, which
  AC-EXECUTORS-SURVIVAL-005.1 forbids it to do.
- **AC-EXECUTORS-SURVIVAL-005.7:** A change to the capability shall take effect at
  the next backend start, and shall not change the lifetime of a control server or
  an instance while the backend that observes the change is still running. The
  cleanup that AC-EXECUTORS-SURVIVAL-005.5 requires at the next start is not an
  exception to this: it is that next start applying the new value, not a running
  backend's lifetime changing underneath it.
- **AC-EXECUTORS-SURVIVAL-005.6:** When the capability is enabled, the system
  shall adopt a surviving control server only as permitted by
  [standalone control-server ownership](standalone-control-server-ownership.md),
  and shall report no recovered instances when adoption is refused.

## Related design

- [Part 1: lifetime and ownership contracts](../system-design/agent-survival-across-restart-01.md)
- [Part 2: control flow, failure and scope](../system-design/agent-survival-across-restart-02.md)
- [Part 3: recovery data contracts](../system-design/agent-survival-across-restart-03.md)

## Out of scope

- Process lifetime, adoption, and the re-tracking mechanism itself, all owned by
  [agent survival across a backend restart](agent-survival-across-restart.md).
- Which control server a backend may adopt, and refusal across an incompatible
  protocol change, owned by
  [standalone control-server ownership](standalone-control-server-ownership.md);
  which backend drives it and what happens when nobody owns one, owned by
  [standalone control-server single driver and unowned lifetime](standalone-control-server-single-driver.md).
- Survival of passthrough sessions. A passthrough agent runs on a terminal owned by
  the backend process, so surviving one means moving that ownership into the control
  server: a separate capability, excluded rather than half-delivered.
  AC-EXECUTORS-SURVIVAL-005.3 states what this contract does require of them. The
  same applies to user-owned terminals, which use that same mechanism.
- Provider-native resume semantics and resume-token clearing, owned by
  [agent resume and runtime recovery](../../agents/requirements/agent-resume-runtime-recovery.md).
- The cold-resume cost for sessions that did not survive, owned by
  [startup-listener-before-recovery](../../startup-listener-before-recovery/spec.md).
- Replaying transcript output beyond the retained events of
  AC-EXECUTORS-SURVIVAL-001.5 and the terminal outcome of this document's
  REQ-EXECUTORS-SURVIVAL-004.
