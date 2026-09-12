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
# Agent Survival Across Backend Restart System Design Part 3

## Purpose and boundaries

Part 3 of the design begun in
[part 1](agent-survival-across-restart-01.md), which states the purpose,
boundaries and components this part assumes. This part holds the data contracts
recovery itself consumes and produces: how instances are enumerated, how the
durable records are read, what an execution is reconstructed from, how a turn
that ended while nobody was attached is applied exactly once, and what agent
events a detached instance must retain.

## Requirement mapping

All three parts of this design are listed, because several requirements are
satisfied by sections in more than one. Part 1 holds process lifetime and the
ownership contracts; part 2 holds the control flow, failure behavior and scope;
part 3 holds the recovery data contracts.

| Requirement | Design section |
| --- | --- |
| `REQ-EXECUTORS-SURVIVAL-001` | [Kill paths that must change together](agent-survival-across-restart-01.md#kill-paths-that-must-change-together), [Event retention is not inherited](#event-retention-is-not-inherited), [Shutdown](agent-survival-across-restart-02.md#shutdown) |
| `REQ-EXECUTORS-SURVIVAL-002` | [Instance enumeration](#instance-enumeration), [Reconstruction inputs](#reconstruction-inputs), [Correlation and refusal](agent-survival-across-restart-02.md#correlation-and-refusal), [Recovery concurrency](agent-survival-across-restart-02.md#recovery-concurrency) |
| `REQ-EXECUTORS-SURVIVAL-003` | [Startup](agent-survival-across-restart-02.md#startup), [Persistence](agent-survival-across-restart-02.md#persistence) |
| `REQ-EXECUTORS-SURVIVAL-004` | [Turn outcome across the detached gap](#turn-outcome-across-the-detached-gap) |
| `REQ-EXECUTORS-SURVIVAL-005` | [Capability gating and scope](agent-survival-across-restart-02.md#capability-gating-and-scope) |
| `REQ-EXECUTORS-CONTROL-OWNERSHIP-001` | [Ownership identity and credential](agent-survival-across-restart-01.md#ownership-identity-and-credential) |
| `REQ-EXECUTORS-CONTROL-OWNERSHIP-002` | [Single driver](agent-survival-across-restart-01.md#single-driver) |
| `REQ-EXECUTORS-CONTROL-OWNERSHIP-003` | [Unowned shutdown](agent-survival-across-restart-01.md#unowned-shutdown) |
| `REQ-EXECUTORS-CONTROL-OWNERSHIP-004` | [Capability compatibility](agent-survival-across-restart-01.md#capability-compatibility) |

## Data and contracts

### Instance enumeration

`GET /api/v1/instances` returns a bare JSON array while the control client
decodes `{"instances": [...]}`, so the pair errors on every call and has never
worked. The client's unit test asserts against a hand-written envelope fixture,
which is why the mismatch is invisible in CI; the fix must include a test that
exercises the real handler, not another fixture.

The server moves to the enveloped shape the client already expects, because an
envelope leaves room for the ownership metadata adoption needs. Enumerated
instances gain the session and task identity agentctl already receives at
creation but neither retains on the instance nor exposes, so correlation uses an
identity the control server itself asserts rather than an unverified join.

The enumerated shape today also carries the agent process environment, which
holds provider credentials. Enumeration for adoption must select the fields it
needs rather than logging or persisting the response.

### Recovery record read port

The write-only executor-record interface already has a private read sibling that
the persistence path type-asserts off the same value in order to carry forward
resume tokens. Recovery extends that existing pattern rather than widening the
writer, preserving the single-writer split.

The producer receives records as a parameter rather than acquiring a database
dependency, so the executor stays constructible without one:

```go
RecoverInstances(ctx context.Context, records []*models.ExecutorRunning) ([]*ExecutorInstance, error)
```

All six implementations change signature. Five keep returning `nil, nil`; the
sixth, Kubernetes, was added after the earlier survey of this area and must not
be missed.

### Reconstruction inputs

The recovery path currently builds an execution with fourteen fields, while the
launch path builds one with twenty-three plus the runtime environment. The
difference is what reconstruction must source. Restorable today: session, task,
workspace path, container identity, runtime, instance identity and port,
persisted metadata, and the agent profile identity, which the durable record
carries in its execution-profile column. Not restorable today: run identity,
task-environment identity, Office profile identity, agent identity, provider
session identity, agent and continuation commands and their arguments, history
setting, workspace source roots, and the runtime environment.

The prompt turn identity is deliberately NOT in that list and has no row in the table
below. It is not an `AC-EXECUTORS-SURVIVAL-002.3` reconstruction value at all, so
`AC-EXECUTORS-SURVIVAL-002.14` does not govern it: the identifier that matters after a
restart is the one the control server assigns and reports for the retained turn, under
`AC-EXECUTORS-SURVIVAL-004.1`, and the backend's own in-memory generation counter is
recreated rather than restored. It is sourced in
[Turn outcome across the detached gap](#turn-outcome-across-the-detached-gap), not here.

`AC-EXECUTORS-SURVIVAL-002.14` requires each of the values it does govern to have exactly
one declared source, drawn from **four and only four**: the recovery-inventory record,
another durable record this backend already owns, the adopted instance, and a
deterministic re-derivation from a value already restored from one of those three. Every
row below names which of the four it uses. Naming them here is what stops the builder
inventing a different answer per value:

| Value | Declared source |
| --- | --- |
| Session, task, workspace path, container identity, runtime, instance identity and port, persisted metadata | Recovery-inventory record (already carried today) |
| Agent profile identity | Recovery-inventory record, execution-profile column |
| Office profile identity | Recovery-inventory record, persisted metadata. The row already stores a filtered metadata map, so this is a new key in an existing column rather than a schema change. Empty for every non-Office launch, which `AC-EXECUTORS-SURVIVAL-002.14` classifies as legitimately empty rather than missing |
| Task-environment identity | Durable store, from the session record's own task-environment reference |
| Workspace source roots | The adopted instance, read back and never pushed |
| Runtime environment | The adopted instance's own environment |
| Run identity | The runtime environment, which carries it as `KANDEV_RUN_ID`. Office launches set it from the run; other launches do not set it at all, so empty is legitimate |
| Provider session identity | The adopted instance, which holds the provider session |
| Agent identity, agent and continuation commands and their arguments, history setting | Re-derived from the restored agent profile and the agent-type registry |

The last row is the one that would otherwise be got wrong. Those values look like
per-launch state and are not: the launch path computes them from the profile and the
registry, so recovery recomputes them the same way rather than asking the instance.
`AC-EXECUTORS-SURVIVAL-002.14` forbids reading them from the instance for a reason
beyond tidiness: a compromised or stale instance could otherwise tell the backend
which command to run for the session's next turn.

**Re-derived values are current, not historical, and that is deliberate.** The other
sources reproduce what the execution held before the restart; a re-derivation reproduces
what the profile and the agent-type registry say *now*. Those diverge exactly when the
profile was edited or the registry changed across the outage — which is to say, on an
upgrade, the scenario this whole capability exists to serve. `AC-EXECUTORS-SURVIVAL-002.3`
therefore scopes its "held before the restart" wording to the values whose declared source
is a record or the adopted instance, and states that the re-derived values follow current
configuration instead.

Both readings were available and only one is safe. Restoring the historical command would
mean persisting the resolved command line and arguments on the durable record and
replaying them after an upgrade — running the previous build's agent invocation against
the new build's binary, and reintroducing the stale-input problem the re-derivation was
chosen to avoid. Re-deriving means a session whose profile changed mid-outage continues
under the new profile, which is the same thing that would happen to it on its next turn
anyway had no restart occurred. The cost is that "identical to before the restart" is not
literally true of four values; the benefit is that recovery cannot resurrect a command
that the current installation would no longer choose to run.

Two rows are worth their own paragraph:

- **Workspace source roots** are held by agentctl itself, set at instance
  creation and changed only by an explicit backend rebind. Adoption therefore
  does not need to restore them to keep the boundary intact; it needs to not send
  a rebind that widens them. The backend reads the roots back from the adopted
  instance rather than pushing its own, which is what
  `AC-EXECUTORS-SURVIVAL-002.5` requires.
- **The runtime environment** is deliberately memory-only so credentials are not
  persisted. It is recoverable from the adopted instance's own environment rather
  than from the database, which keeps that property.

### Turn outcome across the detached gap

Two distinct losses have to be closed, and they need different mechanisms.

The first is the completion waiter. The in-memory channel a prompt waits on is
recreated empty by recovery and nobody listens to it, so a turn that ends while
detached has no reader. agentctl therefore retains each instance's last terminal
turn outcome, and re-tracking retrieves it and applies it before that session's state is
published. Publishing as running is what happens when nothing was retained; when an
outcome WAS applied the session is published in the state that outcome produces, not as
running. Applying it afterwards, or publishing running regardless, would surface a
completed session as running until something else happened to it.

The second is the window between re-tracking an execution and attaching its
streams, which is asynchronous. Events produced in that window land in the
per-instance update channel, which is buffered, so a short window drains on
attachment. That buffering is necessary but it is **not** sufficient, and it must not
be mistaken for the retention `AC-EXECUTORS-SURVIVAL-001.6` requires: see
[Event retention is not inherited](#event-retention-is-not-inherited). What is needed
on top is that the retained outcome and a subsequently delivered terminal event do
not both apply.
The execution already carries a prompt-completion generation guard that prevents
a second terminal event from replacing the first outcome for a prompt, and that
guard still does the deduplication. What it cannot do alone is decide that a
retained outcome and a delivered event describe the *same* turn: it keys on an
in-memory generation counter that recovery recreates from zero, and no prompt-turn
identity is persisted on the executor record. So the retained terminal state
carries a turn identifier that agentctl assigns and reports, the backend records
which identifier it applied, and a later event carrying that identifier is a
no-op. The generation guard remains the mechanism; the turn identifier is what
survives the restart to give it something stable to compare. Without it,
`AC-EXECUTORS-SURVIVAL-004.4` would resolve differently depending on which of the
two observations happened to arrive first.

**Retrieval does not clear the retained outcome; an explicit acknowledgement does.** The
obvious reading of "retain until an owning backend retrieves it" is clear-on-read, and it
loses the outcome for good if the backend dies in the gap between reading it and applying
it — the exact failure `REQ-EXECUTORS-SURVIVAL-004` exists to prevent, reintroduced by the
mechanism meant to close it. So the read is a pure read, repeatable and idempotent, and
the control server drops the outcome only when the backend acknowledges it by turn
identifier — and only once that outcome has been durably applied, never in the same
exchange as the read, or the crash window simply moves from after the read to after the
ack. An acknowledgement naming an identifier the server no longer holds, or never
held, is accepted and does nothing, so a retried acknowledgement is safe.

That leaves the other half. A backend that applied an outcome and died before
acknowledging leaves it retained, and the next backend to adopt reads it again. Nothing
durable records which identifiers this installation already applied, and nothing needs to:
applying a terminal outcome to a turn that already reached that outcome in the durable
session and task state is a no-op, because the second application finds the turn already
terminal and changes nothing. Idempotent application is the invariant carrying the weight
here, not a durable applied-set — a set would have to be written *before* the outcome was
applied to help at all, which moves the same crash window one step earlier rather than
closing it.

### Event retention is not inherited

`AC-EXECUTORS-SURVIVAL-001.5` and `AC-EXECUTORS-SURVIVAL-001.6` require that a
detached instance retain its events and PAUSE rather than discard when the retention
limit is reached. It would be convenient if that were already true of the existing
per-instance update channel. It is not, and building on the assumption that it is
would ship silent loss.

The channel is bounded at one hundred events, and exactly one producer blocks on it:
the manager's forwarding goroutine, whose send selects only against the stop channel.
Every other producer degrades when the channel is full, in two different ways, and an
incomplete list here would be the same defect as the false claim it replaces, so both
sets are stated exactly rather than illustrated.

Six sends in the instance manager degrade. Five discard outright, logging a warning:
the agent ERROR event, MCP-attachment evidence, the MCP-attachment attempt, the
**agent-process-exit error event** carrying exit code and recent stderr, and the
permission-cancelled notification. The sixth does something else and is the one a
survey of `default:` branches misses: the permission-*request* notification waits on
a five-second timer and, on expiry, auto-cancels the permission request it was
announcing. Under a detached instance with a full channel that path does not drop an
event, it silently denies every permission the agent asks for, five seconds apart.
Separately, the ACP adapter has one non-blocking send helper reached from four call
sites, covering context-window and session-model events among others; it logs and
discards when the channel is full.

**That is TEN degrading sites: six in the instance manager and four in the ACP adapter.**
Every count below is stated against that ten, and every site is placed in exactly one of
the two sets. A site that appears in neither is a defect in this section, not a decision
left to the builder.

Three of those matter more than the rest. The two error classes are the events most
likely to matter across a detached gap, and the exit error is the only report that an
agent process died at all. The permission-request timer is the one whose failure is
both silent and behavior-changing rather than merely lossy. Any implementation that
fixes only the producers a previous reading named will ship the others still
degrading, which is why this is an enumeration with a count and not an example.

So retention is work this capability must do, not a property it inherits:

- **COVERED — eight of the ten. While an instance is detached these must block rather
  than degrade.** In the instance manager: the agent ERROR event, the agent-process-exit
  error event, the permission-cancelled notification, and the permission-*request*
  notification. In the ACP adapter: all four call sites of the non-blocking send helper.
  The pause is the specified behavior, not a degradation. Converting the
  permission-request timer is a behavior change and not only a plumbing one: while
  detached it must park rather than deny.
- **EXCLUDED — two of the ten, and exactly two. These stay non-blocking.** The
  MCP-attachment evidence publication and the MCP-attachment attempt publication. Both
  report on backend-to-MCP wiring rather than on the agent's own work, neither is an
  agent event under `AC-EXECUTORS-SURVIVAL-001.5`, and neither may be allowed to delay an
  agent. This is a closed set, not an example: adding a member is an amendment to this
  section, so that the event classes the retention guarantee covers stay a decision on
  the record rather than a judgement each builder makes per call site.
- **A converted send must not be an unconditional send. It selects against the
  instance's own stop signal, as the manager's forwarding goroutine already does, subject
  to the lock constraint in the bullet below.** This is the difference between a pause and a hang, and it is the one place a
  literal reading of "block instead of discarding" produces a defect rather than a
  feature. The agent-process-exit send in particular runs on a goroutine the instance's
  own teardown waits for; made unconditional, it blocks forever on a full buffer with no
  backend attached, so the teardown that would have released it can never run, and
  killing the agent process does not release a blocked channel send. Every stop path in
  this design reaches that state deliberately — the duplicate-loser stop of
  `AC-EXECUTORS-SURVIVAL-002.10`, the deadline stop of `AC-EXECUTORS-SURVIVAL-003.7`, and
  the unowned shutdown of `AC-EXECUTORS-CONTROL-OWNERSHIP-003.1`, which by construction
  runs when no backend is attached and none is coming.
- **The forwarding goroutine is the reference only for a producer that holds no lock
  across its send, which is true of the four instance-manager sites and false of the four
  ACP-adapter ones.** The manager's forwarder selects on the stop channel while holding
  nothing, so a stop always reaches it. The adapter's four call sites reach its
  non-blocking send helper with the adapter's own mutex already held — the helper is named
  for that — and the adapter's close routine acquires that same mutex as its first act,
  before it closes the channel that would signal a parked producer and before it cancels
  the adapter lifetime; the instance manager in turn closes the adapter before it closes
  the instance stop channel. A send converted in place at those four sites therefore parks
  holding the one lock every releasing path must acquire first, and no signal in this
  design can reach it: not the adapter close, not the lifetime cancellation, not the
  instance stop. That is a self-deadlock rather than a pause, and it violates
  `AC-EXECUTORS-SURVIVAL-001.10`'s requirement that the stop complete rather than block.
  **A converted ACP send must therefore not park while holding the adapter mutex.** The
  shape already exists in that file: the adapter's other enqueue path performs a blocking
  send that selects on the adapter lifetime context and holds no lock, and it is what the
  close routine's own cancellation is written to release. Conversion at those four sites
  means routing through that shape — releasing the mutex before parking — not adding a
  select to a send that keeps it.
- **Stopping an instance releases every producer parked on it, immediately and without
  waiting for a backend.** Events retained but undelivered at that moment are discarded
  with the instance. That is not the discard `AC-EXECUTORS-SURVIVAL-001.6` forbids: 001.6
  governs an instance that is being kept alive for a backend that has not arrived yet,
  and this is an instance being destroyed, which has no later reader by definition. A
  stop that waits on a parked producer would make the unowned shutdown unable to
  terminate the very instances it exists to reap, which is a worse failure than losing
  the tail of a transcript nobody will read.
- Blocking a producer while detached parks that agent's turn at the boundary rather
  than losing its output. That is intended: the unowned period bounds how long it can
  last, and it is shorter than the idle timeout.

The retention limit is the same one hundred events, now stated as a configurable
default in `AC-EXECUTORS-SURVIVAL-001.6` rather than an incidental buffer size.

#### What "detached" means to a producer, and where it reads it

Seven of the eight COVERED sites need no attached/detached state at all: a stop-aware
blocking send is correct whether or not a backend is attached, because such a send is
self-limiting the moment a live consumer is draining the channel and is released by
teardown when none ever will be. The permission-*request* timer is the eighth, and the
only one that must branch, because its
attached behavior — announce, wait five seconds, auto-cancel — is a deliberate safety valve
against a stalled UI and must survive. Left unconditional it would hang every permission
request forever; left as it is it denies them all while detached. So it needs a signal, and
naming that signal is this section's job rather than the builder's.

**The signal is agentctl-local, and no cross-process mechanism is introduced.** The producer
evaluates the INSTANCE granularity of the definition in
[Terminology](../requirements/standalone-control-server-ownership.md#terminology): an
instance is attached when its event stream to the owning backend is established. agentctl
establishes and tears down those streams itself, so it already holds that fact; the
producer reads it from the same per-instance state the stream lifecycle maintains, and the
transition to attached is what releases anything parked. Nothing has to be pushed down from
the backend, which matters because a pushed flag would be a second, independently stale
answer to a question agentctl can already answer exactly.

The instance granularity is the correct one of the two, and not by default. The coarser
one — the server has an owning backend — is true during the window after adoption and
before this instance's streams are up, and a permission request raised in that window has
no one to answer it even though the server is owned. Evaluating at the server granularity
would auto-deny exactly there, which is the window
`AC-EXECUTORS-SURVIVAL-004.3` already exists because things get lost in.

### Configuration surface for the numeric tunables

The capability flag itself is a runtime flag, but five numeric values are not, and the
registry cannot hold them: a `RuntimeFlagDefinition` resolves to a boolean read off the
configuration struct, with no shape for a duration or a count. Naming the alternative
here matters because the registry is the only configuration mechanism this design
otherwise mentions, and a builder who reached for it would have to invent something.

They go in the startup configuration catalog, which is where every other numeric
runtime value already lives, and they follow the `agentctl.idleTimeout` precedent
exactly: a catalog key with a compatible environment variable, an owner, and a default,
validated at startup alongside the existing `agentctl.*` entries.

| Value | Consumed by | Catalog key | Environment variable | Default |
| --- | --- | --- | --- | --- |
| Startup re-tracking bound (`AC-EXECUTORS-SURVIVAL-003.7`) | Backend | `agentctl.recoveryDeadline` | `KANDEV_ACP_RECOVERY_DEADLINE` | 30s |
| Per-read timeout (`AC-EXECUTORS-SURVIVAL-002.13`) | Backend | `agentctl.recoveryReadTimeout` | `KANDEV_ACP_RECOVERY_READ_TIMEOUT` | 2s |
| Per-read retry count (`AC-EXECUTORS-SURVIVAL-002.13`) | Backend | `agentctl.recoveryReadRetries` | `KANDEV_ACP_RECOVERY_READ_RETRIES` | 2 |
| Unowned period (`AC-EXECUTORS-CONTROL-OWNERSHIP-003.4`) | agentctl | `agentctl.unownedPeriod` | `KANDEV_ACP_UNOWNED_PERIOD` | 10m |
| Detached event retention limit (`AC-EXECUTORS-SURVIVAL-001.6`) | agentctl | `agentctl.detachedEventLimit` | `KANDEV_ACP_DETACHED_EVENT_LIMIT` | 100 |

The environment names are given here rather than left to the implementation because they
are a public configuration surface: they are what an operator sets, what `docs/public`
documents, and what the launcher pass-through below has to name exactly. They follow the
`KANDEV_ACP_*` prefix the `agentctl` catalog owner already uses for
`agentctl.idleTimeout` / `KANDEV_ACP_IDLE_TIMEOUT`, so the five sort together with the
entry they extend rather than starting a second convention beside it.

The consumer column is the part that decides the work. The first three are read by the
backend from its own configuration and need nothing else. The last two are read inside
agentctl, a separate binary that receives a deliberately private subset of the parent's
configuration, so each needs its environment variable added to the launcher's existing
pass-through to the agentctl child, the same route `KANDEV_ACP_IDLE_TIMEOUT` already
takes. Adding the catalog entry without that step yields a value the backend honours
and the control server silently ignores, which for the unowned period means no
self-termination at all.

Validation lives with the other `agentctl.*` checks, and three of these criteria state
rules that only exist there: the retention limit's accepted range, the product of the
per-read timeout and attempt count, and the unowned period's ordering, clamp and floor.
All three reject rather than clamp where the criteria say reject.
