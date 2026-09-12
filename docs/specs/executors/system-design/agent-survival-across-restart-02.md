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
# Agent Survival Across Backend Restart System Design Part 2

## Purpose and boundaries

Part 2 of the design begun in
[part 1](agent-survival-across-restart-01.md), which states the purpose,
boundaries and components this part assumes; the recovery data contracts are in
[part 3](agent-survival-across-restart-03.md). This part holds the control flow, the failure and recovery behavior, persistence, security,
capability scope and observability.

## Requirement mapping

All three parts of this design are listed, because several requirements are
satisfied by sections in more than one. Part 1 holds process lifetime and the
ownership contracts; part 2 holds the control flow, failure behavior and scope;
part 3 holds the recovery data contracts.

| Requirement | Design section |
| --- | --- |
| `REQ-EXECUTORS-SURVIVAL-001` | [Kill paths that must change together](agent-survival-across-restart-01.md#kill-paths-that-must-change-together), [Event retention is not inherited](agent-survival-across-restart-03.md#event-retention-is-not-inherited), [Shutdown](#shutdown) |
| `REQ-EXECUTORS-SURVIVAL-002` | [Instance enumeration](agent-survival-across-restart-03.md#instance-enumeration), [Reconstruction inputs](agent-survival-across-restart-03.md#reconstruction-inputs), [Correlation and refusal](#correlation-and-refusal), [Recovery concurrency](#recovery-concurrency) |
| `REQ-EXECUTORS-SURVIVAL-003` | [Startup](#startup), [Persistence](#persistence) |
| `REQ-EXECUTORS-SURVIVAL-004` | [Turn outcome across the detached gap](agent-survival-across-restart-03.md#turn-outcome-across-the-detached-gap) |
| `REQ-EXECUTORS-SURVIVAL-005` | [Capability gating and scope](#capability-gating-and-scope) |
| `REQ-EXECUTORS-CONTROL-OWNERSHIP-001` | [Ownership identity and credential](agent-survival-across-restart-01.md#ownership-identity-and-credential) |
| `REQ-EXECUTORS-CONTROL-OWNERSHIP-002` | [Single driver](agent-survival-across-restart-01.md#single-driver) |
| `REQ-EXECUTORS-CONTROL-OWNERSHIP-003` | [Unowned shutdown](agent-survival-across-restart-01.md#unowned-shutdown) |
| `REQ-EXECUTORS-CONTROL-OWNERSHIP-004` | [Capability compatibility](agent-survival-across-restart-01.md#capability-compatibility) |

## Control flow

### Startup

1. Acquire runtime-state ownership. Unchanged, and still the point at which a
   second backend on the same installation is refused.
2. Resolve the capability flag and platform support. When either is off, take
   today's path unchanged, and stop any detached server left by an earlier launch.
3. Read the live standalone recovery-inventory records and take a recovery guard
   for every session they name except the passthrough ones, which
   `AC-EXECUTORS-SURVIVAL-005.3` excludes because they can never be re-tracked,
   BEFORE any control server is contacted. Passthrough is decided here from the durable
   session record's own stored passthrough mode, which outlives a restart. It must NOT be
   decided from the lifecycle manager's in-memory execution state, which is what the
   existing passthrough test reads: that state is empty at this point in startup, so it
   would answer "not passthrough" for every session and the exclusion would silently
   apply to none of them — every passthrough session guarded, every one of those guards
   held until the startup deadline releases it. A record whose mode cannot be read is
   guarded rather than excluded, because an unguarded re-tracking candidate is the
   two-agent outcome the guard exists to prevent, while a wrongly guarded passthrough
   session costs one refused launch until the bound elapses. This is a
   local database read and needs no control server, so there is no reason to defer
   it — and deferring it is what leaves a hole: the backend's listener is already
   bound at this point, so between here and correlation a user launch could otherwise
   start a second agent for a session that is about to be re-tracked. See
   [Recovery concurrency](#recovery-concurrency).
4. Read the recorded control endpoint and identity. With no record, or nothing
   answering it, spawn a fresh server and report no recovered instances.
5. Fetch identity and capabilities, and evaluate the gates in this order: home
   match, then authentication, then capability subset. The order is not
   cosmetic. A refusal recorded as incompatibility authorizes stopping the
   process, so evaluating capabilities first would let a foreign server be
   stopped for running a different build. A server that fails identity or
   authentication, or advertises no identity at all, is left running untouched
   and this backend spawns its own on a free port, which is today's behavior.
   Only an own, authenticated, incompatible server is stopped.
6. On a match, rotate the credential and record the observed capability set. The
   rotation sets the control server's last-successful-renewal instant, and ownership
   renewal starts on its ordinary interval from here — NOT after step 7. Without this the
   adopted server is still counting down the dead owner's period while recovery runs.
7. Call the producer with the records read at step 3, and let the existing consumer
   re-track what it returns, releasing each session's guard as its outcome is
   published.
8. Publish the set of re-tracked sessions, then run orchestrator startup
   reconciliation.

Step 6 is part of adoption, not a follow-up: if credential replacement does not
succeed the backend has not adopted, reports nothing recovered, and issues no
further operation to that server.

Steps 4 through 7 are bounded by a single deadline covering adoption, enumeration and
the reconstruction of every record together, defaulting to thirty seconds and
configurable. **The clock starts at step 4**, the first contact with the recorded
control endpoint — not at step 7. Step 3 is deliberately outside the bound: it is a
local read that must happen for the guards to exist at all, and putting it inside
would let a slow database consume the budget for talking to the control server. One
global bound rather than a per-instance one, because a per-instance bound multiplies
by the number of survivors and is therefore not a bound on startup at all. When it
elapses, records without an outcome are treated as not re-tracked, their live
instances are stopped through the same path as an instance with no record so that
nothing is left running untracked, every guard still held is released, and step 8
proceeds. A re-tracking that finishes after the deadline must not publish its
session or touch task state: reconciliation has already run by then, and a late
write would undo it.

An adoption operation already in flight when the deadline elapses is allowed to
finish; it is not cancelled. Cancelling a credential rotation midway is the one
cancellation that does lasting harm: it leaves the control server holding a
replacement whose confirmation will never arrive, so it sits in the two-phase window
until the unowned period expires. Letting it finish costs nothing, because an adoption
that completes late re-tracks nothing anyway — the deadline has already decided every
record's outcome. This is a direct constraint from
[startup-listener-before-recovery](../../startup-listener-before-recovery/spec.md):
startup work that can block indefinitely is the failure this backend has already
shipped a fix for, and this capability must not reintroduce it.

Step 8 is the ordering `AC-EXECUTORS-SURVIVAL-003.1` requires. The lifecycle
manager is constructed and started well before the orchestrator starts today, so
the order already holds; what is missing is that reconciliation has no way to
know what recovery did, and unconditionally moves an active session to
waiting-for-input, abandons its open turns, moves its task to review, and stamps
it interrupted.

### Correlation and refusal

Instances and records are joined on the session identity agentctl reports. For a
session with more than one live instance, the tiebreak is the record's agent
execution identifier against the instance identifier; no match or an ambiguous
match re-tracks nothing and stops the candidates, because attributing a
transcript to the wrong instance is worse than a cold resume. When exactly one
candidate matches, the losers are stopped rather than left alone: they hold live
agent processes for a session that is now tracked through a different instance, so
leaving them running spends tokens against work nobody will read. Stopping them is
also what keeps the invariant the rest of this design relies on, that every live
instance is either tracked or stopped.

Re-tracking is idempotent: a session already present in the execution store is
left alone rather than added again, matching the existing duplicate handling in
the recovery consumer.

### Recovery concurrency

The backend binds its listener before startup recovery runs, which is deliberate
and already shipped. The consequence for this design is that a user request can
arrive for a session while that session is mid-re-tracking, so `AC-EXECUTORS-SURVIVAL-002.8`
is guarding a real race rather than a theoretical one.

The guard cannot be taken per-session at the moment that session's re-tracking
begins, which is the obvious design and the wrong one. A session's identity is not
known to be a re-tracking candidate until adoption, enumeration and correlation have
all run, and those are inside a thirty-second budget with the listener bound the whole
time. A launch arriving in that window would find nothing guarding it and nothing
tracking it, start a second agent, and then recovery's own idempotency rule would
leave the surviving instance alone as "already tracked" — producing precisely the two
agents for one session the guard exists to prevent, plus a durable row describing the
wrong process.

So the guards are taken from the RECORDS, not from the correlation result, at startup
step 3, before any control server is contacted. Every session named by a live
standalone recovery-inventory record is guarded up front; a session that turns out to
have no live instance simply has its guard released with a not-re-tracked outcome.
Guarding a few sessions that were never going to be re-tracked for a few seconds is
cheap; leaving a real candidate unguarded is not.

Acquisition is one atomic acquire-or-observe against the same in-memory execution
store the launch path already consults, so two recovery invocations cannot both claim
a session and neither can race a launch: whoever observes rather than acquires does
not re-track. A launch request for a guarded session is refused with a distinct
retryable outcome naming recovery, rather than queued behind it: queuing couples a
user request to the whole recovery phase and can outlast the caller's timeout, while a
refusal is immediate, observable, and safe to retry a moment later. Every guard is
released either when its session's outcome is published or when the startup deadline
elapses, whichever comes first, so no guard outlives recovery even if a re-tracking
goroutine is still in flight. The marker needs no durable form: it covers only the
window between recovery starting and reconciliation completing inside one backend run,
and a backend that dies inside that window takes the in-memory executions with it.

### Shutdown

Two stop intents replace today's single one. When the capability is enabled and
the reason is backend shutdown, the manager releases its stream subscriptions and
its ownership renewal and leaves the instance running. Every other stop reason
keeps today's terminating semantics, including user stops and rollback cleanup.
The registered cleanup performs a detach rather than a stop.

## Failure and recovery

| Situation | Behavior |
| --- | --- |
| Recorded endpoint answers, home mismatch | Refuse, never contact again, spawn own server on a free port. |
| Recorded endpoint answers, authentication fails | Refuse. Treated as foreign, because a wrong token cannot distinguish a foreign process from a corrupted own one. |
| Required capability missing | Refuse, stop the survivor and its instances, spawn fresh. Sessions take the capability-disabled path. |
| No recorded endpoint, or nothing answers | Spawn fresh, report no recovered instances. |
| Record with no live instance | Left to the existing stale-execution repair, which preserves resume token and worktree. |
| Instance with no record | Stopped. Without a record there is no session identity to attach it to. |
| A required reconstruction input missing | Refuse to re-track that instance, record which input, stop it. Other instances are unaffected. |
| Adopted server cannot be enumerated | Report nothing recovered, stop no instance, spawn no second server, leave every record to the existing repair path. The server is owned and reachable enough to have been adopted, so stopping instances we cannot see would destroy live work. |
| Credential replacement fails during adoption | Adoption is incomplete: report nothing recovered and issue no further operation until adoption is attempted again. |
| Re-tracking exceeds its bound | Treat the remaining records as not re-tracked, stop their live instances so none is left untracked, and let reconciliation proceed. A re-tracking finishing after the deadline publishes nothing. |
| A reconstruction read fails without answering | Retry within a bounded per-read timeout and retry count. Exhausting them is a failed read, not a missing value: the instance goes to the not-re-tracked path, not the missing-input path, so a network blip is never recorded as an incomplete reconstruction. |
| Backend crashes between rotating the credential and storing it | The superseded credential still authenticates, because agentctl drops it only on explicit confirmation. The next backend adopts normally. |
| Launch requested for a session mid-re-tracking | Refused with a retryable recovery reason, not queued. |
| A stop does not succeed | Retried within the same per-read timeout and retry count; an already-absent instance counts as success. On exhaustion no session is published whose correctness needed that stop, so a duplicate that cannot be stopped also blocks its winner from being re-tracked. Recorded with the instance identity and reason. |
| The turn-status read fails without answering | Retried on the same budget. On exhaustion the session is NOT published as running: the instance goes to the not-re-tracked path and is stopped, because a falsely-running session is repaired by nothing while a stopped one is repaired by the existing stale path. |
| The stored credential cannot be retrieved | Adoption refused with the credential-unavailable reason, distinct from authentication failure because no attempt was made. The server is left running and untouched, this backend spawns its own, and the survivor is reclaimed by its unowned shutdown. |
| Superseded credential rejected | Drop the affected executions, close streams, do not retry. |
| Unowned period elapses | Stop instances and exit. The records are repaired by the next backend to start, through the existing stale-execution path, with resume tokens and worktree identity preserved: agentctl cannot reach the durable store and no backend is attached at that moment. |
| Retained events reach the limit while detached | Event production pauses rather than dropping; it resumes on attach. |
| Adoption still in flight when the startup deadline elapses | Allowed to finish and its result recorded, never cancelled; it re-tracks nothing. Cancelling mid-rotation would strand the control server in the two-phase window with no confirmer. |
| Idle reaping configured off while survival is enabled | The unowned-period ordering constraint is vacuous and the configured period stands; it is never derived from the disabled timeout. |
| Rotation performed while an earlier rotation is unconfirmed | The earlier replacement stops authenticating. At most two credentials remain acceptable: the newest replacement, and the one credential it directly superseded, for adoption only. |
| Launch requested for a session before correlation has identified it | Still refused: guards are taken from the recovery records at startup step 3, before any control server is contacted. |
| Rotation response lost before the backend stores it | Retrying adoption with the superseded credential returns the SAME rotation again rather than allocating a new one, so retries cannot advance the acceptable set past the backend still catching up with it. |
| The stop of an incompatible server does not succeed | Retried on the same budget; already-stopping or already-absent counts as success. On exhaustion no record is repaired, nothing is reported recovered, and this backend starts its own server on a free port. The survivor is reclaimed by its own unowned shutdown. |
| A duplicate loser cannot be stopped | Its winner is stopped too, on the same budget; neither is re-tracked. If the winner's stop also fails, the session keeps its guard for this backend's lifetime and launches for it are refused as not retryable, so nothing adds a third agent beside two nobody can stop. |
| The startup deadline elapses mid stop-retry | The retry sequence finishes rather than being cancelled, like an adoption. The session is not re-tracked either way; finishing decides only whether a live agent is left running. |
| Own server started after a refused or failed adoption | The single control-server record is rewritten to name the NEW server. The old server's credential stays in the secret store, but the record cannot keep pointing at a server that is about to reap itself, or the server we just started would be the unlocatable one. |
| Claim or adoption arrives after an unowned shutdown has begun | Refused with a shutting-down outcome; nothing cancels a shutdown already begun. The backend treats that server as one that does not answer and spawns fresh. |
| Backend stops between retrieving a terminal turn outcome and applying it | The outcome is still retained, because only an acknowledgement discards it. The next backend retrieves and applies it; re-applying to an already-terminal turn changes nothing. |

The pause in the first of those rows is NOT inherited from the existing per-instance
update channel, and treating it as inherited is the mistake this design previously made.
Only one of that channel's producers actually blocks; the rest degrade, in two different
ways, and the error classes are among them. **The count and the per-site enumeration live
in exactly one place, part 3's**
[Event retention is not inherited](agent-survival-across-restart-03.md#event-retention-is-not-inherited),
**and must not be restated here** — this paragraph carried its own count through three
review rounds and was wrong each time, and the second copy is what let a corrected
enumeration sit beside an uncorrected one.

## Persistence

A new installation-scoped control-server record holds the control endpoint, the
control-server identity, the ownership-credential reference, the capability set
observed, and the diagnostic-output location. Exactly one exists per installation.
It is deliberately NOT the per-session executor record: see
[Ownership identity and credential](agent-survival-across-restart-01.md#ownership-identity-and-credential) for why
that placement fails when rows disagree and when no session is live. The
per-session rows are unchanged by this design. No fencing-epoch column is added,
per [Single driver](agent-survival-across-restart-01.md#single-driver).

Record repair on unowned shutdown reuses the existing dead-row repair, which
preserves resume token and worktree identity, rather than deletion. That
maintains the resume-safety invariant.

Standalone row liveness is currently judged by the local process identifier,
which for standalone is the shared control server. That is truthful today only
because the control server always dies with the backend. Once it survives, every
standalone record would report alive whether or not its own instance exists,
including instances the idle reaper stopped, and the repair path would stop
firing. Liveness for a standalone record is therefore judged by the presence of
that record's instance in the adopted server's enumeration, falling back to
unknown when the server cannot be reached, never to alive.

Two things about that check are easy to get wrong, so both are stated rather than left
to the implementation.

**It has two kinds of caller, and only one of them is a pass.** The existing prober is
a stateless, single-row, context-free call. It is reached from the startup
reconciliation sweep, which walks every standalone record once — that is a
reconciliation pass, and within it a single enumeration is taken and reused for every
row, rather than one call per record. It is ALSO reached from the single-session idle
reclaim path, which runs from ordinary event handlers and from a periodic tick that
has nothing to do with startup. Those callers are not a pass: each issues its own
enumeration and must never read one a pass cached, because they can fire at any moment
and a cached answer would age without bound. Threading the pass's enumeration to the
prober is therefore a scope the caller supplies, not a value the prober memoizes.

**A server this backend STARTED is not a server it adopted.** The enumeration that
decides presence is the adopted server's, and on every path where a server ANSWERED but
adoption did not complete — a refused or foreign server, an unretrievable credential, a
server already shutting down, an incompatible server whose stop did not succeed — this
backend starts a fresh one and the survivors keep running on the old. Enumerating the
fresh server there returns a determinate "not present" rather than a failure to reach
anything, so a classification built on it would call every surviving row dead and hand it
to a repair that prunes or rewrites a row whose agent is still alive. That is the outcome
[AC-EXECUTORS-CONTROL-OWNERSHIP-004.7](../requirements/standalone-control-server-ownership.md)
forbids the adoption path from producing, arriving instead through reconciliation. Those
rows therefore classify unknown. That unknown governs only records this backend
INHERITED — rows an earlier launch wrote. A record this backend itself created during this
process lifetime classifies against the enumeration of whichever control server this
backend is using, adopted or freshly started, because that server is where its instance
was created and is the only authority that can answer for it. Without that carve-out, a
backend that spawned a fresh server after a refused adoption would classify every session
it subsequently created as unknown for the rest of its lifetime, and the idle reclaim
path, which reads the same classification, would never reclaim any of them. The case
where NOTHING answered is the opposite and must not be folded into it: no server is holding instances at all, the rows are ordinarily
dead, and the classification stays the process-identifier probe so they are still
repaired. Treating both as unknown would strand every genuinely dead row unrepaired on
the most common path of all, a first start with no survivor.

**The snapshot must post-date recovery's own stops.** Recovery does not only observe
instances, it stops some of them: duplicate losers under the correlation tiebreak, and
every not-re-tracked record's instance when the startup deadline elapses. An
enumeration taken before those stops would show exactly those instances as present,
classify their rows alive, and skip the repair that is the only thing that would ever
correct them — leaving rows permanently claiming processes recovery itself killed. So
any enumeration a classification reads is taken after re-tracking has reached an
outcome for every record.

An outcome reached is not a stop completed, and the gap between them is the remaining
hole. A record whose own stop was still in flight when the enumeration was taken is
reported present by it — true at that instant, false a moment later. Such a record
classifies unknown until its stop resolves, rather than alive from the snapshot, and the
next enumeration answers for it truthfully. Reading it as alive would be the same
permanent-stale-row outcome the paragraph above closes, arriving through a narrower
door.

Worktrees are unaffected. They are durable on disk and every path here preserves
them, including a refused adoption and an unowned shutdown.

## Security

The ownership credential is the sensitive addition. It is a bearer token for a
service that executes commands in a user's worktree, so it lives in the secret
store with rotation on every spawn and every adoption, never in the database and
never in a configuration file. The durable record holds a reference only.

Adoption is a *mutual* authentication boundary, and the direction that is easy to
miss is the server's. Presenting the credential proves only that the backend holds
it. The identity endpoint sits below authentication because it decides whether to
authenticate at all, so anything that can reach it can read the identity and repeat
it; a comparison of echoed values proves nothing about a counterparty that supplies
both sides of it. A process that takes the recorded endpoint after the real server
releases it -- a crash, a reboot, or an unowned shutdown -- would then be handed the
credential and have the replacement it invents persisted. Because an adopted endpoint
becomes the control server for the rest of the backend's life, that is not one
session but every later agent launch, shell execution and workspace file operation.
So `AC-EXECUTORS-CONTROL-OWNERSHIP-001.10` requires the server to prove possession
first, by a per-attempt challenge whose response is derivable only from the
credential, before the credential is sent or any replacement is stored; and
`AC-EXECUTORS-CONTROL-OWNERSHIP-001.11` keeps filesystem paths off the
unauthenticated endpoint, which needs only the opaque identity, the capability set
and the server's resolved unowned period.

Enumeration returns the agent process environment, which carries provider
credentials. The adoption path consumes the fields it needs and must not log or
persist the response.

Workspace source roots are a trust boundary. An adopted instance's permitted
paths are read from that instance and never widened by the backend, which is
stronger than restoring them from a record that does not hold them.

## Capability gating and scope

The capability is a runtime flag in the existing registry, requiring restart,
defaulting off in every shipped profile. Disabled reproduces today's behavior
exactly: all kill paths active, no adoption, no recovered instances.

### Configuration surface for the numeric tunables

The capability flag itself is a runtime flag, but five numeric values are not, and the
registry cannot hold them. Where they live, what they are named, and who reads each one is
stated once, in part 3's
[Configuration surface](agent-survival-across-restart-03.md#configuration-surface-for-the-numeric-tunables),
and is not restated here.

### Platform scope

The Windows kill-on-job-close binding exists specifically to stop a crashed
backend leaking an agentctl that holds the control port and requires manual
intervention before Kandev can start again. Enabling survival on Windows means
removing that protection and depending on adoption to reclaim the process, which
trades a known-good behavior for an untested one on the platform this capability
is least exercised on. Survival is therefore supported on macOS and Linux, and on
Windows the capability reports unavailable and behaves as disabled. The job
object stays.

Unavailable is a third state, not a synonym for disabled, and it needs somewhere
to be seen. The runtime-capability surface that already reports whether the
capability is enabled reports unavailable alongside it, with the reason, so the
Feature Toggles row shows an operator that the platform does not support this
rather than that someone switched it off. Without that distinction the two are
indistinguishable from outside, and `AC-EXECUTORS-SURVIVAL-005.4` would have
nothing observable to assert.

That surface is the runtime flag registry, and today it cannot express this. A flag
definition carries a kind, a label, stability and risk metadata, a mutability flag and
a restart-required flag, and resolves to a boolean bound to a configuration field.
There is no unavailable state and no field able to carry a reason. So the mechanism is
named here rather than left to whoever builds it first, because the two obvious
choices differ in blast radius and only one of them is acceptable:

- **Chosen:** an OPTIONAL availability probe on the shared flag definition — a
  nil-able function returning whether the flag is available on this host and, when it
  is not, a stable machine-readable reason code. Nil for every existing registration,
  which is what keeps this additive: no other flag's behavior, serialization, or
  admin-override path changes, and a nil probe means available. The registry's
  serialized flag state gains an optional availability object carrying the reason
  code; the front end renders a third row state from it, and the reason code is
  translated on the client so the five-language requirement is met with a code rather
  than a server-formatted English string.
- **Rejected:** a bespoke endpoint or a one-off field read only by this flag. It would
  put a second, differently-shaped answer to "can I turn this on?" next to the
  registry's, and the next platform-gated flag would have to choose between them.

An unavailable flag is reported as unavailable whatever its stored value, and behaves
as disabled. Turning it on where it is unavailable is refused rather than silently
stored, so the admin override cannot leave a host claiming a capability it cannot
honour.

### Passthrough scope

A passthrough session's agent runs on a terminal created by an interactive runner
that, for standalone, is constructed inside the **backend** process, not inside
agentctl. The terminal is a direct backend child and the runner's registry is
backend memory, so detaching agentctl does nothing for it: the agent dies with
the backend and there is no state left to adopt. Passthrough sessions therefore
behave as capability-disabled and are never reported as re-tracked. Making them
survive means moving the runner into the control server, which is a separate
capability. The same applies to user-owned terminals, which share the runner.

## Observability

- Structured events for adoption attempted, adopted, and refused with a reason
  drawn from a fixed set, plus detached, unowned shutdown, and instance stopped
  for a missing reconstruction input.
- Counters for adoption outcome by reason, instances re-tracked, instances
  refused by missing input, instances not re-tracked because the startup deadline
  elapsed, reconstruction reads that exhausted their retries, duplicate instances
  stopped after a correlation tiebreak, launches refused while a session was
  re-tracking, superseded-credential rejections, ambiguous correlations, and
  unowned shutdowns.
- The detached control server's diagnostic-output location is recorded in the
  control-server record, so the first question after a surprising restart ("where
  did the survivor log to?") has an answer that does not require guessing a path.
- The refused-by-missing-input counter is the signal that reconstruction is
  incomplete. It is expected to be non-zero during rollout and is the metric that
  decides when the capability can be promoted.

## Related decisions

- [ADR 0018 runtime settings overrides](../../../decisions/0018-runtime-settings-overrides.md)
- [ADR 0019 restart supervisor](../../../decisions/0019-restart-supervisor.md)
