---
created: 2026-09-08
status: draft
requirements:
  - REQ-EXECUTORS-SSH-REACHABILITY-001
  - REQ-EXECUTORS-SSH-REACHABILITY-002
  - REQ-EXECUTORS-SSH-REACHABILITY-003
system_design:
  - ../../specs/executors/system-design/ssh-reachability.md
  - ../../specs/executors/system-design/ssh-reachability-surfaces.md
legacy_specs: []
---

# Implementation Plan: SSH Host Reachability

## Overview

Kandev gains a continuous, cheap answer to one question per configured SSH
host — can the backend still open an authenticated SSH connection to the host
the user trusted — and puts that answer on the SSH settings page.

The build order is bottom-up so that nothing observable exists until the thing
underneath it is proven. Wave 1 delivers two independent primitives that no
caller can reach: the single-host probe with its classification order in
`lifecycle`, and the `executor_reachability` table with its last-write-wins
upsert in the SQLite repository. Wave 2 is the poller, which is the first code
that can call either one, and which owns the hysteresis rule, the interval
configuration, and the shutdown contract. Wave 3 opens the two producers and
consumers of the record that the poller does not own: the HTTP/event
projection, and the launch path, which must never read a record. Waves 4 and 5 are the two user
surfaces, data layer first. Wave 6 proves the whole thing against a real sshd
container and documents the operator key.

The order matters in one specific way: the launch path (task 05) is the
sharpest edge in this capability, because REQ-003 requires that a reachability
record never gate a launch. Building it after the poller means the record
already exists and is populated, so task 05's own tests can assert the negative
— that `CreateInstance` reads no record — against a store that genuinely holds
one rather than against an empty stub.

**Two flows were cut from this package to fit the specification size budget:**
the task-card reachability indicator and the launch path's record *write*. Both
are recorded in the requirement document's `## Out of scope` with enough detail
to seed their own cards, and their acceptance-criterion IDs
(`AC-…-002.8`, `.9` and `AC-…-003.4` through `.7`) are retired, not reused. The
launch path keeps its two surviving obligations: it never gates on a record, and
its failure message names the host and the reason classified from that launch's
own attempt.

**Reconciled against spec round 5 (2026-09-15, commit `b0481c492`).** Four
rounds of spec review closed all 15 open findings from round 4. The changes
that affect this plan and its work orders:

- A configuration reset is now a dedicated second SQL statement, not a branch
  of the observation upsert, and it carries the newly saved `host`. Ownership
  of *detecting* a save lives in `internal/task/service` (`CreateExecutor` /
  `UpdateExecutor`), which notifies an `ExecutorSaveObserver` the reachability
  package implements — not `internal/ssh`, as the pre-round-5 plan assumed.
- `ProbeSSHHost` must bound the SSH handshake on its context, not only the TCP
  dial, or a stalled handshake outlives both `probeTimeout` and `Stop`.
- A new criterion, `AC-EXECUTORS-SSH-REACHABILITY-001.29`, requires a bastion
  (`ProxyJump`) host-key mismatch to classify as `host_key`, matching a
  target-host mismatch. `host` stays the target in every case; a bastion-hop
  failure is disambiguated in `message`, never in `host`.
- The client reconciliation key for a pushed or refetched record is
  `updated_at`, not `checked_at` — a reset clears `checked_at`, so ordering on
  it would make every reset look older than the record it supersedes and get
  discarded. The DTO gained an `updated_at` field.
- The GET routes now have a full edge contract: `404` for a nonexistent or
  soft-deleted id, `400` only for a wrong executor type (not for an
  unresolvable configuration, which is a `200` carrying reason `config`),
  `GET /reachability` ordered by `executor_id` ascending and including
  inactive executors.
- The settings page runs no refresh timer at all while probing is disabled,
  rather than reading a zero effective interval literally.
