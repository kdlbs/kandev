---
status: draft
system: executors
created: 2026-09-07
owners:
  - tbd
---
# SSH Host Reachability Requirements

## Overview

An SSH executor points at a host Kandev does not own, which can move, sleep, or
change address at any time. `SSHExecutor.HealthCheck` returns `nil`
unconditionally and the registry's health sweep runs once at backend startup, so
no configured host is re-checked while Kandev runs. The manual "Test
connection" action is the only reachability signal, and it sits on a settings
page nobody has open when a host goes away.

On 2026-09-06 an orchestrator host's LAN address changed four times in one day.
Every agent bound to the SSH runner failed with a message naming a firewall
while the cause was a stale address; two cards died mid-task.

This capability gives Kandev a continuous, cheap answer to one question per
configured SSH host: can the backend still open an authenticated SSH connection
to the host we trusted? The answer goes on the executor's settings page.

The executor system owns this because the durable contract is an executor's
availability and its failure and recovery behavior, which the
[executor system boundary](../README.md) already claims.

## Terminology

- **SSH executor:** an `executors` row of type `ssh`, carrying the host, port,
  user, identity source, optional `ProxyJump`, and the pinned host fingerprint a
  user trusted at creation time. Exactly those fields are its **connection
  configuration**; no other column of the row is.
- **Reachability probe:** one attempt to open an authenticated SSH transport to
  an executor's resolved target using its pinned fingerprint, and close it. It
  runs no remote command.
- **Reachability record:** the persisted outcome of the most recent probe for
  one executor: state, failure reason, message, consecutive-failure count,
  probed host, that probe's completion timestamp, the timestamp of the most
  recent probe that succeeded, and the timestamp at which the record itself was
  last written.
- **Reachability state:** `unknown`, `reachable`, or `unreachable`.
- **Probe pass:** one sweep of the poller across every eligible SSH executor.
- **Eligible executor:** an SSH executor that is not soft-deleted and whose
  `status` is `active`.
- **Effective interval:** the interval in force after the normalization required
  by AC-EXECUTORS-SSH-REACHABILITY-001.24. Every rule below referring to "the
  interval" means this value.
- **Sticky failure reason:** `config` or `host_key`. Both repeat on every later
  probe until a human changes something, so neither is a transient blip.

## Requirements

### REQ-EXECUTORS-SSH-REACHABILITY-001: Continuous SSH host reachability probing

**Intent:** Turn "is this SSH host still there?" from a question answered only
by a failing launch into a fact Kandev already holds, cheap enough to pay
continuously, so an operator learns the host moved before a card dies on it.

#### Acceptance criteria

