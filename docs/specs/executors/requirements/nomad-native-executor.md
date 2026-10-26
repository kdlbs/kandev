---
status: draft
system: executors
created: 2026-10-24
owners:
  - kandev
---

# Native Nomad executor requirements

## Overview

Operators can run a real Kandev session as native processes directly on a
dedicated Nomad worker's host operating system, without a container or VM
runtime. Native execution is not specific to macOS; macOS is the first
validation target in this package. Support for another host platform requires
its own verified runtime, transport and cleanup capabilities. Nomad owns
placement and resource reservations; Kandev owns the environment and agent
lifecycle. The executor system owns this contract because environment admission,
lifetime and cleanup remain meaningful independently of a task's UI or conversation.

This capability is a built-in executor using the existing executor lifecycle and
secure SSH transport primitives. Nomad owns launch; Kandev retains its agent protocol.
The initial support boundary is trusted workloads whose descendants remain in
agentctl-managed process groups. It is not a sandbox or a claim that arbitrary
daemonized native processes cannot escape. Uncertainty is a retained failure,
not successful release.

## Terminology

- **Worker:** A dedicated, explicitly selected Nomad client node whose host OS
  runs agentctl and agent workloads directly as native processes.
- **Environment:** One Kandev-owned workspace and compute lifetime, independent
  of a single prompt or task status; initially one isolated session environment.
- **Cooperative cleanup:** Authenticated closure of runtime admission followed
  by successful teardown of every managed instance and provisional process group.
- **Quarantine:** A durable refusal to admit more work on a worker whose owned
  process cleanup is uncertain, while retaining the exact cleanup inventory.

## Requirements

### REQ-EXECUTORS-NOMAD-001: Native admission and lifetime

**Intent:** Operators reserve real capacity and retain one exact environment.

#### Acceptance criteria

- **AC-EXECUTORS-NOMAD-001.1:** When a supported profile launches, the system
  shall use one explicitly selected native worker and the host-selected runtime,
  without requiring Docker, a VM, or an alternate scheduler. Unsupported OS,
  driver, toolchain, topology, or cleanup capability shall reject admission with
  the incompatible prerequisite identified.
- **AC-EXECUTORS-NOMAD-001.2:** When capacity is insufficient for a second
  environment's declared CPU and memory reservations, Nomad shall leave that
  environment unallocated. One job group count shall not be presented as a
  worker capacity limit; native reservations shall not be described as hard
  operating-system limits on macOS or on processes outside this managed pool.
- **AC-EXECUTORS-NOMAD-001.3:** When a prompt completes, an ordinary agent Stop
  occurs, or the backend restarts, the environment shall remain recorded for its
  existing authorized reuse lifetime. Shared-session workspaces and automatic
  replacement of lost workspaces shall reject admission.

### REQ-EXECUTORS-NOMAD-002: Exact ownership and recovery

**Intent:** Retried or interrupted operations cannot allocate or destroy a successor.

#### Acceptance criteria

- **AC-EXECUTORS-NOMAD-002.1:** When a provision response is lost or the provider
  or backend restarts, recovery shall find the original operation's exact
  environment, prove no allocation request was committed, or report unknown.
  Unknown outcomes shall prevent replacement; changed inputs under one operation
  identity shall conflict.
- **AC-EXECUTORS-NOMAD-002.2:** When attachment, inspection, or destruction
  receives stale ownership, a different allocation or node identity, or ambiguous
  retained history, it shall reject the mutation and retain the original record.
- **AC-EXECUTORS-NOMAD-002.3:** When the Nomad client is disconnected while its
  agent continues running, the system shall retain that environment as uncertain
  and shall not start duplicate work on that or another node, including an already
  queued second environment after a runtime leader crash while the backend is
  down. Reconnection shall
  reconcile the original allocation before admitting more work.

### REQ-EXECUTORS-NOMAD-003: Cooperative cleanup and quarantine

**Intent:** Successful release has positive evidence; native uncertainty remains visible.

#### Acceptance criteria

- **AC-EXECUTORS-NOMAD-003.1:** When authorized terminal cleanup reaches a
  compatible live runtime, the system shall close all instance and process-start
  admission, drain admitted creation and provisional cleanup, and acknowledge
  success only after every managed process group has stopped. The runtime shall
  remain sealed after that acknowledgement.
