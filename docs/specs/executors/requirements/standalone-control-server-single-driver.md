---
status: draft
system: executors
created: 2026-09-02
owners:
  - kandev
---

# Standalone Control-Server Single Driver and Unowned Lifetime Requirements

## Overview

[Standalone control-server ownership](standalone-control-server-ownership.md)
states what a backend must prove before it may adopt a surviving control server.
This document states what is true afterwards: that exactly one backend drives the
server at a time, and that a server nobody drives stops its instances and exits
rather than leaving agents running unattended.

The split is by subject, not by size. The other document owns admission; this one
owns tenure. Removing the parent-death binding that
[agent survival across a backend restart](agent-survival-across-restart.md)
requires is what makes both necessary: without admission a foreign process could be
driven, and without tenure an abandoned server would leak.

## Terminology

**Control server**, **instance**, **adoption**, **unowned period**, **capability
set**, **recovery inventory**, **owning backend** and **attached** are used exactly
as defined in
[standalone control-server ownership](standalone-control-server-ownership.md).

## Requirements

### REQ-EXECUTORS-CONTROL-OWNERSHIP-002: Exactly one backend drives an adopted control server

**Intent:** Adoption introduces a second potential driver of a live agent. Two
drivers on one instance produce interleaved turns, duplicated completions, and a
transcript that describes neither.

**User story:** As a Kandev operator, I want a superseded backend to lose control
the moment another backend adopts, so a restart overlap cannot corrupt a turn.

#### Acceptance criteria

- **AC-EXECUTORS-CONTROL-OWNERSHIP-002.1:** When a backend adopts a control server,
  the system shall replace the ownership credential as part of adoption, and shall
  reject every subsequent operation presented with the superseded credential once
  that replacement is confirmed.
- **AC-EXECUTORS-CONTROL-OWNERSHIP-002.7:** Credential replacement shall take effect
  in two steps: the control server shall continue to accept the superseded
  credential until the adopting backend confirms, naming the rotation identifier of
  AC-EXECUTORS-CONTROL-OWNERSHIP-002.6, that it has durably stored the replacement,
  and shall stop accepting it on that confirmation. Durably storing the replacement
  shall mean that both the replacement credential is written to the secret store and the
  control-server record's credential reference is updated to name it, and the
  confirmation shall be sent only after both writes have durably completed. Their order
  relative to each other is unconstrained, because a crash between them is recoverable
  either way: with only the secret written the record still names the superseded
  credential, which the control server still accepts and
  AC-EXECUTORS-CONTROL-OWNERSHIP-002.10 replays; with only the record written it names a
  credential the secret store does not hold, which is the credential-unavailable path of
  AC-EXECUTORS-CONTROL-OWNERSHIP-001.9. Sending it
  earlier is the one ordering that loses the server: the control server stops accepting
  the superseded credential while the record still names it, so the next start presents
  a credential that authenticates nothing and cannot adopt a server it owns until the
  unowned period elapses, which is exactly the lockout this two-phase window exists to
  make impossible. When either write fails the confirmation shall not be sent, and
  AC-EXECUTORS-CONTROL-OWNERSHIP-002.5 governs the incomplete adoption. An interruption at
  any point shall therefore leave at least one credential that both sides hold, so
  a backend that fails between replacing and storing can adopt again rather than
  being locked out of a server it owns. Within that window the superseded
  credential shall authenticate only a further adoption attempt or the
  ownership-shutdown operation of AC-EXECUTORS-CONTROL-OWNERSHIP-002.9; it shall not
  authenticate any other instance operation and shall not establish a stream, so the
  window cannot reintroduce the second driver that
  AC-EXECUTORS-CONTROL-OWNERSHIP-002.2 exists to prevent. When no confirmation
  arrives, the superseded credential shall remain acceptable on those terms until
  the unowned period elapses.
- **AC-EXECUTORS-CONTROL-OWNERSHIP-002.5:** When credential replacement does not
  succeed, adoption shall be incomplete: the system shall report no recovered
  instances for that control server and shall not issue any further operation to
  it until adoption is attempted again.
