---
status: active
system: executors
created: 2026-09-09
owners:
  - kandev
---

# SSH Session Transport Liveness Requirements

## Overview

An SSH executor session owns one SSH connection to its remote host for its whole
life. Every agent operation on that session — the HTTP calls to the remote
controller and the long-lived agent event WebSocket — travels through a local
port forward riding that one connection.

Nothing checks that the connection is still alive. When the underlying network
path dies without a TCP reset (a VPN re-key, a NAT table eviction, a lossy mesh
link), the connection becomes a zombie: the local forward keeps accepting and
the remote end never answers. Requests do not fail, they hang, and stay hung
until something else intervenes.

Measured 2026-09-09 against a runner over a VPN link with 12.5% packet loss: an
SSH-executor session stopped delivering traffic for roughly two hours while sessions on other executors, on the same backend, continued
uninterrupted. It recovered only when re-established on a new local forward
port. Nothing detected the loss and nothing reported it.

This capability makes a dead session transport observable in bounded time: it
probes the connection and closes it when it stops answering, turning an unbounded
hang into an ordinary connection error the layers above already report.

There is a second contract here: the watchdog must be owned by the session it
belongs to and gone once that session is gone. Equivalent plumbing
existed once and was removed before it ever shipped, because its background
goroutines outlived their connections under fault injection, so the ownership
rules are contract, not implementation detail.

## Terminology

- **Session SSH client:** the single SSH connection an SSH executor session
  owns from launch or resume until the session is stopped.
- **Transport liveness probe:** a request sent on a session SSH client that
  requires the remote host to answer, used only to learn whether the connection
  carries traffic.
- **Completed probe reply:** an answer received for a probe. An answer that
  declines the request still proves the connection carries traffic and counts
  as completed.
- **Liveness deadline:** the longest a session SSH client may go without a
  completed probe reply before its transport is declared lost.
- **Silence interval:** for a session SSH client with a running watchdog, the
  time elapsed since the later of that watchdog's start and its most recent
  completed probe reply. It is what the liveness deadline is measured against
  and the value reported in the teardown log. A session with no running watchdog
  has none.
- **Unresponsive transport:** a session SSH client whose silence interval
  exceeds twice the probe interval, meaning a probe has been outstanding for
  longer than a further whole interval. A transport is unresponsive for the
  whole span between twice the probe interval and the liveness deadline, before
  it is declared lost. The design says why.
- **Transport teardown:** closing a session's local port forward and its
  session SSH client after the transport is declared lost.
- **Watchdog:** the background activity that sends probes for one session SSH
  client and performs transport teardown for it.

## Prior art

**Our own prior reasoning (wiki).** Searched: collection `wiki` via the QMD MCP
server (qmd 2.8.3), whose `status` resolved a configured vault path with 459
documents — a healthy semantic query, not a silent grep fallback. Two queries: a typed
`lex`+`vec` pair on keepalive / watchdog / dead-connection terms, and a plain
query on liveness probes and background goroutine ownership. **The wiki returned
nothing useful** — the top hits do not touch connection liveness, so there is no
recorded prior position to follow or depart from.

**What others shipped (saas-kb).** Searched: `fsm-docs-server` MCP,
`search_fsm_docs` with `category: "ai_sdlc"`, on SSH-tunnel keepalive and
agent-session heartbeat phrasings, plus a Paperclip-scoped follow-up and one
`get_fsm_doc` on its sandbox-providers reference. Coverage is thin (best
relevance 0.016); two data points are real:

- **Kiro** keeps an SSH tunnel up with an out-of-process supervisor (a macOS
  LaunchAgent that restarts it): supervise from outside, not probe from within.
- **Paperclip** (v2026.517.0) emits a keepalive on its streaming execution
  endpoint **every 15 seconds** so proxies do not idle out, and tunes probe
  timeouts to 45, 60 and 90 seconds to cover cold starts "without masking real
  hangs".