- **AC-EXECUTORS-NOMAD-003.2:** When terminal cleanup reports absence, it shall
  have both successful cooperative cleanup evidence for the exact runtime and
  ownership generation and confirmation that the whole owned Nomad job and its
  recorded allocation have stopped on the known live worker. Repeated cleanup
  shall be idempotent. A delayed receipt or claim shall not authorize a successor.
- **AC-EXECUTORS-NOMAD-003.3:** When teardown fails, its response is lost before
  durable receipt, the runtime identity changes, the worker is lost, or process
  evidence is incomplete, the system shall retain cleanup-pending inventory and
  quarantine the dedicated worker. A terminal allocation, missing job, empty
  allocation list, expired history, shutdown acknowledgement, or closed TCP port
  alone shall not prove managed-process absence.
- **AC-EXECUTORS-NOMAD-003.4:** When quarantine cannot be resolved through the
  original successful cleanup evidence, the system shall require an explicit
  operator recovery procedure with externally verified process absence before
  worker readmission. It shall not automatically clear quarantine or delete
  retained records to make uninstall succeed.

### REQ-EXECUTORS-NOMAD-004: Authenticated runtime delivery and transport

**Intent:** Native execution preserves normal Kandev control and credential boundaries.

#### Acceptance criteria

- **AC-EXECUTORS-NOMAD-004.1:** When the runtime is delivered to a local or remote
  worker, the exact host-selected platform artifact shall be integrity checked
  before execution. Bootstrap secrets and SSH credentials shall not appear in job
  specifications, resource state, command arguments, ordinary logs, or results.
- **AC-EXECUTORS-NOMAD-004.2:** When the backend connects to control or instance
  endpoints, it shall use existing authenticated SSH forwarding with a pinned
  worker host key,
  preserve Kandev bearer authentication and runtime identity, and support HTTP
  and WebSocket traffic. Agentctl shall remain reachable only through loopback
  or the authenticated tunnel; colocation shall not expose an unauthenticated
  public runtime port. Reconnection shall use the same environment and restored
  authenticated transport without a new prompt or allocation.
- **AC-EXECUTORS-NOMAD-004.3:** When setup, cancellation, transport, or recovery
  fails, the user shall receive existing sanitized progress and retryable error
  outcomes; logs shall identify operation, environment, allocation, and node
  without exposing credentials or agent payloads. Declared file, terminal, Git,
  prompt, and result flows shall use Kandev's existing agentctl surfaces.

### REQ-EXECUTORS-NOMAD-005: Installable contribution and honest evidence

**Intent:** The built-in contribution is usable and reviewable upstream.

#### Acceptance criteria

- **AC-EXECUTORS-NOMAD-005.1:** When an operator runs a built Kandev runtime
  in a disposable instance, its built-in Nomad executor shall validate and
  launch through the existing executor profile mechanism. Nomad-specific logic
  shall remain cohesive and separate from shared agent protocol behavior.
- **AC-EXECUTORS-NOMAD-005.2:** Before supported native execution is claimed,
  recorded real end-to-end evidence shall include Kandev durable inventory
  acceptance, authenticated control handshake, an inert ACP child and grandchild,
  real A/B resource admission,
  operation response loss, backend restart, actual Nomad client
  disconnection, positive cooperative cancellation, and negative cleanup with
  quarantine. Missing prerequisites shall be reported as unverified; a skip,
  prepare-only run, mock, or process health check shall not satisfy these flows.

## Out of scope

- Docker, container images, VM provisioning, a custom supervisor or scheduler.
- Hard isolation of hostile native workloads, daemonized process escape, general
  macOS CPU/memory enforcement, or automatic proof after worker/runtime loss.
- Rendered UI and production rollout.
- Automatic support promotion to bare-metal Linux. Nomad exec with cgroups is
  a possible later Docker-free boundary with its own proof and capability profile.
- Placement across a pool of workers and more than one environment per worker;
  see [Nomad worker pool](nomad-worker-pool.md).


## Implementation plans

- [Native Nomad executor](../../../plans/nomad-native-executor/plan.md)
