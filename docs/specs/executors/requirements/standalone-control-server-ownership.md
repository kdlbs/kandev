---
status: draft
system: executors
created: 2026-09-02
owners:
  - kandev
---

# Standalone Control-Server Ownership Requirements

## Overview

The standalone runtime runs one agentctl control server per host, supervising
every worktree and local agent instance. Today that server is bound to the
backend that spawned it: it dies with the backend, so the questions of who owns
it, whether it is the one this installation started, and what should happen when
nobody owns it have never had to be answered.

[Agent survival across a backend restart](agent-survival-across-restart.md)
makes the server outlive its backend. This document owns admission: what a
backend must prove before it may adopt a surviving control server, and when it
must refuse across an incompatible protocol change. What happens after admission,
which backend drives the server and what it does when nobody does, is stated in
[standalone control-server single driver and unowned lifetime](standalone-control-server-single-driver.md).

This contract is stated separately from session survival because it governs the
control server's identity rather than any one session's. It remains true when no
session is being recovered, and the SSH and Kubernetes executors, which also run
agentctl processes this backend does not own outright, adopt the same contract as
they gain survival.

This system owns the contract because executor ownership includes process and
port safety and executor-specific failure and recovery contracts.

## Terminology

These definitions are shared by every document in this feature; the others
reference them rather than restating them.

- **Control server:** The single standalone agentctl process supervising every
  worktree and local agent instance on this host.
- **Instance:** One supervised agent execution inside it, owning one agent
  subprocess and one workspace.
- **Adoption:** A newly started backend taking ownership of a control server
  that outlived the previous backend, and re-tracking its instances.
- **Unowned period:** The bounded time a control server tolerates having no
  owning backend before it stops its instances and exits.
- **Capability set:** The named protocol capabilities a control server
  advertises, matched against an adopting backend's required set.
- **Recovery inventory:** The durable executor records naming which agent
  executions were live when the backend exited.
- **Owning backend:** The single backend whose ownership claim is current, per
  REQ-EXECUTORS-CONTROL-OWNERSHIP-003. There is at most one at any moment.
- **Detached control server:** One whose lifetime is not bound to that of the
  backend that spawned it.
- **Survivable shutdown:** A backend shutdown leaving the control server and its
  agent subprocesses running rather than stopping them.
- **Re-tracking:** Rebuilding a backend-side execution for a surviving instance so
  the session becomes operable again.
- **Reconciliation pass:** One run of startup state reconciliation over every
  standalone recovery-inventory record. A liveness classification made for a single
  record outside such a run is not part of a pass.
- **Attached:** Said of a control server, it means an owning backend exists. Said of
  one instance, it means an owning backend exists *and* that instance's event and
  workspace streams to it are established. The two are not the same moment: a backend
  that has adopted but has not yet established an instance's streams is attached to
  the server and not attached to that instance. **Detached** is the negation, at
  whichever of the two granularities the using criterion names.

## Requirements

### REQ-EXECUTORS-CONTROL-OWNERSHIP-001: Adoption requires proof of ownership

**Intent:** A process answering on the control port is untrusted: it may belong to
another installation, another user, or a manual start, and adopting it would let
one installation drive another's agents.

**User story:** As a Kandev operator, I want a backend to adopt only a control
server it can prove is its own, so two installations on one machine never drive
each other's agents.

#### Acceptance criteria

- **AC-EXECUTORS-CONTROL-OWNERSHIP-001.1:** When the system starts a control
  server, it shall durably record that server's control endpoint, its identity,
  the reference to its ownership credential, the capability set observed, and the
  location of its diagnostic output, in a single record scoped to the installation
  rather than to any session, so a restarted backend locates it even when it was
  placed on a port other than the configured one. Exactly one such record shall
  exist per installation. It shall not be derived from, or stored on, the
  per-session recovery-inventory records, so that it remains present and
  authoritative when no session is live, and so that no tiebreak between
  disagreeing copies is ever required.