- **AC-EXECUTORS-CONTROL-OWNERSHIP-002.6:** Every credential replacement shall carry
  a rotation identifier that the control server assigns from a strictly increasing
  sequence, returns to the adopting backend with the replacement credential, and
  requires on the confirmation of AC-EXECUTORS-CONTROL-OWNERSHIP-002.7. The
  credential issued by the highest-numbered rotation the control server has performed
  shall be the only one that authenticates an instance operation or a stream. A
  confirmation naming an earlier rotation identifier shall be accepted and shall have
  no effect, so a delayed or duplicated confirmation can never revoke a credential
  issued after it. A repeated confirmation of the highest-numbered rotation shall
  likewise be accepted and have no effect beyond the first, so a retried confirmation
  is safe. A confirmation naming an identifier the control server has not issued shall
  be rejected and shall change nothing, because it cannot have come from a backend
  this server rotated for.
- **AC-EXECUTORS-CONTROL-OWNERSHIP-002.8:** At most two credentials shall be
  acceptable to a control server at any moment: the replacement issued by its
  highest-numbered rotation, for every operation, and the single credential that
  rotation directly superseded, on the adoption-only terms of
  AC-EXECUTORS-CONTROL-OWNERSHIP-002.7. Every other credential shall cease to
  authenticate anything, including a replacement issued by an earlier rotation that
  was never confirmed. Without this an abandoned unconfirmed replacement would stay
  valid outside any backend's custody, which is the second driver
  AC-EXECUTORS-CONTROL-OWNERSHIP-002.2 forbids.
- **AC-EXECUTORS-CONTROL-OWNERSHIP-002.9:** The control server shall expose one
  ownership-shutdown operation that stops every instance it supervises, together with
  their agent subprocesses, and exits. It shall be authenticated by any credential in
  the acceptable set of AC-EXECUTORS-CONTROL-OWNERSHIP-002.8, including the superseded
  credential on its adoption-only terms, and shall require no prior adoption and no
  credential rotation. Admitting the superseded credential here does not reintroduce
  the second driver AC-EXECUTORS-CONTROL-OWNERSHIP-002.2 forbids, because the
  operation only destroys: it cannot drive an instance, observe its transcript, or
  outlive the call. Without it a backend that holds only a superseded credential can
  neither adopt nor stop, which would make AC-EXECUTORS-SURVIVAL-005.5 unsatisfiable
  on exactly the crash path AC-EXECUTORS-CONTROL-OWNERSHIP-002.7 exists to create.
- **AC-EXECUTORS-CONTROL-OWNERSHIP-002.10:** Credential replacement shall be idempotent
  under retry. When an adoption attempt presents the credential that the highest-numbered
  rotation directly superseded, and no confirmation has been received for that rotation,
  the control server shall return that same rotation's identifier and replacement
  credential again and shall not allocate a new rotation, so no new identifier is drawn
  from the strictly increasing sequence of AC-EXECUTORS-CONTROL-OWNERSHIP-002.6 and the
  acceptable set of AC-EXECUTORS-CONTROL-OWNERSHIP-002.8 does not move. It shall allocate a
  new rotation only when the presented credential is the one that rotation issued. This satisfies
  AC-EXECUTORS-CONTROL-OWNERSHIP-002.1 rather than excepting it: the attempt still ends
  with the adopting backend holding the replacement, which is what that criterion
  requires. Without this a backend that never receives a rotation's response is driven to
  retry, each retry rotates again, and after the second the credential it still holds falls
  outside the two-credential set of AC-EXECUTORS-CONTROL-OWNERSHIP-002.8 and it is locked
  out of a server it owns until the unowned period elapses, which is the outcome the
  two-phase window of AC-EXECUTORS-CONTROL-OWNERSHIP-002.7 exists to make impossible. A
  retry is therefore unbounded-safe: any number of attempts presenting the superseded
  credential converge on the same rotation.
- **AC-EXECUTORS-CONTROL-OWNERSHIP-002.2:** When a backend adopts a control server,
  the system shall terminate every event and workspace stream established under a
  superseded credential, so a prior holder cannot keep consuming an instance's
  events.
- **AC-EXECUTORS-CONTROL-OWNERSHIP-002.3:** When an operation is rejected for a superseded
  credential, the system shall stop treating the affected executions as its own,
  shall not retry it, and shall not acquire a fresh credential without adopting
  again.
- **AC-EXECUTORS-CONTROL-OWNERSHIP-002.4:** When two backends would otherwise act on
  the same installation, the system shall continue to refuse the second at startup
  through the runtime-state ownership contract in
  [port collision and backend ownership safety](port-collision-safety.md), and
  this capability shall not weaken or bypass that refusal.

