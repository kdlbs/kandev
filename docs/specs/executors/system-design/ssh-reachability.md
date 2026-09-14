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
  poller sweeps many independent hosts with per-host results and bounded
  concurrency. Its conventions are kept without importing it.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-EXECUTORS-SSH-REACHABILITY-001` | [The poller](#the-poller), [The probe](#the-probe), [State transitions](#state-transitions), [Persistence](#persistence), [Configuration](#configuration) |
| `REQ-EXECUTORS-SSH-REACHABILITY-002` | [Surfaces design](ssh-reachability-surfaces.md) |
| `REQ-EXECUTORS-SSH-REACHABILITY-003` | [Surfaces design](ssh-reachability-surfaces.md) |

This document owns the engine: probing, classification, state, persistence and
configuration. Its sibling
[SSH Host Reachability Surfaces](ssh-reachability-surfaces.md) owns the
projection: HTTP and event contracts, the web components, and the launch
interaction. The task-card indicator and the launch-path record write are
deferred; see the requirement document's `## Out of scope`.

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
  fingerprint, closes immediately, runs no remote command.
- `ClassifyDialError(err error) Reason` — maps a dial error to exactly one
  reason, and `ProbeOutcome` embeds its result.

**Classification lives here, not in the reachability package, and that is the
load-bearing decision in this section.** The error types it must distinguish are
`lifecycle`'s own and unexported — `errHostKeyMismatch`
(`executor_ssh_connection.go`) is lowercase, so no other package can `errors.As`
it. Exporting the *classifier* rather than the *error types* keeps that boundary
intact and gives the launch path identical classification without a second
implementation; the reachability package never inspects a dial error.
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

**`internal/ssh` (extended).** Owns the HTTP and WebSocket projection: read one
record, read all records, and the immediate-probe action.

**`internal/ssh` and `apps/web`.** See the [surfaces design](ssh-reachability-surfaces.md).

## The poller

A single ticker loop, started where the lifecycle manager and the integration
pollers start and stopped on backend shutdown. Both `Start` and `Stop` are
idempotent. Every probe the package starts is registered on one package-owned
`WaitGroup` and derives from one package context, so `Stop` cancels and awaits
*all* of them — the in-flight pass, an off-cycle probe after a save, a coalesced
immediate probe — not only the pass. That is what makes an immediate probe's
background context safe: it outlives its caller but not the package. Draining is
bounded because every probe carries `probeTimeout`. A `goleak`
`TestMain` guards the package, matching `healthpoll`.

When the effective interval is `0` the poller does not start at all: there is no
startup pass and no ticker. The immediate-probe route still works, because it
does not go through the poller loop. The package context, `WaitGroup` and
semaphore are still created and registered for cleanup; only the pass and ticker
are skipped. A `Start` returning early before creating them leaves immediate
probes unowned at shutdown, breaking the drain contract and the `goleak`
guard.

One pass:

1. List eligible executors: `type = 'ssh'`, `deleted_at IS NULL`,
   `status = 'active'`, capturing each row's `updated_at` with its id.
2. Sort ascending by the `executors.id` column. `id` is the primary key and is
   never reused, so the order is total; no tiebreak is needed or defined.
3. Acquire the shared probe semaphore per executor, preserving dispatch order.
4. Await all workers, then release the pass.

A `sync.Mutex`-guarded `passRunning` flag makes an overlapping tick a no-op: the
tick is dropped, not queued, and increments a skipped-pass counter. A queued tick
would compound the overload that caused the overlap, and the next tick carries
identical information.

**The semaphore is process-global, not per-pass.** One `passConcurrency`-slot
semaphore, owned by the reachability package, is acquired by every probe whoever
asked for it: a scheduled pass, an off-cycle probe after a save, or an
immediate-probe request. Saving twenty executors in a script therefore opens at
most `passConcurrency` connections, not twenty. A launch's own `DialSSH` is not a
probe and never takes a slot: a launch must not queue behind background
probing.

