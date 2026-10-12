---
status: draft
system: executors
created: 2026-09-02
owners:
  - kandev
---

# Agent Survival Across Backend Restart Requirements

## Overview

A running agent stops whenever the Kandev backend restarts or is upgraded.
In-flight work is discarded and the session is rebuilt from the agent CLI's own
store, a cold resume measured at up to roughly 31 minutes per task under load (see
[startup-listener-before-recovery](../../startup-listener-before-recovery/spec.md)).

Kandev already built the receiving half: the lifecycle manager reconstructs an
execution for every instance a runtime reports as recovered, and that path has never
run because every runtime reports nothing. For worktree and local the gap is larger:
both run on the standalone runtime, whose control server is deliberately terminated
with the backend by several independent mechanisms and whose agent processes are
reaped once nothing is attached. Survival needs a detached process lifetime and
truthful session state after re-tracking.

This document owns process lifetime and the re-tracking mechanism. What must then be
true of the session a user looks at, and how the whole capability is gated, is stated
in [survived session state and capability gating](agent-survival-session-state.md).
Which control server a backend may adopt is stated in
[standalone control-server ownership](standalone-control-server-ownership.md), and
which backend drives it and what happens when nobody owns one in
[standalone control-server single driver and unowned lifetime](standalone-control-server-single-driver.md).
None of these is restated here. This system owns these contracts because executor ownership
includes executor-specific failure and recovery contracts; the agent system owns
provider resume semantics and consumes them without owning process lifetime.

## Terminology