- The pre-launch warning collapsed from two mechanisms (an interactive prompt
  versus a session-recorded non-interactive note) into one: the backend always
  emits a `session.launch.warning` event for the launched session, and the
  gateway replays the latest warning to a later subscriber while the backend
  is running. A client that initiated that launch also renders the same event
  inline. There is no "interactive user" branch on the backend. **The system design
  attributes the producer to `internal/task/service`; that package is not in
  the launch call path.** Per `apps/backend/AGENTS.md`'s Execution Flow
  (Orchestrator → Lifecycle Manager → ExecutorBackend) and the verified call
  site, the producer belongs in
  `internal/agent/runtime/lifecycle/manager_launch.go`'s
  `launchBuildExecutorRequest`, immediately before `rt.CreateInstance` at its
  existing call site, using the `Manager`'s existing `eventPublisher` (the
  same mechanism behind `PublishPrepareProgress`). Tasks 05 and 07 below use
  this verified location; the spec's package reference needs a follow-up
  correction.
- The operator configuration key's environment and YAML paths diverge: an
  unparsable environment value falls back to the default, but an unparsable
  `config.yaml` value refuses boot like every other typed catalog key. A
  negative whole number decodes cleanly from either source and needs its own
  clamp branch (`below 0` → the default).

## Scope

### In scope

- A single-host SSH reachability probe and a total-ordered classification of
  its failure into the closed reason set `config`, `timeout`, `host_key`,
  `auth`, `network`, `unknown`.
- Persistence of exactly one reachability record per SSH executor, surviving
  restart, converging under concurrent writes on the later observation, and
  removed when the executor is soft-deleted.
- A bounded-concurrency poller on a configurable interval, with asymmetric
  hysteresis (a threshold of consecutive failures down, one success up), an
  overlap-skip rule, and a disable switch.
- HTTP and WebSocket projection of the record, an immediate-probe action that
  coalesces concurrent callers, and a change-only change event.
- The launch path as a non-gating consumer: it reads no record, and its
  dial-failure message names the host and the classified reason.
- The SSH executor settings surface and the pre-launch warning, localized in
  all five supported locales.
- Public documentation for the one new operator configuration key.

### Out of scope

- Health probing for any non-SSH executor type. `HealthCheckAll` and every
  other runtime's `HealthCheck` are unchanged, including
  `SSHExecutor.HealthCheck`, which keeps returning `nil`.
- Repairing what the probe finds: re-resolving a moved host, re-pinning a
  changed fingerprint, restarting or migrating a session off an unreachable
  host.
- Gating, deferring, queueing, or re-routing a launch on a probe result.
- **Deferred (seeds its own card):** the task-card reachability indicator, and
  the launch path as a probe *producer* writing `reachable` on a successful
  dial and a classified failure on a failed one. Both are specified in the
  requirement document's `## Out of scope`. Until the latter lands the poller
  is the only writer, so a host that dies between passes lags the settings page
  by up to the failure threshold times the effective interval.
- Alerting outside the product surface (push, email, webhook, chat).
- Probing remote capability on the cadence. Platform detection, `git`
  presence, per-agent binary readiness, shell discovery, and the agentctl
  cache check stay on the existing manual endpoints.
- Any change to the behavior of the existing `POST /api/v1/ssh/test`
  endpoint, which keeps dialing unpinned against a possibly-unsaved form
  configuration and keeps writing no record.
- A runtime feature toggle. The interval key's `0` value is the kill switch,
  and a toggle would leave a retired identity to carry forever.
- History, trend, uptime percentage, or an incident log. One row per executor.

## Technical approach

### Probe primitive (`internal/agent/runtime/lifecycle`)

A new `executor_ssh_reachability_probe.go` exports the probe and its
classification. The probe takes a resolved `*SSHTarget` and a deadline, calls
the existing `DialSSH` with `PinnedFingerprint` set, closes the client
immediately, and returns a typed outcome. It runs no remote command, so its
cost is one TCP connection plus one SSH handshake.

Classification walks the total order `config`, `timeout`, `host_key`, `auth`,
`network`, `unknown` and takes the first match. `timeout` precedes `network`
because a deadline expiry surfaces through `dialDirect` as a dial error and
would otherwise read as a refused connection. `host_key` precedes `auth`
because both arrive as handshake failures from `ssh.NewClientConn` and only
`host_key` is a security event; the existing `errHostKeyMismatch` is the
discriminator. A bastion (`ProxyJump`) host-key rejection also classifies as
`host_key` per `AC-EXECUTORS-SSH-REACHABILITY-001.29`, even though it arrives
as `ssh: bastion dial: …` rather than `errHostKeyMismatch`, because only the
target hop is fingerprint-pinned — the reason names what failed, not which hop.
`host` always stays the target executor's host, success or failure; a
bastion-hop failure is disambiguated in `message` alone, which names the
bastion and states the failure arose on the jump hop.