Cost per pass is `N` TCP connections and handshakes for `N` hosts, at most
`passConcurrency` at a time, each bounded by `probeTimeout`. At the defaults
below a single host costs one handshake per minute.

## The probe

Per executor, inside its own `context.WithTimeout(ctx, probeTimeout)`:

1. Project `executors.config` into an `SSHConnConfig` and resolve it with
   `lifecycle.ResolveSSHTarget` — the same projection the existing handler
   performs in `sshTargetFromExecutorConfig`, lifted into a shared helper so the
   poller and the handlers cannot drift. A resolution failure, including a
   missing host or `ssh_host_fingerprint`, classifies as `config` and returns
   without touching the network.
2. `lifecycle.ProbeSSHHost(ctx, target)`, dialing with the pinned fingerprint
   set, so a changed host key fails the handshake rather than being silently
   accepted. That is the substantive difference from the manual test path, which
   dials unpinned to observe a fingerprint for first-time trust.
3. On success the client is closed immediately and the outcome is `reachable`.

`ClassifyDialError` walks a total order and returns the first match. The table
below is written **in that order**, and the order is the contract:

| Order | Reason | Condition |
| --- | --- | --- |
| 1 | `config` | Target resolution failed before any dial. |
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
still. That pair — a non-empty reason under `reachable` or `unknown` — is the
intended representation of a below-threshold failure, not a contradiction. A single dropped packet must not
paint a healthy host as down, while recovery is reported at once.

A save that changes the connection configuration resets the record to `unknown`
with a zero counter, an empty reason and message, and both timestamps cleared,
then probes out of band if the executor is still eligible, because the prior
streak and success describe a different target. A save that changes nothing
resets nothing.

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
either, and a per-interval write path stays off the row every read touches.

**Deletion is explicit, not a cascade.** `Repository.DeleteExecutor` is a *soft*
delete (`UPDATE executors SET deleted_at = ?, updated_at = ?`), so no foreign-key
cascade can ever fire, and the table declares none. `DeleteExecutor` issues
`DELETE FROM executor_reachability WHERE executor_id = ?` in the same
transaction as the soft delete. That call site is the only place a record is
removed.

**One statement decides everything.** The write is a single upsert; the caller
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
millisecond resolution or finer, keeping the tie rule a backstop, not the common
path.

**That `>` is a text comparison, so the encoding is part of the contract.**
`TIMESTAMP` takes NUMERIC affinity and the driver binds `time.Time` as text
(`2006-01-02 15:04:05.999999999-07:00`), so SQLite orders these lexically, not
temporally. Two rules make lexical order equal time order, and both are
required: **normalize to UTC** before writing, because the driver keeps whatever
offset it is handed and a DST fall-back then sorts an earlier observation later;
and **bind `time.Time`, never a pre-formatted string**, because the wire format
is RFC3339 where UTC renders as `Z`, which sorts after `.` and would let a
whole-second observation beat a later sub-second one. With both held the offset
is constant and fixed-width and the fraction left-aligned. Postgres compares its
`timestamp` type temporally and is unaffected, which is why this belongs in the
SQLite tests: the parity test passes on Postgres while SQLite keeps the wrong
row.

The eligibility guard is the `EXISTS` clause above, and `:seen_updated_at` is the
`updated_at` the probe captured before it dialed. Every mutation of the executor
row — configuration save, status change, soft delete — bumps `updated_at`, so
that one comparison covers all three: it stops a probe in flight against the
*old* target from landing after a configuration reset, and stops a worker from
re-creating a deleted record or writing one for an executor deactivated
mid-pass. The `EXISTS` gates the insert path; the `ON CONFLICT` arm inherits it,
because a row can only conflict if the insert was admitted. A refused write is
logged at debug and dropped; the next probe supersedes it.

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
the documented kill switch into a 60-second cadence. Clamping is therefore the
package's job, applied to the non-negative value the catalog returns: `0`
disables, `1`-`14` becomes `15`, above `3600` becomes `3600`, and a clamp logs
once at startup rather than refusing to boot.

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

This capability adds no runtime feature toggle. The interval key already
provides the kill switch through the mechanism the repository designates for
operator startup settings, and a toggle would leave a retired identity to carry
forever.

