---
status: draft
system: executors
requirements:
  - REQ-EXECUTORS-SURVIVAL-001
  - REQ-EXECUTORS-SURVIVAL-002
  - REQ-EXECUTORS-SURVIVAL-003
  - REQ-EXECUTORS-SURVIVAL-004
  - REQ-EXECUTORS-SURVIVAL-005
  - REQ-EXECUTORS-CONTROL-OWNERSHIP-001
  - REQ-EXECUTORS-CONTROL-OWNERSHIP-002
  - REQ-EXECUTORS-CONTROL-OWNERSHIP-003
  - REQ-EXECUTORS-CONTROL-OWNERSHIP-004
---
# Agent Survival Across Backend Restart System Design Part 1


## Purpose and boundaries

This design gives the standalone runtime a detached control-server lifetime, an
ownership protocol, a recovery producer, and truthful post-restart state, so
that worktree and local agent work survives a backend restart or upgrade.

One design covers all four requirement documents because they are implemented by
one mechanism: the ownership protocol exists to make re-tracking safe, and
splitting the design along the requirement seams would duplicate the same control
flow four times. The requirement documents are
[agent survival](../requirements/agent-survival-across-restart.md) and
[survived session state](../requirements/agent-survival-session-state.md) for the
sessions, and
[control-server ownership](../requirements/standalone-control-server-ownership.md)
and
[single driver and unowned lifetime](../requirements/standalone-control-server-single-driver.md)
for the server.