`ProbeSSHHost` bounds the handshake phase on its context, not only the TCP
dial: `ssh.NewClientConn` takes no context and `ssh.ClientConfig.Timeout`
covers only `DialSSH`'s own TCP connect, so a host that completes TCP and then
stalls the handshake would otherwise hold its goroutine past `probeTimeout`
and past `Stop`. The probe sets a deadline on the connection covering the
handshake and clears it once the transport is up.

Resolution also validates the pin before any dial: `ResolveSSHTarget` does not
reject an empty `PinnedFingerprint`, and `buildClientConfig`'s host-key
callback accepts any key when the pin is empty, so a probe that skipped this
check would dial unpinned and record a substituted host as `reachable`. The
probe returns reason `config` for an empty pin, exactly as for an
unresolvable configuration, before any network access.

`sshTargetFromExecutorConfig` moves out of `internal/ssh/handlers.go` and is
exported from `lifecycle` as `SSHTargetFromExecutorConfig`, so the poller and
the handlers share one projection of `executors.config` and cannot drift. The
handler delegates to it.

### Persistence (`internal/task/models`, `internal/task/repository`)

One new table, created through the existing idempotent
`r.migrate.Apply("executor_reachability.table", …)` mechanism in
`base_migrations.go`, following the `repository_secret_bindings` precedent, so
it is safe on SQLite, on Postgres, and on an already-migrated database:

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

`last_success_at` is distinct from `checked_at` because the pre-launch warning
must name the age of the last *successful* probe, and `checked_at` is
overwritten by every failure. It is `NULL` until the first success and is
cleared only by a connection-configuration reset.

A separate table rather than columns on `executors`: `executors.config` is
user-authored input and `executors.status` is a user-controlled enable switch,
so observed state must not share either row. It also keeps a write path that
fires every interval off the row that every executor read touches.

`UpsertExecutorReachability` carries
`WHERE excluded.checked_at > executor_reachability.checked_at OR
executor_reachability.checked_at IS NULL`, and reads and writes the
consecutive-failure counter inside that same statement's transaction, so a
scheduled pass and an immediate probe racing on one executor converge on the
later observation and the streak cannot be lost to an interleaving.
`DeleteExecutor` deletes the record alongside the soft delete.

A **second** repository method, `ResetExecutorReachability(ctx, executorID,
host string) error`, is a distinct statement, not a branch of the upsert
above: the upsert's `state` CASE has no `unknown` branch and its `WHERE`
admits only a strictly later `checked_at`, which a reset does not carry. It
writes `unknown`, a zero counter, cleared reason/message, the newly saved
`host`, and both timestamps `NULL`, guarded only by `type = 'ssh' AND
deleted_at IS NULL AND status = 'active'` — no `checked_at` or `updated_at`
pin, because a reset is ordered by the save that caused it, not by an
observation clock, and must win over whatever is stored. An executor that is
`ssh` but not `active` is *not* reset (eligibility wins per
`AC-…-001.17`), and the statement lands zero rows for it.

### Poller (`internal/executors/reachability`, new package)

A ticker loop started from `internal/backendapp/main.go` alongside the
integration pollers and registered with `addRuntimeCleanup`. `Start` and
`Stop` are idempotent, `Stop` drains the in-flight pass, and a `goleak`
`TestMain` guards the package — the conventions `healthpoll` established,
adopted without importing it, because `healthpoll.Prober` is shaped around one
integration with many workspaces rather than many independent hosts with
per-host results and bounded concurrency.

One pass lists `type = 'ssh' AND deleted_at IS NULL AND status = 'active'`,
sorts ascending by `executors.id` (the primary key, never reused, so the order
is total and no tiebreak is needed), fans out over a semaphore of
`passConcurrency`, and awaits all workers. A mutex-guarded `passRunning` flag
makes an overlapping tick a no-op — dropped, not queued, because a queued tick
would compound the overload that caused the overlap and carries identical
information.