## Control flow

```text
ticker ──▶ poller.pass()   (only if effective interval > 0)
             │  list eligible SSH executors by executors.id, with updated_at
             ▼
           global semaphore(passConcurrency)  ◀── off-cycle + immediate probes
             ▼
           probe(executor)   SSHTargetFromExecutorConfig ▸ ProbeSSHHost ▸ ClassifyDialError
             ▼
           store.Upsert(observation)   guards: eligible, updated_at, checked_at
             ├─ state or reason changed ──▶ bus.Publish ──▶ WS ──▶ store slice
             └─ unchanged ──▶ stop

launch ──▶ SSHExecutor.CreateInstance   (reads no record, writes no record)
             └─ failed at its dial ────▶ ClassifyDialError ──▶ error names host
```

## Failure and recovery

The poller is best-effort and never a source of truth for a caller. Every
failure mode degrades to a stale or `unknown` record and a log line:

- Listing executors fails: the pass is abandoned with a warning, nothing is
  written, and the next tick retries.
- A probe panics or errors unexpectedly: it classifies `unknown` and the pass
  continues; no other executor is affected.
- A record write fails: logged, the pass completes, the next probe rewrites. A
  lost write costs one interval of freshness.
- A write refused by the eligibility or `updated_at` guard is expected, not an
  error: logged at debug and dropped.
- A probe exceeding `probeTimeout` is abandoned by its context; the client is
  closed on the returning path so it leaks no connection, and its slot released.
- A probe cancelled by `Stop` writes **nothing**, and this is not the same case
  as the one above. Shutdown cancels the package context, so an in-flight dial
  returns `context.Canceled` rather than its own `context.DeadlineExceeded`, and
  none of the guards catch it: a shutdown mutates no executor row, so the
  `EXISTS` and `updated_at` predicates still admit the write. Recorded, it would
  increment `consecutive_failures` for every probe in flight — up to
  `passConcurrency` hosts per restart — and at `failureThreshold` a second
  restart would flip a healthy host to `unreachable`. An operator would then
  come back from a routine deploy to exactly the false alarm this capability
  exists to prevent. `ClassifyDialError` must therefore separate the two context
  errors: only a deadline is a `timeout` observation, and a cancellation is not
  an observation at all.
- The store is unreachable on an API read: the route errors and the surface
  reports reachability as not known, never `unreachable`.

There are no retries inside a pass. The next tick is the retry; an immediate
retry against a host that just refused a connection adds cost, not information.

## Security

The probe uses the pinned fingerprint on the dial to the **target**, so a
host-key change there is a detected `host_key` failure and never a silent
re-pin; the stored fingerprint is read-only to this path. `host_key` reports `unreachable` on its first occurrence
rather than waiting out the threshold: a mismatch is possible interception, and
delaying it a full interval is a security cost with no accuracy benefit.
Credentials are unchanged: the probe reuses the executor's configured identity
source (`ssh-agent` or an identity file) and holds no secret of its own.

`message` carries an SSH error string, which can include host, port, username
and identity path — all values the user configured and can already see on the
same settings page, so the record is not sensitive. `ClassifyDialError` must not
place key material or agent socket contents into `message`.

The probe opens no remote shell and runs no remote command, so it grants nothing
beyond what the manual test already exercises, and adds no new trust boundary.

**`ProxyJump` is a second hop with a weaker trust model, and the reason set must
not paper over it.** An executor pins one fingerprint, and it belongs to the
target. `dialViaJump` verifies the bastion against `~/.ssh/known_hosts` instead,
accepting an absent host with a logged warning — OpenSSH's
`StrictHostKeyChecking=accept-new`. Two consequences the probe inherits:

- A bastion key *mismatch* is rejected, but it surfaces as `ssh: bastion dial:
  …`, not as `errHostKeyMismatch`, so `ClassifyDialError` would call it
  `network` or `unknown`. The interception signal that
  `AC-EXECUTORS-SSH-REACHABILITY-001.7` requires to flip `unreachable` on the
  first occurrence would instead wait out `failureThreshold` under the wrong
  reason. The classifier must map a bastion host-key rejection to `host_key`
  too; the reason describes what failed, not which hop failed.