It owns process lifetime, adoption, the recovery producer, the instance
enumeration contract, and the ordering between recovery and startup state
reconciliation. It is written in three parts, listed under
[Requirement mapping](#requirement-mapping); this is part 1, which also carries the
prior-art record for the whole design. It uses but does not own: provider resume semantics (agent
system), the durable executor-record write path (already owned by the lifecycle
manager), runtime-state ownership locking and port probing (owned by
[port collision and backend ownership safety](../requirements/port-collision-safety.md)),
the runtime flag registry, and the secret store.

`ExecutorTypeWorktree` and `ExecutorTypeLocal` both resolve to the standalone
runtime and differ only in workspace preparer, so one producer and one lifetime
model cover both. Standalone runs one control server supervising many instances,
so exactly one process must survive per installation, not one per session.

## Requirement mapping

All three parts of this design are listed, because several requirements are
satisfied by sections in more than one. Part 1 holds process lifetime and the
ownership contracts; part 2 holds the control flow, failure behavior and scope;
part 3 holds the recovery data contracts.

| Requirement | Design section |
| --- | --- |
| `REQ-EXECUTORS-SURVIVAL-001` | [Kill paths that must change together](#kill-paths-that-must-change-together), [Event retention is not inherited](agent-survival-across-restart-03.md#event-retention-is-not-inherited), [Shutdown](agent-survival-across-restart-02.md#shutdown) |
| `REQ-EXECUTORS-SURVIVAL-002` | [Instance enumeration](agent-survival-across-restart-03.md#instance-enumeration), [Reconstruction inputs](agent-survival-across-restart-03.md#reconstruction-inputs), [Correlation and refusal](agent-survival-across-restart-02.md#correlation-and-refusal), [Recovery concurrency](agent-survival-across-restart-02.md#recovery-concurrency) |
| `REQ-EXECUTORS-SURVIVAL-003` | [Startup](agent-survival-across-restart-02.md#startup), [Persistence](agent-survival-across-restart-02.md#persistence) |
| `REQ-EXECUTORS-SURVIVAL-004` | [Turn outcome across the detached gap](agent-survival-across-restart-03.md#turn-outcome-across-the-detached-gap) |
| `REQ-EXECUTORS-SURVIVAL-005` | [Capability gating and scope](agent-survival-across-restart-02.md#capability-gating-and-scope) |
| `REQ-EXECUTORS-CONTROL-OWNERSHIP-001` | [Ownership identity and credential](#ownership-identity-and-credential) |
| `REQ-EXECUTORS-CONTROL-OWNERSHIP-002` | [Single driver](#single-driver) |
| `REQ-EXECUTORS-CONTROL-OWNERSHIP-003` | [Unowned shutdown](#unowned-shutdown) |
| `REQ-EXECUTORS-CONTROL-OWNERSHIP-004` | [Capability compatibility](#capability-compatibility) |

## Components and responsibilities

| Component | Responsibility |
| --- | --- |
| agentctl control server (`cmd/agentctl`, `internal/agentctl/server`) | Owns instance lifetime. Gains an identity and capability endpoint, a credential rotation operation, an unowned-ownership timer, retained per-turn terminal state, and session identity on enumerated instances. |
| agentctl launcher (`internal/agent/runtime/agentctl/launcher`) | Gains an adopt-before-spawn path, the installation-scoped control-server record, a survivable-detach stop mode, and the detached output sink's path, handed to the child
that opens it. Stops installing parent-death binding when detaching. |
| `StandaloneExecutor` (`internal/agent/runtime/lifecycle`) | Implements the recovery producer against the adopted control server and the recovery-record set handed to it. |
| `lifecycle.Manager` | Decides survivable stop versus terminating stop, owns the recovery-record read port, execution reconstruction, and the re-tracked-session set published to reconciliation. |
| Orchestrator startup reconciliation (`internal/orchestrator`) | Consumes the re-tracked-session set and skips state repair for those sessions. |
| `backendapp` startup and cleanup | Sequences ownership, adoption, recovery, and reconciliation; stops unconditionally terminating the control server through the registered cleanup. |
| Runtime flag registry (`internal/runtimeflags`) | Gates the whole capability. |

## Data and contracts

### Kill paths that must change together

Detaching is not one change. Each of these terminates the control server or its
instances independently, and leaving any one in place produces a capability that
silently does not work:

1. **Parent-liveness pipe.** The launcher passes a pipe read-end as child FD 3
   and sets `KANDEV_PARENT_PIPE_FD`; agentctl blocks on it and exits when the
   write end closes, including on `SIGKILL`.
2. **`Pdeathsig`.** On Linux the child is started with `SIGTERM` on parent death.
3. **Windows job object.** The child is bound to a kill-on-job-close job object.
   This was added deliberately to stop a crashed parent leaking an agentctl that
   holds the control port, so it is not simply removed; see
   [Platform scope](agent-survival-across-restart-02.md#platform-scope).
4. **Manager stop path.** Backend shutdown calls `StopAllAgents`, which reaches
   `StopAgentWithReason`, which stops the agent through the instance's agentctl
   client *before* the executor's `StopInstance`. A detach decision therefore has
   to be made in the manager; changing the executor alone cannot prevent it.
5. **Registered cleanup.** The backend registers the launcher's stop function as
   a cleanup, which closes the pipe and sends `SIGTERM`.
6. **Inherited standard output.** The launcher takes agentctl's stdout and stderr
   as pipes and forwards them into the backend log. When the backend exits, those
   read ends close and agentctl's next write to file descriptor 1 or 2 raises
   `SIGPIPE`, whose default disposition terminates the process. A detached server
   must be given an output sink that outlives the backend, and it must be one
   agentctl itself can bound. A descriptor the launcher opens and the child inherits
   cannot be: nothing rotates it once the launcher's process is gone, which is the
   whole point of detaching, so `AC-EXECUTORS-SURVIVAL-001.3`'s size and retention
   limits would be stated with no enforcer. agentctl therefore opens the sink itself.

   **It does not reuse the backend's own logger, and naming which mechanism it does use
   is the point of this paragraph.** The backend does not build its logger from the
   shared logging configuration at all. It uses a separate backend-specific logger whose
   rotation is a day-segmented policy, with a per-segment size, a total-size ceiling and
   a retained-days rule fixed as internal constants rather than exposed as settings, and
   whose active file name is likewise a constant, accompanied by a day marker and a
   rollover journal beside it. There is nothing per-caller in it to point a second
   process at: doing so would put two processes on one active file and one set of
   rollover journals, each rotating underneath the other. Nor is there an operator
   setting for its limits to inherit — the startup configuration catalog carries a log
   level and a log format, and no size or retention key at all.

   What agentctl uses instead is the shared logger package's ordinary rotating-file
   configuration, which does carry an output path together with a maximum size, a
   backup count and a maximum age. agentctl already constructs exactly that
   configuration today and passes `stdout` as its output path, so the change is that
   path plus those three bounds, not new rotation machinery. The bounds are fixed values
   of this capability, as `AC-EXECUTORS-SURVIVAL-001.3` requires: they are not operator
   settings, so they add no catalog key and no environment variable, exactly as the path
   does not. All three must be set to non-zero values: in that configuration a zero
   backup count and a zero maximum age each mean unlimited, so leaving either at its
   default reproduces the unbounded sink this paragraph exists to prevent. The path is computed by the launcher from the resolved Kandev home, names a
   file distinct from the one the backend writes its own logs to, and is handed to the
   child as an ordinary start-up argument. So neither the path nor the bounds joins the
   five tunables in
   [Configuration surface](agent-survival-across-restart-03.md#configuration-surface-for-the-numeric-tunables).
   The location is named in the control-server record so that both the next backend and
   an operator can find it. An unattended server writing without a bound is its own
   outage.
7. **Per-instance idle reaper.** agentctl reaps an instance with no in-flight
   request and no recent activity on its port (default one hour, scanned each
   minute). An attached backend holds workspace and event streams open, so
   in-flight requests are non-zero; a detached instance is idle by construction
   and is reaped once the timeout elapses.

Paths 1, 2, 4, 5 and 6 are disabled when the capability is enabled. Path 3
governs platform scope. Path 7 is left in place and is the reason the unowned
period must be shorter than the idle timeout — while idle reaping is enabled.
Setting the idle timeout to zero disables the reaper entirely, which removes path 7
rather than shortening it, and the ordering constraint has nothing left to order; see
[Unowned shutdown](#unowned-shutdown).

### Ownership identity and credential

Installation identity is the resolved Kandev home directory, which the backend
already holds an exclusive advisory lock over before it initializes anything.
That lock is the existing proof that exactly one backend acts on one
installation. Adoption therefore does not invent an installation identifier: the
control server records the home directory it was started for, and adoption
requires it to match the adopting backend's own.

The auth token continues to be minted by agentctl and delivered through the
one-shot bootstrap nonce, unchanged. What changes is that the backend stores the
token it receives in the secret store and keeps only a secret reference in the
control-server record described below, so a restarted backend can present it. Storing the token
itself in the record would put a bearer credential for a service that executes
commands in a user's worktree into the database in clear.

The control endpoint becomes durable too. Today the control port lives only in
process configuration, and the launcher relocates to a free port when the
configured one is busy, so a restarted backend has no way to find a relocated
survivor. Recording the endpoint is what makes
`AC-EXECUTORS-CONTROL-OWNERSHIP-001.1` implementable.

**That record is installation-scoped and separate from the recovery inventory.**
The obvious place to put it is the per-session executor record, and that is wrong
in two ways. `executors_running.session_id` is `UNIQUE`, so one row per session
means N copies of one server's endpoint, identity, credential reference and
capability set, with nothing saying which is authoritative and nothing keeping
them equal after a credential rotation writes some and not others. Worse, when no
session is live there is no row at all, so a detached server with no instances
becomes unlocatable: the next backend would relocate to a free port and leave the
survivor orphaned until its unowned period expired. The control-server record is
therefore a single installation-scoped record, written when a server is started or
adopted, read before anything else at startup, and independent of whether any
session exists. The per-session rows keep the agentctl URL and port they already
carry, which remain per-instance addressing, not the locator for the server.

### Why not the launcher's health token

The launcher already proves process ownership over an HTTP endpoint: an
[owned backend](../../launcher/requirements/startup-recovery.md) is one whose
health response carries the launcher's per-launch health token, and a missing or
different token identifies a different process on the port
(`AC-LAUNCHER-STARTUP-001.3`, `AC-LAUNCHER-STARTUP-002.4`). It is the closest
existing primitive, so not reusing it needs an argument.

It proves a strictly weaker thing. The launcher token answers "is this the process
I spawned during *this* launch?", and can live in memory precisely because a launch
never outlives the launcher. Adoption asks "is this a process a *previous* launch
of this installation spawned?", across a process boundary: that needs durable
custody of the credential, presentation authentication rather than comparison of an
echoed value, replacement to fence out a prior holder, and revocation of streams
opened under the old credential. The launcher requirement defines none of these,
and `AC-LAUNCHER-STARTUP-002.5` forbids printing the token while saying nothing
about storing it.

What does transfer is the shape: an opaque per-launch value echoed on an identity
endpoint, compared by the prober, never logged. The adoption identity endpoint
follows that shape deliberately and adds the durable credential the launcher
primitive lacks. This is not a second competing scheme; it is the same idea with
the custody and revocation an across-restart handover requires.

### Single driver

Adoption rotates the credential: the adopting backend presents the stored token,
then immediately replaces it, and agentctl rejects the superseded token and
closes every stream authenticated with it.

Rotation is two-phase, because a single-phase rotation has a crash window that
locks a backend out of a server it owns. If agentctl switched on receipt and the
backend then failed before writing the new token to the secret store, the stored
credential would authenticate nothing and the server would sit unadoptable until
its unowned period expired, still holding live agents. So agentctl accepts both
the superseded and the replacement credential until the backend confirms it has
durably stored the replacement, and only then stops accepting the old one. An
interruption at any point leaves at least one credential both sides hold. The
window is bounded by the unowned period, which is the same bound that already
governs a server whose owner never returns. Inside it the superseded credential
buys only the right to attempt adoption again: it cannot drive an instance and
cannot open a stream, so the two-phase window does not quietly reintroduce the
second driver that rotation exists to eliminate.

Two phases alone are not enough, because they leave the *second* rotation
undefined. A backend can crash after rotating and before confirming, and the next
backend then adopts using the credential the first rotation superseded — which the
window deliberately still accepts. Read naively, that second rotation supersedes only
what it was presented with, leaving the first rotation's unconfirmed replacement
valid and in nobody's custody: a full-power credential that is exactly the second
driver the whole section exists to remove. So each rotation carries an identifier the
control server assigns from a strictly increasing sequence and returns with the
replacement, the confirmation names the rotation it confirms, and the acceptable set
is capped at two: the replacement issued by the highest-numbered rotation, for
everything, and the one credential that rotation directly superseded, for a further
adoption attempt only. Every earlier credential, confirmed or not, stops
authenticating. A confirmation naming a superseded rotation is accepted and does
nothing, so a delayed or duplicated confirmation cannot revoke a credential issued
after it.

Rotation also has to survive losing its own response, which is a different failure from
losing the confirmation and is not covered by the two-phase window. A backend that never
receives the replacement still holds the superseded credential and will retry; if each
retry rotated again, the second one would push the credential it holds out of the
acceptable set and lock it out of a server it owns — the very outcome the window exists to
prevent, reached from the other side. So a rotation is idempotent under retry: presenting
the credential that the highest-numbered rotation superseded, while that rotation is still
unconfirmed, returns THAT rotation again rather than allocating a new one. Only a
credential that a rotation issued buys a further rotation. Retrying is therefore
unbounded-safe, and the acceptable set cannot advance past a backend that is still trying
to catch up with it.

One operation is deliberately outside the rotation rules: the ownership shutdown,
which stops every instance and exits. It accepts any credential in the acceptable
set, including the superseded one, and needs no adoption. Without that carve-out the
disabled-capability cleanup is unsatisfiable in exactly the case the two-phase window
creates: a backend that crashed between rotating and confirming holds only the
superseded credential, and with the capability now off it may not adopt, so it could
neither authenticate a stop nor obtain a credential that would. The server would then
survive until its unowned period despite a backend standing right there able to prove
it owns it. Admitting the superseded credential here is safe because the operation
only destroys. It cannot drive an instance, cannot read a transcript, and does not
outlive the call, so it cannot become the second driver the rest of this section
exists to prevent.

**A separate monotonic fencing epoch is deliberately not introduced.** The
scenario an epoch would guard, two backends concurrently driving one control
server, is already prevented one level up: a second backend on the same home
cannot acquire the runtime-state lock and exits before it launches or adopts
anything, and the lock is released only after all agent teardown has run, so
there is no handover overlap. In the one case where that invariant could fail,
both backends would allocate epochs from the same database and alternate,
producing exactly the behavior rotation produces. An epoch would add a schema
column, an allocation path, and a rejection status without closing a case that
rotation leaves open. Closing the streams is the part that actually matters and
that a request-level epoch check would have missed, because a long-lived stream
authenticates once.

### Unowned shutdown

Ownership is a claim the owning backend keeps current while attached. When no
claim has been current for the unowned period, the control server stops every
instance and exits.

The claim is a single explicit operation that both establishes and renews it, and only
two other operations renew it: the bootstrap handshake and a successful credential
rotation. That is a decision, not an implementation detail, because the
obvious alternative is to treat any authenticated traffic as evidence of an owner and
it is wrong in both directions. A backend attached to an idle session sends no traffic
for minutes at a time and would be reaped as absent; a backend mid-crash can still
have an open stream and would be read as present. Tying the claim to one operation
whose only purpose is to make it makes the signal mean exactly what the timer needs.
It carries no instance identity, so a server with zero instances is still owned, which
is the case that made the control-server record installation-scoped in the first
place.

That leaves the two moments an explicit claim cannot cover, and both are carve-outs
rather than exceptions to the principle: each is a single operation whose success proves
a specific backend is there, which is exactly what the timer needs.

The first is a freshly spawned server, which has no adoption to count as one. The
bootstrap handshake that mints the credential is that first
renewal: it already happens, it already proves both sides, and it gives the timer a
definite start. A server spawned by a backend that then dies before completing the
handshake has no successful renewal at all, so its period runs from process start and
it reaps itself without special handling.

The second is adoption. Rotation is the first authenticated operation an adopting backend
issues, and it must set the renewal instant, because otherwise the adopting backend
inherits whatever is left of the DEAD owner's period. A server whose previous backend died
just under the period ago would then begin its unowned shutdown seconds into recovery and
stop the instances being re-tracked underneath it — publishing sessions as their agents
die, which is the failure the one-way-door ordering below exists to prevent, arriving by a
route that ordering does not cover because no claim was ever made. The adopting backend
then renews on the ordinary interval from the moment its rotation succeeds, rather than
waiting for re-tracking to finish; re-tracking is bounded at thirty seconds by default but
configurable to five minutes, which is well inside a ten-minute period only if renewal has
already started.

This is deliberately not a second timer layered on the existing per-instance idle
reaper, which already terminates instances nothing is attached to after an hour.
The reaper measures the same underlying condition, but it stops instances without
exiting the server and its period is far longer than a restart needs. The unowned
period is therefore required to be shorter than the idle timeout, so the unowned
shutdown always fires first and the reaper's behavior is unchanged in practice. The
default is ten minutes: generous against a restart or upgrade, which take seconds,
and bounded against unattended token spend.

Renewal happens at no more than a third of the unowned period, so two consecutive
failed renewals cannot expire a server whose owner is alive, and a successful
adoption counts as a renewal so an adopting backend never races the timer it just
inherited. The claim is judged by time since the last successful renewal on the
control server's own monotonic clock, never on a wall-clock value either side
supplies, because a clock adjustment during a restart would otherwise expire a live
server. A renewal failure that does not establish the owner is gone does not
shorten the period: the claim is judged by its age, not by the last attempt.

The unowned period and the idle timeout are configured independently, so their
ordering is validated at control-server start rather than assumed. A configured
unowned period that is not shorter than the idle timeout is reduced to half the
idle timeout, and the adjustment recorded. Clamping rather than refusing to start,
because the failure this prevents is cosmetic (the reaper firing first) while
refusing to start would deny the operator every agent over a misconfigured timeout.

**The idle timeout has a sentinel, and the clamp must not treat it as a duration.**
Setting the idle timeout to zero is a supported, validated configuration meaning
*disable idle reaping entirely* — the reaper goroutine is simply not started. Compared
numerically, zero is smaller than any unowned period, so a naive clamp fires on a
perfectly good configuration and reduces the unowned period to half of zero. The
control server would then consider itself unowned from the moment it started, stop
every instance it had just been asked to keep alive, and exit — announced only as a
recorded "adjustment". So the ordering constraint is conditional on idle reaping being
enabled: with the reaper disabled there is nothing for the unowned shutdown to
precede, the constraint is vacuously satisfied, and the configured period stands
unchanged. A floor of one minute applies after any validation or adjustment, so that
no path — sentinel, arithmetic, or a small explicit value — can produce a period at or
near zero, which would expire ownership before an adopting backend could claim it.
The floor is applied last and wins: with an idle timeout short enough that no value
satisfies both the floor and the ordering, the server takes the floor, records that
the ordering could not be honoured, and starts. That trade is the right way round,
because the cost of losing the ordering is the reaper firing first — the cosmetic
failure the clamp already accepts — while the cost of honouring it is the near-zero
period the floor exists to forbid.

Renewal is at an interval STRICTLY shorter than a third of the period, not merely "no
greater than" it. At exactly a third the arithmetic fails at the boundary: two failed
renewals leave the third attempt falling due at the same instant the claim reaches the
period's age, and whether the server survives then depends on an unstated tiebreak
between a renewal and an expiry test. Strict inequality puts the third attempt
strictly inside the period and removes the tie, so the stated guarantee holds without
anyone having to specify which of two simultaneous events wins.

**Beginning the shutdown is a one-way door.** The timer decides once, and from that
moment claims and adoptions are refused with a shutting-down answer rather than racing the
teardown; nothing cancels a shutdown already begun. The alternative — letting a late claim
win against a shutdown in progress — reads as generous and is the worse failure: an
adopting backend would enumerate instances that are being stopped underneath it and publish
their sessions as running just as their agents die. A backend that renews in time is never
affected, because a claim accepted strictly before the decision prevents it. A backend that
gets the shutting-down answer treats that server as one that is not there: report nothing
recovered, touch nothing, start its own.

**The record repair is the next backend's, not the control server's.** agentctl has
no access to the durable store, and by construction no backend is attached when an
unowned shutdown fires, so nothing can write the inventory at that moment. Records
are left claiming a live process until the next backend starts and repairs them
through the existing stale-execution path, which already preserves the resume token
and worktree identity. That path exists to correct exactly this condition.

### Capability compatibility

agentctl advertises a set of named capabilities on its identity endpoint. The
backend requires a set and adopts only when its required set is a subset. This is
a set comparison, not a version ordering: a version integer cannot express which
changes are compatible, so it would either refuse compatible adoptions or accept
incompatible ones.

Identity retrieval and the ownership shutdown sit BELOW that negotiation rather than
inside it: they are not members of the advertised set and are never compared against the
required one. That is forced, not stylistic. Identity retrieval is what decides
compatibility, so it cannot itself be gated on the answer; and the incompatible path's
whole remedy is to stop the server, which is unsatisfiable if the stop is one of the
capabilities that failed to match. Every control server this contract covers answers both,
whatever else it advertises.

On incompatibility the surviving server is stopped and a fresh one started, and
the affected sessions take exactly the path they take with the capability
disabled. A stop that does not succeed changes what may be written, not just what is
logged: the records are left alone rather than repaired, because a record repaired while
its process is still running is repaired by nothing afterwards. This backend starts its own
server on a free port and leaves the survivor to its own unowned shutdown, which nothing is
now renewing, and to the deferred repair that path already carries. **Drain and two-generation coexistence are deliberately deferred.**
Either would require every session to address its own control endpoint, two
credentials, and two port allocations, which is a second capability rather than a
variation of this one. The per-record control endpoint this design already
records is the piece a later drain would need, so deferring costs no rework.

## Prior art

**Our own wiki.** Searched the vault at `/Users/henry/Documents/henry/wiki`
(resolved from `~/.obsidian-wiki/config` to `config.henry`; collection `wiki`,
2,423 documents) through the QMD index, with three queries: process survival
across a parent restart with adoption, fencing and leases; Kandev backend restart,
daemon supervision and executor lifetime; and durable state versus in-memory
completion waiters. The vault holds no page on process survival, adoption,
fencing, or restart-resilient daemons: the same three unrelated pages returned for
every query at falling scores, which is the signature of no coverage rather than
of a missed phrasing. Two pages are transferably relevant.
`concepts/optimistic-vs-pessimistic-concurrency` argues from Agrawal, Carey and
Livny (1987) that under finite resources the right shape is a cheap staleness
check plus serialization at a single point, rather than optimistic retry: the
shape this design takes, with one exclusive owner and credential rotation as the
check, and no epoch allocation path.
`concepts/agent-replay-non-idempotence` argues that re-running an agent is not a
retry, because the second run lands a different result. That is the strongest
argument for this capability existing at all: a cold resume is a replay.

**What others shipped.** Searched the `saas-kb` corpus with the `ai_sdlc` category
filter, four queries covering session persistence across restart, sandbox and VM
lifetime, and control-plane restart with orphan reclamation.

Multica is the closest analogue: a daemon on the user's own computer running local
coding-agent tools. Its answer to a daemon restart is the opposite of this design.
Running tasks fail, "reclaimed after a daemon restart" is a named auto-retryable
failure reason with a two-attempt ceiling, and on restart the daemon re-registers
its runtimes and reclaims tasks that did not end cleanly. That is what Kandev does
today. Multica also heartbeats every fifteen seconds and marks a runtime offline
within about three minutes, which is the datapoint behind a ten-minute unowned
period rather than an hour: nobody in this category treats a multi-hour detachment
as normal.

OpenHands takes the other available approach, making the *state* durable rather
than the process: a conversation directory holding a base state file plus an
append-only event log, restored into a fresh process. It is a real alternative and
it is rejected here, because what must be preserved is a live provider session and
an in-flight turn, and reconstituting those from a log is the cold resume this
capability exists to avoid. Its enterprise sandbox model does have a `PAUSED` state
where the agent is paused but the sandbox keeps running, which is close to the
detached state here.

**What we do differently.** Neither product keeps the agent process itself alive
across a control-plane restart; both accept the loss and rebuild. This design keeps
the process and adopts it, which is only defensible because the control plane and
the agent share a host and an exclusive per-installation lock already decides
ownership. Treat both entries as vendor claims about behavior, not as evidence.