The write path owns hysteresis: on success, `reachable` with a zero counter
from any prior state; on failure, increment, and become `unreachable` only on
the probe where the counter reaches `failureThreshold`. A sticky failure
(`config` or `host_key`) bypasses the threshold and writes `unreachable` on
that probe, because both repeat until a human changes something: the threshold
would buy no confidence, and a host-key mismatch is a possible interception
that must not wait out an interval.

One new catalog entry, `executors.sshReachabilityIntervalSeconds` /
`KANDEV_EXECUTORS_SSHREACHABILITYINTERVALSECONDS`, default `60`, supported
range 15 to 3600, `0` disables. Adding it requires a matching entry in
`auditedStartupEnvironmentInventory()` in `catalog_test.go`, which is
deliberately independent of `startupCatalog`, plus a new `ExecutorsConfig`
section on `config.Config` and its `SetDefault`. The other three knobs stay
compile-time constants — `probeTimeout` 10s, `passConcurrency` 4,
`failureThreshold` 2 — because no operator scenario needs them independently
and each key is a permanent contract.

**The environment and YAML paths do not behave alike, and the clamp needs a
negative branch.** `applyBoundedIntEnv` reads the environment only and returns
early when it is unset, so a `config.yaml` value never reaches the
fallback-and-bounds logic; a non-integer YAML value instead hits
`decodeConfig`'s ordinary typed-key path and refuses boot, exactly like any
other catalog key. A negative whole number decodes cleanly from either source,
so the package's own clamp (not the catalog's) needs a `below 0` branch
yielding the default `60`, alongside `0` disables, `1`–`14` → `15`, and above
`3600` → `3600`, logged once at startup.

Observability follows the `office_stall_*` precedent: `expvar` counters
`executor_ssh_reachability_probe_total` (by outcome),
`executor_ssh_reachability_state_transitions_total` (by destination state),
`executor_ssh_reachability_pass_skipped_total`, and
`executor_ssh_reachability_probe_duration_ms`, each also emitted as a
structured `zap` log. Transitions log at `Warn` downward and `Info` upward;
an unchanged probe does not log, so a steady host is silent.

### API and events (`internal/ssh`, `internal/events`, `pkg/websocket`)

Reachability is its own resource, not fields on the executor DTO, so a record
that changes every interval does not invalidate the executor list payload:

| Route | Purpose |
| --- | --- |
| `GET /api/v1/ssh/reachability` | Every SSH executor's record in one request. |
| `GET /api/v1/ssh/executors/:id/reachability` | One executor's record. |
| `POST /api/v1/ssh/executors/:id/reachability/probe` | Probe now, persist, return the record. |

The `POST` route coalesces through a `golang.org/x/sync/singleflight` group
keyed by executor id and the SSH connection configuration, so two overlapping
requests for one connection open one connection while a request for a changed
host or pin gets its own probe. It works while the poller is disabled. The
coalesced probe does
**not** run on any caller's request context — a disconnecting caller must
neither cancel it nor fail the other caller waiting on it — but it is also
**not** `context.Background()`: it is registered on the reachability package's
own `WaitGroup` and context, the off-cycle-probe primitive task 03 exposes, so
`Stop` still cancels and awaits it. Status codes are `404` for a nonexistent
or soft-deleted executor id, `400` when the id names a non-`ssh` executor, and
`409` for a probe request against an inactive SSH executor. `resolveSSHTarget`'s existing mapping
must **not** be reused wholesale for the probe route — that helper also maps a
config-resolution failure to `400`, which is exactly the case
`AC-…-002.5` requires to return a `200` outcome carrying reason `config`
instead. `GET` on an executor with no record returns the `unknown` shape with
every timestamp, including `updated_at`, `null`, never `404`.
`GET /reachability` orders its entries by `executor_id` ascending — the same
order the pass uses — and includes executors whose `status` is not `active`,
carrying their retained record.

`events.ExecutorReachabilityChanged = "executor.reachability.changed"` is
published only when `state` or `reason` differs from the stored record, and is
bridged in `task_notifications.go` alongside `events.ExecutorUpdated`.
Publishing every probe result would put one message per executor per interval
on every connected client forever. The record DTO carries `updated_at`
alongside `checked_at`; it is the field a client reconciles two racing
payloads on (see Frontend below), because a reset clears `checked_at` but
still advances `updated_at`.

