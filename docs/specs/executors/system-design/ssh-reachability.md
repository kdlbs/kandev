---
status: draft
system: executors
requirements:
  - REQ-EXECUTORS-SSH-REACHABILITY-001
  - REQ-EXECUTORS-SSH-REACHABILITY-002
  - REQ-EXECUTORS-SSH-REACHABILITY-003
created: 2026-09-07
owners:
  - tbd
---
# SSH Host Reachability System Design

## Purpose and boundaries

The executor system owns SSH lifecycle and executor failure and recovery
contracts, so it owns the path answering "is this SSH host still reachable".
This design covers the poller, the probe, its classification, persistence and
configuration. The API, event and web projections are in
[SSH Host Reachability Surfaces](ssh-reachability-surfaces.md).

Contracts this design uses but does not own:

- `internal/agent/runtime/lifecycle` owns SSH target resolution
  (`ResolveSSHTarget`), dialing (`DialSSH`), fingerprint pinning, `ProxyJump`,
  and the dial error types. The probe consumes those, and does not reimplement.
- The task system owns `executors`, `executor_profiles`, and `task_sessions`,
  and the event bus to WebSocket bridge in `internal/gateway/websocket`.
- `internal/common/config` owns the operator configuration catalog.
- `internal/integrations/healthpoll` is precedent, not a dependency: its
  `Prober` is shaped around one integration with many workspaces, while this
  poller sweeps many independent hosts. Its conventions are kept, not imported.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-EXECUTORS-SSH-REACHABILITY-001` | [The poller](#the-poller), [The probe](#the-probe), [State transitions](#state-transitions), [Persistence](#persistence), [Configuration](#configuration) |
| `REQ-EXECUTORS-SSH-REACHABILITY-002` | [Surfaces design](ssh-reachability-surfaces.md) |
| `REQ-EXECUTORS-SSH-REACHABILITY-003` | [Surfaces design](ssh-reachability-surfaces.md) |

The task-card indicator and the launch-path record write are deferred; see the
requirement document's `## Out of scope`.

## Components and responsibilities

**`internal/agent/runtime/lifecycle` (extended).** `SSHExecutor.HealthCheck`
keeps returning `nil`: the runtime is available even when an individual host is
not, and the registry's `HealthCheckAll` sweep is a per-runtime concept called
once from `Manager.Start`. Conflating per-host reachability with runtime health
there would mark the whole SSH runtime down because one of several hosts moved.

This package gains **three** exported functions and the two types they need:

- `SSHTargetFromExecutorConfig(map[string]string) (*SSHTarget, error)` — lifted
  from `internal/ssh`; projects `executors.config` into the `SSHConnConfig` that
  `ResolveSSHTarget` takes. That function needs the typed struct, not a map, so
  without this the reachability package must import `internal/ssh` to read an
  executor's own config.
- `ProbeSSHHost(ctx, target *SSHTarget) ProbeOutcome` — dials with the pinned
  fingerprint, closes immediately, runs no remote command. **It must bound the
  handshake on `ctx`, not only the TCP dial.** `ssh.NewClientConn` takes no
  context and `ssh.ClientConfig.Timeout` covers only `ssh.Dial`'s own TCP
  connect, so a host that completes TCP and then stalls the handshake — a hung
  or filtered sshd, the 2026-09-09 shape — would hold its goroutine past
  `probeTimeout` and past `Stop`. `ProbeSSHHost` sets a deadline on the
  connection covering the handshake and clears it once the transport is up, so
  `AC-EXECUTORS-SSH-REACHABILITY-001.8` can abandon the probe and `001.26` can
  drain.
- `ClassifyDialError(err error) Reason` — maps a dial error to exactly one
  reason, and `ProbeOutcome` embeds its result.

**Classification lives here, not in the reachability package, and that is the
load-bearing decision in this section.** The error types it must distinguish are
`lifecycle`'s own and unexported: `errHostKeyMismatch` is lowercase, so no other
package can `errors.As` it. Exporting the *classifier* rather than the *error
types* keeps that boundary intact and gives the launch path identical
classification without a second implementation.
`Reason` is a string enum whose members are exactly `config`, `network`,
`timeout`, `auth`, `host_key`, `unknown`, plus the empty string for success.