- **AC-EXECUTORS-CONTROL-OWNERSHIP-001.2:** When a backend evaluates a control
  server for adoption, the system shall require both a recorded-identity match
  and successful authentication with the stored credential before issuing any
  operation other than identity and capability retrieval.
- **AC-EXECUTORS-CONTROL-OWNERSHIP-001.3:** The system shall evaluate identity and
  authentication before capability compatibility, and when either fails it shall
  record that reason rather than an incompatibility reason.
- **AC-EXECUTORS-CONTROL-OWNERSHIP-001.4:** When identity does not match or
  authentication fails, the system shall refuse adoption, shall issue no further
  operation of any kind to that process including a stop, and shall start its own
  control server on a different port. A process this installation cannot prove is
  its own is never stopped.
- **AC-EXECUTORS-CONTROL-OWNERSHIP-001.5:** When a control server does not
  advertise an identity or a capability set at all, the system shall treat it as a
  process it cannot prove is its own and shall apply
  AC-EXECUTORS-CONTROL-OWNERSHIP-001.4.
- **AC-EXECUTORS-CONTROL-OWNERSHIP-001.6:** When adoption is refused, the system
  shall record the refusal and exactly one reason, drawn from identity mismatch,
  credential unavailable, authentication failure, capability incompatibility,
  credential-rotation failure, and no answer. When more than one would apply, the
  recorded reason shall be the first that applies in this order: no answer, identity
  mismatch, credential unavailable, authentication failure, capability
  incompatibility, credential-rotation failure. Credential unavailable means the
  stored credential could not be retrieved, so no authentication was attempted; it is
  distinct from authentication failure, which requires an attempt that the control
  server rejected. A control
  server that answers but advertises no identity, or an identity that cannot be
  read, is an identity mismatch and not a no answer, so that a reachable foreign
  process is never recorded as an unreachable one.
- **AC-EXECUTORS-CONTROL-OWNERSHIP-001.9:** When the stored credential cannot be
  retrieved, the system shall refuse adoption with the credential-unavailable reason,
  shall issue no further operation to that control server including a stop, shall
  leave it running, and shall start its own control server on a different port. It
  shall not treat the server as foreign and shall not delete the stored credential itself
  from the secret store, because the failure is on this side and destroying a secret over
  a transient read error is unrecoverable. The control-server record, however, shall be
  rewritten to describe the newly started server as AC-EXECUTORS-CONTROL-OWNERSHIP-001.1
  requires, because exactly one such record exists per installation and it names the
  server this installation is actually using. The surviving server is then reclaimed by
  the unowned shutdown of REQ-EXECUTORS-CONTROL-OWNERSHIP-003 rather than by this backend.
  Retaining the old reference in the record instead would satisfy nothing: the next start
  would be pointed at a server that has since terminated itself, while the server this
  backend just started would be the unlocatable one. Preserving the secret and rewriting
  the record is therefore the only combination consistent with a single
  installation-scoped record.
- **AC-EXECUTORS-CONTROL-OWNERSHIP-001.10:** Proof of ownership shall run in both
  directions. Before the system transmits the stored credential to a control server, and
  before it accepts or durably stores any credential replacement that server returns, it
  shall require that server to demonstrate that it already holds the stored credential.
  The demonstration shall be a challenge the adopting backend generates freshly for each
  adoption attempt and a response derivable only from possession of that credential; the
  credential itself shall not be sent as the demonstration, and a response shall never be
  accepted for a challenge the backend did not generate for that attempt. A
  recorded-identity match shall not by itself authorize transmitting the credential.
  Without this, adoption proves only that the *backend* holds the credential, which is the
  wrong direction: the values compared by
  AC-EXECUTORS-CONTROL-OWNERSHIP-001.2 are retrieved from the counterparty over an
  interface that AC-EXECUTORS-CONTROL-OWNERSHIP-004.6 places below authentication, so any
  process that can read that interface can replay them, and a process that has taken the
  recorded endpoint would then be handed the credential and have the replacement it
  invents persisted in the secret store. That process becomes the control server this
  backend drives for the remainder of its life, which is precisely the outcome
  REQ-EXECUTORS-CONTROL-OWNERSHIP-001 exists to prevent.