- **AC-EXECUTORS-SSH-REACHABILITY-001.1:** When the poller is enabled, the system shall run one probe pass immediately at backend start and then repeat a probe pass once per effective interval for as long as the backend runs.
- **AC-EXECUTORS-SSH-REACHABILITY-001.2:** When a probe pass runs, the system shall probe every eligible SSH executor exactly once in that pass, and shall probe no other executor type.
- **AC-EXECUTORS-SSH-REACHABILITY-001.3:** When a probe pass runs, the system shall probe eligible executors in a stable total order that is the same for every pass over the same set of executors.
- **AC-EXECUTORS-SSH-REACHABILITY-001.4:** When a probe opens an authenticated SSH transport whose host key matches the executor's pinned fingerprint, the system shall record state `reachable`, an empty reason and message, a consecutive-failure count of `0`, the probed host, and the completion timestamp as both the completion and last-success timestamps, then close the transport without running any remote command.
- **AC-EXECUTORS-SSH-REACHABILITY-001.5:** When a probe does not reach an authenticated transport, the system shall record the failure with exactly one machine-readable reason from the closed set `config`, `network`, `timeout`, `auth`, `host_key`, `unknown`, together with a human-readable message, shall increment the consecutive-failure count, and shall leave the last-success timestamp unchanged. The recorded reason shall be reported until a later probe succeeds, including while the state is still `reachable` or `unknown` below the threshold, so the reason is empty only when the most recent completed probe succeeded or none has completed.
- **AC-EXECUTORS-SSH-REACHABILITY-001.6:** When an executor's configuration cannot be resolved into a connection target, or resolves without a pinned fingerprint, the system shall record reason `config` without attempting any network connection. A probe shall never dial a host whose fingerprint is unpinned: such a dial accepts any key presented and would record a substituted host as `reachable`.
- **AC-EXECUTORS-SSH-REACHABILITY-001.7:** When the presented host key does not match the executor's pinned fingerprint, the system shall record reason `host_key`, shall record state `unreachable` on that first mismatch without waiting for the failure threshold, and shall not update the executor's stored fingerprint.
- **AC-EXECUTORS-SSH-REACHABILITY-001.8:** When an individual probe has not completed within its probe deadline, the system shall abandon that probe, record reason `timeout`, and continue the pass.
- **AC-EXECUTORS-SSH-REACHABILITY-001.9:** When a probe fails with a reason that is not a sticky failure reason, the system shall record state `unreachable` if the resulting consecutive-failure count is at or above the failure threshold, and otherwise shall leave the state exactly as it was, so that a below-threshold failure changes the reason and the counter but never the state.
- **AC-EXECUTORS-SSH-REACHABILITY-001.10:** When a probe fails with a sticky failure reason, the system shall record state `unreachable` on that probe regardless of the consecutive-failure count.
- **AC-EXECUTORS-SSH-REACHABILITY-001.11:** When the current state is `unreachable` and a probe succeeds, the system shall record state `reachable` on that single success, without requiring a second confirming probe.
- **AC-EXECUTORS-SSH-REACHABILITY-001.12:** When an executor has no reachability record, the system shall report state `unknown`, and shall keep reporting `unknown` for that executor until its first probe completes.
- **AC-EXECUTORS-SSH-REACHABILITY-001.13:** When a probe pass is still running at the moment the interval elapses, the system shall skip that tick rather than starting a second concurrent pass or queueing the tick for later.
- **AC-EXECUTORS-SSH-REACHABILITY-001.14:** The system shall bound reachability probes running concurrently across the whole backend by one fixed limit, however many executors are configured, saved at once, or probed on request together. A launch's own SSH connection is not a probe and is not bounded by this limit.
- **AC-EXECUTORS-SSH-REACHABILITY-001.15:** When no eligible SSH executor exists, a probe pass shall complete without opening a connection and without emitting a warning or error log.
- **AC-EXECUTORS-SSH-REACHABILITY-001.16:** When an executor is soft-deleted, the system shall delete its reachability record and stop probing it.
- **AC-EXECUTORS-SSH-REACHABILITY-001.17:** When an executor's `status` is not `active`, the system shall not probe it and shall retain its last reachability record unchanged.
- **AC-EXECUTORS-SSH-REACHABILITY-001.18:** When an SSH executor is created, or a save leaves at least one of its connection-configuration fields different from the stored value, the system shall reset that executor's record to state `unknown` with a consecutive-failure count of `0`, an empty failure reason, an empty message, the newly saved host, no completion timestamp, and no last-success timestamp, and shall then probe it without waiting for the next scheduled pass if it is eligible, and not otherwise. A save that leaves every such field equal to the stored value shall reset nothing and probe nothing.
- **AC-EXECUTORS-SSH-REACHABILITY-001.19:** When an executor's connection configuration is saved while a probe against that executor is already in flight, the system shall discard that probe's result rather than write it: it describes the pre-change target.
- **AC-EXECUTORS-SSH-REACHABILITY-001.20:** When an executor becomes ineligible after a pass has listed it and before that probe's result is written, the system shall discard the result, so it cannot re-create a deleted record or write one for an ineligible executor.
- **AC-EXECUTORS-SSH-REACHABILITY-001.21:** When two probe results for the same executor are written concurrently, the system shall retain the one whose completion timestamp is strictly later and discard the other. Completion timestamps shall be recorded at millisecond resolution or finer, and a result whose timestamp equals the stored one shall be discarded.
- **AC-EXECUTORS-SSH-REACHABILITY-001.22:** When the backend restarts, the system shall preserve each executor's last reachability record and both its timestamps rather than clearing them to `unknown`, and shall report the preserved record until a new probe replaces it.
- **AC-EXECUTORS-SSH-REACHABILITY-001.23:** The probe interval, the per-probe deadline, the concurrent-probe limit, and the failure threshold shall each have a single documented default; the interval shall be operator-configurable through one documented configuration key whose value `0` disables the poller entirely.
- **AC-EXECUTORS-SSH-REACHABILITY-001.24:** When the interval is absent, empty, negative, or not a whole number in the environment variable, the system shall use the documented default and start the backend normally. When the configuration file supplies a value that is not a whole number, the system shall refuse to boot with a configuration error, as it does for every other typed key; a negative whole number from either source shall yield the documented default. When the value is `0` the system shall disable the poller. When it is above `0` but outside the documented supported range, the system shall clamp it to the nearer bound and log the clamp once at startup. Every rule that depends on the interval shall use the resulting effective value.
- **AC-EXECUTORS-SSH-REACHABILITY-001.25:** When the poller is disabled by configuration, no probe pass shall run, including the pass at backend start, and each surface that renders a reachability record shall state that periodic probing is off rather than presenting a retained state as a current result.
- **AC-EXECUTORS-SSH-REACHABILITY-001.26:** When the backend shuts down, the system shall stop the poller and shall cancel and await every probe it started, whether scheduled, off-cycle, or requested, leaving no probe goroutine and no probe connection alive once shutdown returns.
- **AC-EXECUTORS-SSH-REACHABILITY-001.27:** When more than one reason could describe a single probe failure, the system shall select the first that applies in the order `config`, `timeout`, `host_key`, `auth`, `network`, `unknown`.
- **AC-EXECUTORS-SSH-REACHABILITY-001.29:** When a probe is rejected because a jump host's key does not match the entry recorded for that host, the system shall record reason `host_key` and shall record state `unreachable` on that first rejection, exactly as for a mismatch on the target host: the reason describes what failed, not which hop failed.
- **AC-EXECUTORS-SSH-REACHABILITY-001.28:** When the poller is disabled by configuration, the immediate-probe action shall still run on request and shall persist its result under the same reason, threshold, and conflict rules as a scheduled probe.