**What we are doing differently.** Both keep a connection *alive*; this
capability *detects that one is dead* and closes it. Kiro's shape is wrong here:
our tunnel process is fine and the forward is still listening, so only the party
holding the connection can tell it has stopped answering.
Paperclip's numbers corroborate both constants we fix: 15 seconds is a shipped
cadence for a long-lived tunnel, and a deadline near 45 seconds a shipped choice
for "long enough not to fire on a slow link, short enough to still detect".

## Requirements

### REQ-EXECUTORS-SSH-TRANSPORT-LIVENESS-001: Session transport liveness detection

**Intent:** A session whose SSH connection has silently stopped carrying
traffic must be detected and closed within a bounded time, so that work on that
session fails with an error instead of hanging indefinitely.

**User story:** As a user running an agent on my own remote machine, I want a
session whose connection has died to fail promptly, so that I find out within a
minute instead of hours later.

#### Acceptance criteria

- **AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.1:** When an SSH executor session
  takes ownership of a session SSH client, on either a new launch or a resume
  of an existing remote controller, the system shall start one watchdog for
  that client after the session has been recorded in the executor's tracked
  sessions, in the same critical section in which that record is made, so that
  no disposal path can run between the record and the start.
- **AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.2:** While a watchdog is running,
  the system shall send transport liveness probes on its session SSH client one
  at a time, spaced by the probe interval, and shall not start a probe while an
  earlier one on the same client is still outstanding. Any completed probe reply
  shall count as evidence of liveness, whether the host accepted or declined
  it.
- **AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.3:** When a session SSH client's
  silence interval becomes strictly greater than the liveness deadline, the
  system shall declare that session's transport lost and perform transport
  teardown. A silence interval exactly equal to the liveness deadline shall not
  declare transport loss. The comparison shall be made from the recorded time of
  the most recent completed probe reply, not from the order in which a reply and
  a deadline expiry became observable; a completed reply not yet accounted for
  shall be accounted for first.
- **AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.4:** While a watchdog is running,
  when a transport liveness probe returns an error, or the session SSH client's
  transport terminates on its own, the system shall declare that session's
  transport lost and tear it down without waiting for the liveness deadline. A
  probe error observed after that session's watchdog has been stopped shall not
  declare transport loss.
- **AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.5:** Transport teardown shall close
  the session's local port forward first and the session SSH client second. An
  error returned by either close shall not prevent the other close, nor prevent
  the teardown from completing and recording its result. A close that returned
  an error shall be named in the teardown warning required by
  `AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.9`, so that a teardown which could
  not fully release its handles is distinguishable in the log from one that did.
- **AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.6:** After transport teardown for a
  session in which the local port forward's close returned no error, a new TCP
  connection to that session's local forward port shall be refused rather than
  accepted. When that close returned an error, teardown still completes per
  `AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.5` and the port is left as the failed
  close produced; the two never both bind.
- **AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.7:** After transport teardown for a
  session, remote status for that session shall report the state
  `disconnected` with an error message identifying transport loss, and shall not
  send any command over the closed client.
- **AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.8:** When a session SSH client
  reached its host through a ProxyJump bastion, transport teardown shall also
  release the bastion connection.
- **AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.9:** When transport teardown
  completes, the system shall record one warning-level log naming the executor
  instance identifier, the remote host, and the session SSH client's silence
  interval. That is the same quantity
  `AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.3` compares against the deadline, so
  it is defined even when no probe reply ever completed.
- **AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.10:** The probe interval shall
  default to 15 seconds and the liveness deadline shall default to 45 seconds.
  The system shall not expose either as operator configuration and shall not add
  a runtime feature toggle.
- **AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.11:** Transport teardown shall be
  local to the Kandev backend: no command on the remote host, no stop of the
  remote controller, and the session's persisted resume metadata left unchanged,
  so a later resume can re-attach to a controller still running.

### REQ-EXECUTORS-SSH-TRANSPORT-LIVENESS-002: Watchdog ownership and termination