- **AC-EXECUTORS-CONTROL-OWNERSHIP-001.11:** The identity and capability retrieval of
  AC-EXECUTORS-CONTROL-OWNERSHIP-004.6, being available without authentication, shall
  disclose only what the identity comparison and the capability negotiation require: the
  opaque per-launch identity value, the advertised capability set, and the server's own
  resolved unowned period. It shall not disclose any filesystem path. Any further value an
  adopting backend needs shall be retrieved only after authentication has succeeded.
- **AC-EXECUTORS-CONTROL-OWNERSHIP-001.7:** When the system decides whether a
  control port is occupied, it shall use the probe contract defined by
  [port collision and backend ownership safety](port-collision-safety.md), so an
  active listener is never reported as absent.
- **AC-EXECUTORS-CONTROL-OWNERSHIP-001.8:** When no recorded control endpoint
  exists, when the record names an endpoint that nothing answers, or when the control
  server answers that it has already begun an unowned shutdown under
  [AC-EXECUTORS-CONTROL-OWNERSHIP-003.9](standalone-control-server-single-driver.md), the
  system shall start a fresh control server and shall report no recovered instances. A
  server that is already shutting down is grouped here rather than with the refusals of
  AC-EXECUTORS-CONTROL-OWNERSHIP-001.6 because nothing about it was refused: it is a server
  that will not be there, and it needs no refusal reason of its own.

### REQ-EXECUTORS-CONTROL-OWNERSHIP-004: An upgrade never adopts an incompatible control server

**Intent:** An upgrade replaces the control-server binary while the surviving
server keeps running the previous build. Adopting across an incompatible protocol
change produces failures that are hard to attribute to the upgrade.

**User story:** As a Kandev operator, I want an upgrade to refuse an incompatible
adoption rather than half-adopt, so an upgrade never corrupts a running
session.

#### Acceptance criteria

- **AC-EXECUTORS-CONTROL-OWNERSHIP-004.1:** When a backend evaluates a control
  server for adoption, the system shall compare that server's advertised
  capability set against its own required set before issuing any operation
  other than identity and capability retrieval, the mutual proof of ownership of
  AC-EXECUTORS-CONTROL-OWNERSHIP-001.10, and the credential replacement that carries it.
  Those three are excepted because AC-EXECUTORS-CONTROL-OWNERSHIP-001.3 requires identity
  and authentication to be evaluated *before* compatibility, and authenticating is itself
  an operation: without this exception the two criteria could not both be satisfied. The
  exception is safe because none of the three drives an instance or observes a
  transcript, and an incompatible server discovered immediately afterwards is stopped by
  AC-EXECUTORS-CONTROL-OWNERSHIP-004.3.
- **AC-EXECUTORS-CONTROL-OWNERSHIP-004.2:** Compatibility shall be decided by
  whether the required capabilities are all advertised, and shall not be
  decided by comparing version ordering.
- **AC-EXECUTORS-CONTROL-OWNERSHIP-004.3:** When the required capabilities are not
  all advertised and identity and authentication have both succeeded, the system
  shall refuse adoption, stop that control server and its instances, repair their
  recovery-inventory records itself once that stop has succeeded, and then start its own
  control server. When the stop does not succeed,
  AC-EXECUTORS-CONTROL-OWNERSHIP-004.7 governs and no record is repaired. That
  repair shall preserve the resume token and worktree identity exactly as
  AC-EXECUTORS-CONTROL-OWNERSHIP-003.5 requires of the same repair, but shall be
  performed immediately rather than deferred: 003.5 defers only because an unowned
  shutdown happens with no backend attached, and here a backend is attached and can
  write the durable store.
- **AC-EXECUTORS-CONTROL-OWNERSHIP-004.4:** When an incompatible control server is
  stopped, the affected sessions shall reach the same state and remain as
  resumable as they would after a restart with the capability disabled.
