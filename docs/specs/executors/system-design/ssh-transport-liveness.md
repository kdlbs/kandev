---
status: current
system: executors
requirements:
  - REQ-EXECUTORS-SSH-TRANSPORT-LIVENESS-001
  - REQ-EXECUTORS-SSH-TRANSPORT-LIVENESS-002
---

# SSH Session Transport Liveness System Design

## Purpose and boundaries

The executor system owns the SSH connection a session runs on, so it owns the
question of whether it is still alive. Nothing above it can: the local port
forward is a listener in this process and keeps accepting whether or not the far
end is reachable, so every layer above sees an open socket and waits.

This design uses, and does not own: the agent runtime's controller client and
event stream (they react to a closed connection, not decide one is dead); the
lifecycle manager's remote-status poll (it reads status, not drives teardown);
host-key trust and target resolution (teardown changes neither).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-EXECUTORS-SSH-TRANSPORT-LIVENESS-001` | [Probe mechanism](#probe-mechanism), [Control flow](#control-flow), [Failure and recovery](#failure-and-recovery) |
| `REQ-EXECUTORS-SSH-TRANSPORT-LIVENESS-002` | [Components and responsibilities](#components-and-responsibilities), [Ownership and termination](#ownership-and-termination) |

Acceptance criteria are cited short: `-001.4` is
`AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.4`.

## Components and responsibilities

- **Session state** (`sshSessionState`, `apps/backend/internal/agent/runtime/lifecycle/executor_ssh.go`).
  Gains the watchdog handle, a transport-lost marker, the timestamp of the last
  completed probe reply, and **two independent once-guards: one for the local
  port forward and one for the session SSH client**. The executor mutex guards
  the first three, as it already guards the session map; the once-guards
  synchronise themselves, because every close through one runs with that mutex
  released.

  Two guards, not one spanning both handles: disposal paths issue remote commands
  over the client *between* the two closes, and a combined "close both" guard
  would either close the client before those commands, stranding them, or defer
  the forwarder close until after them, inverting an ordering the stop path
  guarantees. Each handle gets its own guard, closing exactly once whichever of
  teardown or disposal reaches it first.
- **Transport watchdog** (new file, `executor_ssh_keepalive.go`). One per session
  SSH client. Owns its probe cadence, its deadline, and the decision to declare
  the transport lost. Exposes start, stop-deciding, and await-probe-exit; the last
  two are the first and last steps of one disposal, always in that order, with the
  two handle closes and the path's own remote commands in between.
  [Stop sites](#stop-sites-and-the-ordering-that-matters) says why a single `stop`
  cannot work.
- **SSH executor** (`SSHExecutor`). Starts a watchdog when it records a session,
  stops it on every disposal path, and short-circuits remote status for a session
  whose transport is lost.
- **Port forwarder** (`SSHPortForwarder`). Unchanged, but not because its
  `Close` is safe to call concurrently. That `Close` is
  `select { case <-f.closed: return nil; default: close(f.closed) }` — idempotent
  for *sequential* callers, racy for concurrent ones: both can take the `default`
  branch and the second panics closing an already-closed channel, which is exactly
  teardown-versus-stop (`-002.7`). So **every** close of a session's forwarder,
  teardown and all three disposal paths, goes through that session's once-guard,
  and it is the guard, not the forwarder, that makes the close exactly once.

## Data and contracts

### Probe mechanism

The probe is an SSH global request named `keepalive@openssh.com`, sent on the
session client with `wantReply` set and no payload — the OpenSSH convention,
chosen over two alternatives:

- **Not a remote command.** Running something like `kill -0` opens a session
  channel, and `Client.NewSession` takes no context and blocks until that open is
  answered — so on a wedged connection the probe would itself hang forever, the
  exact failure this capability detects. The existing remote-controller liveness
  probe has this shape, which is why a wedged session can stall the shared
  remote-status poll.
- **Any reply proves liveness.** A peer that does not implement the request
  answers with a failure rather than ignoring it; bytes crossed both ways, which
  is the only thing asked. Hence `-001.2` counts a declined reply as completed,
  and the probe works against hosts that do not recognise the request name.

A probe error is never transient: `SendRequest` fails only when the frame cannot
be written or the multiplexer has torn down, and both mean the connection is
finished. Nothing is retryable, which is why `-001.4` tears down on the first
error rather than counting failures.

`Conn.SendRequest` blocks until the reply arrives or the multiplexer tears down,
and takes no context. That single fact shapes the rest of this design: the send
must live on its own goroutine, and the deadline must be enforced by another.

### Tuning values

Package-level variables in the lifecycle package, not constants, so tests can
shorten them:

| Name | Default | Meaning |
| --- | --- | --- |
| `sshKeepaliveInterval` | 15s | Spacing between probes. |
| `sshKeepaliveDeadline` | 45s | Longest gap with no completed reply before the transport is declared lost. |

Neither is read from configuration, an environment variable, or a runtime flag
(`-001.10`). A non-positive value for either, or a deadline not greater than
twice the interval, means "do not start a watchdog", logged once at debug level
(`-002.9`).

`-002.8`'s three remote-cleanup timeouts become package-level variables on the
same terms, so a backstop test need not wait out 20 or 60 seconds of real time.

Both are new; nothing sets them today. They exist so the lifecycle package's tests
can opt out of the watchdog, and that opt-out is load-bearing: fifteen
`CreateInstance` and `ResumeRemoteInstance` calls, in
`executor_ssh_lifecycle_test.go` and `executor_ssh_stop_resume_test.go`, would
each otherwise acquire a real 15-second watchdog under `goleak.VerifyTestMain`, in
tests not about liveness.

Both are therefore zero for this package's tests by default, so no existing test
starts a watchdog and none needs editing; tests that *do* exercise it set
millisecond values, where `-002.9` binds them too — such a test's deadline must
exceed *twice* its interval or no watchdog starts at all. Retrofitting fifteen
unrelated tests with per-test teardown is rejected: it spreads knowledge of this
capability across files with no reason to know it, and a later test would silently
miss the retrofit. Hence `-002.9` makes an unusable pair a first-class "no
watchdog" outcome instead of an error.

## Control flow

### Two goroutines, and why not fewer or more

A watchdog runs exactly two goroutines:

1. **Prober.** Loops: send one probe, publish the outcome, wait one interval.
   Publication is a non-blocking send on a capacity-1 channel, so the prober
   never blocks on a loop that has already returned. **On a probe error it
   publishes the error first and only then returns, permanently** — publishing
   before returning is what lets the loop tear down on the error rather than
   waiting out the deadline (`-001.4`). It also returns, without starting another
   probe, when the stop signal is set, waiting its interval in a `select` on that
   signal rather than sleeping, so a signal delivered while idle wakes it at once.
   Those are its only two exits, both unconditional.
2. **Watchdog loop.** Selects on the stop signal, the prober's outcomes, and a
   deadline timer. A completed reply resets the timer and records the reply time;
   a probe error or a deadline expiry triggers teardown. After a teardown the
   loop returns; it never performs a second one.

The prober's error exit is what makes an internally-triggered teardown terminate:
on a deadline expiry the loop tears down and returns, and the client close inside
that teardown unblocks the prober's outstanding `SendRequest`. Without it the
prober would take that error, wait its interval, and probe a closed client forever
— the goroutine outliving its connection that removed the earlier plumbing. So
teardown ends probing on its own (`-002.12`).

Probes never overlap (`-001.2`): the prober is sequential, so on a wedged
connection exactly one `SendRequest` is outstanding and the deadline, not a
failure count, ends the session. Spacing is end-to-start, so a slow reply delays
the next probe rather than stacking behind it — which is also what lets the
silence interval stand in for how long a probe has been outstanding in the
disposal reading.

Two precedence rules, because a `select` with several ready cases picks at
random.

**A pending completed reply beats an expired deadline timer.** The loop decides
loss from the *recorded time* of the most recent completed reply, not from which
channel it observed first: a reply that completed inside the deadline can still be
unread when the timer fires, and tearing down there would kill a connection that
had answered. When both are ready the loop accounts for the reply first,
recomputes the silence interval, and declares loss only if that interval is
*strictly greater* than the deadline (`-001.3`) — so the timer firing is never
sufficient alone.

**The stop signal beats both**, so a deliberate stop coinciding with a deadline
expiry cannot log a transport-loss warning (`-002.10`): the loop checks it before
acting on either of the others.

Fewer is not possible: with `SendRequest` uncancellable, one goroutine cannot both
wait for a reply and enforce a deadline. More is not needed: a `Client.Wait`
watcher would report a cleanly-ended connection only marginally sooner than the
prober, whose blocked `SendRequest` returns as soon as the multiplexer tears
down.

### Detection bounds

A connection that ends with an error is detected immediately when a probe is in
flight, and within one probe interval (15 s) when the prober is idle. One that
stops answering without an error is detected at the liveness deadline (45 s),
measured from the last reply.

### Transport teardown

Ordered, and the order is contract (`-001.5`):

1. Under the executor mutex, set the transport-lost marker and read the
   forwarder and client handles. Release the mutex before closing anything, so
   one dead session cannot stall another session's operations.
2. Close the local port forward. Its listener closes, so a new local connection
   is refused rather than accepted
   (`-001.6`).
3. Close the session SSH client, tearing down the multiplexer and ending every
   in-flight forwarded channel — which is what turns a hung request above into an
   ordinary connection error. When the client was reached through a bastion, the
   executor's existing bastion-release path fires from the client's own
   termination (`-001.8`).
4. Log one warning (`-001.9`).

Close runtime listener after client.

Step 2 runs through the session's forwarder once-guard and step 3 through its
client once-guard — the two separate guards described in
[Components and responsibilities](#components-and-responsibilities), which the
disposal paths share. Each handle is closed exactly once no matter which side
gets there first (`-002.5`,
`-002.7`).

Teardown issues no remote command and touches no persisted metadata (`-001.11`),
so the remote controller keeps running and its recorded pid, port and session
directory stay valid for a later resume.

Setting the marker in step 1, before any handle is closed, removes the window a
concurrent status read would see: a reader holding the mutex across both the
marker read and its decision either sees the marker and reports `disconnected`,
or sees open handles — never a closing client.

That requires a change to how remote status locks, and the change is part of this
capability rather than an assumption about existing code. `GetRemoteStatus` today
takes the executor mutex only long enough to read the session out of the map,
releases it, then reads `state.client` and probes unguarded — so a teardown can
close the client between the read and the probe. `-001.7`'s short-circuit must
read the marker and the client handle in one critical section and decide there,
returning `disconnected` without issuing any command. Only the decision needs the
mutex; the healthy-session probe runs after it is released, since holding it
across the probe would let one wedged session block status for every other.

Watchdogs share nothing: two sessions on one host hold separate clients and
watchdogs, so a loss tears down only the one that stopped answering.

Teardown does not remove the session from the executor's session map (`-002.6`);
removing it would make a later stop find no state and return early, skipping the
bookkeeping the stop still owes.

That has one consequence outside teardown. The instance-creation path reuses an
already-tracked session by reading its recorded local forward port — a value that
survives the listener being closed — so reusing a torn-down session would return a
healthy-looking instance pointing at a port that now refuses connections: the
silent dead end this capability exists to remove. The reuse path consults the
marker and refuses, naming transport loss (`-002.14`); creation already reports
errors when the dial fails, so callers handle that shape.

## Ownership and termination

### Start sites

A watchdog starts at the two places the executor takes ownership of a session SSH
client and records it: instance creation, and resume of a live remote controller.
Both start it after the session is in the map **and before releasing the executor
mutex**, in the one critical section that also writes the entry (`-001.1`).
Ordering alone is not enough: both sites today write the entry, release the mutex
and return, so a stop landing in that gap would stop a watchdog that did not exist
yet, which would then start against a client the stop had already closed and
declare loss for a session nobody owns. Starting under the mutex costs two
goroutines and no I/O, removing the window rather than narrowing it.

Starting under the mutex closes the window between record and start, but on its
own does not deliver "at most one watchdog per session"
(`-002.1`). Both sites check for an existing
session, **release the mutex to connect**, and only then re-take it to record, so
two callers for one executor instance identifier can both pass the check and both
reach the recording step. Today the second overwrites the entry, orphaning the
first caller's client and forwarder. With a watchdog attached that orphan stops
being merely wasteful: every disposal path finds a session by reading the map, so
an overwritten entry's watchdog is unreachable by all three and never exits,
probing a connection nobody owns or declaring loss against a live session — the
outlived-goroutine failure that removed the earlier plumbing.

The record is therefore **insert-if-absent**, not assignment. A caller finding an
entry present leaves it and its watchdog alone, starts none of its own, closes
the port forward and SSH client it had just obtained, and proceeds with the
existing session on the same terms as any other reuse, which for a session
already marked transport-lost means the refusal of `-002.14` rather than an
instance. Otherwise it reports no error: losing this race means another caller
already produced what this one was asked for. Re-checking under the mutex at the
point of insertion, not only before connecting, is what makes check and record
atomic.

Every other dial in the SSH paths is short-lived, already closed by its own
`defer`, and starts no watchdog (`-002.2`): the connection test used when
configuring an executor, remote task-directory reclamation, and the untracked dial
that resets a credential-broker-backed resume.

### Stop sites, and the ordering that matters

There are exactly three paths that dispose of a tracked session (`-002.4`):
instance stop, executor close, and the tracked credential-broker resume reset.
Each runs the same sequence.

That sequence has six steps, not three, for one reason: **two of the three paths
issue remote commands over the session SSH client between closing the forward and
closing the client, and those commands cannot be moved.** The stop runs the
cleanup script and the remote controller stop there; the reset the controller
stop. The reset carries one further remote call, a credential-broker reachability
preflight, which runs ahead of the removal and is gated by the same reading.

1. **Under the executor mutex, remove the session from the tracked sessions and
   take the disposal reading.** The reading covers the transport-lost marker and
   the silence interval together and classifies the transport as *lost*,
   *unresponsive*, or *answering* (`-002.8`), and that one reading governs every
   remote command on the path. A session with no running watchdog has no silence
   interval and classifies as *answering*, leaving every existing stop unchanged
   when the tuning values are non-positive.

   **The reset takes the reading before its broker-reachability preflight**, which
   runs ahead of the removal and stays there. A reading taken behind that
   preflight is worthless: on a wedged transport it blocks in
   `Client.NewSession`, so the disposal never reads at all and never skips
   anything. Only the reading moves — the preflight keeps its position, so its
   own failure path still returns before the session is untracked. `-002.8`
   therefore anchors the reading no later than the removal rather than at it,
   which is what makes `-002.15` reachable in the one condition it names. For the
   reset the reading is therefore its own, earlier critical section: the mutex is
   taken to read, released for the preflight, re-taken for the removal. The stop
   and the executor close take one section, reading and removing together.

   **Then release the executor mutex and do not re-take it across steps 2 to 6.**
   An invariant, not a preference: teardown takes the mutex as its own first act,
   and step 2 waits for a loop that may be in the middle of one, so a disposal
   holding it would deadlock against the teardown it waits for. The requirement to
   *start* a watchdog under the mutex (`-001.1`) invites the symmetric mistake.

2. **Signal the watchdog loop and wait for the loop — only the loop — to exit.**
   Bounded: the loop is parked in its `select`, already returned, or performing a
   teardown, and a teardown is two local handle closes and one log line, so the
   wait ends without touching the network. It is not bounded by the prober, which
   is the point of splitting the steps. Once this returns, no teardown for this
   session is running and none can start.

3. **Close the local port forward**, through the forwarder once-guard.

4. **Run this path's own remote commands — unless step 1's reading said *lost*
   or *unresponsive*, in which case skip them entirely.** For a stop these are
   the cleanup script and then the remote controller stop, in that order; for the
   reset, the remote controller stop; the executor close has none, so this step is
   empty for it. The reset's preflight runs ahead of step 1, gated by the same
   reading.

   Step 1's reading is a snapshot, and a transport can wedge *after* it, so each
   command runs under its own timeout and one still outstanding when that expires
   is abandoned (`-002.8`). Without it a wedge landing between the reading and the
   command hangs the disposal for ever: step 2 has already retired the only
   component that would have closed the client. The three timeouts are
   `sshCleanupTimeout` (60 s) for the cleanup script, `sshAgentctlCleanupTimeout`
   (20 s) for the controller stop, and a new one for the reset's preflight, which
   runs on the raw caller context today and is therefore unbounded.

   Abandoning needs a second goroutine, because the disposal's own is inside
   `Client.NewSession`. The disposal runs the command on its own goroutine and
   waits on that goroutine and a timer. On the timer it closes the client
   **through the client once-guard**, so step 5 finds the guard spent rather than
   double-closing, marks the client transport-lost for `-002.15`, and only then
   joins the command goroutine. That join is bounded by the close, not by the
   command, and it keeps the abandoned command inside `-002.4`'s no-leak guarantee
   rather than beside it.

   Skipping is what makes `-002.11` achievable; the next section gives the two
   conditions and why each is needed.

5. **Close the session SSH client**, through the client once-guard.

6. **Wait for the prober to exit.** If it was mid-probe, step 5's close unblocks
   its `SendRequest` and it returns through its error exit; if it was idle, step
   2's signal already woke it out of its interval wait. Both are prompt, so this
   never costs a probe interval, keeping a deliberate stop fast rather than merely
   finite. `-002.13` catches an implementation that waits the interval out;
   `-002.11` one that never returns.

Only after step 6 does the disposal call return, which is what `-002.4` requires:
"the call" is the stop, close, or reset call, not an internal helper.

Four collapses of this sequence look simpler and each breaks a named criterion:

- **Stop before closing** (one `stop` that signals and joins both goroutines,
  called before the client is closed) deadlocks on exactly the connection this
  capability exists for: the prober is blocked in an uncancellable `SendRequest`,
  the loop returns on the signal and never tears down, and nothing else closes the
  client because that was scheduled for after `stop` returns. Returning early from
  `stop` leaves the prober running past the call — the leak `-002.4` forbids.
- **Close before stopping.** The prober's in-flight `SendRequest` errors while the
  loop is still selecting, and a probe error declares the transport lost
  (`-001.4`), so a deliberate stop would log a transport-loss warning, which
  `-002.10` forbids.
- **Close the client early, before the remote commands.** It bounds step 4 by
  making every command fail fast on *every* disposal, including the vast majority
  where the transport is healthy. Those commands remove the task directory and the
  remote controller process, so running them over a just-closed client abandons
  that process on the host every time a session stops. It also inverts an ordering
  the stop path guarantees and `executor_ssh_scripts_test.go` asserts directly:
  cleanup before the remote stop and before the client close.
- **Bound the remote commands with a context timeout instead of skipping.** What
  hangs is not bounded by their context: each already runs under a cleanup
  timeout, and it governs execution, not the channel-open preceding it. Step 4's
  backstop is a different mechanism — closing the client, the only thing that
  unblocks a `Client.NewSession` — and it complements the skip rather than
  replacing it: the skip keeps an already-unresponsive disposal from paying that
  timeout at all (`-002.11`).

The sequence needs no "was I asked to stop?" flag inside teardown. Step 2
removes the only component that can declare loss or log, so by
step 5 nobody is left to misread the resulting error — which is also why `-001.4`
is scoped to a *running* watchdog.

Every step is idempotent. A disposal of a session that never started a watchdog,
or whose watchdog already stopped, does nothing in steps 2 and 6 and succeeds. One
whose watchdog already tore the transport down needs no close of its own (the
once-guards make steps 3 and 5 no-ops) and its prober has exited, so step 6
returns at once.

**Two disposals racing each other.** All three paths remove the session in step 1,
so of two concurrent disposals exactly one gets the record. The other finds
nothing and returns success immediately, closing no handle and waiting for nothing
(`-002.4`). The no-remaining-activity guarantee therefore binds the caller that
removed the session, not both: a shared completion handle would let a backend
shutdown block on an unrelated stop's work.

Termination is provable: the loop exits on its signal, and the prober on the error
produced by a close that step 5 guarantees. The package's `goleak` check over
`TestMain` is the standing regression guard.

### Disposing of a session whose transport is lost or unresponsive

Step 1's reading classifies the transport once, and every remote command on that
disposal path obeys it (`-002.8`). Re-reading per decision would let a teardown
land between two of them and produce a stop that ran the cleanup script but
skipped the controller stop.

*Lost* and *unresponsive* reach the same decision for different reasons, and both
are needed. **Lost** means teardown has run and the client is closed, so a remote
command fails immediately and skipping saves only a pointless error.
**Unresponsive** means the client is still open and has stopped answering, so a
remote command does *not* fail — it blocks in `Client.NewSession`, which takes no
context, and never returns. That makes the skip load-bearing, and it is invisible
to the marker alone: a transport is unresponsive for the whole span between twice
the probe interval and the liveness deadline, while the watchdog has correctly not
yet declared it lost. Consulting only the marker would satisfy `-002.8` and still
hang on `-002.11`.

Deriving *unresponsive* from the silence interval costs no new signalling between
the goroutines. Probes are sent one at a time spaced by the interval, so silence
past one interval means a probe is outstanding — but on a healthy link that is
true from every interval boundary until the reply lands, so a one-interval
threshold would call each healthy session unresponsive for one round trip per
cycle and skip the remote commands of a stop with no reason to skip them. The
threshold is therefore *twice* the interval, which `-002.9` keeps inside the
deadline: a probe still unanswered a whole further interval later is not a slow
reply. The timestamp it is computed from is already on the session state for the
teardown log (`-001.9`) and is read under the same mutex, in the same critical
section, as the marker.

The reading is not a guarantee that the transport survives the disposal, and need
not be: a teardown firing just after it closes the client underneath a command
still in flight from before step 2, which then fails promptly and is logged like
any other. A command on a *closed* client fails immediately; only the
*wedged-but-open* client hangs. The single reading buys a coherent, bounded
decision, not an uninterruptible one.

**The stop and the reset differ in what they report.** A stop treats a skipped or
failed cleanup as non-fatal and completes without error (`-002.8`); its job is to
release local resources and it has done so. The reset cannot: its remote
controller stop is the *point* of the call, killing a stale broker-backed
controller before the resume metadata is cleared so a fresh create can run. If
that stop was skipped, or failed because the client was closed under it — by a
teardown, or by step 4's own backstop — the controller is still running on the
host. The same holds for the preflight, whose helper otherwise reports every
failure as broker-unreachable. The reset therefore leaves the resume metadata
unchanged and reports an error identifying transport loss (`-002.15`), and tells
that from a host that refused the call by the transport-lost mark both closers
set.

## Failure and recovery

A session whose transport is torn down does not recover in place. In-flight
requests fail with connection errors, the agent's event stream disconnects through
the path it already uses for a dropped stream, and the session's work ends with an
error instead of hanging. Recovery is a resume: re-dial, re-verify the fingerprint,
open a fresh forward, re-attach to the still-running controller.

## Persistence

None. The watchdog holds no durable state. The transport-lost marker lives on
in-memory session state for the life of the executor's session entry; persisted
resume metadata is untouched by teardown.

## Security

Probes carry no payload and no credential material, and are sent only on
connections whose host key already matched the pinned fingerprint. Teardown
neither re-pins nor clears trust; the next dial runs the existing check
unchanged.

## Observability

- One warning log at teardown naming SSH session transport loss, the executor
  instance identifier, the remote host, and the silence interval
  (`-001.9`). A forwarder or client close
  that returned an error is named in the same line (`-001.5`).
- One debug log with the executor instance identifier when a watchdog is not
  started, because a tuning value is not positive or the deadline is not greater
  than twice the interval.
- Remote status reports `disconnected` with an error naming transport loss, which
  the executor status badge renders (`-001.7`). It short-circuits on the marker
  and issues no command, so an already-dead session cannot delay the shared
  remote-status poll.
- No new `expvar` counter; the structured logs carry the instance identifier.

## Testing

The fake SSH server answers global requests through `ssh.DiscardRequests`, which
replies. This capability needs the opposite: a server that accepts a global
request, and a channel open, and answers neither — so `SendRequest` and
`Client.NewSession` both block as on a wedged link. The fake server gains a switchable mode for that, so a test can run a session healthy,
flip the server silent, and assert teardown within a shortened deadline — bounded
in milliseconds once the tuning values are lowered, with `goleak` over `TestMain`
covering termination. That silent mode makes the hardest criteria testable:

- **A stop that races a silent transport must return** — flip the server silent,
  stop the session well before the deadline, assert the stop completes
  (`-002.11`). A stop-then-close implementation hangs here rather than failing an
  assertion, so this test needs its own bound. It also proves the step 4 skip: an
  implementation issuing either remote command over the still-open wedged client
  blocks in `Client.NewSession` and never reaches the assertion. The reset needs
  the same shape, its preflight being the earlier of its two remote calls.
- **A command that wedges after the reading must not hang the disposal** — read
  *answering*, then flip the server silent: the stop must still return and report
  the command failed (`-002.8`). Millisecond-bounded once the cleanup timeouts are
  lowered; without the backstop it never returns, so it needs its own bound.
- **A deliberate stop must not report transport loss** — the same setup on a
  *healthy* server, asserting no transport-loss warning is recorded (`-002.10`),
  which is what fails if the steps are reordered to close before signalling.
- **An ordinary disposal between two probes must not wait out the interval** — a
  healthy session with a long probe interval and a longer deadline; let one probe
  complete, then dispose of it and assert the call returns in well under one
  interval (`-002.13`). Use the executor-close path, whose lack of remote commands
  makes the whole call time the watchdog wait the criterion bounds. Without it, a
  disposal that waits for the prober's interval timer passes every other criterion
  while making every ordinary stop take up to fifteen seconds.

`goleak` covers the prober's error exit (`-002.12`) only if some test reaches an
internally-triggered teardown and then ends without stopping the session, so the
deadline-expiry test should do exactly that.

Because both tuning values default to zero for this package's tests (see
[Tuning values](#tuning-values)), every test above sets them explicitly and no
other test in the package starts a watchdog. Those variables are package-level and
mutable, which carries one constraint invisible in the code: **a test that sets
any of them must not call `t.Parallel()`**, and neither may any test that creates
or resumes an SSH session while such a test could run. No test in the package uses
`t.Parallel()` today; it is written down because the instinct on a wait-based
liveness test is to parallelise it, and the result is a data race surfacing under
`-race` as a flake in an unrelated change.

## Related decisions

No architecture decision record is required: this capability restores a transport
property inside an existing owned boundary and adds no public contract,
configuration key, or dependency.