**Save detection lives in `internal/task/service`, not `internal/ssh`.**
`Service.CreateExecutor` and `Service.UpdateExecutor`
(`service_resources.go`) are the only two call sites that can create a
before/after connection-configuration comparison — `internal/ssh` never sees
an executor write. Each notifies an `ExecutorSaveObserver` interface the
service declares and the `internal/executors/reachability` package
implements, wired at construction. `applyExecutorUpdates` mutates the loaded
executor in place, so `UpdateExecutor` must copy the before-values out ahead
of that call; read afterwards they compare the row against itself and the
reset never fires. On any difference in the fields Terminology names (a
create is a difference by definition), the observer calls
`ResetExecutorReachability` with the newly saved host and dispatches an
out-of-band probe through the same off-cycle primitive the immediate-probe
route uses, if the executor is still eligible.

### Launch interaction (`internal/agent/runtime/lifecycle`)

`SSHExecutor.CreateInstance` reads no record and no code path consults one to
decide whether to proceed. With the producer write deferred, it writes none
either, so this task adds no recorder and no dependency from `lifecycle` onto
the store.

The launch-failure message names the target host and the reason classified from
**that launch's own** dial through `ClassifyDialError`, so a user is not
left holding an agent-generated message about a firewall when the cause was a
stale address. Kandev never intercepts or edits agent output; it adds its own
attribution alongside.

**The pre-launch warning is one mechanism, not two.** There is no branch on
whether an interactive user is present. `Manager.launchBuildExecutorRequest`
(`manager_launch.go`), which already resolves the per-executor-type backend
and calls `rt.CreateInstance` at its single call site for every executor type,
starts a bounded asynchronous read of the target executor's reachability record
through a narrow read-only accessor immediately before that call — never inside
`CreateInstance` itself, so the negative test (`CreateInstance` reads no record)
stays meaningful. When the executor is `ssh`, the record's state is
`unreachable`, and either periodic probing is enabled or the record's `checked_at`
is within three times the *default* interval, it publishes
`session.launch.warning` for the launched session via the
`Manager`'s existing `eventPublisher` (the mechanism behind
`PublishPrepareProgress`), carrying `executor_id`, `host`, `state`, `reason`,
`last_success_at`, and a timestamp. A read failure produces no warning and
cannot delay the launch. Every launch path — WS-initiated, a
dependency chain, a workflow transition, an autostart — converges on this one
call site, so none can diverge from another. A client that initiated the
launch itself renders the same event inline at the point of initiation, in
addition to it appearing in the session-scoped event stream; there is no second
backend code path to keep in step.

### Frontend (`apps/web`)

`lib/types/http-ssh.ts` gains the record shape including `probing_enabled`,
which is `false` when the interval key is `0` so a surface can distinguish
"not probed yet" from "probing is off" without inferring it from an absent
timestamp, `probe_interval_seconds`, the effective interval the staleness rule
and the refresh cadence are both computed from, and `updated_at`.
`lib/api/domains/ssh-api.ts` gains the three calls. The settings slice holds
records keyed by executor id and applies the change action, reconciling a
racing refetch against a pushed event by keeping whichever payload carries the
**later `updated_at`**, not `checked_at` — a reset clears `checked_at`, so
ordering on it would discard the reset in favor of the stale record it just
invalidated.

The settings card renders state, probed host, reason and message when
`unreachable`, the consecutive-failure count, and the age of the last probe;
it marks the result stale past three intervals, states plainly when probing is
off, and offers the probe-now action. Because a steady state is silent on the
wire, the card refetches its own record on mount and on the interval while it
is open — and runs **no timer at all** while `probing_enabled` is `false`,
fetching once on open and again after an immediate probe instead of reading a
zero effective interval as a cadence.