**Intent:** Liveness detection must not be paid for with background work that
outlives the connection it was watching. The watchdog belongs to one session,
starts once, and is gone when that session is gone.

#### Acceptance criteria

- **AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.1:** A session shall have at most
  one watchdog at any time. Recording a session in the executor's tracked
  sessions shall not replace an existing record for the same executor instance
  identifier. When a record already exists at that point, the system shall leave
  the existing record and its watchdog in place, start no additional watchdog,
  and close the newly obtained port forward and session SSH client that would
  have been recorded, so neither a connection nor a watchdog is left unowned. The caller that lost shall proceed with the existing session on the
  same terms as any other reuse, including the refusal required by
  `AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.14` when that session's transport has
  been declared lost.
- **AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.2:** An SSH connection not owned by
  a session shall not start a watchdog. This covers the connection test used when
  configuring an executor, remote task-directory reclamation, and the untracked
  dial used to reset a credential-broker-backed resume.
- **AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.3:** When a session has no session
  SSH client, the system shall not start a watchdog and shall not report an
  error.
- **AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.4:** When a session is stopped, the
  SSH executor is closed, or a tracked session is discarded while resetting a
  credential-broker-backed resume, the system shall stop that session's watchdog,
  and no watchdog activity for it shall remain running once that stop, close, or
  reset call returns. Those three are the complete set of
  paths that dispose of a tracked session. Stopping a watchdog that has already
  stopped, or one that was never started, shall make no further calls and shall
  report success. This guarantee binds the disposal call that removed the
  session from the executor's tracked sessions. When two disposal calls run
  concurrently for the same session, the one that does not remove it finds no
  tracked session and shall return success immediately, closing no handle and
  not waiting for the other call's watchdog; the removing call remains bound by
  the guarantee above.
- **AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.5:** Transport teardown for a
  session shall run at most once. A second teardown for the same session shall
  close nothing further and shall report success.
- **AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.6:** Transport teardown shall not
  remove the session from the executor's tracked sessions; removal remains the
  disposal paths' responsibility.
- **AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.7:** When transport teardown and a
  session stop run concurrently for the same session, the local port forward
  shall be closed exactly once, the session SSH client shall be closed exactly
  once, and both operations shall complete without error.
- **AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.8:** When a disposal path would
  issue a remote command over a session SSH client whose transport has been
  declared lost, or whose transport is unresponsive, the system shall skip that
  command. For a session stop this means skipping both the remote cleanup script
  and the remote controller stop, and completing the stop without error. The
  skips shall be decided from a single reading covering both the transport-lost
  marker and the silence interval, taken before that path issues any remote
  command and no later than the session's removal from the executor's tracked
  sessions; that one reading shall govern every remote command on that disposal
  path, so no path can run one remote operation and skip another. A teardown
  firing after that reading shall not cause a stop to return an error. Every
  remote command a disposal path issues shall run under its own remote-cleanup
  timeout, including one that has none today. When a disposal does issue a remote
  command and that command has not completed within that timeout, the system
  shall close the session SSH client without waiting for the command to finish,
  mark that client transport-lost, treat the command as failed, and continue the
  disposal. The disposal shall not return until the abandoned command has ended;
  closing the client bounds that wait, because the close is what releases a
  command blocked opening its channel. When the session has no running watchdog,
  neither skip condition holds and the disposal issues every remote command, still
  bounded by the timeout above.
- **AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.9:** When the probe interval or the
  liveness deadline is not positive, or the deadline is not greater than twice
  the interval, the system shall not start a watchdog and shall record one
  debug-level log naming the executor instance identifier.
- **AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.10:** When a session is stopped
  while its transport is still alive, the system shall not declare that
  session's transport lost and shall not record the transport-loss warning for
  it.
- **AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.11:** When a session is stopped
  while its transport is unresponsive but its liveness deadline has not yet
  expired, the stop shall complete without waiting for that deadline, without
  waiting for any outstanding probe to be answered, and without issuing any
  remote command over that session's SSH client.