### REQ-EXECUTORS-CONTROL-OWNERSHIP-003: An unowned control server terminates itself

**Intent:** Removing the parent-death binding must not trade a restart problem
for a process leak. An unowned control server keeps agents running and spending
tokens with nobody watching.

**User story:** As a Kandev operator, I want an abandoned control server to shut
itself down, so a backend that never returns leaves no agents running
unattended.

#### Acceptance criteria

- **AC-EXECUTORS-CONTROL-OWNERSHIP-003.1:** When a control server has had no owning
  backend for the unowned period, the system shall stop every instance it
  supervises, together with their agent subprocesses, and shall exit.
- **AC-EXECUTORS-CONTROL-OWNERSHIP-003.2:** While a backend holds ownership, the
  system shall keep that ownership current without operator action and shall not
  begin an unowned shutdown, whether or not any user is interacting with a session
  and whether or not any agent is mid-turn. Ownership shall be renewed at an
  interval strictly shorter than one third of the unowned period, so that after two
  consecutive failed renewals a third attempt still falls strictly inside the period;
  a successful adoption shall itself count as a renewal. Expiry shall be judged by
  the elapsed time since the last successful renewal being greater than or equal to
  the unowned period. Because the interval is strictly shorter than one third, the
  third attempt is always due strictly before that test can be reached, so the
  guarantee does not rest on any tiebreak between a renewal and an expiry test
  falling due at the same instant.
- **AC-EXECUTORS-CONTROL-OWNERSHIP-003.8:** The control server shall expose one
  ownership-claim operation that both establishes and renews the claim, authenticated
  by the credential of AC-EXECUTORS-CONTROL-OWNERSHIP-002.6 and naming no instance. A
  successful call shall set the last-successful-renewal instant to the control server's
  own monotonic now. Exactly two other operations shall also set it, and no others: the
  bootstrap handshake that mints the first credential, and a successful credential
  rotation under AC-EXECUTORS-CONTROL-OWNERSHIP-002.7, including the idempotent replay of
  AC-EXECUTORS-CONTROL-OWNERSHIP-002.10, which allocates no new rotation but does prove a
  live backend is still trying to adopt. Beyond those three, no operation
  shall renew ownership: neither an instance operation, nor an open stream, nor
  enumeration, so a
  backend that is attached but idle is not treated as gone and a backend that is busy
  but has stopped renewing is not treated as present. The handshake carve-out is what
  gives a freshly spawned server a claim without a separate call; a server that is
  spawned but never completes that handshake has no successful renewal and its unowned
  period runs from its start, so a backend that dies mid-handshake leaks nothing. The
  rotation carve-out is what makes AC-EXECUTORS-CONTROL-OWNERSHIP-003.2's "a successful
  adoption shall itself count as a renewal" true of an actual operation rather than of
  nothing: rotation is the first authenticated operation an adopting backend issues, so
  without it the adopting backend would inherit whatever remained of the previous
  owner's period, and a server whose owner died just under that period ago would begin
  its unowned shutdown moments into recovery and stop the very instances being
  re-tracked. An adopting backend shall begin issuing the ownership-claim operation on
  the interval of AC-EXECUTORS-CONTROL-OWNERSHIP-003.2 from the point its rotation
  succeeds, and shall not wait until re-tracking has finished to do so.
- **AC-EXECUTORS-CONTROL-OWNERSHIP-003.3:** When ownership renewal fails for a
  reason that does not establish the owner is gone, the system shall not
  shorten the unowned period below its configured value. The claim shall be judged
  by the time elapsed since the last successful renewal, measured on the control
  server's own monotonic clock, and not by the outcome of the most recent attempt
  nor by any wall-clock value either side supplies.
- **AC-EXECUTORS-CONTROL-OWNERSHIP-003.4:** The unowned period shall be
  configurable and shall default to ten minutes. While per-instance idle reaping is
  enabled, the unowned period shall be shorter than the idle timeout, so an unowned
  control server never has its instances reaped for idleness before the unowned
  shutdown runs. Because both values are independently configurable, the system shall
  validate the pair when the control server starts; when idle reaping is enabled and
  the configured unowned period is not shorter than the idle timeout, it shall reduce
  the unowned period to half the idle timeout and shall record that adjustment,
  rather than starting with the ordering inverted.