- Every bastion-path error is about the *bastion*, while the record's `host` is
  the target. A bastion that is down reports the target unreachable and sends an
  operator to a machine that is fine — the same misattribution as the firewall
  message of 2026-09-06, from a different cause. The record must name the host
  actually dialled, or say the failure was on the jump hop.

## Observability

Structured `zap` logs plus `expvar` counters under `/debug/vars`, following the
`office_stall_*` and `routing_*` precedent:

- `executor_ssh_reachability_probe_total`, labelled by outcome (`reachable` plus
  each failure reason).
- `executor_ssh_reachability_state_transitions_total`, by destination state.
- `executor_ssh_reachability_pass_skipped_total`, when a tick is dropped because
  a pass is still running: the interval is shorter than a pass.
- `executor_ssh_reachability_write_refused_total`, when a guard drops a write. A
  rising value means configuration churn, not a fault.
- `executor_ssh_reachability_probe_discarded_total`, when a probe is dropped
  because `Stop` cancelled it. `probe_total` is labelled by outcome, and a
  cancelled probe has no outcome, so without this the shutdown discard above is
  the one drop in the package with no counter behind it — the same reason
  `write_refused_total` exists.
- `executor_ssh_reachability_probe_duration_ms`, the last pass's aggregate
  probe duration.

A transition logs at `Warn` for `reachable` to `unreachable` and `Info` for the
reverse, with executor id, host, reason and failure count. Unchanged results do
not log, so a steady host is silent.

## Prior art and departures

`internal/integrations/healthpoll` is the closest in-repo precedent: an
immediate probe on start, a fixed cadence, a configured-or-skip gate,
best-effort semantics, idempotent `Start`/`Stop`, and a goroutine-leak test.
Jira and Linear persist a last-checked timestamp, a boolean, and an error string
on the config row. This design keeps those conventions and departs from that
persistence shape in four places, because an SSH host is not an HTTP API:

1. **A closed reason set, not only an error string.** Integration health has one
   meaningful failure, credentials rejected. An SSH probe has five a user acts on
   differently, and only a typed reason lets the UI say "fingerprint changed".
2. **Asymmetric hysteresis, with an immediate flip for a deterministic
   failure.** The integration pollers flip on the first result, which would let
   one dropped packet paint a healthy host as unreachable. The rule and its
   reasoning are in [State transitions](#state-transitions).
3. **A separate record, not a column on the configuration row.** Argued in
   [Persistence](#persistence): observed state must not share a row with
   user-authored input.
4. **Changes are pushed, results are not.** Pushing every probe result would put
   one message per executor per interval on the wire forever.

## Testing

Strategy only; the plan's `## Tests` table owns the per-criterion evidence.

- Poller tests drive the ticker with `testing/synctest`, matching
  `healthpoll_test.go`, under a `goleak` `TestMain`.
- Classification and hysteresis are both table-driven — over constructed errors
  for the first, over probe sequences for the second — so the ordering rules and
  the exact probe each state flips on are asserted, not inferred.
- The upsert's ordering rule is exercised on **both** dialects, including the
  whole-second versus sub-second pair that the text-encoding trap above turns
  into a silent SQLite-only failure.
- The guards get a negative test each: soft-deleted, deactivated, or
  reconfigured mid-probe writes nothing.
- The immediate-probe route is tested for caller cancellation and for a
  guard-refused write returning `persisted: false`.
- Web E2E lives in `apps/web/e2e/tests/ssh/` under the `containers` project,
  gated on `KANDEV_E2E_CONTAINERS=1`.

## Related decisions

No new ADR. This design creates no new system boundary and no new ownership
rule: it extends an existing contract using the repository's designated
mechanisms for configuration
([0018](../../../decisions/0018-runtime-settings-overrides.md) covers the
adjacent runtime-override tier), persistence, events and metrics.