- **AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.12:** When transport teardown
  completes for a session, the system shall send no further transport liveness
  probes on that session's SSH client, whether or not that session is
  subsequently stopped.
- **AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.13:** When a session whose transport
  is answering normally is disposed of between two probes, the disposal shall not
  wait for the watchdog's next probe to be sent, and the time it spends waiting
  for that session's watchdog activity to finish shall total less than one probe
  interval. This bounds only the waiting the watchdog imposes; the disposal's own
  remote commands are not bounded by it. It is observed on a disposal path with
  no remote commands, where the whole call time is that waiting.
- **AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.14:** When the instance-creation
  path finds an already-tracked session for the requested executor instance
  identifier whose transport has been declared lost, it shall not return an
  executor instance for it, and shall report an error identifying transport
  loss.
- **AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.15:** When resetting a
  credential-broker-backed resume discards a tracked session whose transport is
  lost or unresponsive, the reset shall skip both its credential-broker
  reachability preflight and the remote controller stop, leave
  the session's persisted resume metadata unchanged, and report an error
  identifying transport loss. It shall report that same error, rather than a
  generic remote-stop or broker-unreachable failure, when it does issue its
  preflight or its remote controller stop and that call fails because the session
  SSH client was closed after the reading required by
  `AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.8`, whether by a teardown or by that
  criterion's own timeout. The transport-lost mark that criterion requires is what
  distinguishes those two causes from a host that refused the call.

## Out of scope

Each exclusion below is a deliberate contract boundary, not an oversight.

- **Reconnecting a session whose transport was lost.** Teardown ends the
  transport and does not re-dial, re-forward, or re-attach. The SSH executor is
  resumable, so a later resume re-establishes the connection.
- **Task, session, and agent state transitions.** This capability does not fail,
  retry, restart, or re-queue the agent's work. It closes the transport; existing
  disconnect handling decides what the user sees.
- **Operator configuration and feature toggles.** The interval and deadline are
  fixed internal values (`AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.10`): no config
  key, environment variable, or runtime flag. A link failing the deadline has been
  unusable for 45 seconds; nothing to tune.
- **The two downstream diagnosability defects.** An empty response payload read
  as success, and the agent's unbounded reconnect backoff, are on their own cards;
  this capability neither fixes nor depends on them.
- **The shape of the remote-status polling loop.** It visits sessions one at a
  time and can be delayed by one that is not answering. This capability bounds
  that delay, because a lost transport is closed and short-circuits status
  (`AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.7`), but does not restructure it.
- **Hardening `SSHPortForwarder.Close` itself.** It is idempotent for sequential
  callers and races for concurrent ones. This capability serialises every close
  of a session's forwarder through that session's once-guard, so no path it
  introduces reaches the race, and making the forwarder internally safe would
  change shared code for no behaviour this contract observes. Named because a
  reader will notice it, not as deferred work.
- **Shortening remote-command timeouts.** Disposal skips its remote commands when
  the transport is lost or unresponsive, and abandons one that outlives its own
  timeout, both under `AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-002.8`. Neither
  shortens those timeouts: a transport answering when the reading is taken runs
  its commands on the same clock as today.
- **Other executors.** Local, Docker, Kubernetes and Sprites executors have their
  own transports and are unchanged.
- **Application-level liveness above the transport.** Health, ping, or heartbeat
  traffic on the controller's endpoints is a separate concern from whether the
  SSH connection carries bytes.
- **Ordered or persisted collections.** This capability introduces no query,
  list, or stored record, so no sort order or tiebreak applies. Its only
  orderings are `AC-EXECUTORS-SSH-TRANSPORT-LIVENESS-001.5`'s teardown sequence
  and the disposal sequence in the design.
- **Multi-hop ProxyJump chains.** One bastion hop is the supported topology.

## System design

See [SSH session transport liveness](../system-design/ssh-transport-liveness.md).