- **AC-EXECUTORS-CONTROL-OWNERSHIP-004.5:** When a control server is started or
  adopted, the system shall durably record the capability set it observed.
- **AC-EXECUTORS-CONTROL-OWNERSHIP-004.6:** Retrieving a control server's identity and
  capability set, and the ownership-shutdown operation of
  [AC-EXECUTORS-CONTROL-OWNERSHIP-002.9](standalone-control-server-single-driver.md),
  shall be available on every control server this contract covers, shall not be members of
  the advertised capability set, and shall not be subject to the compatibility comparison
  of AC-EXECUTORS-CONTROL-OWNERSHIP-004.1. Without that floor the operations this document
  depends on to refuse safely would themselves be negotiated: identity retrieval decides
  compatibility and so cannot depend on it, and
  AC-EXECUTORS-CONTROL-OWNERSHIP-004.3 and AC-EXECUTORS-SURVIVAL-005.5 both have to stop a
  server precisely when its capability set did not match, which is unsatisfiable if the
  stop is one of the capabilities that failed to match. A control server that does not
  answer identity retrieval is covered by AC-EXECUTORS-CONTROL-OWNERSHIP-001.5 and is never
  stopped; one that answers identity but not the ownership shutdown is treated by
  AC-EXECUTORS-CONTROL-OWNERSHIP-004.7 as a stop that did not succeed.
- **AC-EXECUTORS-CONTROL-OWNERSHIP-004.7:** When the stop that
  AC-EXECUTORS-CONTROL-OWNERSHIP-004.3 or AC-EXECUTORS-SURVIVAL-005.5 requires does not
  succeed, meaning the control server returned an error, did not answer, or did not expose
  the operation, the system shall retry it within the same per-read timeout and retry count
  as [AC-EXECUTORS-SURVIVAL-002.13](agent-survival-across-restart.md), and a control server
  reporting that it is already stopping or already absent shall count as success. When the
  retries are exhausted the system shall record the failure with the control-server
  identity and the reason, shall repair no recovery-inventory record for that server,
  shall report no recovered instances, and shall start its own control server on a
  different port so that this installation is not left without one. It shall not issue a
  further stop, and it shall not delete the surviving server's credential from the secret
  store. As in AC-EXECUTORS-CONTROL-OWNERSHIP-001.9 the control-server record itself is
  rewritten to describe the newly started server, because only one such record exists and
  it must name the server this installation is using.
  Repairing records for instances that are still running is the outcome this criterion
  exists to prevent: a record repaired while its process lives is repaired by nothing
  afterwards. The surviving server is instead reclaimed by the unowned shutdown of
  REQ-EXECUTORS-CONTROL-OWNERSHIP-003, because nothing renews its ownership from this
  point, and its records are repaired by the next backend through the same deferred path
  AC-EXECUTORS-CONTROL-OWNERSHIP-003.5 already defines.

## Related design

- [Part 1: lifetime and ownership contracts](../system-design/agent-survival-across-restart-01.md)
- [Part 2: control flow, failure and scope](../system-design/agent-survival-across-restart-02.md)
- [Part 3: recovery data contracts](../system-design/agent-survival-across-restart-03.md)

## Out of scope

- Which backend drives an adopted control server, and what an unowned server does,
  both owned by
  [standalone control-server single driver and unowned lifetime](standalone-control-server-single-driver.md).
- Which sessions are re-tracked after adoption, owned by
  [agent survival across a backend restart](agent-survival-across-restart.md), and how
  their state is made truthful, owned by
  [survived session state and capability gating](agent-survival-session-state.md).
- Refusing a second backend on one installation, and the port-availability probe
  this contract consumes, both owned by
  [port collision and backend ownership safety](port-collision-safety.md).
- Ownership of agentctl processes on the Docker, SSH, Sprites, Kubernetes, and
  remote-Docker executors. They adopt this contract in later work.
- Running two control-server generations at once. An incompatible server is
  stopped (AC-EXECUTORS-CONTROL-OWNERSHIP-004.3), not drained alongside a new
  one.