**The pre-launch warning renders a session event, not a derived client
computation.** The backend always publishes `session.launch.warning` for the
launched session (see Launch interaction above); the gateway replays the latest
warning to a later subscriber while the backend is running. The session view
renders it through the existing session-scoped WS-event handler pattern
(the `executor-prepare.ts` shape), and a client that initiated that launch
also renders the same event inline at the initiation point, naming the host
and the age of the last successful probe. Neither surface derives
"unreachable" from the reachability store slice itself — both render the one
event the backend decided to raise, so there is no second code path that could
diverge from the backend's when-to-warn rule. The launch proceeds without a
confirmation step in either rendering.

All copy goes through `t()` in `executors.json` and `tasks.json` across `en`,
`pt-pt`, `zh-cn`, `zh-hk`, and `zh-tw`, with the failure reason rendered as
translated copy rather than the raw token, and state exposed to assistive
technology as text rather than by color alone.

## Tests

Every acceptance criterion below is traced to the current requirements
revision. A criterion owned by two work orders appears once per side of the
split.

| Acceptance criterion | Evidence |
| --- | --- |
| `AC-…-001.4`, `.5`, `.6`, `.7`, `.8` | `internal/agent/runtime/lifecycle/executor_ssh_reachability_probe_test.go` — `TestProbeSucceedsAndClosesWithoutRemoteCommand`, `TestProbeReportsConfigWithoutDialing`, `TestProbeRejectsUnpinnedFingerprintWithoutDialing`, `TestProbeRejectsChangedHostKeyWithoutRepinning`, `TestProbeDeadlineReportsTimeout`, `TestProbeAbandonsStalledHandshakeAtDeadline`, `TestFailureCarriesExactlyOneReasonAndMessage` |
| `AC-…-001.27`, `.29` | `executor_ssh_reachability_probe_test.go` — `TestClassifyDialErrorOrdering`, table-driven, one case per reason plus the deadline-before-network and host-key-before-auth orderings; `TestBastionHostKeyMismatchClassifiesAsHostKey`, `TestBastionFailureNamesTargetHostAndBastionInMessage` |
| `AC-…-001.16`, `.21`, `.22` | `internal/task/repository/sqlite/executor_reachability_test.go` — `TestSoftDeleteRemovesReachabilityRecord`, `TestUpsertKeepsLaterObservationRegardlessOfCommitOrder`, `TestEqualTimestampWriteIsDiscarded`, `TestRecordAndBothTimestampsSurviveRepositoryReopen`; Postgres parity in `executor_reachability_postgres_test.go` |
| `AC-…-001.1`, `.2`, `.3`, `.13`, `.14`, `.15`, `.17`, `.20`, `.26` | `internal/executors/reachability/poller_test.go` — `TestPassProbesEligibleExecutorsInIDOrder`, `TestPassSkipsNonSSHAndInactiveExecutors`, `TestInactiveExecutorRetainsItsRecord`, `TestOverlappingTickIsDroppedNotQueued`, `TestPassConcurrencyIsBounded`, `TestEmptyPassIsSilent`, `TestResultForNowIneligibleExecutorIsDiscarded`, `TestStopDrainsInFlightPass`, `TestStopDiscardsCancelledProbeResults`; `goleak_test.go` |
| `AC-…-001.9`, `.10`, `.11`, `.12` | `internal/executors/reachability/hysteresis_test.go` — `TestNonStickyFailureFlipsOnThresholdProbe`, `TestStickyFailureFlipsOnFirstProbe`, `TestBelowThresholdFailureStoresReasonWithoutChangingState`, `TestSingleSuccessRecoversFromUnreachable`, `TestAbsentRecordReportsUnknown`, each table-driven over probe sequences asserting the exact probe on which the state flips |
| `AC-…-001.23`, `.24`, `.25` | `internal/common/config/config_test.go` — `TestSSHReachabilityIntervalClampsOutOfRangeAndLogsOnce`, `TestAbsentNegativeOrMalformedIntervalUsesDefault`, `TestZeroIntervalDisablesPollerButNegativeDoesNot`; `internal/executors/reachability/poller_test.go` — `TestDisabledPollerRunsNoPassIncludingAtStart` |
| `AC-…-001.18`, `.19`, `.28`, `AC-…-002.7` | `internal/task/service/service_resources_executors_reachability_test.go` — `TestSavingConnectionConfigNotifiesObserverWithBeforeAndAfterFields`, `TestUnchangedSaveNotifiesNothing`, `TestCreateExecutorAlwaysNotifies`; `internal/executors/reachability/save_observer_test.go` — `TestObserverResetsRecordAndSchedulesOffCycleProbe`, `TestObserverSkipsIneligibleExecutor`; `internal/ssh/reachability_handlers_test.go` — `TestInFlightProbeResultDiscardedAfterConfigSave`, `TestImmediateProbeRunsWhilePollerDisabled`, `TestImmediateProbeRefusedForInactiveExecutor` |
| `AC-…-002.2`, `.3` | `internal/executors/reachability/publish_test.go` — `TestChangePublishesEvent`, `TestUnchangedProbePublishesNothing`, `TestResetPublishesEvent`; `internal/gateway/websocket/task_notifications_test.go` bridge assertion |
| `AC-…-002.1`, `.4`, `.5`, `.6`, `.10`, `.11`, `.12` | `internal/ssh/reachability_handlers_test.go` — `TestOverlappingImmediateProbesShareOneProbe`, `TestSharedProbeCompletesWhenStarterDisconnects`, `TestSharedProbeNotOnCallerRequestContext`, `TestGETReturnsUnknownShapeWithNullUpdatedAtForUnprobedExecutor`, `TestGETReachabilityOrderedByExecutorIDAscending`, `TestGET404ForSoftDeletedExecutor`; `apps/web/components/settings/ssh-reachability-card.test.tsx` — field coverage per state, stale past three intervals, probing-off copy with no refresh timer, load failure renders "not known", not-persisted probe result; `apps/web/lib/state/slices/settings/settings-slice.test.ts` — `applies executor.reachability.changed`, `keeps the later updated_at`; `pnpm run i18n:check` |
| `AC-…-003.1` | `internal/agent/runtime/lifecycle/executor_ssh_reachability_launch_test.go` — `TestCreateInstanceReadsNoReachabilityRecord`, asserting against a store fake that fails the test on any read, plus `TestLaunchProceedsAgainstUnreachableExecutor` |
| `AC-…-003.2`, `.3` | `internal/agent/runtime/lifecycle/manager_launch_reachability_warning_test.go` — `TestDialFailureMessageNamesHostAndClassifiedReason`, `TestWarningPublishedForUnreachableExecutorAcrossEveryLaunchPath` (WS-initiated, dependency chain, workflow transition, autostart), `TestNoWarningWhenProbingDisabledAndRecordStale`, `TestWarningStillPublishedWhenProbingDisabledButRecordFresh`; `apps/web/lib/ws/handlers/ssh-launch-warning.test.ts` — session-stream event applied to session-runtime state; `apps/web/components/task/launch-warning.test.tsx` — inline render for the initiating client names host and last-success age, and does not block |