**`internal/executors/reachability` (new).** Owns the poller, the concurrency
limiter, the hysteresis rule and the store interface: it enumerates eligible
executors, orders them, runs a bounded pass, calls `ProbeSSHHost`, applies the
state rule, writes results and publishes change events. It depends on a narrow
repository interface, not the full task repository.

**`internal/task/repository/sqlite` (extended).** Owns the
`executor_reachability` table, its additive migration, its queries and the
explicit record delete in [Persistence](#persistence).

**`internal/task/service` (extended).** Owns save detection, because it is the
only layer holding both the stored executor and the incoming change.
`Service.UpdateExecutor` (`service_resources.go`) loads the row, applies the
request and bumps `UpdatedAt`; `Service.CreateExecutor` is the creation call
site. Each notifies an `ExecutorSaveObserver` — an interface the service
declares and the reachability package implements, wired at construction, so the
service depends on an interface rather than on the reachability package. The
observer is handed the connection-configuration fields as they stood before the
save and as they stand after, compares exactly the fields Terminology names,
and on any difference performs the reset in [Persistence](#persistence) and
schedules the off-cycle probe. A create is a difference by definition.

**`applyExecutorUpdates` mutates the loaded executor in place**, so the before
values must be copied out before it runs. Read afterwards they compare the row
against itself, every save looks like a no-op, the reset never fires, and a
re-pointed executor keeps reporting the previous host's state indefinitely.

**`internal/ssh` and `apps/web`.** The HTTP and WebSocket projection, the web
components and the launch interaction. Specified in the
[surfaces design](ssh-reachability-surfaces.md), which owns them; this document
states no contract for them.

## The poller

A single ticker loop, started where the lifecycle manager and the integration
pollers start and stopped on backend shutdown. Both `Start` and `Stop` are
idempotent. Every probe the package starts is registered on one package-owned
`WaitGroup` and derives from one package context, so `Stop` cancels and awaits
*all* of them — the in-flight pass, an off-cycle probe after a save, a coalesced
immediate probe — not only the pass. That is what makes an immediate probe's
package-owned context safe: it outlives its caller but not the package. Draining
is bounded because every probe carries `probeTimeout` *and* because
`ProbeSSHHost` bounds the handshake phase as well as the TCP dial; without that
second half a stalled handshake outlives both its deadline and `Stop`. A `goleak`
`TestMain` guards the package, matching `healthpoll`.

When the effective interval is `0` the poller does not start: no startup pass
and no ticker. The immediate-probe route still works, not going through the
poller loop. The package context, `WaitGroup` and semaphore are still created
and registered for cleanup; only the pass and ticker are skipped. A `Start`
returning early before creating them leaves immediate probes unowned at
shutdown, breaking the drain contract and the `goleak` guard.

One pass:

1. List eligible executors: `type = 'ssh'`, `deleted_at IS NULL`,
   `status = 'active'`, capturing each row's `updated_at` with its id.
2. Sort ascending by the `executors.id` column. `id` is the primary key and is
   never reused, so the order is total; no tiebreak is needed or defined.
3. Acquire the shared probe semaphore per executor, preserving dispatch order.
4. Await all workers, then release the pass.

A `sync.Mutex`-guarded `passRunning` flag makes an overlapping tick a no-op: the
tick is dropped, not queued, and increments a skipped-pass counter. Queueing it
would compound the overload that caused the overlap, and the next tick carries
identical information.

**The semaphore is process-global, not per-pass.** One `passConcurrency`-slot
semaphore, owned by the package, is acquired by every probe whoever asked for
it: a scheduled pass, an off-cycle probe after a save, an immediate-probe
request. Saving twenty executors in a script therefore opens at most
`passConcurrency` connections, not twenty. A launch's own `DialSSH` is not a
probe and never takes a slot: a launch must not queue behind background probing.

Cost per pass is `N` TCP connections and handshakes for `N` hosts, at most
`passConcurrency` at a time, each bounded by `probeTimeout`. At the defaults
below, one host costs one handshake per minute.

## The probe

Per executor, inside its own `context.WithTimeout(ctx, probeTimeout)`:

1. Project `executors.config` into an `SSHConnConfig` and resolve it with
   `lifecycle.ResolveSSHTarget` — the same projection the existing handler
   performs in `sshTargetFromExecutorConfig`, lifted into a shared helper so the
   poller and the handlers cannot drift. A resolution failure classifies as
   `config` and returns without touching the network. **`ResolveSSHTarget` does
   not validate the pin, so the probe checks it separately** and returns `config`
   when the resolved target's `PinnedFingerprint` is empty, before any dial.
   Leaving this to resolution is a security defect rather than a gap:
   `buildClientConfig`'s host-key callback returns `nil` unconditionally when the
   pin is empty, so the probe would dial unpinned, accept whatever key answered,
   and record a substituted host as `reachable`.
2. `lifecycle.ProbeSSHHost(ctx, target)`, dialing with the pinned fingerprint
   set, so a changed host key fails the handshake rather than being silently
   accepted. That is the substantive difference from the manual test path, which
   dials unpinned to observe a fingerprint for first-time trust.
3. On success the client is closed immediately and the outcome is `reachable`.

`ClassifyDialError` walks a total order and returns the first match. The table
below is written **in that order**, and the order is the contract:

| Order | Reason | Condition |
| --- | --- | --- |
| 1 | `config` | Target resolution failed, or resolved with no pinned fingerprint, before any dial. |
| 2 | `timeout` | The probe deadline expired, or the error unwraps to a net timeout. |
| 3 | `host_key` | The handshake failed host-key verification against the pin. |
| 4 | `auth` | Transport negotiation completed; authentication failed. |
| 5 | `network` | TCP dial failed: refused, no route, DNS, network unreachable. |
| 6 | `unknown` | Any other error. |

`config` is decided before any dial. `timeout` precedes `network` because a
deadline expiry surfaces as a dial error and would otherwise read as a refused
connection. `host_key` precedes `auth` because both arrive as handshake failures
and only `host_key` is a security event. The stored message is the error text,
which `dialDirect` already stamps with host and port.

## State transitions

Three states, and one counter that is part of the contract rather than an
implementation detail:

- `unknown`: no record, or a record explicitly reset. Never inferred.
- `reachable`: the last probe succeeded.
- `unreachable`: a sticky failure, or a transient failure streak that reached
  `failureThreshold`.

**On success:** `state = reachable`, `consecutive_failures = 0`, reason and
message cleared, `last_success_at = checked_at`. One success is enough from any
prior state: a host that just answered is reachable, and delaying that
conclusion only prolongs a false alarm.

**On a sticky failure (`config` or `host_key`):** `state = unreachable`
immediately, whatever the counter says. The counter still increments and
`last_success_at` is left alone. Both conditions are deterministic and will
repeat on the next probe, so the threshold would buy no confidence — it would
only delay a possible host-key compromise by a full interval.

**On a transient failure (`network`, `timeout`, `auth`, `unknown`):**
`consecutive_failures += 1`, and the state becomes `unreachable` on the probe
where the counter reaches `failureThreshold`. Until then the state is left
**exactly as it was**: `reachable` stays `reachable` and `unknown` stays
`unknown`. A failing probe never *promotes* a state, so it cannot turn `unknown`
into `reachable`; it changes the reason and the counter while the state stands
still. That pair, a non-empty reason under `reachable` or `unknown`, is the
intended representation of a below-threshold failure, not a contradiction: one
dropped packet must not paint a healthy host as down, while recovery is reported
at once.

A save that changes the connection configuration resets the record to `unknown`,
then probes out of band if the executor is still eligible, because the prior
streak and success describe a different target. A save that changes nothing
resets nothing; the mechanism is in [Persistence](#persistence).

## Persistence

New table, one row per SSH executor:

```sql
CREATE TABLE IF NOT EXISTS executor_reachability (
    executor_id          TEXT PRIMARY KEY,
    state                TEXT NOT NULL DEFAULT 'unknown',
    reason               TEXT NOT NULL DEFAULT '',
    message              TEXT NOT NULL DEFAULT '',
    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    host                 TEXT NOT NULL DEFAULT '',
    checked_at           TIMESTAMP,
    last_success_at      TIMESTAMP,
    updated_at           TIMESTAMP NOT NULL
);
```

`last_success_at` exists because the pre-launch warning must name the age of the
last *successful* probe, and `checked_at` is overwritten by every failure. It is
`NULL` until the first success, and only the configuration reset clears it.

A separate table rather than columns on `executors`: `config` is user-authored
input and `status` a user-controlled switch, so observed state must not share
either, and a per-interval write stays off the row every read touches.

**Deletion is explicit, not a cascade.** `Repository.DeleteExecutor` is a *soft*
delete (`UPDATE executors SET deleted_at = ?, updated_at = ?`), so no foreign-key
cascade can fire and the table declares none. `DeleteExecutor` issues `DELETE
FROM executor_reachability WHERE executor_id = ?` in the same transaction as the
soft delete, and is the only call site that removes a record.

**One statement decides every observation.** The write is a single upsert; the caller
supplies the observation (reason, message, host, `checked_at`) and the statement
derives the counter and the state from the row's own prior values, so no
read-modify-write in Go can lose a streak to an interleaving:

```sql
INSERT INTO executor_reachability (executor_id, state, reason, message,
       consecutive_failures, host, checked_at, last_success_at, updated_at)
SELECT :id, :state, :reason, :message, :failures, :host, :checked_at,
       :last_success_at, :now
 WHERE EXISTS (SELECT 1 FROM executors e
                WHERE e.id = :id AND e.deleted_at IS NULL
                  AND e.status = 'active' AND e.updated_at = :seen_updated_at)
ON CONFLICT (executor_id) DO UPDATE SET
    consecutive_failures = CASE WHEN :reason = '' THEN 0
                                ELSE executor_reachability.consecutive_failures + 1 END,
    state = CASE WHEN :reason = '' THEN 'reachable'
                 WHEN :reason IN ('config','host_key') THEN 'unreachable'
                 WHEN executor_reachability.consecutive_failures + 1 >= :threshold
                      THEN 'unreachable'
                 ELSE executor_reachability.state END,
    reason = :reason, message = :message, host = :host,
    checked_at = :checked_at,
    last_success_at = CASE WHEN :reason = '' THEN :checked_at
                           ELSE executor_reachability.last_success_at END,
    updated_at = :now
WHERE executor_reachability.checked_at IS NULL
   OR excluded.checked_at > executor_reachability.checked_at;
```

On the insert path there is no prior row, so the caller supplies the initial
`state` and `failures`; every later write derives both in SQL from the stored
row. The `ON CONFLICT` `WHERE` is last-write-wins on `checked_at`: a scheduled
pass and an immediate probe racing on one executor converge on the later
observation rather than on whichever transaction committed second, and an equal
timestamp is not later, so the incoming row loses. `checked_at` is stored at
millisecond resolution or finer, keeping the tie rule a backstop.

**That `>` is a text comparison, so the encoding is part of the contract.**
`TIMESTAMP` takes NUMERIC affinity and the driver binds `time.Time` as text
(`2006-01-02 15:04:05.999999999-07:00`), so SQLite orders these lexically, not
temporally. Two rules make lexical order equal time order, and both are
required: **normalize to UTC** before writing, because the driver keeps whatever
offset it is handed and a DST fall-back then sorts an earlier observation later;
and **bind `time.Time`, never a pre-formatted string**, because RFC3339 renders
UTC as `Z`, which sorts after `.` and would let a whole-second observation beat
a later sub-second one. Postgres compares temporally and is unaffected, which is
why this belongs in the SQLite tests: the parity test passes on Postgres while
SQLite keeps the wrong row.

The eligibility guard is the `EXISTS` above, and `:seen_updated_at` is the
`updated_at` the probe captured before it dialed. Every mutation of the executor
row — configuration save, status change, soft delete — bumps `updated_at`, so one
comparison covers all three: it stops a probe in flight against the *old* target
from landing after a reset, and stops a worker from re-creating a deleted record
or writing one for an executor deactivated mid-pass. The `EXISTS` gates the
insert; the `ON CONFLICT` arm inherits it, because a row can only conflict if the
insert was admitted. A refused write is logged at debug and dropped.

**The reset is a second statement, and deliberately not that one.** A
configuration reset is not an observation; it invalidates one. The upsert above
cannot express it — its `state` CASE has no `unknown` branch, and its `WHERE`
admits only a strictly later `checked_at`, which a reset does not carry. Nor may
it be expressed as a delete: `DeleteExecutor` is the only call site permitted to
remove a row.

```sql
INSERT INTO executor_reachability (executor_id, state, reason, message,
       consecutive_failures, host, checked_at, last_success_at, updated_at)
SELECT :id, 'unknown', '', '', 0, :host, NULL, NULL, :now
 WHERE EXISTS (SELECT 1 FROM executors e
                WHERE e.id = :id AND e.type = 'ssh'
                  AND e.deleted_at IS NULL AND e.status = 'active')
ON CONFLICT (executor_id) DO UPDATE SET
    state = 'unknown', reason = '', message = '', consecutive_failures = 0,
    host = :host, checked_at = NULL, last_success_at = NULL, updated_at = :now;
```

The `EXISTS` carries a decision, not just a safety check: an executor whose
`status` is not `active` is **not** reset by a save, because
`AC-EXECUTORS-SSH-REACHABILITY-001.17` retains its record unchanged and
eligibility wins over the reset trigger. The statement lands zero rows and the
retained record stands until the executor is reactivated and probed.

`:host` is the newly saved host, so the settings page never renders a pre-change
host beside `unknown` — the window this capability exists to illuminate is
exactly the one that would otherwise lie. There is deliberately no `checked_at`
guard and no `:seen_updated_at` pin: a reset is ordered by the save that caused
it, not by an observation clock, so it must win over whatever is stored. A probe
already in flight against the pre-save target cannot overwrite it afterwards,
because that probe's own write still carries the pre-save `updated_at` and the
upsert's `EXISTS` rejects it. That is the mechanism
`AC-EXECUTORS-SSH-REACHABILITY-001.19` requires, and it is why the reset needs no
clock of its own.

The reset advances the row's `updated_at` like any other write, which is what
lets a client order it against a concurrent probe result: see the surfaces
design's reconciliation rule.

Retention is one row per executor. The table survives restart and the surface
reports the preserved record until a new probe replaces it: a blank panel after
every restart would be worse than a timestamped result marked stale.

The migration is a single additive `CREATE TABLE IF NOT EXISTS` applied through
the existing idempotent mechanism in `internal/task/repository/sqlite`, so it is
safe on SQLite and Postgres and on an already-migrated database.

## Configuration

One catalog entry in `internal/common/config/catalog.go`:

| Key | Environment variable | Default |
| --- | --- | --- |
| `executors.sshReachabilityIntervalSeconds` | `KANDEV_EXECUTORS_SSHREACHABILITYINTERVALSECONDS` | `60` |

Supported range 15 to 3600 seconds; `0` disables the poller. **The catalog does
not clamp.** `applyNonNegativeIntEnv` falls back to the *default* — never to a
bound — for a value that is absent, empty, negative, fractional or unparsable, so
every one of those yields `60`. Its minimum must stay `0`: widening it to `15` to
express the supported range would push `0` below the minimum and silently turn
the documented kill switch into a 60-second cadence.

**That normalization is the environment path only, and the two sources do not
behave alike.** `applyBoundedIntEnv`, which `applyNonNegativeIntEnv` wraps, reads
`nonEmptyEnv(env, entry.EnvVars...)` and returns early when no environment
variable is set, so a value written in `config.yaml` never reaches its
fallback-and-bounds logic at all. A YAML value is decoded by `decodeConfig`,
which sets no `WeaklyTypedInput`, and `Load` returns the decode error, so a
non-integer in `config.yaml` refuses boot exactly as it does for every other
typed key in the catalog. That is the documented behavior rather than an
exception carved out for this key, and
`AC-EXECUTORS-SSH-REACHABILITY-001.24` states it that way instead of promising a
leniency the loader does not implement. A *negative whole number* does decode
cleanly and so does reach the package, which is why the clamp below needs a
negative branch of its own.

Clamping is therefore the package's job, applied to whatever value reaches it:
below `0` yields the default `60`, `0` disables, `1`-`14` becomes `15`, above
`3600` becomes `3600`, and a clamp logs once at startup rather than refusing to
boot.

**The clamped value is the effective interval, and every consumer uses it.**
Staleness (three times the interval), the `probing_enabled` projection, and the
settings page's own refresh cadence all read the effective interval, never the
raw configured number. An operator who writes `5` gets a 15-second cadence and a
45-second staleness horizon, consistently across the backend and the UI.

The other three knobs are compile-time constants, not configuration, because no
operator scenario has been identified that needs them independently and each new
key is a permanent contract:

| Constant | Value | Rationale |
| --- | --- | --- |
| `probeTimeout` | 10s | `sshDialTimeout` is 30s, which would let one hung host hold a slot for half the interval. |
| `passConcurrency` | 4 | Bounds simultaneous outbound connections and file descriptors while keeping a pass short. |
| `failureThreshold` | 2 | Smallest value that suppresses a single-probe blip; worst-case detection is two intervals. |

This capability adds no runtime feature toggle: the interval key already
provides the kill switch through the designated operator-startup mechanism, and
a toggle would leave a retired identity to carry forever.

## Control flow

```text
ticker -> poller.pass()        (only if effective interval > 0)
  | list eligible SSH executors by executors.id, with updated_at
  v
global semaphore(passConcurrency)   <- off-cycle and immediate probes
  v
probe()   SSHTargetFromExecutorConfig > ProbeSSHHost > ClassifyDialError
  v
store.Upsert(observation)      guards: eligible, updated_at, checked_at
  |- state or reason changed -> bus.Publish -> WS -> store slice
  '- unchanged -> stop

save -> ExecutorSaveObserver   (do the connection-config fields differ?)
  '- yes -> store.Reset(host)  guard: eligible -> bus.Publish -> off-cycle probe

launch -> session creation     reads record, emits session.launch.warning
  '- SSHExecutor.CreateInstance      (reads no record, writes no record)
       '- failed at its dial -> ClassifyDialError -> error names host
```

## Failure and recovery

The poller is best-effort and never a source of truth for a caller. Every
failure mode degrades to a stale or `unknown` record and a log line:

- Listing executors fails: the pass is abandoned with a warning, nothing is
  written, the next tick retries.
- A probe panics or errors unexpectedly: it classifies `unknown` and the pass
  continues; no other executor is affected.
- A record write fails: logged, the pass completes, the next probe rewrites,
  costing one interval of freshness.
- A write refused by a guard is expected, not an error: logged at debug and
  dropped.
- A probe exceeding `probeTimeout` is abandoned by its context; the client is
  closed on the returning path so it leaks no connection, and its slot released.
- A probe cancelled by `Stop` writes **nothing**, which is not the case above.
  Shutdown cancels the package context, so an in-flight dial returns
  `context.Canceled`, and no guard catches it: a shutdown mutates no executor
  row, so `EXISTS` and `updated_at` still admit the write. Recorded, it would
  increment `consecutive_failures` for every probe in flight — up to
  `passConcurrency` hosts per restart — and at `failureThreshold` a second
  restart would flip a healthy host to `unreachable`, turning a routine deploy
  into exactly the false alarm this capability exists to prevent.
  `ClassifyDialError` must therefore separate the two context errors: only a
  deadline is a `timeout` observation, and a cancellation is not an observation.
- The store is unreachable on an API read: the route errors and the surface
  reports reachability as not known, never `unreachable`.

There are no retries inside a pass: the next tick is the retry, and an immediate
retry against a host that just refused a connection adds cost, not information.

## Security

The probe uses the pinned fingerprint on the dial to the **target**, so a
host-key change there is a detected `host_key` failure and never a silent
re-pin; the stored fingerprint is read-only to this path. `host_key` reports
`unreachable` on its first occurrence rather than waiting out the threshold: a
mismatch is possible interception, and delaying it a full interval is a security
cost with no accuracy benefit. Credentials are unchanged: the probe reuses the
executor's configured identity source and holds no secret of its own.

`message` carries an SSH error string, which can include host, port, username
and identity path — all values the user configured and can already see on the
same settings page, so the record is not sensitive. `ClassifyDialError` must not
place key material or agent socket contents into `message`.

The probe opens no remote shell and runs no remote command, so it grants nothing
beyond what the manual test already exercises, and adds no new trust boundary.

**`ProxyJump` is a second hop with a weaker trust model, and the reason set must
not paper over it.** An executor pins one fingerprint and it belongs to the
target; `dialViaJump` verifies the bastion against `~/.ssh/known_hosts` instead,
accepting an absent host with a logged warning (OpenSSH's
`StrictHostKeyChecking=accept-new`). Two consequences the probe inherits:

- A bastion key *mismatch* is rejected, but surfaces as `ssh: bastion dial: …`,
  not as `errHostKeyMismatch`, so `ClassifyDialError` would call it `network` or
  `unknown` and wait out `failureThreshold` under the wrong reason. The
  classifier must map a bastion host-key rejection to `host_key`, which
  `AC-EXECUTORS-SSH-REACHABILITY-001.29` now requires observably.
- Every bastion-path error is about the *bastion*, while the record's `host` is
  the target. A bastion that is down reports the target unreachable and sends an
  operator to a machine that is fine — the same misattribution as the firewall
  message of 2026-09-06, from a different cause. **`host` stays the target in
  every case**: it identifies the executor's subject, the column keeps one
  meaning for success and failure alike, and `AC-…-001.4` and `AC-…-002.1` stay
  true for a probe that succeeds through a jump. The disambiguation belongs in
  `message`, which must name the bastion and state that the failure arose on the
  jump hop whenever it did. `message` is rendered beside the state wherever an
  unreachable record is shown, so the operator reads "unreachable … via bastion
  X" instead of being sent to a healthy target.

## Observability

Structured `zap` logs plus `expvar` counters under `/debug/vars`, following the
`office_stall_*` and `routing_*` precedent. Every name below carries the prefix
`executor_ssh_reachability_`:

- `probe_total`, labelled by outcome (`reachable` plus each failure reason).
- `state_transitions_total`, by destination state.
- `pass_skipped_total`, when a tick is dropped because a pass is still running:
  the interval is shorter than a pass.
- `write_refused_total`, when a guard drops a write. A rising value means
  configuration churn, not a fault.
- `reset_total`, when a save invalidates a record.
- `probe_discarded_total`, when `Stop` cancelled a probe. `probe_total` is
  labelled by outcome and a cancelled probe has none, so without this the
  shutdown discard is the package's only uncounted drop.
- `probe_duration_ms`, the last pass's aggregate probe duration.

A transition logs at `Warn` for `reachable` to `unreachable` and `Info` for the
reverse, with executor id, host, reason and failure count. Unchanged results do
not log, so a steady host stays silent.

## Prior art and departures

`internal/integrations/healthpoll` is the closest in-repo precedent: an
immediate probe on start, a fixed cadence, a configured-or-skip gate,
best-effort semantics, idempotent `Start`/`Stop`, and a goroutine-leak test.
Those conventions are kept. Its persistence shape — a timestamp, a boolean and an
error string on the config row — is not, in four places, because an SSH host is
not an HTTP API: a **closed reason set** rather than an error string, since five
failures are acted on differently and only a typed reason lets the UI say
"fingerprint changed"; **asymmetric hysteresis** rather than flipping on the
first result, which would let one dropped packet paint a healthy host as down
(see [State transitions](#state-transitions)); a **separate record** rather than
a column, since observed state must not share a row with user-authored input
(see [Persistence](#persistence)); and **changes pushed, results not**, since
pushing every result would put one message per executor per interval on the wire
forever.

## Testing

Strategy only; the plan's `## Tests` table owns the per-criterion evidence.

- Poller tests drive the ticker with `testing/synctest`, matching
  `healthpoll_test.go`, under a `goleak` `TestMain`.
- Classification and hysteresis are table-driven — over constructed errors and
  over probe sequences — so the ordering rules and the exact probe each state
  flips on are asserted, not inferred.
- The upsert's ordering rule is exercised on **both** dialects, including the
  whole-second versus sub-second pair the text-encoding trap turns into a silent
  SQLite-only failure.
- The guards get a negative test each: soft-deleted, deactivated or reconfigured
  mid-probe writes nothing. The reset gets its own: it lands with no
  `checked_at`, and a probe in flight from before the save does not overwrite it.
- The immediate-probe route is tested for caller cancellation and for a
  guard-refused write returning `persisted: false`.
- Web E2E lives in `apps/web/e2e/tests/ssh/` under the `containers` project,
  gated on `KANDEV_E2E_CONTAINERS=1`.

## Related decisions

No new ADR: this design creates no system boundary and no ownership rule, and
extends an existing contract through the repository's designated mechanisms for
configuration, persistence, events and metrics.
([0018](../../../decisions/0018-runtime-settings-overrides.md) covers the
adjacent runtime-override tier.)