### REQ-EXECUTORS-SSH-REACHABILITY-002: Reachability surfacing

**Intent:** Put the answer where the user already is. The 2026-09-06 outage was
diagnosed from backend logs; a user whose agent is failing should see that the
runner is unreachable without opening one, and stop debugging the agent.

#### Acceptance criteria

- **AC-EXECUTORS-SSH-REACHABILITY-002.1:** The SSH executor's settings page shall present the executor's reachability state, the host it was probed against, the failure reason and message when the state is `unreachable`, the consecutive-failure count, the age of the last completed probe, and the age of the last successful probe or a statement that none has been recorded.
- **AC-EXECUTORS-SSH-REACHABILITY-002.2:** When an executor's reachability state or failure reason changes, connected clients shall receive that change pushed to them, without reloading the page and without polling for it.
- **AC-EXECUTORS-SSH-REACHABILITY-002.3:** When a probe completes without changing the state or the failure reason, the system shall not push a client update for it, so a steady host produces no per-interval broadcast. A client displaying one executor's reachability detail may refresh that single record for itself while that view is open, and shall keep whichever payload carries the later record-update timestamp, discarding an older one however it arrived. Record-update time orders these, not completion time, so a configuration reset, which clears the completion timestamp, is not dropped as older.
- **AC-EXECUTORS-SSH-REACHABILITY-002.4:** When periodic probing is enabled and the age of the last completed probe exceeds three times the effective interval, the surface shall mark the displayed result as stale rather than presenting it as current. When periodic probing is disabled, the surface shall state that probing is off and shall not compute or present staleness.
- **AC-EXECUTORS-SSH-REACHABILITY-002.5:** The SSH executor's settings page shall offer an explicit action that runs one probe immediately, persists its result under the same rules as a scheduled probe, and returns that result to the caller. When the result is not persisted, because the write failed or a guard refused it, the action shall still return the probe outcome and shall mark it as not persisted rather than reporting the probe itself as failed.
- **AC-EXECUTORS-SSH-REACHABILITY-002.6:** When two immediate-probe requests for the same executor overlap, the system shall run one probe and return its single result to both callers, rather than opening two connections to the same host. The shared probe shall run to completion and persist its result even when the caller that started it disconnects first.
- **AC-EXECUTORS-SSH-REACHABILITY-002.7:** When an immediate probe is requested for an executor that exists and is of type `ssh` but whose `status` is not `active`, the system shall refuse the request, shall open no connection, and shall leave the stored record unchanged.
- **AC-EXECUTORS-SSH-REACHABILITY-002.10:** When a reachability surface cannot load a result, it shall report that the reachability of the host is not known and shall not present the failure to load as an unreachable host.
- **AC-EXECUTORS-SSH-REACHABILITY-002.11:** Every reachability surface shall be operable and legible on mobile viewports and shall expose its state to assistive technology as text, not by color alone.
- **AC-EXECUTORS-SSH-REACHABILITY-002.12:** All reachability copy shall be localized through the product's translation layer in every supported locale, with the failure reason rendered as translated copy rather than the raw reason token.