- **AC-EXECUTORS-CONTROL-OWNERSHIP-003.6:** When the per-instance idle timeout is set
  to the value that disables idle reaping, the ordering constraint of
  AC-EXECUTORS-CONTROL-OWNERSHIP-003.4 shall be treated as satisfied and the
  configured unowned period shall be used unchanged, because there is no reaper for
  the unowned shutdown to precede. The system shall never derive an unowned period
  from a disabled idle timeout. Disabling idle reaping is a supported configuration,
  so treating its sentinel as an ordinary duration would let a valid setting collapse
  the unowned period and stop every surviving instance at start.
- **AC-EXECUTORS-CONTROL-OWNERSHIP-003.7:** The unowned period in force after any
  validation or adjustment shall be at least one minute. A configured or derived
  value below that floor shall be raised to it and the adjustment recorded, so that
  no configuration produces an unowned period at or near zero, which would expire
  ownership before an adopting backend could claim it and would leave the renewal
  interval of AC-EXECUTORS-CONTROL-OWNERSHIP-003.2 with no usable value. The floor
  shall be applied after the adjustment of AC-EXECUTORS-CONTROL-OWNERSHIP-003.4 and
  shall take precedence over it: when the idle timeout is short enough that no value
  satisfies both the floor and that ordering, the system shall use the floor, record
  that the ordering could not be honoured, and start. The consequence is only that
  the idle reaper may stop instances before the unowned shutdown runs, which
  AC-EXECUTORS-CONTROL-OWNERSHIP-003.4 already treats as tolerable, whereas honouring
  the ordering there would reintroduce the near-zero period this criterion exists to
  forbid.
- **AC-EXECUTORS-CONTROL-OWNERSHIP-003.9:** The decision to begin an unowned shutdown
  shall be taken once and shall be irreversible. Once taken, the control server shall
  refuse every ownership-claim operation and every adoption attempt with a distinct
  shutting-down outcome, and no claim, adoption or renewal shall cancel a shutdown already
  begun. A claim that the control server accepts strictly before that decision shall
  prevent the shutdown, so a backend that renews in time is never stopped. A backend
  receiving the shutting-down outcome shall treat that control server exactly as
  AC-EXECUTORS-CONTROL-OWNERSHIP-001.8 treats a recorded endpoint that nothing answers: it
  shall report no recovered instances, shall issue no further operation to it, and shall
  start a fresh control server. Without this ordering a backend could adopt a server whose
  instances are already being stopped and re-track them, publishing sessions whose agents
  are being killed as it publishes them — the failure
  [AC-EXECUTORS-SURVIVAL-002.15](agent-survival-across-restart.md) exists to prevent,
  arriving by a route that criterion does not cover.
- **AC-EXECUTORS-CONTROL-OWNERSHIP-003.5:** When an unowned shutdown stops an
  instance, that instance's recovery-inventory record shall be repaired so it no
  longer claims a live process, preserving its resume token and worktree identity.
  That repair shall be performed by the next backend to start, through the existing
  stale-execution repair path, and not by the control server: by construction no
  backend is attached at that moment, and the control server has no access to the
  durable store. The deferral is a consequence of that absence, not a rule about who
  may repair; when a backend is attached and stops a server itself, it repairs
  immediately, as AC-EXECUTORS-CONTROL-OWNERSHIP-004.3 requires. Until that repair runs the record may name a process that no
  longer exists, which is the condition that path already exists to correct.

## Related design

- [Part 1: lifetime and ownership contracts](../system-design/agent-survival-across-restart-01.md)
- [Part 2: control flow, failure and scope](../system-design/agent-survival-across-restart-02.md)
- [Part 3: recovery data contracts](../system-design/agent-survival-across-restart-03.md)

## Out of scope

- What a backend must prove before adopting, and refusal across an incompatible
  protocol change, both owned by
  [standalone control-server ownership](standalone-control-server-ownership.md).
- Which sessions are re-tracked after adoption, owned by
  [agent survival across a backend restart](agent-survival-across-restart.md).
- Refusing a second backend on one installation, owned by
  [port collision and backend ownership safety](port-collision-safety.md).
- Ownership of agentctl processes on the Docker, SSH, Sprites, Kubernetes, and
  remote-Docker executors. They adopt this contract in later work.