Every term this contract uses, including **detached control server**, **survivable
shutdown**, **re-tracking** and **reconciliation pass**, is defined in
[standalone control-server ownership](standalone-control-server-ownership.md#terminology),
which is the single home for this feature's shared vocabulary. They are not restated here.

## Requirements

### REQ-EXECUTORS-SURVIVAL-001: Worktree and local agent work survives a backend restart

**Intent:** A backend restart or upgrade, operator-initiated or from a crash, does
not interrupt agent work already running on worktree or local.

**User story:** As a Kandev operator, I want to restart or upgrade the backend
without stopping my running agents, so a routine restart discards no in-flight work.

#### Acceptance criteria

- **AC-EXECUTORS-SURVIVAL-001.1:** When the backend shuts down gracefully
  and the capability is enabled, the system shall leave the control server and
  every agent subprocess it supervises running, and shall not issue a stop to
  any of them.
- **AC-EXECUTORS-SURVIVAL-001.2:** When the backend exits without a graceful
  shutdown, including `SIGKILL`, the system shall leave the control server and its
  agent subprocesses running without depending on any backend shutdown step.
- **AC-EXECUTORS-SURVIVAL-001.3:** While no backend is attached, the
  system shall keep the control server's diagnostic output writable, so that
  emitting a log line cannot terminate it. That output shall be written to a durable
  file under the Kandev home directory, and to a different file from the one the
  backend writes its own logs to, because two processes rotating one file and one set
  of rollover journals corrupt both. The control server shall bound that file itself,
  by a maximum size, a retained-backup count and a maximum age that are fixed values
  of this capability rather than operator settings, so the sink adds no configuration
  key and no environment variable of its own. That location shall be recorded in the
  control-server record required by AC-EXECUTORS-CONTROL-OWNERSHIP-001.1.
- **AC-EXECUTORS-SURVIVAL-001.4:** When a backend adopts a control server,
  the system shall re-track each surviving instance without starting a new agent
  subprocess and without issuing a resume to the agent provider.
- **AC-EXECUTORS-SURVIVAL-001.5:** While no backend is attached, the system shall
  retain agent events already produced and deliver them when a backend next
  attaches, preserving each instance's own production order. No order is defined
  between events of different instances.
- **AC-EXECUTORS-SURVIVAL-001.6:** The per-instance retention limit shall be a
  bounded, non-zero number of events, shall be configurable, and shall default to one
  hundred events. When retained undelivered events reach it while no backend is
  attached, the system shall pause that instance's event production rather than
  discard an event, and shall resume when a backend attaches. This shall hold for
  every class of agent event the instance produces, including error events; an event
  class that is discarded on a full buffer does not satisfy this.
- **AC-EXECUTORS-SURVIVAL-001.8:** The retention limit of
  AC-EXECUTORS-SURVIVAL-001.6 shall be validated when the control server starts, and
  shall be accepted only between one and ten thousand events inclusive. A configured
  value outside that range, or one that is not a whole number, shall be rejected with
  the control server refusing to start rather than clamped, because the limit governs
  how much memory an unattended server may hold per instance and a silently adjusted
  value would let an operator believe a bound they do not have. The lower end exists
  because a zero-length retention is indistinguishable from the discarding behavior
  AC-EXECUTORS-SURVIVAL-001.6 forbids.
- **AC-EXECUTORS-SURVIVAL-001.9:** The pause required by
  AC-EXECUTORS-SURVIVAL-001.6 shall be decided per instance, on whether that instance's
  event stream to an owning backend is established, and not on whether the control server
  as a whole has an owning backend. While an instance's stream is not established, an
  event class whose attached behavior is to expire after a fixed wait and act on the
  agent's behalf shall not expire: specifically, a permission request the agent raises
  shall remain outstanding, and shall not be auto-cancelled by the elapse of that wait,
  until a backend attaches to that instance or the instance stops. Deciding this at the
  server granularity instead would auto-cancel through the window between adopting a
  server and establishing an instance's streams, which is a window in which nothing can
  answer the request.
- **AC-EXECUTORS-SURVIVAL-001.10:** When an instance is stopped while one or more of its
  event producers are paused under AC-EXECUTORS-SURVIVAL-001.6, the system shall release
  every paused producer for that instance as part of the stop, without waiting for a
  backend to attach, and the stop shall complete rather than block. Events retained but
  not yet delivered at that moment shall be discarded with the instance; that discard is
  not the discard AC-EXECUTORS-SURVIVAL-001.6 forbids, which governs an instance being
  kept alive for a backend that has not arrived, not one being destroyed. This shall hold
  for every stop this feature issues, including the duplicate-loser stop of
  AC-EXECUTORS-SURVIVAL-002.10, the deadline stop of AC-EXECUTORS-SURVIVAL-003.7, and the
  unowned shutdown of AC-EXECUTORS-CONTROL-OWNERSHIP-003.1, which by construction runs
  when no backend is attached and none will attach. Without this a paused producer makes
  the instance unstoppable, so the unowned shutdown could not terminate the instances it
  exists to reap.
- **AC-EXECUTORS-SURVIVAL-001.7:** On every path in this document,
  including refused adoption and unowned shutdown, the system shall leave every
  worktree present on disk and unmodified.

### REQ-EXECUTORS-SURVIVAL-002: Re-tracking reconstructs a usable execution

**Intent:** Re-tracking is worth nothing unless the execution supports what a user
does next: the next prompt, a cancel, the transcript, and workspace file and git
operations. Partial reconstruction is the failure mode most likely to look like
success.

**User story:** As a Kandev user, I want an adopted session to behave like one that
was never interrupted, so I do not find a missing capability only when my next prompt
fails.

#### Acceptance criteria

- **AC-EXECUTORS-SURVIVAL-002.1:** When the system enumerates surviving
  instances, each enumerated instance shall report the session identity the
  control server itself holds for it, and the system shall correlate instances
  to recovery-inventory records by that identity.
- **AC-EXECUTORS-SURVIVAL-002.2:** When exactly one live instance reports a given
  session identity, the system shall re-track that instance, whether or not its
  instance identifier equals the record's agent execution identifier.
- **AC-EXECUTORS-SURVIVAL-002.10:** When more than one live instance reports the
  same session identity, the system shall re-track only the instance whose instance
  identifier equals that session's recovery-inventory record's agent execution
  identifier. When none or more than one matches, it shall re-track none of them and
  shall stop each of them. When exactly one matches, the system shall stop every
  other live instance reporting that session identity.
- **AC-EXECUTORS-SURVIVAL-002.3:** When an execution is re-tracked, the
  system shall restore the agent identity, the profile identities, the agent and
  continuation commands and their arguments, the provider session identity, the task
  environment identity, the run identity, the history setting, the runtime
  environment, and the workspace source roots it held before the restart. For a value
  whose declared source under AC-EXECUTORS-SURVIVAL-002.14 is a re-derivation, "held
  before the restart" shall instead mean re-derived from the agent profile and agent-type
  registry as they stand at recovery, which may differ from the value in force before the
  restart when the profile or registry changed during the outage. That difference is
  intended: those values are computed the same way on an ordinary launch, and restoring a
  historical command would replay a previous build's agent invocation after an upgrade,
  which is the stale input AC-EXECUTORS-SURVIVAL-002.14 forbids reading from the instance
  for the same reason.
- **AC-EXECUTORS-SURVIVAL-002.14:** Every value required by
  AC-EXECUTORS-SURVIVAL-002.3 shall have exactly one declared source, and that source
  shall be one of: the recovery-inventory record, another durable record this backend
  already owns, the adopted instance, or a deterministic re-derivation from a value
  already restored from one of those three. A
  value whose declared source is a re-derivation shall not be read from the adopted
  instance, so that an instance cannot influence what the backend believes about a
  profile, and no source outside these four shall be introduced without amending this
  criterion. A value that is legitimately empty for the launch that created the
  execution, rather than lost, shall be restored as empty and shall not be treated as
  missing under AC-EXECUTORS-SURVIVAL-002.4.
- **AC-EXECUTORS-SURVIVAL-002.4:** When a value required by
  AC-EXECUTORS-SURVIVAL-002.3 is authoritatively absent, meaning its declared source
  answered and the value is not present, the system shall refuse to
  re-track that instance, shall record which value was missing, and shall
  leave the instance to the stop path defined by
  AC-EXECUTORS-SURVIVAL-002.6.
- **AC-EXECUTORS-SURVIVAL-002.13:** When a read of a value required by
  AC-EXECUTORS-SURVIVAL-002.3 fails without answering, the system shall retry that
  read within a bounded per-read timeout and a bounded retry count, both
  configurable, defaulting to two seconds and two retries with no delay between
  attempts. The per-read timeout shall be accepted only at one hundred milliseconds or
  greater, and the retry count only as a whole number between zero and ten inclusive; a
  value outside either range shall be rejected at startup rather than clamped, on the same
  terms as the product ceiling below. Those floors exist because a degenerate value passes
  the product ceiling most easily: a near-zero timeout fails every reconstruction read
  instantly, re-tracking nothing while reporting the capability enabled. The product of the
  timeout and the total attempt count, which is the
  retry count plus one, shall not exceed the lesser of six seconds and one fifth of the
  configured bound of AC-EXECUTORS-SURVIVAL-003.7; that product shall be validated at
  startup against the bound actually configured, not against its default, and a
  configuration exceeding it shall be rejected rather than clamped, so no single
  value's reads can starve every other record however the bound is set. Exhausting either shall be recorded as a failed read
  rather than a missing value, and shall place that instance on the not-re-tracked
  path of AC-EXECUTORS-SURVIVAL-003.7 rather than the missing-input path of
  AC-EXECUTORS-SURVIVAL-002.4.
- **AC-EXECUTORS-SURVIVAL-002.5:** When an execution is re-tracked, the
  system shall not widen the set of filesystem paths that instance's file
  operations may reach beyond the set in force before the restart, including when
  the backend's own copy of that set could not be restored.
- **AC-EXECUTORS-SURVIVAL-002.6:** When a live instance has no
  corresponding recovery-inventory record, the system shall stop that instance,
  because it has no session identity to attach it to.
- **AC-EXECUTORS-SURVIVAL-002.15:** When a stop this document or
  [survived session state](agent-survival-session-state.md) requires does not
  succeed, meaning the control server returned an error or did not answer, the system
  shall retry it within the same per-read timeout and retry count as
  AC-EXECUTORS-SURVIVAL-002.13, and a control server reporting that the instance is
  already absent shall count as success. When the retries are exhausted the system
  shall not publish any session as re-tracked whose correctness depended on that stop
  having happened: specifically, when a losing duplicate under
  AC-EXECUTORS-SURVIVAL-002.10 cannot be stopped, the winning instance for that
  session shall not be re-tracked either, and both shall be left not re-tracked. A
  failed stop shall be recorded with the instance identity and the reason, and shall
  leave the affected records to the existing stale-execution repair path. The winning
  instance for that session shall itself be stopped on these same terms, so that a session
  nobody is tracking is not left with a live agent; when that stop also fails, both are
  recorded and AC-EXECUTORS-SURVIVAL-002.16 governs the session. An instance
  whose stop has already exhausted its retries shall not be stopped again by the
  deadline path of AC-EXECUTORS-SURVIVAL-003.7; it is already recorded and already left
  to repair. Publishing a
  session while a second live agent for it may still be running is the one outcome
  this criterion exists to prevent, and it is worse than a cold resume.
- **AC-EXECUTORS-SURVIVAL-002.7:** When a recovery-inventory record has
  no corresponding live instance, the system shall leave that record to the
  existing stale-execution repair path, which preserves its resume token and
  worktree identity.
- **AC-EXECUTORS-SURVIVAL-002.8:** The system shall read the set of session
  identities named by live standalone recovery-inventory records, excluding those
  [AC-EXECUTORS-SURVIVAL-005.3](agent-survival-session-state.md) excludes, before
  contacting any control server, and shall take a guard for each of them at that point, so no
  session that might be re-tracked is unguarded while adoption, enumeration and
  correlation run. Taking a guard shall be one atomic acquire-or-observe against the
  in-memory execution store: a session already guarded or already tracked is
  observed, not guarded twice, and the caller that observes shall not begin
  re-tracking it. A guard shall be released when that session's outcome is published,
  and every guard still held when the bound of AC-EXECUTORS-SURVIVAL-003.7 elapses
  shall be released then, so no guard outlives recovery, except for a session held under
  AC-EXECUTORS-SURVIVAL-002.16 or one whose stop is still in flight under
  AC-EXECUTORS-SURVIVAL-003.7. While a guard is held, a
  request to start an execution for that session shall be refused with a distinct
  retryable outcome naming recovery as the reason, rather than queued or blocked, so
  a concurrent user action can neither produce two agents for one session nor be held
  open for the duration of recovery. Guards are scoped to one backend process
  lifetime and need no durable representation.
- **AC-EXECUTORS-SURVIVAL-002.16:** When a session reaches a not-re-tracked outcome and
  at least one live instance reporting that session identity could not be stopped, the
  system shall retain that session's guard for the remainder of this backend's lifetime
  rather than releasing it, shall record the session identity together with the instance
  identities left running, and shall refuse a request to start an execution for that
  session with a distinct outcome naming an unstopped agent as the reason. That refusal is
  reported as not retryable within this backend's lifetime, unlike the retryable recovery
  refusal of AC-EXECUTORS-SURVIVAL-002.8, because nothing in this backend's remaining
  lifetime will stop that instance. A backend restart clears the condition, since guards
  do not survive one and the next start re-evaluates the instance. This is the only case
  in which a guard outlives recovery: releasing it would let a user launch add a second
  agent beside one this backend knows it failed to stop, the outcome
  AC-EXECUTORS-SURVIVAL-002.15 exists to prevent.
- **AC-EXECUTORS-SURVIVAL-002.9:** When re-tracking runs more than once against
  the same control server without an intervening restart, the system shall produce
  the same set of tracked executions and create no duplicate execution for an
  already-tracked session.
- **AC-EXECUTORS-SURVIVAL-002.11:** When re-tracking one instance is refused or
  fails, the system shall still re-track every other instance for which it succeeds.
  No order is defined between instances of different sessions, and no such instance's
  outcome shall depend on another's. Instances reporting the same session identity
  are the single exception: their outcomes are decided jointly by
  AC-EXECUTORS-SURVIVAL-002.10 and AC-EXECUTORS-SURVIVAL-002.15.
- **AC-EXECUTORS-SURVIVAL-002.12:** When the adopted control server cannot be
  enumerated, the system shall report no recovered instances, shall leave every
  recovery-inventory record to the existing repair path, shall stop no instance,
  and shall not start a second control server.

## Related design

- [Part 1: lifetime and ownership contracts](../system-design/agent-survival-across-restart-01.md)
- [Part 2: control flow, failure and scope](../system-design/agent-survival-across-restart-02.md)
- [Part 3: recovery data contracts](../system-design/agent-survival-across-restart-03.md)

## Out of scope

- Survival for the Docker, SSH, Sprites, Kubernetes and remote-Docker executors.
  They adopt this contract in later work; this one covers worktree and local.
- Post-restart session and task state, the turn that ended while unattached, and
  capability gating, all owned by
  [survived session state and capability gating](agent-survival-session-state.md).
- Which control server a backend may adopt, and refusal across an incompatible
  protocol change, owned by
  [standalone control-server ownership](standalone-control-server-ownership.md);
  which backend drives it and what happens when nobody owns one, owned by
  [standalone control-server single driver and unowned lifetime](standalone-control-server-single-driver.md).
- Survival of passthrough sessions and of user-owned terminals, excluded by
  [survived session state](agent-survival-session-state.md).
- Provider-native resume semantics and resume-token clearing, owned by
  [agent resume and runtime recovery](../../agents/requirements/agent-resume-runtime-recovery.md).
- Surviving a host reboot, or a host-level termination of the control server. This
  contract covers backend process lifetime only.