## E2E tests

The `containers` Playwright project already gates real-sshd scenarios on
`KANDEV_E2E_CONTAINERS=1` and owns the `kandev-sshd:e2e` image, so all four
flows live in `apps/web/e2e/tests/ssh/reachability.spec.ts` under that project.

| Flow | Acceptance criteria |
| --- | --- |
| A reachable host renders the settings panel row as reachable. | `AC-…-002.1` |
| The sshd container is stopped mid-test; after the failure threshold the settings panel reports the host unreachable by name. | `AC-…-001.9`, `AC-…-002.1` |
| The container is restarted; the next probe clears the panel on a single success. | `AC-…-001.11`, `AC-…-002.1` |
| A launch started against the stopped host is attempted and fails with a message naming the host, rather than being refused. | `AC-…-003.1`, `AC-…-003.3` |

## Work orders

- [x] [Task 01: Single-host SSH probe and failure classification](task-01-probe-and-classification.md)
- [x] [Task 02: Reachability record persistence](task-02-reachability-persistence.md)
- [x] [Task 03: Reachability poller, hysteresis, and configuration](task-03-reachability-poller.md)
- [x] [Task 04: Reachability API, change event, and immediate probe](task-04-reachability-api-and-events.md)
- [x] [Task 05: Launch-path non-gating and failure attribution](task-05-launch-non-gating.md)
- [x] [Task 06: Reachability data layer and settings surface](task-06-settings-surface.md)
- [x] [Task 07: Pre-launch reachability warning](task-07-pre-launch-warning.md)
- [x] [Task 08: Container E2E coverage and operator documentation](task-08-e2e-and-docs.md)