### REQ-EXECUTORS-SSH-REACHABILITY-003: Probe informs launches without gating them

**Intent:** A probe result is evidence, not permission. A host can recover
between the last probe and the launch, so a stale or mistaken probe must never
stand between a user and their own machine.

#### Acceptance criteria

- **AC-EXECUTORS-SSH-REACHABILITY-003.1:** When a launch targets an SSH executor whose state is `unreachable` or `unknown`, or whose record is stale or absent, the system shall attempt the launch and shall not refuse, defer, or re-route it on the basis of the reachability record.
- **AC-EXECUTORS-SSH-REACHABILITY-003.2:** When a launch targets an SSH executor whose state is `unreachable` and periodic probing is enabled, the system shall start a best-effort warning lookup before the attempt, naming the host and the age of the last successful probe or stating that none has been recorded, and shall proceed without confirmation or delay. The warning shall be published on the launched session event stream, and the gateway shall replay the latest warning to a later subscriber while the backend is running. The initiating client may render the same event inline. When periodic probing is disabled, the system shall warn only if the record's completion timestamp is within three times the default interval, because an older retained record is not a current measurement while a record just refreshed on request is.
- **AC-EXECUTORS-SSH-REACHABILITY-003.3:** When a launch on an SSH executor fails because Kandev could not open its own SSH connection, the reported failure shall name the target host and the failure reason classified from that launch's own connection attempt.

## Prior art

Two external legs were required and neither was available: the `wiki-query`
skill and its vault are absent here, and the `saas-kb` / `search_fsm_docs` MCP
server is not exposed. Both are recorded as unavailable, not as empty results.
In-repository prior art did inform this specification:
`internal/integrations/healthpoll` (the Jira and Linear auth-health poller) and
the existing SSH test-connection endpoint; the four departures are recorded in
the system design's `## Prior art and departures`.

## Out of scope

The first two entries are *deferred*, not rejected; the rest are permanent.
A retired criterion ID is never reused.

- **Deferred: task-card reachability indicator.** Cards bound to an
  `unreachable` executor name that host and clear on return to `reachable`,
  showing nothing in any other state. No polling and no new join: cards carry the
  primary executor's id and type, and every `unreachable` entry and exit is
  already pushed. Retires AC-EXECUTORS-SSH-REACHABILITY-002.8 and 002.9.
- **Deferred: launch path as a probe producer.** A launch's own dial is stronger
  evidence than a probe: success writes `reachable`, a dial-step failure writes a
  classified failure, both under the scheduled probe's rules, while a launch that
  never dials writes nothing. Until it lands the poller is the only writer, so a
  host dying between passes lags the settings page by up to the failure threshold
  times the effective interval. Retires
  AC-EXECUTORS-SSH-REACHABILITY-003.4 through 003.7.
- **Health probing for non-SSH executors.** Local, Docker, Kubernetes, and cloud
  executors have their own availability signals; `HealthCheckAll` is unchanged
  for those runtimes.
- **Repairing what the probe finds.** Re-resolving a moved address, re-pinning
  a changed fingerprint, and migrating a session off an unreachable host are
  excluded: the probe reports, a human decides.
- **Gating, deferring, queueing, or re-routing launches on a probe result.**
  Excluded by REQ-EXECUTORS-SSH-REACHABILITY-003, which requires the opposite.
- **Alerting outside the product surface.** No notification, email, webhook or
  chat message is raised when a host goes unreachable.
- **Probing remote capability on the cadence.** Platform detection, `git`
  presence, agent binary readiness, shell discovery and the agentctl cache check
  stay on the manual test-connection endpoint.
- **Changing the existing manual test-connection endpoint.** It keeps dialing
  unpinned, form-supplied configuration for a host that may not be saved yet, and
  writes no record. AC-EXECUTORS-SSH-REACHABILITY-002.5 is a separate action
  against a saved executor and its pin.
- **Rewriting error text produced by an agent.** The 2026-09-06 firewall message
  came from the agent. This contract adds Kandev's own attribution alongside it
  rather than intercepting agent output.
- **Per-session or per-forward liveness.** Whether a running session's port
  forward still carries traffic is a different question from whether the host
  accepts a new connection.
- **History or trend.** Only the most recent result per executor is retained,
  plus the timestamp of the most recent success. No time series, uptime
  percentage or incident log is kept.