### Waves

| Wave | Tasks | Parallel-safe |
| --- | --- | --- |
| 1 | 01, 02 | Yes — disjoint packages, only 02 carries a migration |
| 2 | 03 | No — first caller of both primitives |
| 3 | 04, 05 | Yes — disjoint files, both depend only on 03 |
| 4 | 06 | No — consumes the contract task 04 defines |
| 5 | 07 | No — reads the store slice task 06 creates |
| 6 | 08 | No — proves the assembled surfaces |

## Verification results

All 8 work orders are done. Each task's exact verification commands and
results are recorded in its own `## Results` section
(`task-01-probe-and-classification.md` through
`task-08-e2e-and-docs.md`). Task 08's container E2E spec
(`apps/web/e2e/tests/ssh/reachability.spec.ts`) is the plan's only
end-to-end evidence and passes against a real sshd container
(`KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --project=containers
tests/ssh/reachability.spec.ts`, 2 passed).

## Risks

- **The probe's classification depends on error shapes from
  `golang.org/x/crypto/ssh` that are not part of that package's stable API.**
  A library upgrade could turn a `host_key` failure into `unknown`. Mitigated
  by keying `host_key` off Kandev's own `errHostKeyMismatch` rather than an
  upstream string, and by table-driven tests over constructed errors; the
  residual risk is confined to `auth` versus `network` on exotic errors, where
  the fallback is `unknown` rather than a wrong security verdict.
- **A per-interval write path against SQLite adds sustained write traffic that
  did not exist before.** At the default 60s interval and a realistic executor
  count this is small, but an operator with many SSH executors and a short
  interval could contend with the busy `task_sessions` write path. The
  supported range's 15s floor bounds this; the separate table keeps the
  contention off the executor row.
- **The launch path must read the record for its warning while never reading
  it to decide.** `AC-…-003.2` needs the last-success age, so a read exists in
  the same flow as `AC-…-003.1`'s prohibition. A future change that moves that
  read into `CreateInstance` to "avoid a pointless launch" would silently
  violate REQ-003 without breaking any existing test. Mitigated by keeping the
  read in `Manager.launchBuildExecutorRequest`, the one call site every
  executor-type dispatch already passes through, and by task 05's negative
  test, which asserts against a store fake that fails on any read.
- **The system design's stated package for the warning producer
  (`internal/task/service`) does not match the verified call path.** Every
  launch — WS-initiated, a dependency chain, a workflow transition, an
  autostart — converges in `internal/agent/runtime/lifecycle` at
  `Manager.launchBuildExecutorRequest`'s single `rt.CreateInstance` call site
  (confirmed against `apps/backend/AGENTS.md`'s Execution Flow and
  `manager_launch.go`); `internal/task/service` is not in that path. This plan
  and tasks 05/07 use the verified location. The spec still needs a follow-up
  correction so a future reader isn't sent to the wrong package.
- **Deferring the launch-path write leaves the poller as the only writer.** A
  host that dies between passes lags the settings page by up to the failure
  threshold times the effective interval, and a failed launch — the strongest
  available evidence — updates nothing. Bounded and accepted: `AC-…-003.3`
  still names the host and the classified reason on the failure itself, which
  is what the 2026-09-06 report actually asked for.
- **Adding a catalog entry touches a deliberately duplicated inventory.**
  `auditedStartupEnvironmentInventory()` is independent of `startupCatalog` by
  design; forgetting it fails `TestConfigurationCatalogMatchesAuditedEnvironmentInventory`
  rather than shipping silently, so the risk is a wasted cycle, not a defect.
- **Five-locale copy gates the build.** `check-i18n-keys.mjs` fails on a
  missing key, a dropped placeholder, or a value left identical to English, so
  tasks 06 and 07 each carry their own translation work rather than deferring
  it.
- **The `containers` E2E project needs a real Docker daemon and is slow.**
  Task 08's specs cannot run on a host without Docker, so the rest of the work
  package must be verifiable without them; every acceptance criterion above has
  non-E2E evidence, and the four flows there re-prove end to end what unit
  tests already cover against a host that can actually be stopped.
